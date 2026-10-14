package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/categories"
	categoriesmock "github.com/radynsade/faryengo/internal/categories/mock"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	workspacesmock "github.com/radynsade/faryengo/internal/workspaces/mock"
	"github.com/radynsade/faryengo/pkg/domquery"
)

var (
	testCategoryID        = categories.CategoryID(uuid.MustParse("01920000-0000-7000-8000-000000000004"))
	testMissingCategoryID = categories.CategoryID(uuid.MustParse("01920000-0000-7000-8000-0000000000fe"))
)

func TestNewCategoryService(t *testing.T) {
	for _, tt := range []struct {
		name       string
		transactor Transactor
		repository categories.CategoryRepository
		workspaces workspaces.WorkspaceRepository
		wantErr    error
	}{
		{name: "created", transactor: &appmock.Transactor{}, repository: &categoriesmock.CategoryRepository{}, workspaces: &workspacesmock.WorkspaceRepository{}},
		{name: "nil transactor", repository: &categoriesmock.CategoryRepository{}, workspaces: &workspacesmock.WorkspaceRepository{}, wantErr: ErrTransactorNil},
		{name: "nil repository", transactor: &appmock.Transactor{}, workspaces: &workspacesmock.WorkspaceRepository{}, wantErr: ErrCategoryRepositoryNil},
		{name: "nil workspace repository", transactor: &appmock.Transactor{}, repository: &categoriesmock.CategoryRepository{}, wantErr: ErrWorkspaceRepositoryNil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewCategoryService(tt.transactor, tt.repository, tt.workspaces)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) || (service == nil) != (tt.wantErr != nil) {
				t.Fatalf("NewCategoryService() = (%v, %v), want error %v", service, err, tt.wantErr)
			}
		})
	}
}

func TestCategoryServiceNilReceiver(t *testing.T) {
	ctx := memberContext(t, testWorkspaceID, testOwnerID, workspaces.RoleOwner)

	var service *CategoryService

	created, createErr := service.Create(ctx, validCreateCategoryInput())
	found, findErr := service.FindByID(ctx, testCategoryID)
	list, listErr := service.List(ctx, testWorkspaceID, validCategoryQuery())
	updated, updateErr := service.Update(ctx, validUpdateCategoryInput())
	deleteErr := service.Delete(ctx, testCategoryID)

	for _, err := range []error{createErr, findErr, listErr, updateErr, deleteErr} {
		if !errors.Is(err, ErrCategoryRepositoryNil) {
			t.Fatalf("nil receiver error = %v, want ErrCategoryRepositoryNil", err)
		}
	}

	if created != nil || found != nil || list.Categories != nil || updated != nil {
		t.Fatalf("nil receiver returned values: %v, %v, %v, %v", created, found, list, updated)
	}
}

