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

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
)

type fakeSecurityDB struct {
	execContext  context.Context
	execQuery    string
	execArgs     []any
	execErr      error
	execTag      pgconn.CommandTag
	queryContext context.Context
	query        string
	queryArgs    []any
	row          pgx.Row
	beginErr     error
	tx           *fakeRoleTx
}

func (f *fakeSecurityDB) Begin(ctx context.Context) (pgx.Tx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}

	return f.tx, nil
}

func (f *fakeSecurityDB) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	f.execContext = ctx
	f.execQuery = query
	f.execArgs = args
	return f.execTag, f.execErr
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
		case *int64:
			*destination = value.(int64)
		case *pgtype.UUID:
			*destination = value.(pgtype.UUID)
		case *[]string:
			*destination = value.([]string)
		case *string:
			*destination = value.(string)
		case *bool:
			*destination = value.(bool)
		}
	}

	return nil
}

func testRole(t *testing.T) *security.Role {
	t.Helper()
	role, err := security.NewRole(security.RoleID{1}, testRoleName(t), []security.Permission{
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

func testRoleName(t *testing.T) languages.Text {
	t.Helper()
	english, err := languages.NewTranslation("en", "Administrator")
	if err != nil {
		t.Fatalf("NewTranslation() error = %v", err)
	}

	latvian, err := languages.NewTranslation("lv", "Administrators")
	if err != nil {
		t.Fatalf("NewTranslation() error = %v", err)
	}

	name, err := languages.NewText([]languages.Translation{latvian, english})
	if err != nil {
		t.Fatalf("NewText() error = %v", err)
	}

	return name
}

type fakeRoleTx struct {
	pgx.Tx
	queries    []string
	args       [][]any
	rows       []pgx.Row
	execErr    error
	execFailOn string
	commitErr  error
	committed  bool
	rolledBack bool
}

func (t *fakeRoleTx) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	t.queries = append(t.queries, query)
	t.args = append(t.args, args)
	row := t.rows[0]
	t.rows = t.rows[1:]
	return row
}

func (t *fakeRoleTx) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	t.queries = append(t.queries, query)
	t.args = append(t.args, args)
	if t.execFailOn == "" || strings.Contains(query, t.execFailOn) {
		return pgconn.CommandTag{}, t.execErr
	}

	return pgconn.CommandTag{}, nil
}

func (t *fakeRoleTx) Commit(context.Context) error {
	t.committed = true
	return t.commitErr
}

func (t *fakeRoleTx) Rollback(context.Context) error {
	if !t.committed {
		t.rolledBack = true
	}

	return nil
}

func mustRole(t *testing.T, permissions []security.Permission) *security.Role {
	t.Helper()
	role, err := security.NewRole(security.RoleID{1}, testRoleName(t), permissions)
	if err != nil {
		t.Fatalf("NewRole() error = %v", err)
	}

	return role
}

func TestRoleRepositoryCreate(t *testing.T) {
	ctx := context.Background()
	super := mustRole(t, nil)
	super.SetIsSuper(true)
	missingLanguage := &pgconn.PgError{Code: "23503", ConstraintName: "translation_language_code_fkey"}

	for _, tt := range []struct {
		name       string
		role       *security.Role
		insertErr  error
		beginErr   error
		execErr    error
		wantErr    error
		wantCommit bool
	}{
		{name: "created", role: testRole(t), wantCommit: true},
		{name: "role permissions", role: mustRole(t, []security.Permission{security.PermissionManageRole, security.PermissionViewRole}), wantCommit: true},
		{name: "empty permissions", role: mustRole(t, nil), wantCommit: true},
		{name: "super", role: super, wantCommit: true},
		{name: "duplicate", role: testRole(t), insertErr: pgx.ErrNoRows, wantErr: security.ErrRoleAlreadyExists},
		{name: "nil role", wantErr: ErrNilRole},
		{name: "invalid role", role: &security.Role{}, wantErr: security.ErrInvalidRoleID},
		{name: "begin error", role: testRole(t), beginErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
		{name: "translation error", role: testRole(t), execErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
		{name: "missing language", role: testRole(t), execErr: missingLanguage, wantErr: languages.ErrLanguageNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tx := &fakeRoleTx{rows: []pgx.Row{
				fakeSecurityRow{values: []any{int64(10)}},
				fakeSecurityRow{values: []any{int64(10)}, err: tt.insertErr},
			}, execErr: tt.execErr, execFailOn: `INSERT INTO "translation"`}
			db := &fakeSecurityDB{tx: tx, beginErr: tt.beginErr}
			repository := &RoleRepository{db: db}
			err := repository.Create(ctx, tt.role)
			if !errors.Is(err, tt.wantErr) || tx.committed != tt.wantCommit {
				t.Fatalf("Create() error = %v, committed = %v; want %v and %v", err, tx.committed, tt.wantErr, tt.wantCommit)
			}

			if tt.execErr != nil && !errors.Is(err, tt.execErr) {
				t.Fatal("translation error lost its original cause")
			}

			if len(tx.queries) > 0 && tx.rolledBack == tt.wantCommit {
				t.Fatalf("Create() rolled back = %v, want %v", tx.rolledBack, !tt.wantCommit)
			}

			if tt.wantCommit {
				if len(tx.queries) != 4 || !strings.Contains(tx.queries[1], `INSERT INTO "role"`) || !strings.Contains(tx.queries[1], "ON CONFLICT DO NOTHING") || !strings.Contains(tx.queries[1], "::permission[]") || strings.Contains(tx.queries[1], "manage_user") {
					t.Fatalf("Create() queries = %v, want role insert and two translation inserts", tx.queries)
				}

				if tx.args[1][0] != (pgtype.UUID{Bytes: [16]byte{1}, Valid: true}) || tx.args[1][1] != int64(10) {
					t.Fatalf("Create() role args = %v", tx.args[1])
				}

				if len(tt.role.Permissions()) == 0 {
					if !strings.Contains(tx.queries[1], "ARRAY[]::permission[]") || len(tx.args[1]) != 3 {
						t.Fatalf("Create() role args = %v, want empty permissions", tx.args[1])
					}
				} else {
					permissions := tt.role.Permissions()

					if len(tx.args[1]) != len(permissions)+3 {
						t.Fatalf("Create() role args = %v, want bound permissions", tx.args[1])
					}

					for index, permission := range permissions {
						if tx.args[1][index+2] != string(permission) || strings.Contains(tx.queries[1], string(permission)) {
							t.Fatalf("Create() role args = %v, want bound permission %q", tx.args[1], permission)
						}
					}
				}

				if !strings.Contains(tx.queries[1], `"is_super"`) || tx.args[1][len(tx.args[1])-1] != tt.role.IsSuper() {
					t.Fatalf("Create() lost the super flag: %v", tx.args[1])
				}

				if tx.args[2][1] != "en" || tx.args[2][2] != "Administrator" || tx.args[3][1] != "lv" {
					t.Fatalf("Create() translation args = %v", tx.args[2:])
				}
			}
		})
	}
}

func TestRoleRepositoryUpdate(t *testing.T) {
	ctx := context.Background()
	super := mustRole(t, nil)
	super.SetIsSuper(true)
	missingLanguage := &pgconn.PgError{Code: "23503", ConstraintName: "translation_language_code_fkey"}

	for _, tt := range []struct {
		name       string
		role       *security.Role
		updateErr  error
		beginErr   error
		execErr    error
		wantErr    error
		wantCommit bool
	}{
		{name: "updated", role: testRole(t), wantCommit: true},
		{name: "role permissions", role: mustRole(t, []security.Permission{security.PermissionManageRole, security.PermissionViewRole}), wantCommit: true},
		{name: "clear permissions", role: mustRole(t, nil), wantCommit: true},
		{name: "super", role: super, wantCommit: true},
		{name: "not found", role: testRole(t), updateErr: pgx.ErrNoRows, wantErr: security.ErrRoleNotFound},
		{name: "nil role", wantErr: ErrNilRole},
		{name: "invalid role", role: &security.Role{}, wantErr: security.ErrInvalidRoleID},
		{name: "begin error", role: testRole(t), beginErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
		{name: "translation error", role: testRole(t), execErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
		{name: "missing language", role: testRole(t), execErr: missingLanguage, wantErr: languages.ErrLanguageNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tx := &fakeRoleTx{rows: []pgx.Row{fakeSecurityRow{values: []any{int64(9)}, err: tt.updateErr}}, execErr: tt.execErr, execFailOn: `INSERT INTO "translation"`}
			db := &fakeSecurityDB{tx: tx, beginErr: tt.beginErr}
			repository := &RoleRepository{db: db}
			err := repository.Update(ctx, tt.role)
			if !errors.Is(err, tt.wantErr) || tx.committed != tt.wantCommit {
				t.Fatalf("Update() error = %v, committed = %v; want %v and %v", err, tx.committed, tt.wantErr, tt.wantCommit)
			}

			if len(tx.queries) > 0 && tx.rolledBack == tt.wantCommit {
				t.Fatalf("Update() rolled back = %v, want %v", tx.rolledBack, !tt.wantCommit)
			}

			if tt.wantCommit {
				if len(tx.queries) != 4 || !strings.Contains(tx.queries[0], `UPDATE "role"`) || !strings.Contains(tx.queries[0], `RETURNING "name_id"`) || !strings.Contains(tx.queries[1], `DELETE FROM "translation"`) || tx.args[1][0] != int64(9) {
					t.Fatalf("Update() queries = %v, args = %v", tx.queries, tx.args)
				}

				if tx.args[0][len(tx.args[0])-1] != (pgtype.UUID{Bytes: [16]byte{1}, Valid: true}) || tx.args[2][0] != int64(9) || tx.args[3][0] != int64(9) {
					t.Fatalf("Update() args = %v, want existing role and name ID", tx.args)
				}

				if !strings.Contains(tx.queries[0], `"is_super"`) || tx.args[0][0] != tt.role.IsSuper() {
					t.Fatalf("Update() lost the super flag: %v", tx.args[0])
				}

				permissions := tt.role.Permissions()

				if len(tx.args[0]) != len(permissions)+2 {
					t.Fatalf("Update() role args = %v, want bound permissions", tx.args[0])
				}

				for index, permission := range permissions {
					if tx.args[0][index+1] != string(permission) || strings.Contains(tx.queries[0], string(permission)) {
						t.Fatalf("Update() role args = %v, want bound permission %q", tx.args[0], permission)
					}
				}
			}
		})
	}
}

func TestRoleRepositoryDelete(t *testing.T) {
	ctx := context.Background()
	assigned := &pgconn.PgError{Code: "23503", ConstraintName: "user_role_id_fkey"}
	other := &pgconn.PgError{Code: "23503", ConstraintName: "other_fkey"}

	for _, tt := range []struct {
		name           string
		id             security.RoleID
		tag            pgconn.CommandTag
		storeErr, want error
	}{
		{name: "deleted", id: security.RoleID{1}, tag: pgconn.NewCommandTag("DELETE 1")},
		{name: "missing", id: security.RoleID{1}, tag: pgconn.NewCommandTag("DELETE 0"), want: security.ErrRoleNotFound},
		{name: "assigned", id: security.RoleID{1}, storeErr: assigned, want: security.ErrRoleAlreadyInUse},
		{name: "unrelated constraint", id: security.RoleID{1}, storeErr: other, want: other},
		{name: "canceled", id: security.RoleID{1}, storeErr: context.Canceled, want: context.Canceled},
		{name: "invalid ID", want: security.ErrInvalidRoleID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeSecurityDB{execTag: tt.tag, execErr: tt.storeErr}
			repository := &RoleRepository{db: db}
			err := repository.Delete(ctx, tt.id)

			if !errors.Is(err, tt.want) {
				t.Fatalf("Delete() = %v, want %v", err, tt.want)
			}

			if tt.storeErr != nil && !errors.Is(err, tt.storeErr) {
				t.Fatal("original database error lost")
			}

			if tt.id == (security.RoleID{}) {
				if db.execQuery != "" {
					t.Fatal("invalid ID accessed database")
				}
			} else if db.execContext != ctx || !strings.Contains(db.execQuery, `DELETE FROM "role"`) || len(db.execArgs) != 1 || db.execArgs[0] != (pgtype.UUID{Bytes: [16]byte{1}, Valid: true}) {
				t.Fatalf("incorrect delete query: %+v", db)
			}
		})
	}

	for _, repository := range []*RoleRepository{nil, {}} {
		if err := repository.Delete(ctx, security.RoleID{1}); !errors.Is(err, ErrNilPool) {
			t.Fatalf("nil repository error = %v", err)
		}
	}
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
		wantSuper       bool
	}{
		{name: "found", id: security.RoleID{1}, row: fakeSecurityRow{values: []any{rowID, []string{"view_user", "manage_user"}, []string{"en", "lv"}, []string{"Administrator", "Administrators"}, false}}, wantQuery: true, wantPermissions: []security.Permission{security.PermissionViewUser, security.PermissionManageUser}},
		{name: "role permissions", id: security.RoleID{1}, row: fakeSecurityRow{values: []any{rowID, []string{"manage_role", "view_role"}, []string{"en", "lv"}, []string{"Administrator", "Administrators"}, false}}, wantQuery: true, wantPermissions: []security.Permission{security.PermissionManageRole, security.PermissionViewRole}},
		{name: "super", id: security.RoleID{1}, row: fakeSecurityRow{values: []any{rowID, []string{}, []string{"en", "lv"}, []string{"Administrator", "Administrators"}, true}}, wantQuery: true, wantSuper: true},
		{name: "not found", id: security.RoleID{1}, row: fakeSecurityRow{err: pgx.ErrNoRows}, wantErr: security.ErrRoleNotFound, wantQuery: true},
		{name: "invalid ID", wantErr: security.ErrInvalidRoleID},
		{name: "scan error", id: security.RoleID{1}, row: fakeSecurityRow{err: context.DeadlineExceeded}, wantErr: context.DeadlineExceeded, wantQuery: true},
		{name: "invalid stored permission", id: security.RoleID{1}, row: fakeSecurityRow{values: []any{rowID, []string{"unknown"}, []string{"en"}, []string{"Administrator"}, false}}, wantErr: security.ErrInvalidPermission, wantQuery: true},
		{name: "missing translation", id: security.RoleID{1}, row: fakeSecurityRow{values: []any{rowID, []string{}, []string{}, []string{}, false}}, wantErr: security.ErrInvalidRoleName, wantQuery: true},
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

			if tt.wantQuery && (db.queryContext != ctx || !strings.Contains(db.query, `FROM "role"`) || !strings.Contains(db.query, `"permissions"::text[]`) || !strings.Contains(db.query, `FROM "translation"`) || len(db.queryArgs) != 1 || db.queryArgs[0] != (pgtype.UUID{Bytes: [16]byte{1}, Valid: true})) {
				t.Fatalf("FindByID() query = %q, args = %v", db.query, db.queryArgs)
			}

			if tt.wantErr == nil {
				if role == nil || role.ID() != (security.RoleID{1}) || role.IsSuper() != tt.wantSuper || !slices.Equal(role.Permissions(), tt.wantPermissions) || len(role.Name()) != 2 || role.Name()["en"].Content() != "Administrator" || role.Name()["lv"].Content() != "Administrators" {
					t.Fatalf("FindByID() role = %v, want stored role", role)
				}
			} else if role != nil {
				t.Fatalf("FindByID() role = %v, want nil on error", role)
			}
		})
	}
}

