package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/users/mock"
	"github.com/radynsade/faryengo/pkg/domquery"
)

func TestNewRoleService(t *testing.T) {
	for _, tt := range []struct {
		name       string
		transactor Transactor
		repository users.RoleRepository
		wantErr    error
	}{
		{name: "created", transactor: &appmock.Transactor{}, repository: &mock.RoleRepository{}},
		{name: "nil transactor", repository: &mock.RoleRepository{}, wantErr: ErrTransactorNil},
		{name: "nil repository", transactor: &appmock.Transactor{}, wantErr: ErrRoleRepositoryNil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewRoleService(tt.transactor, tt.repository)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) || (service == nil) != (tt.wantErr != nil) {
				t.Fatalf("NewRoleService() = (%v, %v), want error %v", service, err, tt.wantErr)
			}
		})
	}
}

func TestRoleServiceNilReceiver(t *testing.T) {
	ctx := t.Context()

	var service *RoleService

	created, createErr := service.Create(ctx, input.CreateRoleInput{})
	found, findErr := service.FindByID(ctx, testRoleID)
	list, listErr := service.List(ctx, validRoleQuery())
	updated, updateErr := service.Update(ctx, input.UpdateRoleInput{})
	deleteErr := service.Delete(ctx, testRoleID)

	for _, err := range []error{createErr, findErr, listErr, updateErr, deleteErr} {
		if !errors.Is(err, ErrRoleRepositoryNil) {
			t.Fatalf("nil receiver error = %v, want ErrRoleRepositoryNil", err)
		}
	}

	if created != nil || found != nil || list.Roles != nil || updated != nil {
		t.Fatalf("nil receiver returned values: %v, %v, %v, %v", created, found, list, updated)
	}
}

