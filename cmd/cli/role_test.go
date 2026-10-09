package main

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/users/mock"
)

const testRoleUUID = "01920000-0000-7000-8000-000000000002"

func TestParseRoleCommand(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		want    input.CreateRoleInput
		wantID  string
		wantErr error
	}{
		{
			name: "create",
			args: []string{"users", "create-role", "en:Editor|lv:Redaktors"},
			want: input.CreateRoleInput{Name: map[string]string{"en": "Editor", "lv": "Redaktors"}},
		},
		{
			name: "create with options around the name",
			args: []string{"users", "create-role", "-p", "view_user", "en: Editor ", "--super", "--permission", "manage_role"},
			want: input.CreateRoleInput{
				Name:        map[string]string{"en": "Editor"},
				Permissions: []string{"view_user", "manage_role"},
				IsSuper:     true,
			},
		},
		{
			name: "options separator",
			args: []string{"users", "create-role", "-s", "--", "en:Editor"},
			want: input.CreateRoleInput{Name: map[string]string{"en": "Editor"}, IsSuper: true},
		},
		{name: "permission without value", args: []string{"users", "create-role", "en:Editor", "-p"}, wantErr: ErrCommandInvalid},
		{name: "unknown option", args: []string{"users", "create-role", "en:Editor", "--all"}, wantErr: ErrCommandInvalid},
		{name: "missing name", args: []string{"users", "create-role", "-s"}, wantErr: ErrCommandInvalid},
		{name: "two names", args: []string{"users", "create-role", "en:Editor", "lv:Redaktors"}, wantErr: ErrCommandInvalid},
		{name: "entry without colon", args: []string{"users", "create-role", "Editor"}, wantErr: ErrTranslationsInvalid},
		{name: "name with colon", args: []string{"users", "create-role", "en:Editor:Chief"}, wantErr: ErrTranslationsInvalid},
		{name: "repeated language", args: []string{"users", "create-role", "en:Editor|en:Writer"}, wantErr: ErrTranslationsInvalid},
		{name: "delete", args: []string{"users", "delete-role", testRoleUUID}, wantID: testRoleUUID},
		{name: "delete without UUID", args: []string{"users", "delete-role"}, wantErr: ErrCommandInvalid},
		{name: "delete with braced UUID", args: []string{"users", "delete-role", "{" + testRoleUUID + "}"}, wantErr: ErrUUIDInvalid},
		{name: "delete with malformed UUID", args: []string{"users", "delete-role", strings.Repeat("x", 36)}, wantErr: ErrUUIDInvalid},
		{name: "wrong action", args: []string{"users", "remove-role", testRoleUUID}, wantErr: ErrCommandInvalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command, err := parseRoleCommand(tt.args)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) {
				t.Fatalf("parseRoleCommand(%v) error = %v, want %v", tt.args, err, tt.wantErr)
			}

			if !maps.Equal(command.createInput.Name, tt.want.Name) ||
				!slices.Equal(command.createInput.Permissions, tt.want.Permissions) ||
				command.createInput.IsSuper != tt.want.IsSuper {
				t.Fatalf("parseRoleCommand(%v) input = %+v, want %+v", tt.args, command.createInput, tt.want)
			}

			if tt.wantID != "" && uuid.UUID(command.id).String() != tt.wantID {
				t.Fatalf("parseRoleCommand(%v) ID = %v, want %s", tt.args, uuid.UUID(command.id), tt.wantID)
			}
		})
	}
}

func TestExecuteRoleCommand(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		writeErr   error
		wantOutput string
		wantErrs   []error
		wantText   string
	}{
		{
			name:       "deleted",
			args:       []string{"users", "delete-role", testRoleUUID},
			wantOutput: "deleted role " + testRoleUUID + "\n",
		},
		{
			name:     "delete assigned role",
			args:     []string{"users", "delete-role", testRoleUUID},
			writeErr: users.ErrRoleAlreadyInUse,
			wantErrs: []error{users.ErrRoleAlreadyInUse},
			wantText: "role is assigned to users",
		},
		{
			name:     "delete nil UUID",
			args:     []string{"users", "delete-role", uuid.Nil.String()},
			wantErrs: []error{users.ErrRoleIDInvalid},
			wantText: "invalid role ID",
		},
		{
			name:     "create with unknown permission",
			args:     []string{"users", "create-role", "en:Editor", "-p", "fly"},
			wantErrs: []error{input.ErrCreateRoleInputInvalid, users.ErrPermissionInvalid},
			wantText: "invalid permission",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repository := &mock.RoleRepository{
				DeleteFunc: func(_ context.Context, id users.RoleID) users.ErrRoleDeleteFailed {
					var err users.ErrRoleDeleteFailed

					if tt.writeErr != nil {
						err = mock.NewErrRoleDeleteFailed(id, tt.writeErr)
					}

					return err
				},
			}

			command, err := parseRoleCommand(tt.args)

			if err != nil {
				t.Fatal(err)
			}

			service, err := app.NewRoleService(&appmock.Transactor{}, repository)

			if err != nil {
				t.Fatal(err)
			}

			var output strings.Builder

			err = executeRoleCommand(t.Context(), command, service, &output)

			assertCommandResult(t, output.String(), err, tt.wantOutput, tt.wantErrs, tt.wantText)
		})
	}
}

func TestExecuteCreateRoleCommand(t *testing.T) {
	var created *users.Role

	repository := &mock.RoleRepository{
		CreateFunc: func(_ context.Context, role *users.Role) users.ErrRoleCreateFailed {
			created = role

			return nil
		},
	}

	command, err := parseRoleCommand([]string{"users", "create-role", "en:Administrator", "--super"})

	if err != nil {
		t.Fatal(err)
	}

	service, err := app.NewRoleService(&appmock.Transactor{}, repository)

	if err != nil {
		t.Fatal(err)
	}

	var output strings.Builder

	err = executeRoleCommand(t.Context(), command, service, &output)

	if err != nil || created == nil || !created.IsSuper ||
		output.String() != "created role "+uuid.UUID(created.ID).String()+"\n" {
		t.Fatalf("executeRoleCommand() = (%q, %v), created = %+v", output.String(), err, created)
	}
}
