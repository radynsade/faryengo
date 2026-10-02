package pgxgoqu

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/security"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestCredentialRepository(t *testing.T) {
	for _, tt := range []struct {
		name       string
		byEmail    bool
		rowErr     error
		nilVersion bool
		wantErr    error
	}{
		{name: "email", byEmail: true}, {name: "user ID"},
		{name: "not found", byEmail: true, rowErr: pgx.ErrNoRows, wantErr: security.ErrUserNotFound},
		{name: "query canceled", rowErr: context.Canceled, wantErr: context.Canceled},
		{name: "invalid version", nilVersion: true, wantErr: security.ErrInvalidSession},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, roleID, version := uuid.New(), uuid.New(), uuid.New()

			if tt.nilVersion {
				version = uuid.Nil
			}

			db := &fakeSecurityDB{row: fakeSecurityRow{err: tt.rowErr, values: []any{
				pgtype.UUID{Bytes: id, Valid: true}, pgtype.UUID{Bytes: roleID, Valid: true}, "person@example.com", "+37123456789", "hash", "First", "Last", pgtype.UUID{Bytes: version, Valid: true},
			}}}
			repository := &CredentialRepository{db: db}
			var credentials *security.Credentials
			var err error

			if tt.byEmail {
				credentials, err = repository.FindByEmail(context.Background(), "Person@example.com")
			} else {
				credentials, err = repository.FindByUserID(context.Background(), security.UserID(id))
			}

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v", err)
				}
			} else {
				if err != nil || credentials == nil || credentials.User.ID() != security.UserID(id) || credentials.Version != version {
					t.Fatalf("credentials = %v, error = %v", credentials, err)
				}

				if !strings.Contains(db.query, `"credential_version"`) || !strings.Contains(db.query, "$1") {
					t.Fatalf("query = %s", db.query)
				}

				if tt.byEmail {
					if !strings.Contains(db.query, `lower("email") = lower($1)`) {
						t.Fatalf("email query = %s", db.query)
					}
				} else if _, ok := db.queryArgs[0].(pgtype.UUID); !ok {
					t.Fatalf("UUID parameter type = %T", db.queryArgs[0])
				}
			}
		})
	}
}

func TestUserPasswordSnapshotUpdates(t *testing.T) {
	for _, tt := range []struct {
		name, password string
		conflict       bool
	}{
		{name: "profile preserves original credential", password: "hash"},
		{name: "password mutation matches original credential", password: "new-hash"},
		{name: "concurrent password change", password: "hash", conflict: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			user := testUser(t)

			if err := user.SetPasswordHash(security.PasswordHash(tt.password)); err != nil {
				t.Fatal(err)
			}

			tag := pgconn.NewCommandTag("UPDATE 1")

			if tt.conflict {
				tag = pgconn.NewCommandTag("UPDATE 0")
			}

			db := &fakeSecurityDB{execTag: tag, row: fakeSecurityRow{values: []any{
				pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
				"person@example.com", "+37123456789", "concurrently-changed-hash", "First", "Last",
			}}}
			err := (&UserRepository{db: db}).Update(t.Context(), user)

			if tt.conflict {
				if !errors.Is(err, security.ErrUserConflict) {
					t.Fatalf("stale update accepted: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(db.execQuery, `("password_hash" = $8)`) || len(db.execArgs) != 8 || db.execArgs[3] != tt.password || db.execArgs[7] != "hash" {
				t.Fatalf("password compare-and-swap missing: query %s, arguments %v", db.execQuery, db.execArgs)
			}
		})
	}
}
