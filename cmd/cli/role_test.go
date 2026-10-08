package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	appinput "github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/users"
)

func TestParseCreateRole(t *testing.T) {
	for _, tt := range []struct {
		name        string
		args        []string
		wantSuper   bool
		permissions []users.Permission
		want        error
	}{
		{name: "translated name", args: []string{"users", "create-role", "en:Administrator|lv:Administrators"}},
		{name: "super", args: []string{"users", "create-role", "--super", "en:Administrator|lv:Administrators"}, wantSuper: true},
		{name: "super shorthand", args: []string{"users", "create-role", "en:Administrator|lv:Administrators", "-s"}, wantSuper: true},
		{name: "permission flags", args: []string{"users", "create-role", "-p", "view_user", "en:Administrator|lv:Administrators", "--permission", "manage_user"}, permissions: []users.Permission{users.PermissionViewUser, users.PermissionManageUser}},
		{name: "permission equals", args: []string{"users", "create-role", "en:Administrator|lv:Administrators", "--permission=view_user"}, permissions: []users.Permission{users.PermissionViewUser}},
		{name: "role permission flags", args: []string{"users", "create-role", "-p", "view_role", "en:Administrator|lv:Administrators", "--permission", "manage_role"}, permissions: []users.Permission{users.PermissionViewRole, users.PermissionManageRole}},
		{name: "role permission equals", args: []string{"users", "create-role", "en:Administrator|lv:Administrators", "--permission=manage_role", "--permission=view_role"}, permissions: []users.Permission{users.PermissionManageRole, users.PermissionViewRole}},
		{name: "separator", args: []string{"users", "create-role", "--", "en:Administrator|lv:Administrators"}},
		{name: "malformed translations", args: []string{"users", "create-role", "en:Name|broken"}, want: appinput.ErrInvalidStringTranslations},
		{name: "invalid code", args: []string{"users", "create-role", "EN:Name"}, want: languages.ErrCodeInvalidChars},
		{name: "blank name", args: []string{"users", "create-role", "en: "}, want: languages.ErrTranslationInvalidChars},
		{name: "invalid permission", args: []string{"users", "create-role", "en:Name", "-p", "unknown"}, want: users.ErrPermissionInvalid},
		{name: "empty permission", args: []string{"users", "create-role", "en:Name", "--permission="}, want: users.ErrPermissionInvalid},
		{name: "missing name", args: []string{"users", "create-role", "--super"}, want: errInvalidCommand},
		{name: "extra name", args: []string{"users", "create-role", "en:Name", "lv:Loma"}, want: errInvalidCommand},
		{name: "unknown option", args: []string{"users", "create-role", "en:Name", "--unknown"}, want: errInvalidCommand},
		{name: "missing permission value", args: []string{"users", "create-role", "en:Name", "-p"}, want: errInvalidCommand},
		{name: "flag in place of permission", args: []string{"users", "create-role", "en:Name", "-p", "--super"}, want: errInvalidCommand},
		{name: "old roles group rejected", args: []string{"roles", "create-role", "en:Name"}, want: errInvalidCommand},
		{name: "wrong action", args: []string{"users", "create", "en:Name"}, want: errInvalidCommand},
		{name: "empty args", want: errInvalidCommand},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request, err := parseCreateRole(tt.args)

			if !errors.Is(err, tt.want) {
				t.Fatalf("parseCreateRole() = %v, want %v", err, tt.want)
			}

			if err != nil {
				if !reflect.DeepEqual(request, appinput.CreateRoleInput{}) {
					t.Fatal("failed parse retained input")
				}
			} else if request.Name["en"] != "Administrator" || request.Name["lv"] != "Administrators" || request.IsSuper != tt.wantSuper || !slices.Equal(request.Permissions, tt.permissions) {
				t.Fatalf("incorrect parsed input: %+v", request)
			}
		})
	}
}

