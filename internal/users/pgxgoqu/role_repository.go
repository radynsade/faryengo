package pgxgoqu

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Errors
//

type errRoleWriteFailed struct {
	role *users.Role
	err  error
}

func (e errRoleWriteFailed) Role() *users.Role {
	return e.role
}

func (e errRoleWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of users.ErrRoleCreateFailed

type errRoleCreateFailed struct {
	errRoleWriteFailed
}

func newErrRoleCreateFailed(role *users.Role, err error) *errRoleCreateFailed {
	return &errRoleCreateFailed{
		errRoleWriteFailed: errRoleWriteFailed{role, err},
	}
}

func (e *errRoleCreateFailed) Error() string {
	return "failed to create a role"
}

// Implementation of users.ErrRoleUpdateFailed

type errRoleUpdateFailed struct {
	errRoleWriteFailed
}

func newErrRoleUpdateFailed(role *users.Role, err error) *errRoleUpdateFailed {
	return &errRoleUpdateFailed{
		errRoleWriteFailed: errRoleWriteFailed{role, err},
	}
}

func (e *errRoleUpdateFailed) Error() string {
	return "failed to update a role"
}

// Implementation of users.ErrRoleDeleteFailed

type errRoleDeleteFailed struct {
	id  users.RoleID
	err error
}

func newErrRoleDeleteFailed(id users.RoleID, err error) *errRoleDeleteFailed {
	return &errRoleDeleteFailed{id, err}
}

func (e errRoleDeleteFailed) RoleID() users.RoleID {
	return e.id
}

func (e errRoleDeleteFailed) Unwrap() error {
	return e.err
}

func (e *errRoleDeleteFailed) Error() string {
	return "failed to delete a role"
}

//
// Repository
//

type RoleRepository struct {
	pool pgxdb.DB
}

var _ users.RoleRepository = (*RoleRepository)(nil)

func NewRoleRepository(pool pgxdb.DB) (*RoleRepository, error) {
	var (
		repository *RoleRepository
		err        error
	)

	if pgxdb.IsNil(pool) {
		err = pgxdb.ErrNilDB
	} else {
		repository = &RoleRepository{pool: pool}
	}

	return repository, err
}

// Create writes the role, its name text, and the name translations in one
// transaction, so a failed translation leaves no partial role behind.

func (r *RoleRepository) Create(
	ctx context.Context,
	role *users.Role,
) users.ErrRoleCreateFailed {
	var err users.ErrRoleCreateFailed

	if r == nil || r.pool == nil {
		err = newErrRoleCreateFailed(role, pgxdb.ErrNilDB)
	} else if role == nil {
		err = newErrRoleCreateFailed(role, users.ErrRoleNil)
	} else if validationErr := role.Validate(); validationErr != nil {
		err = newErrRoleCreateFailed(role, validationErr)
	} else if createErr := r.create(ctx, role); createErr != nil {
		err = newErrRoleCreateFailed(role, createErr)
	}

	return err
}

// Update replaces the role's permissions, super designation, and every name
// translation in one transaction.

func (r *RoleRepository) Update(
	ctx context.Context,
	role *users.Role,
) users.ErrRoleUpdateFailed {
	var err users.ErrRoleUpdateFailed

	if r == nil || r.pool == nil {
		err = newErrRoleUpdateFailed(role, pgxdb.ErrNilDB)
	} else if role == nil {
		err = newErrRoleUpdateFailed(role, users.ErrRoleNil)
	} else if validationErr := role.Validate(); validationErr != nil {
		err = newErrRoleUpdateFailed(role, validationErr)
	} else if updateErr := r.update(ctx, role); updateErr != nil {
		err = newErrRoleUpdateFailed(role, updateErr)
	}

	return err
}

// Delete relies on the user foreign key to reject roles assigned to users. The
// delete_role_name trigger removes the name text and its translations in the
// same statement, so a rejected deletion leaves them intact.

func (r *RoleRepository) Delete(
	ctx context.Context,
	id users.RoleID,
) users.ErrRoleDeleteFailed {
	var err users.ErrRoleDeleteFailed

	if r == nil || r.pool == nil {
		err = newErrRoleDeleteFailed(id, pgxdb.ErrNilDB)
	} else if validationErr := id.Validate(); validationErr != nil {
		err = newErrRoleDeleteFailed(id, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Delete("role").
			Where(goqu.Ex{"id": uuid.UUID(id).String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrRoleDeleteFailed(id, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrRoleDeleteFailed(id, mapRoleError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrRoleDeleteFailed(id, users.ErrRoleNotFound)
			}
		}
	}

	return err
}

// Find by an ID

func (r *RoleRepository) FindByID(
	ctx context.Context,
	id users.RoleID,
) (*users.Role, error) {
	var (
		role *users.Role
		err  error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a role by ID: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		role, err = r.find(ctx, goqu.Ex{"id": storedID.String()}, false)

		if err != nil {
			err = fmt.Errorf("failed to find a role %s: %w", storedID, err)
		}
	}

	return role, err
}

// A row lock lasts only until its transaction ends, so locking requires a
// repository built on a pgx.Tx; a pool or a bare connection is rejected before
// I/O.

func (r *RoleRepository) FindByIDForUpdate(
	ctx context.Context,
	id users.RoleID,
) (*users.Role, error) {
	var (
		role *users.Role
		err  error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a role by ID for update: %w", validationErr)
	} else if !pgxdb.IsTransaction(pgxdb.FromContext(ctx, r.pool)) {
		err = pgxdb.ErrTransactionRequired
	} else {
		storedID := uuid.UUID(id)
		role, err = r.find(ctx, goqu.Ex{"id": storedID.String()}, true)

		if err != nil {
			err = fmt.Errorf("failed to find a role %s for update: %w", storedID, err)
		}
	}

	return role, err
}

// Find roles matching a query; Page is 1-based

func (r *RoleRepository) Find(
	ctx context.Context,
	query users.RoleQuery,
) ([]*users.Role, error) {
	var (
		result []*users.Role
		err    error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := validateRoleQuery(query); validationErr != nil {
		err = fmt.Errorf("failed to find roles: %w", validationErr)
	} else {
		sql, args, buildErr := roleFilterDataset(query.Filter).
			Select(roleColumns()...).
			Order(roleOrder(query)...).
			Limit(query.Limit).
			Offset((query.Page - 1) * query.Limit).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the find roles query: %w", buildErr)
		} else {
			rows, queryErr := pgxdb.FromContext(ctx, r.pool).Query(ctx, sql, args...)

			if queryErr != nil {
				err = fmt.Errorf("failed to find roles: %w", queryErr)
			} else {
				defer rows.Close()

				result = make([]*users.Role, 0, query.Limit)

				for rows.Next() {
					var role *users.Role
					role, err = scanRole(rows)

					if err != nil {
						break
					}

					result = append(result, role)
				}

				if err == nil {
					err = rows.Err()
				}

				if err != nil {
					err = fmt.Errorf("failed to read roles: %w", err)
					result = nil
				}
			}
		}
	}

	return result, err
}

// Count roles matching a filter

func (r *RoleRepository) Count(
	ctx context.Context,
	filter users.RoleFilter,
) (int, error) {
	var (
		total int
		err   error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := filter.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to count roles: %w", validationErr)
	} else {
		query, args, buildErr := roleFilterDataset(filter).
			Select(goqu.COUNT("*")).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the count roles query: %w", buildErr)
		} else if scanErr := pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&total); scanErr != nil {
			err = fmt.Errorf("failed to count roles: %w", scanErr)
		}
	}

	return total, err
}

//
// Helpers
//

func (r *RoleRepository) find(
	ctx context.Context,
	filter goqu.Ex,
	forUpdate bool,
) (*users.Role, error) {
	var (
		role *users.Role
		err  error
	)

	dataset := goqu.Dialect("postgres").
		From("role").
		Select(roleColumns()...).
		Where(filter).
		Prepared(true)

	if forUpdate {
		dataset = dataset.ForUpdate(exp.Wait)
	}

	query, args, buildErr := dataset.ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the role lookup query: %w", buildErr)
	} else {
		role, err = scanRole(pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...))
	}

	return role, err
}

