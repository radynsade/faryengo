package security

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrSessionRevoked     = errors.New("session revoked or expired")
	ErrRefreshTokenReused = errors.New("refresh token reused")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrInvalidSession     = errors.New("invalid session")
)

type TokenUse string

const (
	AccessToken  TokenUse = "access"
	RefreshToken TokenUse = "refresh"
)

// Credentials includes the durable version rotated atomically by PostgreSQL
// whenever password credentials change. It is never included in a JWT.
type Credentials struct {
	User    *User
	Version uuid.UUID
}

type Session struct {
	ID                uuid.UUID
	UserID            UserID
	CredentialVersion uuid.UUID
	RefreshHash       string
	ExpiresAt         time.Time
}

type TokenClaims struct {
	UserID    UserID
	SessionID uuid.UUID
	ExpiresAt time.Time
}

type TokenPair struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

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
