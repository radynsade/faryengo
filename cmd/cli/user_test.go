package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	appinput "github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/argon2id"
)

const (
	testRoleUUID = "8e35a76b-cc06-4b5b-8d8c-2c4d9144ff63"
	testUserUUID = "3fc159c0-10b6-4c34-8459-c8544330d366"
	testPassword = "secret-password"
)

func createUserArguments() []string {
	return []string{"security", "create-user", "person@example.com", "First Name", "Last Name", testPassword, "+37123456789", testRoleUUID}
}

func changedUserArguments(index int, value string) []string {
	args := createUserArguments()
	args[index] = value
	return args
}

func TestParseCreateUser(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		wantErr error
	}{
		{name: "valid", args: createUserArguments()},
		{name: "options separator", args: append([]string{"security", "create-user", "--"}, createUserArguments()[2:]...)},
		{name: "password starts with hyphen", args: changedUserArguments(5, "--secret-password")},
		{name: "missing arguments", args: []string{"security", "create-user"}, wantErr: errInvalidCommand},
		{name: "extra argument", args: append(createUserArguments(), "extra"), wantErr: errInvalidCommand},
		{name: "old users group rejected", args: changedUserArguments(0, "users"), wantErr: errInvalidCommand},
		{name: "wrong action", args: changedUserArguments(1, "create"), wantErr: errInvalidCommand},
		{name: "missing command", wantErr: errInvalidCommand},
		{name: "invalid email", args: changedUserArguments(2, "invalid-email"), wantErr: security.ErrEmailInvalid},
		{name: "missing first name", args: changedUserArguments(3, ""), wantErr: security.ErrFirstNameInvalid},
		{name: "missing last name", args: changedUserArguments(4, ""), wantErr: security.ErrLastNameInvalid},
		{name: "short password", args: changedUserArguments(5, "1234567"), wantErr: security.ErrInvalidPassword},
		{name: "blank password", args: changedUserArguments(5, "        "), wantErr: security.ErrInvalidPassword},
		{name: "oversized password", args: changedUserArguments(5, strings.Repeat("x", security.MaxPasswordBytes+1)), wantErr: security.ErrInvalidPassword},
		{name: "invalid phone", args: changedUserArguments(6, "12345678"), wantErr: security.ErrPhoneInvalid},
		{name: "invalid role UUID", args: changedUserArguments(7, "invalid-uuid"), wantErr: security.ErrRoleIDInvalid},
		{name: "nil role UUID", args: changedUserArguments(7, uuid.Nil.String()), wantErr: security.ErrRoleIDInvalid},
		{name: "compact UUID rejected", args: changedUserArguments(7, strings.ReplaceAll(testRoleUUID, "-", "")), wantErr: security.ErrRoleIDInvalid},
		{name: "URN UUID rejected", args: changedUserArguments(7, "urn:uuid:"+testRoleUUID), wantErr: security.ErrRoleIDInvalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request, err := parseCreateUser(tt.args)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseCreateUser error = %v, want %v", err, tt.wantErr)
			}

			if tt.wantErr != nil {
				if request != (appinput.CreateUserInput{}) {
					t.Fatal("failed parse returned user input")
				}

				if !errors.Is(tt.wantErr, errInvalidCommand) && !errors.Is(err, appinput.ErrInvalidCreateUserInput) {
					t.Fatal("validation error lost its input error category")
				}
			} else {
				positional := tt.args[2:]

				if positional[0] == "--" {
					positional = positional[1:]
				}

				if request.Email != positional[0] || request.FirstName != positional[1] || request.LastName != positional[2] ||
					request.Password != positional[3] || request.Phone != positional[4] || uuid.UUID(request.RoleID).String() != testRoleUUID {
					t.Fatal("create-user argument order or values changed")
				}
			}
		})
	}
}

func TestParseDeleteUser(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		wantErr error
	}{
		{name: "valid", args: []string{"security", "delete-user", testUserUUID}},
		{name: "uppercase UUID", args: []string{"security", "delete-user", strings.ToUpper(testUserUUID)}},
		{name: "missing ID", args: []string{"security", "delete-user"}, wantErr: errInvalidCommand},
		{name: "extra argument", args: []string{"security", "delete-user", testUserUUID, "extra"}, wantErr: errInvalidCommand},
		{name: "invalid ID", args: []string{"security", "delete-user", "invalid-uuid"}, wantErr: appinput.ErrInvalidUserID},
		{name: "nil UUID", args: []string{"security", "delete-user", uuid.Nil.String()}, wantErr: appinput.ErrInvalidUserID},
		{name: "compact UUID", args: []string{"security", "delete-user", strings.ReplaceAll(testUserUUID, "-", "")}, wantErr: appinput.ErrInvalidUserID},
		{name: "old users group rejected", args: []string{"users", "delete-user", testUserUUID}, wantErr: errInvalidCommand},
		{name: "wrong action", args: []string{"security", "delete", testUserUUID}, wantErr: errInvalidCommand},
		{name: "missing command", wantErr: errInvalidCommand},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, err := parseDeleteUser(tt.args)

			if !errors.Is(err, tt.wantErr) || (err != nil && uuid.UUID(id) != uuid.Nil) || (err == nil && uuid.UUID(id).String() != testUserUUID) {
				t.Fatalf("parseDeleteUser = %s, %v", uuid.UUID(id), err)
			}
		})
	}
}