func TestCategoryServiceRolePermissions(t *testing.T) {
	operations := categoryOperations()

	for _, tt := range []struct {
		role    workspaces.Role
		actorID users.UserID
		allowed []bool
	}{
		{workspaces.RoleViewer, testViewerID, []bool{true, true, false, false, false}},
		{workspaces.RoleEditor, testEditorID, []bool{true, true, true, true, true}},
		{workspaces.RoleOwner, testOwnerID, []bool{true, true, true, true, true}},
	} {
		for index, operation := range operations {
			t.Run(string(tt.role)+" "+operation.name, func(t *testing.T) {
				workspaceStore := newWorkspaceStore(t, testMemberships()...)
				store := newCategoryStore(t, workspaceStore)
				service := newCategoryStoreService(t, store, workspaceStore)

				err := operation.call(memberContext(t, testWorkspaceID, tt.actorID, tt.role), service)

				if tt.allowed[index] {
					if err != nil {
						t.Fatalf("%s as %s = %v, want it allowed", operation.name, tt.role, err)
					}

					if operation.write && store.writes != 1 {
						t.Fatalf("%s as %s wrote %d times, want once", operation.name, tt.role, store.writes)
					}
				} else if !errors.Is(err, workspaces.ErrRolePermissionDenied) || errors.Is(err, operation.notFound) || store.writes != 0 {
					t.Fatalf("%s as %s = %v after %d writes, want %v and no write", operation.name, tt.role, err, store.writes, workspaces.ErrRolePermissionDenied)
				}
			})
		}
	}

	// A caller whose identity is for another workspace learns nothing about
	// this one or its categories.
	for _, operation := range operations {
		t.Run("identity for another workspace "+operation.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newCategoryStore(t, workspaceStore)
			service := newCategoryStoreService(t, store, workspaceStore)
			otherWorkspaceID := workspaces.WorkspaceID(uuid.MustParse("01920000-0000-7000-8000-0000000000ff"))

			err := operation.call(memberContext(t, otherWorkspaceID, testOutsiderID, workspaces.RoleOwner), service)

			if !errors.Is(err, operation.notFound) || errors.Is(err, workspaces.ErrRolePermissionDenied) || store.writes != 0 {
				t.Fatalf("%s = %v after %d writes, want %v and no write", operation.name, err, store.writes, operation.notFound)
			}
		})
	}

	// A caller without a workspace identity reads and writes nothing.
	for _, operation := range operations {
		t.Run("missing identity "+operation.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newCategoryStore(t, workspaceStore)
			store.readErr = errors.New("read without an identity")
			service := newCategoryStoreService(t, store, workspaceStore)

			err := operation.call(userContext(t, testEditorID), service)

			if !errors.Is(err, ErrUnauthenticated) || !errors.Is(err, workspaces.ErrIdentityMissing) || store.writes != 0 {
				t.Fatalf("%s = %v after %d writes, want %v and no write", operation.name, err, store.writes, ErrUnauthenticated)
			}
		})
	}
}

// The editor's identity was resolved before the write, and the membership has
// changed since. Each write reads the role again under the workspace's lock,
// so it is refused and writes nothing.

func TestCategoryServiceStaleIdentity(t *testing.T) {
	for _, tt := range []struct {
		name    string
		change  func(store *workspaceStore)
		wantErr error
	}{
		{"demoted", func(store *workspaceStore) { store.memberships[store.index(testEditorID)].Role = workspaces.RoleViewer }, workspaces.ErrRolePermissionDenied},
		{"removed", func(store *workspaceStore) { store.memberships = slices.Delete(store.memberships, 1, 2) }, nil},
	} {
		for _, operation := range categoryOperations() {
			if operation.write {
				t.Run(tt.name+" "+operation.name, func(t *testing.T) {
					wantErr := tt.wantErr

					if wantErr == nil {
						wantErr = operation.notFound
					}

					workspaceStore := newWorkspaceStore(t, testMemberships()...)
					tt.change(workspaceStore)
					store := newCategoryStore(t, workspaceStore)
					service := newCategoryStoreService(t, store, workspaceStore)

					err := operation.call(memberContext(t, testWorkspaceID, testEditorID, workspaces.RoleEditor), service)

					if !errors.Is(err, wantErr) || !workspaceStore.locked || store.writes != 0 {
						t.Fatalf("%s = %v after %d writes, locked %t, want %v and no write", operation.name, err, store.writes, workspaceStore.locked, wantErr)
					}
				})
			}
		}
	}
}

