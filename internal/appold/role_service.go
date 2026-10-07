package app

import (
	"context"
	"errors"
	"fmt"

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
	} else {
		var id uuid.UUID
		id, err = uuid.NewV7()

		if err != nil {
			err = fmt.Errorf("generate role ID: %w", err)
		} else {
			role, err = roleFromInput(security.RoleID(id), request.Name, request.Permissions, request.IsSuper)

			if err != nil {
				err = fmt.Errorf("create role values: %w: %w", input.ErrInvalidCreateRoleInput, err)
			}

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

func (s *RoleService) List(ctx context.Context, query security.RoleQuery) (security.RolePage, error) {
	var page security.RolePage
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilRoleRepository
	} else if validationErr := query.Validate(); validationErr != nil {
		err = validationErr
	} else {
		page.Page, page.PageSize = query.Page, query.PageSize
		page.Total, err = s.repository.Count(ctx, query.Filters)

		if err == nil {
			if page.Page > page.Pages() {
				page.Page = page.Pages()
				query.Page = page.Page
			}

			page.Roles, err = s.repository.Find(ctx, query)
		}

		if err != nil {
			err = fmt.Errorf("list roles: %w", err)
			page = security.RolePage{}
		}
	}

	return page, err
}

func (s *RoleService) Update(ctx context.Context, request input.UpdateRoleInput) (*security.Role, error) {
	var role *security.Role
	var replacement languages.Text
	var permissions []security.Permission
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilRoleRepository
	} else {
		err = request.ID.Validate()
		var permissionErr error
		permissions, permissionErr = security.NewPermissions(request.Permissions)
		var nameErr error

		if request.Name != nil {
			replacement, nameErr = security.NewRoleName(request.Name)
		}

		err = errors.Join(err, nameErr, permissionErr)

		if err != nil {
			err = fmt.Errorf("update role values: %w: %w", input.ErrInvalidUpdateRoleInput, err)
		}
	}

	if err == nil {
		var existing *security.Role
		existing, err = s.FindByID(ctx, request.ID)

		if err == nil {
			name, isSuper := existing.Name(), existing.IsSuper()

			if request.Name != nil {
				name = replacement
			}

			if request.Permissions == nil {
				permissions = existing.Permissions()
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
	text, nameErr := security.NewRoleName(name)
	permissions, permissionErr := security.NewPermissions(permissions)
	err := errors.Join(nameErr, permissionErr)

	if err == nil {
		role, err = security.NewRole(id, text, permissions)

		if err == nil {
			role.SetIsSuper(isSuper)
		}
	}

	return role, err
}
