package pgxgoqu

import (
	"context"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/security"
)

//
// Repository
//

// PrincipalRepository projects current account and role data. A projection is
// trusted only after the caller completes session and account-version checks.
type PrincipalRepository struct {
	pool securityDB
}

var _ security.PrincipalRepository = (*PrincipalRepository)(nil)

func NewPrincipalRepository(pool *pgxpool.Pool) (*PrincipalRepository, error) {
	var (
		repository *PrincipalRepository
		err        error
	)

	if pool == nil {
		err = ErrNilPool
	} else {
		repository = &PrincipalRepository{pool: pool}
	}

	return repository, err
}

// Find by an email

func (r *PrincipalRepository) FindByEmail(
	ctx context.Context,
	email security.Email,
) (*security.Principal, error) {
	var principal *security.Principal
	var err error

	if validationErr := email.Validate(); validationErr != nil {
		err = validationErr
	} else {
		principal, err = r.find(ctx, goqu.L("lower(?) = lower(?)", goqu.I("u.email"), string(email)))
	}

	return principal, err
}

// Find by a user ID

func (r *PrincipalRepository) FindByUserID(
	ctx context.Context,
	id security.UserID,
) (*security.Principal, error) {
	var principal *security.Principal
	var err error

	if validationErr := id.Validate(); validationErr != nil {
		err = validationErr
	} else {
		principal, err = r.find(ctx, goqu.Ex{"u.id": uuid.UUID(id).String()}, uuid.UUID(id))
	}

	return principal, err
}

//
// Helpers
//

func (r *PrincipalRepository) find(
	ctx context.Context,
	predicate exp.Expression,
	ids ...uuid.UUID,
) (*security.Principal, error) {
	var principal *security.Principal
	var err error

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			From(goqu.T("user").As("u")).
			Join(goqu.T("role").As("r"), goqu.On(goqu.Ex{"u.role_id": goqu.I("r.id")})).
			Select(
				goqu.I("u.id"),
				goqu.I("r.id"),
				goqu.L("r.permissions::text[]"),
				goqu.I("r.is_super"),
				goqu.I("u.first_name"),
				goqu.I("u.last_name"),
				goqu.I("u.email"),
				goqu.I("u.phone"),
			).
			Where(predicate).
			Prepared(true).
			ToSQL()

		if buildErr == nil && len(ids) > 0 {
			buildErr = bindUUIDArgs(args, [16]byte(ids[0]))
		}

		if buildErr != nil {
			err = fmt.Errorf("build principal lookup: %w", buildErr)
		} else {
			principal, err = scanPrincipal(r.pool.QueryRow(ctx, query, args...))

			if err != nil {
				err = fmt.Errorf("load principal: %w", err)
			}
		}
	}

	return principal, err
}

func scanPrincipal(row pgx.Row) (*security.Principal, error) {
	var principal *security.Principal
	var err error
	var id, roleID pgtype.UUID
	var permissions []string
	var isSuper bool
	var firstName, lastName, email, phone string

	scanErr := row.Scan(
		&id,
		&roleID,
		&permissions,
		&isSuper,
		&firstName,
		&lastName,
		&email,
		&phone,
	)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = security.ErrUserNotFound
	} else if scanErr != nil {
		err = scanErr
	} else {
		values := make([]security.Permission, len(permissions))

		for index, value := range permissions {
			values[index] = security.Permission(value)
		}

		principal = security.NewPrincipal(
			security.UserID(id.Bytes),
			security.RoleID(roleID.Bytes),
			values,
			isSuper,
			security.FirstName(firstName),
			security.LastName(lastName),
			security.Email(email),
			security.Phone(phone),
		)

		err = principal.Validate()

		if err != nil {
			principal = nil
		}
	}

	return principal, err
}