func TestCategoryServiceCreate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		request     input.CreateCategoryInput
		lockErr     error
		writeErr    error
		wantIcon    *categories.Icon
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{
			name:        "created with an icon",
			request:     input.CreateCategoryInput{WorkspaceID: testWorkspaceID, Name: " Groceries ", Icon: "shopping-cart", Color: "green"},
			wantIcon:    new(categories.IconShoppingCart),
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "created without an icon",
			request:     input.CreateCategoryInput{WorkspaceID: testWorkspaceID, Name: "Groceries", Color: "green"},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:    "all fields invalid",
			request: input.CreateCategoryInput{Name: "  ", Icon: "rocket", Color: "pink"},
			wantErrs: []error{
				input.ErrCreateCategoryInputInvalid,
				workspaces.ErrWorkspaceIDInvalid,
				categories.ErrCategoryNameEmpty,
				categories.ErrIconInvalid,
				categories.ErrColorInvalid,
			},
		},
		{
			name:     "long name",
			request:  input.CreateCategoryInput{WorkspaceID: testWorkspaceID, Name: strings.Repeat("a", categories.MaxCategoryNameLength+1), Color: "red"},
			wantErrs: []error{input.ErrCreateCategoryInputInvalid, categories.ErrCategoryNameTooLong},
		},
		{
			name:     "missing color",
			request:  input.CreateCategoryInput{WorkspaceID: testWorkspaceID, Name: "Groceries"},
			wantErrs: []error{input.ErrCreateCategoryInputInvalid, categories.ErrColorInvalid},
		},
		{
			name:        "missing workspace",
			request:     input.CreateCategoryInput{WorkspaceID: workspaces.WorkspaceID(uuid.MustParse("01920000-0000-7000-8000-0000000000ff")), Name: "Groceries", Color: "green"},
			wantErrs:    []error{workspaces.ErrWorkspaceNotFound},
			wantTxCalls: 1,
		},
		{
			name:        "lock failure",
			request:     validCreateCategoryInput(),
			lockErr:     context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
		},
		{
			name:        "create failure",
			request:     validCreateCategoryInput(),
			writeErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			workspaceStore.lockErr = tt.lockErr
			store := newCategoryStore(t, workspaceStore)
			store.writeErr = tt.writeErr
			delete(store.categories, testCategoryID)
			transactor := &appmock.Transactor{}
			service, err := NewCategoryService(transactor, store.repository(), workspaceStore.repository())

			if err != nil {
				t.Fatal(err)
			}

			category, err := service.Create(memberContext(t, tt.request.WorkspaceID, testEditorID, workspaces.RoleEditor), tt.request)

			if transactor.Calls != tt.wantTxCalls || store.writes != tt.wantWrites {
				t.Fatalf("Create() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, store.writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if category != nil || len(store.categories) != 0 {
					t.Fatalf("Create() = %v with %d stored, want nil and nothing stored", category, len(store.categories))
				}
			} else {
				stored := store.categories[category.ID]

				if stored != category || category.WorkspaceID != testWorkspaceID || category.Name != "Groceries" ||
					(category.Icon == nil) != (tt.wantIcon == nil) || (category.Icon != nil && *category.Icon != *tt.wantIcon) {
					t.Fatalf("Create() = %+v, stored %+v", category, stored)
				}

				if uuid.UUID(category.ID).Version() != 7 {
					t.Fatalf("Create() ID version = %d, want 7", uuid.UUID(category.ID).Version())
				}
			}
		})
	}
}

func TestCategoryServiceFindByID(t *testing.T) {
	viewer := memberContext(t, testWorkspaceID, testViewerID, workspaces.RoleViewer)

	for _, tt := range []struct {
		name     string
		ctx      context.Context
		id       categories.CategoryID
		readErr  error
		wantErrs []error
	}{
		{name: "found", ctx: viewer, id: testCategoryID},
		{name: "invalid ID", ctx: viewer, id: categories.CategoryID{}, wantErrs: []error{categories.ErrCategoryIDInvalid}},
		{name: "missing", ctx: viewer, id: testMissingCategoryID, wantErrs: []error{categories.ErrCategoryNotFound}},
		{name: "read failure", ctx: viewer, id: testCategoryID, readErr: context.DeadlineExceeded, wantErrs: []error{context.DeadlineExceeded}},
		{
			name:     "missing identity is checked before the read",
			ctx:      userContext(t, testViewerID),
			id:       testCategoryID,
			readErr:  context.DeadlineExceeded,
			wantErrs: []error{ErrUnauthenticated, workspaces.ErrIdentityMissing},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newCategoryStore(t, workspaceStore)
			store.readErr = tt.readErr
			service := newCategoryStoreService(t, store, workspaceStore)

			category, err := service.FindByID(tt.ctx, tt.id)

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && category != nil {
				t.Fatalf("FindByID() = %v, want nil on error", category)
			} else if len(tt.wantErrs) == 0 && category != store.categories[testCategoryID] {
				t.Fatalf("FindByID() = %+v, want the stored category", category)
			}
		})
	}
}

