package pgxgoqu

import (
	"context"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/security"
)

type UserRepository struct {
	db securityDB
}

var _ security.UserRepository = (*UserRepository)(nil)

func NewUserRepository(pool *pgxpool.Pool) (*UserRepository, error) {
	var repository *UserRepository
	var err error

	if pool == nil {
		err = ErrNilPool
	} else {
		repository = &UserRepository{db: pool}
	}

	return repository, err
}

func (r *UserRepository) Create(ctx context.Context, user *security.User) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if user == nil {
		err = ErrNilUser
	} else if validationErr := validateUser(user); validationErr != nil {
		err = validationErr
	} else {
		id := uuid.UUID(user.ID())
		roleID := uuid.UUID(user.RoleID())
		query, args, buildErr := goqu.Dialect("postgres").
			Insert("user").
			Cols("id", "role_id", "email", "phone", "password_hash", "first_name", "last_name").
			Vals(goqu.Vals{
				id.String(), roleID.String(), string(user.Email()), string(user.Phone()),
				string(user.PasswordHash()), string(user.FirstName()), string(user.LastName()),
			}).
			OnConflict(goqu.DoNothing()).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build create user %s query: %w", id, buildErr)
		} else if bindErr := bindUUIDArgs(args, [16]byte(id), [16]byte(roleID)); bindErr != nil {
			err = fmt.Errorf("build create user %s query: %w", id, bindErr)
		} else {
			tag, execErr := r.db.Exec(ctx, query, args...)

			if execErr != nil {
				err = fmt.Errorf("create user %s: %w", id, execErr)
			} else if tag.RowsAffected() == 0 {
				err = fmt.Errorf("create user %s: %w", id, security.ErrUserAlreadyExists)
			}
		}
	}

	return err
}

func (r *UserRepository) Update(ctx context.Context, user *security.User) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if user == nil {
		err = ErrNilUser
	} else if validationErr := validateUser(user); validationErr != nil {
		err = validationErr
	} else {
		id := uuid.UUID(user.ID())
		roleID := uuid.UUID(user.RoleID())
		query, args, buildErr := goqu.Dialect("postgres").
			Update("user").
			Set(goqu.Record{
				"role_id":       roleID.String(),
				"email":         string(user.Email()),
				"phone":         string(user.Phone()),
				"password_hash": string(user.PasswordHash()),
				"first_name":    string(user.FirstName()),
				"last_name":     string(user.LastName()),
			}).
			Where(goqu.Ex{"id": id.String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build update user %s query: %w", id, buildErr)
		} else if bindErr := bindUUIDArgsAt(args, len(args)-2, [16]byte(roleID), [16]byte(id)); bindErr != nil {
			err = fmt.Errorf("bind update user %s query: %w", id, bindErr)
		} else {
			tag, execErr := r.db.Exec(ctx, query, args...)

			if execErr != nil {
				err = fmt.Errorf("update user %s: %w", id, execErr)
			} else if tag.RowsAffected() == 0 {
				err = fmt.Errorf("update user %s: %w", id, security.ErrUserNotFound)
			}
		}
	}

	return err
}

func (r *UserRepository) Delete(ctx context.Context, id security.UserID) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else {
		userID := uuid.UUID(id)
		query, args, buildErr := goqu.Dialect("postgres").
			Delete("user").
			Where(goqu.Ex{"id": userID.String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build delete user %s query: %w", userID, buildErr)
		} else if bindErr := bindUUIDArgs(args, [16]byte(userID)); bindErr != nil {
			err = fmt.Errorf("bind delete user %s query: %w", userID, bindErr)
		} else {
			tag, execErr := r.db.Exec(ctx, query, args...)

			if execErr != nil {
				err = fmt.Errorf("delete user %s: %w", userID, execErr)
			} else if tag.RowsAffected() == 0 {
				err = fmt.Errorf("delete user %s: %w", userID, security.ErrUserNotFound)
			}
		}
	}

	return err
}

func validateUser(user *security.User) error {
	_, err := security.NewUser(user.ID(), user.RoleID(), user.Email(), user.Phone(), user.PasswordHash(), user.FirstName(), user.LastName())
	if err != nil {
		err = fmt.Errorf("validate user: %w", err)
	}

	return err
}

func (r *UserRepository) FindByID(ctx context.Context, id security.UserID) (*security.User, error) {
	var user *security.User
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else {
		requestedID := uuid.UUID(id)
		query, args, buildErr := goqu.Dialect("postgres").
			From("user").
			Select("id", "role_id", "email", "phone", "password_hash", "first_name", "last_name").
			Where(goqu.Ex{"id": requestedID.String()}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build find user %s query: %w", requestedID, buildErr)
		} else if bindErr := bindUUIDArgs(args, [16]byte(requestedID)); bindErr != nil {
			err = fmt.Errorf("build find user %s query: %w", requestedID, bindErr)
		} else {
			var rowID pgtype.UUID
			var rowRoleID pgtype.UUID
			var email string
			var phone string
			var passwordHash string
			var firstName string
			var lastName string
			scanErr := r.db.QueryRow(ctx, query, args...).Scan(
				&rowID, &rowRoleID, &email, &phone, &passwordHash, &firstName, &lastName,
			)

			if errors.Is(scanErr, pgx.ErrNoRows) {
				err = fmt.Errorf("find user %s: %w", requestedID, security.ErrUserNotFound)
			} else if scanErr != nil {
				err = fmt.Errorf("find user %s: %w", requestedID, scanErr)
			} else {
				user, err = security.NewUser(
					security.UserID(rowID.Bytes),
					security.RoleID(rowRoleID.Bytes),
					security.Email(email),
					security.Phone(phone),
					security.PasswordHash(passwordHash),
					security.FirstName(firstName),
					security.LastName(lastName),
				)

				if err != nil {
					err = fmt.Errorf("decode user %s: %w", requestedID, err)
				}
			}
		}
	}

	return user, err
}
