package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	redislib "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/config"
	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	languagespgxgoqu "github.com/radynsade/faryengo/internal/languages/pgxgoqu"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	sessionidredis "github.com/radynsade/faryengo/internal/security/sessionid/redis"
	"github.com/radynsade/faryengo/internal/users/argon2id"
	userspgxgoqu "github.com/radynsade/faryengo/internal/users/pgxgoqu"
	"github.com/radynsade/faryengo/middleware"
	flashredis "github.com/radynsade/faryengo/pkg/flashmsg/redis"
	adminhandlers "github.com/radynsade/faryengo/web/admin/handlers"
	officehandlers "github.com/radynsade/faryengo/web/office/handlers"
)

//
// Entry point
//

const (
	connectTimeout  = 5 * time.Second
	shutdownTimeout = 10 * time.Second
	flashTTL        = 15 * time.Minute
)

var ErrDatabaseURLMissing = errors.New("DATABASE_URL is required")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx)

	stop()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	var (
		settings config.Config
		pool     *pgxpool.Pool
		client   *redislib.Client
		handler  *adminhandlers.Handler
		office   *officehandlers.Handler
		err      error
	)

	settings, err = config.Load()

	if err != nil {
		err = fmt.Errorf("load server configuration: %w", err)
	} else if strings.TrimSpace(settings.DatabaseURL) == "" {
		err = ErrDatabaseURLMissing
	} else {
		pool, err = pgxpool.New(ctx, settings.DatabaseURL)

		if err != nil {
			err = fmt.Errorf("configure PostgreSQL: %w", err)
		} else {
			defer pool.Close()
		}
	}

	if err == nil {
		client, err = newRedisClient(settings.RedisURL)

		if err == nil {
			defer closeRedis(ctx, client)
		}
	}

	if err == nil {
		err = ping(ctx, pool, client)
	}

	if err == nil {
		handler, office, err = wireHandlers(ctx, settings, pool, client)
	}

	if err == nil {
		mux := http.NewServeMux()

		err = handler.RegisterHandlers(mux)

		if err != nil {
			err = fmt.Errorf("register admin handlers: %w", err)
		} else if officeErr := office.RegisterHandlers(mux); officeErr != nil {
			err = fmt.Errorf("register office handlers: %w", officeErr)
		} else {
			err = serve(ctx, &http.Server{
				Addr:              settings.HTTPAddress,
				Handler:           middleware.RedirectTrailingSlash(mux),
				ReadHeaderTimeout: 5 * time.Second,
				ReadTimeout:       15 * time.Second,
				WriteTimeout:      30 * time.Second,
				IdleTimeout:       60 * time.Second,
			})
		}
	}

	return err
}

// The server stops accepting connections when the context ends and gives
// in-flight requests a bounded time to finish.

func serve(ctx context.Context, server *http.Server) error {
	serveCtx, stop := context.WithCancel(ctx)
	defer stop()

	group, groupCtx := errgroup.WithContext(serveCtx)

	group.Go(func() error {
		var err error

		defer stop()

		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			err = fmt.Errorf("serve HTTP: %w", serveErr)
		}

		return err
	})

	group.Go(func() error {
		<-groupCtx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()

		err := server.Shutdown(shutdownCtx)

		// A timed-out shutdown forces the remaining connections closed, so
		// the serving goroutine exits.
		if err != nil {
			err = fmt.Errorf("shut down HTTP: %w", errors.Join(err, server.Close()))
		}

		return err
	})

	return group.Wait()
}

//
// Helpers
//

// Redis holds Sessions and flash messages, whose writes must never be
// replayed after an ambiguous failure, so automatic retries are off.

func newRedisClient(redisURL string) (*redislib.Client, error) {
	var client *redislib.Client

	options, err := redislib.ParseURL(redisURL)

	if err != nil {
		err = errors.New("invalid REDIS_URL")
	} else {
		options.ContextTimeoutEnabled = true
		options.MaxRetries = -1
		client = redislib.NewClient(options)
	}

	return client, err
}

func closeRedis(ctx context.Context, client *redislib.Client) {
	if err := client.Close(); err != nil {
		slog.ErrorContext(ctx, "close Redis", "error", err)
	}
}

func ping(ctx context.Context, pool *pgxpool.Pool, client *redislib.Client) error {
	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	err := pool.Ping(connectCtx)

	if err != nil {
		err = fmt.Errorf("connect to PostgreSQL: %w", err)
	} else if pingErr := client.Ping(connectCtx).Err(); pingErr != nil {
		err = fmt.Errorf("connect to Redis: %w", pingErr)
	}

	return err
}

// The admin and the office share authentication, but each keeps its own
// session cookie.

func wireHandlers(
	ctx context.Context,
	settings config.Config,
	pool *pgxpool.Pool,
	client *redislib.Client,
) (*adminhandlers.Handler, *officehandlers.Handler, error) {
	var (
		handler            *adminhandlers.Handler
		office             *officehandlers.Handler
		transactor         *pgxdb.Transactor
		languageRepository *languagespgxgoqu.LanguageRepository
		roleRepository     *userspgxgoqu.RoleRepository
		userRepository     *userspgxgoqu.UserRepository
		credentials        *userspgxgoqu.CredentialsSnapshotRepository
		sessionStorage     *sessionidredis.SessionStorage
		passwords          *emailpass.Authenticator
		sessions           *sessionid.Authenticator
		identities         *security.IdentityResolver
		languages          *app.LanguageService
		userService        *app.UserService
		roles              *app.RoleService
		flashes            *flashredis.Store
		err                error
	)

	hasher := argon2id.NewPasswordHasher()
	transactor, err = pgxdb.NewTransactor(pool)

	if err == nil {
		languageRepository, err = languagespgxgoqu.NewLanguageRepository(pool)
	}

	if err == nil {
		roleRepository, err = userspgxgoqu.NewRoleRepository(pool)
	}

	if err == nil {
		userRepository, err = userspgxgoqu.NewUserRepository(pool)
	}

	if err == nil {
		credentials, err = userspgxgoqu.NewCredentialsRepository(pool)
	}

	if err == nil {
		sessionStorage, err = sessionidredis.NewSessionStorage(client)
	}

	if err == nil {
		passwords, err = emailpass.NewAuthenticator(ctx, credentials, hasher, settings.SessionTTL)
	}

	if err == nil {
		sessions, err = sessionid.NewAuthenticator(sessionStorage, credentials)
	}

	if err == nil {
		identities, err = security.NewIdentityResolver(credentials, userRepository, roleRepository)
	}

	if err == nil {
		languages, err = app.NewLanguageService(transactor, languageRepository)
	}

	if err == nil {
		userService, err = app.NewUserService(transactor, userRepository, hasher)
	}

	if err == nil {
		roles, err = app.NewRoleService(transactor, roleRepository, languageRepository)
	}

	if err == nil {
		flashes, err = flashredis.NewStore(client, "admin", flashTTL)
	}

	if err == nil {
		handler, err = adminhandlers.NewHandler(passwords, sessions, identities, userService, roles, languages, flashes, settings.AuthCookieSecure)
	}

	if err == nil {
		office, err = officehandlers.NewHandler(passwords, sessions, identities, settings.AuthCookieSecure)
	}

	if err != nil {
		handler, office = nil, nil
		err = fmt.Errorf("configure the web applications: %w", err)
	}

	return handler, office, err
}
