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
	"github.com/radynsade/faryengo/internal/users"
	usersmock "github.com/radynsade/faryengo/internal/users/mock"
	"github.com/radynsade/faryengo/internal/workspaces"
	workspacesmock "github.com/radynsade/faryengo/internal/workspaces/mock"
	"github.com/radynsade/faryengo/pkg/domquery"
)

var testWorkspaceID = workspaces.WorkspaceID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))

func TestNewWorkspaceService(t *testing.T) {
	for _, tt := range []struct {
		name       string
		transactor Transactor
		repository workspaces.WorkspaceRepository
		users      users.UserRepository
		wantErr    error
	}{
		{name: "created", transactor: &appmock.Transactor{}, repository: &workspacesmock.WorkspaceRepository{}, users: &usersmock.UserRepository{}},
		{name: "nil transactor", repository: &workspacesmock.WorkspaceRepository{}, users: &usersmock.UserRepository{}, wantErr: ErrTransactorNil},
		{name: "nil repository", transactor: &appmock.Transactor{}, users: &usersmock.UserRepository{}, wantErr: ErrWorkspaceRepositoryNil},
		{name: "nil user repository", transactor: &appmock.Transactor{}, repository: &workspacesmock.WorkspaceRepository{}, wantErr: ErrUserRepositoryNil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewWorkspaceService(tt.transactor, tt.repository, tt.users)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) || (service == nil) != (tt.wantErr != nil) {
				t.Fatalf("NewWorkspaceService() = (%v, %v), want error %v", service, err, tt.wantErr)
			}
		})
	}
}

func TestWorkspaceServiceNilReceiver(t *testing.T) {
	ctx := t.Context()

	var service *WorkspaceService

	created, createErr := service.Create(ctx, input.CreateWorkspaceInput{Name: "Design"})
	found, findErr := service.FindByID(ctx, testWorkspaceID)
	list, listErr := service.List(ctx, validWorkspaceQuery())
	memberList, memberListErr := service.ListByMember(ctx, testUserID, validWorkspaceQuery())
	renamed, renameErr := service.Rename(ctx, testUserID, input.RenameWorkspaceInput{ID: testWorkspaceID, Name: "Design"})
	deleteErr := service.Delete(ctx, testWorkspaceID)
	addErr := service.AddMember(ctx, testWorkspaceID, testUserID, workspaces.RoleViewer)
	roleErr := service.ChangeMemberRole(ctx, testUserID, input.ChangeMemberRoleInput{WorkspaceID: testWorkspaceID, UserID: testUserID, Role: "owner"})
	removeErr := service.RemoveMember(ctx, testUserID, testWorkspaceID, testUserID)
	member, memberErr := service.FindForMember(ctx, testWorkspaceID, testUserID)
	createdAsUser, createAsUserErr := service.CreateAsUser(ctx, testUserID, input.CreateWorkspaceInput{Name: "Design"})
	addedByEmail, addByEmailErr := service.AddMemberByEmail(ctx, testUserID, input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "anna@example.com"})
	listedMembers, listMembersErr := service.ListMembers(ctx, testUserID, testWorkspaceID)

	for _, err := range []error{
		createErr, findErr, listErr, memberListErr, renameErr, deleteErr, addErr, roleErr, removeErr, memberErr, createAsUserErr, addByEmailErr, listMembersErr,
	} {
		if !errors.Is(err, ErrWorkspaceRepositoryNil) {
			t.Fatalf("nil receiver error = %v, want ErrWorkspaceRepositoryNil", err)
		}
	}

	if created != nil || found != nil || list.Workspaces != nil || memberList.Workspaces != nil || renamed != nil ||
		member.Workspace != nil || createdAsUser != nil || addedByEmail != nil || listedMembers != nil {
		t.Fatalf("nil receiver returned values: %v, %v, %v, %v, %v, %v, %v, %v, %v", created, found, list, memberList, renamed, member, createdAsUser, addedByEmail, listedMembers)
	}
}

