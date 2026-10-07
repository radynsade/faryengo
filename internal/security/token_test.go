package security_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/security"
)

func TestTokenSessionValidate(t *testing.T) {
	for _, tt := range []struct {
		name string
		hash string
		want error
	}{
		{name: "refresh state", hash: "digest"},
		{name: "missing refresh state", want: security.ErrInvalidSession},
		{name: "blank refresh state", hash: " \t", want: security.ErrInvalidSession},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session := security.NewTokenSession(*security.NewSession(uuid.UUID{1}, security.UserID{2}, uuid.UUID{3}, time.Unix(100, 0)), tt.hash)

			if err := session.Validate(); !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			session.ID = uuid.Nil

			if err := session.Validate(); !errors.Is(err, security.ErrInvalidSession) {
				t.Fatal("token session accepted invalid base state")
			}
		})
	}
}

func TestTokenIssuanceValidate(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*security.TokenIssuance)
		want   error
	}{
		{name: "valid", change: func(*security.TokenIssuance) {}},
		{name: "missing key", change: func(i *security.TokenIssuance) { i.KeyID = "" }, want: security.ErrInvalidSession},
		{name: "same token identity", change: func(i *security.TokenIssuance) { i.RefreshID = i.AccessID }, want: security.ErrInvalidSession},
		{name: "access at issuance", change: func(i *security.TokenIssuance) { i.AccessExpiresAt = i.IssuedAt }, want: security.ErrInvalidSession},
		{name: "access maximum", change: func(i *security.TokenIssuance) { i.AccessExpiresAt = i.IssuedAt.Add(15 * time.Minute) }},
		{name: "access too long", change: func(i *security.TokenIssuance) { i.AccessExpiresAt = i.IssuedAt.Add(15*time.Minute + time.Second) }, want: security.ErrInvalidSession},
		{name: "refresh before access", change: func(i *security.TokenIssuance) { i.RefreshExpiresAt = i.AccessExpiresAt.Add(-time.Second) }, want: security.ErrInvalidSession},
		{name: "refresh maximum", change: func(i *security.TokenIssuance) { i.RefreshExpiresAt = i.IssuedAt.Add(90 * 24 * time.Hour) }},
		{name: "refresh too long", change: func(i *security.TokenIssuance) { i.RefreshExpiresAt = i.IssuedAt.Add(90*24*time.Hour + time.Second) }, want: security.ErrInvalidSession},
	} {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Unix(100, 0)
			issuance := security.TokenIssuance{KeyID: "key", AccessID: uuid.UUID{1}, RefreshID: uuid.UUID{2}, IssuedAt: now, AccessExpiresAt: now.Add(5 * time.Minute), RefreshExpiresAt: now.Add(time.Hour)}
			tt.change(&issuance)
			before := issuance

			if err := issuance.Validate(); !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if issuance != before {
				t.Fatal("Validate changed issuance state")
			}
		})
	}
}