func TestParseDeleteRole(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want error
	}{
		{name: "valid", args: []string{"users", "delete-role", testRoleUUID}},
		{name: "uppercase", args: []string{"users", "delete-role", strings.ToUpper(testRoleUUID)}},
		{name: "invalid", args: []string{"users", "delete-role", "invalid"}, want: users.ErrRoleIDInvalid},
		{name: "nil", args: []string{"users", "delete-role", uuid.Nil.String()}, want: users.ErrRoleIDInvalid},
		{name: "compact", args: []string{"users", "delete-role", strings.ReplaceAll(testRoleUUID, "-", "")}, want: users.ErrRoleIDInvalid},
		{name: "missing ID", args: []string{"users", "delete-role"}, want: errInvalidCommand},
		{name: "extra argument", args: []string{"users", "delete-role", testRoleUUID, "extra"}, want: errInvalidCommand},
		{name: "wrong action", args: []string{"users", "delete", testRoleUUID}, want: errInvalidCommand},
		{name: "old roles group rejected", args: []string{"roles", "delete-role", testRoleUUID}, want: errInvalidCommand},
		{name: "empty args", want: errInvalidCommand},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, err := parseDeleteRole(tt.args)

			if !errors.Is(err, tt.want) || (err == nil && uuid.UUID(id).String() != testRoleUUID) || (err != nil && uuid.UUID(id) != uuid.Nil) {
				t.Fatalf("parseDeleteRole() = %s, %v", uuid.UUID(id), err)
			}
		})
	}
}

type fakeCLIRoleRepository struct {
	ctx                  context.Context
	created              *users.Role
	deleted              users.RoleID
	createErr, deleteErr error
	calls                int
}

func (r *fakeCLIRoleRepository) Create(ctx context.Context, role *users.Role) error {
	r.ctx, r.created = ctx, role
	r.calls++
	return r.createErr
}

func (r *fakeCLIRoleRepository) Update(context.Context, *users.Role) error { return nil }

func (r *fakeCLIRoleRepository) FindByID(context.Context, users.RoleID) (*users.Role, error) {
	return nil, users.ErrRoleNotFound
}

func (r *fakeCLIRoleRepository) Delete(ctx context.Context, id users.RoleID) error {
	r.ctx, r.deleted = ctx, id
	r.calls++
	return r.deleteErr
}

func TestExecuteRoleCommand(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name, action string
		storeErr     error
	}{
		{name: "create", action: "create-role"},
		{name: "missing language", action: "create-role", storeErr: languages.ErrLanguageNotFound},
		{name: "delete", action: "delete-role"},
		{name: "assigned role", action: "delete-role", storeErr: users.ErrRoleAlreadyInUse},
		{name: "missing role", action: "delete-role", storeErr: users.ErrRoleNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"users", tt.action, testRoleUUID}

			if tt.action == "create-role" {
				args = []string{"users", tt.action, "en:Administrator|lv:Administrators", "--super"}
			}

			command, err := parseCommand(args)

			if err != nil || command.group != "users" {
				t.Fatalf("parseCommand() = %v, %v", command, err)
			}

			repository := &fakeCLIRoleRepository{createErr: tt.storeErr, deleteErr: tt.storeErr}
			service, err := app.NewRoleService(repository)

			if err != nil {
				t.Fatal(err)
			}

			var output bytes.Buffer
			err = executeRoleCommand(ctx, command.role, service, &output)

			if !errors.Is(err, tt.storeErr) || repository.calls != 1 || repository.ctx != ctx {
				t.Fatalf("executeRoleCommand() = %v; repository = %+v", err, repository)
			}

			if err != nil {
				if output.Len() != 0 {
					t.Fatal("failed command printed success")
				}
			} else if tt.action == "create-role" {
				if repository.created == nil || !repository.created.IsSuper() || repository.created.Name()["lv"].Content() != "Administrators" || output.String() != fmt.Sprintf("created role %s\n", uuid.UUID(repository.created.ID())) {
					t.Fatalf("incorrect create output: %q", output.String())
				}
			} else if uuid.UUID(repository.deleted).String() != testRoleUUID || output.String() != "deleted role "+testRoleUUID+"\n" {
				t.Fatalf("incorrect delete output: %q", output.String())
			}
		})
	}
}

func (f *fakeCLIRoleRepository) Find(context.Context, users.RoleQuery) ([]*users.Role, error) {
	return nil, nil
}

func (f *fakeCLIRoleRepository) Count(context.Context, users.RoleFilters) (int, error) {
	return 0, nil
}