func TestWorkspaceServiceCreate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		request     input.CreateWorkspaceInput
		beginErr    error
		createErr   error
		wantName    workspaces.WorkspaceName
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{name: "created", request: input.CreateWorkspaceInput{Name: "Design"}, wantName: "Design", wantTxCalls: 1, wantWrites: 1},
		{
			name:        "name trimmed",
			request:     input.CreateWorkspaceInput{Name: "  Design team \n"},
			wantName:    "Design team",
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "limit counted after trimming",
			request:     input.CreateWorkspaceInput{Name: " " + strings.Repeat("ā", workspaces.MaxWorkspaceNameLength) + " "},
			wantName:    workspaces.WorkspaceName(strings.Repeat("ā", workspaces.MaxWorkspaceNameLength)),
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:     "blank name",
			request:  input.CreateWorkspaceInput{Name: " \t "},
			wantErrs: []error{input.ErrCreateWorkspaceInputInvalid, workspaces.ErrWorkspaceNameEmpty},
		},
		{
			name:     "long name",
			request:  input.CreateWorkspaceInput{Name: strings.Repeat("a", workspaces.MaxWorkspaceNameLength+1)},
			wantErrs: []error{input.ErrCreateWorkspaceInputInvalid, workspaces.ErrWorkspaceNameTooLong},
		},
		{
			name:     "invalid UTF-8",
			request:  input.CreateWorkspaceInput{Name: "Design\xff"},
			wantErrs: []error{input.ErrCreateWorkspaceInputInvalid, workspaces.ErrWorkspaceNameInvalidChars},
		},
		{
			name:        "transaction not started",
			request:     input.CreateWorkspaceInput{Name: "Design"},
			beginErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
		},
		{
			name:        "already exists",
			request:     input.CreateWorkspaceInput{Name: "Design"},
			createErr:   workspaces.ErrWorkspaceAlreadyExists,
			wantErrs:    []error{workspaces.ErrWorkspaceAlreadyExists},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0

			var stored *workspaces.Workspace

			repository := &workspacesmock.WorkspaceRepository{
				CreateFunc: func(ctx context.Context, workspace *workspaces.Workspace) workspaces.ErrWorkspaceCreateFailed {
					var err workspaces.ErrWorkspaceCreateFailed

					writes++
					stored = workspace

					if !appmock.InTransaction(ctx) {
						t.Error("Create() ran outside a transaction")
					}

					if tt.createErr != nil {
						err = workspacesmock.NewErrWorkspaceCreateFailed(workspace, tt.createErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{BeginErr: tt.beginErr}
			service, err := NewWorkspaceService(transactor, repository, &usersmock.UserRepository{})

			if err != nil {
				t.Fatal(err)
			}

			workspace, err := service.Create(t.Context(), tt.request)

			if transactor.Calls != tt.wantTxCalls || writes != tt.wantWrites {
				t.Fatalf("Create() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if workspace != nil {
					t.Fatalf("Create() workspace = %v, want nil on error", workspace)
				}
			} else if workspace != stored || workspace.Name != tt.wantName || uuid.UUID(workspace.ID).Version() != 7 {
				t.Fatalf("Create() workspace = %+v, stored = %+v, want the stored workspace %q with a UUIDv7", workspace, stored, tt.wantName)
			}
		})
	}
}

func TestWorkspaceServiceCreateAsUser(t *testing.T) {
	for _, tt := range []struct {
		name        string
		creatorID   users.UserID
		request     input.CreateWorkspaceInput
		createErr   error
		addErr      error
		wantErrs    []error
		wantTxCalls int
		wantCreates int
		wantAdds    int
	}{
		{
			name:        "created with the creator as its owner",
			creatorID:   testUserID,
			request:     input.CreateWorkspaceInput{Name: " Family budget "},
			wantTxCalls: 1,
			wantCreates: 1,
			wantAdds:    1,
		},
		{
			name:     "missing creator",
			request:  input.CreateWorkspaceInput{Name: "Family budget"},
			wantErrs: []error{users.ErrUserIDInvalid},
		},
		{
			name:      "invalid name",
			creatorID: testUserID,
			request:   input.CreateWorkspaceInput{Name: "  "},
			wantErrs:  []error{input.ErrCreateWorkspaceInputInvalid, workspaces.ErrWorkspaceNameEmpty},
		},
		{
			name:        "workspace not written",
			creatorID:   testUserID,
			request:     input.CreateWorkspaceInput{Name: "Family budget"},
			createErr:   workspaces.ErrWorkspaceAlreadyExists,
			wantErrs:    []error{workspaces.ErrWorkspaceAlreadyExists},
			wantTxCalls: 1,
			wantCreates: 1,
		},
		{
			name:        "creator deleted meanwhile",
			creatorID:   testUserID,
			request:     input.CreateWorkspaceInput{Name: "Family budget"},
			addErr:      users.ErrUserNotFound,
			wantErrs:    []error{users.ErrUserNotFound},
			wantTxCalls: 1,
			wantCreates: 1,
			wantAdds:    1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			creates, adds := 0, 0

			var stored *workspaces.Workspace

			repository := &workspacesmock.WorkspaceRepository{
				CreateFunc: func(ctx context.Context, workspace *workspaces.Workspace) workspaces.ErrWorkspaceCreateFailed {
					var err workspaces.ErrWorkspaceCreateFailed

					creates++
					stored = workspace

					if !appmock.InTransaction(ctx) {
						t.Error("Create() ran outside a transaction")
					}

					if tt.createErr != nil {
						err = workspacesmock.NewErrWorkspaceCreateFailed(workspace, tt.createErr)
					}

					return err
				},
				AddMemberFunc: func(ctx context.Context, membership workspaces.Membership) workspaces.ErrMembershipAddFailed {
					var err workspaces.ErrMembershipAddFailed

					adds++

					if !appmock.InTransaction(ctx) || membership != workspaces.NewMembership(stored.ID, tt.creatorID, workspaces.RoleOwner) {
						t.Errorf("AddMember() called with (%v, %+v), want the creator as the owner of the stored workspace in a transaction", ctx, membership)
					}

					if tt.addErr != nil {
						err = workspacesmock.NewErrMembershipAddFailed(membership, tt.addErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewWorkspaceService(transactor, repository, &usersmock.UserRepository{})

			if err != nil {
				t.Fatal(err)
			}

			workspace, err := service.CreateAsUser(t.Context(), tt.creatorID, tt.request)

			if transactor.Calls != tt.wantTxCalls || creates != tt.wantCreates || adds != tt.wantAdds {
				t.Fatalf(
					"CreateAsUser() calls = (transaction %d, create %d, add %d), want (%d, %d, %d)",
					transactor.Calls, creates, adds, tt.wantTxCalls, tt.wantCreates, tt.wantAdds,
				)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && workspace != nil {
				t.Fatalf("CreateAsUser() workspace = %v, want nil on error", workspace)
			} else if len(tt.wantErrs) == 0 && (workspace != stored || workspace.Name != "Family budget") {
				t.Fatalf("CreateAsUser() workspace = %+v, want the stored, trimmed workspace", workspace)
			}
		})
	}
}

func TestWorkspaceServiceFindByID(t *testing.T) {
	existing := workspaces.NewWorkspace(testWorkspaceID, "Design")

	for _, tt := range []struct {
		name      string
		id        workspaces.WorkspaceID
		findErr   error
		wantErrs  []error
		wantCalls int
	}{
		{name: "found", id: testWorkspaceID, wantCalls: 1},
		{name: "invalid ID", wantErrs: []error{workspaces.ErrWorkspaceIDInvalid}},
		{
			name:      "not found",
			id:        testWorkspaceID,
			findErr:   workspaces.ErrWorkspaceNotFound,
			wantErrs:  []error{workspaces.ErrWorkspaceNotFound},
			wantCalls: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			calls := 0

			repository := &workspacesmock.WorkspaceRepository{
				FindByIDFunc: func(callCtx context.Context, id workspaces.WorkspaceID) (*workspaces.Workspace, error) {
					var workspace *workspaces.Workspace

					calls++

					if callCtx != ctx || id != tt.id {
						t.Errorf("FindByID() called with (%v, %v)", callCtx, id)
					}

					if tt.findErr == nil {
						workspace = existing
					}

					return workspace, tt.findErr
				},
			}

			service, err := NewWorkspaceService(&appmock.Transactor{}, repository, &usersmock.UserRepository{})

			if err != nil {
				t.Fatal(err)
			}

			workspace, err := service.FindByID(ctx, tt.id)

			if calls != tt.wantCalls {
				t.Fatalf("FindByID() repository calls = %d, want %d", calls, tt.wantCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && workspace != nil {
				t.Fatalf("FindByID() workspace = %v, want nil on error", workspace)
			} else if len(tt.wantErrs) == 0 && workspace != existing {
				t.Fatalf("FindByID() workspace = %v, want %v", workspace, existing)
			}
		})
	}
}

// The workspace comes with the member's role. A User who is not a member
// cannot tell the workspace from a missing one.

func TestWorkspaceServiceFindForMember(t *testing.T) {
	storageErr := errors.New("storage unavailable")

	for _, tt := range []struct {
		name        string
		workspaceID workspaces.WorkspaceID
		userID      users.UserID
		readErr     error
		wantRole    workspaces.Role
		wantErrs    []error
	}{
		{name: "owner", workspaceID: testWorkspaceID, userID: testOwnerID, wantRole: workspaces.RoleOwner},
		{name: "viewer", workspaceID: testWorkspaceID, userID: testViewerID, wantRole: workspaces.RoleViewer},
		{
			name:        "not a member reads as missing",
			workspaceID: testWorkspaceID,
			userID:      testOutsiderID,
			wantErrs:    []error{workspaces.ErrWorkspaceNotFound},
		},
		{
			name:        "missing workspace",
			workspaceID: workspaces.WorkspaceID(uuid.MustParse("01920000-0000-7000-8000-0000000000ff")),
			userID:      testOwnerID,
			wantErrs:    []error{workspaces.ErrWorkspaceNotFound},
		},
		{
			name:        "membership read failure",
			workspaceID: testWorkspaceID,
			userID:      testOwnerID,
			readErr:     storageErr,
			wantErrs:    []error{storageErr},
		},
		{name: "invalid workspace ID", userID: testOwnerID, wantErrs: []error{workspaces.ErrWorkspaceIDInvalid}},
		{name: "invalid user ID", workspaceID: testWorkspaceID, wantErrs: []error{users.ErrUserIDInvalid}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newWorkspaceStore(t, testMemberships()...)
			store.readErr = tt.readErr
			service := newStoreService(t, store, &usersmock.UserRepository{})

			found, err := service.FindForMember(t.Context(), tt.workspaceID, tt.userID)

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && found != (MemberWorkspace{}) {
				t.Fatalf("FindForMember() = %+v, want nothing on error", found)
			} else if len(tt.wantErrs) == 0 && (found.Workspace != store.workspace || found.Role != tt.wantRole) {
				t.Fatalf("FindForMember() = %+v, want the workspace with role %q", found, tt.wantRole)
			}
		})
	}
}

// Every office operation is tried by a member of each role, on a workspace
// that keeps another owner and another member to change, so only the actor's
// role decides the outcome. A denied operation writes nothing.

func TestWorkspaceServiceRolePermissions(t *testing.T) {
	operations := []struct {
		name  string
		write bool
		call  func(service *WorkspaceService, actorID, targetID users.UserID) error
	}{
		{"view", false, func(service *WorkspaceService, actorID, _ users.UserID) error {
			_, err := service.FindForMember(t.Context(), testWorkspaceID, actorID)
			return err
		}},
		{"list members", false, func(service *WorkspaceService, actorID, _ users.UserID) error {
			_, err := service.ListMembers(t.Context(), actorID, testWorkspaceID)
			return err
		}},
		{"rename", true, func(service *WorkspaceService, actorID, _ users.UserID) error {
			_, err := service.Rename(t.Context(), actorID, input.RenameWorkspaceInput{ID: testWorkspaceID, Name: "Research"})
			return err
		}},
		{"add member", true, func(service *WorkspaceService, actorID, _ users.UserID) error {
			_, err := service.AddMemberByEmail(t.Context(), actorID, input.AddMemberByEmailInput{
				WorkspaceID: testWorkspaceID,
				Email:       "grace@example.com",
				Role:        "editor",
			})
			return err
		}},
		{"change role", true, func(service *WorkspaceService, actorID, targetID users.UserID) error {
			return service.ChangeMemberRole(t.Context(), actorID, input.ChangeMemberRoleInput{
				WorkspaceID: testWorkspaceID,
				UserID:      targetID,
				Role:        "owner",
			})
		}},
		{"remove member", true, func(service *WorkspaceService, actorID, targetID users.UserID) error {
			return service.RemoveMember(t.Context(), actorID, testWorkspaceID, targetID)
		}},
	}

	for _, tt := range []struct {
		role     workspaces.Role
		actorID  users.UserID
		targetID users.UserID
		allowed  []bool
	}{
		{workspaces.RoleViewer, testViewerID, testEditorID, []bool{true, true, false, false, false, false}},
		{workspaces.RoleEditor, testEditorID, testViewerID, []bool{true, true, false, false, false, false}},
		{workspaces.RoleOwner, testOwnerID, testEditorID, []bool{true, true, true, true, true, true}},
	} {
		for index, operation := range operations {
			t.Run(string(tt.role)+" "+operation.name, func(t *testing.T) {
				store := newWorkspaceStore(t, testMemberships()...)
				service := newStoreService(t, store, testUserRepository(testMembers()...))

				err := operation.call(service, tt.actorID, tt.targetID)

				if tt.allowed[index] {
					if err != nil {
						t.Fatalf("%s as %s = %v, want it allowed", operation.name, tt.role, err)
					}

					if operation.write && store.writes != 1 {
						t.Fatalf("%s as %s wrote %d times, want once", operation.name, tt.role, store.writes)
					}
				} else if !errors.Is(err, workspaces.ErrRolePermissionDenied) || errors.Is(err, workspaces.ErrWorkspaceNotFound) || store.writes != 0 {
					t.Fatalf("%s as %s = %v after %d writes, want %v and no write", operation.name, tt.role, err, store.writes, workspaces.ErrRolePermissionDenied)
				}
			})
		}
	}

	// A User outside the workspace, or acting on a missing one, learns
	// nothing about it. A missing workspace is one the store holds under
	// another ID, where the actor is an owner.
	for _, tt := range []struct {
		name     string
		storedID workspaces.WorkspaceID
		actorID  users.UserID
	}{
		{"not a member", testWorkspaceID, testOutsiderID},
		{"missing workspace", workspaces.WorkspaceID(uuid.MustParse("01920000-0000-7000-8000-0000000000ff")), testOwnerID},
	} {
		for _, operation := range operations {
			t.Run(tt.name+" "+operation.name, func(t *testing.T) {
				store := newWorkspaceStore(t, testMemberships()...)
				store.workspace.ID = tt.storedID

				for index := range store.memberships {
					store.memberships[index].WorkspaceID = tt.storedID
				}

				service := newStoreService(t, store, testUserRepository(testMembers()...))
				err := operation.call(service, tt.actorID, testEditorID)

				if !errors.Is(err, workspaces.ErrWorkspaceNotFound) || errors.Is(err, workspaces.ErrRolePermissionDenied) || store.writes != 0 {
					t.Fatalf("%s = %v after %d writes, want %v and no write", operation.name, err, store.writes, workspaces.ErrWorkspaceNotFound)
				}
			})
		}
	}
}

func TestWorkspaceServiceList(t *testing.T) {
	withoutPage := validWorkspaceQuery()
	withoutPage.Page = 0
	unknownSort := validWorkspaceQuery()
	unknownSort.SortBy = "created_at"
	longFilter := validWorkspaceQuery()
	longFilter.Filter.NameLike = strings.Repeat("a", workspaces.MaxWorkspaceFilterNameLikeLength+1)
	otherMember := users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000009"))
	withOtherMember := validWorkspaceQuery()
	withOtherMember.Filter.MemberID = &otherMember

	for _, tt := range []struct {
		name         string
		byMember     bool
		memberID     users.UserID
		query        workspaces.WorkspaceQuery
		countErr     error
		findErr      error
		wantMemberID *users.UserID
		wantErrs     []error
		wantCalls    int
		wantTotal    int
	}{
		{name: "listed", query: validWorkspaceQuery(), wantCalls: 2, wantTotal: 7},
		{name: "missing page", query: withoutPage, wantErrs: []error{workspaces.ErrWorkspaceQueryInvalid}},
		{name: "unknown sort", query: unknownSort, wantErrs: []error{workspaces.ErrWorkspaceQueryInvalid}},
		{name: "invalid filter", query: longFilter, wantErrs: []error{workspaces.ErrWorkspaceFilterNameLikeTooLong}},
		{
			name:      "count failure",
			query:     validWorkspaceQuery(),
			countErr:  context.DeadlineExceeded,
			wantErrs:  []error{context.DeadlineExceeded},
			wantCalls: 1,
		},
		{
			name:      "find failure",
			query:     validWorkspaceQuery(),
			findErr:   context.DeadlineExceeded,
			wantErrs:  []error{context.DeadlineExceeded},
			wantCalls: 2,
		},
		{
			name:         "listed by member",
			byMember:     true,
			memberID:     testUserID,
			query:        validWorkspaceQuery(),
			wantMemberID: &testUserID,
			wantCalls:    2,
			wantTotal:    7,
		},
		{
			name:         "member replaces the query's member",
			byMember:     true,
			memberID:     testUserID,
			query:        withOtherMember,
			wantMemberID: &testUserID,
			wantCalls:    2,
			wantTotal:    7,
		},
		{
			name:     "invalid member",
			byMember: true,
			query:    validWorkspaceQuery(),
			wantErrs: []error{users.ErrUserIDInvalid},
		},
		{
			name:     "invalid query by member",
			byMember: true,
			memberID: testUserID,
			query:    withoutPage,
			wantErrs: []error{workspaces.ErrWorkspaceQueryInvalid},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			calls := 0
			found := []*workspaces.Workspace{workspaces.NewWorkspace(testWorkspaceID, "Design")}
			checkFilter := func(callCtx context.Context, filter workspaces.WorkspaceFilter) {
				memberMatches := (filter.MemberID == nil && tt.wantMemberID == nil) ||
					(filter.MemberID != nil && tt.wantMemberID != nil && *filter.MemberID == *tt.wantMemberID)

				if callCtx != ctx || filter.NameLike != tt.query.Filter.NameLike || !memberMatches {
					t.Errorf("repository called with (%v, %+v), want member %v", callCtx, filter, tt.wantMemberID)
				}
			}

			repository := &workspacesmock.WorkspaceRepository{
				CountFunc: func(callCtx context.Context, filter workspaces.WorkspaceFilter) (int, error) {
					calls++
					checkFilter(callCtx, filter)

					return 7, tt.countErr
				},
				FindFunc: func(callCtx context.Context, query workspaces.WorkspaceQuery) ([]*workspaces.Workspace, error) {
					calls++
					checkFilter(callCtx, query.Filter)

					if query.Page != tt.query.Page || query.SortBy != tt.query.SortBy {
						t.Errorf("Find() called with %+v", query)
					}

					return found, tt.findErr
				},
			}

			service, err := NewWorkspaceService(&appmock.Transactor{}, repository, &usersmock.UserRepository{})

			if err != nil {
				t.Fatal(err)
			}

			var list WorkspaceList

			if tt.byMember {
				list, err = service.ListByMember(ctx, tt.memberID, tt.query)
			} else {
				list, err = service.List(ctx, tt.query)
			}

			if calls != tt.wantCalls {
				t.Fatalf("list repository calls = %d, want %d", calls, tt.wantCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && (list.Total != 0 || list.Workspaces != nil) {
				t.Fatalf("list = %+v, want empty on error", list)
			} else if len(tt.wantErrs) == 0 && (list.Total != tt.wantTotal || len(list.Workspaces) != len(found)) {
				t.Fatalf("list = %+v, want %d of total %d", list, len(found), tt.wantTotal)
			}
		})
	}
}

// The workspace is locked before the owner's role is read and the new name
// written.

func TestWorkspaceServiceRename(t *testing.T) {
	for _, tt := range []struct {
		name        string
		actorID     users.UserID
		request     input.RenameWorkspaceInput
		lockErr     error
		writeErr    error
		wantName    workspaces.WorkspaceName
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{
			name:        "renamed",
			actorID:     testOwnerID,
			request:     input.RenameWorkspaceInput{ID: testWorkspaceID, Name: " Research "},
			wantName:    "Research",
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:    "all fields invalid",
			request: input.RenameWorkspaceInput{Name: "  "},
			wantErrs: []error{
				input.ErrRenameWorkspaceInputInvalid,
				workspaces.ErrWorkspaceIDInvalid,
				users.ErrUserIDInvalid,
				workspaces.ErrWorkspaceNameEmpty,
			},
			wantName: "Design",
		},
		{
			name:     "long name",
			actorID:  testOwnerID,
			request:  input.RenameWorkspaceInput{ID: testWorkspaceID, Name: strings.Repeat("a", workspaces.MaxWorkspaceNameLength+1)},
			wantErrs: []error{input.ErrRenameWorkspaceInputInvalid, workspaces.ErrWorkspaceNameTooLong},
			wantName: "Design",
		},
		{
			name:        "lock failure",
			actorID:     testOwnerID,
			request:     input.RenameWorkspaceInput{ID: testWorkspaceID, Name: "Research"},
			lockErr:     context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantName:    "Design",
			wantTxCalls: 1,
		},
		{
			name:        "update failure",
			actorID:     testOwnerID,
			request:     input.RenameWorkspaceInput{ID: testWorkspaceID, Name: "Research"},
			writeErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantName:    "Design",
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newWorkspaceStore(t, testMemberships()...)
			store.lockErr, store.writeErr = tt.lockErr, tt.writeErr
			transactor := &appmock.Transactor{}
			service, err := NewWorkspaceService(transactor, store.repository(), &usersmock.UserRepository{})

			if err != nil {
				t.Fatal(err)
			}

			workspace, err := service.Rename(t.Context(), tt.actorID, tt.request)

			if transactor.Calls != tt.wantTxCalls || store.writes != tt.wantWrites {
				t.Fatalf("Rename() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, store.writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if store.workspace.Name != tt.wantName {
				t.Fatalf("stored name = %q, want %q", store.workspace.Name, tt.wantName)
			}

			if len(tt.wantErrs) > 0 && workspace != nil {
				t.Fatalf("Rename() workspace = %v, want nil on error", workspace)
			} else if len(tt.wantErrs) == 0 && workspace != store.workspace {
				t.Fatalf("Rename() workspace = %+v, want the stored workspace", workspace)
			}
		})
	}
}

func TestWorkspaceServiceDelete(t *testing.T) {
	for _, tt := range []struct {
		name        string
		id          workspaces.WorkspaceID
		deleteErr   error
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{name: "deleted", id: testWorkspaceID, wantTxCalls: 1, wantWrites: 1},
		{name: "invalid ID", wantErrs: []error{workspaces.ErrWorkspaceIDInvalid}},
		{
			name:        "not found",
			id:          testWorkspaceID,
			deleteErr:   workspaces.ErrWorkspaceNotFound,
			wantErrs:    []error{workspaces.ErrWorkspaceNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0

			repository := &workspacesmock.WorkspaceRepository{
				DeleteFunc: func(ctx context.Context, id workspaces.WorkspaceID) workspaces.ErrWorkspaceDeleteFailed {
					var err workspaces.ErrWorkspaceDeleteFailed

					writes++

					if !appmock.InTransaction(ctx) || id != tt.id {
						t.Errorf("Delete() called with (%v, %v), want a transaction", ctx, id)
					}

					if tt.deleteErr != nil {
						err = workspacesmock.NewErrWorkspaceDeleteFailed(id, tt.deleteErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewWorkspaceService(transactor, repository, &usersmock.UserRepository{})

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

// Adding a member by ID serves the admin panel and the CLI, so it checks no
// workspace role and needs no lock: the workspace may have no members at all.

func TestWorkspaceServiceAddMember(t *testing.T) {
	for _, tt := range []struct {
		name        string
		workspaceID workspaces.WorkspaceID
		userID      users.UserID
		role        workspaces.Role
		existing    []workspaces.Membership
		writeErr    error
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{name: "added", workspaceID: testWorkspaceID, userID: testUserID, role: workspaces.RoleEditor, wantTxCalls: 1, wantWrites: 1},
		{
			name:        "added beside an owner",
			workspaceID: testWorkspaceID,
			userID:      testUserID,
			role:        workspaces.RoleViewer,
			existing:    testMemberships(),
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:     "add without a workspace",
			userID:   testUserID,
			role:     workspaces.RoleViewer,
			wantErrs: []error{workspaces.ErrMembershipInvalid, workspaces.ErrWorkspaceIDInvalid},
		},
		{
			name:        "add without a user",
			workspaceID: testWorkspaceID,
			role:        workspaces.RoleViewer,
			wantErrs:    []error{workspaces.ErrMembershipInvalid, users.ErrUserIDInvalid},
		},
		{
			name:        "add without a role",
			workspaceID: testWorkspaceID,
			userID:      testUserID,
			wantErrs:    []error{workspaces.ErrMembershipInvalid, workspaces.ErrRoleInvalid},
		},
		{
			name:        "already a member",
			workspaceID: testWorkspaceID,
			userID:      testOwnerID,
			role:        workspaces.RoleViewer,
			existing:    testMemberships(),
			wantErrs:    []error{workspaces.ErrMembershipAlreadyExists},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "add a missing user",
			workspaceID: testWorkspaceID,
			userID:      testUserID,
			role:        workspaces.RoleViewer,
			writeErr:    users.ErrUserNotFound,
			wantErrs:    []error{users.ErrUserNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newWorkspaceStore(t, tt.existing...)
			store.writeErr = tt.writeErr
			transactor := &appmock.Transactor{}
			service, err := NewWorkspaceService(transactor, store.repository(), &usersmock.UserRepository{})

			if err != nil {
				t.Fatal(err)
			}

			err = service.AddMember(t.Context(), tt.workspaceID, tt.userID, tt.role)

			if transactor.Calls != tt.wantTxCalls || store.writes != tt.wantWrites || store.locked {
				t.Fatalf(
					"calls = (transaction %d, write %d, locked %t), want (%d, %d, false)",
					transactor.Calls, store.writes, store.locked, tt.wantTxCalls, tt.wantWrites,
				)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) == 0 && store.role(tt.userID) != tt.role {
				t.Fatalf("stored role = %q, want %q", store.role(tt.userID), tt.role)
			}
		})
	}
}

func validWorkspaceQuery() workspaces.WorkspaceQuery {
	return workspaces.WorkspaceQuery{
		Filter:    workspaces.WorkspaceFilter{NameLike: "des"},
		SortOrder: domquery.SortOrderAsc,
		SortBy:    workspaces.WorkspaceSortName,
		Limit:     20,
		Page:      1,
	}
}

// An owner adds a User found by email, ignoring letter case; a member added
// without a role is a viewer.

func TestWorkspaceServiceAddMemberByEmail(t *testing.T) {
	for _, tt := range []struct {
		name       string
		request    input.AddMemberByEmailInput
		writeErr   error
		wantRole   workspaces.Role
		wantErrs   []error
		wantWrites int
	}{
		{
			name:       "added as a viewer by default",
			request:    input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "Grace@Example.com"},
			wantRole:   workspaces.RoleViewer,
			wantWrites: 1,
		},
		{
			name:       "added as an editor",
			request:    input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "grace@example.com", Role: "editor"},
			wantRole:   workspaces.RoleEditor,
			wantWrites: 1,
		},
		{
			name:       "added as an owner",
			request:    input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "grace@example.com", Role: "owner"},
			wantRole:   workspaces.RoleOwner,
			wantWrites: 1,
		},
		{
			name:    "invalid fields",
			request: input.AddMemberByEmailInput{Email: "Grace <grace@example.com>", Role: "admin"},
			wantErrs: []error{
				input.ErrAddMemberByEmailInputInvalid,
				workspaces.ErrWorkspaceIDInvalid,
				users.ErrEmailInvalid,
				workspaces.ErrRoleInvalid,
			},
		},
		{
			name:     "unknown email",
			request:  input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "nobody@example.com"},
			wantErrs: []error{users.ErrUserNotFound},
		},
		{
			name:       "already a member",
			request:    input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "edgar@example.com"},
			wantErrs:   []error{workspaces.ErrMembershipAlreadyExists},
			wantWrites: 1,
		},
		{
			name:       "write failure",
			request:    input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "grace@example.com"},
			writeErr:   context.DeadlineExceeded,
			wantErrs:   []error{context.DeadlineExceeded},
			wantWrites: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newWorkspaceStore(t, testMemberships()...)
			store.writeErr = tt.writeErr
			service := newStoreService(t, store, testUserRepository(testMembers()...))

			user, err := service.AddMemberByEmail(t.Context(), testOwnerID, tt.request)

			if store.writes != tt.wantWrites {
				t.Fatalf("writes = %d, want %d", store.writes, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && user != nil {
				t.Fatalf("AddMemberByEmail() user = %v, want nil on error", user)
			} else if len(tt.wantErrs) == 0 && (user == nil || user.ID != testNewcomerID || store.role(testNewcomerID) != tt.wantRole) {
				t.Fatalf("AddMemberByEmail() user = %v with role %q, want the newcomer as %q", user, store.role(testNewcomerID), tt.wantRole)
			}
		})
	}
}

// An owner changes any member's role, their own included, as long as the
// workspace keeps an owner.

func TestWorkspaceServiceChangeMemberRole(t *testing.T) {
	twoOwners := append(testMemberships(), membershipOf(testNewcomerID, workspaces.RoleOwner))

	for _, tt := range []struct {
		name        string
		memberships []workspaces.Membership
		actorID     users.UserID
		request     input.ChangeMemberRoleInput
		writeErr    error
		wantRole    workspaces.Role
		wantErrs    []error
		wantWrites  int
	}{
		{
			name:       "editor promoted",
			actorID:    testOwnerID,
			request:    input.ChangeMemberRoleInput{WorkspaceID: testWorkspaceID, UserID: testEditorID, Role: "owner"},
			wantRole:   workspaces.RoleOwner,
			wantWrites: 1,
		},
		{
			name:       "viewer made an editor",
			actorID:    testOwnerID,
			request:    input.ChangeMemberRoleInput{WorkspaceID: testWorkspaceID, UserID: testViewerID, Role: "editor"},
			wantRole:   workspaces.RoleEditor,
			wantWrites: 1,
		},
		{
			name:        "another owner demoted",
			memberships: twoOwners,
			actorID:     testOwnerID,
			request:     input.ChangeMemberRoleInput{WorkspaceID: testWorkspaceID, UserID: testNewcomerID, Role: "viewer"},
			wantRole:    workspaces.RoleViewer,
			wantWrites:  1,
		},
		{
			name:        "owner demotes themselves beside another owner",
			memberships: twoOwners,
			actorID:     testOwnerID,
			request:     input.ChangeMemberRoleInput{WorkspaceID: testWorkspaceID, UserID: testOwnerID, Role: "editor"},
			wantRole:    workspaces.RoleEditor,
			wantWrites:  1,
		},
		{
			name:     "last owner demoted",
			actorID:  testOwnerID,
			request:  input.ChangeMemberRoleInput{WorkspaceID: testWorkspaceID, UserID: testOwnerID, Role: "editor"},
			wantRole: workspaces.RoleOwner,
			wantErrs: []error{workspaces.ErrMembershipLastOwner},
		},
		{
			name:       "last owner kept an owner",
			actorID:    testOwnerID,
			request:    input.ChangeMemberRoleInput{WorkspaceID: testWorkspaceID, UserID: testOwnerID, Role: "owner"},
			wantRole:   workspaces.RoleOwner,
			wantWrites: 1,
		},
		{
			name:    "invalid fields",
			request: input.ChangeMemberRoleInput{Role: "admin"},
			wantErrs: []error{
				input.ErrChangeMemberRoleInputInvalid,
				workspaces.ErrWorkspaceIDInvalid,
				users.ErrUserIDInvalid,
				workspaces.ErrRoleInvalid,
			},
		},
		{
			name:     "missing role",
			actorID:  testOwnerID,
			request:  input.ChangeMemberRoleInput{WorkspaceID: testWorkspaceID, UserID: testEditorID},
			wantRole: workspaces.RoleEditor,
			wantErrs: []error{input.ErrChangeMemberRoleInputInvalid, workspaces.ErrRoleInvalid},
		},
		{
			name:     "not a member",
			actorID:  testOwnerID,
			request:  input.ChangeMemberRoleInput{WorkspaceID: testWorkspaceID, UserID: testNewcomerID, Role: "editor"},
			wantErrs: []error{workspaces.ErrMembershipNotFound},
		},
		{
			name:       "write failure",
			actorID:    testOwnerID,
			request:    input.ChangeMemberRoleInput{WorkspaceID: testWorkspaceID, UserID: testEditorID, Role: "viewer"},
			writeErr:   context.DeadlineExceeded,
			wantRole:   workspaces.RoleEditor,
			wantErrs:   []error{context.DeadlineExceeded},
			wantWrites: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			memberships := tt.memberships

			if memberships == nil {
				memberships = testMemberships()
			}

			store := newWorkspaceStore(t, memberships...)
			store.writeErr = tt.writeErr
			service := newStoreService(t, store, &usersmock.UserRepository{})

			err := service.ChangeMemberRole(t.Context(), tt.actorID, tt.request)

			if store.writes != tt.wantWrites {
				t.Fatalf("writes = %d, want %d", store.writes, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if role := store.role(tt.request.UserID); role != tt.wantRole {
				t.Fatalf("stored role = %q, want %q", role, tt.wantRole)
			}
		})
	}
}

// An owner removes any member while the workspace keeps an owner.

func TestWorkspaceServiceRemoveMember(t *testing.T) {
	twoOwners := append(testMemberships(), membershipOf(testNewcomerID, workspaces.RoleOwner))

	for _, tt := range []struct {
		name        string
		memberships []workspaces.Membership
		userID      users.UserID
		writeErr    error
		wantRemoved bool
		wantErrs    []error
		wantWrites  int
	}{
		{name: "editor removed", userID: testEditorID, wantRemoved: true, wantWrites: 1},
		{name: "another owner removed", memberships: twoOwners, userID: testNewcomerID, wantRemoved: true, wantWrites: 1},
		{name: "owner leaves beside another owner", memberships: twoOwners, userID: testOwnerID, wantRemoved: true, wantWrites: 1},
		{name: "last owner removed", userID: testOwnerID, wantErrs: []error{workspaces.ErrMembershipLastOwner}},
		{name: "not a member", userID: testNewcomerID, wantErrs: []error{workspaces.ErrMembershipNotFound}},
		{name: "invalid user", wantErrs: []error{users.ErrUserIDInvalid}},
		{
			name:       "write failure",
			userID:     testViewerID,
			writeErr:   context.DeadlineExceeded,
			wantErrs:   []error{context.DeadlineExceeded},
			wantWrites: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			memberships := tt.memberships

			if memberships == nil {
				memberships = testMemberships()
			}

			store := newWorkspaceStore(t, memberships...)
			store.writeErr = tt.writeErr
			before := store.role(tt.userID)
			service := newStoreService(t, store, &usersmock.UserRepository{})

			err := service.RemoveMember(t.Context(), testOwnerID, testWorkspaceID, tt.userID)

			if store.writes != tt.wantWrites {
				t.Fatalf("writes = %d, want %d", store.writes, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if removed := before != "" && store.role(tt.userID) == ""; removed != tt.wantRemoved {
				t.Fatalf("member removed = %t, want %t", removed, tt.wantRemoved)
			}
		})
	}
}

// Every member sees the members in name order, each with their role.

func TestWorkspaceServiceListMembers(t *testing.T) {
	storageErr := errors.New("storage unavailable")

	for _, tt := range []struct {
		name      string
		id        workspaces.WorkspaceID
		readErr   error
		usersErr  error
		wantErrs  []error
		wantNames []string
		wantRoles []workspaces.Role
	}{
		{
			name:      "listed",
			id:        testWorkspaceID,
			wantNames: []string{"Edgar", "Olga", "Vera"},
			wantRoles: []workspaces.Role{workspaces.RoleEditor, workspaces.RoleOwner, workspaces.RoleViewer},
		},
		{name: "invalid ID", wantErrs: []error{workspaces.ErrWorkspaceIDInvalid}},
		{name: "membership failure", id: testWorkspaceID, readErr: storageErr, wantErrs: []error{storageErr}},
		{name: "user failure", id: testWorkspaceID, usersErr: storageErr, wantErrs: []error{storageErr}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newWorkspaceStore(t, testMemberships()...)
			store.readErr = tt.readErr
			userRepository := testUserRepository(testMembers()...)
			findByIDs := userRepository.FindByIDsFunc
			userRepository.FindByIDsFunc = func(ctx context.Context, ids []users.UserID) ([]*users.User, error) {
				found, err := findByIDs(ctx, ids)

				if tt.usersErr != nil {
					found, err = nil, tt.usersErr
				}

				return found, err
			}

			service := newStoreService(t, store, userRepository)

			members, err := service.ListMembers(t.Context(), testViewerID, tt.id)

			assertErrors(t, err, tt.wantErrs)

			if len(members) != len(tt.wantNames) {
				t.Fatalf("ListMembers() = %v, want %v", members, tt.wantNames)
			}

			for index, member := range members {
				if string(member.User.FirstName) != tt.wantNames[index] || member.Role != tt.wantRoles[index] {
					t.Fatalf("member %d = %s as %q, want %s as %q", index, member.User.FirstName, member.Role, tt.wantNames[index], tt.wantRoles[index])
				}
			}
		})
	}
}

//
// Workspace store
//

// The workspace has an owner, an editor, and a viewer. The newcomer has an
// account but no membership, and the outsider is neither.

var (
	testOwnerID    = users.UserID(uuid.MustParse("01920000-0000-7000-8000-0000000000a1"))
	testEditorID   = users.UserID(uuid.MustParse("01920000-0000-7000-8000-0000000000a2"))
	testViewerID   = users.UserID(uuid.MustParse("01920000-0000-7000-8000-0000000000a3"))
	testNewcomerID = users.UserID(uuid.MustParse("01920000-0000-7000-8000-0000000000a4"))
	testOutsiderID = users.UserID(uuid.MustParse("01920000-0000-7000-8000-0000000000a5"))
)

func testMemberships() []workspaces.Membership {
	return []workspaces.Membership{
		membershipOf(testOwnerID, workspaces.RoleOwner),
		membershipOf(testEditorID, workspaces.RoleEditor),
		membershipOf(testViewerID, workspaces.RoleViewer),
	}
}

func membershipOf(userID users.UserID, role workspaces.Role) workspaces.Membership {
	return workspaces.NewMembership(testWorkspaceID, userID, role)
}

func testMembers() []*users.User {
	return []*users.User{
		testMember(testOwnerID, "olga@example.com", "Olga"),
		testMember(testEditorID, "edgar@example.com", "Edgar"),
		testMember(testViewerID, "vera@example.com", "Vera"),
		testMember(testNewcomerID, "grace@example.com", "Grace"),
	}
}

func testMember(id users.UserID, email users.Email, firstName users.FirstName) *users.User {
	user := testUser()
	user.ID, user.Email, user.FirstName = id, email, firstName

	return user
}

// Users are found by email ignoring letter case, and by IDs in first-name
// order.

func testUserRepository(found ...*users.User) *usersmock.UserRepository {
	return &usersmock.UserRepository{
		FindByEmailFunc: func(_ context.Context, email users.Email) (*users.User, error) {
			var (
				user *users.User
				err  = users.ErrUserNotFound
			)

			for _, candidate := range found {
				if strings.EqualFold(string(candidate.Email), string(email)) {
					user, err = candidate, nil
				}
			}

			return user, err
		},
		FindByIDsFunc: func(_ context.Context, ids []users.UserID) ([]*users.User, error) {
			result := []*users.User{}

			for _, candidate := range found {
				if slices.Contains(ids, candidate.ID) {
					result = append(result, candidate)
				}
			}

			slices.SortFunc(result, func(a, b *users.User) int {
				return strings.Compare(string(a.FirstName), string(b.FirstName))
			})

			return result, nil
		},
	}
}

// workspaceStore keeps one workspace and its memberships in memory behind the
// complete repository contract. Every write must run in a transaction, and
// every write but adding a member must follow the workspace's lock, like the
// service's own. Writes are counted, and fail with writeErr when it is set.

type workspaceStore struct {
	t           *testing.T
	workspace   *workspaces.Workspace
	memberships workspaces.Memberships
	lockErr     error
	readErr     error
	writeErr    error
	locked      bool
	writes      int
}

func newWorkspaceStore(t *testing.T, memberships ...workspaces.Membership) *workspaceStore {
	return &workspaceStore{
		t:           t,
		workspace:   workspaces.NewWorkspace(testWorkspaceID, "Design"),
		memberships: slices.Clone(memberships),
	}
}

func newStoreService(t *testing.T, store *workspaceStore, userRepository users.UserRepository) *WorkspaceService {
	t.Helper()

	service, err := NewWorkspaceService(&appmock.Transactor{}, store.repository(), userRepository)

	if err != nil {
		t.Fatal(err)
	}

	return service
}

func (s *workspaceStore) role(userID users.UserID) workspaces.Role {
	var role workspaces.Role

	if index := s.index(userID); index >= 0 {
		role = s.memberships[index].Role
	}

	return role
}

func (s *workspaceStore) index(userID users.UserID) int {
	return slices.IndexFunc(s.memberships, func(membership workspaces.Membership) bool {
		return membership.UserID == userID
	})
}

func (s *workspaceStore) write(ctx context.Context, needsLock bool) error {
	s.writes++

	if !appmock.InTransaction(ctx) {
		s.t.Error("write ran outside a transaction")
	}

	if needsLock && !s.locked {
		s.t.Error("write ran before the workspace was locked")
	}

	return s.writeErr
}

func (s *workspaceStore) repository() *workspacesmock.WorkspaceRepository {
	return &workspacesmock.WorkspaceRepository{
		FindByIDFunc: func(_ context.Context, id workspaces.WorkspaceID) (*workspaces.Workspace, error) {
			var (
				found *workspaces.Workspace
				err   = workspaces.ErrWorkspaceNotFound
			)

			if s.readErr != nil {
				err = s.readErr
			} else if id == s.workspace.ID {
				found, err = s.workspace, nil
			}

			return found, err
		},
		FindByIDForUpdateFunc: func(ctx context.Context, id workspaces.WorkspaceID) (*workspaces.Workspace, error) {
			var (
				found *workspaces.Workspace
				err   = workspaces.ErrWorkspaceNotFound
			)

			if !appmock.InTransaction(ctx) {
				s.t.Error("FindByIDForUpdate() ran outside a transaction")
			}

			if s.lockErr != nil {
				err = s.lockErr
			} else if id == s.workspace.ID {
				copied := *s.workspace
				found, err, s.locked = &copied, nil, true
			}

			return found, err
		},
		UpdateFunc: func(ctx context.Context, workspace *workspaces.Workspace) workspaces.ErrWorkspaceUpdateFailed {
			var err workspaces.ErrWorkspaceUpdateFailed

			if writeErr := s.write(ctx, true); writeErr != nil {
				err = workspacesmock.NewErrWorkspaceUpdateFailed(workspace, writeErr)
			} else {
				s.workspace = workspace
			}

			return err
		},
		AddMemberFunc: func(ctx context.Context, membership workspaces.Membership) workspaces.ErrMembershipAddFailed {
			var err workspaces.ErrMembershipAddFailed

			writeErr := s.write(ctx, false)

			if writeErr == nil && s.index(membership.UserID) >= 0 {
				writeErr = workspaces.ErrMembershipAlreadyExists
			}

			if writeErr != nil {
				err = workspacesmock.NewErrMembershipAddFailed(membership, writeErr)
			} else {
				s.memberships = append(s.memberships, membership)
			}

			return err
		},
		UpdateMemberFunc: func(ctx context.Context, membership workspaces.Membership) workspaces.ErrMembershipUpdateFailed {
			var err workspaces.ErrMembershipUpdateFailed

			writeErr := s.write(ctx, true)
			index := s.index(membership.UserID)

			if writeErr == nil && index < 0 {
				writeErr = workspaces.ErrMembershipNotFound
			}

			if writeErr != nil {
				err = workspacesmock.NewErrMembershipUpdateFailed(membership, writeErr)
			} else {
				s.memberships[index] = membership
			}

			return err
		},
		RemoveMemberFunc: func(ctx context.Context, membership workspaces.Membership) workspaces.ErrMembershipRemoveFailed {
			var err workspaces.ErrMembershipRemoveFailed

			writeErr := s.write(ctx, true)
			index := s.index(membership.UserID)

			if writeErr == nil && index < 0 {
				writeErr = workspaces.ErrMembershipNotFound
			}

			if writeErr != nil {
				err = workspacesmock.NewErrMembershipRemoveFailed(membership, writeErr)
			} else {
				s.memberships = slices.Delete(s.memberships, index, index+1)
			}

			return err
		},
		FindMemberFunc: func(_ context.Context, id workspaces.WorkspaceID, userID users.UserID) (workspaces.Membership, error) {
			var membership workspaces.Membership

			err := s.readErr
			index := s.index(userID)

			if err == nil && (id != s.workspace.ID || index < 0) {
				err = workspaces.ErrMembershipNotFound
			} else if err == nil {
				membership = s.memberships[index]
			}

			return membership, err
		},
		FindMembersFunc: func(_ context.Context, id workspaces.WorkspaceID) (workspaces.Memberships, error) {
			var result workspaces.Memberships

			err := s.readErr

			if err == nil && id == s.workspace.ID {
				result = slices.Clone(s.memberships)
			} else if err == nil {
				result = workspaces.Memberships{}
			}

			return result, err
		},
	}
}
