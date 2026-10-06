package security

import (
	"slices"

	"github.com/google/uuid"
)

type Principal struct {
	UserID      UserID
	FirstName   FirstName
	LastName    LastName
	Email       Email
	SessionID   uuid.UUID
	RoleID      RoleID
	Permissions []Permission
	IsSuper     bool
}

func (p Principal) HasPermission(permission Permission) bool {
	return permission.Validate() == nil && (p.IsSuper || slices.Contains(p.Permissions, permission))
}