func TestUserRepositoryCreate(t *testing.T) {
	ctx := context.Background()
	otherForeignKey := &pgconn.PgError{Code: "23503", ConstraintName: "other_fkey"}

	for _, tt := range []struct {
		name     string
		user     *security.User
		tag      pgconn.CommandTag
		execErr  error
		wantErr  error
		wantExec bool
	}{
		{name: "created", user: testUser(t), tag: pgconn.NewCommandTag("INSERT 0 1"), wantExec: true},
		{name: "duplicate", user: testUser(t), tag: pgconn.NewCommandTag("INSERT 0 0"), wantErr: security.ErrUserAlreadyExists, wantExec: true},
		{name: "missing role", user: testUser(t), execErr: &pgconn.PgError{Code: "23503", ConstraintName: "user_role_id_fkey"}, wantErr: security.ErrRoleNotFound, wantExec: true},
		{name: "unrelated foreign key", user: testUser(t), execErr: otherForeignKey, wantErr: otherForeignKey, wantExec: true},
		{name: "nil user", wantErr: ErrNilUser},
		{name: "invalid user", user: &security.User{}, wantErr: security.ErrInvalidRoleID},
		{name: "database error", user: testUser(t), execErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantExec: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeSecurityDB{execTag: tt.tag, execErr: tt.execErr}
			repository := &UserRepository{db: db}
			err := repository.Create(ctx, tt.user)

			if tt.execErr != nil && !errors.Is(err, tt.execErr) {
				t.Fatalf("Create() lost the database error: %v", err)
			}

			if !errors.Is(err, tt.wantErr) || (db.execQuery != "") != tt.wantExec {
				t.Fatalf("Create() error = %v, query = %q; want %v and exec %v", err, db.execQuery, tt.wantErr, tt.wantExec)
			}

			if tt.wantExec {
				if db.execContext != ctx || !strings.Contains(db.execQuery, `INSERT INTO "user"`) || !strings.Contains(db.execQuery, "ON CONFLICT DO NOTHING") || strings.Contains(db.execQuery, "person@example.com") {
					t.Fatalf("Create() query = %q, context = %v", db.execQuery, db.execContext)
				}

				if len(db.execArgs) != 7 || db.execArgs[0] != (pgtype.UUID{Bytes: [16]byte{2}, Valid: true}) || db.execArgs[1] != (pgtype.UUID{Bytes: [16]byte{1}, Valid: true}) || db.execArgs[2] != "person@example.com" || db.execArgs[4] != "hash" {
					t.Fatalf("Create() args = %v, want seven bound values with UUIDs first", db.execArgs)
				}
			}
		})
	}
}

