package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Role command
//

const (
	usersGroup        = "users"
	createRoleCommand = "create-role"
	deleteRoleCommand = "delete-role"
)

var ErrTranslationsInvalid = errors.New(`invalid translations: use "<languageCode>:<name>|<languageCode>:<name>"`)

type roleCommand struct {
	action      string
	createInput input.CreateRoleInput
	id          users.RoleID
}

func parseRoleCommand(args []string) (roleCommand, error) {
	var (
		command roleCommand
		err     error
	)

	if len(args) < 2 || args[0] != usersGroup {
		err = ErrCommandInvalid
	} else {
		command.action = args[1]

		switch command.action {
		case createRoleCommand:
			command.createInput, err = parseCreateRole(args[2:])
		case deleteRoleCommand:
			command.id, err = parseDeleteRole(args[2:])
		default:
			err = ErrCommandInvalid
		}
	}

	if err != nil {
		command = roleCommand{}
	}

	return command, err
}

// Executing a command

func (c roleCommand) execute(
	ctx context.Context,
	services services,
	stdout io.Writer,
) error {
	return executeRoleCommand(ctx, c, services.roles, stdout)
}

func executeRoleCommand(
	ctx context.Context,
	command roleCommand,
	service *app.RoleService,
	stdout io.Writer,
) error {
	var err error

	switch command.action {
	case createRoleCommand:
		role, createErr := service.Create(ctx, command.createInput)

		if createErr != nil {
			err = describeError(createErr)
		} else if _, writeErr := fmt.Fprintf(stdout, "created role %s\n", uuid.UUID(role.ID)); writeErr != nil {
			err = fmt.Errorf("write create-role result: %w", writeErr)
		}
	case deleteRoleCommand:
		if deleteErr := service.Delete(ctx, command.id); deleteErr != nil {
			err = describeError(deleteErr)
		} else if _, writeErr := fmt.Fprintf(stdout, "deleted role %s\n", uuid.UUID(command.id)); writeErr != nil {
			err = fmt.Errorf("write delete-role result: %w", writeErr)
		}
	default:
		err = ErrCommandInvalid
	}

	return err
}

//
// Helpers
//

// Options may appear anywhere around the name until "--" ends option parsing.

func parseCreateRole(args []string) (input.CreateRoleInput, error) {
	var (
		request    input.CreateRoleInput
		positional []string
		err        error
	)

	parseOptions := true

	for index := 0; index < len(args) && err == nil; index++ {
		argument := args[index]

		switch {
		case parseOptions && argument == "--":
			parseOptions = false
		case parseOptions && (argument == "--super" || argument == "-s"):
			request.IsSuper = true
		case parseOptions && (argument == "--permission" || argument == "-p"):
			if index+1 >= len(args) {
				err = ErrCommandInvalid
			} else {
				index++
				request.Permissions = append(request.Permissions, args[index])
			}
		case parseOptions && strings.HasPrefix(argument, "-"):
			err = ErrCommandInvalid
		default:
			positional = append(positional, argument)
		}
	}

	if err == nil && len(positional) != 1 {
		err = ErrCommandInvalid
	}

	if err == nil {
		request.Name, err = parseTranslations(positional[0])
	}

	if err != nil {
		request = input.CreateRoleInput{}
	}

	return request, err
}

func parseDeleteRole(args []string) (users.RoleID, error) {
	var (
		id  uuid.UUID
		err error
	)

	if len(args) != 1 {
		err = ErrCommandInvalid
	} else {
		id, err = parseUUID(args[0])
	}

	return users.RoleID(id), err
}

// Each entry holds exactly one colon, so names cannot contain ":" or "|".
// Codes and names are trimmed and left to the service to validate; a repeated
// code is rejected rather than silently replacing the earlier name.

func parseTranslations(encoded string) (map[string]string, error) {
	var err error

	entries := strings.Split(encoded, "|")
	translations := make(map[string]string, len(entries))

	for _, entry := range entries {
		code, name, found := strings.Cut(entry, ":")
		code = strings.TrimSpace(code)

		if _, repeated := translations[code]; !found || strings.Contains(name, ":") || repeated {
			err = ErrTranslationsInvalid
		} else {
			translations[code] = strings.TrimSpace(name)
		}

		if err != nil {
			break
		}
	}

	if err != nil {
		translations = nil
	}

	return translations, err
}
