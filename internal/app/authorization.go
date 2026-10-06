package app

import (
	"fmt"

	"github.com/radynsade/faryengo/internal/security"
)

// Authorize checks an already authenticated principal, independently of how the
// request supplied its credentials. Principals must come from authentication.
func Authorize(principal security.Principal, permission security.Permission) error {
	var err error

	if validationErr := permission.Validate(); validationErr != nil {
		err = fmt.Errorf("authorize permission: %w", validationErr)
	} else if !principal.HasPermission(permission) {
		err = security.ErrPermissionDenied
	}

	return err
}
