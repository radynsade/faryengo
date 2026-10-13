package workspaces

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/users"
)

//
// Workspace ID
//

var ErrWorkspaceIDInvalid = errors.New("invalid workspace ID")

type WorkspaceID uuid.UUID

func NewWorkspaceID(value uuid.UUID) (WorkspaceID, error) {
	var (
		err error
		id  WorkspaceID
	)

	id = WorkspaceID(value)
	err = id.Validate()
	return id, err
}

func (id WorkspaceID) Validate() error {
	var err error

	if uuid.UUID(id) == uuid.Nil {
		err = ErrWorkspaceIDInvalid
	}

	return err
}

//
// Workspace name
//

const MaxWorkspaceNameLength = 100

var (
	ErrWorkspaceNameInvalid      = errors.New("invalid workspace name")
	ErrWorkspaceNameEmpty        = errors.New("is empty")
	ErrWorkspaceNameInvalidChars = errors.New("invalid characters")
	ErrWorkspaceNameUntrimmed    = errors.New("has surrounding whitespace")
	ErrWorkspaceNameTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxWorkspaceNameLength)
)

type WorkspaceName string

// A Workspace name is stored trimmed, so a name given by a person is trimmed
// before it is constructed.

func NewWorkspaceName(value string) (WorkspaceName, error) {
	var (
		err  error
		name WorkspaceName
	)

	name = WorkspaceName(value)
	err = name.Validate()
	return name, err
}

func (n WorkspaceName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrWorkspaceNameEmpty
	} else if !utf8.ValidString(string(n)) {
		err = ErrWorkspaceNameInvalidChars
	} else if strings.TrimSpace(string(n)) != string(n) {
		err = ErrWorkspaceNameUntrimmed
	} else if utf8.RuneCountInString(string(n)) > MaxWorkspaceNameLength {
		err = ErrWorkspaceNameTooLong
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrWorkspaceNameInvalid, err)
	}

	return err
}

//
// Action
//

type Action string

const (
	ActionViewContent   Action = "view_content"
	ActionEditContent   Action = "edit_content"
	ActionRename        Action = "rename"
	ActionManageMembers Action = "manage_members"
)

//
// Role
//

var (
	ErrRoleInvalid          = errors.New("invalid role")
	ErrRolePermissionDenied = errors.New("permission denied")
)

type Role string

const (
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleOwner  Role = "owner"
)

func NewRole(value string) (Role, error) {
	var (
		err  error
		role Role
	)

	role = Role(value)
	err = role.Validate()
	return role, err
}

func (r Role) Validate() error {
	var err error

	switch r {
	case RoleViewer, RoleEditor, RoleOwner:
	default:
		err = ErrRoleInvalid
	}

	return err
}

// A viewer only views a Workspace's content, an editor also edits it, and an
// owner also renames the Workspace and manages its members. An invalid Role
// allows nothing.

func (r Role) Allows(action Action) bool {
	var allowed bool

	switch action {
	case ActionViewContent:
		allowed = r == RoleViewer || r == RoleEditor || r == RoleOwner
	case ActionEditContent:
		allowed = r == RoleEditor || r == RoleOwner
	case ActionRename, ActionManageMembers:
		allowed = r == RoleOwner
	}

	return allowed
}

//
// Membership
//

var ErrMembershipInvalid = errors.New("invalid membership")

type Membership struct {
	WorkspaceID WorkspaceID
	UserID      users.UserID
	Role        Role
}

func NewMembership(workspaceID WorkspaceID, userID users.UserID, role Role) Membership {
	return Membership{
		WorkspaceID: workspaceID,
		UserID:      userID,
		Role:        role,
	}
}

func (m Membership) Validate() error {
	err := errors.Join(
		m.WorkspaceID.Validate(),
		m.UserID.Validate(),
		m.Role.Validate(),
	)

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrMembershipInvalid, err)
	}

	return err
}

//
// Memberships
//

var ErrMembershipLastOwner = errors.New("user is the last owner of the workspace")

type Memberships []Membership

func NewMemberships(memberships []Membership) Memberships {
	return Memberships(memberships)
}

func (m Memberships) Validate() error {
	var err error

	for _, membership := range m {
		err = membership.Validate()

		if err != nil {
			break
		}
	}

	return err
}

// A Workspace that has an owner keeps one, so its only owner can be neither
// removed nor demoted.

func (m Memberships) IsLastOwner(userID users.UserID) bool {
	var isOwner bool

	owners := 0

	for _, membership := range m {
		if membership.Role == RoleOwner {
			owners++
			isOwner = isOwner || membership.UserID == userID
		}
	}

	return isOwner && owners == 1
}

//
// Workspace
//

var (
	ErrWorkspaceInvalid = errors.New("invalid workspace")
	ErrWorkspaceNil     = errors.New("is nil")
)

type Workspace struct {
	ID   WorkspaceID
	Name WorkspaceName
}

func NewWorkspace(id WorkspaceID, name WorkspaceName) *Workspace {
	return &Workspace{
		ID:   id,
		Name: name,
	}
}

func (w *Workspace) Validate() error {
	var err error

	if w == nil {
		err = ErrWorkspaceNil
	} else {
		err = errors.Join(
			w.ID.Validate(),
			w.Name.Validate(),
		)
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrWorkspaceInvalid, err)
	}

	return err
}
