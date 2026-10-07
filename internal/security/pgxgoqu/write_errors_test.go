package pgxgoqu

import (
	"errors"
	"testing"

	"github.com/radynsade/faryengo/internal/security"
)

func TestWriteErrorsRetainOperationData(t *testing.T) {
	role, user := testRole(t), testUser(t)
	var roles *RoleRepository
	var users *UserRepository

	for _, tt := range []struct {
		name  string
		run   func() error
		check func(error) bool
	}{
		{name: "create role", run: func() error { return roles.Create(t.Context(), role) }, check: func(err error) bool {
			e, ok := errors.AsType[security.ErrRoleCreateFailed](err)
			return ok && e.Role() == role
		}},
		{name: "update role", run: func() error { return roles.Update(t.Context(), role) }, check: func(err error) bool {
			e, ok := errors.AsType[security.ErrRoleUpdateFailed](err)
			return ok && e.Role() == role
		}},
		{name: "delete role", run: func() error { return roles.Delete(t.Context(), role.ID) }, check: func(err error) bool {
			e, ok := errors.AsType[security.ErrRoleDeleteFailed](err)
			return ok && e.RoleID() == role.ID
		}},
		{name: "create user", run: func() error { return users.Create(t.Context(), user) }, check: func(err error) bool {
			e, ok := errors.AsType[security.ErrUserCreateFailed](err)
			return ok && e.User() == user
		}},
		{name: "update user", run: func() error { return users.Update(t.Context(), user) }, check: func(err error) bool {
			e, ok := errors.AsType[security.ErrUserUpdateFailed](err)
			return ok && e.User() == user
		}},
		{name: "delete user", run: func() error { return users.Delete(t.Context(), user.ID) }, check: func(err error) bool {
			e, ok := errors.AsType[security.ErrUserDeleteFailed](err)
			return ok && e.UserID() == user.ID
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()

			if !errors.Is(err, ErrNilPool) || !tt.check(err) {
				t.Fatalf("operation error lost its data or cause: %v", err)
			}
		})
	}
}