func (r *RoleRepository) create(
	ctx context.Context,
	role *users.Role,
) error {
	err := pgxdb.InTransaction(ctx, r.pool, func(ctx context.Context) error {
		var nameID int64

		db := pgxdb.FromContext(ctx, r.pool)
		err := db.QueryRow(ctx, `INSERT INTO "text" DEFAULT VALUES RETURNING id`).Scan(&nameID)

		if err == nil {
			err = insertRole(ctx, db, role, nameID)
		}

		if err == nil {
			err = insertRoleName(ctx, db, nameID, role.Name)
		}

		return err
	})

	return err
}

func (r *RoleRepository) update(
	ctx context.Context,
	role *users.Role,
) error {
	err := pgxdb.InTransaction(ctx, r.pool, func(ctx context.Context) error {
		db := pgxdb.FromContext(ctx, r.pool)
		nameID, err := updateRole(ctx, db, role)

		if err == nil {
			_, err = db.Exec(ctx, `DELETE FROM "translation" WHERE text_id = $1`, nameID)
		}

		if err == nil {
			err = insertRoleName(ctx, db, nameID, role.Name)
		}

		return err
	})

	return err
}

func insertRole(
	ctx context.Context,
	db pgxdb.DB,
	role *users.Role,
	nameID int64,
) error {
	query, args, err := goqu.Dialect("postgres").
		Insert("role").
		Rows(goqu.Record{
			"id":          uuid.UUID(role.ID).String(),
			"name_id":     nameID,
			"permissions": rolePermissionsArray(role.Permissions),
			"is_super":    role.IsSuper,
		}).
		OnConflict(goqu.DoNothing()).
		Prepared(true).
		ToSQL()

	if err == nil {
		var tag pgconn.CommandTag

		tag, err = db.Exec(ctx, query, args...)

		if err == nil && tag.RowsAffected() == 0 {
			err = users.ErrRoleAlreadyExists
		}
	}

	return err
}

