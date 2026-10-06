package security

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionRevoked     = errors.New("session revoked or expired")
	ErrInvalidSession     = errors.New("invalid session")
)

// Credentials includes the durable version rotated atomically by PostgreSQL
// whenever password credentials change. It remains server-side.
type Credentials struct {
	User    *User
	Version uuid.UUID
}

// Session is server-side authentication state shared by all mechanisms.
type Session struct {
	ID                uuid.UUID
	UserID            UserID
	CredentialVersion uuid.UUID
	ExpiresAt         time.Time
}

// SessionGrant is an opaque credential and its server-controlled expiration.
// Only the ID is sent in the browser cookie; no identity or claims are encoded.
type SessionGrant struct {
	ID        string
	ExpiresAt time.Time
}
