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
// Every method acts for the caller in the context. Creating and listing
// Workspaces needs the caller's security.Identity, which must have a User.
// Every other method needs the caller's workspaces.Identity for the Workspace
// it names, whose Role must allow the operation; ResolveIdentity provides it.
// A caller without the identity is reported as ErrUnauthenticated, and a
// Workspace the caller does not belong to as ErrWorkspaceNotFound.
//
// The identity's Role was read before the operation began. Every write locks
// the Workspace and reads the caller's Role again under that lock, so a Role
// change and a write that relies on it apply one after another.

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

// The caller's identity for one Workspace: their User and the Role their
// membership gives them. A User who is not a member cannot tell the Workspace
// from a missing one, so both are reported as ErrWorkspaceNotFound.

func (s *WorkspaceService) ResolveIdentity(
	ctx context.Context,
	id workspaces.WorkspaceID,
) (*workspaces.Identity, error) {
	var (
		identity   *workspaces.Identity
		user       *users.User
		membership workspaces.Membership
		err        error
	)

	if err = s.check(); err == nil {
		user, err = currentUser(ctx)

		if err == nil {
			err = id.Validate()
		}

		if err != nil {
			err = fmt.Errorf("resolve a workspace identity: %w", err)
		}
	}

	if err == nil {
		membership, err = s.repository.FindMember(ctx, id, user.ID)

		if errors.Is(err, workspaces.ErrMembershipNotFound) {
			err = workspaces.ErrWorkspaceNotFound
		}

		if err == nil {
			identity = workspaces.NewIdentity(id, user, membership.Role)
		} else {
			err = fmt.Errorf("resolve the identity of user %s in workspace %s: %w", uuid.UUID(user.ID), uuid.UUID(id), err)
		}
	}

	return identity, err
}

// The caller becomes the Workspace's owner in the same transaction, so a
// Workspace is never left behind without the User who created it. A creator
// deleted in the meantime is reported by the repository as
// users.ErrUserNotFound.

func (s *WorkspaceService) Create(
	ctx context.Context,
	request input.CreateWorkspaceInput,
) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		creator   *users.User
		err       error
	)

	if err = s.check(); err == nil {
		creator, err = currentUser(ctx)

		if err != nil {
			err = fmt.Errorf("create workspace: %w", err)
		}
	}

	if err == nil {
		workspace, err = s.create(ctx, request, creator.ID)
	}

	return workspace, err
}

// The Workspace comes with the caller's Role in it, so the caller can offer
// only what that Role allows.

func (s *WorkspaceService) FindForMember(
	ctx context.Context,
	id workspaces.WorkspaceID,
) (MemberWorkspace, error) {
	var (
		found    MemberWorkspace
		identity *workspaces.Identity
		err      error
	)

	if err = s.check(); err == nil {
		err = id.Validate()

		if err == nil {
			identity, err = authorizeMember(ctx, id, workspaces.ActionViewContent, workspaces.ErrWorkspaceNotFound)
		}

		if err == nil {
			found.Workspace, err = s.repository.FindByID(ctx, id)
			found.Role = identity.Role
		}

		if err != nil {
			err = fmt.Errorf("find workspace %s of a member: %w", uuid.UUID(id), err)
			found = MemberWorkspace{}
		}
	}

	return found, err
}

// The member criterion of the query is replaced by the caller, so they list
// only their own Workspaces.

func (s *WorkspaceService) List(
	ctx context.Context,
	query workspaces.WorkspaceQuery,
) (WorkspaceList, error) {
	var (
		list WorkspaceList
		user *users.User
		err  error
	)

	if err = s.check(); err == nil {
		user, err = currentUser(ctx)

		if err == nil {
			query.Filter.MemberID = &user.ID
			err = query.Validate()
		}

		if err == nil {
			list.Total, err = s.repository.Count(ctx, query.Filter)
		}

		if err == nil {
			list.Workspaces, err = s.repository.Find(ctx, query)
		}

		if err != nil {
			err = fmt.Errorf("list workspaces of a member: %w", err)
			list = WorkspaceList{}
		}
	}

	return list, err
}

// Rename

