package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/users/mock"
)

const testUserUUID = "01920000-0000-7000-8000-000000000001"

func createUserArguments() []string {
	return []string{"users", "create-user", "anna@example.com", "Anna", "Bērziņa", "correct horse", "+37120000000", testRoleUUID}
}

func TestParseUserCommand(t *testing.T) {
	wrongRole := createUserArguments()
	wrongRole[7] = "not-a-uuid"

	for _, tt := range []struct {
		name    string
		args    []string
		want    input.CreateUserInput
		wantID  string
		wantErr error
	}{
		{
			name: "create",
			args: createUserArguments(),
			want: input.CreateUserInput{
				RoleID:    users.RoleID(uuid.MustParse(testRoleUUID)),
				Email:     "anna@example.com",
				Phone:     "+37120000000",
				Password:  "correct horse",
				FirstName: "Anna",
				LastName:  "Bērziņa",
			},
		},
		{name: "create with missing phone", args: createUserArguments()[:7], wantErr: ErrCommandInvalid},
		{name: "create with extra argument", args: append(createUserArguments(), "extra"), wantErr: ErrCommandInvalid},
		{name: "create with malformed role UUID", args: wrongRole, wantErr: ErrUUIDInvalid},
		{name: "delete", args: []string{"users", "delete-user", testUserUUID}, wantID: testUserUUID},
		{name: "delete without UUID", args: []string{"users", "delete-user"}, wantErr: ErrCommandInvalid},
		{name: "delete with unhyphenated UUID", args: []string{"users", "delete-user", strings.ReplaceAll(testUserUUID, "-", "")}, wantErr: ErrUUIDInvalid},
		{name: "wrong action", args: []string{"users", "remove-user", testUserUUID}, wantErr: ErrCommandInvalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command, err := parseUserCommand(tt.args)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) || command.createInput != tt.want {
				t.Fatalf("parseUserCommand(%v) = (%+v, %v), want (%+v, %v)", tt.args, command.createInput, err, tt.want, tt.wantErr)
			}

			if tt.wantID != "" && uuid.UUID(command.id).String() != tt.wantID {
				t.Fatalf("parseUserCommand(%v) ID = %v, want %s", tt.args, uuid.UUID(command.id), tt.wantID)
			}
		})
	}
}

func TestExecuteUserCommand(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		createErr  error
		deleteErr  error
		wantOutput string
		wantErrs   []error
		wantText   string
	}{
		{
			name:       "created",
			args:       createUserArguments(),
			wantOutput: "created user " + testUserUUID + "\n",
		},
		{
			name:      "missing role",
			args:      createUserArguments(),
			createErr: users.ErrRoleNotFound,
			wantErrs:  []error{users.ErrRoleNotFound},
			wantText:  "role not found",
		},
		{
			name:      "duplicate email",
			args:      createUserArguments(),
			createErr: users.ErrUserAlreadyExists,
			wantErrs:  []error{users.ErrUserAlreadyExists},
			wantText:  "user already exists",
		},
		{
			name:     "short password",
			args:     []string{"users", "create-user", "anna@example.com", "Anna", "Bērziņa", "short", "+37120000000", testRoleUUID},
			wantErrs: []error{input.ErrCreateUserInputInvalid, users.ErrPasswordTooShort},
			wantText: "invalid password",
		},
		{
			name:       "deleted",
			args:       []string{"users", "delete-user", testUserUUID},
			wantOutput: "deleted user " + testUserUUID + "\n",
		},
		{
			name:      "delete missing user",
			args:      []string{"users", "delete-user", testUserUUID},
			deleteErr: users.ErrUserNotFound,
			wantErrs:  []error{users.ErrUserNotFound},
			wantText:  "user not found",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stored *users.User

			repository := &mock.UserRepository{
				CreateFunc: func(_ context.Context, user *users.User) users.ErrUserCreateFailed {
					var err users.ErrUserCreateFailed

					stored = user

					if tt.createErr != nil {
						err = mock.NewErrUserCreateFailed(user, tt.createErr)
					}

					return err
				},
				FindByIDFunc: func(_ context.Context, _ users.UserID) (*users.User, error) {
					copied := *stored
					copied.ID = users.UserID(uuid.MustParse(testUserUUID))

					return &copied, nil
				},
				DeleteFunc: func(_ context.Context, id users.UserID) users.ErrUserDeleteFailed {
					var err users.ErrUserDeleteFailed

					if tt.deleteErr != nil {
						err = mock.NewErrUserDeleteFailed(id, tt.deleteErr)
					}

					return err
				},
			}

			hasher := &mock.PasswordHasher{
				HashFunc: func(_ context.Context, password string) (users.PasswordHash, error) {
					return users.PasswordHash("hash:" + password), nil
				},
			}

			command, err := parseUserCommand(tt.args)

			if err != nil {
				t.Fatal(err)
			}

			service, err := app.NewUserService(&appmock.Transactor{}, repository, hasher)

			if err != nil {
				t.Fatal(err)
			}

			var output strings.Builder

			err = executeUserCommand(t.Context(), command, service, &output)

			assertCommandResult(t, output.String(), err, tt.wantOutput, tt.wantErrs, tt.wantText)
		})
	}
}
