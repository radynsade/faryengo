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
// Member workspace
//

// A Workspace as one of its members sees it: their Role decides what they may
// do in it.

type MemberWorkspace struct {
	Workspace *workspaces.Workspace
	Role      workspaces.Role
}

//
// Workspace member
//

type WorkspaceMember struct {
	User *users.User
	Role workspaces.Role
}

//
// Workspace service
//

// Every write runs in a transaction, so it is atomic on its own and joins a
// transaction already carried by the caller's context. Users are read only to
// find a new member by email and to list members.
//
// Methods that take an acting User serve the office: the actor's membership
// Role must allow the operation, and a Workspace the actor does not belong to
// is reported as ErrWorkspaceNotFound. The other methods serve the admin panel
// and the CLI, which system permissions govern, and check no Workspace Role.

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

// The creator becomes the Workspace's owner in the same transaction, so a
// Workspace is never left behind without the User who created it. A missing
// creator is reported by the repository as users.ErrUserNotFound.

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

// The Workspace comes with the member's Role, so the caller can offer only
// what that Role allows.

func (s *WorkspaceService) FindForMember(
	ctx context.Context,
	id workspaces.WorkspaceID,
	userID users.UserID,
) (MemberWorkspace, error) {
	var (
		found      MemberWorkspace
		membership workspaces.Membership
		err        error
	)

	if err = s.check(); err == nil {
		err = errors.Join(id.Validate(), userID.Validate())

		if err != nil {
			err = fmt.Errorf("find workspace of a member: %w", err)
		}
	}

	if err == nil {
		membership, err = s.authorize(ctx, id, userID, workspaces.ActionViewContent)

		if err == nil {
			found.Workspace, err = s.repository.FindByID(ctx, id)
			found.Role = membership.Role
		}

		if err != nil {
			err = fmt.Errorf("find workspace %s of user %s: %w", uuid.UUID(id), uuid.UUID(userID), err)
		}
	}

	if err != nil {
		found = MemberWorkspace{}
	}

	return found, err
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

// The Workspace is locked first, so concurrent renames apply one after
// another and the actor's Role cannot change in between.

func (s *WorkspaceService) Rename(
	ctx context.Context,
	actorID users.UserID,
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
			actorID.Validate(),
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
			_, err := s.authorizeForUpdate(ctx, workspace.ID, actorID, workspaces.ActionRename)

			if err == nil {
				if updateErr := s.repository.Update(ctx, workspace); updateErr != nil {
					err = updateErr
				}
			}

			return err
		})

		if err != nil {
			err = fmt.Errorf("rename workspace %s as user %s: %w", uuid.UUID(request.ID), uuid.UUID(actorID), err)
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
	role workspaces.Role,
) error {
	var membership workspaces.Membership

	err := s.check()

	if err == nil {
		membership = workspaces.NewMembership(workspaceID, userID, role)
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
	actorID users.UserID,
	id workspaces.WorkspaceID,
) ([]WorkspaceMember, error) {
	var (
		members     []WorkspaceMember
		memberships workspaces.Memberships
		found       []*users.User
		err         error
	)

	if err = s.check(); err == nil {
		err = errors.Join(id.Validate(), actorID.Validate())

		if err != nil {
			err = fmt.Errorf("list workspace members: %w", err)
		}
	}

	if err == nil {
		_, err = s.authorize(ctx, id, actorID, workspaces.ActionViewContent)

		if err == nil {
			memberships, err = s.repository.FindMembers(ctx, id)
		}

		if err == nil {
			ids := make([]users.UserID, 0, len(memberships))

			for _, membership := range memberships {
				ids = append(ids, membership.UserID)
			}

			found, err = s.users.FindByIDs(ctx, ids)
		}

		if err == nil {
			roles := make(map[users.UserID]workspaces.Role, len(memberships))

			for _, membership := range memberships {
				roles[membership.UserID] = membership.Role
			}

			members = make([]WorkspaceMember, 0, len(found))

			for _, user := range found {
				members = append(members, WorkspaceMember{User: user, Role: roles[user.ID]})
			}
		}

		if err != nil {
			err = fmt.Errorf("list members of workspace %s as user %s: %w", uuid.UUID(id), uuid.UUID(actorID), err)
			members = nil
		}
	}

	return members, err
}

// The User is found by email, ignoring letter case. A missing User is
// reported as users.ErrUserNotFound and an existing membership as
// ErrMembershipAlreadyExists. The added User is returned, so the caller can
// name them.

func (s *WorkspaceService) AddMemberByEmail(
	ctx context.Context,
	actorID users.UserID,
	request input.AddMemberByEmailInput,
) (*users.User, error) {
	var (
		user  *users.User
		email users.Email
		role  workspaces.Role
		err   error
	)

	if err = s.check(); err == nil {
		var emailErr, roleErr error

		email, emailErr = users.NewEmail(request.Email)

		if request.Role == "" {
			role = workspaces.RoleViewer
		} else {
			role, roleErr = workspaces.NewRole(request.Role)
		}

		err = errors.Join(
			request.WorkspaceID.Validate(),
			actorID.Validate(),
			emailErr,
			roleErr,
		)

		if err != nil {
			err = fmt.Errorf("add workspace member by email: %w: %w", input.ErrAddMemberByEmailInputInvalid, err)
		}
	}

	if err == nil {
		err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
			_, err := s.authorizeForUpdate(ctx, request.WorkspaceID, actorID, workspaces.ActionManageMembers)

			if err == nil {
				user, err = s.users.FindByEmail(ctx, email)
			}

			if err == nil {
				if addErr := s.repository.AddMember(ctx, workspaces.NewMembership(request.WorkspaceID, user.ID, role)); addErr != nil {
					err = addErr
				}
			}

			return err
		})

		if err != nil {
			err = fmt.Errorf("add a member by email to workspace %s as user %s: %w", uuid.UUID(request.WorkspaceID), uuid.UUID(actorID), err)
		}
	}

	if err != nil {
		user = nil
	}

	return user, err
}

