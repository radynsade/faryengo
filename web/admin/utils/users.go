package utils

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	"github.com/radynsade/faryengo/pkg/domquery"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
)

//
// User query
//

const DefaultUserPageSize = 20

// The query string carries the filters, sorting, and page, so every list
// state has its own address. The limits match the domain's user filter.

type userQueryParameters struct {
	IDLike    string `form:"uuid" validate:"utf8,maxbytes=36,excludesall=0x00"`
	EmailLike string `form:"email" validate:"utf8,maxbytes=254,excludesall=0x00"`
	NameLike  string `form:"name" validate:"utf8,maxbytes=804,excludesall=0x00"`
	RoleID    string `form:"role" validate:"omitempty,uuid_input"`
	Sort      string `form:"sort" validate:"oneof=id email first_name last_name created_at"`
	Order     string `form:"order" validate:"oneof=asc desc"`
	Page      int    `form:"page" validate:"min=1,max=1000000"`
	Size      int    `form:"size" validate:"min=1,max=100"`
}

var userFilterFields = []string{"uuid", "email", "name", "role"}

// The parsed query keeps the submitted filters even when validation fails, so
// the filter form shows them beside their errors.

func ParseUserQuery(request *http.Request) (users.UserQuery, error) {
	parameters := userQueryParameters{
		Sort:  string(users.UserSortCreatedAt),
		Order: "desc",
		Page:  1,
		Size:  DefaultUserPageSize,
	}

	values, err := url.ParseQuery(request.URL.RawQuery)

	if err == nil {
		err = checkFields(values, []string{"uuid", "email", "name", "role", "sort", "order", "page", "size"}, nil)
	}

	if err == nil {
		parameters.IDLike = strings.TrimSpace(values.Get("uuid"))
		parameters.EmailLike = strings.TrimSpace(values.Get("email"))
		parameters.NameLike = strings.TrimSpace(values.Get("name"))
		parameters.RoleID = strings.TrimSpace(values.Get("role"))

		for _, parameter := range []struct {
			key    string
			target *string
		}{{"sort", &parameters.Sort}, {"order", &parameters.Order}} {
			if value := values.Get(parameter.key); value != "" {
				*parameter.target = value
			}
		}

		for _, parameter := range []struct {
			key    string
			target *int
		}{{"page", &parameters.Page}, {"size", &parameters.Size}} {
			if value := values.Get(parameter.key); value != "" && err == nil {
				*parameter.target, err = strconv.Atoi(value)
			}
		}
	}

	if err == nil {
		err = requestvalidation.Validate(request.Context(), parameters)
	}

	query := users.UserQuery{
		Filter: users.UserFilter{
			IDLike:    parameters.IDLike,
			EmailLike: parameters.EmailLike,
			NameLike:  parameters.NameLike,
		},
		SortBy:    users.UserSort(parameters.Sort),
		SortOrder: domquery.SortOrder(parameters.Order == "asc"),
		Limit:     uint(max(parameters.Size, 0)),
		Page:      uint(max(parameters.Page, 0)),
	}

	if parsed, parseErr := uuid.Parse(parameters.RoleID); parseErr == nil {
		roleID := users.RoleID(parsed)
		query.Filter.RoleID = &roleID
	}

	if err != nil {
		err = fmt.Errorf("parse a user query: %w: %w", users.ErrInvalidUserQuery, err)
	}

	return query, err
}

// Links built from a rejected query must still be valid, so its sorting and
// paging fall back to the defaults while its filters stay.

func SanitizeUserQuery(query users.UserQuery) users.UserQuery {
	if query.SortBy.Validate() != nil {
		query.SortBy = users.UserSortCreatedAt
	}

	if query.Limit == 0 || query.Limit > 100 {
		query.Limit = DefaultUserPageSize
	}

	query.Page = 1

	return query
}

func UserFilterFieldErrors(request *http.Request, err error) components.FieldErrors {
	return filterFieldErrors(request.Context(), err, userFilterFields)
}

//
// User errors
//

// A failure that belongs to a form field is shown beside that field, so the
// message here only matters when no field claims it.

func UserError(request *http.Request, err error) (int, string) {
	status, messageID := http.StatusInternalServerError, "errors.users"

	switch {
	case errors.Is(err, input.ErrDeleteOwnUser):
		status, messageID = http.StatusConflict, "errors.user_delete_self"
	case errors.Is(err, users.ErrUserNotFound):
		status, messageID = http.StatusNotFound, "errors.user_not_found"
	case errors.Is(err, users.ErrUserConflict):
		status, messageID = http.StatusConflict, "errors.user_conflict"
	case errors.Is(err, users.ErrUserAlreadyExists):
		status, messageID = http.StatusConflict, "errors.user_exists"
	case errors.Is(err, users.ErrInvalidUserQuery), errors.Is(err, users.ErrUserFilterInvalid):
		status, messageID = http.StatusBadRequest, "errors.user_query"
	case errors.Is(err, users.ErrUserIDInvalid), errors.Is(err, ErrFormInvalid):
		status, messageID = http.StatusBadRequest, "errors.user_form"
	case errors.Is(err, users.ErrRoleNotFound),
		errors.Is(err, requestvalidation.ErrInvalidRequest),
		errors.Is(err, input.ErrCreateUserInputInvalid),
		errors.Is(err, input.ErrUpdateUserInputInvalid):
		status, messageID = http.StatusUnprocessableEntity, "errors.user_values"
	}

	if status == http.StatusInternalServerError {
		slog.ErrorContext(request.Context(), "admin user operation", "error", err)
	}

	return status, messageID
}

//
// User values
//

// Moments are shown in UTC, so every viewer sees the same value.

const userMomentLayout = "2006-01-02 15:04:05 UTC"

// A User's Role is named in the interface language; a Role missing from the
// given ones is shown by its identity.

func UserRow(
	user *users.User,
	current *security.Identity,
	roles []*users.Role,
	code string,
	catalog []*languages.Language,
) pages.UserRow {
	roleID := uuid.UUID(user.RoleID).String()
	row := pages.UserRow{
		ID:        uuid.UUID(user.ID).String(),
		FirstName: string(user.FirstName),
		LastName:  string(user.LastName),
		Email:     string(user.Email),
		Phone:     string(user.Phone),
		RoleID:    roleID,
		RoleName:  roleID,
		CreatedAt: user.CreatedAt.UTC().Format(userMomentLayout),
		UpdatedAt: user.UpdatedAt.UTC().Format(userMomentLayout),
		Current:   current != nil && current.User != nil && user.ID == current.User.ID,
	}

	for _, role := range roles {
		if role.ID == user.RoleID {
			row.RoleName = RoleName(role, code, catalog)
		}
	}

	return row
}

func FormFromUser(user *users.User) UserForm {
	return UserForm{
		FirstName: string(user.FirstName),
		LastName:  string(user.LastName),
		Email:     string(user.Email),
		Phone:     string(user.Phone),
		RoleID:    uuid.UUID(user.RoleID).String(),
		UpdatedAt: user.UpdatedAt,
	}
}

func RoleOptions(
	roles []*users.Role,
	selected string,
	code string,
	catalog []*languages.Language,
) []pages.RoleOption {
	options := make([]pages.RoleOption, 0, len(roles))

	for _, role := range roles {
		id := uuid.UUID(role.ID).String()

		options = append(options, pages.RoleOption{
			Value:    id,
			Label:    RoleName(role, code, catalog),
			Selected: strings.EqualFold(id, selected),
		})
	}

	return options
}

func FormatUpdatedAt(moment time.Time) string {
	return moment.UTC().Format(time.RFC3339Nano)
}