func (s *WorkspaceService) Rename(
	ctx context.Context,
	request input.RenameWorkspaceInput,
) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		identity  *workspaces.Identity
		err       error
	)

	if err = s.check(); err == nil {
		name, nameErr := workspaces.NewWorkspaceName(strings.TrimSpace(request.Name))

		err = errors.Join(request.ID.Validate(), nameErr)

		if err == nil {
			workspace = workspaces.NewWorkspace(request.ID, name)
		} else {
			err = fmt.Errorf("rename workspace: %w: %w", input.ErrRenameWorkspaceInputInvalid, err)
		}
	}

	if err == nil {
		identity, err = authorizeMember(ctx, request.ID, workspaces.ActionRename, workspaces.ErrWorkspaceNotFound)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				_, err := reauthorizeMember(ctx, s.repository, identity, workspaces.ActionRename, workspaces.ErrWorkspaceNotFound)

				if err == nil {
					if updateErr := s.repository.Update(ctx, workspace); updateErr != nil {
						err = updateErr
					}
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf("rename workspace %s: %w", uuid.UUID(request.ID), err)
		}
	}

	if err != nil {
		workspace = nil
	}

	return workspace, err
}

// Members come back in name order. A membership whose User was deleted in
// the meantime is left out, as the deletion removes it too.

func (s *WorkspaceService) ListMembers(
	ctx context.Context,
	id workspaces.WorkspaceID,
) ([]WorkspaceMember, error) {
	var (
		members     []WorkspaceMember
		memberships workspaces.Memberships
		found       []*users.User
		err         error
	)

	if err = s.check(); err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("list workspace members: %w", err)
		}
	}

	if err == nil {
		_, err = authorizeMember(ctx, id, workspaces.ActionViewContent, workspaces.ErrWorkspaceNotFound)

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
			err = fmt.Errorf("list members of workspace %s: %w", uuid.UUID(id), err)
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
	request input.AddMemberByEmailInput,
) (*users.User, error) {
	var (
		user     *users.User
		identity *workspaces.Identity
		email    users.Email
		role     workspaces.Role
		err      error
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
			emailErr,
			roleErr,
		)

		if err != nil {
			err = fmt.Errorf("add workspace member by email: %w: %w", input.ErrAddMemberByEmailInputInvalid, err)
		}
	}

	if err == nil {
		identity, err = authorizeMember(ctx, request.WorkspaceID, workspaces.ActionManageMembers, workspaces.ErrWorkspaceNotFound)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				_, err := reauthorizeMember(ctx, s.repository, identity, workspaces.ActionManageMembers, workspaces.ErrWorkspaceNotFound)

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

// A User who is not a member is reported as ErrMembershipNotFound, a change
// to the caller's own role as ErrMembershipOwnRole, and demoting the
// Workspace's last owner as ErrMembershipLastOwner.

func (s *WorkspaceService) ChangeMemberRole(
	ctx context.Context,
	request input.ChangeMemberRoleInput,
) error {
	var (
		membership workspaces.Membership
		identity   *workspaces.Identity
	)

	err := s.check()

	if err == nil {
		role, roleErr := workspaces.NewRole(request.Role)

		err = errors.Join(
			request.WorkspaceID.Validate(),
			request.UserID.Validate(),
			roleErr,
		)

		if err == nil {
			membership = workspaces.NewMembership(request.WorkspaceID, request.UserID, role)
		} else {
			err = fmt.Errorf("change workspace member role: %w: %w", input.ErrChangeMemberRoleInputInvalid, err)
		}
	}

	if err == nil {
		identity, err = authorizeMember(ctx, membership.WorkspaceID, workspaces.ActionManageMembers, workspaces.ErrWorkspaceNotFound)

		if err == nil && membership.UserID == identity.User.ID {
			err = workspaces.ErrMembershipOwnRole
		}

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				var current workspaces.Membership

				_, err := reauthorizeMember(ctx, s.repository, identity, workspaces.ActionManageMembers, workspaces.ErrWorkspaceNotFound)

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
		}

		if err != nil {
			err = fmt.Errorf(
				"change role of user %s in workspace %s: %w",
				uuid.UUID(request.UserID),
				uuid.UUID(request.WorkspaceID),
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
	workspaceID workspaces.WorkspaceID,
	userID users.UserID,
) error {
	var identity *workspaces.Identity

	err := s.check()

	if err == nil {
		err = errors.Join(workspaceID.Validate(), userID.Validate())

		if err != nil {
			err = fmt.Errorf("remove workspace member: %w", err)
		}
	}

	if err == nil {
		identity, err = authorizeMember(ctx, workspaceID, workspaces.ActionManageMembers, workspaces.ErrWorkspaceNotFound)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				var membership workspaces.Membership

				_, err := reauthorizeMember(ctx, s.repository, identity, workspaces.ActionManageMembers, workspaces.ErrWorkspaceNotFound)

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
		}

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

func (s *WorkspaceService) create(
	ctx context.Context,
	request input.CreateWorkspaceInput,
	creatorID users.UserID,
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
			} else {
				membership := workspaces.NewMembership(workspace.ID, creatorID, workspaces.RoleOwner)

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
