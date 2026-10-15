package pgxgoqu

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/radynsade/faryengo/internal/budgets"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/period"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Errors
//

type errBudgetWriteFailed struct {
	budget *budgets.Budget
	err    error
}

func (e errBudgetWriteFailed) Budget() *budgets.Budget {
	return e.budget
}

func (e errBudgetWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of budgets.ErrBudgetCreateFailed

type errBudgetCreateFailed struct {
	errBudgetWriteFailed
}

func newErrBudgetCreateFailed(budget *budgets.Budget, err error) *errBudgetCreateFailed {
	return &errBudgetCreateFailed{
		errBudgetWriteFailed: errBudgetWriteFailed{budget, err},
	}
}

func (e *errBudgetCreateFailed) Error() string {
	return "failed to create a budget"
}

// Implementation of budgets.ErrBudgetUpdateFailed

type errBudgetUpdateFailed struct {
	errBudgetWriteFailed
}

func newErrBudgetUpdateFailed(budget *budgets.Budget, err error) *errBudgetUpdateFailed {
	return &errBudgetUpdateFailed{
		errBudgetWriteFailed: errBudgetWriteFailed{budget, err},
	}
}

func (e *errBudgetUpdateFailed) Error() string {
	return "failed to update a budget"
}

// Implementation of budgets.ErrBudgetDeleteFailed

type errBudgetDeleteFailed struct {
	id  budgets.ID
	err error
}

func newErrBudgetDeleteFailed(id budgets.ID, err error) *errBudgetDeleteFailed {
	return &errBudgetDeleteFailed{id, err}
}

func (e *errBudgetDeleteFailed) BudgetID() budgets.ID {
	return e.id
}

func (e *errBudgetDeleteFailed) Unwrap() error {
	return e.err
}

func (e *errBudgetDeleteFailed) Error() string {
	return "failed to delete a budget"
}

//
// Repository
//

type BudgetRepository struct {
	pool pgxdb.DB
}

var _ budgets.BudgetRepository = (*BudgetRepository)(nil)

func NewBudgetRepository(pool pgxdb.DB) (*BudgetRepository, error) {
	var (
		repository *BudgetRepository
		err        error
	)

	if pgxdb.IsNil(pool) {
		err = pgxdb.ErrNilDB
	} else {
		repository = &BudgetRepository{pool: pool}
	}

	return repository, err
}

// The budget and its categories are written in one transaction. The workspace,
// currency, and category foreign keys report a missing workspace, currency, or
// category, and the primary key reports an existing budget.

func (r *BudgetRepository) Create(
	ctx context.Context,
	budget *budgets.Budget,
) budgets.ErrBudgetCreateFailed {
	var err budgets.ErrBudgetCreateFailed

	if r == nil || r.pool == nil {
		err = newErrBudgetCreateFailed(budget, pgxdb.ErrNilDB)
	} else if budget == nil {
		err = newErrBudgetCreateFailed(budget, budgets.ErrBudgetNil)
	} else if validationErr := budget.Validate(); validationErr != nil {
		err = newErrBudgetCreateFailed(budget, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Insert("budget").
			Rows(goqu.Record{
				"id":               uuid.UUID(budget.ID).String(),
				"workspace_id":     uuid.UUID(budget.WorkspaceID).String(),
				"currency_code":    string(budget.Currency),
				"period_from":      storedTime(budget.Period.From),
				"period_to":        storedTime(budget.Period.To),
				"period_repeat":    string(budget.Period.Repeat.Kind),
				"period_every":     storedEvery(budget.Period.Repeat),
				"period_until":     storedTime(budget.Period.Repeat.Until),
				"period_time_zone": budget.Period.Location().String(),
				"amount":           int64(budget.Amount),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrBudgetCreateFailed(budget, buildErr)
		} else {
			writeErr := pgxdb.InTransaction(ctx, r.pool, func(ctx context.Context) error {
				_, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

				if execErr == nil {
					execErr = r.insertCategories(ctx, budget)
				}

				return execErr
			})

			if writeErr != nil {
				err = newErrBudgetCreateFailed(budget, mapBudgetError(writeErr))
			}
		}
	}

	return err
}

// The workspace is part of the match rather than the update, so a budget is
// never moved to another workspace. Its categories are replaced in the same
// transaction.

func (r *BudgetRepository) Update(
	ctx context.Context,
	budget *budgets.Budget,
) budgets.ErrBudgetUpdateFailed {
	var err budgets.ErrBudgetUpdateFailed

	if r == nil || r.pool == nil {
		err = newErrBudgetUpdateFailed(budget, pgxdb.ErrNilDB)
	} else if budget == nil {
		err = newErrBudgetUpdateFailed(budget, budgets.ErrBudgetNil)
	} else if validationErr := budget.Validate(); validationErr != nil {
		err = newErrBudgetUpdateFailed(budget, validationErr)
	} else {
		storedID := uuid.UUID(budget.ID).String()

		updateQuery, updateArgs, updateBuildErr := goqu.Dialect("postgres").
			Update("budget").
			Set(goqu.Record{
				"currency_code":    string(budget.Currency),
				"period_from":      storedTime(budget.Period.From),
				"period_to":        storedTime(budget.Period.To),
				"period_repeat":    string(budget.Period.Repeat.Kind),
				"period_every":     storedEvery(budget.Period.Repeat),
				"period_until":     storedTime(budget.Period.Repeat.Until),
				"period_time_zone": budget.Period.Location().String(),
				"amount":           int64(budget.Amount),
			}).
			Where(goqu.Ex{
				"id":           storedID,
				"workspace_id": uuid.UUID(budget.WorkspaceID).String(),
			}).
			Prepared(true).
			ToSQL()

		deleteQuery, deleteArgs, deleteBuildErr := goqu.Dialect("postgres").
			Delete("budget_category").
			Where(goqu.Ex{"budget_id": storedID}).
			Prepared(true).
			ToSQL()

		if buildErr := errors.Join(updateBuildErr, deleteBuildErr); buildErr != nil {
			err = newErrBudgetUpdateFailed(budget, buildErr)
		} else {
			writeErr := pgxdb.InTransaction(ctx, r.pool, func(ctx context.Context) error {
				db := pgxdb.FromContext(ctx, r.pool)
				tag, execErr := db.Exec(ctx, updateQuery, updateArgs...)

				if execErr == nil && tag.RowsAffected() == 0 {
					execErr = budgets.ErrBudgetNotFound
				}

				if execErr == nil {
					_, execErr = db.Exec(ctx, deleteQuery, deleteArgs...)
				}

				if execErr == nil {
					execErr = r.insertCategories(ctx, budget)
				}

				return execErr
			})

			if writeErr != nil {
				err = newErrBudgetUpdateFailed(budget, mapBudgetError(writeErr))
			}
		}
	}

	return err
}

// The budget's categories are removed with it by the cascading foreign key.

func (r *BudgetRepository) Delete(
	ctx context.Context,
	id budgets.ID,
) budgets.ErrBudgetDeleteFailed {
	var err budgets.ErrBudgetDeleteFailed

	if r == nil || r.pool == nil {
		err = newErrBudgetDeleteFailed(id, pgxdb.ErrNilDB)
	} else if validationErr := id.Validate(); validationErr != nil {
		err = newErrBudgetDeleteFailed(id, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Delete("budget").
			Where(goqu.Ex{"id": uuid.UUID(id).String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrBudgetDeleteFailed(id, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrBudgetDeleteFailed(id, mapBudgetError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrBudgetDeleteFailed(id, budgets.ErrBudgetNotFound)
			}
		}
	}

	return err
}

// Find by an ID

func (r *BudgetRepository) FindByID(
	ctx context.Context,
	id budgets.ID,
) (*budgets.Budget, error) {
	var (
		budget *budgets.Budget
		err    error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a budget by ID: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		budget, err = r.find(ctx, goqu.Ex{"id": storedID.String()}, false)

		if err != nil {
			err = fmt.Errorf("failed to find a budget %s: %w", storedID, err)
		}
	}

	return budget, err
}

// A row lock lasts only until its transaction ends, so locking requires a
// handle that can provide a transaction; a pool or a bare connection is
// rejected before I/O. Only the budget row is locked; writers of its
// categories lock it first.

func (r *BudgetRepository) FindByIDForUpdate(
	ctx context.Context,
	id budgets.ID,
) (*budgets.Budget, error) {
	var (
		budget *budgets.Budget
		err    error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a budget by ID for update: %w", validationErr)
	} else if !pgxdb.IsTransaction(pgxdb.FromContext(ctx, r.pool)) {
		err = pgxdb.ErrTransactionRequired
	} else {
		storedID := uuid.UUID(id)
		budget, err = r.find(ctx, goqu.Ex{"id": storedID.String()}, true)

		if err != nil {
			err = fmt.Errorf("failed to find a budget %s for update: %w", storedID, err)
		}
	}

	return budget, err
}

// Find budgets matching a query; Page is 1-based

func (r *BudgetRepository) Find(
	ctx context.Context,
	query budgets.BudgetQuery,
) ([]*budgets.Budget, error) {
	var (
		result []*budgets.Budget
		err    error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := query.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find budgets: %w", validationErr)
	} else {
		sql, args, buildErr := budgetFilterDataset(query.Filter).
			Select(budgetColumns()...).
			Order(budgetOrder(query)...).
			Limit(query.Limit).
			Offset((query.Page - 1) * query.Limit).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the find budgets query: %w", buildErr)
		} else {
			rows, queryErr := pgxdb.FromContext(ctx, r.pool).Query(ctx, sql, args...)

			if queryErr != nil {
				err = fmt.Errorf("failed to find budgets: %w", queryErr)
			} else {
				defer rows.Close()

				result = make([]*budgets.Budget, 0, query.Limit)

				for rows.Next() {
					var budget *budgets.Budget
					budget, err = scanBudget(rows)

					if err != nil {
						break
					}

					result = append(result, budget)
				}

				if err == nil {
					err = rows.Err()
				}

				if err != nil {
					err = fmt.Errorf("failed to read budgets: %w", err)
					result = nil
				}
			}
		}
	}

	return result, err
}

// Count budgets matching a filter

func (r *BudgetRepository) Count(
	ctx context.Context,
	filter budgets.BudgetFilter,
) (int, error) {
	var (
		total int
		err   error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := filter.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to count budgets: %w", validationErr)
	} else {
		query, args, buildErr := budgetFilterDataset(filter).
			Select(goqu.COUNT("*")).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the count budgets query: %w", buildErr)
		} else if scanErr := pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&total); scanErr != nil {
			err = fmt.Errorf("failed to count budgets: %w", scanErr)
		}
	}

	return total, err
}

//
// Helpers
//

func (r *BudgetRepository) find(
	ctx context.Context,
	filter goqu.Ex,
	forUpdate bool,
) (*budgets.Budget, error) {
	var (
		budget *budgets.Budget
		err    error
	)

	dataset := goqu.Dialect("postgres").
		From("budget").
		Select(budgetColumns()...).
		Where(filter).
		Prepared(true)

	if forUpdate {
		dataset = dataset.ForUpdate(exp.Wait)
	}

	query, args, buildErr := dataset.ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the budget lookup query: %w", buildErr)
	} else {
		budget, err = scanBudget(pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...))
	}

	return budget, err
}

func (r *BudgetRepository) insertCategories(ctx context.Context, budget *budgets.Budget) error {
	query, args, err := budgetCategoriesInsert(budget).ToSQL()

	if err == nil {
		_, err = pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)
	}

	return err
}

