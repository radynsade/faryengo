package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Access
//

// Services learn who calls them from the context: a security.Identity for
// operations on the whole system, and a workspaces.Identity for operations
// inside one Workspace. A caller without the identity an operation needs is
// reported as ErrUnauthenticated, joined with the missing identity's error.

var ErrUnauthenticated = errors.New("unauthenticated")

// The identity is granted the operation when its Role grants any of the
// permissions. The caller adds the operation context.

func authorizeSystem(ctx context.Context, permissions ...users.Permission) (*security.Identity, error) {
	identity, err := currentIdentity(ctx)

	if err == nil {
		granted := false

		for _, permission := range permissions {
			granted = granted || identity.Can(permission)
		}

		if !granted {
			err = fmt.Errorf("%w: requires %v", users.ErrPermissionDenied, permissions)
			identity = nil
		}
	}

	return identity, err
}

// Any authenticated identity, the system identity included.

func currentIdentity(ctx context.Context) (*security.Identity, error) {
	identity, err := security.IdentityFromContext(ctx)

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrUnauthenticated, err)
	}

	return identity, err
}

// An identity that acts as an account; the system identity has none.

func currentUser(ctx context.Context) (*users.User, error) {
	var user *users.User

	identity, err := currentIdentity(ctx)

	if err == nil && identity.User == nil {
		err = fmt.Errorf("%w: the identity has no user", ErrUnauthenticated)
	} else if err == nil {
		user = identity.User
	}

	return user, err
}

// A member's identity for one Workspace permits nothing in another, so an
// identity for a different Workspace is reported with notFound, like one of a
// User who is not a member. The caller adds the operation context.

func authorizeMember(
	ctx context.Context,
	workspaceID workspaces.WorkspaceID,
	action workspaces.Action,
	notFound error,
) (*workspaces.Identity, error) {
	identity, err := currentMember(ctx)

	if err == nil && identity.WorkspaceID != workspaceID {
		err = notFound
	} else if err == nil && !identity.Can(action) {
		err = fmt.Errorf("%w: a workspace %s may not %s", workspaces.ErrRolePermissionDenied, identity.Role, action)
	}

	if err != nil {
		identity = nil
	}

	return identity, err
}

// A member's identity for any Workspace; one without a User acts for nobody.

func currentMember(ctx context.Context) (*workspaces.Identity, error) {
	identity, err := workspaces.IdentityFromContext(ctx)

	if err == nil && identity.User == nil {
		err = fmt.Errorf("%w: the workspace identity has no user", ErrUnauthenticated)
		identity = nil
	} else if err != nil {
		err = fmt.Errorf("%w: %w", ErrUnauthenticated, err)
	}

	return identity, err
}

// The identity's Role was read before the transaction began and may be out of
// date. The Workspace is locked first and the member's Role is read again
// under that lock, so a Role change and a write that relies on it apply one
// after another. A missing Workspace and a member who has left are reported
// with notFound. It must run in a transaction; the caller adds the operation
// context.

func reauthorizeMember(
	ctx context.Context,
	repository workspaces.WorkspaceRepository,
	identity *workspaces.Identity,
	action workspaces.Action,
	notFound error,
) (workspaces.Membership, error) {
	var membership workspaces.Membership

	_, err := repository.FindByIDForUpdate(ctx, identity.WorkspaceID)

	if err == nil {
		membership, err = repository.FindMember(ctx, identity.WorkspaceID, identity.User.ID)
	}

	if errors.Is(err, workspaces.ErrWorkspaceNotFound) || errors.Is(err, workspaces.ErrMembershipNotFound) {
		err = notFound
	} else if err == nil && !membership.Role.Allows(action) {
		err = fmt.Errorf("%w: a workspace %s may not %s", workspaces.ErrRolePermissionDenied, membership.Role, action)
	}

	if err != nil {
		membership = workspaces.Membership{}
	}

	return membership, err
}
