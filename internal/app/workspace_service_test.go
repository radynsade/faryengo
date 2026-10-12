package app

import (
	"context"
	"errors"
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
	renamed, renameErr := service.Rename(ctx, input.RenameWorkspaceInput{ID: testWorkspaceID, Name: "Design"})
	deleteErr := service.Delete(ctx, testWorkspaceID)
	addErr := service.AddMember(ctx, testWorkspaceID, testUserID)
	removeErr := service.RemoveMember(ctx, testWorkspaceID, testUserID)
	member, memberErr := service.FindForMember(ctx, testWorkspaceID, testUserID)
	createdAsUser, createAsUserErr := service.CreateAsUser(ctx, testUserID, input.CreateWorkspaceInput{Name: "Design"})
	addedByEmail, addByEmailErr := service.AddMemberByEmail(ctx, input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "anna@example.com"})
	listedMembers, listMembersErr := service.ListMembers(ctx, testWorkspaceID)

	for _, err := range []error{createErr, findErr, listErr, memberListErr, renameErr, deleteErr, addErr, removeErr, memberErr, createAsUserErr, addByEmailErr, listMembersErr} {
		if !errors.Is(err, ErrWorkspaceRepositoryNil) {
			t.Fatalf("nil receiver error = %v, want ErrWorkspaceRepositoryNil", err)
		}
	}

	if created != nil || found != nil || list.Workspaces != nil || memberList.Workspaces != nil || renamed != nil || member != nil || createdAsUser != nil || addedByEmail != nil || listedMembers != nil {
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
			name:        "created with the creator as its member",
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

					if !appmock.InTransaction(ctx) || membership != workspaces.NewMembership(stored.ID, tt.creatorID) {
						t.Errorf("AddMember() called with (%v, %+v), want the creator of the stored workspace in a transaction", ctx, membership)
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

func TestWorkspaceServiceFindForMember(t *testing.T) {
	existing := workspaces.NewWorkspace(testWorkspaceID, "Design")
	storageErr := errors.New("storage unavailable")

	for _, tt := range []struct {
		name          string
		workspaceID   workspaces.WorkspaceID
		userID        users.UserID
		findErr       error
		isMember      bool
		memberErr     error
		wantErrs      []error
		wantFinds     int
		wantChecks    int
		wantWorkspace bool
	}{
		{name: "member", workspaceID: testWorkspaceID, userID: testUserID, isMember: true, wantFinds: 1, wantChecks: 1, wantWorkspace: true},
		{
			name:        "not a member reads as missing",
			workspaceID: testWorkspaceID,
			userID:      testUserID,
			wantErrs:    []error{workspaces.ErrWorkspaceNotFound},
			wantFinds:   1,
			wantChecks:  1,
		},
		{
			name:        "missing workspace",
			workspaceID: testWorkspaceID,
			userID:      testUserID,
			findErr:     workspaces.ErrWorkspaceNotFound,
			wantErrs:    []error{workspaces.ErrWorkspaceNotFound},
			wantFinds:   1,
		},
		{
			name:        "membership check failure",
			workspaceID: testWorkspaceID,
			userID:      testUserID,
			memberErr:   storageErr,
			wantErrs:    []error{storageErr},
			wantFinds:   1,
			wantChecks:  1,
		},
		{name: "invalid workspace ID", userID: testUserID, wantErrs: []error{workspaces.ErrMembershipInvalid, workspaces.ErrWorkspaceIDInvalid}},
		{name: "invalid user ID", workspaceID: testWorkspaceID, wantErrs: []error{workspaces.ErrMembershipInvalid, users.ErrUserIDInvalid}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			finds, checks := 0, 0

			repository := &workspacesmock.WorkspaceRepository{
				FindByIDFunc: func(_ context.Context, id workspaces.WorkspaceID) (*workspaces.Workspace, error) {
					var workspace *workspaces.Workspace

					finds++

					if id != tt.workspaceID {
						t.Errorf("FindByID() called with %v", id)
					}

					if tt.findErr == nil {
						workspace = existing
					}

					return workspace, tt.findErr
				},
				HasMemberFunc: func(_ context.Context, membership workspaces.Membership) (bool, error) {
					checks++

					if membership != workspaces.NewMembership(tt.workspaceID, tt.userID) {
						t.Errorf("HasMember() called with %+v", membership)
					}

					return tt.isMember, tt.memberErr
				},
			}

			service, err := NewWorkspaceService(&appmock.Transactor{}, repository, &usersmock.UserRepository{})

			if err != nil {
				t.Fatal(err)
			}

			workspace, err := service.FindForMember(t.Context(), tt.workspaceID, tt.userID)

			if finds != tt.wantFinds || checks != tt.wantChecks {
				t.Fatalf("calls = (find %d, check %d), want (%d, %d)", finds, checks, tt.wantFinds, tt.wantChecks)
			}

			assertErrors(t, err, tt.wantErrs)

			if (workspace == existing) != tt.wantWorkspace || (workspace != nil && !tt.wantWorkspace) {
				t.Fatalf("FindForMember() workspace = %v, want returned %t", workspace, tt.wantWorkspace)
			}
		})
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

func TestWorkspaceServiceRename(t *testing.T) {
	for _, tt := range []struct {
		name        string
		request     input.RenameWorkspaceInput
		lockErr     error
		updateErr   error
		wantName    workspaces.WorkspaceName
		wantErrs    []error
		wantTxCalls int
		wantLocks   int
		wantWrites  int
	}{
		{
			name:        "renamed",
			request:     input.RenameWorkspaceInput{ID: testWorkspaceID, Name: " Research "},
			wantName:    "Research",
			wantTxCalls: 1,
			wantLocks:   1,
			wantWrites:  1,
		},
		{
			name:     "all fields invalid",
			request:  input.RenameWorkspaceInput{Name: "  "},
			wantErrs: []error{input.ErrRenameWorkspaceInputInvalid, workspaces.ErrWorkspaceIDInvalid, workspaces.ErrWorkspaceNameEmpty},
		},
		{
			name:     "long name",
			request:  input.RenameWorkspaceInput{ID: testWorkspaceID, Name: strings.Repeat("a", workspaces.MaxWorkspaceNameLength+1)},
			wantErrs: []error{input.ErrRenameWorkspaceInputInvalid, workspaces.ErrWorkspaceNameTooLong},
		},
		{
			name:        "not found",
			request:     input.RenameWorkspaceInput{ID: testWorkspaceID, Name: "Research"},
			lockErr:     workspaces.ErrWorkspaceNotFound,
			wantErrs:    []error{workspaces.ErrWorkspaceNotFound},
			wantTxCalls: 1,
			wantLocks:   1,
		},
		{
			name:        "update failure",
			request:     input.RenameWorkspaceInput{ID: testWorkspaceID, Name: "Research"},
			updateErr:   context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
			wantLocks:   1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			locks, writes := 0, 0

			var stored *workspaces.Workspace

			repository := &workspacesmock.WorkspaceRepository{
				FindByIDForUpdateFunc: func(ctx context.Context, id workspaces.WorkspaceID) (*workspaces.Workspace, error) {
					var workspace *workspaces.Workspace

					locks++

					if !appmock.InTransaction(ctx) || id != tt.request.ID {
						t.Errorf("FindByIDForUpdate() called with (%v, %v), want a transaction", ctx, id)
					}

					if writes > 0 {
						t.Error("FindByIDForUpdate() called after Update()")
					}

					if tt.lockErr == nil {
						workspace = workspaces.NewWorkspace(id, "Old")
					}

					return workspace, tt.lockErr
				},
				UpdateFunc: func(ctx context.Context, workspace *workspaces.Workspace) workspaces.ErrWorkspaceUpdateFailed {
					var err workspaces.ErrWorkspaceUpdateFailed

					writes++
					stored = workspace

					if !appmock.InTransaction(ctx) {
						t.Error("Update() ran outside a transaction")
					}

					if tt.updateErr != nil {
						err = workspacesmock.NewErrWorkspaceUpdateFailed(workspace, tt.updateErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewWorkspaceService(transactor, repository, &usersmock.UserRepository{})

			if err != nil {
				t.Fatal(err)
			}

			workspace, err := service.Rename(t.Context(), tt.request)

			if transactor.Calls != tt.wantTxCalls || locks != tt.wantLocks || writes != tt.wantWrites {
				t.Fatalf(
					"Rename() calls = (transaction %d, lock %d, write %d), want (%d, %d, %d)",
					transactor.Calls, locks, writes, tt.wantTxCalls, tt.wantLocks, tt.wantWrites,
				)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if workspace != nil {
					t.Fatalf("Rename() workspace = %v, want nil on error", workspace)
				}
			} else if workspace != stored || workspace.ID != tt.request.ID || workspace.Name != tt.wantName {
				t.Fatalf("Rename() workspace = %+v, stored = %+v, want %v named %q", workspace, stored, tt.request.ID, tt.wantName)
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

func TestWorkspaceServiceMembers(t *testing.T) {
	for _, tt := range []struct {
		name        string
		remove      bool
		workspaceID workspaces.WorkspaceID
		userID      users.UserID
		writeErr    error
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{name: "added", workspaceID: testWorkspaceID, userID: testUserID, wantTxCalls: 1, wantWrites: 1},
		{
			name:     "add without a workspace",
			userID:   testUserID,
			wantErrs: []error{workspaces.ErrMembershipInvalid, workspaces.ErrWorkspaceIDInvalid},
		},
		{
			name:        "add without a user",
			workspaceID: testWorkspaceID,
			wantErrs:    []error{workspaces.ErrMembershipInvalid, users.ErrUserIDInvalid},
		},
		{
			name:        "already a member",
			workspaceID: testWorkspaceID,
			userID:      testUserID,
			writeErr:    workspaces.ErrMembershipAlreadyExists,
			wantErrs:    []error{workspaces.ErrMembershipAlreadyExists},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "add to a missing workspace",
			workspaceID: testWorkspaceID,
			userID:      testUserID,
			writeErr:    workspaces.ErrWorkspaceNotFound,
			wantErrs:    []error{workspaces.ErrWorkspaceNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "add a missing user",
			workspaceID: testWorkspaceID,
			userID:      testUserID,
			writeErr:    users.ErrUserNotFound,
			wantErrs:    []error{users.ErrUserNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{name: "removed", remove: true, workspaceID: testWorkspaceID, userID: testUserID, wantTxCalls: 1, wantWrites: 1},
		{
			name:        "remove without a user",
			remove:      true,
			workspaceID: testWorkspaceID,
			wantErrs:    []error{workspaces.ErrMembershipInvalid, users.ErrUserIDInvalid},
		},
		{
			name:        "remove a non-member",
			remove:      true,
			workspaceID: testWorkspaceID,
			userID:      testUserID,
			writeErr:    workspaces.ErrMembershipNotFound,
			wantErrs:    []error{workspaces.ErrMembershipNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0
			want := workspaces.NewMembership(tt.workspaceID, tt.userID)
			checkWrite := func(ctx context.Context, membership workspaces.Membership) {
				writes++

				if !appmock.InTransaction(ctx) || membership != want {
					t.Errorf("membership write called with (%v, %+v), want %+v in a transaction", ctx, membership, want)
				}
			}

			repository := &workspacesmock.WorkspaceRepository{
				AddMemberFunc: func(ctx context.Context, membership workspaces.Membership) workspaces.ErrMembershipAddFailed {
					var err workspaces.ErrMembershipAddFailed

					checkWrite(ctx, membership)

					if tt.remove {
						t.Error("AddMember() called while removing")
					}

					if tt.writeErr != nil {
						err = workspacesmock.NewErrMembershipAddFailed(membership, tt.writeErr)
					}

					return err
				},
				RemoveMemberFunc: func(ctx context.Context, membership workspaces.Membership) workspaces.ErrMembershipRemoveFailed {
					var err workspaces.ErrMembershipRemoveFailed

					checkWrite(ctx, membership)

					if !tt.remove {
						t.Error("RemoveMember() called while adding")
					}

					if tt.writeErr != nil {
						err = workspacesmock.NewErrMembershipRemoveFailed(membership, tt.writeErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewWorkspaceService(transactor, repository, &usersmock.UserRepository{})

			if err != nil {
				t.Fatal(err)
			}

			if tt.remove {
				err = service.RemoveMember(t.Context(), tt.workspaceID, tt.userID)
			} else {
				err = service.AddMember(t.Context(), tt.workspaceID, tt.userID)
			}

			if transactor.Calls != tt.wantTxCalls || writes != tt.wantWrites {
				t.Fatalf("calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)
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

func TestWorkspaceServiceAddMemberByEmail(t *testing.T) {
	member := testUser()

	for _, tt := range []struct {
		name        string
		request     input.AddMemberByEmailInput
		findErr     error
		addErr      error
		wantErrs    []error
		wantFinds   int
		wantAdds    int
		wantTxCalls int
	}{
		{
			name:        "added",
			request:     input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "Anna@Example.com"},
			wantFinds:   1,
			wantAdds:    1,
			wantTxCalls: 1,
		},
		{
			name:     "invalid fields",
			request:  input.AddMemberByEmailInput{Email: "Anna <anna@example.com>"},
			wantErrs: []error{input.ErrAddMemberByEmailInputInvalid, workspaces.ErrWorkspaceIDInvalid, users.ErrEmailInvalid},
		},
		{
			name:      "unknown email",
			request:   input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "nobody@example.com"},
			findErr:   users.ErrUserNotFound,
			wantErrs:  []error{users.ErrUserNotFound},
			wantFinds: 1,
		},
		{
			name:        "already a member",
			request:     input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "anna@example.com"},
			addErr:      workspaces.ErrMembershipAlreadyExists,
			wantErrs:    []error{workspaces.ErrMembershipAlreadyExists},
			wantFinds:   1,
			wantAdds:    1,
			wantTxCalls: 1,
		},
		{
			name:        "workspace deleted meanwhile",
			request:     input.AddMemberByEmailInput{WorkspaceID: testWorkspaceID, Email: "anna@example.com"},
			addErr:      workspaces.ErrWorkspaceNotFound,
			wantErrs:    []error{workspaces.ErrWorkspaceNotFound},
			wantFinds:   1,
			wantAdds:    1,
			wantTxCalls: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			finds, adds := 0, 0

			userRepository := &usersmock.UserRepository{
				FindByEmailFunc: func(_ context.Context, email users.Email) (*users.User, error) {
					var user *users.User

					finds++

					if email != users.Email(tt.request.Email) {
						t.Errorf("FindByEmail() called with %q", email)
					}

					if tt.findErr == nil {
						user = member
					}

					return user, tt.findErr
				},
			}

			repository := &workspacesmock.WorkspaceRepository{
				AddMemberFunc: func(ctx context.Context, membership workspaces.Membership) workspaces.ErrMembershipAddFailed {
					var err workspaces.ErrMembershipAddFailed

					adds++

					if !appmock.InTransaction(ctx) || membership != workspaces.NewMembership(tt.request.WorkspaceID, member.ID) {
						t.Errorf("AddMember() called with (%v, %+v), want the found user in a transaction", ctx, membership)
					}

					if tt.addErr != nil {
						err = workspacesmock.NewErrMembershipAddFailed(membership, tt.addErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewWorkspaceService(transactor, repository, userRepository)

			if err != nil {
				t.Fatal(err)
			}

			user, err := service.AddMemberByEmail(t.Context(), tt.request)

			if finds != tt.wantFinds || adds != tt.wantAdds || transactor.Calls != tt.wantTxCalls {
				t.Fatalf("calls = (find %d, add %d, transaction %d), want (%d, %d, %d)", finds, adds, transactor.Calls, tt.wantFinds, tt.wantAdds, tt.wantTxCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if (user == member) != (len(tt.wantErrs) == 0) || (user != nil && len(tt.wantErrs) > 0) {
				t.Fatalf("AddMemberByEmail() user = %v, want the member only on success", user)
			}
		})
	}
}

func TestWorkspaceServiceListMembers(t *testing.T) {
	member := testUser()
	storageErr := errors.New("storage unavailable")

	for _, tt := range []struct {
		name      string
		id        workspaces.WorkspaceID
		idsErr    error
		usersErr  error
		wantErrs  []error
		wantCalls int
		wantCount int
	}{
		{name: "listed", id: testWorkspaceID, wantCalls: 2, wantCount: 1},
		{name: "invalid ID", wantErrs: []error{workspaces.ErrWorkspaceIDInvalid}},
		{name: "membership failure", id: testWorkspaceID, idsErr: storageErr, wantErrs: []error{storageErr}, wantCalls: 1},
		{name: "user failure", id: testWorkspaceID, usersErr: storageErr, wantErrs: []error{storageErr}, wantCalls: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0

			repository := &workspacesmock.WorkspaceRepository{
				FindMemberIDsFunc: func(_ context.Context, id workspaces.WorkspaceID) ([]users.UserID, error) {
					calls++

					if id != tt.id {
						t.Errorf("FindMemberIDs() called with %v", id)
					}

					return []users.UserID{member.ID}, tt.idsErr
				},
			}

			userRepository := &usersmock.UserRepository{
				FindByIDsFunc: func(_ context.Context, ids []users.UserID) ([]*users.User, error) {
					calls++

					if len(ids) != 1 || ids[0] != member.ID {
						t.Errorf("FindByIDs() called with %v", ids)
					}

					return []*users.User{member}, tt.usersErr
				},
			}

			service, err := NewWorkspaceService(&appmock.Transactor{}, repository, userRepository)

			if err != nil {
				t.Fatal(err)
			}

			members, err := service.ListMembers(t.Context(), tt.id)

			if calls != tt.wantCalls {
				t.Fatalf("repository calls = %d, want %d", calls, tt.wantCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(members) != tt.wantCount || (tt.wantCount > 0 && members[0] != member) {
				t.Fatalf("ListMembers() = %v, want %d members", members, tt.wantCount)
			}
		})
	}
}
