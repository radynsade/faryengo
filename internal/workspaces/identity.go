package workspaces

import (
	"context"
	"errors"

	"github.com/radynsade/faryengo/internal/users"
)

//
// Identity
//

// A member's identity within one Workspace: the User and the Role their
// membership gives them there. It permits only what that Role allows, and
// only in that Workspace.

type Identity struct {
	WorkspaceID WorkspaceID
	User        *users.User
	Role        Role
}

func NewIdentity(workspaceID WorkspaceID, user *users.User, role Role) *Identity {
	return &Identity{
		WorkspaceID: workspaceID,
		User:        user,
		Role:        role,
	}
}

func (i *Identity) Can(action Action) bool {
	return i != nil && i.Role.Allows(action)
}

//
// Identity context
//

// The member's identity travels to application services in the context. A
// context without one, or with a nil one, reports ErrIdentityMissing.

var ErrIdentityMissing = errors.New("no workspace identity in the context")

type identityContextKey struct{}

func WithIdentity(ctx context.Context, identity *Identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, identity)
}

func IdentityFromContext(ctx context.Context) (*Identity, error) {
	var err error

	identity, _ := ctx.Value(identityContextKey{}).(*Identity)

	if identity == nil {
		err = ErrIdentityMissing
	}

	return identity, err
}
