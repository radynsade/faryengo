package pgxgoqu

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Errors
//

type errUserWriteFailed struct {
	user *users.User
	err  error
}

func (e errUserWriteFailed) User() *users.User {
	return e.user
}

func (e errUserWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of users.ErrUserCreateFailed

type errUserCreateFailed struct {
	errUserWriteFailed
}

func newErrUserCreateFailed(user *users.User, err error) *errUserCreateFailed {
	return &errUserCreateFailed{
		errUserWriteFailed: errUserWriteFailed{user, err},
	}
}

func (e *errUserCreateFailed) Error() string {
	return "failed to create a user"
}

// Implementation of users.ErrUserUpdateFailed

type errUserUpdateFailed struct {
	errUserWriteFailed
}

func newErrUserUpdateFailed(user *users.User, err error) *errUserUpdateFailed {
	return &errUserUpdateFailed{
		errUserWriteFailed: errUserWriteFailed{user, err},
	}
}

func (e *errUserUpdateFailed) Error() string {
	return "failed to update a user"
}

// Implementation of users.ErrUserDeleteFailed

type errUserDeleteFailed struct {
	id  users.UserID
	err error
}

func newErrUserDeleteFailed(id users.UserID, err error) *errUserDeleteFailed {
	return &errUserDeleteFailed{id, err}
}

func (e errUserDeleteFailed) UserID() users.UserID {
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
	pool pgxdb.DB
}

var _ users.UserRepository = (*UserRepository)(nil)

func NewUserRepository(pool pgxdb.DB) (*UserRepository, error) {
	var (
		repository *UserRepository
		err        error
	)

	if pgxdb.IsNil(pool) {
		err = pgxdb.ErrNilDB
	} else {
		repository = &UserRepository{pool: pool}
	}

	return repository, err
}

// Create leaves every timestamp to the database defaults.

func (r *UserRepository) Create(
	ctx context.Context,
	user *users.User,
) users.ErrUserCreateFailed {
	var err users.ErrUserCreateFailed

	if r == nil || r.pool == nil {
		err = newErrUserCreateFailed(user, pgxdb.ErrNilDB)
	} else if user == nil {
		err = newErrUserCreateFailed(user, users.ErrUserNil)
	} else if validationErr := user.Validate(); validationErr != nil {
		err = newErrUserCreateFailed(user, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Insert("user").
			Rows(goqu.Record{
				"id":            uuid.UUID(user.ID).String(),
				"role_id":       uuid.UUID(user.RoleID).String(),
				"email":         string(user.Email),
				"phone":         string(user.Phone),
				"password_hash": string(user.PasswordHash),
				"first_name":    string(user.FirstName),
				"last_name":     string(user.LastName),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrUserCreateFailed(user, buildErr)
		} else {
			_, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrUserCreateFailed(user, mapUserError(execErr))
			}
		}
	}

	return err
}

// Update uses the loaded UpdatedAt as an optimistic concurrency token. Database
// triggers own every persisted timestamp, so reload the user after a write.

func (r *UserRepository) Update(
	ctx context.Context,
	user *users.User,
) users.ErrUserUpdateFailed {
	var err users.ErrUserUpdateFailed

	if r == nil || r.pool == nil {
		err = newErrUserUpdateFailed(user, pgxdb.ErrNilDB)
	} else if user == nil {
		err = newErrUserUpdateFailed(user, users.ErrUserNil)
	} else if validationErr := user.Validate(); validationErr != nil {
		err = newErrUserUpdateFailed(user, validationErr)
	} else if user.UpdatedAt.IsZero() {
		err = newErrUserUpdateFailed(user, users.ErrUserConflict)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Update("user").
			Set(goqu.Record{
				"role_id":       uuid.UUID(user.RoleID).String(),
				"email":         string(user.Email),
				"phone":         string(user.Phone),
				"password_hash": string(user.PasswordHash),
				"first_name":    string(user.FirstName),
				"last_name":     string(user.LastName),
			}).
			Where(goqu.Ex{
				"id":         uuid.UUID(user.ID).String(),
				"updated_at": user.UpdatedAt,
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrUserUpdateFailed(user, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrUserUpdateFailed(user, mapUserError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrUserUpdateFailed(user, r.explainMissedUpdate(ctx, user.ID))
			}
		}
	}

	return err
}

// Delete

func (r *UserRepository) Delete(
	ctx context.Context,
	id users.UserID,
) users.ErrUserDeleteFailed {
	var err users.ErrUserDeleteFailed

	if r == nil || r.pool == nil {
		err = newErrUserDeleteFailed(id, pgxdb.ErrNilDB)
	} else if validationErr := id.Validate(); validationErr != nil {
		err = newErrUserDeleteFailed(id, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Delete("user").
			Where(goqu.Ex{"id": uuid.UUID(id).String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrUserDeleteFailed(id, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrUserDeleteFailed(id, mapUserError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrUserDeleteFailed(id, users.ErrUserNotFound)
			}
		}
	}

	return err
}

// Find by an ID

func (r *UserRepository) FindByID(
	ctx context.Context,
	id users.UserID,
) (*users.User, error) {
	var (
		user *users.User
		err  error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a user by ID: %w", validationErr)
	} else {
		storedID := uuid.UUID(id)
		user, err = r.find(ctx, goqu.Ex{"id": storedID.String()})

		if err != nil {
			err = fmt.Errorf("failed to find a user %s: %w", storedID, err)
		}
	}

	return user, err
}

// Find by an email, ignoring letter case

func (r *UserRepository) FindByEmail(
	ctx context.Context,
	email users.Email,
) (*users.User, error) {
	var (
		user *users.User
		err  error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := email.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a user by email: %w", validationErr)
	} else {
		user, err = r.find(ctx, userEmailPredicate("email", email))

		if err != nil {
			err = fmt.Errorf("failed to find a user by email: %w", err)
		}
	}

	return user, err
}

//
// Helpers
//

func (r *UserRepository) find(
	ctx context.Context,
	filter exp.Expression,
) (*users.User, error) {
	var (
		user *users.User
		err  error
	)

	query, args, buildErr := goqu.Dialect("postgres").
		From("user").
		Select(userColumns()...).
		Where(filter).
		Prepared(true).
		ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the user lookup query: %w", buildErr)
	} else {
		var record userRecord

		scanErr := pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...).Scan(record.targets()...)

		if errors.Is(scanErr, pgx.ErrNoRows) {
			err = users.ErrUserNotFound
		} else if scanErr != nil {
			err = fmt.Errorf("scan user: %w", scanErr)
		} else {
			user, err = record.user()
		}
	}

	return user, err
}

// An update that matched no row either targeted a missing user or carried an
// outdated UpdatedAt; only a second lookup can tell the two apart.

func (r *UserRepository) explainMissedUpdate(
	ctx context.Context,
	id users.UserID,
) error {
	_, err := r.find(ctx, goqu.Ex{"id": uuid.UUID(id).String()})

	if err == nil {
		err = users.ErrUserConflict
	} else if !errors.Is(err, users.ErrUserNotFound) {
		err = fmt.Errorf("check the user after a missed update: %w", err)
	}

	return err
}

func userColumns() []any {
	return []any{
		"id",
		"role_id",
		"email",
		"email_changed_at",
		"phone",
		"phone_changed_at",
		"password_hash",
		"password_changed_at",
		"first_name",
		"last_name",
		"updated_at",
		"created_at",
	}
}

func userEmailPredicate(column string, email users.Email) exp.Expression {
	return goqu.L("lower(?) = lower(?)", goqu.I(column), string(email))
}

type userRecord struct {
	id, roleID                                                              pgtype.UUID
	email, phone, passwordHash, firstName, lastName                         string
	emailChangedAt, phoneChangedAt, passwordChangedAt, updatedAt, createdAt time.Time
}

func (u *userRecord) targets() []any {
	return []any{
		&u.id,
		&u.roleID,
		&u.email,
		&u.emailChangedAt,
		&u.phone,
		&u.phoneChangedAt,
		&u.passwordHash,
		&u.passwordChangedAt,
		&u.firstName,
		&u.lastName,
		&u.updatedAt,
		&u.createdAt,
	}
}

func (u *userRecord) user() (*users.User, error) {
	user := users.NewUser(
		users.UserID(u.id.Bytes),
		users.RoleID(u.roleID.Bytes),
		users.Email(u.email),
		u.emailChangedAt,
		users.Phone(u.phone),
		u.phoneChangedAt,
		users.PasswordHash(u.passwordHash),
		u.passwordChangedAt,
		users.FirstName(u.firstName),
		users.LastName(u.lastName),
		u.updatedAt,
		u.createdAt,
	)

	err := user.Validate()

	if err != nil {
		err = fmt.Errorf("failed to decode a user: %w", err)
		user = nil
	}

	return user, err
}

func mapUserError(err error) error {
	result := err

	if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "user_role_id_fkey":
			result = errors.Join(users.ErrRoleNotFound, err)
		case postgresErr.Code == "23505" && postgresErr.ConstraintName == "user_pkey":
			result = errors.Join(users.ErrUserAlreadyExists, err)
		case postgresErr.Code == "23505" && postgresErr.ConstraintName == "user_email_unique_idx":
			result = errors.Join(users.ErrUserAlreadyExists, err)
		}
	}

	return result
}
