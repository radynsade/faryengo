package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/budgets"
	budgetsmock "github.com/radynsade/faryengo/internal/budgets/mock"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	workspacesmock "github.com/radynsade/faryengo/internal/workspaces/mock"
	"github.com/radynsade/faryengo/pkg/domquery"
)

var (
	testBudgetID         = budgets.ID(uuid.MustParse("01920000-0000-7000-8000-000000000007"))
	testMissingBudgetID  = budgets.ID(uuid.MustParse("01920000-0000-7000-8000-0000000000fc"))
	testOtherCategoryID  = categories.ID(uuid.MustParse("01920000-0000-7000-8000-000000000006"))
	testStoredCategories = budgets.CategoryIDs{testCategoryID}
)

func TestNewBudgetService(t *testing.T) {
	for _, tt := range []struct {
		name       string
		transactor Transactor
		repository budgets.BudgetRepository
		workspaces workspaces.WorkspaceRepository
		wantErr    error
	}{
		{name: "created", transactor: &appmock.Transactor{}, repository: &budgetsmock.BudgetRepository{}, workspaces: &workspacesmock.WorkspaceRepository{}},
		{name: "nil transactor", repository: &budgetsmock.BudgetRepository{}, workspaces: &workspacesmock.WorkspaceRepository{}, wantErr: ErrTransactorNil},
		{name: "nil repository", transactor: &appmock.Transactor{}, workspaces: &workspacesmock.WorkspaceRepository{}, wantErr: ErrBudgetRepositoryNil},
		{name: "nil workspace repository", transactor: &appmock.Transactor{}, repository: &budgetsmock.BudgetRepository{}, wantErr: ErrWorkspaceRepositoryNil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewBudgetService(tt.transactor, tt.repository, tt.workspaces)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) || (service == nil) != (tt.wantErr != nil) {
				t.Fatalf("NewBudgetService() = (%v, %v), want error %v", service, err, tt.wantErr)
			}
		})
	}
}

func TestBudgetServiceNilReceiver(t *testing.T) {
	ctx := memberContext(t, testWorkspaceID, testOwnerID, workspaces.RoleOwner)

	var service *BudgetService

	created, createErr := service.Create(ctx, validCreateBudgetInput())
	found, findErr := service.FindByID(ctx, testBudgetID)
	list, listErr := service.List(ctx, testWorkspaceID, validBudgetQuery())
	updated, updateErr := service.Update(ctx, validUpdateBudgetInput())
	deleteErr := service.Delete(ctx, testBudgetID)

	for _, err := range []error{createErr, findErr, listErr, updateErr, deleteErr} {
		if !errors.Is(err, ErrBudgetRepositoryNil) {
			t.Fatalf("nil receiver error = %v, want ErrBudgetRepositoryNil", err)
		}
	}

	if created != nil || found != nil || list.Budgets != nil || updated != nil {
		t.Fatalf("nil receiver returned values: %v, %v, %v, %v", created, found, list, updated)
	}
}

