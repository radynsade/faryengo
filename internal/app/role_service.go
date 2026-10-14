package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Role list
//

type RoleList struct {
	Roles []*users.Role
	Total int
}

//
// Role service
//

// Every write runs in a transaction, so it is atomic on its own and joins a
// transaction already carried by the caller's context. A Role name must
// include the catalog's fallback language translation, so writes read the
// fallback language before validating.
//
// Every method acts for the caller's security.Identity in the context.
// Managing Roles needs PermissionManageRole. Reading them needs permission to
// view or manage either Roles or Users, because Users are shown and edited
// with their Roles. A caller without an identity is reported as
// ErrUnauthenticated and one without the permission as
// users.ErrPermissionDenied.

var ErrRoleRepositoryNil = errors.New("role repository is nil")

var roleReaders = []users.Permission{
	users.PermissionViewRole,
	users.PermissionManageRole,
	users.PermissionViewUser,
	users.PermissionManageUser,
}

type RoleService struct {
	transactor Transactor
	repository users.RoleRepository
	languages  languages.LanguageRepository
}

func NewRoleService(
	transactor Transactor,
	repository users.RoleRepository,
	languageRepository languages.LanguageRepository,
) (*RoleService, error) {
	var (
		service *RoleService
		err     error
	)

	if transactor == nil {
		err = ErrTransactorNil
	} else if repository == nil {
		err = ErrRoleRepositoryNil
	} else if languageRepository == nil {
		err = ErrLanguageRepositoryNil
	} else {
		service = &RoleService{transactor: transactor, repository: repository, languages: languageRepository}
	}

	return service, err
}

// A name translation in a language missing from the catalog is reported by
// the repository as languages.ErrLanguageNotFound.

func (s *RoleService) Create(
	ctx context.Context,
	request input.CreateRoleInput,
) (*users.Role, error) {
	var (
		role *users.Role
		err  error
	)

	if err = s.check(); err == nil {
		_, err = authorizeSystem(ctx, users.PermissionManageRole)

		if err != nil {
			err = fmt.Errorf("create role: %w", err)
		}
	}

	if err == nil {
		var (
			id       uuid.UUID
			roleID   users.RoleID
			fallback languages.Code
		)

		id, err = uuid.NewV7()

		if err == nil {
			roleID, err = users.NewRoleID(id)
		}

		if err != nil {
			err = fmt.Errorf("generate role ID: %w", err)
		} else if fallback, err = s.fallback(ctx); err != nil {
			err = fmt.Errorf("create role: %w", err)
		} else {
			role, err = newRole(roleID, request.Name, request.Permissions, request.IsSuper, fallback)

			if err != nil {
				err = fmt.Errorf("create role: %w: %w", input.ErrCreateRoleInputInvalid, err)
			}
		}

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				var err error

				if createErr := s.repository.Create(ctx, role); createErr != nil {
					err = createErr
				}

				return err
			})

			if err != nil {
				err = fmt.Errorf("create role %s: %w", id, err)
			}
		}
	}

	if err != nil {
		role = nil
	}

	return role, err
}

// Find by an ID

func (s *RoleService) FindByID(ctx context.Context, id users.RoleID) (*users.Role, error) {
	var (
		role *users.Role
		err  error
	)

	if err = s.check(); err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("find role: %w", err)
		} else {
			_, err = authorizeSystem(ctx, roleReaders...)

			if err == nil {
				role, err = s.repository.FindByID(ctx, id)
			}

			if err != nil {
				err = fmt.Errorf("find role %s: %w", uuid.UUID(id), err)
				role = nil
			}
		}
	}

	return role, err
}

// List one page of the Roles matching a query, with the total number of
// matching Roles

func (s *RoleService) List(ctx context.Context, query users.RoleQuery) (RoleList, error) {
	var (
		list RoleList
		err  error
	)

	if err = s.check(); err == nil {
		_, err = authorizeSystem(ctx, roleReaders...)

		if err == nil {
			err = query.Validate()
		}

		if err != nil {
			err = fmt.Errorf("list roles: %w", err)
		}
	}

	if err == nil {
		list.Total, err = s.repository.Count(ctx, query.Filter)

		if err == nil {
			list.Roles, err = s.repository.Find(ctx, query)
		}

		if err != nil {
			err = fmt.Errorf("list roles: %w", err)
			list = RoleList{}
		}
	}

	return list, err
}

