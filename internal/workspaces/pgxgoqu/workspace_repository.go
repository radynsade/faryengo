package pgxgoqu

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Errors
//

type errWorkspaceWriteFailed struct {
	workspace *workspaces.Workspace
	err       error
}

func (e errWorkspaceWriteFailed) Workspace() *workspaces.Workspace {
	return e.workspace
}

func (e errWorkspaceWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of workspaces.ErrWorkspaceCreateFailed

type errWorkspaceCreateFailed struct {
	errWorkspaceWriteFailed
}

func newErrWorkspaceCreateFailed(workspace *workspaces.Workspace, err error) *errWorkspaceCreateFailed {
	return &errWorkspaceCreateFailed{
		errWorkspaceWriteFailed: errWorkspaceWriteFailed{workspace, err},
	}
}

func (e *errWorkspaceCreateFailed) Error() string {
	return "failed to create a workspace"
}

// Implementation of workspaces.ErrWorkspaceUpdateFailed

type errWorkspaceUpdateFailed struct {
	errWorkspaceWriteFailed
}

func newErrWorkspaceUpdateFailed(workspace *workspaces.Workspace, err error) *errWorkspaceUpdateFailed {
	return &errWorkspaceUpdateFailed{
		errWorkspaceWriteFailed: errWorkspaceWriteFailed{workspace, err},
	}
}

func (e *errWorkspaceUpdateFailed) Error() string {
	return "failed to update a workspace"
}

// Implementation of workspaces.ErrWorkspaceDeleteFailed

type errWorkspaceDeleteFailed struct {
	id  workspaces.WorkspaceID
	err error
}

func newErrWorkspaceDeleteFailed(id workspaces.WorkspaceID, err error) *errWorkspaceDeleteFailed {
	return &errWorkspaceDeleteFailed{id, err}
}

func (e errWorkspaceDeleteFailed) WorkspaceID() workspaces.WorkspaceID {
	return e.id
}

func (e errWorkspaceDeleteFailed) Unwrap() error {
	return e.err
}

func (e *errWorkspaceDeleteFailed) Error() string {
	return "failed to delete a workspace"
}

type errMembershipWriteFailed struct {
	membership workspaces.Membership
	err        error
}

func (e errMembershipWriteFailed) Membership() workspaces.Membership {
	return e.membership
}

func (e errMembershipWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of workspaces.ErrMembershipAddFailed

type errMembershipAddFailed struct {
	errMembershipWriteFailed
}

func newErrMembershipAddFailed(membership workspaces.Membership, err error) *errMembershipAddFailed {
	return &errMembershipAddFailed{
		errMembershipWriteFailed: errMembershipWriteFailed{membership, err},
	}
}

func (e *errMembershipAddFailed) Error() string {
	return "failed to add a workspace member"
}

// Implementation of workspaces.ErrMembershipRemoveFailed

type errMembershipRemoveFailed struct {
	errMembershipWriteFailed
}

func newErrMembershipRemoveFailed(membership workspaces.Membership, err error) *errMembershipRemoveFailed {
	return &errMembershipRemoveFailed{
		errMembershipWriteFailed: errMembershipWriteFailed{membership, err},
	}
}

func (e *errMembershipRemoveFailed) Error() string {
	return "failed to remove a workspace member"
}

//
// Repository
//

type WorkspaceRepository struct {
	pool pgxdb.DB
}

var _ workspaces.WorkspaceRepository = (*WorkspaceRepository)(nil)

func NewWorkspaceRepository(pool pgxdb.DB) (*WorkspaceRepository, error) {
	var (
		repository *WorkspaceRepository
		err        error
	)

	if pgxdb.IsNil(pool) {
		err = pgxdb.ErrNilDB
	} else {
		repository = &WorkspaceRepository{pool: pool}
	}

	return repository, err
}

// Create

func (r *WorkspaceRepository) Create(
	ctx context.Context,
	workspace *workspaces.Workspace,
) workspaces.ErrWorkspaceCreateFailed {
	var err workspaces.ErrWorkspaceCreateFailed

	if r == nil || r.pool == nil {
		err = newErrWorkspaceCreateFailed(workspace, pgxdb.ErrNilDB)
	} else if workspace == nil {
		err = newErrWorkspaceCreateFailed(workspace, workspaces.ErrWorkspaceNil)
	} else if validationErr := workspace.Validate(); validationErr != nil {
		err = newErrWorkspaceCreateFailed(workspace, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Insert("workspace").
			Rows(goqu.Record{
				"id":   uuid.UUID(workspace.ID).String(),
				"name": string(workspace.Name),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrWorkspaceCreateFailed(workspace, buildErr)
		} else {
			_, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrWorkspaceCreateFailed(workspace, mapWorkspaceError(execErr))
			}
		}
	}

	return err
}

// Update

func (r *WorkspaceRepository) Update(
	ctx context.Context,
	workspace *workspaces.Workspace,
) workspaces.ErrWorkspaceUpdateFailed {
	var err workspaces.ErrWorkspaceUpdateFailed

	if r == nil || r.pool == nil {
		err = newErrWorkspaceUpdateFailed(workspace, pgxdb.ErrNilDB)
	} else if workspace == nil {
		err = newErrWorkspaceUpdateFailed(workspace, workspaces.ErrWorkspaceNil)
	} else if validationErr := workspace.Validate(); validationErr != nil {
		err = newErrWorkspaceUpdateFailed(workspace, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Update("workspace").
			Set(goqu.Record{"name": string(workspace.Name)}).
			Where(goqu.Ex{"id": uuid.UUID(workspace.ID).String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrWorkspaceUpdateFailed(workspace, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrWorkspaceUpdateFailed(workspace, mapWorkspaceError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrWorkspaceUpdateFailed(workspace, workspaces.ErrWorkspaceNotFound)
			}
		}
	}

	return err
}

// The membership foreign key cascades, so deleting a workspace removes its
// memberships in the same statement.

func (r *WorkspaceRepository) Delete(
	ctx context.Context,
	id workspaces.WorkspaceID,
) workspaces.ErrWorkspaceDeleteFailed {
	var err workspaces.ErrWorkspaceDeleteFailed

	if r == nil || r.pool == nil {
		err = newErrWorkspaceDeleteFailed(id, pgxdb.ErrNilDB)
	} else if validationErr := id.Validate(); validationErr != nil {
		err = newErrWorkspaceDeleteFailed(id, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Delete("workspace").
			Where(goqu.Ex{"id": uuid.UUID(id).String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrWorkspaceDeleteFailed(id, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrWorkspaceDeleteFailed(id, mapWorkspaceError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrWorkspaceDeleteFailed(id, workspaces.ErrWorkspaceNotFound)
			}
		}
	}

	return err
}

// Find by an ID

func (r *WorkspaceRepository) FindByID(
	ctx context.Context,
	id workspaces.WorkspaceID,
) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		err       error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a workspace by ID: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		workspace, err = r.find(ctx, goqu.Ex{"id": storedID.String()}, false)

		if err != nil {
			err = fmt.Errorf("failed to find a workspace %s: %w", storedID, err)
		}
	}

	return workspace, err
}

// A row lock lasts only until its transaction ends, so locking requires a
// handle that can provide a transaction; a pool or a bare connection is
// rejected before I/O.

func (r *WorkspaceRepository) FindByIDForUpdate(
	ctx context.Context,
	id workspaces.WorkspaceID,
) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		err       error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a workspace by ID for update: %w", validationErr)
	} else if !pgxdb.IsTransaction(pgxdb.FromContext(ctx, r.pool)) {
		err = pgxdb.ErrTransactionRequired
	} else {
		storedID := uuid.UUID(id)
		workspace, err = r.find(ctx, goqu.Ex{"id": storedID.String()}, true)

		if err != nil {
			err = fmt.Errorf("failed to find a workspace %s for update: %w", storedID, err)
		}
	}

	return workspace, err
}

// Find workspaces matching a query; Page is 1-based

func (r *WorkspaceRepository) Find(
	ctx context.Context,
	query workspaces.WorkspaceQuery,
) ([]*workspaces.Workspace, error) {
	var (
		result []*workspaces.Workspace
		err    error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := query.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find workspaces: %w", validationErr)
	} else {
		sql, args, buildErr := workspaceFilterDataset(query.Filter).
			Select(workspaceColumns()...).
			Order(workspaceOrder(query)...).
			Limit(query.Limit).
			Offset((query.Page - 1) * query.Limit).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the find workspaces query: %w", buildErr)
		} else {
			rows, queryErr := pgxdb.FromContext(ctx, r.pool).Query(ctx, sql, args...)

			if queryErr != nil {
				err = fmt.Errorf("failed to find workspaces: %w", queryErr)
			} else {
				defer rows.Close()

				result = make([]*workspaces.Workspace, 0, query.Limit)

				for rows.Next() {
					var workspace *workspaces.Workspace
					workspace, err = scanWorkspace(rows)

					if err != nil {
						break
					}

					result = append(result, workspace)
				}

				if err == nil {
					err = rows.Err()
				}

				if err != nil {
					err = fmt.Errorf("failed to read workspaces: %w", err)
					result = nil
				}
			}
		}
	}

	return result, err
}

// Count workspaces matching a filter

func (r *WorkspaceRepository) Count(
	ctx context.Context,
	filter workspaces.WorkspaceFilter,
) (int, error) {
	var (
		total int
		err   error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := filter.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to count workspaces: %w", validationErr)
	} else {
		query, args, buildErr := workspaceFilterDataset(filter).
			Select(goqu.COUNT("*")).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the count workspaces query: %w", buildErr)
		} else if scanErr := pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&total); scanErr != nil {
			err = fmt.Errorf("failed to count workspaces: %w", scanErr)
		}
	}

	return total, err
}

// The membership foreign keys report a missing workspace or user, and the
// primary key reports an existing membership.

func (r *WorkspaceRepository) AddMember(
	ctx context.Context,
	membership workspaces.Membership,
) workspaces.ErrMembershipAddFailed {
	var err workspaces.ErrMembershipAddFailed

	if r == nil || r.pool == nil {
		err = newErrMembershipAddFailed(membership, pgxdb.ErrNilDB)
	} else if validationErr := membership.Validate(); validationErr != nil {
		err = newErrMembershipAddFailed(membership, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Insert("workspace_membership").
			Rows(goqu.Record{
				"workspace_id": uuid.UUID(membership.WorkspaceID).String(),
				"user_id":      uuid.UUID(membership.UserID).String(),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrMembershipAddFailed(membership, buildErr)
		} else {
			_, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrMembershipAddFailed(membership, mapMembershipError(execErr))
			}
		}
	}

	return err
}

// Remove a member

func (r *WorkspaceRepository) RemoveMember(
	ctx context.Context,
	membership workspaces.Membership,
) workspaces.ErrMembershipRemoveFailed {
	var err workspaces.ErrMembershipRemoveFailed

	if r == nil || r.pool == nil {
		err = newErrMembershipRemoveFailed(membership, pgxdb.ErrNilDB)
	} else if validationErr := membership.Validate(); validationErr != nil {
		err = newErrMembershipRemoveFailed(membership, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Delete("workspace_membership").
			Where(goqu.Ex{
				"workspace_id": uuid.UUID(membership.WorkspaceID).String(),
				"user_id":      uuid.UUID(membership.UserID).String(),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrMembershipRemoveFailed(membership, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrMembershipRemoveFailed(membership, execErr)
			} else if tag.RowsAffected() == 0 {
				err = newErrMembershipRemoveFailed(membership, workspaces.ErrMembershipNotFound)
			}
		}
	}

	return err
}

// Report whether a user is a member of a workspace

func (r *WorkspaceRepository) HasMember(
	ctx context.Context,
	membership workspaces.Membership,
) (bool, error) {
	var (
		count int
		err   error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := membership.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to check a workspace member: %w", validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			From("workspace_membership").
			Select(goqu.COUNT("*")).
			Where(goqu.Ex{
				"workspace_id": uuid.UUID(membership.WorkspaceID).String(),
				"user_id":      uuid.UUID(membership.UserID).String(),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the workspace member query: %w", buildErr)
		} else if scanErr := pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&count); scanErr != nil {
			err = fmt.Errorf("failed to check a workspace member: %w", scanErr)
		}
	}

	return count > 0, err
}

// Find the IDs of a workspace's members

func (r *WorkspaceRepository) FindMemberIDs(
	ctx context.Context,
	id workspaces.WorkspaceID,
) ([]users.UserID, error) {
	var (
		result []users.UserID
		err    error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find workspace members: %w", validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			From("workspace_membership").
			Select("user_id").
			Where(goqu.Ex{"workspace_id": uuid.UUID(id).String()}).
			Order(goqu.C("user_id").Asc()).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the workspace members query: %w", buildErr)
		} else {
			result, err = r.scanMemberIDs(ctx, query, args)

			if err != nil {
				err = fmt.Errorf("failed to find members of workspace %s: %w", uuid.UUID(id), err)
			}
		}
	}

	return result, err
}

//
// Helpers
//

func (r *WorkspaceRepository) find(
	ctx context.Context,
	filter goqu.Ex,
	forUpdate bool,
) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		err       error
	)

	dataset := goqu.Dialect("postgres").
		From("workspace").
		Select(workspaceColumns()...).
		Where(filter).
		Prepared(true)

	if forUpdate {
		dataset = dataset.ForUpdate(exp.Wait)
	}

	query, args, buildErr := dataset.ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the workspace lookup query: %w", buildErr)
	} else {
		workspace, err = scanWorkspace(pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...))
	}

	return workspace, err
}