func TestBudgetServiceRolePermissions(t *testing.T) {
	operations := budgetOperations()

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
				store := newBudgetStore(t, workspaceStore)
				service := newBudgetStoreService(t, store, workspaceStore)

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
	// this one or its budgets.
	for _, operation := range operations {
		t.Run("identity for another workspace "+operation.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newBudgetStore(t, workspaceStore)
			service := newBudgetStoreService(t, store, workspaceStore)
			otherWorkspaceID := workspaces.ID(uuid.MustParse("01920000-0000-7000-8000-0000000000ff"))

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
			store := newBudgetStore(t, workspaceStore)
			store.readErr = errors.New("read without an identity")
			service := newBudgetStoreService(t, store, workspaceStore)

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

func TestBudgetServiceStaleIdentity(t *testing.T) {
	for _, tt := range []struct {
		name    string
		change  func(store *workspaceStore)
		wantErr error
	}{
		{"demoted", func(store *workspaceStore) { store.memberships[store.index(testEditorID)].Role = workspaces.RoleViewer }, workspaces.ErrRolePermissionDenied},
		{"removed", func(store *workspaceStore) { store.memberships = slices.Delete(store.memberships, 1, 2) }, nil},
	} {
		for _, operation := range budgetOperations() {
			if operation.write {
				t.Run(tt.name+" "+operation.name, func(t *testing.T) {
					wantErr := tt.wantErr

					if wantErr == nil {
						wantErr = operation.notFound
					}

					workspaceStore := newWorkspaceStore(t, testMemberships()...)
					tt.change(workspaceStore)
					store := newBudgetStore(t, workspaceStore)
					service := newBudgetStoreService(t, store, workspaceStore)

					err := operation.call(memberContext(t, testWorkspaceID, testEditorID, workspaces.RoleEditor), service)

					if !errors.Is(err, wantErr) || !workspaceStore.locked || store.writes != 0 {
						t.Fatalf("%s = %v after %d writes, locked %t, want %v and no write", operation.name, err, store.writes, workspaceStore.locked, wantErr)
					}
				})
			}
		}
	}
}

func TestBudgetServiceCreate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		request     input.CreateBudgetInput
		lockErr     error
		writeErr    error
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{
			name:        "created",
			request:     validCreateBudgetInput(),
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "created with a zero amount",
			request:     input.CreateBudgetInput{WorkspaceID: testWorkspaceID, CategoryIDs: []categories.ID{testCategoryID}, CurrencyCode: "EUR"},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:    "all fields invalid",
			request: input.CreateBudgetInput{CurrencyCode: "eur", Amount: -1},
			wantErrs: []error{
				input.ErrCreateBudgetInputInvalid,
				workspaces.ErrIDInvalid,
				budgets.ErrCategoryIDsEmpty,
				currencies.ErrCodeInvalid,
				budgets.ErrAmountNegative,
			},
		},
		{
			name: "invalid category",
			request: input.CreateBudgetInput{
				WorkspaceID:  testWorkspaceID,
				CategoryIDs:  []categories.ID{testCategoryID, {}},
				CurrencyCode: "EUR",
				Amount:       100_00,
			},
			wantErrs: []error{input.ErrCreateBudgetInputInvalid, categories.ErrIDInvalid},
		},
		{
			name: "duplicate category",
			request: input.CreateBudgetInput{
				WorkspaceID:  testWorkspaceID,
				CategoryIDs:  []categories.ID{testCategoryID, testCategoryID},
				CurrencyCode: "EUR",
				Amount:       100_00,
			},
			wantErrs: []error{input.ErrCreateBudgetInputInvalid, budgets.ErrCategoryIDsDuplicate},
		},
		{
			name:        "category missing or in another workspace",
			request:     validCreateBudgetInput(),
			writeErr:    categories.ErrCategoryNotFound,
			wantErrs:    []error{categories.ErrCategoryNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "missing currency",
			request:     validCreateBudgetInput(),
			writeErr:    currencies.ErrCurrencyNotFound,
			wantErrs:    []error{currencies.ErrCurrencyNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name: "missing workspace",
			request: input.CreateBudgetInput{
				WorkspaceID:  workspaces.ID(uuid.MustParse("01920000-0000-7000-8000-0000000000ff")),
				CategoryIDs:  []categories.ID{testCategoryID},
				CurrencyCode: "EUR",
				Amount:       100_00,
			},
			wantErrs:    []error{workspaces.ErrWorkspaceNotFound},
			wantTxCalls: 1,
		},
		{
			name:        "lock failure",
			request:     validCreateBudgetInput(),
			lockErr:     context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
		},
		{
			name:        "create failure",
			request:     validCreateBudgetInput(),
			writeErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			workspaceStore.lockErr = tt.lockErr
			store := newBudgetStore(t, workspaceStore)
			store.writeErr = tt.writeErr
			delete(store.budgets, testBudgetID)
			transactor := &appmock.Transactor{}
			service, err := NewBudgetService(transactor, store.repository(), workspaceStore.repository())

			if err != nil {
				t.Fatal(err)
			}

			budget, err := service.Create(memberContext(t, tt.request.WorkspaceID, testEditorID, workspaces.RoleEditor), tt.request)

			if transactor.Calls != tt.wantTxCalls || store.writes != tt.wantWrites {
				t.Fatalf("Create() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, store.writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if budget != nil || len(store.budgets) != 0 {
					t.Fatalf("Create() = %v with %d stored, want nil and nothing stored", budget, len(store.budgets))
				}
			} else {
				stored := store.budgets[budget.ID]

				if stored != budget || budget.WorkspaceID != testWorkspaceID ||
					!slices.Equal(budget.Categories, tt.request.CategoryIDs) ||
					budget.Currency != currencies.Code(tt.request.CurrencyCode) ||
					budget.Amount != budgets.Amount(tt.request.Amount) {
					t.Fatalf("Create() = %+v, stored %+v, want the request's values", budget, stored)
				}

				if uuid.UUID(budget.ID).Version() != 7 {
					t.Fatalf("Create() ID version = %d, want 7", uuid.UUID(budget.ID).Version())
				}
			}
		})
	}
}

// The Budget keeps its own copy of the requested Categories.

func TestBudgetServiceCreateCopiesCategories(t *testing.T) {
	workspaceStore := newWorkspaceStore(t, testMemberships()...)
	store := newBudgetStore(t, workspaceStore)
	service := newBudgetStoreService(t, store, workspaceStore)
	request := validCreateBudgetInput()

	budget, err := service.Create(memberContext(t, testWorkspaceID, testEditorID, workspaces.RoleEditor), request)

	if err != nil {
		t.Fatal(err)
	}

	request.CategoryIDs[0] = testOtherCategoryID

	if budget.Categories[0] != testCategoryID {
		t.Fatalf("Create() categories = %v, want them unaffected by the request", budget.Categories)
	}
}

func TestBudgetServiceFindByID(t *testing.T) {
	viewer := memberContext(t, testWorkspaceID, testViewerID, workspaces.RoleViewer)

	for _, tt := range []struct {
		name     string
		ctx      context.Context
		id       budgets.ID
		readErr  error
		wantErrs []error
	}{
		{name: "found", ctx: viewer, id: testBudgetID},
		{name: "invalid ID", ctx: viewer, id: budgets.ID{}, wantErrs: []error{budgets.ErrIDInvalid}},
		{name: "missing", ctx: viewer, id: testMissingBudgetID, wantErrs: []error{budgets.ErrBudgetNotFound}},
		{name: "read failure", ctx: viewer, id: testBudgetID, readErr: context.DeadlineExceeded, wantErrs: []error{context.DeadlineExceeded}},
		{
			name:     "missing identity is checked before the read",
			ctx:      userContext(t, testViewerID),
			id:       testBudgetID,
			readErr:  context.DeadlineExceeded,
			wantErrs: []error{ErrUnauthenticated, workspaces.ErrIdentityMissing},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newBudgetStore(t, workspaceStore)
			store.readErr = tt.readErr
			service := newBudgetStoreService(t, store, workspaceStore)

			budget, err := service.FindByID(tt.ctx, tt.id)

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && budget != nil {
				t.Fatalf("FindByID() = %v, want nil on error", budget)
			} else if len(tt.wantErrs) == 0 && budget != store.budgets[testBudgetID] {
				t.Fatalf("FindByID() = %+v, want the stored budget", budget)
			}
		})
	}
}

func TestBudgetServiceList(t *testing.T) {
	otherWorkspaceID := workspaces.ID(uuid.MustParse("01920000-0000-7000-8000-0000000000ff"))
	withOtherWorkspace := validBudgetQuery()
	withOtherWorkspace.Filter.WorkspaceID = &otherWorkspaceID
	withoutSort := validBudgetQuery()
	withoutSort.SortBy = ""
	viewer := memberContext(t, testWorkspaceID, testViewerID, workspaces.RoleViewer)

	for _, tt := range []struct {
		name     string
		ctx      context.Context
		query    budgets.BudgetQuery
		countErr error
		wantErrs []error
	}{
		{name: "listed", ctx: viewer, query: validBudgetQuery()},
		{name: "workspace criterion replaced", ctx: viewer, query: withOtherWorkspace},
		{name: "invalid query", ctx: viewer, query: withoutSort, wantErrs: []error{budgets.ErrBudgetQueryInvalid}},
		{name: "count failure", ctx: viewer, query: validBudgetQuery(), countErr: context.DeadlineExceeded, wantErrs: []error{context.DeadlineExceeded}},
		{
			name:     "missing identity",
			ctx:      userContext(t, testViewerID),
			query:    validBudgetQuery(),
			countErr: context.DeadlineExceeded,
			wantErrs: []error{ErrUnauthenticated, workspaces.ErrIdentityMissing},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var gotFilter budgets.BudgetFilter

			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newBudgetStore(t, workspaceStore)
			repository := store.repository()
			repository.CountFunc = func(_ context.Context, filter budgets.BudgetFilter) (int, error) {
				gotFilter = filter
				return 1, tt.countErr
			}

			service, err := NewBudgetService(&appmock.Transactor{}, repository, workspaceStore.repository())

			if err != nil {
				t.Fatal(err)
			}

			list, err := service.List(tt.ctx, testWorkspaceID, tt.query)

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if list.Budgets != nil || list.Total != 0 {
					t.Fatalf("List() = %+v, want an empty list on error", list)
				}
			} else {
				if gotFilter.WorkspaceID == nil || *gotFilter.WorkspaceID != testWorkspaceID {
					t.Fatalf("Count() filter workspace = %v, want %v", gotFilter.WorkspaceID, testWorkspaceID)
				}

				if list.Total != 1 || len(list.Budgets) != 1 || list.Budgets[0] != store.budgets[testBudgetID] {
					t.Fatalf("List() = %+v, want the stored budget and a total of 1", list)
				}
			}
		})
	}
}

