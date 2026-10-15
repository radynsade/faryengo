package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/accounts"
	accountsmock "github.com/radynsade/faryengo/internal/accounts/mock"
	"github.com/radynsade/faryengo/internal/app/input"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	workspacesmock "github.com/radynsade/faryengo/internal/workspaces/mock"
	"github.com/radynsade/faryengo/pkg/domquery"
)

var (
	testAccountID        = accounts.ID(uuid.MustParse("01920000-0000-7000-8000-000000000005"))
	testMissingAccountID = accounts.ID(uuid.MustParse("01920000-0000-7000-8000-0000000000fd"))
)

func TestNewAccountService(t *testing.T) {
	for _, tt := range []struct {
		name       string
		transactor Transactor
		repository accounts.AccountRepository
		workspaces workspaces.WorkspaceRepository
		wantErr    error
	}{
		{name: "created", transactor: &appmock.Transactor{}, repository: &accountsmock.AccountRepository{}, workspaces: &workspacesmock.WorkspaceRepository{}},
		{name: "nil transactor", repository: &accountsmock.AccountRepository{}, workspaces: &workspacesmock.WorkspaceRepository{}, wantErr: ErrTransactorNil},
		{name: "nil repository", transactor: &appmock.Transactor{}, workspaces: &workspacesmock.WorkspaceRepository{}, wantErr: ErrAccountRepositoryNil},
		{name: "nil workspace repository", transactor: &appmock.Transactor{}, repository: &accountsmock.AccountRepository{}, wantErr: ErrWorkspaceRepositoryNil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewAccountService(tt.transactor, tt.repository, tt.workspaces)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) || (service == nil) != (tt.wantErr != nil) {
				t.Fatalf("NewAccountService() = (%v, %v), want error %v", service, err, tt.wantErr)
			}
		})
	}
}

func TestAccountServiceNilReceiver(t *testing.T) {
	ctx := memberContext(t, testWorkspaceID, testOwnerID, workspaces.RoleOwner)

	var service *AccountService

	created, createErr := service.Create(ctx, validCreateAccountInput())
	found, findErr := service.FindByID(ctx, testAccountID)
	list, listErr := service.List(ctx, testWorkspaceID, validAccountQuery())
	updated, updateErr := service.Update(ctx, validUpdateAccountInput())
	deleteErr := service.Delete(ctx, testAccountID)

	for _, err := range []error{createErr, findErr, listErr, updateErr, deleteErr} {
		if !errors.Is(err, ErrAccountRepositoryNil) {
			t.Fatalf("nil receiver error = %v, want ErrAccountRepositoryNil", err)
		}
	}

	if created != nil || found != nil || list.Accounts != nil || updated != nil {
		t.Fatalf("nil receiver returned values: %v, %v, %v, %v", created, found, list, updated)
	}
}

