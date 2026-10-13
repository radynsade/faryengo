package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Workspace list
//

type WorkspaceList struct {
	Workspaces []*workspaces.Workspace
	Total      int
}

//
// Workspace service
//

// Every write runs in a transaction, so it is atomic on its own and joins a
// transaction already carried by the caller's context. Users are read only to
// find a new member by email.

var ErrWorkspaceRepositoryNil = errors.New("workspace repository is nil")

type WorkspaceService struct {
	transactor Transactor
	repository workspaces.WorkspaceRepository
	users      users.UserRepository
}

func NewWorkspaceService(
	transactor Transactor,
	repository workspaces.WorkspaceRepository,
	userRepository users.UserRepository,
) (*WorkspaceService, error) {
	var (
		service *WorkspaceService
		err     error
	)

	if transactor == nil {
		err = ErrTransactorNil
	} else if repository == nil {
		err = ErrWorkspaceRepositoryNil
	} else if userRepository == nil {
		err = ErrUserRepositoryNil
	} else {
		service = &WorkspaceService{transactor: transactor, repository: repository, users: userRepository}
	}

	return service, err
}

// Create

func (s *WorkspaceService) Create(
	ctx context.Context,
	request input.CreateWorkspaceInput,
) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		err       error
	)

	if err = s.check(); err == nil {
		workspace, err = s.create(ctx, request, nil)
	}

	return workspace, err
}

// The creator becomes the Workspace's first member in the same transaction,
// so a Workspace is never left behind without the User who created it. A
// missing creator is reported by the repository as users.ErrUserNotFound.

func (s *WorkspaceService) CreateAsUser(
	ctx context.Context,
	creatorID users.UserID,
	request input.CreateWorkspaceInput,
) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		err       error
	)

	if err = s.check(); err == nil {
		err = creatorID.Validate()

		if err != nil {
			err = fmt.Errorf("create workspace as a user: %w", err)
		} else {
			workspace, err = s.create(ctx, request, &creatorID)
		}
	}

	return workspace, err
}

// Find by an ID

func (s *WorkspaceService) FindByID(
	ctx context.Context,
	id workspaces.WorkspaceID,
) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		err       error
	)

	if err = s.check(); err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("find workspace: %w", err)
		} else {
			workspace, err = s.repository.FindByID(ctx, id)

			if err != nil {
				err = fmt.Errorf("find workspace %s: %w", uuid.UUID(id), err)
				workspace = nil
			}
		}
	}

	return workspace, err
}

// A Workspace the User does not belong to is reported as
// ErrWorkspaceNotFound, so a caller cannot tell it from a missing one.

func (s *WorkspaceService) FindForMember(
	ctx context.Context,
	id workspaces.WorkspaceID,
	userID users.UserID,
) (*workspaces.Workspace, error) {
	var (
		workspace  *workspaces.Workspace
		membership workspaces.Membership
		isMember   bool
		err        error
	)

	if err = s.check(); err == nil {
		membership = workspaces.NewMembership(id, userID)
		err = membership.Validate()

		if err != nil {
			err = fmt.Errorf("find workspace of a member: %w", err)
		}
	}

	if err == nil {
		workspace, err = s.repository.FindByID(ctx, id)

		if err == nil {
			isMember, err = s.repository.HasMember(ctx, membership)
		}

		if err == nil && !isMember {
			err = workspaces.ErrWorkspaceNotFound
		}

		if err != nil {
			err = fmt.Errorf("find workspace %s of user %s: %w", uuid.UUID(id), uuid.UUID(userID), err)
		}
	}

	if err != nil {
		workspace = nil
	}

	return workspace, err
}

// List one page of the Workspaces matching a query, with the total number of
// matching Workspaces

func (s *WorkspaceService) List(
	ctx context.Context,
	query workspaces.WorkspaceQuery,
) (WorkspaceList, error) {
	var (
		list WorkspaceList
		err  error
	)

	if err = s.check(); err == nil {
		list, err = s.list(ctx, query)

		if err != nil {
			err = fmt.Errorf("list workspaces: %w", err)
		}
	}

	return list, err
}

// The member criterion of the query is replaced by the given User. A User
// that does not exist belongs to no Workspace, so the list is empty.

