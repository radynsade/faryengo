package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
)

//
// Language command
//

const (
	languagesGroup        = "languages"
	createLanguageCommand = "create-language"
	deleteLanguageCommand = "delete-language"
)

type languageCommand struct {
	action      string
	createInput input.CreateLanguageInput
	code        string
}

func parseLanguageCommand(args []string) (languageCommand, error) {
	var (
		command languageCommand
		err     error
	)

	if len(args) < 2 || args[0] != languagesGroup {
		err = ErrCommandInvalid
	} else {
		command.action = args[1]

		switch command.action {
		case createLanguageCommand:
			command.createInput, err = parseCreateLanguage(args[2:])
		case deleteLanguageCommand:
			command.code, err = parseDeleteLanguage(args[2:])
		default:
			err = ErrCommandInvalid
		}
	}

	if err != nil {
		command = languageCommand{}
	}

	return command, err
}

// Executing a command

func (c languageCommand) execute(
	ctx context.Context,
	services services,
	stdout io.Writer,
) error {
	return executeLanguageCommand(ctx, c, services.languages, stdout)
}

func executeLanguageCommand(
	ctx context.Context,
	command languageCommand,
	service *app.LanguageService,
	stdout io.Writer,
) error {
	var err error

	switch command.action {
	case createLanguageCommand:
		language, createErr := service.Create(ctx, command.createInput)

		if createErr != nil {
			err = describeError(createErr)
		} else if _, writeErr := fmt.Fprintf(stdout, "created language %s\n", language.Code); writeErr != nil {
			err = fmt.Errorf("write create-language result: %w", writeErr)
		}
	case deleteLanguageCommand:
		if deleteErr := service.Delete(ctx, command.code); deleteErr != nil {
			err = describeError(deleteErr)
		} else if _, writeErr := fmt.Fprintf(stdout, "deleted language %s\n", command.code); writeErr != nil {
			err = fmt.Errorf("write delete-language result: %w", writeErr)
		}
	default:
		err = ErrCommandInvalid
	}

	return err
}

//
// Helpers
//

// Options may appear anywhere among the positional arguments until "--" ends
// option parsing.

func parseCreateLanguage(args []string) (input.CreateLanguageInput, error) {
	var (
		request    input.CreateLanguageInput
		positional []string
		err        error
	)

	parseOptions := true

	for _, argument := range args {
		switch {
		case parseOptions && argument == "--":
			parseOptions = false
		case parseOptions && (argument == "--fallback" || argument == "-f"):
			request.IsFallback = true
		case parseOptions && strings.HasPrefix(argument, "-"):
			err = ErrCommandInvalid
		default:
			positional = append(positional, argument)
		}

		if err != nil {
			break
		}
	}

	if err == nil && len(positional) != 3 {
		err = ErrCommandInvalid
	}

	if err == nil {
		request.Code = positional[0]
		request.EnglishName = positional[1]
		request.NativeName = positional[2]
	} else {
		request = input.CreateLanguageInput{}
	}

	return request, err
}

func parseDeleteLanguage(args []string) (string, error) {
	var (
		code string
		err  error
	)

	if len(args) != 1 {
		err = ErrCommandInvalid
	} else {
		code = args[0]
	}

	return code, err
}
