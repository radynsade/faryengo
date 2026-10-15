package app

import (
	"context"
	"testing"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Identity contexts
//

// Services act for the identities their context carries; these build the
// contexts each test calls them with.

func systemContext(t *testing.T) context.Context {
	return security.WithIdentity(t.Context(), security.NewSystemIdentity())
}

// The User's Role grants exactly the given permissions.

func userContext(t *testing.T, userID users.UserID, permissions ...users.Permission) context.Context {
	role := users.NewRole(testRoleID, users.RoleName{"en": "Tester"}, users.NewPermissions(permissions), false)
	identity := &security.Identity{User: &users.User{ID: userID, RoleID: testRoleID}, Role: role}

	return security.WithIdentity(t.Context(), identity)
}

// The member acts in the Workspace with the given Role, which the transport
// read from their membership before calling the service.

func memberContext(
	t *testing.T,
	workspaceID workspaces.ID,
	userID users.UserID,
	role workspaces.Role,
) context.Context {
	ctx := userContext(t, userID)

	return workspaces.WithIdentity(ctx, workspaces.NewIdentity(workspaceID, &users.User{ID: userID}, role))
}