func updateRole(
	ctx context.Context,
	db pgxdb.DB,
	role *users.Role,
) (int64, error) {
	var nameID int64

	query, args, err := goqu.Dialect("postgres").
		Update("role").
		Set(goqu.Record{
			"permissions": rolePermissionsArray(role.Permissions),
			"is_super":    role.IsSuper,
		}).
		Where(goqu.Ex{"id": uuid.UUID(role.ID).String()}).
		Returning("name_id").
		Prepared(true).
		ToSQL()

	if err == nil {
		err = db.QueryRow(ctx, query, args...).Scan(&nameID)

		if errors.Is(err, pgx.ErrNoRows) {
			err = users.ErrRoleNotFound
		}
	}

	return nameID, err
}

func insertRoleName(
	ctx context.Context,
	db pgxdb.DB,
	nameID int64,
	name users.RoleName,
) error {
	rows := make([]any, 0, len(name))

	for _, code := range slices.Sorted(maps.Keys(name)) {
		rows = append(rows, goqu.Record{
			"text_id":       nameID,
			"language_code": string(code),
			"content":       string(name[code]),
		})
	}

	query, args, err := goqu.Dialect("postgres").
		Insert("translation").
		Rows(rows...).
		Prepared(true).
		ToSQL()

	if err == nil {
		_, err = db.Exec(ctx, query, args...)
		err = mapRoleError(err)
	}

	return err
}

func roleColumns() []any {
	return []any{
		"id",
		goqu.L(`ARRAY(SELECT language_code FROM "translation" WHERE text_id = "role".name_id ORDER BY language_code)`),
		goqu.L(`ARRAY(SELECT content FROM "translation" WHERE text_id = "role".name_id ORDER BY language_code)`),
		goqu.L(`"role".permissions::text[]`),
		"is_super",
	}
}