func TestBudgetServiceUpdate(t *testing.T) {
	for _, tt := range []struct {
		name           string
		request        input.UpdateBudgetInput
		lockErr        error
		writeErr       error
		wantCategories budgets.CategoryIDs
		wantCurrency   currencies.Code
		wantAmount     budgets.Amount
		wantErrs       []error
		wantTxCalls    int
		wantWrites     int
	}{
		{
			name:           "updated",
			request:        validUpdateBudgetInput(),
			wantCategories: budgets.CategoryIDs{testOtherCategoryID, testCategoryID},
			wantCurrency:   "USD",
			wantAmount:     250_00,
			wantTxCalls:    1,
			wantWrites:     1,
		},
		{
			name: "all fields invalid",
			request: input.UpdateBudgetInput{
				CategoryIDs:  []categories.ID{testCategoryID, testCategoryID},
				CurrencyCode: "",
				Amount:       -1,
			},
			wantErrs: []error{
				input.ErrUpdateBudgetInputInvalid,
				budgets.ErrIDInvalid,
				budgets.ErrCategoryIDsDuplicate,
				currencies.ErrCodeInvalid,
				budgets.ErrAmountNegative,
			},
			wantCategories: testStoredCategories,
			wantCurrency:   "EUR",
			wantAmount:     100_00,
		},
		{
			name:           "missing",
			request:        input.UpdateBudgetInput{ID: testMissingBudgetID, CategoryIDs: []categories.ID{testCategoryID}, CurrencyCode: "EUR", Amount: 1},
			wantErrs:       []error{budgets.ErrBudgetNotFound},
			wantCategories: testStoredCategories,
			wantCurrency:   "EUR",
			wantAmount:     100_00,
			wantTxCalls:    1,
		},
		{
			name:           "category missing or in another workspace",
			request:        validUpdateBudgetInput(),
			writeErr:       categories.ErrCategoryNotFound,
			wantErrs:       []error{categories.ErrCategoryNotFound},
			wantCategories: testStoredCategories,
			wantCurrency:   "EUR",
			wantAmount:     100_00,
			wantTxCalls:    1,
			wantWrites:     1,
		},
		{
			name:           "lock failure",
			request:        validUpdateBudgetInput(),
			lockErr:        context.DeadlineExceeded,
			wantErrs:       []error{context.DeadlineExceeded},
			wantCategories: testStoredCategories,
			wantCurrency:   "EUR",
			wantAmount:     100_00,
			wantTxCalls:    1,
		},
		{
			name:           "update failure",
			request:        validUpdateBudgetInput(),
			writeErr:       context.DeadlineExceeded,
			wantErrs:       []error{context.DeadlineExceeded},
			wantCategories: testStoredCategories,
			wantCurrency:   "EUR",
			wantAmount:     100_00,
			wantTxCalls:    1,
			wantWrites:     1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			workspaceStore.lockErr = tt.lockErr
			store := newBudgetStore(t, workspaceStore)
			store.writeErr = tt.writeErr
			transactor := &appmock.Transactor{}
			service, err := NewBudgetService(transactor, store.repository(), workspaceStore.repository())

			if err != nil {
				t.Fatal(err)
			}

			budget, err := service.Update(memberContext(t, testWorkspaceID, testEditorID, workspaces.RoleEditor), tt.request)

			if transactor.Calls != tt.wantTxCalls || store.writes != tt.wantWrites {
				t.Fatalf("Update() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, store.writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			stored := store.budgets[testBudgetID]

			// The budget keeps its workspace.
			if !slices.Equal(stored.Categories, tt.wantCategories) || stored.Currency != tt.wantCurrency ||
				stored.Amount != tt.wantAmount || stored.WorkspaceID != testWorkspaceID {
				t.Fatalf(
					"stored budget = %+v, want categories %v, currency %s, and amount %d in workspace %v",
					stored, tt.wantCategories, tt.wantCurrency, tt.wantAmount, testWorkspaceID,
				)
			}

			if len(tt.wantErrs) > 0 && budget != nil {
				t.Fatalf("Update() = %v, want nil on error", budget)
			} else if len(tt.wantErrs) == 0 && budget != stored {
				t.Fatalf("Update() = %+v, want the stored budget", budget)
			}
		})
	}
}

func TestBudgetServiceDelete(t *testing.T) {
	for _, tt := range []struct {
		name        string
		id          budgets.ID
		writeErr    error
		wantErrs    []error
		wantStored  bool
		wantTxCalls int
		wantWrites  int
	}{
		{name: "deleted", id: testBudgetID, wantTxCalls: 1, wantWrites: 1},
		{name: "invalid ID", id: budgets.ID{}, wantErrs: []error{budgets.ErrIDInvalid}, wantStored: true},
		{name: "missing", id: testMissingBudgetID, wantErrs: []error{budgets.ErrBudgetNotFound}, wantStored: true, wantTxCalls: 1},
		{
			name:        "delete failure",
			id:          testBudgetID,
			writeErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantStored:  true,
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newBudgetStore(t, workspaceStore)
			store.writeErr = tt.writeErr
			transactor := &appmock.Transactor{}
			service, err := NewBudgetService(transactor, store.repository(), workspaceStore.repository())

			if err != nil {
				t.Fatal(err)
			}

			err = service.Delete(memberContext(t, testWorkspaceID, testEditorID, workspaces.RoleEditor), tt.id)

			if transactor.Calls != tt.wantTxCalls || store.writes != tt.wantWrites {
				t.Fatalf("Delete() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, store.writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if _, stored := store.budgets[testBudgetID]; stored != tt.wantStored {
				t.Fatalf("budget stored = %t, want %t", stored, tt.wantStored)
			}
		})
	}
}

//
// Helpers
//

func validCreateBudgetInput() input.CreateBudgetInput {
	return input.CreateBudgetInput{
		WorkspaceID:  testWorkspaceID,
		CategoryIDs:  []categories.ID{testCategoryID, testOtherCategoryID},
		CurrencyCode: "EUR",
		Amount:       100_00,
	}
}

func validUpdateBudgetInput() input.UpdateBudgetInput {
	return input.UpdateBudgetInput{
		ID:           testBudgetID,
		CategoryIDs:  []categories.ID{testOtherCategoryID, testCategoryID},
		CurrencyCode: "USD",
		Amount:       250_00,
	}
}

func validBudgetQuery() budgets.BudgetQuery {
	return budgets.NewBudgetQuery(budgets.BudgetFilter{}, domquery.SortOrderAsc, budgets.BudgetSortAmount, 10, 1)
}

// Every operation on the test Budget or its Workspace, with the error that
// reports either as missing to a caller who is not a member.

type budgetOperation struct {
	name     string
	write    bool
	notFound error
	call     func(ctx context.Context, service *BudgetService) error
}

func budgetOperations() []budgetOperation {
	return []budgetOperation{
		{"view", false, budgets.ErrBudgetNotFound, func(ctx context.Context, service *BudgetService) error {
			_, err := service.FindByID(ctx, testBudgetID)
			return err
		}},
		{"list", false, workspaces.ErrWorkspaceNotFound, func(ctx context.Context, service *BudgetService) error {
			_, err := service.List(ctx, testWorkspaceID, validBudgetQuery())
			return err
		}},
		{"create", true, workspaces.ErrWorkspaceNotFound, func(ctx context.Context, service *BudgetService) error {
			_, err := service.Create(ctx, validCreateBudgetInput())
			return err
		}},
		{"update", true, budgets.ErrBudgetNotFound, func(ctx context.Context, service *BudgetService) error {
			_, err := service.Update(ctx, validUpdateBudgetInput())
			return err
		}},
		{"delete", true, budgets.ErrBudgetNotFound, func(ctx context.Context, service *BudgetService) error {
			return service.Delete(ctx, testBudgetID)
		}},
	}
}

// The store holds one Budget in the test Workspace. Writes must run in a
// transaction after the Workspace was locked.

type budgetStore struct {
	t         *testing.T
	workspace *workspaceStore
	budgets   map[budgets.ID]*budgets.Budget
	readErr   error
	writeErr  error
	writes    int
}

func newBudgetStore(t *testing.T, workspace *workspaceStore) *budgetStore {
	return &budgetStore{
		t:         t,
		workspace: workspace,
		budgets: map[budgets.ID]*budgets.Budget{
			testBudgetID: budgets.NewBudget(testBudgetID, testWorkspaceID, slices.Clone(testStoredCategories), "EUR", 100_00),
		},
	}
}

func newBudgetStoreService(t *testing.T, store *budgetStore, workspaceStore *workspaceStore) *BudgetService {
	t.Helper()

	service, err := NewBudgetService(&appmock.Transactor{}, store.repository(), workspaceStore.repository())

	if err != nil {
		t.Fatal(err)
	}

	return service
}

func (s *budgetStore) write(ctx context.Context) error {
	s.writes++

	if !appmock.InTransaction(ctx) {
		s.t.Error("write ran outside a transaction")
	}

	if !s.workspace.locked {
		s.t.Error("write ran before the workspace was locked")
	}

	return s.writeErr
}

func (s *budgetStore) repository() *budgetsmock.BudgetRepository {
	return &budgetsmock.BudgetRepository{
		CreateFunc: func(ctx context.Context, budget *budgets.Budget) budgets.ErrBudgetCreateFailed {
			var err budgets.ErrBudgetCreateFailed

			if writeErr := s.write(ctx); writeErr != nil {
				err = budgetsmock.NewErrBudgetCreateFailed(budget, writeErr)
			} else {
				s.budgets[budget.ID] = budget
			}

			return err
		},
		UpdateFunc: func(ctx context.Context, budget *budgets.Budget) budgets.ErrBudgetUpdateFailed {
			var err budgets.ErrBudgetUpdateFailed

			writeErr := s.write(ctx)

			if writeErr == nil && s.budgets[budget.ID] == nil {
				writeErr = budgets.ErrBudgetNotFound
			}

			if writeErr != nil {
				err = budgetsmock.NewErrBudgetUpdateFailed(budget, writeErr)
			} else {
				s.budgets[budget.ID] = budget
			}

			return err
		},
		DeleteFunc: func(ctx context.Context, id budgets.ID) budgets.ErrBudgetDeleteFailed {
			var err budgets.ErrBudgetDeleteFailed

			writeErr := s.write(ctx)

			if writeErr == nil && s.budgets[id] == nil {
				writeErr = budgets.ErrBudgetNotFound
			}

			if writeErr != nil {
				err = budgetsmock.NewErrBudgetDeleteFailed(id, writeErr)
			} else {
				delete(s.budgets, id)
			}

			return err
		},
		FindByIDFunc: func(_ context.Context, id budgets.ID) (*budgets.Budget, error) {
			var (
				found *budgets.Budget
				err   = budgets.ErrBudgetNotFound
			)

			if s.readErr != nil {
				err = s.readErr
			} else if budget, ok := s.budgets[id]; ok {
				found, err = budget, nil
			}

			return found, err
		},
		FindFunc: func(_ context.Context, query budgets.BudgetQuery) ([]*budgets.Budget, error) {
			var result []*budgets.Budget

			for _, budget := range s.budgets {
				if query.Filter.WorkspaceID != nil && budget.WorkspaceID == *query.Filter.WorkspaceID {
					result = append(result, budget)
				}
			}

			return result, s.readErr
		},
		CountFunc: func(_ context.Context, filter budgets.BudgetFilter) (int, error) {
			total := 0

			for _, budget := range s.budgets {
				if filter.WorkspaceID != nil && budget.WorkspaceID == *filter.WorkspaceID {
					total++
				}
			}

			return total, s.readErr
		},
	}
}