func TestAccountServiceRolePermissions(t *testing.T) {
	operations := accountOperations()

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
				store := newAccountStore(t, workspaceStore)
				service := newAccountStoreService(t, store, workspaceStore)

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
	// this one or its accounts.
	for _, operation := range operations {
		t.Run("identity for another workspace "+operation.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newAccountStore(t, workspaceStore)
			service := newAccountStoreService(t, store, workspaceStore)
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
			store := newAccountStore(t, workspaceStore)
			store.readErr = errors.New("read without an identity")
			service := newAccountStoreService(t, store, workspaceStore)

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

func TestAccountServiceStaleIdentity(t *testing.T) {
	for _, tt := range []struct {
		name    string
		change  func(store *workspaceStore)
		wantErr error
	}{
		{"demoted", func(store *workspaceStore) { store.memberships[store.index(testEditorID)].Role = workspaces.RoleViewer }, workspaces.ErrRolePermissionDenied},
		{"removed", func(store *workspaceStore) { store.memberships = slices.Delete(store.memberships, 1, 2) }, nil},
	} {
		for _, operation := range accountOperations() {
			if operation.write {
				t.Run(tt.name+" "+operation.name, func(t *testing.T) {
					wantErr := tt.wantErr

					if wantErr == nil {
						wantErr = operation.notFound
					}

					workspaceStore := newWorkspaceStore(t, testMemberships()...)
					tt.change(workspaceStore)
					store := newAccountStore(t, workspaceStore)
					service := newAccountStoreService(t, store, workspaceStore)

					err := operation.call(memberContext(t, testWorkspaceID, testEditorID, workspaces.RoleEditor), service)

					if !errors.Is(err, wantErr) || !workspaceStore.locked || store.writes != 0 {
						t.Fatalf("%s = %v after %d writes, locked %t, want %v and no write", operation.name, err, store.writes, workspaceStore.locked, wantErr)
					}
				})
			}
		}
	}
}

func TestAccountServiceCreate(t *testing.T) {
	invalidUserID := users.UserID{}

	for _, tt := range []struct {
		name        string
		request     input.CreateAccountInput
		lockErr     error
		readErr     error
		writeErr    error
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{
			name:        "created with a user",
			request:     input.CreateAccountInput{WorkspaceID: testWorkspaceID, UserID: new(testViewerID), CurrencyCode: "EUR", Name: " Cash "},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "created without a user",
			request:     input.CreateAccountInput{WorkspaceID: testWorkspaceID, CurrencyCode: "EUR", Name: "Cash"},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:    "all fields invalid",
			request: input.CreateAccountInput{UserID: &invalidUserID, Name: "  "},
			wantErrs: []error{
				input.ErrCreateAccountInputInvalid,
				workspaces.ErrIDInvalid,
				users.ErrUserIDInvalid,
				currencies.ErrCodeInvalid,
				accounts.ErrNameEmpty,
			},
		},
		{
			name:     "invalid currency",
			request:  input.CreateAccountInput{WorkspaceID: testWorkspaceID, CurrencyCode: "eur", Name: "Cash"},
			wantErrs: []error{input.ErrCreateAccountInputInvalid, currencies.ErrCodeInvalid},
		},
		{
			name:        "missing currency",
			request:     validCreateAccountInput(),
			writeErr:    currencies.ErrCurrencyNotFound,
			wantErrs:    []error{currencies.ErrCurrencyNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:     "long name",
			request:  input.CreateAccountInput{WorkspaceID: testWorkspaceID, CurrencyCode: "EUR", Name: strings.Repeat("a", accounts.MaxNameLength+1)},
			wantErrs: []error{input.ErrCreateAccountInputInvalid, accounts.ErrNameTooLong},
		},
		{
			name:        "user is not a member",
			request:     input.CreateAccountInput{WorkspaceID: testWorkspaceID, UserID: new(testNewcomerID), CurrencyCode: "EUR", Name: "Cash"},
			wantErrs:    []error{users.ErrUserNotFound},
			wantTxCalls: 1,
		},
		{
			name:        "membership read failure",
			request:     input.CreateAccountInput{WorkspaceID: testWorkspaceID, UserID: new(testViewerID), CurrencyCode: "EUR", Name: "Cash"},
			readErr:     context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
		},
		{
			name:        "missing workspace",
			request:     input.CreateAccountInput{WorkspaceID: workspaces.ID(uuid.MustParse("01920000-0000-7000-8000-0000000000ff")), CurrencyCode: "EUR", Name: "Cash"},
			wantErrs:    []error{workspaces.ErrWorkspaceNotFound},
			wantTxCalls: 1,
		},
		{
			name:        "lock failure",
			request:     validCreateAccountInput(),
			lockErr:     context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
		},
		{
			name:        "create failure",
			request:     validCreateAccountInput(),
			writeErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			workspaceStore.lockErr = tt.lockErr
			store := newAccountStore(t, workspaceStore)
			store.writeErr = tt.writeErr
			delete(store.accounts, testAccountID)
			workspaceRepository := workspaceStore.repository()

			// Only the membership of the account's user fails to be read;
			// the caller's own role is read as usual.
			if tt.readErr != nil {
				findMember := workspaceRepository.FindMemberFunc
				workspaceRepository.FindMemberFunc = func(ctx context.Context, id workspaces.ID, userID users.UserID) (workspaces.Membership, error) {
					var (
						membership workspaces.Membership
						err        = tt.readErr
					)

					if userID == testEditorID {
						membership, err = findMember(ctx, id, userID)
					}

					return membership, err
				}
			}

			transactor := &appmock.Transactor{}
			service, err := NewAccountService(transactor, store.repository(), workspaceRepository)

			if err != nil {
				t.Fatal(err)
			}

			account, err := service.Create(memberContext(t, tt.request.WorkspaceID, testEditorID, workspaces.RoleEditor), tt.request)

			if transactor.Calls != tt.wantTxCalls || store.writes != tt.wantWrites {
				t.Fatalf("Create() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, store.writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if account != nil || len(store.accounts) != 0 {
					t.Fatalf("Create() = %v with %d stored, want nil and nothing stored", account, len(store.accounts))
				}
			} else {
				stored := store.accounts[account.ID]

				if stored != account || account.WorkspaceID != testWorkspaceID || account.Name != "Cash" ||
					account.CurrencyCode != "EUR" || !sameUserID(account.UserID, tt.request.UserID) {
					t.Fatalf("Create() = %+v, stored %+v", account, stored)
				}

				if uuid.UUID(account.ID).Version() != 7 {
					t.Fatalf("Create() ID version = %d, want 7", uuid.UUID(account.ID).Version())
				}
			}
		})
	}
}

func TestAccountServiceFindByID(t *testing.T) {
	viewer := memberContext(t, testWorkspaceID, testViewerID, workspaces.RoleViewer)

	for _, tt := range []struct {
		name     string
		ctx      context.Context
		id       accounts.ID
		readErr  error
		wantErrs []error
	}{
		{name: "found", ctx: viewer, id: testAccountID},
		{name: "invalid ID", ctx: viewer, id: accounts.ID{}, wantErrs: []error{accounts.ErrIDInvalid}},
		{name: "missing", ctx: viewer, id: testMissingAccountID, wantErrs: []error{accounts.ErrAccountNotFound}},
		{name: "read failure", ctx: viewer, id: testAccountID, readErr: context.DeadlineExceeded, wantErrs: []error{context.DeadlineExceeded}},
		{
			name:     "missing identity is checked before the read",
			ctx:      userContext(t, testViewerID),
			id:       testAccountID,
			readErr:  context.DeadlineExceeded,
			wantErrs: []error{ErrUnauthenticated, workspaces.ErrIdentityMissing},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newAccountStore(t, workspaceStore)
			store.readErr = tt.readErr
			service := newAccountStoreService(t, store, workspaceStore)

			account, err := service.FindByID(tt.ctx, tt.id)

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && account != nil {
				t.Fatalf("FindByID() = %v, want nil on error", account)
			} else if len(tt.wantErrs) == 0 && account != store.accounts[testAccountID] {
				t.Fatalf("FindByID() = %+v, want the stored account", account)
			}
		})
	}
}

func TestAccountServiceList(t *testing.T) {
	otherWorkspaceID := workspaces.ID(uuid.MustParse("01920000-0000-7000-8000-0000000000ff"))
	withOtherWorkspace := validAccountQuery()
	withOtherWorkspace.Filter.WorkspaceID = &otherWorkspaceID
	withoutSort := validAccountQuery()
	withoutSort.SortBy = ""
	viewer := memberContext(t, testWorkspaceID, testViewerID, workspaces.RoleViewer)

	for _, tt := range []struct {
		name     string
		ctx      context.Context
		query    accounts.AccountQuery
		countErr error
		wantErrs []error
	}{
		{name: "listed", ctx: viewer, query: validAccountQuery()},
		{name: "workspace criterion replaced", ctx: viewer, query: withOtherWorkspace},
		{name: "invalid query", ctx: viewer, query: withoutSort, wantErrs: []error{accounts.ErrAccountQueryInvalid}},
		{name: "count failure", ctx: viewer, query: validAccountQuery(), countErr: context.DeadlineExceeded, wantErrs: []error{context.DeadlineExceeded}},
		{
			name:     "missing identity",
			ctx:      userContext(t, testViewerID),
			query:    validAccountQuery(),
			countErr: context.DeadlineExceeded,
			wantErrs: []error{ErrUnauthenticated, workspaces.ErrIdentityMissing},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var gotFilter accounts.AccountFilter

			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newAccountStore(t, workspaceStore)
			repository := store.repository()
			repository.CountFunc = func(_ context.Context, filter accounts.AccountFilter) (int, error) {
				gotFilter = filter
				return 1, tt.countErr
			}

			service, err := NewAccountService(&appmock.Transactor{}, repository, workspaceStore.repository())

			if err != nil {
				t.Fatal(err)
			}

			list, err := service.List(tt.ctx, testWorkspaceID, tt.query)

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if list.Accounts != nil || list.Total != 0 {
					t.Fatalf("List() = %+v, want an empty list on error", list)
				}
			} else {
				if gotFilter.WorkspaceID == nil || *gotFilter.WorkspaceID != testWorkspaceID {
					t.Fatalf("Count() filter workspace = %v, want %v", gotFilter.WorkspaceID, testWorkspaceID)
				}

				if list.Total != 1 || len(list.Accounts) != 1 || list.Accounts[0] != store.accounts[testAccountID] {
					t.Fatalf("List() = %+v, want the stored account and a total of 1", list)
				}
			}
		})
	}
}

