package main

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/users"
)

//
// User command
//

const (
	createUserCommand = "create-user"
	deleteUserCommand = "delete-user"
)

type userCommand struct {
	action      string
	createInput input.CreateUserInput
	id          users.UserID
}

func parseUserCommand(args []string) (userCommand, error) {
	var (
		command userCommand
		err     error
	)

	if len(args) < 2 || args[0] != usersGroup {
		err = ErrCommandInvalid
	} else {
		command.action = args[1]

		switch command.action {
		case createUserCommand:
			command.createInput, err = parseCreateUser(args[2:])
		case deleteUserCommand:
			command.id, err = parseDeleteUser(args[2:])
		default:
			err = ErrCommandInvalid
		}
	}

	if err != nil {
		command = userCommand{}
	}

	return command, err
}

// Executing a command

func (c userCommand) execute(
	ctx context.Context,
	services services,
	stdout io.Writer,
) error {
	return executeUserCommand(ctx, c, services.users, stdout)
}

func executeUserCommand(
	ctx context.Context,
	command userCommand,
	service *app.UserService,
	stdout io.Writer,
) error {
	var err error

	switch command.action {
	case createUserCommand:
		user, createErr := service.Create(ctx, command.createInput)

		if createErr != nil {
			err = describeError(createErr)
		} else if _, writeErr := fmt.Fprintf(stdout, "created user %s\n", uuid.UUID(user.ID)); writeErr != nil {
			err = fmt.Errorf("write create-user result: %w", writeErr)
		}
	case deleteUserCommand:
		if deleteErr := service.Delete(ctx, command.id); deleteErr != nil {
			err = describeError(deleteErr)
		} else if _, writeErr := fmt.Fprintf(stdout, "deleted user %s\n", uuid.UUID(command.id)); writeErr != nil {
			err = fmt.Errorf("write delete-user result: %w", writeErr)
		}
	default:
		err = ErrCommandInvalid
	}

	return err
}

//
// Helpers
//

func parseCreateUser(args []string) (input.CreateUserInput, error) {
	var (
		request input.CreateUserInput
		roleID  uuid.UUID
		err     error
	)

	if len(args) != 6 {
		err = ErrCommandInvalid
	} else {
		roleID, err = parseUUID(args[5])
	}

	if err == nil {
		request = input.CreateUserInput{
			RoleID:    users.RoleID(roleID),
			Email:     args[0],
			Phone:     args[4],
			Password:  args[3],
			FirstName: args[1],
			LastName:  args[2],
		}
	}

	return request, err
}

func parseDeleteUser(args []string) (users.UserID, error) {
	var (
		id  uuid.UUID
		err error
	)

	if len(args) != 1 {
		err = ErrCommandInvalid
	} else {
		id, err = parseUUID(args[0])
	}

	return users.UserID(id), err
}
