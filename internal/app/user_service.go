package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/users"
)

//
// User service
//

// Every write runs in a transaction, together with the reload that returns
// the database-owned timestamps, so a failed reload undoes the write. A
// password is hashed before the transaction begins, to keep it short.

var (
	ErrUserRepositoryNil = errors.New("user repository is nil")
	ErrPasswordHasherNil = errors.New("password hasher is nil")
)

type UserService struct {
	transactor Transactor
	repository users.UserRepository
	hasher     users.PasswordHasher
}

func NewUserService(
	transactor Transactor,
	repository users.UserRepository,
	hasher users.PasswordHasher,
) (*UserService, error) {
	var (
		service *UserService
		err     error
	)

	if transactor == nil {
		err = ErrTransactorNil
	} else if repository == nil {
		err = ErrUserRepositoryNil
	} else if hasher == nil {
		err = ErrPasswordHasherNil
	} else {
		service = &UserService{transactor: transactor, repository: repository, hasher: hasher}
	}

	return service, err
}

// Create

func (s *UserService) Create(
	ctx context.Context,
	request input.CreateUserInput,
) (*users.User, error) {
	var (
		user *users.User
		hash users.PasswordHash
		err  error
	)

	if err = s.check(); err == nil {
		err = errors.Join(
			request.RoleID.Validate(),
			users.Email(request.Email).Validate(),
			users.Phone(request.Phone).Validate(),
			users.Password(request.Password).Validate(),
			users.FirstName(request.FirstName).Validate(),
			users.LastName(request.LastName).Validate(),
		)

		if err != nil {
			err = fmt.Errorf("create user: %w: %w", input.ErrCreateUserInputInvalid, err)
		}
	}

	if err == nil {
		var id uuid.UUID

		id, err = uuid.NewV7()

		if err != nil {
			err = fmt.Errorf("generate user ID: %w", err)
		} else {
			hash, err = s.hasher.Hash(ctx, request.Password)

			if err != nil {
				err = fmt.Errorf("hash password of user %s: %w", id, err)
			}
		}

		if err == nil {
			user = users.NewUser(
				users.UserID(id),
				request.RoleID,
				users.Email(request.Email),
				time.Time{},
				users.Phone(request.Phone),
				time.Time{},
				hash,
				time.Time{},
				users.FirstName(request.FirstName),
				users.LastName(request.LastName),
				time.Time{},
				time.Time{},
			)

			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				var err error

				if createErr := s.repository.Create(ctx, user); createErr != nil {
					err = createErr
				} else {
					user, err = s.reload(ctx, user.ID)
				}

				return err
			})

			if err != nil {
				err = fmt.Errorf("create user %s: %w", id, err)
			}
		}
	}

	if err != nil {
		user = nil
	}

	return user, err
}

// Find by an ID

func (s *UserService) FindByID(ctx context.Context, id users.UserID) (*users.User, error) {
	var (
		user *users.User
		err  error
	)

	if s == nil || s.repository == nil {
		err = ErrUserRepositoryNil
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("find user: %w", validationErr)
	} else {
		user, err = s.repository.FindByID(ctx, id)

		if err != nil {
			err = fmt.Errorf("find user %s: %w", uuid.UUID(id), err)
			user = nil
		}
	}

	return user, err
}

// The update replaces the User's account details and Role, and its password
// only when a new one is given. The request's UpdatedAt guards against
// overwriting a newer change, which the repository reports as
// ErrUserConflict.

func (s *UserService) Update(
	ctx context.Context,
	request input.UpdateUserInput,
) (*users.User, error) {
	var (
		user *users.User
		hash users.PasswordHash
		err  error
	)

	if err = s.check(); err == nil {
		var passwordErr error

		if request.Password != nil {
			passwordErr = users.Password(*request.Password).Validate()
		}

		err = errors.Join(
			request.ID.Validate(),
			request.RoleID.Validate(),
			users.Email(request.Email).Validate(),
			users.Phone(request.Phone).Validate(),
			passwordErr,
			users.FirstName(request.FirstName).Validate(),
			users.LastName(request.LastName).Validate(),
		)

		if err != nil {
			err = fmt.Errorf("update user: %w: %w", input.ErrUpdateUserInputInvalid, err)
		}
	}

	if err == nil && request.Password != nil {
		hash, err = s.hasher.Hash(ctx, *request.Password)

		if err != nil {
			err = fmt.Errorf("hash password of user %s: %w", uuid.UUID(request.ID), err)
		}
	}

	if err == nil {
		err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
			existing, err := s.repository.FindByID(ctx, request.ID)

			if err == nil {
				if request.Password == nil {
					hash = existing.PasswordHash
				}

				user = users.NewUser(
					existing.ID,
					request.RoleID,
					users.Email(request.Email),
					existing.EmailChangedAt,
					users.Phone(request.Phone),
					existing.PhoneChangedAt,
					hash,
					existing.PasswordChangedAt,
					users.FirstName(request.FirstName),
					users.LastName(request.LastName),
					request.UpdatedAt,
					existing.CreatedAt,
				)

				if updateErr := s.repository.Update(ctx, user); updateErr != nil {
					err = updateErr
				} else {
					user, err = s.reload(ctx, user.ID)
				}
			}

			return err
		})

		if err != nil {
			err = fmt.Errorf("update user %s: %w", uuid.UUID(request.ID), err)
		}
	}

	if err != nil {
		user = nil
	}

	return user, err
}

// Delete

func (s *UserService) Delete(ctx context.Context, id users.UserID) error {
	err := s.check()

	if err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("delete user: %w", err)
		}
	}

	if err == nil {
		err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
			var err error

			if deleteErr := s.repository.Delete(ctx, id); deleteErr != nil {
				err = deleteErr
			}

			return err
		})

		if err != nil {
			err = fmt.Errorf("delete user %s: %w", uuid.UUID(id), err)
		}
	}

	return err
}

//
// Helpers
//

func (s *UserService) check() error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrUserRepositoryNil
	} else if s.transactor == nil {
		err = ErrTransactorNil
	} else if s.hasher == nil {
		err = ErrPasswordHasherNil
	}

	return err
}

func (s *UserService) reload(ctx context.Context, id users.UserID) (*users.User, error) {
	user, err := s.repository.FindByID(ctx, id)

	if err != nil {
		err = fmt.Errorf("reload user %s: %w", uuid.UUID(id), err)
		user = nil
	}

	return user, err
}
