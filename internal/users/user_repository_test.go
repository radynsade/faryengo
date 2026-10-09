package users

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestUserQueryValidate(t *testing.T) {
	roleID := RoleID(uuid.MustParse("01920000-0000-7000-8000-000000000002"))
	valid := func() UserQuery {
		return UserQuery{SortBy: UserSortEmail, Limit: 20, Page: 1}
	}

	tests := []struct {
		name     string
		change   func(query *UserQuery)
		wantErrs []error
	}{
		{"valid", func(*UserQuery) {}, nil},
		{"every filter", func(query *UserQuery) {
			query.Filter = UserFilter{IDLike: "0192", EmailLike: "ada", NameLike: "Ada Love", RoleID: &roleID}
		}, nil},
		{"unknown sort", func(query *UserQuery) { query.SortBy = "phone" }, []error{ErrInvalidUserQuery}},
		{"missing limit", func(query *UserQuery) { query.Limit = 0 }, []error{ErrInvalidUserQuery}},
		{"missing page", func(query *UserQuery) { query.Page = 0 }, []error{ErrInvalidUserQuery}},
		{"NUL in the ID", func(query *UserQuery) { query.Filter.IDLike = "a\x00" }, []error{ErrUserFilterInvalid, ErrUserFilterIDLikeInvalidChars}},
		{"invalid UTF-8 in the email", func(query *UserQuery) { query.Filter.EmailLike = "\xff" }, []error{ErrUserFilterInvalid, ErrUserFilterEmailLikeInvalidChars}},
		{"NUL in the name", func(query *UserQuery) { query.Filter.NameLike = "\x00" }, []error{ErrUserFilterInvalid, ErrUserFilterNameLikeInvalidChars}},
		{"long ID", func(query *UserQuery) {
			query.Filter.IDLike = strings.Repeat("a", MaxUserFilterIDLikeLength+1)
		}, []error{ErrUserFilterInvalid, ErrUserFilterIDLikeTooLong}},
		{"long email", func(query *UserQuery) {
			query.Filter.EmailLike = strings.Repeat("a", MaxUserFilterEmailLikeLength+1)
		}, []error{ErrUserFilterInvalid, ErrUserFilterEmailLikeTooLong}},
		{"long name counts characters", func(query *UserQuery) {
			query.Filter.NameLike = strings.Repeat("ā", MaxUserFilterNameLikeLength)
		}, nil},
		{"long name", func(query *UserQuery) {
			query.Filter.NameLike = strings.Repeat("a", MaxUserFilterNameLikeLength+1)
		}, []error{ErrUserFilterInvalid, ErrUserFilterNameLikeTooLong}},
		{"nil role", func(query *UserQuery) { query.Filter.RoleID = &RoleID{} }, []error{ErrUserFilterInvalid, ErrRoleIDInvalid}},
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

func TestUserFilterCount(t *testing.T) {
	roleID := RoleID(uuid.MustParse("01920000-0000-7000-8000-000000000002"))

	tests := []struct {
		name   string
		filter UserFilter
		want   int
	}{
		{"empty", UserFilter{}, 0},
		{"one", UserFilter{EmailLike: "ada"}, 1},
		{"every", UserFilter{IDLike: "1", EmailLike: "a", NameLike: "b", RoleID: &roleID}, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Count(); got != tt.want {
				t.Fatalf("Count() = %d, want %d", got, tt.want)
			}
		})
	}
}
