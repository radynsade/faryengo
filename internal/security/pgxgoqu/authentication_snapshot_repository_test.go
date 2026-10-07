package pgxgoqu

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/security"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestAuthenticationSnapshotRepository(t *testing.T) {
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
		{name: "invalid version", nilVersion: true, wantErr: security.ErrInvalidAuthenticationSnapshot},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, roleID, version := newTestUUID(t), newTestUUID(t), newTestUUID(t)

			if tt.nilVersion {
				version = uuid.Nil
			}

			db := &fakeSecurityDB{row: fakeSecurityRow{err: tt.rowErr, values: []any{
				pgtype.UUID{Bytes: id, Valid: true}, pgtype.UUID{Bytes: roleID, Valid: true}, "person@example.com", "+37123456789", "hash", "First", "Last", pgtype.UUID{Bytes: version, Valid: true}, time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			}}}
			repository := &AuthenticationSnapshotRepository{pool: db}
			var credentials *security.AuthenticationSnapshot
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
				if err != nil || credentials == nil || credentials.User.ID != security.UserID(id) || credentials.Version != version {
					t.Fatalf("credentials = %v, error = %v", credentials, err)
				}

				if !strings.Contains(db.query, `"authentication_snapshot_version"`) || !strings.Contains(db.query, "$1") {
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

func TestAuthenticationSnapshotRepositoryInvalidate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		tag     string
		execErr error
		wantErr error
	}{
		{name: "invalidated", tag: "UPDATE 1"},
		{name: "not found", tag: "UPDATE 0", wantErr: security.ErrUserNotFound},
		{name: "query canceled", execErr: context.Canceled, wantErr: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id := newTestUUID(t)
			ctx := t.Context()
			db := &fakeSecurityDB{execTag: pgconn.NewCommandTag(tt.tag), execErr: tt.execErr}
			err := (&AuthenticationSnapshotRepository{pool: db}).Invalidate(ctx, security.UserID(id))

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Invalidate() error = %v, want %v", err, tt.wantErr)
			}

			if db.execContext != ctx || !strings.Contains(db.execQuery, `"authentication_snapshot_version"=uuidv7()`) ||
				len(db.execArgs) != 1 || db.execArgs[0] != (pgtype.UUID{Bytes: id, Valid: true}) {
				t.Fatalf("credential invalidation query = %s, arguments = %v", db.execQuery, db.execArgs)
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

			user.PasswordHash = security.PasswordHash(tt.password)

			tag := pgconn.NewCommandTag("UPDATE 1")

			if tt.conflict {
				tag = pgconn.NewCommandTag("UPDATE 0")
			}

			db := &fakeSecurityDB{execTag: tag, row: fakeSecurityRow{values: []any{
				pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
				"person@example.com", "+37123456789", "concurrently-changed-hash", "First", "Last", time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			}}}
			err := (&UserRepository{pool: db}).Update(t.Context(), user)

			if tt.conflict {
				if !errors.Is(err, security.ErrUserConflict) {
					t.Fatalf("stale update accepted: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(db.execQuery, `("updated_at" = $8)`) || len(db.execArgs) != 8 || db.execArgs[3] != tt.password || !db.execArgs[7].(time.Time).Equal(user.UpdatedAt) {
				t.Fatalf("password compare-and-swap missing: query %s, arguments %v", db.execQuery, db.execArgs)
			}
		})
	}
}

func newTestUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()

	if err != nil {
		t.Fatalf("generate UUIDv7: %v", err)
	}

	return id
}
