package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
)

type fakeRoleStore struct {
	ctx                                      context.Context
	id                                       security.RoleID
	stored, written                          *security.Role
	createErr, updateErr, findErr, deleteErr error
	creates, updates, finds, deletes         int
	query                                    security.RoleQuery
	filters                                  security.RoleFilters
	roles                                    []*security.Role
	total                                    int
	listErr, countErr                        error
}

func (f *fakeRoleStore) Create(ctx context.Context, role *security.Role) error {
	f.ctx, f.written = ctx, role
	f.creates++
	return f.createErr
}

func (f *fakeRoleStore) Update(ctx context.Context, role *security.Role) error {
	f.ctx, f.written = ctx, role
	f.updates++
	return f.updateErr
}

func (f *fakeRoleStore) FindByID(ctx context.Context, id security.RoleID) (*security.Role, error) {
	f.ctx, f.id = ctx, id
	f.finds++
	return f.stored, f.findErr
}

func (f *fakeRoleStore) Delete(ctx context.Context, id security.RoleID) error {
	f.ctx, f.id = ctx, id
	f.deletes++
	return f.deleteErr
}

func TestRoleServiceCreate(t *testing.T) {
	ctx := context.Background()
	valid := input.CreateRoleInput{Name: map[string]string{"en": "Administrator", "lv": "Administrators"}, Permissions: []security.Permission{security.PermissionViewUser}, IsSuper: true}

	for _, tt := range []struct {
		name           string
		request        input.CreateRoleInput
		storeErr, want error
		writes         int
	}{
		{name: "created", request: valid, writes: 1},
		{name: "invalid input", want: input.ErrInvalidCreateRoleInput},
		{name: "duplicate", request: valid, storeErr: security.ErrRoleAlreadyExists, want: security.ErrRoleAlreadyExists, writes: 1},
		{name: "missing language", request: valid, storeErr: languages.ErrLanguageNotFound, want: languages.ErrLanguageNotFound, writes: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeRoleStore{createErr: tt.storeErr}
			service, err := NewRoleService(store)

			if err != nil {
				t.Fatal(err)
			}

			role, err := service.Create(ctx, tt.request)

			if !errors.Is(err, tt.want) || store.creates != tt.writes {
				t.Fatalf("Create() = %v, %v; store = %+v", role, err, store)
			}

			if err == nil {
				if role != store.written || uuid.UUID(role.ID()) == uuid.Nil || role.Name()["lv"].Content() != "Administrators" || !role.IsSuper() || !slices.Equal(role.Permissions(), valid.Permissions) || store.ctx != ctx {
					t.Fatalf("created role = %v, store = %+v", role, store)
				}

				valid.Name["en"] = "Changed"

				if role.Name()["en"].Content() != "Administrator" {
					t.Fatal("input mutated role")
				}

				valid.Name["en"] = "Administrator"
			} else if role != nil {
				t.Fatal("failed create returned a role")
			}
		})
	}
}

func TestRoleServiceUpdate(t *testing.T) {
	ctx := context.Background()
	setFalse := false

	for _, tt := range []struct {
		name                     string
		request                  input.UpdateRoleInput
		findErr, updateErr, want error
		missing                  bool
		writes                   int
	}{
		{name: "preserve omitted fields", request: input.UpdateRoleInput{ID: security.RoleID{1}}, writes: 1},
		{name: "replace translations", request: input.UpdateRoleInput{ID: security.RoleID{1}, Name: map[string]string{"lv": "Jauna loma"}}, writes: 1},
		{name: "clear permissions and super", request: input.UpdateRoleInput{ID: security.RoleID{1}, Permissions: []security.Permission{}, IsSuper: &setFalse}, writes: 1},
		{name: "invalid input", request: input.UpdateRoleInput{ID: security.RoleID{1}, Name: map[string]string{}}, want: input.ErrInvalidUpdateRoleInput},
		{name: "missing", request: input.UpdateRoleInput{ID: security.RoleID{1}}, missing: true, want: security.ErrRoleNotFound},
		{name: "read failure", request: input.UpdateRoleInput{ID: security.RoleID{1}}, findErr: context.Canceled, want: context.Canceled},
		{name: "write failure", request: input.UpdateRoleInput{ID: security.RoleID{1}}, updateErr: context.Canceled, want: context.Canceled, writes: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stored, err := roleFromInput(security.RoleID{1}, map[string]string{"en": "Old"}, []security.Permission{security.PermissionViewUser}, true)

			if err != nil {
				t.Fatal(err)
			}

			store := &fakeRoleStore{stored: stored, findErr: tt.findErr, updateErr: tt.updateErr}

			if tt.missing {
				store.stored = nil
			}

			service, err := NewRoleService(store)

			if err != nil {
				t.Fatal(err)
			}

			role, err := service.Update(ctx, tt.request)

			if !errors.Is(err, tt.want) || store.updates != tt.writes {
				t.Fatalf("Update() = %v, %v; store = %+v", role, err, store)
			}

			if err == nil {
				if role.ID() != tt.request.ID || role != store.written || role == stored || store.ctx != ctx {
					t.Fatal("incorrect update identity or context")
				}

				if tt.request.Name == nil {
					if role.Name()["en"].Content() != "Old" {
						t.Fatal("omitted name changed")
					}
				} else if len(role.Name()) != 1 || role.Name()["lv"].Content() != "Jauna loma" {
					t.Fatal("name was not replaced")
				}

				if tt.request.Permissions == nil {
					if !slices.Equal(role.Permissions(), stored.Permissions()) {
						t.Fatal("omitted permissions changed")
					}
				} else if len(role.Permissions()) != 0 {
					t.Fatal("permissions not cleared")
				}

				if (tt.request.IsSuper == nil && !role.IsSuper()) || (tt.request.IsSuper != nil && role.IsSuper() != *tt.request.IsSuper) {
					t.Fatal("incorrect super flag")
				}
			} else if role != nil {
				t.Fatal("failed update returned a role")
			}

			if stored.Name()["en"].Content() != "Old" || len(stored.Permissions()) != 1 || !stored.IsSuper() {
				t.Fatal("update mutated stored object")
			}

			if errors.Is(tt.want, input.ErrInvalidUpdateRoleInput) && store.finds != 0 {
				t.Fatal("invalid input accessed repository")
			}
		})
	}
}

