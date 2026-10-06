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
	languagepg "github.com/radynsade/faryengo/internal/languages/pgxgoqu"
	"github.com/radynsade/faryengo/internal/security/argon2id"
	"github.com/radynsade/faryengo/internal/security/pgxgoqu"
	securityredis "github.com/radynsade/faryengo/internal/security/redis"
	"github.com/radynsade/faryengo/middleware"
	flashredis "github.com/radynsade/faryengo/pkg/flashmsg/redis"
	"github.com/radynsade/faryengo/web/admin"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	settings, err := config.Load()

	if err != nil {
		return fmt.Errorf("load server configuration: %w", err)
	}

	if strings.TrimSpace(settings.DatabaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, settings.DatabaseURL)

	if err != nil {
		return fmt.Errorf("configure PostgreSQL: %w", err)
	}

	defer pool.Close()
	redisOptions, err := redislib.ParseURL(settings.RedisURL)

	if err != nil {
		return errors.New("invalid REDIS_URL")
	}

	redisOptions.ContextTimeoutEnabled = true
	// A timed-out refresh rotation must not be automatically replayed.
	redisOptions.MaxRetries = -1
	client := redislib.NewClient(redisOptions)
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "close Redis", "error", closeErr)
		}
	}()

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(connectCtx); err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}

	if err := client.Ping(connectCtx).Err(); err != nil {
		return fmt.Errorf("connect to Redis: %w", err)
	}

	credentials, err := pgxgoqu.NewCredentialRepository(pool)

	if err != nil {
		return fmt.Errorf("configure credential repository: %w", err)
	}

	roles, err := pgxgoqu.NewRoleRepository(pool)

	if err != nil {
		return fmt.Errorf("configure authorization roles: %w", err)
	}

	sessions, err := securityredis.NewSessionStore(client)

	if err != nil {
		return fmt.Errorf("configure session store: %w", err)
	}

	limiter, err := securityredis.NewRateLimiter(client)

	if err != nil {
		return fmt.Errorf("configure sign-in limiter: %w", err)
	}

	service, err := app.NewAuthenticationService(ctx, app.AuthenticationDependencies{Credentials: credentials, Invalidator: credentials,
		Roles: roles, Hasher: argon2id.NewHasher(), Revoker: sessions})

	if err != nil {
		return fmt.Errorf("configure authentication: %w", err)
	}

	browserSessions, err := app.NewSessionAuthenticationService(service, sessions, settings.SessionTTL)

	if err != nil {
		return fmt.Errorf("configure browser authentication: %w", err)
	}

	roleService, err := app.NewRoleService(roles)

	if err != nil {
		return fmt.Errorf("configure role management: %w", err)
	}

	languageRepository, err := languagepg.NewLanguageRepository(pool)

	if err != nil {
		return fmt.Errorf("configure language repository: %w", err)
	}

	languageService, err := app.NewLanguageService(languageRepository)

	if err != nil {
		return fmt.Errorf("configure language service: %w", err)
	}

	flashes, err := flashredis.NewStore(client, "admin", 15*time.Minute)

	if err != nil {
		return fmt.Errorf("configure admin flash store: %w", err)
	}

	adminHandler, err := admin.NewHandler(browserSessions, roleService, languageService, limiter, flashes, settings.AuthCookieSecure)

	if err != nil {
		return fmt.Errorf("configure admin transport: %w", err)
	}

	mux := http.NewServeMux()

	if err := adminHandler.RegisterHandlers(mux); err != nil {
		return fmt.Errorf("register admin handlers: %w", err)
	}

	server := &http.Server{Addr: settings.HTTPAddress, Handler: middleware.RedirectTrailingSlash(mux),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	return serve(ctx, server)
}

func serve(ctx context.Context, server *http.Server) error {
	serveCtx, stop := context.WithCancel(ctx)
	defer stop()
	group, groupCtx := errgroup.WithContext(serveCtx)
	group.Go(func() error {
		defer stop()
		var err error

		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			err = fmt.Errorf("serve HTTP: %w", serveErr)
		}

		return err
	})
	group.Go(func() error {
		<-groupCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		err := server.Shutdown(shutdownCtx)

		if err != nil {
			// Shutdown timed out; force closure so the serving goroutine exits.
			closeErr := server.Close()
			err = fmt.Errorf("shut down HTTP: %w", errors.Join(err, closeErr))
		}

		return err
	})
	return group.Wait()
}