// A User who is not a member is reported as ErrMembershipNotFound, and
// demoting the Workspace's last owner as ErrMembershipLastOwner.

func (s *WorkspaceService) ChangeMemberRole(
	ctx context.Context,
	actorID users.UserID,
	request input.ChangeMemberRoleInput,
) error {
	var membership workspaces.Membership

	err := s.check()

	if err == nil {
		role, roleErr := workspaces.NewRole(request.Role)

		err = errors.Join(
			request.WorkspaceID.Validate(),
			request.UserID.Validate(),
			actorID.Validate(),
			roleErr,
		)

		if err == nil {
			membership = workspaces.NewMembership(request.WorkspaceID, request.UserID, role)
		} else {
			err = fmt.Errorf("change workspace member role: %w: %w", input.ErrChangeMemberRoleInputInvalid, err)
		}
	}

	if err == nil {
		err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
			var current workspaces.Membership

			_, err := s.authorizeForUpdate(ctx, membership.WorkspaceID, actorID, workspaces.ActionManageMembers)

			if err == nil {
				current, err = s.repository.FindMember(ctx, membership.WorkspaceID, membership.UserID)
			}

			if err == nil && membership.Role != workspaces.RoleOwner {
				err = s.checkOwnerRemains(ctx, current)
			}

			if err == nil {
				if updateErr := s.repository.UpdateMember(ctx, membership); updateErr != nil {
					err = updateErr
				}
			}

			return err
		})

		if err != nil {
			err = fmt.Errorf(
				"change role of user %s in workspace %s as user %s: %w",
				uuid.UUID(request.UserID),
				uuid.UUID(request.WorkspaceID),
				uuid.UUID(actorID),
				err,
			)
		}
	}

	return err
}

// A User who is not a member is reported as ErrMembershipNotFound, and
// removing the Workspace's last owner as ErrMembershipLastOwner.

func (s *WorkspaceService) RemoveMember(
	ctx context.Context,
	actorID users.UserID,
	workspaceID workspaces.WorkspaceID,
	userID users.UserID,
) error {
	err := s.check()

	if err == nil {
		err = errors.Join(workspaceID.Validate(), userID.Validate(), actorID.Validate())

		if err != nil {
			err = fmt.Errorf("remove workspace member: %w", err)
		}
	}

	if err == nil {
		err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
			var membership workspaces.Membership

			_, err := s.authorizeForUpdate(ctx, workspaceID, actorID, workspaces.ActionManageMembers)

			if err == nil {
				membership, err = s.repository.FindMember(ctx, workspaceID, userID)
			}

			if err == nil {
				err = s.checkOwnerRemains(ctx, membership)
			}

			if err == nil {
				if removeErr := s.repository.RemoveMember(ctx, membership); removeErr != nil {
					err = removeErr
				}
			}

			return err
		})

		if err != nil {
			err = fmt.Errorf(
				"remove user %s from workspace %s as user %s: %w",
				uuid.UUID(userID),
				uuid.UUID(workspaceID),
				uuid.UUID(actorID),
				err,
			)
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

// A User who is not a member cannot tell the Workspace from a missing one, so
// both are reported as ErrWorkspaceNotFound. The caller adds the operation
// context.

func (s *WorkspaceService) authorize(
	ctx context.Context,
	id workspaces.WorkspaceID,
	actorID users.UserID,
	action workspaces.Action,
) (workspaces.Membership, error) {
	membership, err := s.repository.FindMember(ctx, id, actorID)

	if errors.Is(err, workspaces.ErrMembershipNotFound) {
		err = workspaces.ErrWorkspaceNotFound
	} else if err == nil && !membership.Role.Allows(action) {
		err = fmt.Errorf("%w: a workspace %s may not %s", workspaces.ErrRolePermissionDenied, membership.Role, action)
	}

	if err != nil {
		membership = workspaces.Membership{}
	}

	return membership, err
}

// The Workspace is locked before the actor's Role is read, so writes to one
// Workspace's memberships apply one after another. It must run in a
// transaction.

func (s *WorkspaceService) authorizeForUpdate(
	ctx context.Context,
	id workspaces.WorkspaceID,
	actorID users.UserID,
	action workspaces.Action,
) (workspaces.Membership, error) {
	var membership workspaces.Membership

	_, err := s.repository.FindByIDForUpdate(ctx, id)

	if err == nil {
		membership, err = s.authorize(ctx, id, actorID, action)
	}

	return membership, err
}

// The caller holds the Workspace's lock, so no other owner can leave between
// this check and the change it guards.

func (s *WorkspaceService) checkOwnerRemains(ctx context.Context, membership workspaces.Membership) error {
	var err error

	if membership.Role == workspaces.RoleOwner {
		var memberships workspaces.Memberships

		memberships, err = s.repository.FindMembers(ctx, membership.WorkspaceID)

		if err == nil && memberships.IsLastOwner(membership.UserID) {
			err = workspaces.ErrMembershipLastOwner
		}
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
				membership := workspaces.NewMembership(workspace.ID, *creatorID, workspaces.RoleOwner)

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