func budgetCategoriesInsert(budget *budgets.Budget) *goqu.InsertDataset {
	var (
		storedID          = uuid.UUID(budget.ID).String()
		storedWorkspaceID = uuid.UUID(budget.WorkspaceID).String()
		rows              = make([]any, 0, len(budget.Categories))
	)

	for index, categoryID := range budget.Categories {
		rows = append(rows, goqu.Record{
			"budget_id":    storedID,
			"workspace_id": storedWorkspaceID,
			"category_id":  uuid.UUID(categoryID).String(),
			"position":     index,
		})
	}

	return goqu.Dialect("postgres").
		Insert("budget_category").
		Rows(rows...).
		Prepared(true)
}

func budgetColumns() []any {
	return []any{
		"id",
		"workspace_id",
		goqu.L(`ARRAY(` +
			`SELECT "budget_category".category_id FROM "budget_category" ` +
			`WHERE "budget_category".budget_id = "budget".id ` +
			`ORDER BY "budget_category".position)`),
		"currency_code",
		"period_from",
		"period_to",
		"period_repeat",
		goqu.L(`(extract(epoch FROM "budget".period_every) * 1000000)::bigint`),
		"period_until",
		"period_time_zone",
		"amount",
	}
}

func budgetFilterDataset(filter budgets.BudgetFilter) *goqu.SelectDataset {
	dataset := goqu.Dialect("postgres").From("budget")

	if filter.IDLike != "" {
		dataset = dataset.Where(goqu.L(`"budget".id::text ILIKE ?`, pgxdb.LikeSubstring(filter.IDLike)))
	}

	if filter.WorkspaceID != nil {
		dataset = dataset.Where(goqu.Ex{"workspace_id": uuid.UUID(*filter.WorkspaceID).String()})
	}

	if filter.CategoryID != nil {
		dataset = dataset.Where(goqu.L(
			`EXISTS (SELECT 1 FROM "budget_category" `+
				`WHERE "budget_category".budget_id = "budget".id AND "budget_category".category_id = ?)`,
			uuid.UUID(*filter.CategoryID).String(),
		))
	}

	if filter.Currency != nil {
		dataset = dataset.Where(goqu.Ex{"currency_code": string(*filter.Currency)})
	}

	return dataset
}

