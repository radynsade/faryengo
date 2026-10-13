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
// Membership
//

var ErrMembershipInvalid = errors.New("invalid membership")

type Membership struct {
	WorkspaceID WorkspaceID
	UserID      users.UserID
}

func NewMembership(workspaceID WorkspaceID, userID users.UserID) (Membership, error) {
	var (
		err        error
		membership Membership
	)

	membership = Membership{
		WorkspaceID: workspaceID,
		UserID:      userID,
	}

	err = membership.Validate()
	return membership, err
}

func (m Membership) Validate() error {
	err := errors.Join(
		m.WorkspaceID.Validate(),
		m.UserID.Validate(),
	)

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrMembershipInvalid, err)
	}

	return err
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
