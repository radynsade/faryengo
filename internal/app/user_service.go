package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
)

var (
	ErrNilUserRepository = errors.New("nil user repository")
	ErrNilPasswordHasher = errors.New("nil password hasher")
)

type UserService struct {
	repository security.UserRepository
	hasher     security.PasswordHasher
}

func NewUserService(repository security.UserRepository, hasher security.PasswordHasher) (*UserService, error) {
	var service *UserService
	var err error

	if repository == nil {
		err = ErrNilUserRepository
	} else if hasher == nil {
		err = ErrNilPasswordHasher
	} else {
		service = &UserService{repository: repository, hasher: hasher}
	}

	return service, err
}

func (s *UserService) Create(ctx context.Context, request input.CreateUserInput) (*security.User, error) {
	var user *security.User
	var values userValues
	var password security.Password
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilUserRepository
	} else if s.hasher == nil {
		err = ErrNilPasswordHasher
	} else {
		values, err = userValuesFromInput(request.RoleID, request.Email, request.Phone, request.FirstName, request.LastName)
		var passwordErr error
		password, passwordErr = security.NewRegistrationPassword(request.Password)
		err = errors.Join(err, passwordErr)

		if err != nil {
			err = fmt.Errorf("create user values: %w: %w", input.ErrInvalidCreateUserInput, err)
		}
	}

	if err == nil {
		var id uuid.UUID
		id, err = uuid.NewV7()

		if err != nil {
			err = fmt.Errorf("generate user ID: %w", err)
		} else {
			var hash security.PasswordHash
			hash, err = s.hasher.Hash(ctx, password.Value())
			if err != nil {
				err = fmt.Errorf("hash user password: %w", err)
			} else {
				user, err = security.NewUser(
					security.UserID(id), request.RoleID, values.email,
					values.phone, hash, values.firstName,
					values.lastName,
				)
				if err != nil {
					err = fmt.Errorf("create user: %w", err)
				} else if createErr := s.repository.Create(ctx, user); createErr != nil {
					user = nil
					err = fmt.Errorf("create user %s: %w", id, createErr)
				}
			}
		}
	}

	return user, err
}

func (s *UserService) Update(ctx context.Context, request input.UpdateUserInput) (*security.User, error) {
	var user *security.User
	var values userValues
	var password security.Password
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilUserRepository
	} else if s.hasher == nil {
		err = ErrNilPasswordHasher
	} else {
		values, err = userValuesFromInput(request.RoleID, request.Email, request.Phone, request.FirstName, request.LastName)
		err = errors.Join(err, request.ID.Validate())

		if request.Password != nil {
			var passwordErr error
			password, passwordErr = security.NewPassword(*request.Password)
			err = errors.Join(err, passwordErr)
		}

		if err != nil {
			err = fmt.Errorf("update user values: %w: %w", input.ErrInvalidUpdateUserInput, err)
		}
	}

	if err == nil {
		var existing *security.User
		existing, err = s.repository.FindByID(ctx, request.ID)

		if err != nil {
			err = fmt.Errorf("load user %s: %w", uuid.UUID(request.ID), err)
		} else if existing == nil {
			err = fmt.Errorf("load user %s: %w", uuid.UUID(request.ID), security.ErrUserNotFound)
		}

		if err == nil {
			user, err = security.NewUser(
				request.ID, request.RoleID, values.email,
				values.phone, existing.PasswordHash(), values.firstName,
				values.lastName,
			)
			if err != nil {
				err = fmt.Errorf("update user %s: %w", uuid.UUID(request.ID), err)
			} else if request.Password != nil {
				var hash security.PasswordHash
				hash, err = s.hasher.Hash(ctx, password.Value())

				if err != nil {
					err = fmt.Errorf("hash user password: %w", err)
				} else if hashErr := user.SetPasswordHash(hash); hashErr != nil {
					err = fmt.Errorf("change user password: %w", hashErr)
				}
			}

			if err == nil {
				if updateErr := s.repository.Update(ctx, user); updateErr != nil {
					err = fmt.Errorf("update user %s: %w", uuid.UUID(request.ID), updateErr)
				}
			}

			if err != nil {
				user = nil
			}
		}
	}

	return user, err
}

func (s *UserService) Delete(ctx context.Context, id security.UserID) error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilUserRepository
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("delete user: %w", validationErr)
	} else if deleteErr := s.repository.Delete(ctx, id); deleteErr != nil {
		err = fmt.Errorf("delete user %s: %w", uuid.UUID(id), deleteErr)
	}

	return err
}
