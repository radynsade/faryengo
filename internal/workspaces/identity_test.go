package workspaces

import (
	"context"
	"errors"
	"testing"

	"github.com/radynsade/faryengo/internal/users"
)

func TestIdentityCan(t *testing.T) {
	tests := []struct {
		name     string
		identity *Identity
		action   Action
		want     bool
	}{
		{"nil identity", nil, ActionViewContent, false},
		{"allowed", NewIdentity(testWorkspaceID, &users.User{ID: testUserID}, RoleEditor), ActionEditContent, true},
		{"not allowed", NewIdentity(testWorkspaceID, &users.User{ID: testUserID}, RoleViewer), ActionEditContent, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.identity.Can(tt.action); got != tt.want {
				t.Fatalf("Can(%q) = %t, want %t", tt.action, got, tt.want)
			}
		})
	}
}

func TestIdentityFromContext(t *testing.T) {
	identity := NewIdentity(testWorkspaceID, &users.User{ID: testUserID}, RoleViewer)

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
