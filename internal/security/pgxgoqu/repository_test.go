package pgxgoqu

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/radynsade/faryengo/internal/security"
)

type fakeSecurityDB struct {
	execContext  context.Context
	execQuery    string
	execArgs     []any
	execErr      error
	queryContext context.Context
	query        string
	queryArgs    []any
	row          pgx.Row
}

func (f *fakeSecurityDB) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	f.execContext = ctx
	f.execQuery = query
	f.execArgs = args
	return pgconn.CommandTag{}, f.execErr
}

func (f *fakeSecurityDB) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	f.queryContext = ctx
	f.query = query
	f.queryArgs = args
	return f.row
}

type fakeSecurityRow struct {
	values []any
	err    error
}

func (r fakeSecurityRow) Scan(destinations ...any) error {
	if r.err != nil {
		return r.err
	}

	for index, value := range r.values {
		switch destination := destinations[index].(type) {
		case *pgtype.UUID:
			*destination = value.(pgtype.UUID)
		case *[]string:
			*destination = value.([]string)
		case *string:
			*destination = value.(string)
		}
	}

	return nil
}

func testRole(t *testing.T) *security.Role {
	t.Helper()
	role, err := security.NewRole(security.RoleID{1}, []security.Permission{
		security.PermissionViewUser, security.PermissionManageUser,
	})
	if err != nil {
		t.Fatalf("NewRole() error = %v", err)
	}

	return role
}

func testUser(t *testing.T) *security.User {
	t.Helper()
	user, err := security.NewUser(
		security.UserID{2},
		security.RoleID{1},
		"person@example.com",
		"+37123456789",
		"hash",
		"First",
		"Last",
	)
	if err != nil {
		t.Fatalf("NewUser() error = %v", err)
	}

	return user
}

func TestRepositoryConstructorsRejectNilPool(t *testing.T) {
	roleRepository, roleErr := NewRoleRepository(nil)
	userRepository, userErr := NewUserRepository(nil)

	if roleRepository != nil || !errors.Is(roleErr, ErrNilPool) || userRepository != nil || !errors.Is(userErr, ErrNilPool) {
		t.Fatalf("constructors = (%v, %v), (%v, %v), want nil repositories and ErrNilPool", roleRepository, roleErr, userRepository, userErr)
	}
}

func TestRoleRepositorySave(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name     string
		role     *security.Role
		execErr  error
		wantErr  error
		wantExec bool
	}{
		{name: "upsert", role: testRole(t), wantExec: true},
		{name: "empty permissions", role: mustRole(t, nil), wantExec: true},
		{name: "nil role", wantErr: ErrNilRole},
		{name: "invalid role", role: &security.Role{}, wantErr: security.ErrInvalidRoleID},
		{name: "database error", role: testRole(t), execErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantExec: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeSecurityDB{execErr: tt.execErr}
			repository := &RoleRepository{db: db}
			err := repository.Save(ctx, tt.role)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Save() error = %v, want %v", err, tt.wantErr)
			}

			if (db.execQuery != "") != tt.wantExec {
				t.Fatalf("Save() executed query = %v, want %v", db.execQuery != "", tt.wantExec)
			}

			if tt.wantExec {
				if db.execContext != ctx {
					t.Fatal("Save() did not forward context")
				}

				if !strings.Contains(db.execQuery, `INSERT INTO "role"`) || !strings.Contains(db.execQuery, "ON CONFLICT") || !strings.Contains(db.execQuery, `"excluded"."permissions"`) || !strings.Contains(db.execQuery, "::permission[]") || strings.Contains(db.execQuery, "manage_user") {
					t.Fatalf("Save() query = %q, want parameterized role upsert", db.execQuery)
				}

				if len(db.execArgs) == 0 || db.execArgs[0] != (pgtype.UUID{Bytes: [16]byte{1}, Valid: true}) {
					t.Fatalf("Save() args = %v, want native UUID first", db.execArgs)
				}

				if len(tt.role.Permissions()) == 0 {
					if !strings.Contains(db.execQuery, "ARRAY[]::permission[]") || len(db.execArgs) != 1 {
						t.Fatalf("Save() = (%q, %v), want empty text array", db.execQuery, db.execArgs)
					}
				} else if !strings.Contains(db.execQuery, "ARRAY[$2::permission,$3::permission]::permission[]") || len(db.execArgs) != 3 || db.execArgs[1] != "view_user" || db.execArgs[2] != "manage_user" {
					t.Fatalf("Save() = (%q, %v), want ordered bound permissions", db.execQuery, db.execArgs)
				}
			}
		})
	}
}

