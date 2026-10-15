package pgxgoqu

import (
	"context"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/radynsade/faryengo/internal/accounts"
	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Errors
//

type errAccountWriteFailed struct {
	account *accounts.Account
	err     error
}

func (e errAccountWriteFailed) Account() *accounts.Account {
	return e.account
}

func (e errAccountWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of accounts.ErrAccountCreateFailed

type errAccountCreateFailed struct {
	errAccountWriteFailed
}

func newErrAccountCreateFailed(account *accounts.Account, err error) *errAccountCreateFailed {
	return &errAccountCreateFailed{
		errAccountWriteFailed: errAccountWriteFailed{account, err},
	}
}

func (e *errAccountCreateFailed) Error() string {
	return "failed to create an account"
}

// Implementation of accounts.ErrAccountUpdateFailed

type errAccountUpdateFailed struct {
	errAccountWriteFailed
}

func newErrAccountUpdateFailed(account *accounts.Account, err error) *errAccountUpdateFailed {
	return &errAccountUpdateFailed{
		errAccountWriteFailed: errAccountWriteFailed{account, err},
	}
}

func (e *errAccountUpdateFailed) Error() string {
	return "failed to update an account"
}

// Implementation of accounts.ErrAccountDeleteFailed

type errAccountDeleteFailed struct {
	id  accounts.ID
	err error
}

func newErrAccountDeleteFailed(id accounts.ID, err error) *errAccountDeleteFailed {
	return &errAccountDeleteFailed{id, err}
}

func (e *errAccountDeleteFailed) AccountID() accounts.ID {
	return e.id
}

func (e *errAccountDeleteFailed) Unwrap() error {
	return e.err
}

func (e *errAccountDeleteFailed) Error() string {
	return "failed to delete an account"
}

//
// Repository
//

type AccountRepository struct {
	pool pgxdb.DB
}

var _ accounts.AccountRepository = (*AccountRepository)(nil)

func NewAccountRepository(pool pgxdb.DB) (*AccountRepository, error) {
	var (
		repository *AccountRepository
		err        error
	)

	if pgxdb.IsNil(pool) {
		err = pgxdb.ErrNilDB
	} else {
		repository = &AccountRepository{pool: pool}
	}

	return repository, err
}

// The workspace and user foreign keys report a missing workspace or user, and
// the primary key reports an existing account.

func (r *AccountRepository) Create(
	ctx context.Context,
	account *accounts.Account,
) accounts.ErrAccountCreateFailed {
	var err accounts.ErrAccountCreateFailed

	if r == nil || r.pool == nil {
		err = newErrAccountCreateFailed(account, pgxdb.ErrNilDB)
	} else if account == nil {
		err = newErrAccountCreateFailed(account, accounts.ErrAccountNil)
	} else if validationErr := account.Validate(); validationErr != nil {
		err = newErrAccountCreateFailed(account, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Insert("account").
			Rows(goqu.Record{
				"id":           uuid.UUID(account.ID).String(),
				"workspace_id": uuid.UUID(account.WorkspaceID).String(),
				"user_id":      storedUserID(account.UserID),
				"name":         string(account.Name),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrAccountCreateFailed(account, buildErr)
		} else {
			_, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrAccountCreateFailed(account, mapAccountError(execErr))
			}
		}
	}

	return err
}

// The workspace is part of the match rather than the update, so an account is
// never moved to another workspace.

func (r *AccountRepository) Update(
	ctx context.Context,
	account *accounts.Account,
) accounts.ErrAccountUpdateFailed {
	var err accounts.ErrAccountUpdateFailed

	if r == nil || r.pool == nil {
		err = newErrAccountUpdateFailed(account, pgxdb.ErrNilDB)
	} else if account == nil {
		err = newErrAccountUpdateFailed(account, accounts.ErrAccountNil)
	} else if validationErr := account.Validate(); validationErr != nil {
		err = newErrAccountUpdateFailed(account, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Update("account").
			Set(goqu.Record{
				"user_id": storedUserID(account.UserID),
				"name":    string(account.Name),
			}).
			Where(goqu.Ex{
				"id":           uuid.UUID(account.ID).String(),
				"workspace_id": uuid.UUID(account.WorkspaceID).String(),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrAccountUpdateFailed(account, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrAccountUpdateFailed(account, mapAccountError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrAccountUpdateFailed(account, accounts.ErrAccountNotFound)
			}
		}
	}

	return err
}

// Delete

func (r *AccountRepository) Delete(
	ctx context.Context,
	id accounts.ID,
) accounts.ErrAccountDeleteFailed {
	var err accounts.ErrAccountDeleteFailed

	if r == nil || r.pool == nil {
		err = newErrAccountDeleteFailed(id, pgxdb.ErrNilDB)
	} else if validationErr := id.Validate(); validationErr != nil {
		err = newErrAccountDeleteFailed(id, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Delete("account").
			Where(goqu.Ex{"id": uuid.UUID(id).String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrAccountDeleteFailed(id, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrAccountDeleteFailed(id, mapAccountError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrAccountDeleteFailed(id, accounts.ErrAccountNotFound)
			}
		}
	}

	return err
}

// Find by an ID

func (r *AccountRepository) FindByID(
	ctx context.Context,
	id accounts.ID,
) (*accounts.Account, error) {
	var (
		account *accounts.Account
		err     error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find an account by ID: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		account, err = r.find(ctx, goqu.Ex{"id": storedID.String()}, false)

		if err != nil {
			err = fmt.Errorf("failed to find an account %s: %w", storedID, err)
		}
	}

	return account, err
}

// A row lock lasts only until its transaction ends, so locking requires a
// handle that can provide a transaction; a pool or a bare connection is
// rejected before I/O.

func (r *AccountRepository) FindByIDForUpdate(
	ctx context.Context,
	id accounts.ID,
) (*accounts.Account, error) {
	var (
		account *accounts.Account
		err     error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find an account by ID for update: %w", validationErr)
	} else if !pgxdb.IsTransaction(pgxdb.FromContext(ctx, r.pool)) {
		err = pgxdb.ErrTransactionRequired
	} else {
		storedID := uuid.UUID(id)
		account, err = r.find(ctx, goqu.Ex{"id": storedID.String()}, true)

		if err != nil {
			err = fmt.Errorf("failed to find an account %s for update: %w", storedID, err)
		}
	}

	return account, err
}

// Find accounts matching a query; Page is 1-based

func (r *AccountRepository) Find(
	ctx context.Context,
	query accounts.AccountQuery,
) ([]*accounts.Account, error) {
	var (
		result []*accounts.Account
		err    error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := query.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find accounts: %w", validationErr)
	} else {
		sql, args, buildErr := accountFilterDataset(query.Filter).
			Select(accountColumns()...).
			Order(accountOrder(query)...).
			Limit(query.Limit).
			Offset((query.Page - 1) * query.Limit).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the find accounts query: %w", buildErr)
		} else {
			rows, queryErr := pgxdb.FromContext(ctx, r.pool).Query(ctx, sql, args...)

			if queryErr != nil {
				err = fmt.Errorf("failed to find accounts: %w", queryErr)
			} else {
				defer rows.Close()

				result = make([]*accounts.Account, 0, query.Limit)

				for rows.Next() {
					var account *accounts.Account
					account, err = scanAccount(rows)

					if err != nil {
						break
					}

					result = append(result, account)
				}

				if err == nil {
					err = rows.Err()
				}

				if err != nil {
					err = fmt.Errorf("failed to read accounts: %w", err)
					result = nil
				}
			}
		}
	}

	return result, err
}

// Count accounts matching a filter

func (r *AccountRepository) Count(
	ctx context.Context,
	filter accounts.AccountFilter,
) (int, error) {
	var (
		total int
		err   error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := filter.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to count accounts: %w", validationErr)
	} else {
		query, args, buildErr := accountFilterDataset(filter).
			Select(goqu.COUNT("*")).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the count accounts query: %w", buildErr)
		} else if scanErr := pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&total); scanErr != nil {
			err = fmt.Errorf("failed to count accounts: %w", scanErr)
		}
	}

	return total, err
}

//
// Helpers
//

func (r *AccountRepository) find(
	ctx context.Context,
	filter goqu.Ex,
	forUpdate bool,
) (*accounts.Account, error) {
	var (
		account *accounts.Account
		err     error
	)

	dataset := goqu.Dialect("postgres").
		From("account").
		Select(accountColumns()...).
		Where(filter).
		Prepared(true)

	if forUpdate {
		dataset = dataset.ForUpdate(exp.Wait)
	}

	query, args, buildErr := dataset.ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the account lookup query: %w", buildErr)
	} else {
		account, err = scanAccount(pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...))
	}

	return account, err
}

func storedUserID(userID *users.UserID) *string {
	var stored *string

	if userID != nil {
		value := uuid.UUID(*userID).String()
		stored = &value
	}

	return stored
}

func accountColumns() []any {
	return []any{
		"id",
		"workspace_id",
		"user_id",
		"name",
	}
}

func accountFilterDataset(filter accounts.AccountFilter) *goqu.SelectDataset {
	dataset := goqu.Dialect("postgres").From("account")

	if filter.IDLike != "" {
		dataset = dataset.Where(goqu.L(`"account".id::text ILIKE ?`, pgxdb.LikeSubstring(filter.IDLike)))
	}

	if filter.NameLike != "" {
		dataset = dataset.Where(goqu.L(`"account".name ILIKE ?`, pgxdb.LikeSubstring(filter.NameLike)))
	}

	if filter.WorkspaceID != nil {
		dataset = dataset.Where(goqu.Ex{"workspace_id": uuid.UUID(*filter.WorkspaceID).String()})
	}

	if filter.UserID != nil {
		dataset = dataset.Where(goqu.Ex{"user_id": uuid.UUID(*filter.UserID).String()})
	}

	return dataset
}

// Name sorts ignore letter case. The ID is the tie-breaker for every sort.

func accountOrder(query accounts.AccountQuery) []exp.OrderedExpression {
	var sort exp.Orderable = goqu.C("id")

	if query.SortBy == accounts.AccountSortName {
		sort = goqu.L(`lower("account".name)`)
	}

	order := []exp.OrderedExpression{sort.Asc()}

	if query.SortOrder.IsDesc() {
		order[0] = sort.Desc()
	}

	if query.SortBy != accounts.AccountSortID {
		order = append(order, goqu.C("id").Asc())
	}

	return order
}

func scanAccount(row pgx.Row) (*accounts.Account, error) {
	var (
		account     *accounts.Account
		err         error
		id          pgtype.UUID
		workspaceID pgtype.UUID
		userID      pgtype.UUID
		name        string
	)

	scanErr := row.Scan(&id, &workspaceID, &userID, &name)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = accounts.ErrAccountNotFound
	} else if scanErr != nil {
		err = fmt.Errorf("scan account: %w", scanErr)
	} else {
		var (
			accountID          accounts.ID
			accountWorkspaceID workspaces.ID
			accountUserID      *users.UserID
			accountName        accounts.Name
		)

		accountID, err = accounts.NewID(uuid.UUID(id.Bytes))

		if err == nil {
			accountWorkspaceID, err = workspaces.NewID(uuid.UUID(workspaceID.Bytes))
		}

		if err == nil && userID.Valid {
			var value users.UserID

			value, err = users.NewUserID(uuid.UUID(userID.Bytes))
			accountUserID = &value
		}

		if err == nil {
			accountName, err = accounts.NewName(name)
		}

		if err == nil {
			account = accounts.NewAccount(
				accountID,
				accountWorkspaceID,
				accountUserID,
				accountName,
			)
		} else {
			err = fmt.Errorf("failed to decode an account: %w: %w", accounts.ErrAccountInvalid, err)
		}
	}

	return account, err
}

func mapAccountError(err error) error {
	result := err

	if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case postgresErr.Code == "23505" && postgresErr.ConstraintName == "account_pkey":
			result = errors.Join(accounts.ErrAccountAlreadyExists, err)
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "account_workspace_id_fkey":
			result = errors.Join(workspaces.ErrWorkspaceNotFound, err)
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "account_user_id_fkey":
			result = errors.Join(users.ErrUserNotFound, err)
		}
	}

	return result
}
