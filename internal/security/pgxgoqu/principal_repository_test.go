package pgxgoqu

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/radynsade/faryengo/internal/security"
)

func TestPrincipalRepository(t *testing.T) {
	for _, tt := range []struct {
		name       string
		byEmail    bool
		super      bool
		permission string
		rowErr     error
		want       error
	}{
		{name: "by user", permission: "view_user"},
		{name: "by email", byEmail: true, permission: "manage_role"},
		{name: "super", super: true, permission: "view_user"},
		{name: "missing", rowErr: pgx.ErrNoRows, want: security.ErrUserNotFound},
		{name: "outage", rowErr: context.Canceled, want: context.Canceled},
		{name: "invalid stored permission", permission: "unknown", want: security.ErrPermissionInvalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeSecurityDB{row: fakeSecurityRow{err: tt.rowErr, values: []any{
				pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, pgtype.UUID{Bytes: [16]byte{2}, Valid: true},
				[]string{tt.permission}, tt.super, "Alice", "Example", "alice@example.com", "+37123456789",
			}}}
			repository := &PrincipalRepository{pool: db}
			var principal *security.Principal
			var err error

			if tt.byEmail {
				principal, err = repository.FindByEmail(t.Context(), "Alice@example.com")
			} else {
				principal, err = repository.FindByUserID(t.Context(), security.UserID{1})
			}

			if !errors.Is(err, tt.want) {
				t.Fatalf("lookup error = %v, want %v", err, tt.want)
			}

			if tt.want != nil {
				if principal != nil {
					t.Fatal("failed lookup returned principal")
				}
			} else if principal == nil || principal.UserID != (security.UserID{1}) || principal.IsSuper != tt.super || !principal.HasPermission(security.Permission(tt.permission)) {
				t.Fatalf("unexpected principal: %+v", principal)
			}

			if db.queryContext != t.Context() || strings.Contains(db.query, "Alice@example.com") || strings.Contains(db.query, "password_hash") || !strings.Contains(db.query, "INNER JOIN") {
				t.Fatalf("unsafe or incomplete query: %s", db.query)
			}
		})
	}
}

func TestPrincipalRepositoryInvalidInput(t *testing.T) {
	db := &fakeSecurityDB{}
	repository := &PrincipalRepository{pool: db}

	for _, tt := range []struct {
		name string
		run  func() error
		want error
	}{
		{name: "email", run: func() error { _, err := repository.FindByEmail(t.Context(), "bad"); return err }, want: security.ErrInvalidEmail},
		{name: "ID", run: func() error { _, err := repository.FindByUserID(t.Context(), security.UserID{}); return err }, want: security.ErrInvalidUserID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, tt.want) || db.query != "" {
				t.Fatalf("unexpected lookup: %v, %s", err, db.query)
			}
		})
	}

	var missing *PrincipalRepository

	if _, err := missing.FindByUserID(t.Context(), security.UserID{1}); !errors.Is(err, ErrNilPool) {
		t.Fatal(err)
	}

	if repo, err := NewPrincipalRepository(nil); repo != nil || !errors.Is(err, ErrNilPool) {
		t.Fatalf("constructor = %v, %v", repo, err)
	}
}
