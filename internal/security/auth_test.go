package security_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radynsade/faryengo/internal/security"
)

func TestAuthenticationSnapshotValidate(t *testing.T) {
	for _, tt := range []struct {
		name     string
		snapshot *security.AuthenticationSnapshot
		want     error
	}{
		{name: "valid", snapshot: security.NewAuthenticationSnapshot(domainUser(), uuid.UUID{3})},
		{name: "nil", want: security.ErrInvalidAuthenticationSnapshot},
		{name: "missing user", snapshot: security.NewAuthenticationSnapshot(nil, uuid.UUID{3}), want: security.ErrInvalidAuthenticationSnapshot},
		{name: "missing version", snapshot: security.NewAuthenticationSnapshot(domainUser(), uuid.Nil), want: security.ErrInvalidAuthenticationSnapshot},
		{name: "invalid user", snapshot: security.NewAuthenticationSnapshot(&security.User{}, uuid.UUID{3}), want: security.ErrInvalidUser},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.snapshot.Validate()

			if !errors.Is(err, tt.want) || (tt.want != nil && !errors.Is(err, security.ErrInvalidAuthenticationSnapshot)) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSessionValidate(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*security.Session)
		want   error
	}{
		{name: "valid", change: func(*security.Session) {}},
		{name: "expired state remains intrinsically valid", change: func(s *security.Session) { s.ExpiresAt = time.Unix(1, 0) }},
		{name: "missing ID", change: func(s *security.Session) { s.ID = uuid.Nil }, want: security.ErrInvalidSession},
		{name: "missing user", change: func(s *security.Session) { s.UserID = security.UserID{} }, want: security.ErrInvalidSession},
		{name: "missing version", change: func(s *security.Session) { s.AuthenticationSnapshotVersion = uuid.Nil }, want: security.ErrInvalidSession},
		{name: "missing expiration", change: func(s *security.Session) { s.ExpiresAt = time.Time{} }, want: security.ErrInvalidSession},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session := security.NewSession(uuid.UUID{1}, security.UserID{2}, uuid.UUID{3}, time.Unix(100, 0))
			tt.change(session)
			before := *session

			if err := session.Validate(); !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if *session != before {
				t.Fatal("Validate changed session state")
			}
		})
	}
}

func TestPrincipalPermissions(t *testing.T) {
	for _, tt := range []struct {
		name        string
		super       bool
		permissions []security.Permission
		permission  security.Permission
		allowed     bool
	}{
		{name: "granted", permissions: []security.Permission{security.PermissionViewUser}, permission: security.PermissionViewUser, allowed: true},
		{name: "denied", permission: security.PermissionViewUser},
		{name: "manage does not grant view", permissions: []security.Permission{security.PermissionManageUser}, permission: security.PermissionViewUser},
		{name: "super", super: true, permission: security.PermissionManageRole, allowed: true},
		{name: "super rejects unknown", super: true, permission: "unknown"},
		{name: "super rejects empty", super: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			user := domainUser()
			principal := security.NewPrincipal(user.ID, user.RoleID, tt.permissions, tt.super,
				user.FirstName, user.LastName, user.Email, user.Phone)

			if err := principal.Validate(); err != nil {
				t.Fatal(err)
			}

			if principal.HasPermission(tt.permission) != tt.allowed {
				t.Fatal("unexpected authorization decision")
			}
		})
	}
}

func TestPrincipalValidateAndPermissionOwnership(t *testing.T) {
	user := domainUser()
	permissions := []security.Permission{security.PermissionViewUser}
	principal := security.NewPrincipal(user.ID, user.RoleID, permissions, false, user.FirstName, user.LastName, user.Email, user.Phone)
	permissions[0] = security.PermissionManageUser

	if !principal.HasPermission(security.PermissionViewUser) || principal.HasPermission(security.PermissionManageUser) {
		t.Fatal("principal retained externally mutable permission slice")
	}

	principal.Permissions = []security.Permission{"unknown"}

	if err := principal.Validate(); !errors.Is(err, security.ErrInvalidPrincipal) || !errors.Is(err, security.ErrInvalidPermission) {
		t.Fatalf("Validate() = %v, want invalid principal and permission", err)
	}

	var missing *security.Principal

	if !errors.Is(missing.Validate(), security.ErrInvalidPrincipal) {
		t.Fatal("nil principal accepted")
	}
}
