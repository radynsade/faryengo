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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/security"
)

//
// Errors
//

var ErrNilUser = errors.New("nil user")

type errUserWriteFailed struct {
	user *security.User
	err  error
}

func (e errUserWriteFailed) User() *security.User {
	return e.user
}

func (e errUserWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of security.ErrUserCreateFailed

type errUserCreateFailed struct {
	errUserWriteFailed
}

func newErrUserCreateFailed(user *security.User, err error) *errUserCreateFailed {
	return &errUserCreateFailed{
		errUserWriteFailed: errUserWriteFailed{user, err},
	}
}

func (e *errUserCreateFailed) Error() string {
	return "failed to create a user"
}

// Implementation of security.ErrUserUpdateFailed

type errUserUpdateFailed struct {
	errUserWriteFailed
}

func newErrUserUpdateFailed(user *security.User, err error) *errUserUpdateFailed {
	return &errUserUpdateFailed{
		errUserWriteFailed: errUserWriteFailed{user, err},
	}
}

func (e *errUserUpdateFailed) Error() string {
	return "failed to update a user"
}

// Implementation of security.ErrUserDeleteFailed

type errUserDeleteFailed struct {
	id  security.UserID
	err error
}

func newErrUserDeleteFailed(id security.UserID, err error) *errUserDeleteFailed {
	return &errUserDeleteFailed{id, err}
}

func (e errUserDeleteFailed) UserID() security.UserID {
	return e.id
}

func (e errUserDeleteFailed) Unwrap() error {
	return e.err
}

func (e *errUserDeleteFailed) Error() string {
	return "failed to delete a user"
}

//
// Repository
//

type UserRepository struct {
	pool securityDB
}

var _ security.UserRepository = (*UserRepository)(nil)

func NewUserRepository(pool *pgxpool.Pool) (*UserRepository, error) {
	var (
		repository *UserRepository
		err        error
	)

	if pool == nil {
		err = ErrNilPool
	} else {
		repository = &UserRepository{pool: pool}
	}

	return repository, err
}

// Create

func (r *UserRepository) Create(
	ctx context.Context,
	user *security.User,
) security.ErrUserCreateFailed {
	var err security.ErrUserCreateFailed

	if r == nil || r.pool == nil {
		err = newErrUserCreateFailed(user, ErrNilPool)
	} else if user == nil {
		err = newErrUserCreateFailed(user, ErrNilUser)
	} else if validationErr := user.Validate(); validationErr != nil {
		err = newErrUserCreateFailed(user, validationErr)
	} else {
		id, roleID := uuid.UUID(user.ID), uuid.UUID(user.RoleID)

		query, args, buildErr := goqu.Dialect("postgres").
			Insert("user").
			Cols("id", "role_id", "email", "phone", "password_hash", "first_name", "last_name").
			Vals(goqu.Vals{
				id.String(),
				roleID.String(),
				string(user.Email),
				string(user.Phone),
				string(user.PasswordHash),
				string(user.FirstName),
				string(user.LastName),
			}).
			OnConflict(goqu.DoNothing()).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrUserCreateFailed(user, fmt.Errorf("build create user query: %w", buildErr))
		} else if bindErr := bindUUIDArgs(args, [16]byte(id), [16]byte(roleID)); bindErr != nil {
			err = newErrUserCreateFailed(user, bindErr)
		} else {
			tag, execErr := r.pool.Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrUserCreateFailed(user, mapUserError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrUserCreateFailed(user, security.ErrUserAlreadyExists)
			}
		}
	}

	return err
}

// Update uses the loaded UpdatedAt as an optimistic concurrency token. Reload
// after a successful write; database triggers own all persisted timestamps.

func (r *UserRepository) Update(
	ctx context.Context,
	user *security.User,
) security.ErrUserUpdateFailed {
	var err security.ErrUserUpdateFailed

	if r == nil || r.pool == nil {
		err = newErrUserUpdateFailed(user, ErrNilPool)
	} else if user == nil {
		err = newErrUserUpdateFailed(user, ErrNilUser)
	} else if validationErr := user.Validate(); validationErr != nil {
		err = newErrUserUpdateFailed(user, validationErr)
	} else if user.UpdatedAt.IsZero() {
		err = newErrUserUpdateFailed(user, security.ErrUserConflict)
	} else {
		id, roleID := uuid.UUID(user.ID), uuid.UUID(user.RoleID)

		query, args, buildErr := goqu.Dialect("postgres").
			Update("user").
			Set(goqu.Record{
				"role_id":       roleID.String(),
				"email":         string(user.Email),
				"phone":         string(user.Phone),
				"password_hash": string(user.PasswordHash),
				"first_name":    string(user.FirstName),
				"last_name":     string(user.LastName),
			}).
			Where(goqu.Ex{
				"id":         id.String(),
				"updated_at": user.UpdatedAt,
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrUserUpdateFailed(user, fmt.Errorf("build update user query: %w", buildErr))
		} else if bindErr := bindUUIDArgsAt(args, len(args)-3, [16]byte(roleID), [16]byte(id)); bindErr != nil {
			err = newErrUserUpdateFailed(user, bindErr)
		} else {
			tag, execErr := r.pool.Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrUserUpdateFailed(user, mapUserError(execErr))
			} else if tag.RowsAffected() == 0 {
				_, findErr := r.FindByID(ctx, user.ID)

				if findErr != nil {
					err = newErrUserUpdateFailed(user, fmt.Errorf("check user after unsuccessful update: %w", findErr))
				} else {
					err = newErrUserUpdateFailed(user, security.ErrUserConflict)
				}
			}
		}
	}

	return err
}

