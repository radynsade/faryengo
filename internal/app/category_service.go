package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Category list
//

type CategoryList struct {
	Categories []*categories.Category
	Total      int
}

//
// Category service
//

// Every method acts for the member in the context: the caller's
// workspaces.Identity must be for the Category's Workspace, and its Role must
// allow the operation: viewing content to read, editing content to write. A
// caller without the identity is reported as ErrUnauthenticated. A User who is
// not a member cannot tell the Workspace or its Categories from missing ones,
// so they are reported as workspaces.ErrWorkspaceNotFound or
// categories.ErrCategoryNotFound.
//
// The identity's Role was read before the operation began. Every write runs in
// a transaction, locks the Workspace, and reads the caller's Role again under
// that lock, so a Role change and a write that relies on it apply one after
// another.

var ErrCategoryRepositoryNil = errors.New("category repository is nil")

type CategoryService struct {
	transactor Transactor
	repository categories.CategoryRepository
	workspaces workspaces.WorkspaceRepository
}

func NewCategoryService(
	transactor Transactor,
	repository categories.CategoryRepository,
	workspaceRepository workspaces.WorkspaceRepository,
) (*CategoryService, error) {
	var (
		service *CategoryService
		err     error
	)

	if transactor == nil {
		err = ErrTransactorNil
	} else if repository == nil {
		err = ErrCategoryRepositoryNil
	} else if workspaceRepository == nil {
		err = ErrWorkspaceRepositoryNil
	} else {
		service = &CategoryService{transactor: transactor, repository: repository, workspaces: workspaceRepository}
	}

	return service, err
}

// Create

func (s *CategoryService) Create(
	ctx context.Context,
	request input.CreateCategoryInput,
) (*categories.Category, error) {
	var (
		category *categories.Category
		identity *workspaces.Identity
		err      error
	)

	if err = s.check(); err == nil {
		name, icon, color, valuesErr := categoryValues(request.Name, request.Icon, request.Color)

		err = errors.Join(request.WorkspaceID.Validate(), valuesErr)

		if err != nil {
			err = fmt.Errorf("create category: %w: %w", input.ErrCreateCategoryInputInvalid, err)
		} else {
			var (
				id         uuid.UUID
				categoryID categories.ID
			)

			id, err = uuid.NewV7()

			if err == nil {
				categoryID, err = categories.NewID(id)
			}

			if err != nil {
				err = fmt.Errorf("generate category ID: %w", err)
			} else {
				category = categories.NewCategory(categoryID, request.WorkspaceID, name, icon, color)
			}
		}
	}

	if err == nil {
		identity, err = authorizeMember(ctx, category.WorkspaceID, workspaces.ActionEditContent, workspaces.ErrWorkspaceNotFound)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				_, err := reauthorizeMember(ctx, s.workspaces, identity, workspaces.ActionEditContent, workspaces.ErrWorkspaceNotFound)

				if err == nil {
					if createErr := s.repository.Create(ctx, category); createErr != nil {
						err = createErr
					}
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf(
				"create category %s in workspace %s: %w",
				uuid.UUID(category.ID),
				uuid.UUID(request.WorkspaceID),
				err,
			)
		}
	}

	if err != nil {
		category = nil
	}

	return category, err
}

// The caller's identity is checked before the Category is read, so a caller
// without one learns nothing about it.

func (s *CategoryService) FindByID(
	ctx context.Context,
	id categories.ID,
) (*categories.Category, error) {
	var (
		category *categories.Category
		err      error
	)

	if err = s.check(); err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("find category: %w", err)
		}
	}

	if err == nil {
		_, err = currentMember(ctx)

		if err == nil {
			category, err = s.repository.FindByID(ctx, id)
		}

		if err == nil {
			_, err = authorizeMember(ctx, category.WorkspaceID, workspaces.ActionViewContent, categories.ErrCategoryNotFound)
		}

		if err != nil {
			err = fmt.Errorf("find category %s: %w", uuid.UUID(id), err)
			category = nil
		}
	}

	return category, err
}

// The Workspace criterion of the query is replaced by the given Workspace, so
// a member lists only that Workspace's Categories.