func TestAccountServiceUpdate(t *testing.T) {
	invalidUserID := users.UserID{}

	for _, tt := range []struct {
		name        string
		request     input.UpdateAccountInput
		storedUser  *users.UserID
		lockErr     error
		writeErr    error
		wantName    accounts.Name
		wantUser    *users.UserID
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{
			name:        "updated",
			request:     input.UpdateAccountInput{ID: testAccountID, UserID: new(testViewerID), Name: " Card "},
			wantName:    "Card",
			wantUser:    new(testViewerID),
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "user removed",
			request:     input.UpdateAccountInput{ID: testAccountID, Name: "Cash"},
			wantName:    "Cash",
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "former member kept",
			request:     input.UpdateAccountInput{ID: testAccountID, UserID: new(testNewcomerID), Name: "Card"},
			storedUser:  new(testNewcomerID),
			wantName:    "Card",
			wantUser:    new(testNewcomerID),
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "user is not a member",
			request:     input.UpdateAccountInput{ID: testAccountID, UserID: new(testNewcomerID), Name: "Card"},
			wantErrs:    []error{users.ErrUserNotFound},
			wantName:    "Cash",
			wantUser:    new(testEditorID),
			wantTxCalls: 1,
		},
		{
			name:    "all fields invalid",
			request: input.UpdateAccountInput{UserID: &invalidUserID, Name: ""},
			wantErrs: []error{
				input.ErrUpdateAccountInputInvalid,
				accounts.ErrIDInvalid,
				users.ErrUserIDInvalid,
				accounts.ErrNameEmpty,
			},
			wantName: "Cash",
			wantUser: new(testEditorID),
		},
		{
			name:        "missing",
			request:     input.UpdateAccountInput{ID: testMissingAccountID, Name: "Card"},
			wantErrs:    []error{accounts.ErrAccountNotFound},
			wantName:    "Cash",
			wantUser:    new(testEditorID),
			wantTxCalls: 1,
		},
		{
			name:        "lock failure",
			request:     validUpdateAccountInput(),
			lockErr:     context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantName:    "Cash",
			wantUser:    new(testEditorID),
			wantTxCalls: 1,
		},
		{
			name:        "update failure",
			request:     validUpdateAccountInput(),
			writeErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantName:    "Cash",
			wantUser:    new(testEditorID),
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			workspaceStore.lockErr = tt.lockErr
			store := newAccountStore(t, workspaceStore)
			store.writeErr = tt.writeErr

			if tt.storedUser != nil {
				store.accounts[testAccountID].UserID = tt.storedUser
			}

			transactor := &appmock.Transactor{}
			service, err := NewAccountService(transactor, store.repository(), workspaceStore.repository())

			if err != nil {
				t.Fatal(err)
			}

			account, err := service.Update(memberContext(t, testWorkspaceID, testEditorID, workspaces.RoleEditor), tt.request)

			if transactor.Calls != tt.wantTxCalls || store.writes != tt.wantWrites {
				t.Fatalf("Update() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, store.writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			stored := store.accounts[testAccountID]

			// The account keeps the currency it was created with.
			if stored.Name != tt.wantName || stored.CurrencyCode != "EUR" || stored.WorkspaceID != testWorkspaceID ||
				!sameUserID(stored.UserID, tt.wantUser) {
				t.Fatalf(
					"stored account = %+v, want name %q, currency EUR, and user %v in workspace %v",
					stored, tt.wantName, tt.wantUser, testWorkspaceID,
				)
			}

			if len(tt.wantErrs) > 0 && account != nil {
				t.Fatalf("Update() = %v, want nil on error", account)
			} else if len(tt.wantErrs) == 0 && account != stored {
				t.Fatalf("Update() = %+v, want the stored account", account)
			}
		})
	}
}

func TestAccountServiceDelete(t *testing.T) {
	for _, tt := range []struct {
		name        string
		id          accounts.ID
		writeErr    error
		wantErrs    []error
		wantStored  bool
		wantTxCalls int
		wantWrites  int
	}{
		{name: "deleted", id: testAccountID, wantTxCalls: 1, wantWrites: 1},
		{name: "invalid ID", id: accounts.ID{}, wantErrs: []error{accounts.ErrIDInvalid}, wantStored: true},
		{name: "missing", id: testMissingAccountID, wantErrs: []error{accounts.ErrAccountNotFound}, wantStored: true, wantTxCalls: 1},
		{
			name:        "delete failure",
			id:          testAccountID,
			writeErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantStored:  true,
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspaceStore := newWorkspaceStore(t, testMemberships()...)
			store := newAccountStore(t, workspaceStore)
			store.writeErr = tt.writeErr
			transactor := &appmock.Transactor{}
			service, err := NewAccountService(transactor, store.repository(), workspaceStore.repository())

			if err != nil {
				t.Fatal(err)
			}

			err = service.Delete(memberContext(t, testWorkspaceID, testEditorID, workspaces.RoleEditor), tt.id)

			if transactor.Calls != tt.wantTxCalls || store.writes != tt.wantWrites {
				t.Fatalf("Delete() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, store.writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if _, stored := store.accounts[testAccountID]; stored != tt.wantStored {
				t.Fatalf("account stored = %t, want %t", stored, tt.wantStored)
			}
		})
	}
}

//
// Helpers
//

func validCreateAccountInput() input.CreateAccountInput {
	return input.CreateAccountInput{WorkspaceID: testWorkspaceID, UserID: new(testEditorID), CurrencyCode: "EUR", Name: "Cash"}
}

func validUpdateAccountInput() input.UpdateAccountInput {
	return input.UpdateAccountInput{ID: testAccountID, UserID: new(testViewerID), Name: "Card"}
}

func validAccountQuery() accounts.AccountQuery {
	return accounts.NewAccountQuery(accounts.AccountFilter{}, domquery.SortOrderAsc, accounts.AccountSortName, 10, 1)
}

func sameUserID(a *users.UserID, b *users.UserID) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

// Every operation on the test Account or its Workspace, with the error that
// reports either as missing to a caller who is not a member.

type accountOperation struct {
	name     string
	write    bool
	notFound error
	call     func(ctx context.Context, service *AccountService) error
}

func accountOperations() []accountOperation {
	return []accountOperation{
		{"view", false, accounts.ErrAccountNotFound, func(ctx context.Context, service *AccountService) error {
			_, err := service.FindByID(ctx, testAccountID)
			return err
		}},
		{"list", false, workspaces.ErrWorkspaceNotFound, func(ctx context.Context, service *AccountService) error {
			_, err := service.List(ctx, testWorkspaceID, validAccountQuery())
			return err
		}},
		{"create", true, workspaces.ErrWorkspaceNotFound, func(ctx context.Context, service *AccountService) error {
			_, err := service.Create(ctx, validCreateAccountInput())
			return err
		}},
		{"update", true, accounts.ErrAccountNotFound, func(ctx context.Context, service *AccountService) error {
			_, err := service.Update(ctx, validUpdateAccountInput())
			return err
		}},
		{"delete", true, accounts.ErrAccountNotFound, func(ctx context.Context, service *AccountService) error {
			return service.Delete(ctx, testAccountID)
		}},
	}
}

// The store holds one Account of the editor in the test Workspace. Writes must
// run in a transaction after the Workspace was locked.

type accountStore struct {
	t         *testing.T
	workspace *workspaceStore
	accounts  map[accounts.ID]*accounts.Account
	readErr   error
	writeErr  error
	writes    int
}

func newAccountStore(t *testing.T, workspace *workspaceStore) *accountStore {
	return &accountStore{
		t:         t,
		workspace: workspace,
		accounts: map[accounts.ID]*accounts.Account{
			testAccountID: accounts.NewAccount(testAccountID, testWorkspaceID, new(testEditorID), "EUR", "Cash"),
		},
	}
}

func newAccountStoreService(t *testing.T, store *accountStore, workspaceStore *workspaceStore) *AccountService {
	t.Helper()

	service, err := NewAccountService(&appmock.Transactor{}, store.repository(), workspaceStore.repository())

	if err != nil {
		t.Fatal(err)
	}

	return service
}

func (s *accountStore) write(ctx context.Context) error {
	s.writes++

	if !appmock.InTransaction(ctx) {
		s.t.Error("write ran outside a transaction")
	}

	if !s.workspace.locked {
		s.t.Error("write ran before the workspace was locked")
	}

	return s.writeErr
}

func (s *accountStore) repository() *accountsmock.AccountRepository {
	return &accountsmock.AccountRepository{
		CreateFunc: func(ctx context.Context, account *accounts.Account) accounts.ErrAccountCreateFailed {
			var err accounts.ErrAccountCreateFailed

			if writeErr := s.write(ctx); writeErr != nil {
				err = accountsmock.NewErrAccountCreateFailed(account, writeErr)
			} else {
				s.accounts[account.ID] = account
			}

			return err
		},
		UpdateFunc: func(ctx context.Context, account *accounts.Account) accounts.ErrAccountUpdateFailed {
			var err accounts.ErrAccountUpdateFailed

			writeErr := s.write(ctx)

			if writeErr == nil && s.accounts[account.ID] == nil {
				writeErr = accounts.ErrAccountNotFound
			}

			if writeErr != nil {
				err = accountsmock.NewErrAccountUpdateFailed(account, writeErr)
			} else {
				s.accounts[account.ID] = account
			}

			return err
		},
		DeleteFunc: func(ctx context.Context, id accounts.ID) accounts.ErrAccountDeleteFailed {
			var err accounts.ErrAccountDeleteFailed

			writeErr := s.write(ctx)

			if writeErr == nil && s.accounts[id] == nil {
				writeErr = accounts.ErrAccountNotFound
			}

			if writeErr != nil {
				err = accountsmock.NewErrAccountDeleteFailed(id, writeErr)
			} else {
				delete(s.accounts, id)
			}

			return err
		},
		FindByIDFunc: func(_ context.Context, id accounts.ID) (*accounts.Account, error) {
			var (
				found *accounts.Account
				err   = accounts.ErrAccountNotFound
			)

			if s.readErr != nil {
				err = s.readErr
			} else if account, ok := s.accounts[id]; ok {
				found, err = account, nil
			}

			return found, err
		},
		FindFunc: func(_ context.Context, query accounts.AccountQuery) ([]*accounts.Account, error) {
			var result []*accounts.Account

			for _, account := range s.accounts {
				if query.Filter.WorkspaceID != nil && account.WorkspaceID == *query.Filter.WorkspaceID {
					result = append(result, account)
				}
			}

			return result, s.readErr
		},
		CountFunc: func(_ context.Context, filter accounts.AccountFilter) (int, error) {
			total := 0

			for _, account := range s.accounts {
				if filter.WorkspaceID != nil && account.WorkspaceID == *filter.WorkspaceID {
					total++
				}
			}

			return total, s.readErr
		},
	}
}