func (s *WorkspaceService) ListByMember(
	ctx context.Context,
	userID users.UserID,
	query workspaces.WorkspaceQuery,
) (WorkspaceList, error) {
	var (
		list WorkspaceList
		err  error
	)

	if err = s.check(); err == nil {
		err = userID.Validate()

		if err == nil {
			query.Filter.MemberID = &userID
			list, err = s.list(ctx, query)
		}

		if err != nil {
			err = fmt.Errorf("list workspaces of user %s: %w", uuid.UUID(userID), err)
		}
	}

	return list, err
}

// The Workspace is locked first, so a missing one is reported as
// ErrWorkspaceNotFound and concurrent renames apply one after another.

func (s *WorkspaceService) Rename(
	ctx context.Context,
	request input.RenameWorkspaceInput,
) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		err       error
	)

	if err = s.check(); err == nil {
		name, nameErr := workspaces.NewWorkspaceName(strings.TrimSpace(request.Name))

		err = errors.Join(
			request.ID.Validate(),
			nameErr,
		)

		if err == nil {
			workspace = workspaces.NewWorkspace(request.ID, name)
		} else {
			err = fmt.Errorf("rename workspace: %w: %w", input.ErrRenameWorkspaceInputInvalid, err)
		}
	}

	if err == nil {
		err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
			_, err := s.repository.FindByIDForUpdate(ctx, workspace.ID)

			if err == nil {
				if updateErr := s.repository.Update(ctx, workspace); updateErr != nil {
					err = updateErr
				}
			}

			return err
		})

		if err != nil {
			err = fmt.Errorf("rename workspace %s: %w", uuid.UUID(request.ID), err)
		}
	}

	if err != nil {
		workspace = nil
	}

	return workspace, err
}

// Deleting a Workspace removes its memberships.

func (s *WorkspaceService) Delete(ctx context.Context, id workspaces.WorkspaceID) error {
	err := s.check()

	if err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("delete workspace: %w", err)
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
			err = fmt.Errorf("delete workspace %s: %w", uuid.UUID(id), err)
		}
	}

	return err
}

// A missing Workspace or User is reported by the repository as
// ErrWorkspaceNotFound or users.ErrUserNotFound, and an existing membership
// as ErrMembershipAlreadyExists.

func (s *WorkspaceService) AddMember(
	ctx context.Context,
	workspaceID workspaces.WorkspaceID,
	userID users.UserID,
) error {
	var membership workspaces.Membership

	err := s.check()

	if err == nil {
		membership = workspaces.NewMembership(workspaceID, userID)
		err = membership.Validate()

		if err != nil {
			err = fmt.Errorf("add workspace member: %w", err)
		}
	}

	if err == nil {
		err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
			var err error

			if addErr := s.repository.AddMember(ctx, membership); addErr != nil {
				err = addErr
			}

			return err
		})

		if err != nil {
			err = fmt.Errorf("add user %s to workspace %s: %w", uuid.UUID(userID), uuid.UUID(workspaceID), err)
		}
	}

	return err
}

// Members come back in name order. A membership whose User was deleted in
// the meantime is left out, as the deletion removes it too.

func (s *WorkspaceService) ListMembers(
	ctx context.Context,
	id workspaces.WorkspaceID,
) ([]*users.User, error) {
	var (
		members []*users.User
		ids     []users.UserID
		err     error
	)

	if err = s.check(); err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("list workspace members: %w", err)
		}
	}

	if err == nil {
		ids, err = s.repository.FindMemberIDs(ctx, id)

		if err == nil {
			members, err = s.users.FindByIDs(ctx, ids)
		}

		if err != nil {
			err = fmt.Errorf("list members of workspace %s: %w", uuid.UUID(id), err)
			members = nil
		}
	}

	return members, err
}

// The User is found by email, ignoring letter case. A missing User is
// reported as users.ErrUserNotFound, a missing Workspace as
// ErrWorkspaceNotFound, and an existing membership as
// ErrMembershipAlreadyExists. The added User is returned, so the caller can
// name them.

