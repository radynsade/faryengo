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

// CredentialRepository reads user credentials and their version in one snapshot.
type CredentialRepository struct{ db securityDB }

func NewCredentialRepository(pool *pgxpool.Pool) (*CredentialRepository, error) {
	var repository *CredentialRepository
	var err error

	if pool == nil {
		err = ErrNilPool
	} else {
		repository = &CredentialRepository{db: pool}
	}

	return repository, err
}

func (r *CredentialRepository) FindByEmail(ctx context.Context, email security.Email) (*security.Credentials, error) {
	var credentials *security.Credentials
	var err error

	if validationErr := email.Validate(); validationErr != nil {
		err = fmt.Errorf("find credentials by email: %w", validationErr)
	} else {
		credentials, err = r.find(ctx, goqu.L("lower(?) = lower(?)", goqu.I("email"), string(email)))
	}

	return credentials, err
}

func (r *CredentialRepository) FindByUserID(ctx context.Context, id security.UserID) (*security.Credentials, error) {
	return r.find(ctx, goqu.Ex{"id": uuid.UUID(id).String()}, uuid.UUID(id))
}

func (r *CredentialRepository) find(ctx context.Context, predicate exp.Expression, ids ...uuid.UUID) (*security.Credentials, error) {
	var credentials *security.Credentials
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else {
		query, args, buildErr := goqu.Dialect("postgres").From("user").
			Select("id", "role_id", "email", "phone", "password_hash", "first_name", "last_name", "credential_version").
			Where(predicate).Prepared(true).ToSQL()

		if buildErr == nil && len(ids) > 0 {
			buildErr = bindUUIDArgs(args, [16]byte(ids[0]))
		}

		if buildErr != nil {
			err = fmt.Errorf("build credential query: %w", buildErr)
		} else {
			var id, roleID, version pgtype.UUID
			var email, phone, hash, firstName, lastName string
			scanErr := r.db.QueryRow(ctx, query, args...).Scan(&id, &roleID, &email, &phone, &hash, &firstName, &lastName, &version)

			if errors.Is(scanErr, pgx.ErrNoRows) {
				err = fmt.Errorf("find credentials: %w", security.ErrUserNotFound)
			} else if scanErr != nil {
				err = fmt.Errorf("find credentials: %w", scanErr)
			} else if !version.Valid || uuid.UUID(version.Bytes) == uuid.Nil {
				err = fmt.Errorf("decode credential version: %w", security.ErrInvalidSession)
			} else {
				user, userErr := security.NewUser(security.UserID(id.Bytes), security.RoleID(roleID.Bytes), security.Email(email), security.Phone(phone), security.PasswordHash(hash), security.FirstName(firstName), security.LastName(lastName))

				if userErr != nil {
					err = fmt.Errorf("decode credentials: %w", userErr)
				} else {
					credentials = &security.Credentials{User: user, Version: uuid.UUID(version.Bytes)}
				}
			}
		}
	}

	return credentials, err
}

// Invalidate persists account-wide revocation before any Redis cleanup.
func (r *CredentialRepository) Invalidate(ctx context.Context, id security.UserID) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if id.Validate() != nil {
		err = security.ErrInvalidSession
	} else {
		query, args, buildErr := goqu.Dialect("postgres").Update("user").
			Set(goqu.Record{"credential_version": goqu.L("uuidv7()")}).
			Where(goqu.Ex{"id": uuid.UUID(id).String()}).Prepared(true).ToSQL()

		if buildErr == nil {
			buildErr = bindUUIDArgs(args, [16]byte(id))
		}

		if buildErr != nil {
			err = fmt.Errorf("build credential invalidation query: %w", buildErr)
		} else {
			tag, execErr := r.db.Exec(ctx, query, args...)

			if execErr != nil {
				err = fmt.Errorf("invalidate credentials: %w", execErr)
			} else if tag.RowsAffected() != 1 {
				err = security.ErrUserNotFound
			}
		}
	}

	return err
}