// Delete

func (r *UserRepository) Delete(
	ctx context.Context,
	id security.UserID,
) security.ErrUserDeleteFailed {
	var err security.ErrUserDeleteFailed

	if r == nil || r.pool == nil {
		err = newErrUserDeleteFailed(id, ErrNilPool)
	} else if validationErr := id.Validate(); validationErr != nil {
		err = newErrUserDeleteFailed(id, validationErr)
	} else {
		rawID := uuid.UUID(id)

		query, args, buildErr := goqu.Dialect("postgres").
			Delete("user").
			Where(goqu.Ex{"id": rawID.String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrUserDeleteFailed(id, fmt.Errorf("build delete user query: %w", buildErr))
		} else if bindErr := bindUUIDArgs(args, [16]byte(rawID)); bindErr != nil {
			err = newErrUserDeleteFailed(id, bindErr)
		} else {
			tag, execErr := r.pool.Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrUserDeleteFailed(id, execErr)
			} else if tag.RowsAffected() == 0 {
				err = newErrUserDeleteFailed(id, security.ErrUserNotFound)
			}
		}
	}

	return err
}

// Find by an ID

func (r *UserRepository) FindByID(
	ctx context.Context,
	id security.UserID,
) (*security.User, error) {
	var user *security.User
	var err error

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else if validationErr := id.Validate(); validationErr != nil {
		err = validationErr
	} else {
		user, err = r.find(ctx, goqu.Ex{"id": uuid.UUID(id).String()}, uuid.UUID(id))
	}

	return user, err
}

// Find by an email

func (r *UserRepository) FindByEmail(
	ctx context.Context,
	email security.Email,
) (*security.User, error) {
	var user *security.User
	var err error

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else if validationErr := email.Validate(); validationErr != nil {
		err = validationErr
	} else {
		user, err = r.find(ctx, goqu.L("lower(?) = lower(?)", goqu.I("email"), string(email)))
	}

	return user, err
}

//
// Helpers
//

func (r *UserRepository) find(
	ctx context.Context,
	predicate exp.Expression,
	ids ...uuid.UUID,
) (*security.User, error) {
	var user *security.User

	query, args, err := goqu.Dialect("postgres").
		From("user").
		Select(userColumns()...).
		Where(predicate).
		Prepared(true).
		ToSQL()

	if err == nil && len(ids) > 0 {
		err = bindUUIDArgs(args, [16]byte(ids[0]))
	}

	if err == nil {
		user, err = scanUser(r.pool.QueryRow(ctx, query, args...))
	}

	if err != nil {
		err = fmt.Errorf("find user: %w", err)
	}

	return user, err
}

func userColumns() []any {
	return []any{
		"id",
		"role_id",
		"email",
		"phone",
		"password_hash",
		"first_name",
		"last_name",
		"email_changed_at",
		"phone_changed_at",
		"password_changed_at",
		"updated_at",
		"created_at",
	}
}

func scanUser(row pgx.Row) (*security.User, error) {
	var user *security.User
	var err error
	var id, roleID pgtype.UUID
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
		&emailChangedAt,
		&phoneChangedAt,
		&passwordChangedAt,
		&updatedAt,
		&createdAt,
	)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = security.ErrUserNotFound
	} else if scanErr != nil {
		err = scanErr
	} else {
		user = security.NewUser(
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

		err = user.Validate()

		if err != nil {
			user = nil
		}
	}

	return user, err
}

func mapUserError(err error) error {
	result := err

	if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "user_role_id_fkey":
			result = errors.Join(security.ErrRoleNotFound, err)
		case postgresErr.Code == "23505" && (postgresErr.ConstraintName == "user_email_unique_idx" || postgresErr.ConstraintName == "user_pkey"):
			result = errors.Join(security.ErrUserAlreadyExists, err)
		}
	}

	return result
}
