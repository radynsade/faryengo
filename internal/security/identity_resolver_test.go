package security

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/users/mock"
)

func TestNewIdentityResolverRejectsMissingDependencies(t *testing.T) {
	credentials := &mock.CredentialsRepository{}
	userRepo := &mock.UserRepository{}
	roleRepo := &mock.RoleRepository{}

	tests := []struct {
		name        string
		credentials users.CredentialsSnapshotRepository
		users       users.UserRepository
		roles       users.RoleRepository
	}{
		{"missing credentials", nil, userRepo, roleRepo},
		{"missing users", credentials, nil, roleRepo},
		{"missing roles", credentials, userRepo, nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver, err := NewIdentityResolver(test.credentials, test.users, test.roles)

			if resolver != nil || !errors.Is(err, ErrIdentityResolverInvalid) {
				t.Fatalf("got %v, %v, want %v", resolver, err, ErrIdentityResolverInvalid)
			}
		})
	}
}

func TestIdentityResolverResolve(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	role := users.NewRole(users.RoleID(uuid.Must(uuid.NewV7())), users.RoleName{"en": "Viewer"}, users.Permissions{users.PermissionViewRole}, false)
	user := &users.User{ID: users.UserID(uuid.Must(uuid.NewV7())), RoleID: role.ID}
	version := uuid.Must(uuid.NewV7())
	newSession := func() *Session {
		return &Session{
			ID:                 uuid.Must(uuid.NewV7()),
			UserID:             user.ID,
			CredentialsVersion: version,
			ExpiresAt:          time.Now().Add(time.Hour),
		}
	}

	tests := []struct {
		name       string
		session    func() *Session
		versionErr error
		version    uuid.UUID
		userErr    error
		roleErr    error
		missing    bool
		want       error
	}{
		{name: "valid session", session: newSession, version: version},
		{name: "invalid session", session: func() *Session { return nil }, version: version, want: ErrSessionInvalid},
		{name: "expired session", session: func() *Session {
			session := newSession()
			session.ExpiresAt = time.Now().Add(-time.Second)

			return session
		}, version: version, want: ErrSessionRevoked},
		{name: "replaced credentials version", session: newSession, version: uuid.Must(uuid.NewV7()), want: ErrSessionRevoked},
		{name: "deleted user", session: newSession, versionErr: users.ErrUserNotFound, want: ErrSessionRevoked},
		{name: "user deleted after the version check", session: newSession, version: version, userErr: users.ErrUserNotFound, want: ErrSessionRevoked},
		{name: "missing role", session: newSession, version: version, roleErr: users.ErrRoleNotFound, want: users.ErrRoleNotFound},
		{name: "storage failure", session: newSession, versionErr: storageErr, want: storageErr},
		{name: "missing resolver", session: newSession, missing: true, want: ErrIdentityResolverNil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				credentials users.CredentialsSnapshotRepository
				userRepo    users.UserRepository
				roleRepo    users.RoleRepository
			)

			if !test.missing {
				credentials = &mock.CredentialsRepository{
					FindVersionByUserIDFunc: func(context.Context, users.UserID) (uuid.UUID, error) {
						return test.version, test.versionErr
					},
				}
				userRepo = &mock.UserRepository{
					FindByIDFunc: func(context.Context, users.UserID) (*users.User, error) {
						return user, test.userErr
					},
				}
				roleRepo = &mock.RoleRepository{
					FindByIDFunc: func(context.Context, users.RoleID) (*users.Role, error) {
						return role, test.roleErr
					},
				}
			}

			var resolver *IdentityResolver

			if !test.missing {
				var err error

				resolver, err = NewIdentityResolver(credentials, userRepo, roleRepo)

				if err != nil {
					t.Fatalf("create: %v", err)
				}
			}

			session := test.session()
			identity, err := resolver.Resolve(context.Background(), session)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if test.want != nil && identity != nil {
				t.Fatal("a failed resolution returned an identity")
			}

			if test.want == nil && (identity.Session != session || identity.User != user || identity.Role != role) {
				t.Fatalf("identity = %+v", identity)
			}
		})
	}
}