func TestUserRepositoryUpdate(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name     string
		user     *security.User
		tag      pgconn.CommandTag
		execErr  error
		wantErr  error
		wantExec bool
	}{
		{name: "updated", user: testUser(t), tag: pgconn.NewCommandTag("UPDATE 1"), wantExec: true},
		{name: "not found", user: testUser(t), tag: pgconn.NewCommandTag("UPDATE 0"), wantErr: security.ErrUserNotFound, wantExec: true},
		{name: "nil user", wantErr: ErrNilUser},
		{name: "invalid user", user: &security.User{}, wantErr: security.ErrInvalidRoleID},
		{name: "database error", user: testUser(t), execErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantExec: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeSecurityDB{execTag: tt.tag, execErr: tt.execErr, row: fakeSecurityRow{err: pgx.ErrNoRows}}
			repository := &UserRepository{db: db}
			err := repository.Update(ctx, tt.user)
			if !errors.Is(err, tt.wantErr) || (db.execQuery != "") != tt.wantExec {
				t.Fatalf("Update() error = %v, query = %q; want %v and exec %v", err, db.execQuery, tt.wantErr, tt.wantExec)
			}

			if tt.wantExec {
				if db.execContext != ctx || !strings.Contains(db.execQuery, `UPDATE "user"`) || !strings.Contains(db.execQuery, `"id" =`) || strings.Contains(db.execQuery, "person@example.com") {
					t.Fatalf("Update() query = %q, context = %v", db.execQuery, db.execContext)
				}

				if len(db.execArgs) != 8 || db.execArgs[len(db.execArgs)-3] != (pgtype.UUID{Bytes: [16]byte{1}, Valid: true}) || db.execArgs[len(db.execArgs)-2] != (pgtype.UUID{Bytes: [16]byte{2}, Valid: true}) {
					t.Fatalf("Update() args = %v, want eight bound values with the credential snapshot last", db.execArgs)
				}
			}
		})
	}
}

