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
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilUserRepository
	} else if s.hasher == nil {
		err = ErrNilPasswordHasher
	} else if validationErr := request.Validate(); validationErr != nil {
		err = fmt.Errorf("validate create user input: %w", validationErr)
	} else {
		var id uuid.UUID
		id, err = uuid.NewRandom()
		if err != nil {
			err = fmt.Errorf("generate user ID: %w", err)
		} else {
			var hash security.PasswordHash
			hash, err = s.hasher.Hash(ctx, request.Password)
			if err != nil {
				err = fmt.Errorf("hash user password: %w", err)
			} else {
				user, err = security.NewUser(
					security.UserID(id), request.RoleID, security.Email(request.Email),
					security.Phone(request.Phone), hash, security.FirstName(request.FirstName),
					security.LastName(request.LastName),
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
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilUserRepository
	} else if s.hasher == nil {
		err = ErrNilPasswordHasher
	} else if validationErr := request.Validate(); validationErr != nil {
		err = fmt.Errorf("validate update user input: %w", validationErr)
	} else {
		var hash security.PasswordHash
		if request.Password == nil {
			var existing *security.User
			existing, err = s.repository.FindByID(ctx, request.ID)
			if err != nil {
				err = fmt.Errorf("load user %s: %w", uuid.UUID(request.ID), err)
			} else if existing == nil {
				err = fmt.Errorf("load user %s: %w", uuid.UUID(request.ID), security.ErrUserNotFound)
			} else {
				hash = existing.PasswordHash()
			}
		} else {
			hash, err = s.hasher.Hash(ctx, *request.Password)
			if err != nil {
				err = fmt.Errorf("hash user password: %w", err)
			}
		}

		if err == nil {
			user, err = security.NewUser(
				request.ID, request.RoleID, security.Email(request.Email),
				security.Phone(request.Phone), hash, security.FirstName(request.FirstName),
				security.LastName(request.LastName),
			)
			if err != nil {
				err = fmt.Errorf("update user %s: %w", uuid.UUID(request.ID), err)
			} else if updateErr := s.repository.Update(ctx, user); updateErr != nil {
				user = nil
				err = fmt.Errorf("update user %s: %w", uuid.UUID(request.ID), updateErr)
			}
		}
	}

	return user, err
}

func (s *UserService) Delete(ctx context.Context, id security.UserID) error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilUserRepository
	} else if uuid.UUID(id) == uuid.Nil {
		err = fmt.Errorf("delete user: %w", input.ErrInvalidUserID)
	} else if deleteErr := s.repository.Delete(ctx, id); deleteErr != nil {
		err = fmt.Errorf("delete user %s: %w", uuid.UUID(id), deleteErr)
	}

	return err
}
