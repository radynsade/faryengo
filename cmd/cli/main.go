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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/app"
	appinput "github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/config"
	"github.com/radynsade/faryengo/internal/languages"
	languagespgxgoqu "github.com/radynsade/faryengo/internal/languages/pgxgoqu"
)

const usage = `Usage:
  bin/cli languages create-language <code> <englishName> <nativeName> [--fallback|-f]
  bin/cli languages delete-language <code>
  bin/cli users create-user <email> <firstName> <lastName> <password> <phone> <roleUUID>
  bin/cli users delete-user <userUUID>
  bin/cli users create-role <nameTranslations> [--super|-s] [--permission|-p <permission>]...
  bin/cli users delete-role <roleUUID>`

var errInvalidCommand = errors.New("invalid command")

type cliCommand struct {
	group    string
	action   string
	language languageCommand
	user     userCommand
	role     roleCommand
}

type languageCommand struct {
	action      string
	createInput appinput.CreateLanguageInput
	code        string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		_, err := fmt.Fprintln(stdout, usage)
		if err != nil {
			err = fmt.Errorf("write CLI usage: %w", err)
		}

		return err
	}

	command, err := parseCommand(args)
	if err != nil {
		if _, writeErr := fmt.Fprintln(stderr, usage); writeErr != nil {
			return fmt.Errorf("write CLI usage: %w", writeErr)
		}

		return fmt.Errorf("parse CLI command: %w", err)
	}

	settings, err := config.Load()
	if err != nil {
		return fmt.Errorf("read configuration: %w", err)
	}

	if strings.TrimSpace(settings.DatabaseURL) == "" {
		return errors.New("PostgreSQL connection string is required: set DATABASE_URL or add it to .env")
	}

	pool, err := pgxpool.New(ctx, settings.DatabaseURL)
	if err != nil {
		return fmt.Errorf("configure PostgreSQL connection: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}

	switch command.group {
	case "languages":
		err = runLanguageCommand(ctx, command.language, pool, stdout)
	case "users":
		switch command.action {
		case "create-user", "delete-user":
			err = runUserCommand(ctx, command.user, pool, stdout)
		case "create-role", "delete-role":
			err = runRoleCommand(ctx, command.role, pool, stdout)
		default:
			err = errInvalidCommand
		}
	default:
		err = errInvalidCommand
	}

	return err
}

func parseCommand(args []string) (cliCommand, error) {
	var command cliCommand
	var err error

	if len(args) < 2 {
		err = errInvalidCommand
	} else {
		command.group = args[0]
		command.action = args[1]

		switch command.group {
		case "languages":
			command.language, err = parseLanguageCommand(args)
		case "users":
			switch command.action {
			case "create-user", "delete-user":
				command.user, err = parseUserCommand(args)
			case "create-role", "delete-role":
				command.role, err = parseRoleCommand(args)
			default:
				err = errInvalidCommand
			}
		default:
			err = errInvalidCommand
		}
	}

	if err != nil {
		command = cliCommand{}
	}

	return command, err
}

func runLanguageCommand(ctx context.Context, command languageCommand, pool *pgxpool.Pool, stdout io.Writer) error {
	repository, err := languagespgxgoqu.NewLanguageRepository(pool)
	if err != nil {
		return fmt.Errorf("configure language repository: %w", err)
	}

	service, err := app.NewLanguageService(repository)
	if err != nil {
		return fmt.Errorf("configure language service: %w", err)
	}

	return executeLanguageCommand(ctx, command, service, stdout)
}

func executeLanguageCommand(ctx context.Context, command languageCommand, service *app.LanguageService, stdout io.Writer) error {
	var err error

	switch command.action {
	case "create-language":
		var languageCode string
		language, createErr := service.Create(ctx, command.createInput)
		if createErr != nil {
			err = fmt.Errorf("run create-language command: %w", createErr)
		} else {
			languageCode = string(language.Code())
			if _, writeErr := fmt.Fprintf(stdout, "created language %s\n", languageCode); writeErr != nil {
				err = fmt.Errorf("write create-language result: %w", writeErr)
			}
		}
	case "delete-language":
		if deleteErr := service.Delete(ctx, command.code); deleteErr != nil {
			err = fmt.Errorf("run delete-language command: %w", deleteErr)
		} else if _, writeErr := fmt.Fprintf(stdout, "deleted language %s\n", command.code); writeErr != nil {
			err = fmt.Errorf("write delete-language result: %w", writeErr)
		}
	default:
		err = errInvalidCommand
	}

	return err
}

func parseLanguageCommand(args []string) (languageCommand, error) {
	var command languageCommand
	var err error

	if len(args) < 2 || args[0] != "languages" {
		err = errInvalidCommand
	} else {
		switch args[1] {
		case "create-language":
			command.createInput, err = parseCreateLanguage(args)
			command.action = "create-language"
		case "delete-language":
			command.code, err = parseDeleteLanguage(args)
			command.action = "delete-language"
		default:
			err = errInvalidCommand
		}
	}

	return command, err
}

func parseCreateLanguage(args []string) (appinput.CreateLanguageInput, error) {
	var input appinput.CreateLanguageInput
	var err error

	if len(args) < 2 || args[0] != "languages" || args[1] != "create-language" {
		err = errInvalidCommand
	} else {
		var positional []string
		parseOptions := true

		for _, argument := range args[2:] {
			switch {
			case parseOptions && argument == "--":
				parseOptions = false
			case parseOptions && (argument == "--fallback" || argument == "-f"):
				input.IsFallback = true
			case parseOptions && strings.HasPrefix(argument, "-"):
				err = errInvalidCommand
			default:
				positional = append(positional, argument)
			}
		}

		if err == nil {
			if len(positional) != 3 {
				err = errInvalidCommand
			} else {
				input.Code = positional[0]
				input.EnglishName = positional[1]
				input.NativeName = positional[2]

				_, codeErr := languages.NewLanguageCode(input.Code)
				_, englishErr := languages.NewLanguageEnglishName(input.EnglishName)
				_, nativeErr := languages.NewLanguageNativeName(input.NativeName)

				if valueErr := errors.Join(codeErr, englishErr, nativeErr); valueErr != nil {
					err = fmt.Errorf("create-language arguments: %w: %w", appinput.ErrInvalidCreateLanguageInput, valueErr)
				}
			}
		}
	}

	if err != nil {
		input = appinput.CreateLanguageInput{}
	}

	return input, err
}

func parseDeleteLanguage(args []string) (string, error) {
	var code string
	var err error

	if len(args) != 3 || args[0] != "languages" || args[1] != "delete-language" {
		err = errInvalidCommand
	} else {
		code = args[2]
	}

	return code, err
}