func (s *WorkspaceService) AddMemberByEmail(
	ctx context.Context,
	request input.AddMemberByEmailInput,
) (*users.User, error) {
	var (
		user  *users.User
		email users.Email
		err   error
	)

	if err = s.check(); err == nil {
		var emailErr error

		email, emailErr = users.NewEmail(request.Email)

		err = errors.Join(
			request.WorkspaceID.Validate(),
			emailErr,
		)

		if err != nil {
			err = fmt.Errorf("add workspace member by email: %w: %w", input.ErrAddMemberByEmailInputInvalid, err)
		}
	}

	if err == nil {
		var membership workspaces.Membership

		user, err = s.users.FindByEmail(ctx, email)

		if err == nil {
			membership = workspaces.NewMembership(request.WorkspaceID, user.ID)
			err = membership.Validate()
		}

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				var err error

				if addErr := s.repository.AddMember(ctx, membership); addErr != nil {
					err = addErr
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf("add a member by email to workspace %s: %w", uuid.UUID(request.WorkspaceID), err)
		}
	}

	if err != nil {
		user = nil
	}

	return user, err
}

// A User who is not a member is reported by the repository as
// ErrMembershipNotFound.

func (s *WorkspaceService) RemoveMember(
	ctx context.Context,
	workspaceID workspaces.WorkspaceID,
	userID users.UserID,
) error {
	var membership workspaces.Membership

	err := s.check()

	if err == nil {
		membership = workspaces.NewMembership(workspaceID, userID)
		err = membership.Validate()

		if err != nil {
			err = fmt.Errorf("remove workspace member: %w", err)
		}
	}

	if err == nil {
		err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
			var err error

			if removeErr := s.repository.RemoveMember(ctx, membership); removeErr != nil {
				err = removeErr
			}

			return err
		})

		if err != nil {
			err = fmt.Errorf("remove user %s from workspace %s: %w", uuid.UUID(userID), uuid.UUID(workspaceID), err)
		}
	}

	return err
}

//
// Helpers
//

func (s *WorkspaceService) check() error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrWorkspaceRepositoryNil
	} else if s.transactor == nil {
		err = ErrTransactorNil
	} else if s.users == nil {
		err = ErrUserRepositoryNil
	}

	return err
}

// Without a creator the Workspace starts with no members.

func (s *WorkspaceService) create(
	ctx context.Context,
	request input.CreateWorkspaceInput,
	creatorID *users.UserID,
) (*workspaces.Workspace, error) {
	var workspace *workspaces.Workspace

	name, err := workspaces.NewWorkspaceName(strings.TrimSpace(request.Name))

	if err != nil {
		err = fmt.Errorf("create workspace: %w: %w", input.ErrCreateWorkspaceInputInvalid, err)
	} else {
		var (
			id          uuid.UUID
			workspaceID workspaces.WorkspaceID
		)

		id, err = uuid.NewV7()

		if err == nil {
			workspaceID, err = workspaces.NewWorkspaceID(id)
		}

		if err != nil {
			err = fmt.Errorf("generate workspace ID: %w", err)
		} else {
			workspace = workspaces.NewWorkspace(workspaceID, name)
		}
	}

	if err == nil {
		err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
			var err error

			if createErr := s.repository.Create(ctx, workspace); createErr != nil {
				err = createErr
			} else if creatorID != nil {
				membership := workspaces.NewMembership(workspace.ID, *creatorID)

				if membershipErr := membership.Validate(); membershipErr != nil {
					err = membershipErr
				} else if addErr := s.repository.AddMember(ctx, membership); addErr != nil {
					err = addErr
				}
			}

			return err
		})

		if err != nil {
			err = fmt.Errorf("create workspace %s: %w", uuid.UUID(workspace.ID), err)
		}
	}

	if err != nil {
		workspace = nil
	}

	return workspace, err
}

// The caller adds the operation context.

func (s *WorkspaceService) list(
	ctx context.Context,
	query workspaces.WorkspaceQuery,
) (WorkspaceList, error) {
	var list WorkspaceList

	err := query.Validate()

	if err == nil {
		list.Total, err = s.repository.Count(ctx, query.Filter)
	}

	if err == nil {
		list.Workspaces, err = s.repository.Find(ctx, query)
	}

	if err != nil {
		list = WorkspaceList{}
	}

	return list, err
}