func TestUserRepositoryDelete(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name    string
		tag     pgconn.CommandTag
		execErr error
		wantErr error
	}{
		{name: "deleted", tag: pgconn.NewCommandTag("DELETE 1")},
		{name: "not found", tag: pgconn.NewCommandTag("DELETE 0"), wantErr: security.ErrUserNotFound},
		{name: "database error", execErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeSecurityDB{execTag: tt.tag, execErr: tt.execErr}
			repository := &UserRepository{db: db}
			err := repository.Delete(ctx, security.UserID{2})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Delete() error = %v, want %v", err, tt.wantErr)
			}

			if db.execContext != ctx || !strings.Contains(db.execQuery, `DELETE FROM "user"`) || !strings.Contains(db.execQuery, `WHERE ("id" =`) || len(db.execArgs) != 1 || db.execArgs[0] != (pgtype.UUID{Bytes: [16]byte{2}, Valid: true}) {
				t.Fatalf("Delete() query = %q, args = %v, context = %v", db.execQuery, db.execArgs, db.execContext)
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
	if err := roleRepository.Create(ctx, testRole(t)); !errors.Is(err, ErrNilPool) {
		t.Fatalf("RoleRepository.Create(nil receiver) = %v, want %v", err, ErrNilPool)
	}

	if err := roleRepository.Update(ctx, testRole(t)); !errors.Is(err, ErrNilPool) {
		t.Fatalf("RoleRepository.Update(nil receiver) = %v, want %v", err, ErrNilPool)
	}

	if _, err := roleRepository.FindByID(ctx, security.RoleID{1}); !errors.Is(err, ErrNilPool) {
		t.Fatalf("RoleRepository.FindByID(nil receiver) = %v, want %v", err, ErrNilPool)
	}

	if err := userRepository.Create(ctx, testUser(t)); !errors.Is(err, ErrNilPool) {
		t.Fatalf("UserRepository.Create(nil receiver) = %v, want %v", err, ErrNilPool)
	}

	if err := userRepository.Update(ctx, testUser(t)); !errors.Is(err, ErrNilPool) {
		t.Fatalf("UserRepository.Update(nil receiver) = %v, want %v", err, ErrNilPool)
	}

	if err := userRepository.Delete(ctx, security.UserID{2}); !errors.Is(err, ErrNilPool) {
		t.Fatalf("UserRepository.Delete(nil receiver) = %v, want %v", err, ErrNilPool)
	}

	if _, err := userRepository.FindByID(ctx, security.UserID{2}); !errors.Is(err, ErrNilPool) {
		t.Fatalf("UserRepository.FindByID(nil receiver) = %v, want %v", err, ErrNilPool)
	}
}

func TestBindUUIDArgsRejectsMissingArguments(t *testing.T) {
	if err := bindUUIDArgs(nil, [16]byte{1}); err == nil {
		t.Fatal("bindUUIDArgs(nil) = nil, want an error")
	}

	if err := bindUUIDArgsAt([]any{"value"}, 1, [16]byte{1}); err == nil {
		t.Fatal("bindUUIDArgsAt(out of range) = nil, want an error")
	}
}
