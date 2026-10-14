package security

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/users"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestSessionValidate(t *testing.T) {
	valid := Session{
		ID:                 uuid.Must(uuid.NewV7()),
		UserID:             users.UserID(uuid.Must(uuid.NewV7())),
		CredentialsVersion: uuid.Must(uuid.NewV7()),
		ExpiresAt:          time.Now().Add(time.Hour),
	}

	tests := []struct {
		name   string
		change func(session *Session) *Session
		want   error
	}{
		{"valid", func(session *Session) *Session { return session }, nil},
		{"nil", func(*Session) *Session { return nil }, ErrSessionNil},
		{"nil ID", func(session *Session) *Session { session.ID = uuid.Nil; return session }, ErrSessionNilID},
		{"nil user ID", func(session *Session) *Session {
			session.UserID = users.UserID{}
			return session
		}, users.ErrUserIDInvalid},
		{"nil credentials version", func(session *Session) *Session {
			session.CredentialsVersion = uuid.Nil
			return session
		}, ErrSessionNilCredentialsVersion},
		{"zero expires at", func(session *Session) *Session {
			session.ExpiresAt = time.Time{}
			return session
		}, ErrSessionZeroExpiresAt},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := valid
			err := test.change(&session).Validate()

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if test.want != nil && !errors.Is(err, ErrSessionInvalid) {
				t.Fatalf("got %v, want it wrapped in %v", err, ErrSessionInvalid)
			}
		})
	}
}

func TestIdentityCan(t *testing.T) {
	tests := []struct {
		name     string
		identity *Identity
		want     bool
	}{
		{"nil identity", nil, false},
		{"granted", &Identity{Role: &users.Role{Permissions: users.Permissions{users.PermissionViewRole}}}, true},
		{"not granted", &Identity{Role: &users.Role{}}, false},
		{"super role", &Identity{Role: &users.Role{IsSuper: true}}, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.identity.Can(users.PermissionViewRole); got != test.want {
				t.Fatalf("got %t, want %t", got, test.want)
			}
		})
	}
}

func TestNewSystemIdentity(t *testing.T) {
	identity := NewSystemIdentity()

	if identity.Session != nil || identity.User != nil {
		t.Fatalf("NewSystemIdentity() = %+v, want no Session or User", identity)
	}

	for _, permission := range users.AllPermissions() {
		if !identity.Can(permission) {
			t.Fatalf("NewSystemIdentity().Can(%s) = false, want true", permission)
		}
	}
}

func TestIdentityFromContext(t *testing.T) {
	identity := &Identity{Role: &users.Role{}}

	tests := []struct {
		name    string
		ctx     context.Context
		want    *Identity
		wantErr error
	}{
		{"stored", WithIdentity(context.Background(), identity), identity, nil},
		{"missing", context.Background(), nil, ErrIdentityMissing},
		{"nil", WithIdentity(context.Background(), nil), nil, ErrIdentityMissing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IdentityFromContext(tt.ctx)

			if got != tt.want || !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) {
				t.Fatalf("IdentityFromContext() = (%v, %v), want (%v, %v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
