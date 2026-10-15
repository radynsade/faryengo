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
	testWorkspaceID = ID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))
	testUserID      = users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000001"))
)

func TestNewWorkspaceID(t *testing.T) {
	tests := []struct {
		name  string
		value uuid.UUID
		want  error
	}{
		{"valid", uuid.UUID(testWorkspaceID), nil},
		{"nil", uuid.Nil, ErrIDInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewID(tt.value)

			if id != ID(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
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
		{"surrounding spaces", "  Design  ", ErrNameUntrimmed},
		{"only whitespace", " \t ", ErrNameEmpty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, err := NewName(tt.value)

			if name != Name(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewWorkspaceName(%q) = (%q, %v), want %v", tt.value, name, err, tt.want)
			}
		})
	}
}

func TestWorkspaceNameValidate(t *testing.T) {
	tests := []struct {
		name          string
		workspaceName Name
		want          error
	}{
		{"valid", "Design", nil},
		{"limit in multibyte characters", Name(strings.Repeat("ā", MaxNameLength)), nil},
		{"limit in ASCII characters", Name(strings.Repeat("a", MaxNameLength)), nil},
		{"empty", "", ErrNameEmpty},
		{"blank", " \t", ErrNameEmpty},
		{"invalid UTF-8", "Design\xff", ErrNameInvalidChars},
		{"leading space", " Design", ErrNameUntrimmed},
		{"trailing newline", "Design\n", ErrNameUntrimmed},
		{"too long", Name(strings.Repeat("a", MaxNameLength+1)), ErrNameTooLong},
		{"too long in multibyte characters", Name(strings.Repeat("ā", MaxNameLength+1)), ErrNameTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.workspaceName.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if tt.want != nil && !errors.Is(err, ErrNameInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrNameInvalid)
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
		{"nil ID", NewWorkspace(ID{}, "Design"), []error{ErrIDInvalid}},
		{"invalid name", NewWorkspace(testWorkspaceID, " "), []error{ErrNameEmpty}},
		{"invalid ID and name", NewWorkspace(ID{}, " "), []error{ErrIDInvalid, ErrNameEmpty}},
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

func TestNewRole(t *testing.T) {
	tests := []struct {
		value string
		want  error
	}{
		{"viewer", nil},
		{"editor", nil},
		{"owner", nil},
		{"", ErrRoleInvalid},
		{"Owner", ErrRoleInvalid},
		{"admin", ErrRoleInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			role, err := NewRole(tt.value)

			if role != Role(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewRole(%q) = (%q, %v), want %v", tt.value, role, err, tt.want)
			}
		})
	}
}

func TestRoleValidate(t *testing.T) {
	tests := []struct {
		name string
		role Role
		want error
	}{
		{"viewer", RoleViewer, nil},
		{"editor", RoleEditor, nil},
		{"owner", RoleOwner, nil},
		{"empty", "", ErrRoleInvalid},
		{"surrounding spaces", " owner ", ErrRoleInvalid},
		{"unknown", "admin", ErrRoleInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.role.Validate(); !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

// Every role and action pair is listed, so a new role or action must decide
// its permissions here too. An invalid role and an unknown action allow
// nothing.

func TestRoleAllows(t *testing.T) {
	actions := []Action{ActionViewContent, ActionEditContent, ActionRename, ActionManageMembers, "delete"}

	tests := []struct {
		role Role
		want []bool
	}{
		{RoleViewer, []bool{true, false, false, false, false}},
		{RoleEditor, []bool{true, true, false, false, false}},
		{RoleOwner, []bool{true, true, true, true, false}},
		{"", []bool{false, false, false, false, false}},
		{"admin", []bool{false, false, false, false, false}},
	}

	for _, tt := range tests {
		for index, action := range actions {
			t.Run(string(tt.role)+" "+string(action), func(t *testing.T) {
				if got := tt.role.Allows(action); got != tt.want[index] {
					t.Fatalf("Role(%q).Allows(%q) = %t, want %t", tt.role, action, got, tt.want[index])
				}
			})
		}
	}
}

// The constructor only assigns, so an invalid membership is reported by its
// validation.

func TestMembershipValidate(t *testing.T) {
	tests := []struct {
		name        string
		workspaceID ID
		userID      users.UserID
		role        Role
		wantErrs    []error
	}{
		{"valid", testWorkspaceID, testUserID, RoleViewer, nil},
		{"nil workspace", ID{}, testUserID, RoleOwner, []error{ErrIDInvalid}},
		{"nil user", testWorkspaceID, users.UserID{}, RoleEditor, []error{users.ErrUserIDInvalid}},
		{"missing role", testWorkspaceID, testUserID, "", []error{ErrRoleInvalid}},
		{"unknown role", testWorkspaceID, testUserID, "admin", []error{ErrRoleInvalid}},
		{
			"nil workspace and user and unknown role",
			ID{},
			users.UserID{},
			"admin",
			[]error{ErrIDInvalid, users.ErrUserIDInvalid, ErrRoleInvalid},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			membership := NewMembership(tt.workspaceID, tt.userID, tt.role)

			if membership.WorkspaceID != tt.workspaceID || membership.UserID != tt.userID || membership.Role != tt.role {
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

func TestMembershipsValidate(t *testing.T) {
	valid := NewMembership(testWorkspaceID, testUserID, RoleOwner)

	tests := []struct {
		name        string
		memberships Memberships
		want        error
	}{
		{"empty", NewMemberships(nil), nil},
		{"valid", NewMemberships([]Membership{valid}), nil},
		{"invalid role", NewMemberships([]Membership{valid, NewMembership(testWorkspaceID, testUserID, "admin")}), ErrRoleInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.memberships.Validate(); !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestMembershipsIsLastOwner(t *testing.T) {
	otherID := users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000002"))
	member := func(userID users.UserID, role Role) Membership {
		return NewMembership(testWorkspaceID, userID, role)
	}

	tests := []struct {
		name        string
		memberships Memberships
		want        bool
	}{
		{"the only owner", Memberships{member(testUserID, RoleOwner), member(otherID, RoleEditor)}, true},
		{"one of two owners", Memberships{member(testUserID, RoleOwner), member(otherID, RoleOwner)}, false},
		{"not an owner", Memberships{member(testUserID, RoleEditor), member(otherID, RoleOwner)}, false},
		{"not a member", Memberships{member(otherID, RoleOwner)}, false},
		{"no members", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.memberships.IsLastOwner(testUserID); got != tt.want {
				t.Fatalf("IsLastOwner() = %t, want %t", got, tt.want)
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
