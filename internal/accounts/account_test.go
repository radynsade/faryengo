package accounts

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

var (
	testAccountID   = ID(uuid.MustParse("01920000-0000-7000-8000-000000000005"))
	testWorkspaceID = workspaces.ID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))
	testUserID      = users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000001"))
)

func userIDPointer(id users.UserID) *users.UserID {
	return &id
}

func TestNewID(t *testing.T) {
	tests := []struct {
		name  string
		value uuid.UUID
		want  error
	}{
		{"valid", uuid.UUID(testAccountID), nil},
		{"nil", uuid.Nil, ErrIDInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewID(tt.value)

			if id != ID(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewID(%s) = (%v, %v), want %v", tt.value, id, err, tt.want)
			}
		})
	}
}

// The constructor keeps the value as given, even when it is invalid.

func TestNewName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  error
	}{
		{"valid", "Cash", nil},
		{"only whitespace", " \t ", ErrNameEmpty},
		{"too long", strings.Repeat("a", MaxNameLength+1), ErrNameTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, err := NewName(tt.value)

			if name != Name(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewName(%q) = (%q, %v), want %v", tt.value, name, err, tt.want)
			}
		})
	}
}

func TestNameValidate(t *testing.T) {
	tests := []struct {
		name        string
		accountName Name
		want        error
	}{
		{"valid", "Cash", nil},
		{"single character", "a", nil},
		{"limit in ASCII characters", Name(strings.Repeat("a", MaxNameLength)), nil},
		{"limit in multibyte characters", Name(strings.Repeat("ā", MaxNameLength)), nil},
		{"empty", "", ErrNameEmpty},
		{"only whitespace", " ", ErrNameEmpty},
		{"invalid UTF-8", "Cash\xff", ErrNameInvalidChars},
		{"too long", Name(strings.Repeat("ā", MaxNameLength+1)), ErrNameTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.accountName.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Name(%q).Validate() = %v, want %v", tt.accountName, err, tt.want)
			}

			if err != nil && !errors.Is(err, ErrNameInvalid) {
				t.Fatalf("Name(%q).Validate() = %v, want it to wrap %v", tt.accountName, err, ErrNameInvalid)
			}
		})
	}
}

func TestAccountValidate(t *testing.T) {
	tests := []struct {
		name        string
		id          ID
		workspaceID workspaces.ID
		userID      *users.UserID
		accountName Name
		want        []error
	}{
		{"valid with a user", testAccountID, testWorkspaceID, userIDPointer(testUserID), "Cash", nil},
		{"valid without a user", testAccountID, testWorkspaceID, nil, "Cash", nil},
		{"invalid ID", ID{}, testWorkspaceID, nil, "Cash", []error{ErrIDInvalid}},
		{"invalid workspace ID", testAccountID, workspaces.ID{}, nil, "Cash", []error{workspaces.ErrIDInvalid}},
		{"invalid user ID", testAccountID, testWorkspaceID, userIDPointer(users.UserID{}), "Cash", []error{users.ErrUserIDInvalid}},
		{"invalid name", testAccountID, testWorkspaceID, nil, "", []error{ErrNameInvalid}},
		{"invalid ID and name", ID{}, testWorkspaceID, nil, " ", []error{ErrIDInvalid, ErrNameInvalid}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := NewAccount(tt.id, tt.workspaceID, tt.userID, tt.accountName)

			if account.ID != tt.id ||
				account.WorkspaceID != tt.workspaceID ||
				account.UserID != tt.userID ||
				account.Name != tt.accountName {
				t.Fatalf("NewAccount() = %+v, want the given ID, workspace, user, and name", account)
			}

			err := account.Validate()

			if (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if err != nil && !errors.Is(err, ErrAccountInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrAccountInvalid)
			}

			for _, want := range tt.want {
				if !errors.Is(err, want) {
					t.Fatalf("Validate() = %v, want it to include %v", err, want)
				}
			}
		})
	}

	if err := (*Account)(nil).Validate(); !errors.Is(err, ErrAccountNil) || !errors.Is(err, ErrAccountInvalid) {
		t.Fatalf("Validate() on nil = %v, want %v wrapped in %v", err, ErrAccountNil, ErrAccountInvalid)
	}
}
