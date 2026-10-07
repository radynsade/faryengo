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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
)

//
// Errors
//

var ErrNilRole = errors.New("nil role")

type errRoleWriteFailed struct {
	role *security.Role
	err  error
}

func (e errRoleWriteFailed) Role() *security.Role {
	return e.role
}

func (e errRoleWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of security.ErrRoleCreateFailed

type errRoleCreateFailed struct {
	errRoleWriteFailed
}

func newErrRoleCreateFailed(role *security.Role, err error) *errRoleCreateFailed {
	return &errRoleCreateFailed{
		errRoleWriteFailed: errRoleWriteFailed{role, err},
	}
}

func (e *errRoleCreateFailed) Error() string {
	return "failed to create a role"
}

// Implementation of security.ErrRoleUpdateFailed

type errRoleUpdateFailed struct {
	errRoleWriteFailed
}

func newErrRoleUpdateFailed(role *security.Role, err error) *errRoleUpdateFailed {
	return &errRoleUpdateFailed{
		errRoleWriteFailed: errRoleWriteFailed{role, err},
	}
}

func (e *errRoleUpdateFailed) Error() string {
	return "failed to update a role"
}

// Implementation of security.ErrRoleDeleteFailed

type errRoleDeleteFailed struct {
	id  security.RoleID
	err error
}

func newErrRoleDeleteFailed(id security.RoleID, err error) *errRoleDeleteFailed {
	return &errRoleDeleteFailed{id, err}
}

func (e errRoleDeleteFailed) RoleID() security.RoleID {
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
	pool roleDB
}

var _ security.RoleRepository = (*RoleRepository)(nil)

func NewRoleRepository(pool *pgxpool.Pool) (*RoleRepository, error) {
	var (
		repository *RoleRepository
		err        error
	)

	if pool == nil {
		err = ErrNilPool
	} else {
		repository = &RoleRepository{pool: pool}
	}

	return repository, err
}

// Create

func (r *RoleRepository) Create(
	ctx context.Context,
	role *security.Role,
) security.ErrRoleCreateFailed {
	var err security.ErrRoleCreateFailed

	if r == nil || r.pool == nil {
		err = newErrRoleCreateFailed(role, ErrNilPool)
	} else if role == nil {
		err = newErrRoleCreateFailed(role, ErrNilRole)
	} else if validationErr := role.Validate(); validationErr != nil {
		err = newErrRoleCreateFailed(role, validationErr)
	} else if createErr := r.createValidRole(ctx, role); createErr != nil {
		err = newErrRoleCreateFailed(role, createErr)
	}

	return err
}

// Update

func (r *RoleRepository) Update(
	ctx context.Context,
	role *security.Role,
) security.ErrRoleUpdateFailed {
	var err security.ErrRoleUpdateFailed

	if r == nil || r.pool == nil {
		err = newErrRoleUpdateFailed(role, ErrNilPool)
	} else if role == nil {
		err = newErrRoleUpdateFailed(role, ErrNilRole)
	} else if validationErr := role.Validate(); validationErr != nil {
		err = newErrRoleUpdateFailed(role, validationErr)
	} else if updateErr := r.updateValidRole(ctx, role); updateErr != nil {
		err = newErrRoleUpdateFailed(role, updateErr)
	}

	return err
}

// Delete relies on PostgreSQL's foreign key to reject roles assigned to users.
// The existing delete_role_name trigger removes the name and its translations
// in the same statement, so a rejected deletion leaves them intact.

func (r *RoleRepository) Delete(
	ctx context.Context,
	id security.RoleID,
) security.ErrRoleDeleteFailed {
	var err security.ErrRoleDeleteFailed

	if r == nil || r.pool == nil {
		err = newErrRoleDeleteFailed(id, ErrNilPool)
	} else if validationErr := id.Validate(); validationErr != nil {
		err = newErrRoleDeleteFailed(id, fmt.Errorf("delete role: %w", validationErr))
	} else {
		storedID := uuid.UUID(id)

		query, args, buildErr := goqu.Dialect("postgres").
			Delete("role").
			Where(goqu.Ex{"id": storedID.String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrRoleDeleteFailed(id, fmt.Errorf("build delete role %s query: %w", storedID, buildErr))
		} else if bindErr := bindUUIDArgs(args, [16]byte(storedID)); bindErr != nil {
			err = newErrRoleDeleteFailed(id, fmt.Errorf("bind delete role %s query: %w", storedID, bindErr))
		} else {
			tag, deleteErr := r.pool.Exec(ctx, query, args...)

			if deleteErr != nil {
				err = newErrRoleDeleteFailed(id, fmt.Errorf("delete role %s: %w", storedID, mapRoleError(deleteErr)))
			} else if tag.RowsAffected() == 0 {
				err = newErrRoleDeleteFailed(id, fmt.Errorf("delete role %s: %w", storedID, security.ErrRoleNotFound))
			}
		}
	}

	return err
}

// Find by an ID

func (r *RoleRepository) FindByID(
	ctx context.Context,
	id security.RoleID,
) (*security.Role, error) {
	var role *security.Role
	var err error

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("find role by ID: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)

		query, args, buildErr := goqu.Dialect("postgres").
			From("role").
			Select(roleColumns()...).
			Where(goqu.Ex{"id": storedID.String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build find role %s query: %w", storedID, buildErr)
		} else if bindErr := bindUUIDArgs(args, [16]byte(storedID)); bindErr != nil {
			err = fmt.Errorf("build find role %s query: %w", storedID, bindErr)
		} else {
			role, err = scanRole(r.pool.QueryRow(ctx, query, args...))

			if errors.Is(err, pgx.ErrNoRows) {
				err = fmt.Errorf("find role %s: %w", storedID, security.ErrRoleNotFound)
			} else if err != nil {
				err = fmt.Errorf("find role %s: %w", storedID, err)
			}
		}
	}

	return role, err
}

// Count roles matching filters

func (r *RoleRepository) Count(
	ctx context.Context,
	filters security.RoleFilter,
) (int, error) {
	var total int
	var err error

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else if filterErr := filters.Validate(); filterErr != nil {
		err = filterErr
	} else {
		query, args, buildErr := roleFilterQuery(filters).
			Select(goqu.COUNT("*")).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build count roles query: %w", buildErr)
		} else if scanErr := r.pool.QueryRow(ctx, query, args...).Scan(&total); scanErr != nil {
			err = fmt.Errorf("count roles: %w", scanErr)
		}
	}

	return total, err
}

// Find roles matching a query

func (r *RoleRepository) Find(
	ctx context.Context,
	options security.RoleQuery,
) ([]*security.Role, error) {
	var roles []*security.Role
	var err error

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else if validationErr := options.Validate(); validationErr != nil {
		err = validationErr
	} else {
		query := roleFilterQuery(options.Filters()).
			Select(roleColumns()...)

		var sort exp.Orderable = goqu.C("id")

		switch options.SortBy() {
		case security.RoleSortName:
			sort = goqu.L(`lower((SELECT content FROM "translation" t JOIN "language" l ON l.code = t.language_code WHERE t.text_id = "role".name_id ORDER BY (t.language_code = ?) DESC, l.is_fallback DESC, t.language_code LIMIT 1))`, string(options.Language))
		case security.RoleSortIsSuper:
			sort = goqu.C("is_super")
		}

		order := sort.Asc()

		if options.SortOrder().IsDesc() {
			order = sort.Desc()
		}

		query = query.Order(order)

		if options.SortBy() != security.RoleSortID {
			query = query.OrderAppend(goqu.C("id").Asc())
		}

		sql, args, buildErr := query.
			Limit(uint(options.Limit())).
			Offset(uint((options.Page() - 1) * options.Limit())).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build find roles query: %w", buildErr)
		} else {
			rows, queryErr := r.pool.Query(ctx, sql, args...)

			if queryErr != nil {
				err = fmt.Errorf("find roles: %w", queryErr)
			} else {
				defer rows.Close()

				roles = make([]*security.Role, 0, options.Limit())

				for rows.Next() {
					var role *security.Role
					role, err = scanRole(rows)

					if err != nil {
						break
					}

					roles = append(roles, role)
				}

				if err == nil {
					err = rows.Err()
				}

				if err != nil {
					err = fmt.Errorf("read roles: %w", err)
				}
			}
		}
	}

	if err != nil {
		roles = nil
	}

	return roles, err
}

//
// Helpers
//

func (r *RoleRepository) createValidRole(
	ctx context.Context,
	role *security.Role,
) error {
	id := uuid.UUID(role.ID)

	tx, err := r.pool.Begin(ctx)

	if err != nil {
		err = fmt.Errorf("begin create role %s: %w", id, err)
	} else {
		defer func() { _ = tx.Rollback(ctx) }()

		var nameID int64

		err = tx.QueryRow(ctx, `INSERT INTO "text" DEFAULT VALUES RETURNING id`).Scan(&nameID)

		if err != nil {
			err = fmt.Errorf("create name for role %s: %w", id, err)
		} else {
			var (
				query string
				args  []any
			)

			query, args, err = goqu.Dialect("postgres").
				Insert("role").
				Cols("id", "name_id", "permissions", "is_super").
				Vals(goqu.Vals{
					id.String(),
					nameID,
					rolePermissionsArray(role.Permissions),
					role.IsSuper,
				}).
				OnConflict(goqu.DoNothing()).
				Returning("name_id").
				Prepared(true).
				ToSQL()

			if err == nil {
				err = bindUUIDArgs(args, [16]byte(id))
			}

			if err == nil {
				err = tx.QueryRow(ctx, query, args...).Scan(&nameID)

				if errors.Is(err, pgx.ErrNoRows) {
					err = security.ErrRoleAlreadyExists
				}
			}

			if err == nil {
				err = writeRoleTranslations(ctx, tx, id, nameID, role.Name)
			}

			if err == nil {
				err = tx.Commit(ctx)
			}

			if err != nil {
				err = fmt.Errorf("create role %s: %w", id, err)
			}
		}
	}

	return err
}

func (r *RoleRepository) updateValidRole(
	ctx context.Context,
	role *security.Role,
) error {
	id := uuid.UUID(role.ID)

	tx, err := r.pool.Begin(ctx)

	if err != nil {
		err = fmt.Errorf("begin update role %s: %w", id, err)
	} else {
		defer func() { _ = tx.Rollback(ctx) }()

		var (
			query string
			args  []any
		)

		query, args, err = goqu.Dialect("postgres").
			Update("role").
			Set(goqu.Record{
				"permissions": rolePermissionsArray(role.Permissions),
				"is_super":    role.IsSuper,
			}).
			Where(goqu.Ex{"id": id.String()}).
			Returning("name_id").
			Prepared(true).
			ToSQL()

		if err == nil {
			err = bindUUIDArgsAt(args, len(args)-1, [16]byte(id))
		}

		var nameID int64

		if err == nil {
			err = tx.QueryRow(ctx, query, args...).Scan(&nameID)

			if errors.Is(err, pgx.ErrNoRows) {
				err = security.ErrRoleNotFound
			}
		}

		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM "translation" WHERE text_id = $1`, nameID)
		}

		if err == nil {
			err = writeRoleTranslations(ctx, tx, id, nameID, role.Name)
		}

		if err == nil {
			err = tx.Commit(ctx)
		}

		if err != nil {
			err = fmt.Errorf("update role %s: %w", id, err)
		}
	}

	return err
}

func roleColumns() []any {
	return []any{
		"id",
		goqu.L(`"permissions"::text[]`),
		goqu.L(`ARRAY(SELECT language_code FROM "translation" WHERE text_id = "role".name_id ORDER BY language_code)`),
		goqu.L(`ARRAY(SELECT content FROM "translation" WHERE text_id = "role".name_id ORDER BY language_code)`),
		"is_super",
	}
}

func roleFilterQuery(filters security.RoleFilter) *goqu.SelectDataset {
	query := goqu.Dialect("postgres").From("role")

	if filters.IDLike != "" {
		query = query.Where(goqu.L(`"role".id::text ILIKE ?`, likeSubstring(filters.IDLike)))
	}

	if filters.NameLike != "" {
		query = query.Where(goqu.L(`EXISTS (SELECT 1 FROM "translation" WHERE text_id = "role".name_id AND content ILIKE ?)`, likeSubstring(filters.NameLike)))
	}

	if len(filters.Permissions) > 0 {
		query = query.Where(goqu.Or(
			goqu.Ex{"is_super": true},
			goqu.L(`"role".permissions @> ?`, rolePermissionsArray(filters.Permissions)),
		))
	}

	if filters.IsSuper != nil {
		query = query.Where(goqu.Ex{"is_super": *filters.IsSuper})
	}

	return query
}

func likeSubstring(value string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value) + "%"
}

func rolePermissionsArray(permissions []security.Permission) exp.LiteralExpression {
	placeholders := make([]string, len(permissions))
	values := make([]any, len(permissions))

	for index, permission := range permissions {
		placeholders[index] = "?::permission"
		values[index] = string(permission)
	}

	return goqu.L("ARRAY["+strings.Join(placeholders, ",")+"]::permission[]", values...)
}

func writeRoleTranslations(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
	nameID int64,
	name languages.Text,
) error {
	var err error

	for _, code := range slices.Sorted(maps.Keys(name)) {
		var (
			query string
			args  []any
		)

		query, args, err = goqu.Dialect("postgres").
			Insert("translation").
			Cols("text_id", "language_code", "content").
			Vals(goqu.Vals{nameID, string(code), string(name[code])}).
			Prepared(true).
			ToSQL()

		if err == nil {
			_, err = tx.Exec(ctx, query, args...)

			if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok &&
				postgresErr.Code == "23503" && postgresErr.ConstraintName == "translation_language_code_fkey" {
				err = errors.Join(languages.ErrLanguageNotFound, err)
			}
		}

		if err != nil {
			err = fmt.Errorf("write name translation for role %s: %w", id, err)

			break
		}
	}

	return err
}

func scanRole(row pgx.Row) (*security.Role, error) {
	var role *security.Role
	var id pgtype.UUID
	var permissions, codes, contents []string
	var isSuper bool

	err := row.Scan(&id, &permissions, &codes, &contents, &isSuper)

	if err == nil {
		if len(codes) != len(contents) {
			err = fmt.Errorf("decode role: mismatched name translations")
		} else {
			name := make(languages.Text, len(codes))

			for index, code := range codes {
				if _, duplicate := name[languages.Code(code)]; duplicate {
					err = fmt.Errorf("decode role: duplicate translation: %w", languages.ErrInvalidText)

					break
				}

				name[languages.Code(code)] = languages.TranslationContent(contents[index])
			}

			if err == nil {
				rawPermissions := make([]security.Permission, len(permissions))

				for index, permission := range permissions {
					rawPermissions[index] = security.Permission(permission)
				}

				role = security.NewRole(
					security.RoleID(id.Bytes),
					name,
					rawPermissions,
					isSuper,
				)

				err = role.Validate()

				if err != nil {
					role = nil
				}
			}
		}
	}

	return role, err
}

func mapRoleError(err error) error {
	result := err

	if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case (postgresErr.Code == "23503" || postgresErr.Code == "23001") && postgresErr.ConstraintName == "user_role_id_fkey":
			result = fmt.Errorf("%w: %w", security.ErrRoleAlreadyInUse, err)
		}
	}

	return result
}
