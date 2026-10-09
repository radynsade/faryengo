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
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/pkg/domquery"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// The pool never connects: every case must fail before any I/O.
func newLazyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")

	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

func TestConstructorsRejectNilPool(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{"user", func() error { _, err := NewUserRepository(nil); return err }},
		{"role", func() error { _, err := NewRoleRepository(nil); return err }},
		{"credentials", func() error { _, err := NewCredentialsRepository(nil); return err }},
		{"typed nil pool", func() error { _, err := NewUserRepository((*pgxpool.Pool)(nil)); return err }},
		{"typed nil connection", func() error { _, err := NewRoleRepository((*pgx.Conn)(nil)); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, pgxdb.ErrNilDB) {
				t.Fatalf("got %v, want %v", err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestMethodsRejectMissingPool(t *testing.T) {
	ctx := context.Background()
	userID := users.UserID(uuid.Must(uuid.NewV7()))
	roleID := users.RoleID(uuid.Must(uuid.NewV7()))

	tests := []struct {
		name string
		call func() error
	}{
		{"user create", func() error { return (&UserRepository{}).Create(ctx, &users.User{}) }},
		{"user update", func() error { return (&UserRepository{}).Update(ctx, &users.User{}) }},
		{"user delete", func() error { return (&UserRepository{}).Delete(ctx, userID) }},
		{"user find by ID", func() error { _, err := (*UserRepository)(nil).FindByID(ctx, userID); return err }},
		{"user find by email", func() error { _, err := (&UserRepository{}).FindByEmail(ctx, "a@b.c"); return err }},
		{"role create", func() error { return (&RoleRepository{}).Create(ctx, &users.Role{}) }},
		{"role update", func() error { return (&RoleRepository{}).Update(ctx, &users.Role{}) }},
		{"role delete", func() error { return (&RoleRepository{}).Delete(ctx, roleID) }},
		{"role find by ID", func() error { _, err := (&RoleRepository{}).FindByID(ctx, roleID); return err }},
		{"role find by ID for update", func() error { _, err := (&RoleRepository{}).FindByIDForUpdate(ctx, roleID); return err }},
		{"role find", func() error { _, err := (&RoleRepository{}).Find(ctx, users.RoleQuery{}); return err }},
		{"role count", func() error { _, err := (&RoleRepository{}).Count(ctx, users.RoleFilter{}); return err }},
		{"credentials find by email", func() error {
			_, err := (&CredentialsSnapshotRepository{}).FindByEmail(ctx, "a@b.c")
			return err
		}},
		{"credentials find version", func() error {
			_, err := (*CredentialsSnapshotRepository)(nil).FindVersionByUserID(ctx, userID)
			return err
		}},
		{"credentials rotate version", func() error { return (&CredentialsSnapshotRepository{}).RotateVersion(ctx, userID) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, pgxdb.ErrNilDB) {
				t.Fatalf("got %v, want %v", err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestMethodsValidateBeforeIO(t *testing.T) {
	ctx := context.Background()
	pool := newLazyPool(t)
	userRepository := &UserRepository{pool: pool}
	roles := &RoleRepository{pool: pool}
	credentials := &CredentialsSnapshotRepository{pool: pool}
	validRoleID := users.RoleID(uuid.Must(uuid.NewV7()))
	unloadedUser := users.NewUser(
		users.UserID(uuid.Must(uuid.NewV7())),
		validRoleID,
		"user@example.com",
		time.Time{},
		"+15551234567",
		time.Time{},
		"hash",
		time.Time{},
		"Ada",
		"Lovelace",
		time.Time{},
		time.Time{},
	)

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"user create nil", func() error { return userRepository.Create(ctx, nil) }, users.ErrUserNil},
		{"user create invalid", func() error { return userRepository.Create(ctx, &users.User{}) }, users.ErrUserInvalid},
		{"user update nil", func() error { return userRepository.Update(ctx, nil) }, users.ErrUserNil},
		{"user update invalid", func() error { return userRepository.Update(ctx, &users.User{}) }, users.ErrUserInvalid},
		{"user update without a loaded version", func() error {
			return userRepository.Update(ctx, unloadedUser)
		}, users.ErrUserConflict},
		{"user delete invalid ID", func() error { return userRepository.Delete(ctx, users.UserID{}) }, users.ErrUserIDInvalid},
		{"user find by invalid ID", func() error {
			_, err := userRepository.FindByID(ctx, users.UserID{})
			return err
		}, users.ErrUserIDInvalid},
		{"user find by invalid email", func() error {
			_, err := userRepository.FindByEmail(ctx, "Ada <user@example.com>")
			return err
		}, users.ErrEmailInvalid},
		{"credentials find by invalid email", func() error {
			_, err := credentials.FindByEmail(ctx, "Ada <user@example.com>")
			return err
		}, users.ErrEmailInvalid},
		{"credentials find version by invalid ID", func() error {
			_, err := credentials.FindVersionByUserID(ctx, users.UserID{})
			return err
		}, users.ErrUserIDInvalid},
		{"credentials rotate version of invalid ID", func() error {
			return credentials.RotateVersion(ctx, users.UserID{})
		}, users.ErrUserIDInvalid},
		{"role create nil", func() error { return roles.Create(ctx, nil) }, users.ErrRoleNil},
		{"role create invalid", func() error { return roles.Create(ctx, &users.Role{}) }, users.ErrRoleInvalid},
		{"role update invalid", func() error { return roles.Update(ctx, &users.Role{}) }, users.ErrRoleInvalid},
		{"role delete invalid ID", func() error { return roles.Delete(ctx, users.RoleID{}) }, users.ErrRoleIDInvalid},
		{"role find by invalid ID", func() error {
			_, err := roles.FindByID(ctx, users.RoleID{})
			return err
		}, users.ErrRoleIDInvalid},
		{"role find by ID for update outside a transaction", func() error {
			_, err := roles.FindByIDForUpdate(ctx, validRoleID)
			return err
		}, pgxdb.ErrTransactionRequired},
		{"role find without sort", func() error {
			_, err := roles.Find(ctx, users.RoleQuery{Limit: 10, Page: 1})
			return err
		}, users.ErrInvalidRoleQuery},
		{"role find with zero page", func() error {
			_, err := roles.Find(ctx, users.RoleQuery{SortBy: users.RoleSortID, Limit: 10})
			return err
		}, users.ErrInvalidRoleQuery},
		{"role find with invalid filter", func() error {
			_, err := roles.Find(ctx, users.RoleQuery{
				Filter: users.RoleFilter{IDLike: "\x00"},
				SortBy: users.RoleSortID,
				Limit:  10,
				Page:   1,
			})
			return err
		}, users.ErrRoleFilterInvalid},
		{"role count with invalid filter", func() error {
			_, err := roles.Count(ctx, users.RoleFilter{NameLike: strings.Repeat("a", users.MaxRoleFilterNameLikeLength+1)})
			return err
		}, users.ErrRoleFilterInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestRoleFindQuery(t *testing.T) {
	isSuper := false

	tests := []struct {
		name     string
		query    users.RoleQuery
		wantSQL  []string
		wantArgs []any
	}{
		{
			name:     "sort by ID descending without a tie-breaker",
			query:    users.RoleQuery{SortBy: users.RoleSortID, SortOrder: domquery.SortOrderDesc, Limit: 10, Page: 3},
			wantSQL:  []string{`ORDER BY "id" DESC LIMIT $1 OFFSET $2`},
			wantArgs: []any{int64(10), int64(20)},
		},
		{
			name:  "sort by name with the ID tie-breaker",
			query: users.RoleQuery{SortBy: users.RoleSortName, SortOrder: domquery.SortOrderAsc, Limit: 5, Page: 1},
			wantSQL: []string{
				`ORDER BY l.is_fallback DESC, t.language_code LIMIT 1)) ASC, "id" ASC LIMIT $1`,
			},
			wantArgs: []any{int64(5)},
		},
		{
			name: "every filter",
			query: users.RoleQuery{
				Filter: users.RoleFilter{
					IDLike:      "01_",
					NameLike:    "50%",
					Permissions: users.Permissions{users.PermissionViewUser, users.PermissionViewRole},
					IsSuper:     &isSuper,
				},
				SortBy: users.RoleSortIsSuper,
				Limit:  1,
				Page:   1,
			},
			wantSQL: []string{
				`"role".id::text ILIKE $1`,
				`content ILIKE $2`,
				`("is_super" IS TRUE) OR "role".permissions @> ARRAY[$3::permission,$4::permission]::permission[]`,
				`("is_super" IS FALSE)`,
				`ORDER BY "is_super" DESC, "id" ASC`,
			},
			wantArgs: []any{`%01\_%`, `%50\%%`, "view_user", "view_role", int64(1)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql, args, err := roleFilterDataset(test.query.Filter).
				Select(roleColumns()...).
				Order(roleOrder(test.query)...).
				Limit(test.query.Limit).
				Offset((test.query.Page - 1) * test.query.Limit).
				Prepared(true).
				ToSQL()

			if err != nil {
				t.Fatalf("build query: %v", err)
			}

			for _, fragment := range test.wantSQL {
				if !strings.Contains(sql, fragment) {
					t.Errorf("query %q does not contain %q", sql, fragment)
				}
			}

			if len(args) != len(test.wantArgs) {
				t.Fatalf("got args %v, want %v", args, test.wantArgs)
			}

			for index, want := range test.wantArgs {
				if args[index] != want {
					t.Errorf("arg %d: got %#v, want %#v", index, args[index], want)
				}
			}
		})
	}
}

func TestMapErrors(t *testing.T) {
	cause := errors.New("connection reset")

	tests := []struct {
		name    string
		mapper  func(error) error
		err     error
		want    error
		wantRaw bool
	}{
		{"role in use", mapRoleError, &pgconn.PgError{Code: "23503", ConstraintName: "user_role_id_fkey"}, users.ErrRoleAlreadyInUse, true},
		{"role restricted", mapRoleError, &pgconn.PgError{Code: "23001", ConstraintName: "user_role_id_fkey"}, users.ErrRoleAlreadyInUse, true},
		{"role name language missing", mapRoleError, &pgconn.PgError{Code: "23503", ConstraintName: "translation_language_code_fkey"}, languages.ErrLanguageNotFound, true},
		{"role unrelated", mapRoleError, cause, cause, false},
		{"user role missing", mapUserError, &pgconn.PgError{Code: "23503", ConstraintName: "user_role_id_fkey"}, users.ErrRoleNotFound, true},
		{"user ID taken", mapUserError, &pgconn.PgError{Code: "23505", ConstraintName: "user_pkey"}, users.ErrUserAlreadyExists, true},
		{"user email taken", mapUserError, &pgconn.PgError{Code: "23505", ConstraintName: "user_email_unique_idx"}, users.ErrUserAlreadyExists, true},
		{"user unrelated", mapUserError, cause, cause, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.mapper(test.err)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if _, ok := errors.AsType[*pgconn.PgError](err); ok != test.wantRaw {
				t.Fatalf("PostgreSQL cause preserved: got %t, want %t", ok, test.wantRaw)
			}
		})
	}
}