func (s *CategoryService) List(
	ctx context.Context,
	workspaceID workspaces.ID,
	query categories.CategoryQuery,
) (CategoryList, error) {
	var (
		list CategoryList
		err  error
	)

	if err = s.check(); err == nil {
		query.Filter.WorkspaceID = &workspaceID
		err = query.Validate()

		if err != nil {
			err = fmt.Errorf("list categories: %w", err)
		}
	}

	if err == nil {
		_, err = authorizeMember(ctx, workspaceID, workspaces.ActionViewContent, workspaces.ErrWorkspaceNotFound)

		if err == nil {
			list.Total, err = s.repository.Count(ctx, query.Filter)
		}

		if err == nil {
			list.Categories, err = s.repository.Find(ctx, query)
		}

		if err != nil {
			err = fmt.Errorf("list categories of workspace %s: %w", uuid.UUID(workspaceID), err)
			list = CategoryList{}
		}
	}

	return list, err
}

// The Category keeps its Workspace; only its name, icon, and color change.

func (s *CategoryService) Update(
	ctx context.Context,
	request input.UpdateCategoryInput,
) (*categories.Category, error) {
	var (
		category *categories.Category
		identity *workspaces.Identity
		name     categories.Name
		icon     *categories.Icon
		color    categories.Color
		err      error
	)

	if err = s.check(); err == nil {
		var valuesErr error

		name, icon, color, valuesErr = categoryValues(request.Name, request.Icon, request.Color)

		err = errors.Join(request.ID.Validate(), valuesErr)

		if err != nil {
			err = fmt.Errorf("update category: %w: %w", input.ErrUpdateCategoryInputInvalid, err)
		}
	}

	if err == nil {
		identity, err = s.authorizeEdit(ctx)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				stored, err := s.findForUpdate(ctx, identity, request.ID)

				if err == nil {
					category = categories.NewCategory(stored.ID, stored.WorkspaceID, name, icon, color)

					if updateErr := s.repository.Update(ctx, category); updateErr != nil {
						err = updateErr
					}
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf("update category %s: %w", uuid.UUID(request.ID), err)
		}
	}

	if err != nil {
		category = nil
	}

	return category, err
}

// Delete

func (s *CategoryService) Delete(ctx context.Context, id categories.ID) error {
	var identity *workspaces.Identity

	err := s.check()

	if err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("delete category: %w", err)
		}
	}

	if err == nil {
		identity, err = s.authorizeEdit(ctx)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				_, err := s.findForUpdate(ctx, identity, id)

				if err == nil {
					if deleteErr := s.repository.Delete(ctx, id); deleteErr != nil {
						err = deleteErr
					}
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf("delete category %s: %w", uuid.UUID(id), err)
		}
	}

	return err
}

//
// Helpers
//

func (s *CategoryService) check() error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrCategoryRepositoryNil
	} else if s.transactor == nil {
		err = ErrTransactorNil
	} else if s.workspaces == nil {
		err = ErrWorkspaceRepositoryNil
	}

	return err
}

// The Category's Workspace is learned only inside the transaction, so the
// caller's identity is checked first for the Workspace it belongs to.

func (s *CategoryService) authorizeEdit(ctx context.Context) (*workspaces.Identity, error) {
	identity, err := currentMember(ctx)

	if err == nil {
		identity, err = authorizeMember(ctx, identity.WorkspaceID, workspaces.ActionEditContent, categories.ErrCategoryNotFound)
	}

	return identity, err
}

// The Category is read only to learn its Workspace, which never changes, so
// locking the Workspace afterwards keeps the lock order of a Workspace
// deletion, which locks the Workspace before its Categories. A Category of
// another Workspace than the identity's is reported as ErrCategoryNotFound.
// It must run in a transaction.

func (s *CategoryService) findForUpdate(
	ctx context.Context,
	identity *workspaces.Identity,
	id categories.ID,
) (*categories.Category, error) {
	category, err := s.repository.FindByID(ctx, id)

	if err == nil && category.WorkspaceID != identity.WorkspaceID {
		err = categories.ErrCategoryNotFound
	}

	if err == nil {
		_, err = reauthorizeMember(ctx, s.workspaces, identity, workspaces.ActionEditContent, categories.ErrCategoryNotFound)
	}

	if err != nil {
		category = nil
	}

	return category, err
}

// Build the values a create or update request supplies; an empty icon means
// no icon. Every invalid value is reported.

func categoryValues(
	rawName string,
	rawIcon string,
	rawColor string,
) (categories.Name, *categories.Icon, categories.Color, error) {
	var (
		icon    *categories.Icon
		iconErr error
	)

	name, nameErr := categories.NewName(strings.TrimSpace(rawName))

	if rawIcon != "" {
		var value categories.Icon

		value, iconErr = categories.NewIcon(rawIcon)
		icon = &value
	}

	color, colorErr := categories.NewColor(rawColor)

	return name, icon, color, errors.Join(nameErr, iconErr, colorErr)
}
