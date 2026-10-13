package workspaces

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/pkg/domquery"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

var (
	testWorkspaceID = WorkspaceID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))
	testUserID      = users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000001"))
)

func TestNewWorkspaceID(t *testing.T) {
	tests := []struct {
		name  string
		value uuid.UUID
		want  error
	}{
		{"valid", uuid.UUID(testWorkspaceID), nil},
		{"nil", uuid.Nil, ErrWorkspaceIDInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewWorkspaceID(tt.value)

			if id != WorkspaceID(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewWorkspaceID(%s) = (%v, %v), want %v", tt.value, id, err, tt.want)
			}
		})
	}
}

// The constructor keeps the value as given, so surrounding whitespace is
// rejected rather than trimmed.

func TestNewWorkspaceName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  error
	}{
		{"valid", "Design team", nil},
		{"surrounding spaces", "  Design  ", ErrWorkspaceNameUntrimmed},
		{"only whitespace", " \t ", ErrWorkspaceNameEmpty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, err := NewWorkspaceName(tt.value)

			if name != WorkspaceName(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewWorkspaceName(%q) = (%q, %v), want %v", tt.value, name, err, tt.want)
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
		wantErrs  []error
	}{
		{"valid", NewWorkspace(testWorkspaceID, "Design"), nil},
		{"nil", nil, []error{ErrWorkspaceNil}},
		{"nil ID", NewWorkspace(WorkspaceID{}, "Design"), []error{ErrWorkspaceIDInvalid}},
		{"invalid name", NewWorkspace(testWorkspaceID, " "), []error{ErrWorkspaceNameEmpty}},
		{"invalid ID and name", NewWorkspace(WorkspaceID{}, " "), []error{ErrWorkspaceIDInvalid, ErrWorkspaceNameEmpty}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.workspace.Validate()

			if (err == nil) != (len(tt.wantErrs) == 0) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErrs)
			}

			for _, want := range tt.wantErrs {
				if !errors.Is(err, want) {
					t.Fatalf("Validate() = %v, want %v", err, want)
				}
			}

			if err != nil && !errors.Is(err, ErrWorkspaceInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrWorkspaceInvalid)
			}
		})
	}
}

// The constructor only assigns, so an invalid membership is reported by its
// validation.

func TestMembershipValidate(t *testing.T) {
	tests := []struct {
		name        string
		workspaceID WorkspaceID
		userID      users.UserID
		wantErrs    []error
	}{
		{"valid", testWorkspaceID, testUserID, nil},
		{"nil workspace", WorkspaceID{}, testUserID, []error{ErrWorkspaceIDInvalid}},
		{"nil user", testWorkspaceID, users.UserID{}, []error{users.ErrUserIDInvalid}},
		{"nil workspace and user", WorkspaceID{}, users.UserID{}, []error{ErrWorkspaceIDInvalid, users.ErrUserIDInvalid}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			membership := NewMembership(tt.workspaceID, tt.userID, RoleViewer)

			if membership.WorkspaceID != tt.workspaceID || membership.UserID != tt.userID || membership.Role != RoleViewer {
				t.Fatalf("NewMembership() = %+v, want the given workspace, user, and role", membership)
			}

			err := membership.Validate()

			if (err == nil) != (len(tt.wantErrs) == 0) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErrs)
			}

			for _, want := range tt.wantErrs {
				if !errors.Is(err, want) {
					t.Fatalf("Validate() = %v, want %v", err, want)
				}
			}

			if err != nil && !errors.Is(err, ErrMembershipInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrMembershipInvalid)
			}
		})
	}
}

// The filter and query constructors only assign, so they keep invalid values
// for their validation to report.

func TestNewWorkspaceQuery(t *testing.T) {
	filter := NewWorkspaceFilter("0192", "\xff", &testUserID)
	query := NewWorkspaceQuery(filter, domquery.SortOrderAsc, WorkspaceSortName, 0, 1)

	if query.Filter != filter || query.SortOrder != domquery.SortOrderAsc ||
		query.SortBy != WorkspaceSortName || query.Limit != 0 || query.Page != 1 {
		t.Fatalf("NewWorkspaceQuery() = %+v, want the given values", query)
	}

	if filter.IDLike != "0192" || filter.NameLike != "\xff" || filter.MemberID != &testUserID {
		t.Fatalf("NewWorkspaceFilter() = %+v, want the given values", filter)
	}

	if err := query.Validate(); !errors.Is(err, ErrWorkspaceFilterInvalid) {
		t.Fatalf("Validate() = %v, want %v", err, ErrWorkspaceFilterInvalid)
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