func (r *WorkspaceRepository) scanMemberIDs(
	ctx context.Context,
	query string,
	args []any,
) ([]users.UserID, error) {
	result := []users.UserID{}

	rows, err := pgxdb.FromContext(ctx, r.pool).Query(ctx, query, args...)

	if err == nil {
		defer rows.Close()

		for rows.Next() {
			var (
				id       pgtype.UUID
				memberID users.UserID
			)

			err = rows.Scan(&id)

			if err == nil {
				memberID, err = users.NewUserID(uuid.UUID(id.Bytes))
			}

			if err != nil {
				break
			}

			result = append(result, memberID)
		}

		if err == nil {
			err = rows.Err()
		}
	}

	if err != nil {
		result = nil
	}

	return result, err
}

func workspaceColumns() []any {
	return []any{
		"id",
		"name",
	}
}

func workspaceFilterDataset(filter workspaces.WorkspaceFilter) *goqu.SelectDataset {
	dataset := goqu.Dialect("postgres").From("workspace")

	if filter.IDLike != "" {
		dataset = dataset.Where(goqu.L(`"workspace".id::text ILIKE ?`, likeSubstring(filter.IDLike)))
	}

	if filter.NameLike != "" {
		dataset = dataset.Where(goqu.L(`"workspace".name ILIKE ?`, likeSubstring(filter.NameLike)))
	}

	if filter.MemberID != nil {
		dataset = dataset.Where(goqu.L(
			`EXISTS (SELECT 1 FROM "workspace_membership" WHERE workspace_id = "workspace".id AND user_id = ?)`,
			uuid.UUID(*filter.MemberID).String(),
		))
	}

	return dataset
}

