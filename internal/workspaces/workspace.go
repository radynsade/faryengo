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
// before it is validated.

func TrimWorkspaceName(name string) WorkspaceName {
	return WorkspaceName(strings.TrimSpace(name))
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

func NewMembership(workspaceID WorkspaceID, userID users.UserID) Membership {
	return Membership{
		WorkspaceID: workspaceID,
		UserID:      userID,
	}
}

func (m Membership) Validate() error {
	err := m.WorkspaceID.Validate()

	if err == nil {
		err = m.UserID.Validate()
	}

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
		err = w.ID.Validate()
	}

	if err == nil {
		err = w.Name.Validate()
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrWorkspaceInvalid, err)
	}

	return err
}