func mustRole(t *testing.T, permissions []security.Permission) *security.Role {
	t.Helper()
	role, err := security.NewRole(security.RoleID{1}, permissions)
	if err != nil {
		t.Fatalf("NewRole() error = %v", err)
	}

	return role
}

func TestRoleRepositoryFindByID(t *testing.T) {
	ctx := context.Background()
	rowID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	for _, tt := range []struct {
		name            string
		id              security.RoleID
		row             pgx.Row
		wantErr         error
		wantQuery       bool
		wantPermissions []security.Permission
	}{
		{name: "found", id: security.RoleID{1}, row: fakeSecurityRow{values: []any{rowID, []string{"view_user", "manage_user"}}}, wantQuery: true, wantPermissions: []security.Permission{security.PermissionViewUser, security.PermissionManageUser}},
		{name: "found empty", id: security.RoleID{1}, row: fakeSecurityRow{values: []any{rowID, []string{}}}, wantQuery: true},
		{name: "not found", id: security.RoleID{1}, row: fakeSecurityRow{err: pgx.ErrNoRows}, wantErr: security.ErrRoleNotFound, wantQuery: true},
		{name: "invalid ID", wantErr: security.ErrInvalidRoleID},
		{name: "scan error", id: security.RoleID{1}, row: fakeSecurityRow{err: context.DeadlineExceeded}, wantErr: context.DeadlineExceeded, wantQuery: true},
		{name: "invalid stored permission", id: security.RoleID{1}, row: fakeSecurityRow{values: []any{rowID, []string{"unknown"}}}, wantErr: security.ErrInvalidPermission, wantQuery: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeSecurityDB{row: tt.row}
			repository := &RoleRepository{db: db}
			role, err := repository.FindByID(ctx, tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FindByID() error = %v, want %v", err, tt.wantErr)
			}

			if (db.query != "") != tt.wantQuery {
				t.Fatalf("FindByID() queried = %v, want %v", db.query != "", tt.wantQuery)
			}

			if tt.wantQuery {
				if db.queryContext != ctx || !strings.Contains(db.query, `FROM "role"`) || !strings.Contains(db.query, `"permissions"::text[]`) || !strings.Contains(db.query, "$1") {
					t.Fatalf("FindByID() query = %q, context = %v", db.query, db.queryContext)
				}

				if len(db.queryArgs) != 1 || db.queryArgs[0] != (pgtype.UUID{Bytes: [16]byte{1}, Valid: true}) {
					t.Fatalf("FindByID() args = %v, want native UUID", db.queryArgs)
				}
			}

			if tt.wantErr == nil {
				if role == nil || role.ID() != (security.RoleID{1}) || !slices.Equal(role.Permissions(), tt.wantPermissions) {
					t.Fatalf("FindByID() role = %v, want stored role", role)
				}
			} else if role != nil {
				t.Fatalf("FindByID() role = %v, want nil on error", role)
			}
		})
	}
}

func TestUserRepositorySave(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name     string
		user     *security.User
		execErr  error
		wantErr  error
		wantExec bool
	}{
		{name: "upsert", user: testUser(t), wantExec: true},
		{name: "nil user", wantErr: ErrNilUser},
		{name: "invalid user", user: &security.User{}, wantErr: security.ErrInvalidRoleID},
		{name: "database error", user: testUser(t), execErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantExec: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeSecurityDB{execErr: tt.execErr}
			repository := &UserRepository{db: db}
			err := repository.Save(ctx, tt.user)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Save() error = %v, want %v", err, tt.wantErr)
			}

			if (db.execQuery != "") != tt.wantExec {
				t.Fatalf("Save() executed query = %v, want %v", db.execQuery != "", tt.wantExec)
			}

			if tt.wantExec {
				if db.execContext != ctx || !strings.Contains(db.execQuery, `INSERT INTO "user"`) || !strings.Contains(db.execQuery, "ON CONFLICT") || !strings.Contains(db.execQuery, `"excluded"."role_id"`) || strings.Contains(db.execQuery, "person@example.com") {
					t.Fatalf("Save() query = %q, context = %v", db.execQuery, db.execContext)
				}

				if len(db.execArgs) != 7 || db.execArgs[0] != (pgtype.UUID{Bytes: [16]byte{2}, Valid: true}) || db.execArgs[1] != (pgtype.UUID{Bytes: [16]byte{1}, Valid: true}) || db.execArgs[2] != "person@example.com" || db.execArgs[4] != "hash" {
					t.Fatalf("Save() args = %v, want seven bound values with UUIDs first", db.execArgs)
				}
			}
		})
	}
}

