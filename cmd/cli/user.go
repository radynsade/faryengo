package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/app"
	appinput "github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/users/argon2id"
	"github.com/radynsade/faryengo/internal/users/pgxgoqu"
)

type userCommand struct {
	action      string
	createInput appinput.CreateUserInput
	id          users.UserID
}

func parseUserCommand(args []string) (userCommand, error) {
	var command userCommand
	var err error

	if len(args) < 2 || args[0] != "users" {
		err = errInvalidCommand
	} else {
		command.action = args[1]

		switch command.action {
		case "create-user":
			command.createInput, err = parseCreateUser(args)
		case "delete-user":
			command.id, err = parseDeleteUser(args)
		default:
			err = errInvalidCommand
		}
	}

	if err != nil {
		command = userCommand{}
	}

	return command, err
}

func parseCreateUser(args []string) (appinput.CreateUserInput, error) {
	var request appinput.CreateUserInput
	var err error

	if len(args) < 2 || args[0] != "users" || args[1] != "create-user" {
		err = errInvalidCommand
	} else {
		positional := args[2:]

		if len(positional) > 0 && positional[0] == "--" {
			positional = positional[1:]
		}

		if len(positional) != 6 {
			err = errInvalidCommand
		} else {
			roleID, roleErr := parseCommandUUID(positional[5], users.ErrRoleIDInvalid)

			if roleErr != nil {
				err = fmt.Errorf("validate create-user role ID: %w: %w", appinput.ErrInvalidCreateUserInput, roleErr)
			} else {
				request = appinput.CreateUserInput{
					Email: positional[0], FirstName: positional[1], LastName: positional[2],
					Password: positional[3], Phone: positional[4], RoleID: users.RoleID(roleID),
				}

				if validationErr := userArgumentValues(request); validationErr != nil {
					err = fmt.Errorf("validate create-user arguments: %w", validationErr)
				}
			}
		}
	}

	if err != nil {
		request = appinput.CreateUserInput{}
	}

	return request, err
}

func parseDeleteUser(args []string) (users.UserID, error) {
	var id users.UserID
	var err error

	if len(args) != 3 || args[0] != "users" || args[1] != "delete-user" {
		err = errInvalidCommand
	} else {
		parsed, parseErr := parseCommandUUID(args[2], appinput.ErrInvalidUserID)

		if parseErr != nil {
			err = fmt.Errorf("validate delete-user ID: %w", parseErr)
		} else {
			id = users.UserID(parsed)
		}
	}

	return id, err
}

func parseCommandUUID(value string, invalid error) (uuid.UUID, error) {
	var id uuid.UUID
	var err error

	if len(value) != 36 {
		err = invalid
	} else if errors.Is(invalid, users.ErrRoleIDInvalid) {
		parsed, parseErr := users.NewRoleID(value)
		id, err = uuid.UUID(parsed), parseErr
	} else {
		parsed, parseErr := users.NewUserID(value)
		id, err = uuid.UUID(parsed), parseErr
	}

	return id, err
}

func runUserCommand(ctx context.Context, command userCommand, pool *pgxpool.Pool, stdout io.Writer) error {
	repository, err := pgxgoqu.NewUserRepository(pool)

	if err != nil {
		err = fmt.Errorf("configure user repository: %w", err)
	} else {
		service, serviceErr := app.NewUserService(repository, argon2id.NewHasher())

		if serviceErr != nil {
			err = fmt.Errorf("configure user service: %w", serviceErr)
		} else {
			err = executeUserCommand(ctx, command, service, stdout)
		}
	}

	return err
}

func executeUserCommand(ctx context.Context, command userCommand, service *app.UserService, stdout io.Writer) error {
	var err error

	switch command.action {
	case "create-user":
		user, createErr := service.Create(ctx, command.createInput)

		if createErr != nil {
			err = fmt.Errorf("run create-user command: %w", createErr)
		} else if _, writeErr := fmt.Fprintf(stdout, "created user %s\n", uuid.UUID(user.ID())); writeErr != nil {
			err = fmt.Errorf("write create-user result: %w", writeErr)
		}
	case "delete-user":
		if deleteErr := service.Delete(ctx, command.id); deleteErr != nil {
			err = fmt.Errorf("run delete-user command: %w", deleteErr)
		} else if _, writeErr := fmt.Fprintf(stdout, "deleted user %s\n", uuid.UUID(command.id)); writeErr != nil {
			err = fmt.Errorf("write delete-user result: %w", writeErr)
		}
	default:
		err = errInvalidCommand
	}

	return err
}

// Validate primitive CLI arguments with domain constructors before connecting.
func userArgumentValues(request appinput.CreateUserInput) error {
	_, emailErr := users.NewEmail(request.Email)
	_, phoneErr := users.NewPhone(request.Phone)
	_, firstErr := users.NewFirstName(request.FirstName)
	_, lastErr := users.NewLastName(request.LastName)
	_, passwordErr := users.NewRegistrationPassword(request.Password)
	err := errors.Join(emailErr, phoneErr, firstErr, lastErr, passwordErr)

	if err != nil {
		err = fmt.Errorf("%w: %w", appinput.ErrInvalidCreateUserInput, err)
	}

	return err
}
