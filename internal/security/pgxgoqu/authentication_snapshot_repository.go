package pgxgoqu

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// AuthenticationSnapshotRepository reads a user and its authentication version
// in one database snapshot.
type AuthenticationSnapshotRepository struct {
	pool securityDB
}

var _ security.AuthenticationSnapshotRepository = (*AuthenticationSnapshotRepository)(nil)
var _ security.AuthenticationSnapshotInvalidator = (*AuthenticationSnapshotRepository)(nil)

func NewAuthenticationSnapshotRepository(pool *pgxpool.Pool) (*AuthenticationSnapshotRepository, error) {
	var (
		repository *AuthenticationSnapshotRepository
		err        error
	)

	if pool == nil {
		err = ErrNilPool
	} else {
		repository = &AuthenticationSnapshotRepository{pool: pool}
	}

	return repository, err
}

// Find by an email

func (r *AuthenticationSnapshotRepository) FindByEmail(
	ctx context.Context,
	email security.Email,
) (*security.AuthenticationSnapshot, error) {
	var snapshot *security.AuthenticationSnapshot
	var err error

	if validationErr := email.Validate(); validationErr != nil {
		err = fmt.Errorf("find snapshot by email: %w", validationErr)
	} else {
		snapshot, err = r.find(ctx, goqu.L("lower(?) = lower(?)", goqu.I("email"), string(email)))
	}

	return snapshot, err
}

// Find by a user ID

func (r *AuthenticationSnapshotRepository) FindByUserID(
	ctx context.Context,
	id security.UserID,
) (*security.AuthenticationSnapshot, error) {
	var snapshot *security.AuthenticationSnapshot
	var err error

	if validationErr := id.Validate(); validationErr != nil {
		err = validationErr
	} else {
		snapshot, err = r.find(ctx, goqu.Ex{"id": uuid.UUID(id).String()}, uuid.UUID(id))
	}

	return snapshot, err
}

// Invalidate persists account-wide revocation before any Redis cleanup.

func (r *AuthenticationSnapshotRepository) Invalidate(
	ctx context.Context,
	id security.UserID,
) error {
	var err error

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else if id.Validate() != nil {
		err = security.ErrInvalidAuthenticationSnapshot
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Update("user").
			Set(goqu.Record{
				"authentication_snapshot_version": goqu.L("uuidv7()"),
			}).
			Where(goqu.Ex{"id": uuid.UUID(id).String()}).
			Prepared(true).
			ToSQL()

		if buildErr == nil {
			buildErr = bindUUIDArgs(args, [16]byte(id))
		}

		if buildErr != nil {
			err = fmt.Errorf("build authentication snapshot invalidation query: %w", buildErr)
		} else {
			tag, execErr := r.pool.Exec(ctx, query, args...)

			if execErr != nil {
				err = fmt.Errorf("invalidate snapshot: %w", execErr)
			} else if tag.RowsAffected() != 1 {
				err = security.ErrUserNotFound
			}
		}
	}

	return err
}

//
// Helpers
//

func (r *AuthenticationSnapshotRepository) find(
	ctx context.Context,
	predicate exp.Expression,
	ids ...uuid.UUID,
) (*security.AuthenticationSnapshot, error) {
	var snapshot *security.AuthenticationSnapshot
	var err error

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			From("user").
			Select(
				"id",
				"role_id",
				"email",
				"phone",
				"password_hash",
				"first_name",
				"last_name",
				"authentication_snapshot_version",
				"email_changed_at",
				"phone_changed_at",
				"password_changed_at",
				"updated_at",
				"created_at",
			).
			Where(predicate).
			Prepared(true).
			ToSQL()

		if buildErr == nil && len(ids) > 0 {
			buildErr = bindUUIDArgs(args, [16]byte(ids[0]))
		}

		if buildErr != nil {
			err = fmt.Errorf("build authentication snapshot query: %w", buildErr)
		} else {
			snapshot, err = scanAuthenticationSnapshot(r.pool.QueryRow(ctx, query, args...))
		}
	}

	return snapshot, err
}

func scanAuthenticationSnapshot(row pgx.Row) (*security.AuthenticationSnapshot, error) {
	var snapshot *security.AuthenticationSnapshot
	var err error
	var id, roleID, version pgtype.UUID
	var email, phone, hash, firstName, lastName string
	var emailChangedAt, phoneChangedAt, passwordChangedAt, updatedAt, createdAt time.Time

	scanErr := row.Scan(
		&id,
		&roleID,
		&email,
		&phone,
		&hash,
		&firstName,
		&lastName,
		&version,
		&emailChangedAt,
		&phoneChangedAt,
		&passwordChangedAt,
		&updatedAt,
		&createdAt,
	)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = fmt.Errorf("find snapshot: %w", security.ErrUserNotFound)
	} else if scanErr != nil {
		err = fmt.Errorf("find snapshot: %w", scanErr)
	} else if !version.Valid || uuid.UUID(version.Bytes) == uuid.Nil {
		err = fmt.Errorf("decode authentication snapshot version: %w", security.ErrInvalidAuthenticationSnapshot)
	} else {
		user := security.NewUser(
			security.UserID(id.Bytes),
			security.RoleID(roleID.Bytes),
			security.Email(email),
			emailChangedAt,
			security.Phone(phone),
			phoneChangedAt,
			security.PasswordHash(hash),
			passwordChangedAt,
			security.FirstName(firstName),
			security.LastName(lastName),
			updatedAt,
			createdAt,
		)

		userErr := user.Validate()

		if userErr != nil {
			err = fmt.Errorf("decode snapshot: %w", userErr)
		} else {
			snapshot = &security.AuthenticationSnapshot{
				User:    user,
				Version: uuid.UUID(version.Bytes),
			}
		}
	}

	return snapshot, err
}
