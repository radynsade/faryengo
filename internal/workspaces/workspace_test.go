package workspaces

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/users"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

var (
	testWorkspaceID = WorkspaceID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))
	testUserID      = users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000001"))
)

func TestTrimWorkspaceName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  WorkspaceName
	}{
		{"already trimmed", "Design", "Design"},
		{"surrounding spaces", "  Design  ", "Design"},
		{"tabs and newlines", "\t\nDesign team\n", "Design team"},
		{"inner whitespace kept", "Design   team", "Design   team"},
		{"only whitespace", " \t ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TrimWorkspaceName(tt.value); got != tt.want {
				t.Fatalf("TrimWorkspaceName(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestWorkspaceNameValidate(t *testing.T) {
	tests := []struct {
		name          string
		workspaceName WorkspaceName
		want          error
	}{
		{"valid", "Design", nil},
		{"limit in multibyte characters", WorkspaceName(strings.Repeat("ā", MaxWorkspaceNameLength)), nil},
		{"limit in ASCII characters", WorkspaceName(strings.Repeat("a", MaxWorkspaceNameLength)), nil},
		{"empty", "", ErrWorkspaceNameEmpty},
		{"blank", " \t", ErrWorkspaceNameEmpty},
		{"invalid UTF-8", "Design\xff", ErrWorkspaceNameInvalidChars},
		{"leading space", " Design", ErrWorkspaceNameUntrimmed},
		{"trailing newline", "Design\n", ErrWorkspaceNameUntrimmed},
		{"too long", WorkspaceName(strings.Repeat("a", MaxWorkspaceNameLength+1)), ErrWorkspaceNameTooLong},
		{"too long in multibyte characters", WorkspaceName(strings.Repeat("ā", MaxWorkspaceNameLength+1)), ErrWorkspaceNameTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.workspaceName.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if tt.want != nil && !errors.Is(err, ErrWorkspaceNameInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrWorkspaceNameInvalid)
			}
		})
	}
}

func TestWorkspaceValidate(t *testing.T) {
	tests := []struct {
		name      string
		workspace *Workspace
		want      error
	}{
		{"valid", NewWorkspace(testWorkspaceID, "Design"), nil},
		{"nil", nil, ErrWorkspaceNil},
		{"nil ID", NewWorkspace(WorkspaceID{}, "Design"), ErrWorkspaceIDInvalid},
		{"invalid name", NewWorkspace(testWorkspaceID, " "), ErrWorkspaceNameEmpty},
		{"ID checked before name", NewWorkspace(WorkspaceID{}, " "), ErrWorkspaceIDInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.workspace.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if tt.want != nil && !errors.Is(err, ErrWorkspaceInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrWorkspaceInvalid)
			}
		})
	}
}

func TestMembershipValidate(t *testing.T) {
	tests := []struct {
		name       string
		membership Membership
		want       error
	}{
		{"valid", NewMembership(testWorkspaceID, testUserID), nil},
		{"nil workspace", NewMembership(WorkspaceID{}, testUserID), ErrWorkspaceIDInvalid},
		{"nil user", NewMembership(testWorkspaceID, users.UserID{}), users.ErrUserIDInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.membership.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if tt.want != nil && !errors.Is(err, ErrMembershipInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrMembershipInvalid)
			}
		})
	}
}

func TestWorkspaceQueryValidate(t *testing.T) {
	valid := func() WorkspaceQuery {
		return WorkspaceQuery{SortBy: WorkspaceSortName, Limit: 20, Page: 1}
	}

	tests := []struct {
		name     string
		change   func(query *WorkspaceQuery)
		wantErrs []error
	}{
		{"valid", func(*WorkspaceQuery) {}, nil},
		{"every filter", func(query *WorkspaceQuery) {
			query.Filter = WorkspaceFilter{IDLike: "0192", NameLike: "des", MemberID: &testUserID}
		}, nil},
		{"unknown sort", func(query *WorkspaceQuery) { query.SortBy = "created_at" }, []error{ErrWorkspaceQueryInvalid}},
		{"missing limit", func(query *WorkspaceQuery) { query.Limit = 0 }, []error{ErrWorkspaceQueryInvalid}},
		{"missing page", func(query *WorkspaceQuery) { query.Page = 0 }, []error{ErrWorkspaceQueryInvalid}},
		{"NUL in the ID", func(query *WorkspaceQuery) { query.Filter.IDLike = "a\x00" }, []error{ErrWorkspaceFilterInvalid, ErrWorkspaceFilterIDLikeInvalidChars}},
		{"invalid UTF-8 in the name", func(query *WorkspaceQuery) { query.Filter.NameLike = "\xff" }, []error{ErrWorkspaceFilterInvalid, ErrWorkspaceFilterNameLikeInvalidChars}},
		{"long ID", func(query *WorkspaceQuery) {
			query.Filter.IDLike = strings.Repeat("a", MaxWorkspaceFilterIDLikeLength+1)
		}, []error{ErrWorkspaceFilterInvalid, ErrWorkspaceFilterIDLikeTooLong}},
		{"long name counts characters", func(query *WorkspaceQuery) {
			query.Filter.NameLike = strings.Repeat("ā", MaxWorkspaceFilterNameLikeLength)
		}, nil},
		{"long name", func(query *WorkspaceQuery) {
			query.Filter.NameLike = strings.Repeat("a", MaxWorkspaceFilterNameLikeLength+1)
		}, []error{ErrWorkspaceFilterInvalid, ErrWorkspaceFilterNameLikeTooLong}},
		{"nil member", func(query *WorkspaceQuery) { query.Filter.MemberID = &users.UserID{} }, []error{ErrWorkspaceFilterInvalid, users.ErrUserIDInvalid}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := valid()
			tt.change(&query)
			err := query.Validate()

			if (err == nil) != (len(tt.wantErrs) == 0) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErrs)
			}

			for _, want := range tt.wantErrs {
				if !errors.Is(err, want) {
					t.Fatalf("Validate() = %v, want %v", err, want)
				}
			}
		})
	}
}

func TestWorkspaceFilterCount(t *testing.T) {
	tests := []struct {
		name   string
		filter WorkspaceFilter
		want   int
	}{
		{"empty", WorkspaceFilter{}, 0},
		{"one", WorkspaceFilter{NameLike: "des"}, 1},
		{"every", WorkspaceFilter{IDLike: "1", NameLike: "a", MemberID: &testUserID}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Count(); got != tt.want {
				t.Fatalf("Count() = %d, want %d", got, tt.want)
			}
		})
	}
}
