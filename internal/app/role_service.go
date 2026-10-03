package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
)

var ErrNilRoleRepository = errors.New("nil role repository")

type RoleService struct {
	repository security.RoleRepository
}

func NewRoleService(repository security.RoleRepository) (*RoleService, error) {
	var service *RoleService
	var err error

	if repository == nil {
		err = ErrNilRoleRepository
	} else {
		service = &RoleService{repository: repository}
	}

	return service, err
}

func (s *RoleService) Create(ctx context.Context, request input.CreateRoleInput) (*security.Role, error) {
	var role *security.Role
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilRoleRepository
	} else if validationErr := request.Validate(); validationErr != nil {
		err = fmt.Errorf("validate create role input: %w", validationErr)
	} else {
		var id uuid.UUID
		id, err = uuid.NewRandom()

		if err != nil {
			err = fmt.Errorf("generate role ID: %w", err)
		} else {
			role, err = roleFromInput(security.RoleID(id), request.Name, request.Permissions, request.IsSuper)

			if err == nil {
				if createErr := s.repository.Create(ctx, role); createErr != nil {
					err = fmt.Errorf("create role %s: %w", id, createErr)
				}
			}
		}
	}

	if err != nil {
		role = nil
	}

	return role, err
}

func (s *RoleService) FindByID(ctx context.Context, id security.RoleID) (*security.Role, error) {
	var role *security.Role
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilRoleRepository
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("find role: %w", validationErr)
	} else {
		role, err = s.repository.FindByID(ctx, id)

		if err != nil {
			err = fmt.Errorf("load role %s: %w", uuid.UUID(id), err)
		} else if role == nil {
			err = fmt.Errorf("load role %s: %w", uuid.UUID(id), security.ErrRoleNotFound)
		}
	}

	if err != nil {
		role = nil
	}

	return role, err
}

func (s *RoleService) Update(ctx context.Context, request input.UpdateRoleInput) (*security.Role, error) {
	var role *security.Role
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilRoleRepository
	} else if validationErr := request.Validate(); validationErr != nil {
		err = fmt.Errorf("validate update role input: %w", validationErr)
	} else {
		var existing *security.Role
		existing, err = s.FindByID(ctx, request.ID)

		if err == nil {
			name, permissions, isSuper := existing.Name(), existing.Permissions(), existing.IsSuper()

			if request.Name != nil {
				name, err = roleNameFromInput(request.Name)
			}

			if request.Permissions != nil {
				permissions = request.Permissions
			}

			if request.IsSuper != nil {
				isSuper = *request.IsSuper
			}

			if err == nil {
				role, err = security.NewRole(request.ID, name, permissions)

				if err == nil {
					role.SetIsSuper(isSuper)

					if updateErr := s.repository.Update(ctx, role); updateErr != nil {
						err = fmt.Errorf("update role %s: %w", uuid.UUID(request.ID), updateErr)
					}
				}
			}
		}
	}

	if err != nil {
		role = nil
	}

	return role, err
}

func (s *RoleService) Delete(ctx context.Context, id security.RoleID) error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilRoleRepository
	} else if validationErr := id.Validate(); validationErr != nil {
		err = fmt.Errorf("delete role: %w", validationErr)
	} else if deleteErr := s.repository.Delete(ctx, id); deleteErr != nil {
		err = fmt.Errorf("delete role %s: %w", uuid.UUID(id), deleteErr)
	}

	return err
}

func roleFromInput(id security.RoleID, name map[string]string, permissions []security.Permission, isSuper bool) (*security.Role, error) {
	var role *security.Role
	text, err := roleNameFromInput(name)

	if err == nil {
		role, err = security.NewRole(id, text, permissions)

		if err == nil {
			role.SetIsSuper(isSuper)
		}
	}

	return role, err
}

func roleNameFromInput(name map[string]string) (languages.Text, error) {
	translations := make([]languages.Translation, 0, len(name))
	var text languages.Text
	var err error

	for _, code := range slices.Sorted(maps.Keys(name)) {
		var translation languages.Translation
		translation, err = languages.NewTranslation(languages.LanguageCode(code), name[code])

		if err != nil {
			err = fmt.Errorf("map role name %q: %w", code, err)
			break
		}

		translations = append(translations, translation)
	}

	if err == nil {
		text, err = languages.NewText(translations)
	}

	return text, err
}
