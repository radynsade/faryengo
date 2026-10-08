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
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/security"
)

//
// Repository
//

type PrincipalRepository struct {
	pool pgxdb.DB
}

var _ security.PrincipalRepository = (*PrincipalRepository)(nil)

func NewPrincipalRepository(pool pgxdb.DB) (*PrincipalRepository, error) {
	var (
		repository *PrincipalRepository
		err        error
	)

	if pgxdb.IsNil(pool) {
		err = pgxdb.ErrNilDB
	} else {
		repository = &PrincipalRepository{pool: pool}
	}

	return repository, err
}

// Find by an email, ignoring letter case

func (r *PrincipalRepository) FindByEmail(
	ctx context.Context,
	email security.Email,
) (*security.Principal, error) {
	var (
		principal *security.Principal
		err       error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := email.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a principal by email: %w", validationErr)
	} else {
		principal, err = r.find(ctx, userEmailPredicate("u.email", email))

		if err != nil {
			err = fmt.Errorf("failed to find a principal by email: %w", err)
		}
	}

	return principal, err
}

// Find by a user ID

func (r *PrincipalRepository) FindByUserID(
	ctx context.Context,
	id security.UserID,
) (*security.Principal, error) {
	var (
		principal *security.Principal
		err       error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a principal by user ID: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		principal, err = r.find(ctx, goqu.Ex{"u.id": storedID.String()})

		if err != nil {
			err = fmt.Errorf("failed to find a principal for user %s: %w", storedID, err)
		}
	}

	return principal, err
}

//
// Helpers
//

func (r *PrincipalRepository) find(
	ctx context.Context,
	filter exp.Expression,
) (*security.Principal, error) {
	var (
		principal *security.Principal
		err       error
	)

	query, args, buildErr := goqu.Dialect("postgres").
		From(goqu.T("user").As("u")).
		Join(goqu.T("role").As("r"), goqu.On(goqu.Ex{"u.role_id": goqu.I("r.id")})).
		Select(principalColumns()...).
		Where(filter).
		Prepared(true).
		ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the principal lookup query: %w", buildErr)
	} else {
		principal, err = scanPrincipal(pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...))
	}

	return principal, err
}

func principalColumns() []any {
	return []any{
		goqu.I("u.id"),
		goqu.I("r.id"),
		goqu.L(`"r".permissions::text[]`),
		goqu.I("r.is_super"),
		goqu.I("u.first_name"),
		goqu.I("u.last_name"),
		goqu.I("u.email"),
		goqu.I("u.phone"),
	}
}

func scanPrincipal(row pgx.Row) (*security.Principal, error) {
	var (
		principal                         *security.Principal
		err                               error
		id, roleID                        pgtype.UUID
		permissions                       []string
		isSuper                           bool
		firstName, lastName, email, phone string
	)

	scanErr := row.Scan(&id, &roleID, &permissions, &isSuper, &firstName, &lastName, &email, &phone)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = security.ErrUserNotFound
	} else if scanErr != nil {
		err = fmt.Errorf("scan principal: %w", scanErr)
	} else {
		principalPermissions := make(security.Permissions, len(permissions))

		for index, permission := range permissions {
			principalPermissions[index] = security.Permission(permission)
		}

		principal = security.NewPrincipal(
			security.UserID(id.Bytes),
			security.RoleID(roleID.Bytes),
			principalPermissions,
			isSuper,
			security.FirstName(firstName),
			security.LastName(lastName),
			security.Email(email),
			security.Phone(phone),
		)

		err = principal.Validate()

		if err != nil {
			err = fmt.Errorf("failed to decode a principal: %w", err)
			principal = nil
		}
	}

	return principal, err
}
