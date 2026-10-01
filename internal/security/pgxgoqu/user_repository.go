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

func (r *UserRepository) Save(ctx context.Context, user *security.User) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if user == nil {
		err = ErrNilUser
	} else if _, validationErr := security.NewUser(user.ID(), user.RoleID(), user.Email(), user.Phone(), user.PasswordHash(), user.FirstName(), user.LastName()); validationErr != nil {
		err = fmt.Errorf("validate user: %w", validationErr)
	} else {
		id := uuid.UUID(user.ID())
		roleID := uuid.UUID(user.RoleID())
		query, args, buildErr := goqu.Dialect("postgres").
			Insert("user").
			Cols("id", "role_id", "email", "phone", "password_hash", "first_name", "last_name").
			Vals(goqu.Vals{
				id.String(),
				roleID.String(),
				string(user.Email()),
				string(user.Phone()),
				string(user.PasswordHash()),
				string(user.FirstName()),
				string(user.LastName()),
			}).
			OnConflict(goqu.DoUpdate("id", goqu.Record{
				"role_id":       goqu.I("excluded.role_id"),
				"email":         goqu.I("excluded.email"),
				"phone":         goqu.I("excluded.phone"),
				"password_hash": goqu.I("excluded.password_hash"),
				"first_name":    goqu.I("excluded.first_name"),
				"last_name":     goqu.I("excluded.last_name"),
			})).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build save user %s query: %w", id, buildErr)
		} else if bindErr := bindUUIDArgs(args, [16]byte(id), [16]byte(roleID)); bindErr != nil {
			err = fmt.Errorf("build save user %s query: %w", id, bindErr)
		} else {
			_, execErr := r.db.Exec(ctx, query, args...)

			if execErr != nil {
				err = fmt.Errorf("save user %s: %w", id, execErr)
			}
		}
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