// The ID is the tie-breaker for every sort.

func budgetOrder(query budgets.BudgetQuery) []exp.OrderedExpression {
	var sort exp.Orderable = goqu.C("id")

	if query.SortBy == budgets.BudgetSortAmount {
		sort = goqu.C("amount")
	}

	order := []exp.OrderedExpression{sort.Asc()}

	if query.SortOrder.IsDesc() {
		order[0] = sort.Desc()
	}

	if query.SortBy != budgets.BudgetSortID {
		order = append(order, goqu.C("id").Asc())
	}

	return order
}

func scanBudget(row pgx.Row) (*budgets.Budget, error) {
	var (
		budget      *budgets.Budget
		err         error
		id          pgtype.UUID
		workspaceID pgtype.UUID
		categoryIDs []pgtype.UUID
		code        string
		stored      storedPeriod
		amount      int64
	)

	scanErr := row.Scan(
		&id,
		&workspaceID,
		&categoryIDs,
		&code,
		&stored.from,
		&stored.to,
		&stored.repeat,
		&stored.everyMicroseconds,
		&stored.until,
		&stored.timeZone,
		&amount,
	)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = budgets.ErrBudgetNotFound
	} else if scanErr != nil {
		err = fmt.Errorf("scan budget: %w", scanErr)
	} else {
		var (
			budgetID          budgets.ID
			budgetWorkspaceID workspaces.ID
			budgetCategoryIDs budgets.CategoryIDs
			budgetCurrency    currencies.Code
			budgetPeriod      period.Period
			budgetAmount      budgets.Amount
		)

		budgetID, err = budgets.NewID(uuid.UUID(id.Bytes))

		if err == nil {
			budgetWorkspaceID, err = workspaces.NewID(uuid.UUID(workspaceID.Bytes))
		}

		if err == nil {
			budgetCategoryIDs, err = decodeCategoryIDs(categoryIDs)
		}

		if err == nil {
			budgetCurrency, err = currencies.NewCode(code)
		}

		if err == nil {
			budgetPeriod, err = stored.decode()
		}

		if err == nil {
			budgetAmount, err = budgets.NewAmount(amount)
		}

		if err == nil {
			budget = budgets.NewBudget(
				budgetID,
				budgetWorkspaceID,
				budgetCategoryIDs,
				budgetCurrency,
				budgetPeriod,
				budgetAmount,
			)
		} else {
			err = fmt.Errorf("failed to decode a budget: %w: %w", budgets.ErrBudgetInvalid, err)
		}
	}

	return budget, err
}