func TestCategoryServiceList(t *testing.T) {
	otherWorkspaceID := workspaces.WorkspaceID(uuid.MustParse("01920000-0000-7000-8000-0000000000ff"))
	withOtherWorkspace := validCategoryQuery()
	withOtherWorkspace.Filter.WorkspaceID = &otherWorkspaceID
	withoutSort := validCategoryQuery()
	withoutSort.SortBy = ""
	viewer := memberContext(t, testWorkspaceID, testViewerID, workspaces.RoleViewer)

	for _, tt := range []struct {
		name     string
		ctx      context.Context
		query    categories.CategoryQuery
		countErr error
		wantErrs []error
	}{
		{name: "listed", ctx: viewer, query: validCategoryQuery()},
		{name: "workspace criterion replaced", ctx: viewer, query: withOtherWorkspace},
		{name: "invalid query", ctx: viewer, query: withoutSort, wantErrs: []error{categories.ErrCategoryQueryInvalid}},
		{name: "count failure", ctx: viewer, query: validCategoryQuery(), countErr: context.DeadlineExceeded, wantErrs: []error{context.DeadlineExceeded}},
		{
			name:     "missing identity",
			ctx:      userContext(t, testViewerID),
			query:    validCategoryQuery(),
			countErr: context.DeadlineExceeded,
			wantErrs: []error{ErrUnauthenticated, workspaces.ErrIdentityMissing},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var gotFilter categories.CategoryFilter

			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newCategoryStore(t, workspaceStore)
			repository := store.repository()
			repository.CountFunc = func(_ context.Context, filter categories.CategoryFilter) (int, error) {
				gotFilter = filter
				return 1, tt.countErr
			}

			service, err := NewCategoryService(&appmock.Transactor{}, repository, workspaceStore.repository())

			if err != nil {
				t.Fatal(err)
			}

			list, err := service.List(tt.ctx, testWorkspaceID, tt.query)

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if list.Categories != nil || list.Total != 0 {
					t.Fatalf("List() = %+v, want an empty list on error", list)
				}
			} else {
				if gotFilter.WorkspaceID == nil || *gotFilter.WorkspaceID != testWorkspaceID {
					t.Fatalf("Count() filter workspace = %v, want %v", gotFilter.WorkspaceID, testWorkspaceID)
				}

				if list.Total != 1 || len(list.Categories) != 1 || list.Categories[0] != store.categories[testCategoryID] {
					t.Fatalf("List() = %+v, want the stored category and a total of 1", list)
				}
			}
		})
	}
}

func TestCategoryServiceUpdate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		request     input.UpdateCategoryInput
		lockErr     error
		writeErr    error
		wantName    categories.CategoryName
		wantIcon    *categories.Icon
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{
			name:        "updated",
			request:     input.UpdateCategoryInput{ID: testCategoryID, Name: " Transport ", Icon: "bus", Color: "blue"},
			wantName:    "Transport",
			wantIcon:    new(categories.IconBus),
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "icon removed",
			request:     input.UpdateCategoryInput{ID: testCategoryID, Name: "Groceries", Color: "green"},
			wantName:    "Groceries",
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:    "all fields invalid",
			request: input.UpdateCategoryInput{Name: "", Icon: "Bus", Color: ""},
			wantErrs: []error{
				input.ErrUpdateCategoryInputInvalid,
				categories.ErrCategoryIDInvalid,
				categories.ErrCategoryNameEmpty,
				categories.ErrIconInvalid,
				categories.ErrColorInvalid,
			},
			wantName: "Groceries",
			wantIcon: new(categories.IconShoppingCart),
		},
		{
			name:        "missing",
			request:     input.UpdateCategoryInput{ID: testMissingCategoryID, Name: "Transport", Color: "blue"},
			wantErrs:    []error{categories.ErrCategoryNotFound},
			wantName:    "Groceries",
			wantIcon:    new(categories.IconShoppingCart),
			wantTxCalls: 1,
		},
		{
			name:        "lock failure",
			request:     validUpdateCategoryInput(),
			lockErr:     context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantName:    "Groceries",
			wantIcon:    new(categories.IconShoppingCart),
			wantTxCalls: 1,
		},
		{
			name:        "update failure",
			request:     validUpdateCategoryInput(),
			writeErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantName:    "Groceries",
			wantIcon:    new(categories.IconShoppingCart),
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			workspaceStore.lockErr = tt.lockErr
			store := newCategoryStore(t, workspaceStore)
			store.writeErr = tt.writeErr
			transactor := &appmock.Transactor{}
			service, err := NewCategoryService(transactor, store.repository(), workspaceStore.repository())

			if err != nil {
				t.Fatal(err)
			}

			category, err := service.Update(memberContext(t, testWorkspaceID, testEditorID, workspaces.RoleEditor), tt.request)

			if transactor.Calls != tt.wantTxCalls || store.writes != tt.wantWrites {
				t.Fatalf("Update() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, store.writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			stored := store.categories[testCategoryID]

			if stored.Name != tt.wantName || stored.WorkspaceID != testWorkspaceID ||
				(stored.Icon == nil) != (tt.wantIcon == nil) || (stored.Icon != nil && *stored.Icon != *tt.wantIcon) {
				t.Fatalf("stored category = %+v, want name %q and icon %v in workspace %v", stored, tt.wantName, tt.wantIcon, testWorkspaceID)
			}

			if len(tt.wantErrs) > 0 && category != nil {
				t.Fatalf("Update() = %v, want nil on error", category)
			} else if len(tt.wantErrs) == 0 && category != stored {
				t.Fatalf("Update() = %+v, want the stored category", category)
			}
		})
	}
}

