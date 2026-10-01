package security

import "errors"

var ErrInvalidPermission = errors.New("invalid permission")

type Permission string

const (
	PermissionManageUser Permission = "manage_user"
	PermissionViewUser   Permission = "view_user"
)

func (p Permission) Validate() error {
	switch p {
	case PermissionManageUser, PermissionViewUser:
		return nil
	default:
		return ErrInvalidPermission
	}
}
