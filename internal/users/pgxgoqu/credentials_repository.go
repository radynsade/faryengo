package pgxgoqu

import (
	"context"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Repository
//

const credentialsVersionColumn = "credentials_version"

type CredentialsSnapshotRepository struct {
	pool pgxdb.DB
}

var _ users.CredentialsSnapshotRepository = (*CredentialsSnapshotRepository)(nil)

func NewCredentialsRepository(pool pgxdb.DB) (*CredentialsSnapshotRepository, error) {
	var (
		repository *CredentialsSnapshotRepository
		err        error
	)

	if pgxdb.IsNil(pool) {
		err = pgxdb.ErrNilDB
	} else {
		repository = &CredentialsSnapshotRepository{pool: pool}
	}

	return repository, err
}

// The user and its version come from one row in one statement, so the version
// always describes the password hash returned beside it.

func (r *CredentialsSnapshotRepository) FindByEmail(
	ctx context.Context,
	email users.Email,
) (*users.CredentialsSnapshot, error) {
	var (
		snapshot *users.CredentialsSnapshot
		err      error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := email.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find credentials by email: %w", validationErr)
	} else {
		snapshot, err = r.findByEmail(ctx, email)

		if err != nil {
			err = fmt.Errorf("failed to find credentials by email: %w", err)
		}
	}

	return snapshot, err
}

// Find a version by a user ID

func (r *CredentialsSnapshotRepository) FindVersionByUserID(
	ctx context.Context,
	id users.UserID,
) (uuid.UUID, error) {
	var (
		version uuid.UUID
		err     error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a credentials version by user ID: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		version, err = r.findVersion(ctx, storedID)

		if err != nil {
			err = fmt.Errorf("failed to find a credentials version for user %s: %w", storedID, err)
		}
	}

	return version, err
}

// The database generates the new version, so it is never chosen by the caller
// and never repeats an earlier one.

func (r *CredentialsSnapshotRepository) RotateVersion(
	ctx context.Context,
	id users.UserID,
) error {
	var err error

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to rotate a credentials version: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		err = r.rotateVersion(ctx, storedID)

		if err != nil {
			err = fmt.Errorf("failed to rotate a credentials version for user %s: %w", storedID, err)
		}
	}

	return err
}

//
// Helpers
//

func (r *CredentialsSnapshotRepository) findByEmail(
	ctx context.Context,
	email users.Email,
) (*users.CredentialsSnapshot, error) {
	var (
		snapshot *users.CredentialsSnapshot
		err      error
	)

	query, args, buildErr := goqu.Dialect("postgres").
		From("user").
		Select(append(userColumns(), credentialsVersionColumn)...).
		Where(userEmailPredicate("email", email)).
		Prepared(true).
		ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the credentials lookup query: %w", buildErr)
	} else {
		var (
			record  userRecord
			version pgtype.UUID
		)

		scanErr := pgxdb.FromContext(ctx, r.pool).
			QueryRow(ctx, query, args...).
			Scan(append(record.targets(), &version)...)

		if errors.Is(scanErr, pgx.ErrNoRows) {
			err = users.ErrUserNotFound
		} else if scanErr != nil {
			err = fmt.Errorf("scan credentials: %w", scanErr)
		} else {
			snapshot, err = credentialsSnapshot(&record, version)
		}
	}

	return snapshot, err
}

func (r *CredentialsSnapshotRepository) findVersion(
	ctx context.Context,
	storedID uuid.UUID,
) (uuid.UUID, error) {
	var (
		version uuid.UUID
		err     error
	)

	query, args, buildErr := goqu.Dialect("postgres").
		From("user").
		Select(credentialsVersionColumn).
		Where(goqu.Ex{"id": storedID.String()}).
		Prepared(true).
		ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the credentials version lookup query: %w", buildErr)
	} else {
		var stored pgtype.UUID

		scanErr := pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&stored)

		if errors.Is(scanErr, pgx.ErrNoRows) {
			err = users.ErrUserNotFound
		} else if scanErr != nil {
			err = fmt.Errorf("scan credentials version: %w", scanErr)
		} else if !stored.Valid || uuid.UUID(stored.Bytes) == uuid.Nil {
			err = users.ErrCredentialsVersionNil
		} else {
			version = uuid.UUID(stored.Bytes)
		}
	}

	return version, err
}

func (r *CredentialsSnapshotRepository) rotateVersion(
	ctx context.Context,
	storedID uuid.UUID,
) error {
	var err error

	query, args, buildErr := goqu.Dialect("postgres").
		Update("user").
		Set(goqu.Record{credentialsVersionColumn: goqu.L("uuidv7()")}).
		Where(goqu.Ex{"id": storedID.String()}).
		Prepared(true).
		ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the credentials version rotation query: %w", buildErr)
	} else {
		tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

		if execErr != nil {
			err = execErr
		} else if tag.RowsAffected() == 0 {
			err = users.ErrUserNotFound
		}
	}

	return err
}

func credentialsSnapshot(
	record *userRecord,
	version pgtype.UUID,
) (*users.CredentialsSnapshot, error) {
	var snapshot *users.CredentialsSnapshot

	user, err := record.user()

	if err == nil {
		snapshot = users.NewCredentialsSnapshot(user, uuid.UUID(version.Bytes))
		err = snapshot.Validate()
	}

	if err != nil {
		snapshot = nil
		err = fmt.Errorf("failed to decode credentials: %w", err)
	}

	return snapshot, err
}