// PostgreSQL keeps instants without their location, so the period's time zone
// is kept beside them; a period without times keeps UTC. The step is kept as
// an interval with PostgreSQL's precision of a microsecond, the precision of
// its timestamps too.

func storedTime(value *time.Time) any {
	var stored any

	if value != nil {
		stored = *value
	}

	return stored
}

func storedEvery(repeat period.Repeat) any {
	var stored any

	if repeat.Kind == period.RepeatEvery {
		stored = goqu.L("?::double precision * interval '1 microsecond'", repeat.Every.Microseconds())
	}

	return stored
}

type storedPeriod struct {
	from              pgtype.Timestamptz
	to                pgtype.Timestamptz
	repeat            string
	everyMicroseconds *int64
	until             pgtype.Timestamptz
	timeZone          string
}

// The instants are read in the period's time zone, so repeated windows follow
// its calendar again. The period is validated as a whole, since its parts must
// still fit together.

func (s storedPeriod) decode() (period.Period, error) {
	var (
		result   period.Period
		location *time.Location
		kind     period.RepeatKind
		every    time.Duration
	)

	location, err := time.LoadLocation(s.timeZone)

	if err == nil {
		kind, err = period.NewRepeatKind(s.repeat)
	}

	if err == nil {
		if s.everyMicroseconds != nil {
			every = time.Duration(*s.everyMicroseconds) * time.Microsecond
		}

		result = period.NewPeriod(
			decodeTime(s.from, location),
			decodeTime(s.to, location),
			period.NewRepeat(kind, every, decodeTime(s.until, location)),
		)
		err = result.Validate()
	}

	if err != nil {
		result = period.Period{}
	}

	return result, err
}

