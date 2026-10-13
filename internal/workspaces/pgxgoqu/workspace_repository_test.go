package pgxgoqu

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
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

func TestNewWorkspaceRepositoryRejectsNilDB(t *testing.T) {
	tests := []struct {
		name string
		pool pgxdb.DB
	}{
		{"nil", nil},
		{"typed nil pool", (*pgxpool.Pool)(nil)},
		{"typed nil connection", (*pgx.Conn)(nil)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, err := NewWorkspaceRepository(test.pool)

			if !errors.Is(err, pgxdb.ErrNilDB) || repository != nil {
				t.Fatalf("got (%v, %v), want (nil, %v)", repository, err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestWorkspaceMethodsRejectMissingPool(t *testing.T) {
	ctx := context.Background()
	id := workspaces.WorkspaceID(uuid.Must(uuid.NewV7()))
	userID := users.UserID(uuid.Must(uuid.NewV7()))
	membership := workspaces.NewMembership(id, userID, workspaces.RoleViewer)

	tests := []struct {
		name string
		call func() error
	}{
		{"create", func() error { return (&WorkspaceRepository{}).Create(ctx, &workspaces.Workspace{}) }},
		{"update", func() error { return (&WorkspaceRepository{}).Update(ctx, &workspaces.Workspace{}) }},
		{"delete", func() error { return (*WorkspaceRepository)(nil).Delete(ctx, id) }},
		{"find by ID", func() error { _, err := (*WorkspaceRepository)(nil).FindByID(ctx, id); return err }},
		{"find by ID for update", func() error { _, err := (&WorkspaceRepository{}).FindByIDForUpdate(ctx, id); return err }},
		{"find", func() error { _, err := (&WorkspaceRepository{}).Find(ctx, workspaces.WorkspaceQuery{}); return err }},
		{"count", func() error { _, err := (&WorkspaceRepository{}).Count(ctx, workspaces.WorkspaceFilter{}); return err }},
		{"add member", func() error { return (&WorkspaceRepository{}).AddMember(ctx, membership) }},
		{"update member", func() error { return (&WorkspaceRepository{}).UpdateMember(ctx, membership) }},
		{"remove member", func() error { return (&WorkspaceRepository{}).RemoveMember(ctx, membership) }},
		{"find member", func() error { _, err := (&WorkspaceRepository{}).FindMember(ctx, id, userID); return err }},
		{"find members", func() error { _, err := (*WorkspaceRepository)(nil).FindMembers(ctx, id); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, pgxdb.ErrNilDB) {
				t.Fatalf("got %v, want %v", err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestWorkspaceMethodsValidateBeforeIO(t *testing.T) {
	ctx := context.Background()
	repository := &WorkspaceRepository{pool: newLazyPool(t)}
	validID := workspaces.WorkspaceID(uuid.Must(uuid.NewV7()))
	validUserID := users.UserID(uuid.Must(uuid.NewV7()))
	untrimmed := workspaces.NewWorkspace(validID, " Design")

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"create nil", func() error { return repository.Create(ctx, nil) }, workspaces.ErrWorkspaceNil},
		{"create invalid", func() error { return repository.Create(ctx, &workspaces.Workspace{}) }, workspaces.ErrWorkspaceInvalid},
		{"create untrimmed name", func() error { return repository.Create(ctx, untrimmed) }, workspaces.ErrWorkspaceNameUntrimmed},
		{"update nil", func() error { return repository.Update(ctx, nil) }, workspaces.ErrWorkspaceNil},
		{"update invalid", func() error { return repository.Update(ctx, &workspaces.Workspace{}) }, workspaces.ErrWorkspaceInvalid},
		{"delete invalid ID", func() error { return repository.Delete(ctx, workspaces.WorkspaceID{}) }, workspaces.ErrWorkspaceIDInvalid},
		{"find by invalid ID", func() error {
			_, err := repository.FindByID(ctx, workspaces.WorkspaceID{})
			return err
		}, workspaces.ErrWorkspaceIDInvalid},
		{"find by invalid ID for update", func() error {
			_, err := repository.FindByIDForUpdate(ctx, workspaces.WorkspaceID{})
			return err
		}, workspaces.ErrWorkspaceIDInvalid},
		{"find by ID for update outside a transaction", func() error {
			_, err := repository.FindByIDForUpdate(ctx, validID)
			return err
		}, pgxdb.ErrTransactionRequired},
		{"find without sort", func() error {
			_, err := repository.Find(ctx, workspaces.WorkspaceQuery{Limit: 10, Page: 1})
			return err
		}, workspaces.ErrWorkspaceQueryInvalid},
		{"find with zero page", func() error {
			_, err := repository.Find(ctx, workspaces.WorkspaceQuery{SortBy: workspaces.WorkspaceSortID, Limit: 10})
			return err
		}, workspaces.ErrWorkspaceQueryInvalid},
		{"find with invalid member", func() error {
			_, err := repository.Find(ctx, workspaces.WorkspaceQuery{
				Filter: workspaces.WorkspaceFilter{MemberID: &users.UserID{}},
				SortBy: workspaces.WorkspaceSortID,
				Limit:  10,
				Page:   1,
			})
			return err
		}, users.ErrUserIDInvalid},
		{"count with invalid filter", func() error {
			_, err := repository.Count(ctx, workspaces.WorkspaceFilter{NameLike: strings.Repeat("a", workspaces.MaxWorkspaceFilterNameLikeLength+1)})
			return err
		}, workspaces.ErrWorkspaceFilterInvalid},
		{"add member without a user", func() error {
			return repository.AddMember(ctx, workspaces.NewMembership(validID, users.UserID{}, workspaces.RoleViewer))
		}, users.ErrUserIDInvalid},
		{"add member without a workspace", func() error {
			return repository.AddMember(ctx, workspaces.NewMembership(workspaces.WorkspaceID{}, validUserID, workspaces.RoleViewer))
		}, workspaces.ErrMembershipInvalid},
		{"add member without a role", func() error {
			return repository.AddMember(ctx, workspaces.NewMembership(validID, validUserID, ""))
		}, workspaces.ErrRoleInvalid},
		{"update member with an unknown role", func() error {
			return repository.UpdateMember(ctx, workspaces.NewMembership(validID, validUserID, "admin"))
		}, workspaces.ErrRoleInvalid},
		{"update member without a user", func() error {
			return repository.UpdateMember(ctx, workspaces.NewMembership(validID, users.UserID{}, workspaces.RoleOwner))
		}, workspaces.ErrMembershipInvalid},
		{"find members of an invalid workspace", func() error {
			_, err := repository.FindMembers(ctx, workspaces.WorkspaceID{})
			return err
		}, workspaces.ErrWorkspaceIDInvalid},
		{"find member without a user", func() error {
			_, err := repository.FindMember(ctx, validID, users.UserID{})
			return err
		}, users.ErrUserIDInvalid},
		{"find member of an invalid workspace", func() error {
			_, err := repository.FindMember(ctx, workspaces.WorkspaceID{}, validUserID)
			return err
		}, workspaces.ErrWorkspaceIDInvalid},
		{"remove member without a user", func() error {
			return repository.RemoveMember(ctx, workspaces.NewMembership(validID, users.UserID{}, workspaces.RoleViewer))
		}, workspaces.ErrMembershipInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestWorkspaceWriteFailuresExposeInput(t *testing.T) {
	ctx := context.Background()
	workspace := &workspaces.Workspace{}
	id := workspaces.WorkspaceID{}
	membership := workspaces.Membership{}
	repository := &WorkspaceRepository{}

	if err := repository.Create(ctx, workspace); err == nil || err.Workspace() != workspace {
		t.Fatalf("Create() = %v, want a failure exposing the workspace", err)
	}

	if err := repository.Update(ctx, workspace); err == nil || err.Workspace() != workspace {
		t.Fatalf("Update() = %v, want a failure exposing the workspace", err)
	}

	if err := repository.Delete(ctx, id); err == nil || err.WorkspaceID() != id {
		t.Fatalf("Delete() = %v, want a failure exposing the ID", err)
	}

	if err := repository.AddMember(ctx, membership); err == nil || err.Membership() != membership {
		t.Fatalf("AddMember() = %v, want a failure exposing the membership", err)
	}

	if err := repository.UpdateMember(ctx, membership); err == nil || err.Membership() != membership {
		t.Fatalf("UpdateMember() = %v, want a failure exposing the membership", err)
	}

	if err := repository.RemoveMember(ctx, membership); err == nil || err.Membership() != membership {
		t.Fatalf("RemoveMember() = %v, want a failure exposing the membership", err)
	}
}

func TestWorkspaceFindQuery(t *testing.T) {
	memberID := users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000001"))

	tests := []struct {
		name     string
		query    workspaces.WorkspaceQuery
		wantSQL  []string
		wantArgs []any
	}{
		{
			name:     "sort by ID descending without a tie-breaker",
			query:    workspaces.WorkspaceQuery{SortBy: workspaces.WorkspaceSortID, SortOrder: domquery.SortOrderDesc, Limit: 10, Page: 3},
			wantSQL:  []string{`ORDER BY "id" DESC LIMIT $1 OFFSET $2`},
			wantArgs: []any{int64(10), int64(20)},
		},
		{
			name:     "sort by name with the ID tie-breaker",
			query:    workspaces.WorkspaceQuery{SortBy: workspaces.WorkspaceSortName, SortOrder: domquery.SortOrderAsc, Limit: 5, Page: 1},
			wantSQL:  []string{`ORDER BY lower("workspace".name) ASC, "id" ASC LIMIT $1`},
			wantArgs: []any{int64(5)},
		},
		{
			name: "every filter",
			query: workspaces.WorkspaceQuery{
				Filter: workspaces.WorkspaceFilter{
					IDLike:   "01_",
					NameLike: "50%",
					MemberID: &memberID,
				},
				SortBy: workspaces.WorkspaceSortName,
				Limit:  1,
				Page:   1,
			},
			wantSQL: []string{
				`"workspace".id::text ILIKE $1`,
				`"workspace".name ILIKE $2`,
				`EXISTS (SELECT 1 FROM "workspace_membership" WHERE workspace_id = "workspace".id AND user_id = $3)`,
			},
			wantArgs: []any{`%01\_%`, `%50\%%`, "01920000-0000-7000-8000-000000000001", int64(1)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql, args, err := workspaceFilterDataset(test.query.Filter).
				Select(workspaceColumns()...).
				Order(workspaceOrder(test.query)...).
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

func TestMapWorkspaceErrors(t *testing.T) {
	cause := errors.New("connection reset")

	tests := []struct {
		name    string
		mapper  func(error) error
		err     error
		want    error
		wantRaw bool
	}{
		{"workspace ID taken", mapWorkspaceError, &pgconn.PgError{Code: "23505", ConstraintName: "workspace_pkey"}, workspaces.ErrWorkspaceAlreadyExists, true},
		{"workspace unrelated", mapWorkspaceError, cause, cause, false},
		{"membership exists", mapMembershipError, &pgconn.PgError{Code: "23505", ConstraintName: "workspace_membership_pkey"}, workspaces.ErrMembershipAlreadyExists, true},
		{"membership workspace missing", mapMembershipError, &pgconn.PgError{Code: "23503", ConstraintName: "workspace_membership_workspace_id_fkey"}, workspaces.ErrWorkspaceNotFound, true},
		{"membership user missing", mapMembershipError, &pgconn.PgError{Code: "23503", ConstraintName: "workspace_membership_user_id_fkey"}, users.ErrUserNotFound, true},
		{"membership unrelated", mapMembershipError, cause, cause, false},
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