func TestCategoryServiceDelete(t *testing.T) {
	for _, tt := range []struct {
		name        string
		id          categories.CategoryID
		writeErr    error
		wantErrs    []error
		wantStored  bool
		wantTxCalls int
		wantWrites  int
	}{
		{name: "deleted", id: testCategoryID, wantTxCalls: 1, wantWrites: 1},
		{name: "invalid ID", id: categories.CategoryID{}, wantErrs: []error{categories.ErrCategoryIDInvalid}, wantStored: true},
		{name: "missing", id: testMissingCategoryID, wantErrs: []error{categories.ErrCategoryNotFound}, wantStored: true, wantTxCalls: 1},
		{
			name:        "delete failure",
			id:          testCategoryID,
			writeErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantStored:  true,
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newCategoryStore(t, workspaceStore)
			store.writeErr = tt.writeErr
			transactor := &appmock.Transactor{}
			service, err := NewCategoryService(transactor, store.repository(), workspaceStore.repository())

			if err != nil {
				t.Fatal(err)
			}

			err = service.Delete(memberContext(t, testWorkspaceID, testEditorID, workspaces.RoleEditor), tt.id)

			if transactor.Calls != tt.wantTxCalls || store.writes != tt.wantWrites {
				t.Fatalf("Delete() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, store.writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if _, stored := store.categories[testCategoryID]; stored != tt.wantStored {
				t.Fatalf("category stored = %t, want %t", stored, tt.wantStored)
			}
		})
	}
}

//
// Helpers
//

func validCreateCategoryInput() input.CreateCategoryInput {
	return input.CreateCategoryInput{WorkspaceID: testWorkspaceID, Name: "Groceries", Icon: "shopping-cart", Color: "green"}
}

func validUpdateCategoryInput() input.UpdateCategoryInput {
	return input.UpdateCategoryInput{ID: testCategoryID, Name: "Transport", Icon: "bus", Color: "blue"}
}

func validCategoryQuery() categories.CategoryQuery {
	return categories.NewCategoryQuery(categories.CategoryFilter{}, domquery.SortOrderAsc, categories.CategorySortName, 10, 1)
}

// Every operation on the test Category or its Workspace, with the error that
// reports either as missing to a caller who is not a member.

type categoryOperation struct {
	name     string
	write    bool
	notFound error
	call     func(ctx context.Context, service *CategoryService) error
}

func categoryOperations() []categoryOperation {
	return []categoryOperation{
		{"view", false, categories.ErrCategoryNotFound, func(ctx context.Context, service *CategoryService) error {
			_, err := service.FindByID(ctx, testCategoryID)
			return err
		}},
		{"list", false, workspaces.ErrWorkspaceNotFound, func(ctx context.Context, service *CategoryService) error {
			_, err := service.List(ctx, testWorkspaceID, validCategoryQuery())
			return err
		}},
		{"create", true, workspaces.ErrWorkspaceNotFound, func(ctx context.Context, service *CategoryService) error {
			_, err := service.Create(ctx, validCreateCategoryInput())
			return err
		}},
		{"update", true, categories.ErrCategoryNotFound, func(ctx context.Context, service *CategoryService) error {
			_, err := service.Update(ctx, validUpdateCategoryInput())
			return err
		}},
		{"delete", true, categories.ErrCategoryNotFound, func(ctx context.Context, service *CategoryService) error {
			return service.Delete(ctx, testCategoryID)
		}},
	}
}

