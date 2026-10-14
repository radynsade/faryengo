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

	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Errors
//

type errCategoryWriteFailed struct {
	category *categories.Category
	err      error
}

func (e errCategoryWriteFailed) Category() *categories.Category {
	return e.category
}

func (e errCategoryWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of categories.ErrCategoryCreateFailed

type errCategoryCreateFailed struct {
	errCategoryWriteFailed
}

func newErrCategoryCreateFailed(category *categories.Category, err error) *errCategoryCreateFailed {
	return &errCategoryCreateFailed{
		errCategoryWriteFailed: errCategoryWriteFailed{category, err},
	}
}

func (e *errCategoryCreateFailed) Error() string {
	return "failed to create a category"
}

// Implementation of categories.ErrCategoryUpdateFailed

type errCategoryUpdateFailed struct {
	errCategoryWriteFailed
}

func newErrCategoryUpdateFailed(category *categories.Category, err error) *errCategoryUpdateFailed {
	return &errCategoryUpdateFailed{
		errCategoryWriteFailed: errCategoryWriteFailed{category, err},
	}
}

func (e *errCategoryUpdateFailed) Error() string {
	return "failed to update a category"
}

// Implementation of categories.ErrCategoryDeleteFailed

type errCategoryDeleteFailed struct {
	id  categories.CategoryID
	err error
}

func newErrCategoryDeleteFailed(id categories.CategoryID, err error) *errCategoryDeleteFailed {
	return &errCategoryDeleteFailed{id, err}
}

func (e *errCategoryDeleteFailed) CategoryID() categories.CategoryID {
	return e.id
}

func (e *errCategoryDeleteFailed) Unwrap() error {
	return e.err
}

func (e *errCategoryDeleteFailed) Error() string {
	return "failed to delete a category"
}

//
// Repository
//

type CategoryRepository struct {
	pool pgxdb.DB
}

var _ categories.CategoryRepository = (*CategoryRepository)(nil)

func NewCategoryRepository(pool pgxdb.DB) (*CategoryRepository, error) {
	var (
		repository *CategoryRepository
		err        error
	)

	if pgxdb.IsNil(pool) {
		err = pgxdb.ErrNilDB
	} else {
		repository = &CategoryRepository{pool: pool}
	}

	return repository, err
}

// The workspace foreign key reports a missing workspace, and the primary key
// reports an existing category.

func (r *CategoryRepository) Create(
	ctx context.Context,
	category *categories.Category,
) categories.ErrCategoryCreateFailed {
	var err categories.ErrCategoryCreateFailed

	if r == nil || r.pool == nil {
		err = newErrCategoryCreateFailed(category, pgxdb.ErrNilDB)
	} else if category == nil {
		err = newErrCategoryCreateFailed(category, categories.ErrCategoryNil)
	} else if validationErr := category.Validate(); validationErr != nil {
		err = newErrCategoryCreateFailed(category, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Insert("category").
			Rows(goqu.Record{
				"id":           uuid.UUID(category.ID).String(),
				"workspace_id": uuid.UUID(category.WorkspaceID).String(),
				"name":         string(category.Name),
				"icon":         storedIcon(category.Icon),
				"color":        string(category.Color),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrCategoryCreateFailed(category, buildErr)
		} else {
			_, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrCategoryCreateFailed(category, mapCategoryError(execErr))
			}
		}
	}

	return err
}

// The workspace is part of the match rather than the update, so a category is
// never moved to another workspace.

func (r *CategoryRepository) Update(
	ctx context.Context,
	category *categories.Category,
) categories.ErrCategoryUpdateFailed {
	var err categories.ErrCategoryUpdateFailed

	if r == nil || r.pool == nil {
		err = newErrCategoryUpdateFailed(category, pgxdb.ErrNilDB)
	} else if category == nil {
		err = newErrCategoryUpdateFailed(category, categories.ErrCategoryNil)
	} else if validationErr := category.Validate(); validationErr != nil {
		err = newErrCategoryUpdateFailed(category, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Update("category").
			Set(goqu.Record{
				"name":  string(category.Name),
				"icon":  storedIcon(category.Icon),
				"color": string(category.Color),
			}).
			Where(goqu.Ex{
				"id":           uuid.UUID(category.ID).String(),
				"workspace_id": uuid.UUID(category.WorkspaceID).String(),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrCategoryUpdateFailed(category, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrCategoryUpdateFailed(category, mapCategoryError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrCategoryUpdateFailed(category, categories.ErrCategoryNotFound)
			}
		}
	}

	return err
}

// Delete

func (r *CategoryRepository) Delete(
	ctx context.Context,
	id categories.CategoryID,
) categories.ErrCategoryDeleteFailed {
	var err categories.ErrCategoryDeleteFailed

	if r == nil || r.pool == nil {
		err = newErrCategoryDeleteFailed(id, pgxdb.ErrNilDB)
	} else if validationErr := id.Validate(); validationErr != nil {
		err = newErrCategoryDeleteFailed(id, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Delete("category").
			Where(goqu.Ex{"id": uuid.UUID(id).String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrCategoryDeleteFailed(id, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrCategoryDeleteFailed(id, mapCategoryError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrCategoryDeleteFailed(id, categories.ErrCategoryNotFound)
			}
		}
	}

	return err
}

// Find by an ID

func (r *CategoryRepository) FindByID(
	ctx context.Context,
	id categories.CategoryID,
) (*categories.Category, error) {
	var (
		category *categories.Category
		err      error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a category by ID: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		category, err = r.find(ctx, goqu.Ex{"id": storedID.String()}, false)

		if err != nil {
			err = fmt.Errorf("failed to find a category %s: %w", storedID, err)
		}
	}

	return category, err
}

// A row lock lasts only until its transaction ends, so locking requires a
// handle that can provide a transaction; a pool or a bare connection is
// rejected before I/O.

func (r *CategoryRepository) FindByIDForUpdate(
	ctx context.Context,
	id categories.CategoryID,
) (*categories.Category, error) {
	var (
		category *categories.Category
		err      error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a category by ID for update: %w", validationErr)
	} else if !pgxdb.IsTransaction(pgxdb.FromContext(ctx, r.pool)) {
		err = pgxdb.ErrTransactionRequired
	} else {
		storedID := uuid.UUID(id)
		category, err = r.find(ctx, goqu.Ex{"id": storedID.String()}, true)

		if err != nil {
			err = fmt.Errorf("failed to find a category %s for update: %w", storedID, err)
		}
	}

	return category, err
}

// Find categories matching a query; Page is 1-based

func (r *CategoryRepository) Find(
	ctx context.Context,
	query categories.CategoryQuery,
) ([]*categories.Category, error) {
	var (
		result []*categories.Category
		err    error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := query.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find categories: %w", validationErr)
	} else {
		sql, args, buildErr := categoryFilterDataset(query.Filter).
			Select(categoryColumns()...).
			Order(categoryOrder(query)...).
			Limit(query.Limit).
			Offset((query.Page - 1) * query.Limit).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the find categories query: %w", buildErr)
		} else {
			rows, queryErr := pgxdb.FromContext(ctx, r.pool).Query(ctx, sql, args...)

			if queryErr != nil {
				err = fmt.Errorf("failed to find categories: %w", queryErr)
			} else {
				defer rows.Close()

				result = make([]*categories.Category, 0, query.Limit)

				for rows.Next() {
					var category *categories.Category
					category, err = scanCategory(rows)

					if err != nil {
						break
					}

					result = append(result, category)
				}

				if err == nil {
					err = rows.Err()
				}

				if err != nil {
					err = fmt.Errorf("failed to read categories: %w", err)
					result = nil
				}
			}
		}
	}

	return result, err
}

// Count categories matching a filter

func (r *CategoryRepository) Count(
	ctx context.Context,
	filter categories.CategoryFilter,
) (int, error) {
	var (
		total int
		err   error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := filter.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to count categories: %w", validationErr)
	} else {
		query, args, buildErr := categoryFilterDataset(filter).
			Select(goqu.COUNT("*")).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the count categories query: %w", buildErr)
		} else if scanErr := pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&total); scanErr != nil {
			err = fmt.Errorf("failed to count categories: %w", scanErr)
		}
	}

	return total, err
}

//
// Helpers
//

func (r *CategoryRepository) find(
	ctx context.Context,
	filter goqu.Ex,
	forUpdate bool,
) (*categories.Category, error) {
	var (
		category *categories.Category
		err      error
	)

	dataset := goqu.Dialect("postgres").
		From("category").
		Select(categoryColumns()...).
		Where(filter).
		Prepared(true)

	if forUpdate {
		dataset = dataset.ForUpdate(exp.Wait)
	}

	query, args, buildErr := dataset.ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the category lookup query: %w", buildErr)
	} else {
		category, err = scanCategory(pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...))
	}

	return category, err
}

func storedIcon(icon *categories.Icon) *string {
	var stored *string

	if icon != nil {
		value := string(*icon)
		stored = &value
	}

	return stored
}

func categoryColumns() []any {
	return []any{
		"id",
		"workspace_id",
		"name",
		"icon",
		"color",
	}
}

func categoryFilterDataset(filter categories.CategoryFilter) *goqu.SelectDataset {
	dataset := goqu.Dialect("postgres").From("category")

	if filter.IDLike != "" {
		dataset = dataset.Where(goqu.L(`"category".id::text ILIKE ?`, likeSubstring(filter.IDLike)))
	}

	if filter.NameLike != "" {
		dataset = dataset.Where(goqu.L(`"category".name ILIKE ?`, likeSubstring(filter.NameLike)))
	}

	if filter.WorkspaceID != nil {
		dataset = dataset.Where(goqu.Ex{"workspace_id": uuid.UUID(*filter.WorkspaceID).String()})
	}

	return dataset
}

// Name sorts ignore letter case. The ID is the tie-breaker for every sort.

func categoryOrder(query categories.CategoryQuery) []exp.OrderedExpression {
	var sort exp.Orderable = goqu.C("id")

	if query.SortBy == categories.CategorySortName {
		sort = goqu.L(`lower("category".name)`)
	}

	order := []exp.OrderedExpression{sort.Asc()}

	if query.SortOrder.IsDesc() {
		order[0] = sort.Desc()
	}

	if query.SortBy != categories.CategorySortID {
		order = append(order, goqu.C("id").Asc())
	}

	return order
}

func likeSubstring(value string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value) + "%"
}

func scanCategory(row pgx.Row) (*categories.Category, error) {
	var (
		category    *categories.Category
		err         error
		id          pgtype.UUID
		workspaceID pgtype.UUID
		name        string
		icon        pgtype.Text
		color       string
	)

	scanErr := row.Scan(&id, &workspaceID, &name, &icon, &color)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = categories.ErrCategoryNotFound
	} else if scanErr != nil {
		err = fmt.Errorf("scan category: %w", scanErr)
	} else {
		var (
			categoryID          categories.CategoryID
			categoryWorkspaceID workspaces.WorkspaceID
			categoryName        categories.CategoryName
			categoryIcon        *categories.Icon
			categoryColor       categories.Color
		)

		categoryID, err = categories.NewCategoryID(uuid.UUID(id.Bytes))

		if err == nil {
			categoryWorkspaceID, err = workspaces.NewWorkspaceID(uuid.UUID(workspaceID.Bytes))
		}

		if err == nil {
			categoryName, err = categories.NewCategoryName(name)
		}

		if err == nil && icon.Valid {
			var value categories.Icon

			value, err = categories.NewIcon(icon.String)
			categoryIcon = &value
		}

		if err == nil {
			categoryColor, err = categories.NewColor(color)
		}

		if err == nil {
			category = categories.NewCategory(
				categoryID,
				categoryWorkspaceID,
				categoryName,
				categoryIcon,
				categoryColor,
			)
		} else {
			err = fmt.Errorf("failed to decode a category: %w: %w", categories.ErrCategoryInvalid, err)
		}
	}

	return category, err
}

func mapCategoryError(err error) error {
	result := err

	if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case postgresErr.Code == "23505" && postgresErr.ConstraintName == "category_pkey":
			result = errors.Join(categories.ErrCategoryAlreadyExists, err)
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "category_workspace_id_fkey":
			result = errors.Join(workspaces.ErrWorkspaceNotFound, err)
		}
	}

	return result
}
