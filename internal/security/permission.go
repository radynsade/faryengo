package security

import "errors"

var ErrInvalidPermission = errors.New("invalid permission")

type Permission string

const (
	PermissionManageUser Permission = "manage_user"
	PermissionViewUser   Permission = "view_user"
	PermissionManageRole Permission = "manage_role"
	PermissionViewRole   Permission = "view_role"
)

func (p Permission) Validate() error {
	switch p {
	case PermissionManageUser, PermissionViewUser, PermissionManageRole, PermissionViewRole:
		return nil
	default:
		return ErrInvalidPermission
	}
}