func decodeTime(stored pgtype.Timestamptz, location *time.Location) *time.Time {
	var value *time.Time

	if stored.Valid {
		value = new(stored.Time.In(location))
	}

	return value
}

// The collection is validated as a whole, since a stored budget must still
// cover at least one category.

func decodeCategoryIDs(stored []pgtype.UUID) (budgets.CategoryIDs, error) {
	var (
		categoryIDs budgets.CategoryIDs
		err         error
	)

	ids := make([]categories.ID, 0, len(stored))

	for _, value := range stored {
		var id categories.ID
		id, err = categories.NewID(uuid.UUID(value.Bytes))

		if err != nil {
			break
		}

		ids = append(ids, id)
	}

	if err == nil {
		categoryIDs = budgets.NewCategoryIDs(ids)
		err = categoryIDs.Validate()
	}

	if err != nil {
		categoryIDs = nil
	}

	return categoryIDs, err
}

// InTransaction keeps the statement's error in its chain, so the PostgreSQL
// error is found even when a rollback failure is joined to it.

func mapBudgetError(err error) error {
	result := err

	if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case postgresErr.Code == "23505" && postgresErr.ConstraintName == "budget_pkey":
			result = errors.Join(budgets.ErrBudgetAlreadyExists, err)
		case postgresErr.Code == "23505" && postgresErr.ConstraintName == "budget_category_pkey":
			result = errors.Join(budgets.ErrCategoryIDsDuplicate, err)
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "budget_workspace_id_fkey":
			result = errors.Join(workspaces.ErrWorkspaceNotFound, err)
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "budget_currency_code_fkey":
			result = errors.Join(currencies.ErrCurrencyNotFound, err)
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "budget_category_category_fkey":
			result = errors.Join(categories.ErrCategoryNotFound, err)
		}
	}

	return result
}
