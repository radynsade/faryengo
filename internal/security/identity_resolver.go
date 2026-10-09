package security

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/users"
)

//
// Identity resolver
//

// The resolver derives an Identity from a Session however the Session was
// carried, so every authentication mechanism shares one rule. The Session is
// accepted only while it can still authenticate: it is valid and unexpired,
// and the User exists and still holds the credentials version the Session
// captured. The User's current Role is loaded on every resolution, so changes
// to authority apply to existing Sessions at once. A missing User is reported
// as ErrSessionRevoked.

var (
	ErrIdentityResolverNil     = errors.New("identity resolver is nil")
	ErrIdentityResolverInvalid = errors.New("invalid identity resolver configuration")
)

type IdentityResolver struct {
	credentials users.CredentialsSnapshotRepository
	users       users.UserRepository
	roles       users.RoleRepository
}

func NewIdentityResolver(
	credentials users.CredentialsSnapshotRepository,
	userRepository users.UserRepository,
	roleRepository users.RoleRepository,
) (*IdentityResolver, error) {
	var (
		resolver *IdentityResolver
		err      error
	)

	if credentials == nil || userRepository == nil || roleRepository == nil {
		err = fmt.Errorf("create an identity resolver: %w", ErrIdentityResolverInvalid)
	} else {
		resolver = &IdentityResolver{
			credentials: credentials,
			users:       userRepository,
			roles:       roleRepository,
		}
	}

	return resolver, err
}

// Resolve

func (r *IdentityResolver) Resolve(ctx context.Context, session *Session) (*Identity, error) {
	var (
		identity *Identity
		version  uuid.UUID
		user     *users.User
		role     *users.Role
		err      error
	)

	if r == nil || r.credentials == nil || r.users == nil || r.roles == nil {
		err = ErrIdentityResolverNil
	} else {
		err = session.Validate()
	}

	if err == nil && !time.Now().Before(session.ExpiresAt) {
		err = ErrSessionRevoked
	}

	if err == nil {
		version, err = r.credentials.FindVersionByUserID(ctx, session.UserID)

		if err == nil && version != session.CredentialsVersion {
			err = ErrSessionRevoked
		}
	}

	if err == nil {
		user, err = r.users.FindByID(ctx, session.UserID)
	}

	if err == nil {
		role, err = r.roles.FindByID(ctx, user.RoleID)
	}

	if errors.Is(err, users.ErrUserNotFound) {
		err = fmt.Errorf("%w: %w", ErrSessionRevoked, err)
	}

	if err == nil {
		identity = &Identity{Session: session, User: user, Role: role}
	} else {
		err = fmt.Errorf("resolve an identity: %w", err)
	}

	return identity, err
}
