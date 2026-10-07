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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
)

type roleDB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type RoleRepository struct {
	db roleDB
}

var _ security.RoleRepository = (*RoleRepository)(nil)

func NewRoleRepository(pool *pgxpool.Pool) (*RoleRepository, error) {
	var repository *RoleRepository
	var err error

	if pool == nil {
		err = ErrNilPool
	} else {
		repository = &RoleRepository{db: pool}
	}

	return repository, err
}

func (r *RoleRepository) Create(ctx context.Context, role *security.Role) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if role == nil {
		err = ErrNilRole
	} else if validationErr := validateRole(role); validationErr != nil {
		err = validationErr
	} else {
		err = r.createValidRole(ctx, role)
	}

	return err
}

func (r *RoleRepository) createValidRole(ctx context.Context, role *security.Role) error {
	id := uuid.UUID(role.ID())
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create role %s: %w", id, err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	var nameID int64
	err = tx.QueryRow(ctx, `INSERT INTO "text" DEFAULT VALUES RETURNING id`).Scan(&nameID)
	if err != nil {
		return fmt.Errorf("create name for role %s: %w", id, err)
	}

	query, args, err := goqu.Dialect("postgres").
		Insert("role").
		Cols("id", "name_id", "permissions", "is_super").
		Vals(goqu.Vals{id.String(), nameID, rolePermissionsArray(role.Permissions()), role.IsSuper()}).
		OnConflict(goqu.DoNothing()).
		Returning("name_id").
		Prepared(true).
		ToSQL()
	if err != nil {
		return fmt.Errorf("build create role %s query: %w", id, err)
	}

	if err = bindUUIDArgs(args, [16]byte(id)); err != nil {
		return fmt.Errorf("bind create role %s query: %w", id, err)
	}

	var insertedNameID int64
	err = tx.QueryRow(ctx, query, args...).Scan(&insertedNameID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("create role %s: %w", id, security.ErrRoleAlreadyExists)
	} else if err != nil {
		return fmt.Errorf("create role %s: %w", id, err)
	}

	if err = writeRoleTranslations(ctx, tx, id, insertedNameID, role.Name()); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create role %s: %w", id, err)
	}

	return nil
}

func (r *RoleRepository) Update(ctx context.Context, role *security.Role) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if role == nil {
		err = ErrNilRole
	} else if validationErr := validateRole(role); validationErr != nil {
		err = validationErr
	} else {
		err = r.updateValidRole(ctx, role)
	}

	return err
}

func (r *RoleRepository) updateValidRole(ctx context.Context, role *security.Role) error {
	id := uuid.UUID(role.ID())
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin update role %s: %w", id, err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	query, args, err := goqu.Dialect("postgres").
		Update("role").
		Set(goqu.Record{"permissions": rolePermissionsArray(role.Permissions()), "is_super": role.IsSuper()}).
		Where(goqu.Ex{"id": id.String()}).
		Returning("name_id").
		Prepared(true).
		ToSQL()
	if err != nil {
		return fmt.Errorf("build update role %s query: %w", id, err)
	}

	if err = bindUUIDArgsAt(args, len(args)-1, [16]byte(id)); err != nil {
		return fmt.Errorf("bind update role %s query: %w", id, err)
	}

	var nameID int64
	err = tx.QueryRow(ctx, query, args...).Scan(&nameID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("update role %s: %w", id, security.ErrRoleNotFound)
	} else if err != nil {
		return fmt.Errorf("update role %s: %w", id, err)
	}

	_, err = tx.Exec(ctx, `DELETE FROM "translation" WHERE text_id = $1`, nameID)
	if err != nil {
		return fmt.Errorf("replace name for role %s: %w", id, err)
	}

	if err = writeRoleTranslations(ctx, tx, id, nameID, role.Name()); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit update role %s: %w", id, err)
	}

	return nil
}

func validateRole(role *security.Role) error {
	_, err := security.NewRole(role.ID(), role.Name(), role.Permissions())
	if err != nil {
		err = fmt.Errorf("validate role: %w", err)
	}

	return err
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

func writeRoleTranslations(ctx context.Context, tx pgx.Tx, id uuid.UUID, nameID int64, name languages.Text) error {
	for _, translation := range name.Translations() {
		query, args, err := goqu.Dialect("postgres").
			Insert("translation").
			Cols("text_id", "language_code", "content").
			Vals(goqu.Vals{nameID, string(translation.LanguageCode()), translation.Content()}).
			Prepared(true).
			ToSQL()
		if err != nil {
			return fmt.Errorf("build name translation for role %s: %w", id, err)
		}

		_, err = tx.Exec(ctx, query, args...)
		if err != nil {
			var foreignKey *pgconn.PgError

			if errors.As(err, &foreignKey) && foreignKey.Code == "23503" && foreignKey.ConstraintName == "translation_language_code_fkey" {
				err = fmt.Errorf("%w: %w", languages.ErrLanguageNotFound, err)
			}

			return fmt.Errorf("write name translation for role %s: %w", id, err)
		}
	}

	return nil
}

func (r *RoleRepository) FindByID(ctx context.Context, id security.RoleID) (*security.Role, error) {
	var role *security.Role
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("find role by ID: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		query, args, buildErr := goqu.Dialect("postgres").
			From("role").
			Select("id", goqu.L(`"permissions"::text[]`),
				goqu.L(`ARRAY(SELECT language_code FROM "translation" WHERE text_id = "role".name_id ORDER BY language_code)`),
				goqu.L(`ARRAY(SELECT content FROM "translation" WHERE text_id = "role".name_id ORDER BY language_code)`), "is_super").
			Where(goqu.Ex{"id": storedID.String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build find role %s query: %w", storedID, buildErr)
		} else if bindErr := bindUUIDArgs(args, [16]byte(storedID)); bindErr != nil {
			err = fmt.Errorf("build find role %s query: %w", storedID, bindErr)
		} else {
			role, err = scanRole(r.db.QueryRow(ctx, query, args...))

			if errors.Is(err, pgx.ErrNoRows) {
				err = fmt.Errorf("find role %s: %w", storedID, security.ErrRoleNotFound)
			} else if err != nil {
				err = fmt.Errorf("find role %s: %w", storedID, err)
			}
		}
	}

	return role, err
}

// Delete relies on PostgreSQL's foreign key to reject roles assigned to users.
// The existing delete_role_name trigger removes the name and its translations
// in the same statement, so a rejected deletion leaves them intact.
func (r *RoleRepository) Delete(ctx context.Context, id security.RoleID) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("delete role: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		query, args, buildErr := goqu.Dialect("postgres").Delete("role").
			Where(goqu.Ex{"id": storedID.String()}).Prepared(true).ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build delete role %s query: %w", storedID, buildErr)
		} else if bindErr := bindUUIDArgs(args, [16]byte(storedID)); bindErr != nil {
			err = fmt.Errorf("bind delete role %s query: %w", storedID, bindErr)
		} else {
			tag, deleteErr := r.db.Exec(ctx, query, args...)

			if deleteErr != nil {
				var foreignKey *pgconn.PgError

				if errors.As(deleteErr, &foreignKey) && (foreignKey.Code == "23503" || foreignKey.Code == "23001") && foreignKey.ConstraintName == "user_role_id_fkey" {
					deleteErr = fmt.Errorf("%w: %w", security.ErrRoleAlreadyInUse, deleteErr)
				}

				err = fmt.Errorf("delete role %s: %w", storedID, deleteErr)
			} else if tag.RowsAffected() == 0 {
				err = fmt.Errorf("delete role %s: %w", storedID, security.ErrRoleNotFound)
			}
		}
	}

	return err
}
