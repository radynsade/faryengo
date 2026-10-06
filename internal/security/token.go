package security

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
	ErrRefreshTokenReused = errors.New("refresh token reused")
)

type TokenUse string

const (
	AccessToken  TokenUse = "access"
	RefreshToken TokenUse = "refresh"
)

func (u TokenUse) Validate() error {
	var err error

	if u != AccessToken && u != RefreshToken {
		err = ErrInvalidToken
	}

	return err
}

// TokenSession adds refresh state to a revocable authenticated session.
type TokenSession struct {
	Session
	RefreshHash string
}

type TokenClaims struct {
	UserID    UserID
	SessionID uuid.UUID
	ExpiresAt time.Time
}

// TokenIssuance contains the public claims needed to reproduce an identical
// signed pair across instances. It contains neither tokens nor signing secrets.
type TokenIssuance struct {
	KeyID            string
	AccessID         uuid.UUID
	RefreshID        uuid.UUID
	IssuedAt         time.Time
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

func (i TokenIssuance) Validate() error {
	var err error

	if strings.TrimSpace(i.KeyID) == "" || i.AccessID == uuid.Nil || i.RefreshID == uuid.Nil || i.AccessID == i.RefreshID ||
		i.IssuedAt.IsZero() || !i.AccessExpiresAt.After(i.IssuedAt) || i.AccessExpiresAt.Sub(i.IssuedAt) > 15*time.Minute ||
		i.RefreshExpiresAt.Before(i.AccessExpiresAt) || i.RefreshExpiresAt.Sub(i.IssuedAt) > 90*24*time.Hour {
		err = ErrInvalidSession
	}

	return err
}

type TokenPair struct {
	Issuance         TokenIssuance
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}
