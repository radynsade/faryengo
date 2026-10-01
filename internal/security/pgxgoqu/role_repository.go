package pgxgoqu

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/security"
)

type RoleRepository struct {
	db securityDB
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

func (r *RoleRepository) Save(ctx context.Context, role *security.Role) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if role == nil {
		err = ErrNilRole
	} else if _, validationErr := security.NewRole(role.ID(), role.Permissions()); validationErr != nil {
		err = fmt.Errorf("validate role: %w", validationErr)
	} else {
		id := uuid.UUID(role.ID())
		permissions := role.Permissions()
		placeholders := make([]string, len(permissions))
		values := make([]any, len(permissions))
		for index, permission := range permissions {
			placeholders[index] = "?::permission"
			values[index] = string(permission)
		}

		array := goqu.L("ARRAY["+strings.Join(placeholders, ",")+"]::permission[]", values...)
		query, args, buildErr := goqu.Dialect("postgres").
			Insert("role").
			Cols("id", "permissions").
			Vals(goqu.Vals{id.String(), array}).
			OnConflict(goqu.DoUpdate("id", goqu.Record{
				"permissions": goqu.I("excluded.permissions"),
			})).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build save role %s query: %w", id, buildErr)
		} else if bindErr := bindUUIDArgs(args, [16]byte(id)); bindErr != nil {
			err = fmt.Errorf("build save role %s query: %w", id, bindErr)
		} else {
			_, execErr := r.db.Exec(ctx, query, args...)

			if execErr != nil {
				err = fmt.Errorf("save role %s: %w", id, execErr)
			}
		}
	}

	return err
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
			Select("id", goqu.L(`"permissions"::text[]`)).
			Where(goqu.Ex{"id": storedID.String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build find role %s query: %w", storedID, buildErr)
		} else if bindErr := bindUUIDArgs(args, [16]byte(storedID)); bindErr != nil {
			err = fmt.Errorf("build find role %s query: %w", storedID, bindErr)
		} else {
			var rowID pgtype.UUID
			var rawPermissions []string
			scanErr := r.db.QueryRow(ctx, query, args...).Scan(&rowID, &rawPermissions)

			if errors.Is(scanErr, pgx.ErrNoRows) {
				err = fmt.Errorf("find role %s: %w", storedID, security.ErrRoleNotFound)
			} else if scanErr != nil {
				err = fmt.Errorf("find role %s: %w", storedID, scanErr)
			} else {
				permissions := make([]security.Permission, len(rawPermissions))
				for index, permission := range rawPermissions {
					permissions[index] = security.Permission(permission)
				}

				role, err = security.NewRole(security.RoleID(rowID.Bytes), permissions)
				if err != nil {
					err = fmt.Errorf("decode role %s: %w", storedID, err)
				}
			}
		}
	}

	return role, err
}