func roleFilterDataset(filter users.RoleFilter) *goqu.SelectDataset {
	dataset := goqu.Dialect("postgres").From("role")

	if filter.IDLike != "" {
		dataset = dataset.Where(goqu.L(`"role".id::text ILIKE ?`, likeSubstring(filter.IDLike)))
	}

	if filter.NameLike != "" {
		dataset = dataset.Where(goqu.L(
			`EXISTS (SELECT 1 FROM "translation" WHERE text_id = "role".name_id AND content ILIKE ?)`,
			likeSubstring(filter.NameLike),
		))
	}

	// A super role grants every permission, so it matches any permission set.
	if len(filter.Permissions) > 0 {
		dataset = dataset.Where(goqu.Or(
			goqu.Ex{"is_super": true},
			goqu.L(`"role".permissions @> ?`, rolePermissionsArray(filter.Permissions)),
		))
	}

	if filter.IsSuper != nil {
		dataset = dataset.Where(goqu.Ex{"is_super": *filter.IsSuper})
	}

	return dataset
}

// Roles sort by the fallback language's name translation, or by the
// alphabetically first translation when the fallback one is missing. The ID is
// the tie-breaker for every sort.

func roleOrder(query users.RoleQuery) []exp.OrderedExpression {
	var sort exp.Orderable = goqu.C("id")

	switch query.SortBy {
	case users.RoleSortName:
		sort = goqu.L(`lower((SELECT t.content FROM "translation" AS t ` +
			`JOIN "language" AS l ON l.code = t.language_code ` +
			`WHERE t.text_id = "role".name_id ` +
			`ORDER BY l.is_fallback DESC, t.language_code LIMIT 1))`)
	case users.RoleSortIsSuper:
		sort = goqu.C("is_super")
	}

	order := []exp.OrderedExpression{sort.Asc()}

	if query.SortOrder.IsDesc() {
		order[0] = sort.Desc()
	}

	if query.SortBy != users.RoleSortID {
		order = append(order, goqu.C("id").Asc())
	}

	return order
}

func validateRoleQuery(query users.RoleQuery) error {
	err := query.Validate()

	if err == nil {
		err = query.SortBy.Validate()
	}

	if err == nil && (query.Limit == 0 || query.Page == 0) {
		err = users.ErrInvalidRoleQuery
	}

	return err
}

func likeSubstring(value string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value) + "%"
}

func rolePermissionsArray(permissions users.Permissions) exp.LiteralExpression {
	placeholders := make([]string, len(permissions))
	values := make([]any, len(permissions))

	for index, permission := range permissions {
		placeholders[index] = "?::permission"
		values[index] = string(permission)
	}

	return goqu.L("ARRAY["+strings.Join(placeholders, ",")+"]::permission[]", values...)
}

func scanRole(row pgx.Row) (*users.Role, error) {
	var (
		role                         *users.Role
		err                          error
		id                           pgtype.UUID
		codes, contents, permissions []string
		isSuper                      bool
	)

	scanErr := row.Scan(&id, &codes, &contents, &permissions, &isSuper)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = users.ErrRoleNotFound
	} else if scanErr != nil {
		err = fmt.Errorf("scan role: %w", scanErr)
	} else if len(codes) != len(contents) {
		err = errors.New("failed to decode a role: mismatched name translations")
	} else {
		name := make(users.RoleName, len(codes))

		for index, code := range codes {
			name[languages.Code(code)] = languages.Translation(contents[index])
		}

		rolePermissions := make(users.Permissions, len(permissions))

		for index, permission := range permissions {
			rolePermissions[index] = users.Permission(permission)
		}

		role = users.NewRole(users.RoleID(id.Bytes), name, rolePermissions, isSuper)
		err = role.Validate()

		if err != nil {
			err = fmt.Errorf("failed to decode a role: %w", err)
			role = nil
		}
	}

	return role, err
}

func mapRoleError(err error) error {
	result := err

	if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case (postgresErr.Code == "23503" || postgresErr.Code == "23001") &&
			postgresErr.ConstraintName == "user_role_id_fkey":
			result = errors.Join(users.ErrRoleAlreadyInUse, err)
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "translation_language_code_fkey":
			result = errors.Join(languages.ErrLanguageNotFound, err)
		}
	}

	return result
}