func TestRoleServiceFindAndDelete(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name           string
		id             security.RoleID
		storeErr, want error
	}{
		{name: "valid ID", id: security.RoleID{1}},
		{name: "invalid ID", want: security.ErrInvalidRoleID},
		{name: "missing", id: security.RoleID{1}, storeErr: security.ErrRoleNotFound, want: security.ErrRoleNotFound},
		{name: "assigned", id: security.RoleID{1}, storeErr: security.ErrRoleAlreadyInUse, want: security.ErrRoleAlreadyInUse},
		{name: "storage failure", id: security.RoleID{1}, storeErr: context.Canceled, want: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stored, err := roleFromInput(security.RoleID{1}, map[string]string{"en": "Role"}, nil, false)

			if err != nil {
				t.Fatal(err)
			}

			store := &fakeRoleStore{stored: stored, findErr: tt.storeErr, deleteErr: tt.storeErr}
			service, err := NewRoleService(store)

			if err != nil {
				t.Fatal(err)
			}

			role, findErr := service.FindByID(ctx, tt.id)
			deleteErr := service.Delete(ctx, tt.id)

			if !errors.Is(findErr, tt.want) || !errors.Is(deleteErr, tt.want) || (findErr != nil && role != nil) {
				t.Fatalf("FindByID() = %v, %v; Delete() = %v", role, findErr, deleteErr)
			}

			if tt.id == (security.RoleID{}) {
				if store.finds != 0 || store.deletes != 0 {
					t.Fatal("invalid ID accessed repository")
				}
			} else if store.ctx != ctx || store.id != tt.id || store.finds != 1 || store.deletes != 1 {
				t.Fatal("incorrect ID or context")
			}
		})
	}
}

func TestRoleServiceMissingDependencies(t *testing.T) {
	service, err := NewRoleService(nil)

	if service != nil || !errors.Is(err, ErrNilRoleRepository) {
		t.Fatal("nil repository accepted")
	}

	for _, tt := range []struct {
		name    string
		service *RoleService
	}{
		{name: "nil service"}, {name: "zero service", service: &RoleService{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, createErr := tt.service.Create(context.Background(), input.CreateRoleInput{})
			_, findErr := tt.service.FindByID(context.Background(), security.RoleID{1})
			_, updateErr := tt.service.Update(context.Background(), input.UpdateRoleInput{})
			deleteErr := tt.service.Delete(context.Background(), security.RoleID{1})

			for _, err := range []error{createErr, findErr, updateErr, deleteErr} {
				if !errors.Is(err, ErrNilRoleRepository) {
					t.Fatalf("error = %v", err)
				}
			}
		})
	}

	store := &fakeRoleStore{}
	service, err = NewRoleService(store)

	if err != nil {
		t.Fatal(err)
	}

	if role, err := service.FindByID(context.Background(), security.RoleID{1}); role != nil || !errors.Is(err, security.ErrRoleNotFound) {
		t.Fatalf("nil lookup = %v, %v", role, err)
	}
}

func (f *fakeRoleStore) Find(ctx context.Context, query security.RoleQuery) ([]*security.Role, error) {
	f.ctx, f.query = ctx, query
	return f.roles, f.listErr
}

func (f *fakeRoleStore) Count(ctx context.Context, filters security.RoleFilters) (int, error) {
	f.ctx, f.filters = ctx, filters
	return f.total, f.countErr
}

func TestRoleServiceList(t *testing.T) {
	for _, tt := range []struct {
		name                       string
		total, requested, wantPage int
		countErr, listErr, want    error
	}{
		{name: "first", total: 30, requested: 1, wantPage: 1},
		{name: "last", total: 30, requested: 999, wantPage: 2},
		{name: "empty", requested: 999, wantPage: 1},
		{name: "count failure", requested: 1, countErr: context.Canceled, want: context.Canceled},
		{name: "list failure", requested: 1, listErr: context.Canceled, want: context.Canceled},
		{name: "bad query", requested: 0, want: security.ErrInvalidRoleQuery},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeRoleStore{total: tt.total, countErr: tt.countErr, listErr: tt.listErr}
			service, err := NewRoleService(store)

			if err != nil {
				t.Fatal(err)
			}

			query := security.RoleQuery{Filters: security.RoleFilters{NameLike: "Test"}, Sort: security.RoleSortName, Language: "en", Page: tt.requested, PageSize: 25}
			page, err := service.List(t.Context(), query)

			if !errors.Is(err, tt.want) {
				t.Fatalf("List() = %v, want %v", err, tt.want)
			}

			if err == nil && (page.Page != tt.wantPage || page.Total != tt.total || store.query.Page != tt.wantPage || store.filters.NameLike != "Test" || store.ctx != t.Context()) {
				t.Fatalf("page = %+v, query = %+v", page, store.query)
			}
		})
	}
}