// Name sorts ignore letter case. The ID is the tie-breaker for every sort.

func workspaceOrder(query workspaces.WorkspaceQuery) []exp.OrderedExpression {
	var sort exp.Orderable = goqu.C("id")

	if query.SortBy == workspaces.WorkspaceSortName {
		sort = goqu.L(`lower("workspace".name)`)
	}

	order := []exp.OrderedExpression{sort.Asc()}

	if query.SortOrder.IsDesc() {
		order[0] = sort.Desc()
	}

	if query.SortBy != workspaces.WorkspaceSortID {
		order = append(order, goqu.C("id").Asc())
	}

	return order
}

func likeSubstring(value string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value) + "%"
}

func scanWorkspace(row pgx.Row) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		err       error
		id        pgtype.UUID
		name      string
	)

	scanErr := row.Scan(&id, &name)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = workspaces.ErrWorkspaceNotFound
	} else if scanErr != nil {
		err = fmt.Errorf("scan workspace: %w", scanErr)
	} else {
		var (
			workspaceID   workspaces.WorkspaceID
			workspaceName workspaces.WorkspaceName
		)

		workspaceID, err = workspaces.NewWorkspaceID(uuid.UUID(id.Bytes))

		if err == nil {
			workspaceName, err = workspaces.NewWorkspaceName(name)
		}

		if err == nil {
			workspace = workspaces.NewWorkspace(workspaceID, workspaceName)
		} else {
			err = fmt.Errorf("failed to decode a workspace: %w: %w", workspaces.ErrWorkspaceInvalid, err)
		}
	}

	return workspace, err
}

func mapWorkspaceError(err error) error {
	result := err

	if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		if postgresErr.Code == "23505" && postgresErr.ConstraintName == "workspace_pkey" {
			result = errors.Join(workspaces.ErrWorkspaceAlreadyExists, err)
		}
	}

	return result
}

func mapMembershipError(err error) error {
	result := err

	if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case postgresErr.Code == "23505" && postgresErr.ConstraintName == "workspace_membership_pkey":
			result = errors.Join(workspaces.ErrMembershipAlreadyExists, err)
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "workspace_membership_workspace_id_fkey":
			result = errors.Join(workspaces.ErrWorkspaceNotFound, err)
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "workspace_membership_user_id_fkey":
			result = errors.Join(users.ErrUserNotFound, err)
		}
	}

	return result
}
