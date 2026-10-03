package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/app"
	appinput "github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/pgxgoqu"
)

type roleCommand struct {
	action      string
	createInput appinput.CreateRoleInput
	id          security.RoleID
}

func parseRoleCommand(args []string) (roleCommand, error) {
	var command roleCommand
	var err error

	if len(args) < 2 || args[0] != "security" {
		err = errInvalidCommand
	} else {
		command.action = args[1]

		switch command.action {
		case "create-role":
			command.createInput, err = parseCreateRole(args)
		case "delete-role":
			command.id, err = parseDeleteRole(args)
		default:
			err = errInvalidCommand
		}
	}

	if err != nil {
		command = roleCommand{}
	}

	return command, err
}

func parseCreateRole(args []string) (appinput.CreateRoleInput, error) {
	var request appinput.CreateRoleInput
	var err error

	if len(args) < 2 || args[0] != "security" || args[1] != "create-role" {
		err = errInvalidCommand
	} else {
		var positional []string
		parseOptions := true

		for index := 2; index < len(args); index++ {
			argument := args[index]

			switch {
			case parseOptions && argument == "--":
				parseOptions = false
			case parseOptions && (argument == "--super" || argument == "-s"):
				request.IsSuper = true
			case parseOptions && (argument == "--permission" || argument == "-p"):
				if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
					err = errInvalidCommand
				} else {
					index++
					request.Permissions = append(request.Permissions, security.Permission(args[index]))
				}
			case parseOptions && strings.HasPrefix(argument, "--permission="):
				request.Permissions = append(request.Permissions, security.Permission(strings.TrimPrefix(argument, "--permission=")))
			case parseOptions && strings.HasPrefix(argument, "-"):
				err = errInvalidCommand
			default:
				positional = append(positional, argument)
			}

			if err != nil {
				break
			}
		}

		if err == nil {
			if len(positional) != 1 {
				err = errInvalidCommand
			} else {
				request.Name, err = appinput.ParseStringTranslations(positional[0])

				if err != nil {
					err = fmt.Errorf("parse create-role name: %w", err)
				} else if validationErr := request.Validate(); validationErr != nil {
					err = fmt.Errorf("validate create-role arguments: %w", validationErr)
				}
			}
		}
	}

	if err != nil {
		request = appinput.CreateRoleInput{}
	}

	return request, err
}

func parseDeleteRole(args []string) (security.RoleID, error) {
	var id security.RoleID
	var err error

	if len(args) != 3 || args[0] != "security" || args[1] != "delete-role" {
		err = errInvalidCommand
	} else {
		parsed, parseErr := parseCommandUUID(args[2], security.ErrInvalidRoleID)

		if parseErr != nil {
			err = fmt.Errorf("validate delete-role ID: %w", parseErr)
		} else {
			id = security.RoleID(parsed)
		}
	}

	return id, err
}

func runRoleCommand(ctx context.Context, command roleCommand, pool *pgxpool.Pool, stdout io.Writer) error {
	repository, err := pgxgoqu.NewRoleRepository(pool)

	if err != nil {
		err = fmt.Errorf("configure role repository: %w", err)
	} else {
		service, serviceErr := app.NewRoleService(repository)

		if serviceErr != nil {
			err = fmt.Errorf("configure role service: %w", serviceErr)
		} else {
			err = executeRoleCommand(ctx, command, service, stdout)
		}
	}

	return err
}

func executeRoleCommand(ctx context.Context, command roleCommand, service *app.RoleService, stdout io.Writer) error {
	var err error

	switch command.action {
	case "create-role":
		role, createErr := service.Create(ctx, command.createInput)

		if createErr != nil {
			err = fmt.Errorf("run create-role command: %w", createErr)
		} else if _, writeErr := fmt.Fprintf(stdout, "created role %s\n", uuid.UUID(role.ID())); writeErr != nil {
			err = fmt.Errorf("write create-role result: %w", writeErr)
		}
	case "delete-role":
		if deleteErr := service.Delete(ctx, command.id); deleteErr != nil {
			err = fmt.Errorf("run delete-role command: %w", deleteErr)
		} else if _, writeErr := fmt.Fprintf(stdout, "deleted role %s\n", uuid.UUID(command.id)); writeErr != nil {
			err = fmt.Errorf("write delete-role result: %w", writeErr)
		}
	default:
		err = errInvalidCommand
	}

	return err
}
