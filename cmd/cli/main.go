package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/config"
	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/languages"
	languagespgxgoqu "github.com/radynsade/faryengo/internal/languages/pgxgoqu"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/users/argon2id"
	userspgxgoqu "github.com/radynsade/faryengo/internal/users/pgxgoqu"
)

//
// Usage
//

const usage = `Usage:
  bin/cli help
  bin/cli languages create-language <code> <englishName> <nativeName> [--fallback|-f]
  bin/cli languages delete-language <code>
  bin/cli users create-role <nameTranslations> [--super|-s] [--permission|-p <permission>]...
  bin/cli users delete-role <roleUUID>
  bin/cli users create-user <email> <firstName> <lastName> <password> <phone> <roleUUID>
  bin/cli users delete-user <userUUID>`

var (
	ErrCommandInvalid     = errors.New("invalid command")
	ErrUUIDInvalid        = errors.New("invalid UUID: use the hyphenated format")
	ErrDatabaseURLMissing = errors.New("PostgreSQL connection string is required: set DATABASE_URL or add it to .env")
)

//
// Command
//

type services struct {
	languages *app.LanguageService
	roles     *app.RoleService
	users     *app.UserService
}

type command interface {
	execute(ctx context.Context, services services, stdout io.Writer) error
}

func parseCommand(args []string) (command, error) {
	var (
		result command
		err    error
	)

	if len(args) < 2 {
		err = ErrCommandInvalid
	} else {
		switch {
		case args[0] == languagesGroup:
			result, err = asCommand(parseLanguageCommand(args))
		case args[0] == usersGroup && (args[1] == createRoleCommand || args[1] == deleteRoleCommand):
			result, err = asCommand(parseRoleCommand(args))
		case args[0] == usersGroup:
			result, err = asCommand(parseUserCommand(args))
		default:
			err = ErrCommandInvalid
		}
	}

	if err != nil {
		result = nil
	}

	return result, err
}

func asCommand[Command command](parsed Command, err error) (command, error) {
	return parsed, err
}

//
// Entry point
//

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)

	stop()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Arguments are parsed before configuration is read, so a malformed command
// fails without a database connection.

func run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
) error {
	var err error

	if isHelp(args) {
		if _, writeErr := fmt.Fprintln(stdout, usage); writeErr != nil {
			err = fmt.Errorf("write usage: %w", writeErr)
		}
	} else {
		parsed, parseErr := parseCommand(args)

		if parseErr != nil {
			err = fmt.Errorf("parse command: %w", parseErr)

			if _, writeErr := fmt.Fprintln(stderr, usage); writeErr != nil {
				err = errors.Join(err, fmt.Errorf("write usage: %w", writeErr))
			}
		} else {
			err = runCommand(ctx, parsed, stdout)
		}
	}

	return err
}

func runCommand(
	ctx context.Context,
	parsed command,
	stdout io.Writer,
) error {
	var (
		settings config.Config
		pool     *pgxpool.Pool
		err      error
	)

	settings, err = config.Load()

	if err != nil {
		err = fmt.Errorf("read configuration: %w", err)
	} else if strings.TrimSpace(settings.DatabaseURL) == "" {
		err = ErrDatabaseURLMissing
	} else {
		pool, err = pgxpool.New(ctx, settings.DatabaseURL)

		if err != nil {
			err = fmt.Errorf("configure PostgreSQL connection: %w", err)
		} else {
			defer pool.Close()

			err = pool.Ping(ctx)

			if err != nil {
				err = fmt.Errorf("connect to PostgreSQL: %w", err)
			}
		}
	}

	if err == nil {
		var wired services

		wired, err = wireServices(pool)

		if err == nil {
			err = parsed.execute(ctx, wired, stdout)
		}
	}

	return err
}

//
// Helpers
//

func isHelp(args []string) bool {
	return len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help")
}

func wireServices(pool *pgxpool.Pool) (services, error) {
	var (
		wired              services
		transactor         *pgxdb.Transactor
		languageRepository *languagespgxgoqu.LanguageRepository
		roleRepository     *userspgxgoqu.RoleRepository
		userRepository     *userspgxgoqu.UserRepository
		err                error
	)

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
		wired.languages, err = app.NewLanguageService(transactor, languageRepository)
	}

	if err == nil {
		wired.roles, err = app.NewRoleService(transactor, roleRepository, languageRepository)
	}

	if err == nil {
		wired.users, err = app.NewUserService(transactor, userRepository, argon2id.NewPasswordHasher())
	}

	if err != nil {
		err = fmt.Errorf("configure services: %w", err)
		wired = services{}
	}

	return wired, err
}

// Only the canonical 36-character form is accepted; uuid.Parse alone also
// accepts URN, braced, and unhyphenated forms.

func parseUUID(argument string) (uuid.UUID, error) {
	var (
		id  uuid.UUID
		err error
	)

	if len(argument) != 36 {
		err = ErrUUIDInvalid
	} else {
		id, err = uuid.Parse(argument)

		if err != nil {
			err = fmt.Errorf("%w: %w", ErrUUIDInvalid, err)
			id = uuid.Nil
		}
	}

	return id, err
}

// Repository write failures hide their causes from their messages, so known
// domain conflicts are named explicitly while infrastructure causes stay
// hidden.

func describeError(err error) error {
	result := err

	for _, conflict := range []error{
		languages.ErrLanguageNotFound,
		languages.ErrLanguageAlreadyExists,
		languages.ErrFallbackLanguageAlreadyExists,
		languages.ErrFallbackLanguageAlreadyInUse,
		languages.ErrLanguageInUse,
		users.ErrRoleNotFound,
		users.ErrRoleAlreadyExists,
		users.ErrRoleAlreadyInUse,
		users.ErrUserNotFound,
		users.ErrUserAlreadyExists,
	} {
		if errors.Is(err, conflict) {
			result = fmt.Errorf("%w: %w", err, conflict)

			break
		}
	}

	return result
}