// The update replaces every field of the Role. The Role is locked first, so a
// missing one is reported as ErrRoleNotFound and concurrent updates apply one
// after another.

func (s *RoleService) Update(
	ctx context.Context,
	request input.UpdateRoleInput,
) (*users.Role, error) {
	var (
		role *users.Role
		err  error
	)

	if err = s.check(); err == nil {
		_, err = authorizeSystem(ctx, users.PermissionManageRole)

		if err != nil {
			err = fmt.Errorf("update role %s: %w", uuid.UUID(request.ID), err)
		}
	}

	if err == nil {
		var fallback languages.Code

		fallback, err = s.fallback(ctx)

		if err != nil {
			err = fmt.Errorf("update role: %w", err)
		} else {
			role, err = newRole(request.ID, request.Name, request.Permissions, request.IsSuper, fallback)

			if err != nil {
				err = fmt.Errorf("update role: %w: %w", input.ErrUpdateRoleInputInvalid, err)
			}
		}
	}

	if err == nil {
		err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
			_, err := s.repository.FindByIDForUpdate(ctx, role.ID)

			if err == nil {
				if updateErr := s.repository.Update(ctx, role); updateErr != nil {
					err = updateErr
				}
			}

			return err
		})

		if err != nil {
			err = fmt.Errorf("update role %s: %w", uuid.UUID(request.ID), err)
		}
	}

	if err != nil {
		role = nil
	}

	return role, err
}

// A Role assigned to any User is reported by the repository as
// ErrRoleAlreadyInUse.

func (s *RoleService) Delete(ctx context.Context, id users.RoleID) error {
	err := s.check()

	if err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("delete role: %w", err)
		}
	}

	if err == nil {
		_, err = authorizeSystem(ctx, users.PermissionManageRole)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				var err error

				if deleteErr := s.repository.Delete(ctx, id); deleteErr != nil {
					err = deleteErr
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf("delete role %s: %w", uuid.UUID(id), err)
		}
	}

	return err
}

//
// Helpers
//

func (s *RoleService) check() error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrRoleRepositoryNil
	} else if s.transactor == nil {
		err = ErrTransactorNil
	} else if s.languages == nil {
		err = ErrLanguageRepositoryNil
	}

	return err
}

// The catalog may have no fallback language; the empty code reports that.

func (s *RoleService) fallback(ctx context.Context) (languages.Code, error) {
	var code languages.Code

	language, err := s.languages.FindFallback(ctx)

	if errors.Is(err, languages.ErrLanguageNotFound) {
		err = nil
	} else if err != nil {
		err = fmt.Errorf("find the fallback language: %w", err)
	} else {
		code = language.Code
	}

	return code, err
}

// Every field is validated, rather than stopping at the first failure, so a
// caller can report all invalid fields at once.

func newRole(
	id users.RoleID,
	name map[string]string,
	permissions []string,
	isSuper bool,
	fallback languages.Code,
) (*users.Role, error) {
	var (
		role            *users.Role
		translations    map[languages.Code]languages.Translation
		rolePermissions []users.Permission
	)

	// The primitive constructors return their values even when they are
	// invalid; the name and permissions validation below reports them, so a
	// missing fallback translation comes before any other failure of the name.
	if name != nil {
		translations = make(map[languages.Code]languages.Translation, len(name))

		for code, translation := range name {
			languageCode, _ := languages.NewCode(code)
			translations[languageCode], _ = languages.NewTranslation(translation)
		}
	}

	if permissions != nil {
		rolePermissions = make([]users.Permission, 0, len(permissions))

		for _, value := range permissions {
			permission, _ := users.NewPermission(value)
			rolePermissions = append(rolePermissions, permission)
		}
	}

	roleName := users.NewRoleName(languages.NewText(translations))
	permissionSet := users.NewPermissions(rolePermissions)

	err := errors.Join(
		id.Validate(),
		roleName.ValidateWithFallback(fallback),
		permissionSet.Validate(),
	)

	if err == nil {
		role = users.NewRole(id, roleName, permissionSet, isSuper)
	}

	return role, err
}
