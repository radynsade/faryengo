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

type AuthenticationSnapshotRepository struct {
	pool pgxdb.DB
}

var (
	_ security.AuthenticationSnapshotRepository  = (*AuthenticationSnapshotRepository)(nil)
	_ security.AuthenticationSnapshotInvalidator = (*AuthenticationSnapshotRepository)(nil)
)

func NewAuthenticationSnapshotRepository(pool pgxdb.DB) (*AuthenticationSnapshotRepository, error) {
	var (
		repository *AuthenticationSnapshotRepository
		err        error
	)

	if pgxdb.IsNil(pool) {
		err = pgxdb.ErrNilDB
	} else {
		repository = &AuthenticationSnapshotRepository{pool: pool}
	}

	return repository, err
}

// Find by an email, ignoring letter case

func (r *AuthenticationSnapshotRepository) FindByEmail(
	ctx context.Context,
	email security.Email,
) (*security.AuthenticationSnapshot, error) {
	var (
		snapshot *security.AuthenticationSnapshot
		err      error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := email.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find an authentication snapshot by email: %w", validationErr)
	} else {
		snapshot, err = r.find(ctx, userEmailPredicate("email", email))

		if err != nil {
			err = fmt.Errorf("failed to find an authentication snapshot by email: %w", err)
		}
	}

	return snapshot, err
}

// Find by a user ID

func (r *AuthenticationSnapshotRepository) FindByUserID(
	ctx context.Context,
	id security.UserID,
) (*security.AuthenticationSnapshot, error) {
	var (
		snapshot *security.AuthenticationSnapshot
		err      error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find an authentication snapshot by user ID: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		snapshot, err = r.find(ctx, goqu.Ex{"id": storedID.String()})

		if err != nil {
			err = fmt.Errorf("failed to find an authentication snapshot for user %s: %w", storedID, err)
		}
	}

	return snapshot, err
}

// Invalidate rotates the snapshot version so every session holding the
// previous version stops authenticating. It persists account-wide revocation in
// PostgreSQL before any session storage cleanup.

func (r *AuthenticationSnapshotRepository) Invalidate(
	ctx context.Context,
	id security.UserID,
) error {
	var err error

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to invalidate an authentication snapshot: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)

		query, args, buildErr := goqu.Dialect("postgres").
			Update("user").
			Set(goqu.Record{"authentication_snapshot_version": goqu.L("uuidv7()")}).
			Where(goqu.Ex{"id": storedID.String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the authentication snapshot invalidation query: %w", buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = fmt.Errorf("failed to invalidate an authentication snapshot for user %s: %w", storedID, execErr)
			} else if tag.RowsAffected() == 0 {
				err = fmt.Errorf(
					"failed to invalidate an authentication snapshot for user %s: %w",
					storedID,
					security.ErrUserNotFound,
				)
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
	filter exp.Expression,
) (*security.AuthenticationSnapshot, error) {
	var (
		snapshot *security.AuthenticationSnapshot
		err      error
	)

	query, args, buildErr := goqu.Dialect("postgres").
		From("user").
		Select(append(userColumns(), "authentication_snapshot_version")...).
		Where(filter).
		Prepared(true).
		ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the authentication snapshot lookup query: %w", buildErr)
	} else {
		var (
			record  userRecord
			version pgtype.UUID
		)

		scanErr := pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...).Scan(append(record.targets(), &version)...)

		if errors.Is(scanErr, pgx.ErrNoRows) {
			err = security.ErrUserNotFound
		} else if scanErr != nil {
			err = fmt.Errorf("scan authentication snapshot: %w", scanErr)
		} else {
			var user *security.User

			user, err = record.user()

			if err == nil {
				snapshot = security.NewAuthenticationSnapshot(user, uuid.UUID(version.Bytes))
				err = snapshot.Validate()
			}

			if err != nil {
				snapshot = nil
			}
		}
	}

	return snapshot, err
}
