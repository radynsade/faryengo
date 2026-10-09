package utils

import (
	"errors"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Authentication
//

var ErrUnauthenticated = errors.New("not authenticated")

// A missing, invalid, revoked, or expired Session, wrong credentials, and a
// deleted User mean the visitor is not signed in. Any other failure is
// operational.

func IsUnauthenticated(err error) bool {
	return errors.Is(err, ErrUnauthenticated) ||
		errors.Is(err, security.ErrCredentialsInvalid) ||
		errors.Is(err, security.ErrSessionInvalid) ||
		errors.Is(err, security.ErrSessionRevoked) ||
		errors.Is(err, users.ErrUserNotFound)
}