func TestUserRepositoryFindByID(t *testing.T) {
	ctx := context.Background()
	rowID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	rowRoleID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	for _, tt := range []struct {
		name    string
		row     pgx.Row
		wantErr error
	}{
		{name: "found", row: fakeSecurityRow{values: []any{rowID, rowRoleID, "person@example.com", "+37123456789", "hash", "First", "Last"}}},
		{name: "not found", row: fakeSecurityRow{err: pgx.ErrNoRows}, wantErr: security.ErrUserNotFound},
		{name: "scan error", row: fakeSecurityRow{err: context.DeadlineExceeded}, wantErr: context.DeadlineExceeded},
		{name: "invalid stored email", row: fakeSecurityRow{values: []any{rowID, rowRoleID, "", "+37123456789", "hash", "First", "Last"}}, wantErr: security.ErrInvalidEmail},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeSecurityDB{row: tt.row}
			repository := &UserRepository{db: db}
			user, err := repository.FindByID(ctx, security.UserID{2})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FindByID() error = %v, want %v", err, tt.wantErr)
			}

			if db.queryContext != ctx || !strings.Contains(db.query, `FROM "user"`) || !strings.Contains(db.query, "$1") {
				t.Fatalf("FindByID() query = %q, context = %v", db.query, db.queryContext)
			}

			if len(db.queryArgs) != 1 || db.queryArgs[0] != (pgtype.UUID{Bytes: [16]byte{2}, Valid: true}) {
				t.Fatalf("FindByID() args = %v, want native UUID", db.queryArgs)
			}

			if tt.wantErr == nil {
				if user == nil || user.ID() != (security.UserID{2}) || user.RoleID() != (security.RoleID{1}) || user.Email() != "person@example.com" || user.Phone() != "+37123456789" || user.PasswordHash() != "hash" || user.FirstName() != "First" || user.LastName() != "Last" {
					t.Fatalf("FindByID() user = %v, want stored user", user)
				}
			} else if user != nil {
				t.Fatalf("FindByID() user = %v, want nil on error", user)
			}
		})
	}
}

func TestRepositoryNilReceivers(t *testing.T) {
	ctx := context.Background()
	var roleRepository *RoleRepository
	var userRepository *UserRepository
	if err := roleRepository.Save(ctx, testRole(t)); !errors.Is(err, ErrNilPool) {
		t.Fatalf("RoleRepository.Save(nil receiver) = %v, want %v", err, ErrNilPool)
	}

	if _, err := roleRepository.FindByID(ctx, security.RoleID{1}); !errors.Is(err, ErrNilPool) {
		t.Fatalf("RoleRepository.FindByID(nil receiver) = %v, want %v", err, ErrNilPool)
	}

	if err := userRepository.Save(ctx, testUser(t)); !errors.Is(err, ErrNilPool) {
		t.Fatalf("UserRepository.Save(nil receiver) = %v, want %v", err, ErrNilPool)
	}

	if _, err := userRepository.FindByID(ctx, security.UserID{2}); !errors.Is(err, ErrNilPool) {
		t.Fatalf("UserRepository.FindByID(nil receiver) = %v, want %v", err, ErrNilPool)
	}
}

func TestBindUUIDArgsRejectsMissingArguments(t *testing.T) {
	if err := bindUUIDArgs(nil, [16]byte{1}); err == nil {
		t.Fatal("bindUUIDArgs(nil) = nil, want an error")
	}
}
