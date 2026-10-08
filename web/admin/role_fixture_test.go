package admin

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/users"
)

func (r *httpCredentials) Create(_ context.Context, role *users.Role) error {
	r.roleWrites++
	r.otherRoles = append(r.otherRoles, role)
	return nil
}

func (r *httpCredentials) Update(_ context.Context, role *users.Role) error {
	r.roleWrites++

	if r.role.ID() == role.ID() {
		r.role = role
	}

	for index, stored := range r.otherRoles {
		if stored.ID() == role.ID() {
			r.otherRoles[index] = role
		}
	}

	return nil
}

func (r *httpCredentials) Delete(_ context.Context, id users.RoleID) error {
	r.roleWrites++
	err := r.deleteErr

	if err == nil && r.role.ID() == id {
		err = users.ErrRoleAlreadyInUse
	}

	if err == nil {
		r.otherRoles = slices.DeleteFunc(r.otherRoles, func(role *users.Role) bool { return role.ID() == id })
	}

	return err
}

func (r *httpCredentials) matchingRoles(filters users.RoleFilters) []*users.Role {
	result := make([]*users.Role, 0)

	for _, role := range append([]*users.Role{r.role}, r.otherRoles...) {
		matches := strings.Contains(uuid.UUID(role.ID()).String(), strings.ToLower(filters.IDLike))
		nameMatches := filters.NameLike == ""

		for _, translation := range role.Name().Translations() {
			nameMatches = nameMatches || strings.Contains(strings.ToLower(translation.Content()), strings.ToLower(filters.NameLike))
		}

		matches = matches && nameMatches && (filters.IsSuper == nil || *filters.IsSuper == role.IsSuper())

		for _, permission := range filters.Permissions {
			matches = matches && (role.IsSuper() || slices.Contains(role.Permissions(), permission))
		}

		if matches {
			result = append(result, role)
		}
	}

	return result
}

func (r *httpCredentials) Count(_ context.Context, filters users.RoleFilters) (int, error) {
	r.roleReads++
	r.lastFilters = filters
	return len(r.matchingRoles(filters)), r.roleListErr
}

func (r *httpCredentials) Find(_ context.Context, query users.RoleQuery) ([]*users.Role, error) {
	r.roleReads++
	r.lastQuery = query
	result := r.matchingRoles(query.Filters)
	slices.SortFunc(result, func(a, b *users.Role) int {
		order := strings.Compare(uuid.UUID(a.ID()).String(), uuid.UUID(b.ID()).String())

		if query.Sort == users.RoleSortName {
			order = strings.Compare(a.Name().Translations()[0].Content(), b.Name().Translations()[0].Content())
		}

		if query.Descending {
			order = -order
		}

		return order
	})
	start := min((query.Page-1)*query.PageSize, len(result))
	return result[start:min(start+query.PageSize, len(result))], r.roleListErr
}

type httpLanguages struct{}

func (httpLanguages) Create(context.Context, *languages.Language) error { return nil }
func (httpLanguages) Update(context.Context, *languages.Language) error { return nil }
func (httpLanguages) Delete(context.Context, languages.Code) error      { return nil }
func (httpLanguages) FindByCode(context.Context, languages.Code) (*languages.Language, error) {
	return nil, languages.ErrLanguageNotFound
}
func (httpLanguages) FindFallback(context.Context) (*languages.Language, error) {
	return nil, languages.ErrLanguageNotFound
}
func (httpLanguages) FindAll(context.Context) ([]*languages.Language, error) {
	return []*languages.Language{
		languages.NewLanguage("en", "English", "English", true),
		languages.NewLanguage("lv", "Latvian", "Latviešu", false),
	}, nil
}