func TestParseCommandGroups(t *testing.T) {
	for _, tt := range []struct {
		name, group string
		args        []string
		wantErr     error
	}{
		{name: "languages", group: "languages", args: []string{"languages", "delete-language", "lv"}},
		{name: "create user", group: "security", args: createUserArguments()},
		{name: "delete user", group: "security", args: []string{"security", "delete-user", testUserUUID}},
		{name: "create role", group: "security", args: []string{"security", "create-role", "en:Administrator", "--super"}},
		{name: "delete role", group: "security", args: []string{"security", "delete-role", testRoleUUID}},
		{name: "unknown group", args: []string{"unknown", "delete-user", testUserUUID}, wantErr: errInvalidCommand},
		{name: "unknown user action", args: []string{"security", "update-user"}, wantErr: errInvalidCommand},
		{name: "unknown role action", args: []string{"security", "update-role"}, wantErr: errInvalidCommand},
		{name: "malformed role name", args: []string{"security", "create-role", "broken"}, wantErr: appinput.ErrInvalidStringTranslations},
		{name: "missing action", args: []string{"security"}, wantErr: errInvalidCommand},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command, err := parseCommand(tt.args)

			if !errors.Is(err, tt.wantErr) || command.group != tt.group {
				t.Fatalf("parseCommand = %q, %v", command.group, err)
			}

			if err != nil && !reflect.DeepEqual(command, cliCommand{}) {
				t.Fatal("failed command retained arguments")
			}
		})
	}
}

type fakeCLIUserRepository struct {
	ctx         context.Context
	created     *security.User
	deleted     security.UserID
	createErr   error
	deleteErr   error
	createCalls int
	deleteCalls int
}

func (r *fakeCLIUserRepository) Create(ctx context.Context, user *security.User) error {
	r.ctx, r.created = ctx, user
	r.createCalls++
	return r.createErr
}

func (r *fakeCLIUserRepository) Delete(ctx context.Context, id security.UserID) error {
	r.ctx, r.deleted = ctx, id
	r.deleteCalls++
	return r.deleteErr
}

func (*fakeCLIUserRepository) Update(context.Context, *security.User) error {
	return security.ErrUserNotFound
}

func (*fakeCLIUserRepository) FindByID(context.Context, security.UserID) (*security.User, error) {
	return nil, security.ErrUserNotFound
}

func TestCreateUserCommand(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "created"},
		{name: "duplicate email", err: security.ErrUserAlreadyExists},
		{name: "missing role", err: security.ErrRoleNotFound},
		{name: "database failure", err: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command, err := parseUserCommand(createUserArguments())

			if err != nil {
				t.Fatal(err)
			}

			repository := &fakeCLIUserRepository{createErr: tt.err}
			hasher := argon2id.NewHasher()
			service, err := app.NewUserService(repository, hasher)

			if err != nil {
				t.Fatal(err)
			}

			ctx := t.Context()
			var stdout bytes.Buffer
			err = executeUserCommand(ctx, command, service, &stdout)

			if !errors.Is(err, tt.err) || repository.createCalls != 1 || repository.ctx != ctx || repository.created == nil {
				t.Fatalf("executeUserCommand error = %v, create calls = %d", err, repository.createCalls)
			}

			user := repository.created

			if uuid.UUID(user.ID()) == uuid.Nil || uuid.UUID(user.RoleID()).String() != testRoleUUID || user.Email() != "person@example.com" ||
				user.Phone() != "+37123456789" || user.FirstName() != "First Name" || user.LastName() != "Last Name" ||
				!strings.HasPrefix(string(user.PasswordHash()), "$argon2id$") {
				t.Fatal("created user does not match the input or uses an invalid password hash")
			}

			if tt.err == nil {
				if stdout.String() != fmt.Sprintf("created user %s\n", uuid.UUID(user.ID())) {
					t.Fatalf("success output = %q", stdout.String())
				}

				matches, verifyErr := hasher.Verify(ctx, testPassword, user.PasswordHash())

				if verifyErr != nil || !matches {
					t.Fatalf("created password cannot authenticate: %v", verifyErr)
				}
			} else if stdout.Len() != 0 {
				t.Fatal("failed creation printed success")
			}

			if strings.Contains(stdout.String(), testPassword) || strings.Contains(stdout.String(), string(user.PasswordHash())) {
				t.Fatal("creation printed credentials")
			}
		})
	}
}

func TestDeleteUserCommand(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "deleted"},
		{name: "not found", err: security.ErrUserNotFound},
		{name: "database failure", err: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command, err := parseUserCommand([]string{"security", "delete-user", testUserUUID})

			if err != nil {
				t.Fatal(err)
			}

			repository := &fakeCLIUserRepository{deleteErr: tt.err}
			service, err := app.NewUserService(repository, argon2id.NewHasher())

			if err != nil {
				t.Fatal(err)
			}

			ctx := t.Context()
			var stdout bytes.Buffer
			err = executeUserCommand(ctx, command, service, &stdout)
			wantOutput := ""

			if tt.err == nil {
				wantOutput = "deleted user " + testUserUUID + "\n"
			}

			if !errors.Is(err, tt.err) || stdout.String() != wantOutput || repository.deleteCalls != 1 || repository.ctx != ctx ||
				uuid.UUID(repository.deleted).String() != testUserUUID || repository.createCalls != 0 {
				t.Fatalf("executeUserCommand = %q, %v, delete calls = %d", stdout.String(), err, repository.deleteCalls)
			}
		})
	}
}