// The store holds one Category in the test Workspace. Writes must run in a
// transaction after the Workspace was locked.

type categoryStore struct {
	t          *testing.T
	workspace  *workspaceStore
	categories map[categories.CategoryID]*categories.Category
	readErr    error
	writeErr   error
	writes     int
}

func newCategoryStore(t *testing.T, workspace *workspaceStore) *categoryStore {
	return &categoryStore{
		t:         t,
		workspace: workspace,
		categories: map[categories.CategoryID]*categories.Category{
			testCategoryID: categories.NewCategory(testCategoryID, testWorkspaceID, "Groceries", new(categories.IconShoppingCart), categories.ColorGreen),
		},
	}
}

func newCategoryStoreService(t *testing.T, store *categoryStore, workspaceStore *workspaceStore) *CategoryService {
	t.Helper()

	service, err := NewCategoryService(&appmock.Transactor{}, store.repository(), workspaceStore.repository())

	if err != nil {
		t.Fatal(err)
	}

	return service
}

func (s *categoryStore) write(ctx context.Context) error {
	s.writes++

	if !appmock.InTransaction(ctx) {
		s.t.Error("write ran outside a transaction")
	}

	if !s.workspace.locked {
		s.t.Error("write ran before the workspace was locked")
	}

	return s.writeErr
}

func (s *categoryStore) repository() *categoriesmock.CategoryRepository {
	return &categoriesmock.CategoryRepository{
		CreateFunc: func(ctx context.Context, category *categories.Category) categories.ErrCategoryCreateFailed {
			var err categories.ErrCategoryCreateFailed

			if writeErr := s.write(ctx); writeErr != nil {
				err = categoriesmock.NewErrCategoryCreateFailed(category, writeErr)
			} else {
				s.categories[category.ID] = category
			}

			return err
		},
		UpdateFunc: func(ctx context.Context, category *categories.Category) categories.ErrCategoryUpdateFailed {
			var err categories.ErrCategoryUpdateFailed

			writeErr := s.write(ctx)

			if writeErr == nil && s.categories[category.ID] == nil {
				writeErr = categories.ErrCategoryNotFound
			}

			if writeErr != nil {
				err = categoriesmock.NewErrCategoryUpdateFailed(category, writeErr)
			} else {
				s.categories[category.ID] = category
			}

			return err
		},
		DeleteFunc: func(ctx context.Context, id categories.CategoryID) categories.ErrCategoryDeleteFailed {
			var err categories.ErrCategoryDeleteFailed

			writeErr := s.write(ctx)

			if writeErr == nil && s.categories[id] == nil {
				writeErr = categories.ErrCategoryNotFound
			}

			if writeErr != nil {
				err = categoriesmock.NewErrCategoryDeleteFailed(id, writeErr)
			} else {
				delete(s.categories, id)
			}

			return err
		},
		FindByIDFunc: func(_ context.Context, id categories.CategoryID) (*categories.Category, error) {
			var (
				found *categories.Category
				err   = categories.ErrCategoryNotFound
			)

			if s.readErr != nil {
				err = s.readErr
			} else if category, ok := s.categories[id]; ok {
				found, err = category, nil
			}

			return found, err
		},
		FindFunc: func(_ context.Context, query categories.CategoryQuery) ([]*categories.Category, error) {
			var result []*categories.Category

			for _, category := range s.categories {
				if query.Filter.WorkspaceID != nil && category.WorkspaceID == *query.Filter.WorkspaceID {
					result = append(result, category)
				}
			}

			return result, s.readErr
		},
		CountFunc: func(_ context.Context, filter categories.CategoryFilter) (int, error) {
			total := 0

			for _, category := range s.categories {
				if filter.WorkspaceID != nil && category.WorkspaceID == *filter.WorkspaceID {
					total++
				}
			}

			return total, s.readErr
		},
	}
}