func TestRoleServiceCreate(t *testing.T) {
	valid := input.CreateRoleInput{
		Name:        map[string]string{"en": "Editor", "lv": "Redaktors"},
		Permissions: []string{"view_user", "manage_role"},
	}

	for _, tt := range []struct {
		name        string
		request     input.CreateRoleInput
		beginErr    error
		createErr   error
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{name: "created", request: valid, wantTxCalls: 1, wantWrites: 1},
		{
			name:        "created super without permissions",
			request:     input.CreateRoleInput{Name: map[string]string{"en": "Administrator"}, IsSuper: true},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:     "all fields invalid",
			request:  input.CreateRoleInput{Permissions: []string{"view_user", "fly"}},
			wantErrs: []error{input.ErrCreateRoleInputInvalid, languages.ErrTextNil, users.ErrPermissionInvalid},
		},
		{
			name:     "invalid translation code",
			request:  input.CreateRoleInput{Name: map[string]string{"EN": "Editor"}},
			wantErrs: []error{input.ErrCreateRoleInputInvalid, languages.ErrCodeInvalid},
		},
		{
			name:     "long name",
			request:  input.CreateRoleInput{Name: map[string]string{"en": strings.Repeat("a", 101)}},
			wantErrs: []error{input.ErrCreateRoleInputInvalid, users.ErrRoleNameTooLong},
		},
		{
			name:        "transaction not started",
			request:     valid,
			beginErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
		},
		{
			name:        "missing language",
			request:     valid,
			createErr:   languages.ErrLanguageNotFound,
			wantErrs:    []error{languages.ErrLanguageNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0

			var stored *users.Role

			repository := &mock.RoleRepository{
				CreateFunc: func(ctx context.Context, role *users.Role) users.ErrRoleCreateFailed {
					var err users.ErrRoleCreateFailed

					writes++
					stored = role

					if !appmock.InTransaction(ctx) {
						t.Error("Create() ran outside a transaction")
					}

					if tt.createErr != nil {
						err = mock.NewErrRoleCreateFailed(role, tt.createErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{BeginErr: tt.beginErr}
			service, err := NewRoleService(transactor, repository)

			if err != nil {
				t.Fatal(err)
			}

			role, err := service.Create(t.Context(), tt.request)

			if transactor.Calls != tt.wantTxCalls || writes != tt.wantWrites {
				t.Fatalf("Create() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if role != nil {
					t.Fatalf("Create() role = %v, want nil on error", role)
				}
			} else {
				assertRole(t, role, tt.request.Name, tt.request.Permissions, tt.request.IsSuper)

				if role != stored || uuid.UUID(role.ID).Version() != 7 {
					t.Fatalf("Create() role = %+v, stored = %+v, want the stored role with a UUIDv7", role, stored)
				}
			}
		})
	}
}

func TestRoleServiceFindByID(t *testing.T) {
	existing := users.NewRole(testRoleID, users.RoleName{"en": "Editor"}, nil, false)

	for _, tt := range []struct {
		name      string
		id        users.RoleID
		findErr   error
		wantErrs  []error
		wantCalls int
	}{
		{name: "found", id: testRoleID, wantCalls: 1},
		{name: "invalid ID", wantErrs: []error{users.ErrRoleIDInvalid}},
		{
			name:      "not found",
			id:        testRoleID,
			findErr:   users.ErrRoleNotFound,
			wantErrs:  []error{users.ErrRoleNotFound},
			wantCalls: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			calls := 0

			repository := &mock.RoleRepository{
				FindByIDFunc: func(callCtx context.Context, id users.RoleID) (*users.Role, error) {
					var role *users.Role

					calls++

					if callCtx != ctx || id != tt.id {
						t.Errorf("FindByID() called with (%v, %v)", callCtx, id)
					}

					if tt.findErr == nil {
						role = existing
					}

					return role, tt.findErr
				},
			}

			service, err := NewRoleService(&appmock.Transactor{}, repository)

			if err != nil {
				t.Fatal(err)
			}

			role, err := service.FindByID(ctx, tt.id)

			if calls != tt.wantCalls {
				t.Fatalf("FindByID() repository calls = %d, want %d", calls, tt.wantCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && role != nil {
				t.Fatalf("FindByID() role = %v, want nil on error", role)
			} else if len(tt.wantErrs) == 0 && role != existing {
				t.Fatalf("FindByID() role = %v, want %v", role, existing)
			}
		})
	}
}

func TestRoleServiceList(t *testing.T) {
	roles := []*users.Role{
		users.NewRole(testRoleID, users.RoleName{"en": "Editor"}, nil, false),
	}
	withoutPage := validRoleQuery()
	withoutPage.Page = 0
	unknownSort := validRoleQuery()
	unknownSort.SortBy = "email"
	longFilter := validRoleQuery()
	longFilter.Filter.NameLike = strings.Repeat("a", users.MaxRoleFilterNameLikeLength+1)

	for _, tt := range []struct {
		name       string
		query      users.RoleQuery
		countErr   error
		findErr    error
		wantErrs   []error
		wantCalls  int
		wantResult RoleList
	}{
		{name: "listed", query: validRoleQuery(), wantCalls: 2, wantResult: RoleList{Roles: roles, Total: 7}},
		{name: "missing page", query: withoutPage, wantErrs: []error{users.ErrInvalidRoleQuery}},
		{name: "unknown sort", query: unknownSort, wantErrs: []error{users.ErrInvalidRoleQuery}},
		{name: "invalid filter", query: longFilter, wantErrs: []error{users.ErrRoleFilterNameLikeTooLong}},
		{
			name:      "count failure",
			query:     validRoleQuery(),
			countErr:  context.DeadlineExceeded,
			wantErrs:  []error{context.DeadlineExceeded},
			wantCalls: 1,
		},
		{
			name:      "find failure",
			query:     validRoleQuery(),
			findErr:   context.DeadlineExceeded,
			wantErrs:  []error{context.DeadlineExceeded},
			wantCalls: 2,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			calls := 0

			repository := &mock.RoleRepository{
				CountFunc: func(callCtx context.Context, filter users.RoleFilter) (int, error) {
					calls++

					if callCtx != ctx || filter.NameLike != tt.query.Filter.NameLike {
						t.Errorf("Count() called with (%v, %+v)", callCtx, filter)
					}

					return 7, tt.countErr
				},
				FindFunc: func(callCtx context.Context, query users.RoleQuery) ([]*users.Role, error) {
					calls++

					if callCtx != ctx || query.Page != tt.query.Page || query.SortBy != tt.query.SortBy {
						t.Errorf("Find() called with (%v, %+v)", callCtx, query)
					}

					return roles, tt.findErr
				},
			}

			service, err := NewRoleService(&appmock.Transactor{}, repository)

			if err != nil {
				t.Fatal(err)
			}

			list, err := service.List(ctx, tt.query)

			if calls != tt.wantCalls {
				t.Fatalf("List() repository calls = %d, want %d", calls, tt.wantCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if list.Total != tt.wantResult.Total || len(list.Roles) != len(tt.wantResult.Roles) {
				t.Fatalf("List() = %+v, want %+v", list, tt.wantResult)
			}
		})
	}
}

func TestRoleServiceUpdate(t *testing.T) {
	valid := input.UpdateRoleInput{
		ID:          testRoleID,
		Name:        map[string]string{"en": "Reviewer"},
		Permissions: []string{},
		IsSuper:     true,
	}

	for _, tt := range []struct {
		name        string
		request     input.UpdateRoleInput
		lockErr     error
		updateErr   error
		wantErrs    []error
		wantTxCalls int
		wantLocks   int
		wantWrites  int
	}{
		{name: "updated", request: valid, wantTxCalls: 1, wantLocks: 1, wantWrites: 1},
		{
			name:     "invalid fields",
			request:  input.UpdateRoleInput{Name: map[string]string{}, Permissions: []string{"fly"}},
			wantErrs: []error{input.ErrUpdateRoleInputInvalid, users.ErrRoleIDInvalid, languages.ErrTextWithoutTranslations, users.ErrPermissionInvalid},
		},
		{
			name:        "not found",
			request:     valid,
			lockErr:     users.ErrRoleNotFound,
			wantErrs:    []error{users.ErrRoleNotFound},
			wantTxCalls: 1,
			wantLocks:   1,
		},
		{
			name:        "missing language",
			request:     valid,
			updateErr:   languages.ErrLanguageNotFound,
			wantErrs:    []error{languages.ErrLanguageNotFound},
			wantTxCalls: 1,
			wantLocks:   1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			locks, writes := 0, 0

			var stored *users.Role

			repository := &mock.RoleRepository{
				FindByIDForUpdateFunc: func(ctx context.Context, id users.RoleID) (*users.Role, error) {
					var role *users.Role

					locks++

					if !appmock.InTransaction(ctx) || id != tt.request.ID {
						t.Errorf("FindByIDForUpdate() called with (%v, %v), want a transaction", ctx, id)
					}

					if tt.lockErr == nil {
						role = users.NewRole(id, users.RoleName{"en": "Old"}, users.Permissions{users.PermissionViewUser}, false)
					}

					return role, tt.lockErr
				},
				UpdateFunc: func(ctx context.Context, role *users.Role) users.ErrRoleUpdateFailed {
					var err users.ErrRoleUpdateFailed

					writes++
					stored = role

					if !appmock.InTransaction(ctx) {
						t.Error("Update() ran outside a transaction")
					}

					if tt.updateErr != nil {
						err = mock.NewErrRoleUpdateFailed(role, tt.updateErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewRoleService(transactor, repository)

			if err != nil {
				t.Fatal(err)
			}

			role, err := service.Update(t.Context(), tt.request)

			if transactor.Calls != tt.wantTxCalls || locks != tt.wantLocks || writes != tt.wantWrites {
				t.Fatalf(
					"Update() calls = (transaction %d, lock %d, write %d), want (%d, %d, %d)",
					transactor.Calls, locks, writes, tt.wantTxCalls, tt.wantLocks, tt.wantWrites,
				)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if role != nil {
					t.Fatalf("Update() role = %v, want nil on error", role)
				}
			} else {
				assertRole(t, role, tt.request.Name, tt.request.Permissions, tt.request.IsSuper)

				if role != stored || role.ID != tt.request.ID {
					t.Fatalf("Update() role = %+v, stored = %+v, want the stored role %v", role, stored, tt.request.ID)
				}
			}
		})
	}
}

func TestRoleServiceDelete(t *testing.T) {
	for _, tt := range []struct {
		name        string
		id          users.RoleID
		deleteErr   error
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{name: "deleted", id: testRoleID, wantTxCalls: 1, wantWrites: 1},
		{name: "invalid ID", wantErrs: []error{users.ErrRoleIDInvalid}},
		{
			name:        "assigned to users",
			id:          testRoleID,
			deleteErr:   users.ErrRoleAlreadyInUse,
			wantErrs:    []error{users.ErrRoleAlreadyInUse},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "not found",
			id:          testRoleID,
			deleteErr:   users.ErrRoleNotFound,
			wantErrs:    []error{users.ErrRoleNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0

			repository := &mock.RoleRepository{
				DeleteFunc: func(ctx context.Context, id users.RoleID) users.ErrRoleDeleteFailed {
					var err users.ErrRoleDeleteFailed

					writes++

					if !appmock.InTransaction(ctx) || id != tt.id {
						t.Errorf("Delete() called with (%v, %v), want a transaction", ctx, id)
					}

					if tt.deleteErr != nil {
						err = mock.NewErrRoleDeleteFailed(id, tt.deleteErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewRoleService(transactor, repository)

			if err != nil {
				t.Fatal(err)
			}

			err = service.Delete(t.Context(), tt.id)

			if transactor.Calls != tt.wantTxCalls || writes != tt.wantWrites {
				t.Fatalf("Delete() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)
		})
	}
}

func validRoleQuery() users.RoleQuery {
	return users.RoleQuery{
		Filter:    users.RoleFilter{NameLike: "edit"},
		SortOrder: domquery.SortOrderAsc,
		SortBy:    users.RoleSortName,
		Limit:     20,
		Page:      1,
	}
}

func assertRole(
	t *testing.T,
	role *users.Role,
	name map[string]string,
	permissions []string,
	isSuper bool,
) {
	t.Helper()

	if role == nil || len(role.Name) != len(name) || len(role.Permissions) != len(permissions) || role.IsSuper != isSuper {
		t.Fatalf("role = %+v, want name %v, permissions %v, super %v", role, name, permissions, isSuper)
	}

	for code, translation := range name {
		if role.Name[languages.Code(code)] != languages.Translation(translation) {
			t.Fatalf("role name = %v, want %v", role.Name, name)
		}
	}

	for index, permission := range permissions {
		if role.Permissions[index] != users.Permission(permission) {
			t.Fatalf("role permissions = %v, want %v", role.Permissions, permissions)
		}
	}
}
