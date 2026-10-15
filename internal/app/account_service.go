package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/accounts"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Account list
//

type AccountList struct {
	Accounts []*accounts.Account
	Total    int
}

//
// Account service
//

// Every method acts for the member in the context: the caller's
// workspaces.Identity must be for the Account's Workspace, and its Role must
// allow the operation: viewing content to read, editing content to write. A
// caller without the identity is reported as ErrUnauthenticated. A User who is
// not a member cannot tell the Workspace or its Accounts from missing ones, so
// they are reported as workspaces.ErrWorkspaceNotFound or
// accounts.ErrAccountNotFound.
//
// An Account's User must be a member of the Account's Workspace when it is
// assigned; any other User is reported as users.ErrUserNotFound, so a member
// learns nothing about Users outside the Workspace. An Account keeps its User
// after that User leaves the Workspace.
//
// The identity's Role was read before the operation began. Every write runs in
// a transaction, locks the Workspace, and reads the caller's Role again under
// that lock, so a Role change and a write that relies on it apply one after
// another.

var ErrAccountRepositoryNil = errors.New("account repository is nil")

type AccountService struct {
	transactor Transactor
	repository accounts.AccountRepository
	workspaces workspaces.WorkspaceRepository
}

func NewAccountService(
	transactor Transactor,
	repository accounts.AccountRepository,
	workspaceRepository workspaces.WorkspaceRepository,
) (*AccountService, error) {
	var (
		service *AccountService
		err     error
	)

	if transactor == nil {
		err = ErrTransactorNil
	} else if repository == nil {
		err = ErrAccountRepositoryNil
	} else if workspaceRepository == nil {
		err = ErrWorkspaceRepositoryNil
	} else {
		service = &AccountService{transactor: transactor, repository: repository, workspaces: workspaceRepository}
	}

	return service, err
}

// Create

func (s *AccountService) Create(
	ctx context.Context,
	request input.CreateAccountInput,
) (*accounts.Account, error) {
	var (
		account  *accounts.Account
		identity *workspaces.Identity
		err      error
	)

	if err = s.check(); err == nil {
		name, valuesErr := accountValues(request.UserID, request.Name)

		err = errors.Join(request.WorkspaceID.Validate(), valuesErr)

		if err != nil {
			err = fmt.Errorf("create account: %w: %w", input.ErrCreateAccountInputInvalid, err)
		} else {
			var (
				id        uuid.UUID
				accountID accounts.ID
			)

			id, err = uuid.NewV7()

			if err == nil {
				accountID, err = accounts.NewID(id)
			}

			if err != nil {
				err = fmt.Errorf("generate account ID: %w", err)
			} else {
				account = accounts.NewAccount(accountID, request.WorkspaceID, request.UserID, name)
			}
		}
	}

	if err == nil {
		identity, err = authorizeMember(ctx, account.WorkspaceID, workspaces.ActionEditContent, workspaces.ErrWorkspaceNotFound)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				_, err := reauthorizeMember(ctx, s.workspaces, identity, workspaces.ActionEditContent, workspaces.ErrWorkspaceNotFound)

				if err == nil && account.UserID != nil {
					err = s.checkUser(ctx, account.WorkspaceID, *account.UserID)
				}

				if err == nil {
					if createErr := s.repository.Create(ctx, account); createErr != nil {
						err = createErr
					}
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf(
				"create account %s in workspace %s: %w",
				uuid.UUID(account.ID),
				uuid.UUID(request.WorkspaceID),
				err,
			)
		}
	}

	if err != nil {
		account = nil
	}

	return account, err
}

// The caller's identity is checked before the Account is read, so a caller
// without one learns nothing about it.

func (s *AccountService) FindByID(
	ctx context.Context,
	id accounts.ID,
) (*accounts.Account, error) {
	var (
		account *accounts.Account
		err     error
	)

	if err = s.check(); err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("find account: %w", err)
		}
	}

	if err == nil {
		_, err = currentMember(ctx)

		if err == nil {
			account, err = s.repository.FindByID(ctx, id)
		}

		if err == nil {
			_, err = authorizeMember(ctx, account.WorkspaceID, workspaces.ActionViewContent, accounts.ErrAccountNotFound)
		}

		if err != nil {
			err = fmt.Errorf("find account %s: %w", uuid.UUID(id), err)
			account = nil
		}
	}

	return account, err
}

// The Workspace criterion of the query is replaced by the given Workspace, so
// a member lists only that Workspace's Accounts.

func (s *AccountService) List(
	ctx context.Context,
	workspaceID workspaces.ID,
	query accounts.AccountQuery,
) (AccountList, error) {
	var (
		list AccountList
		err  error
	)

	if err = s.check(); err == nil {
		query.Filter.WorkspaceID = &workspaceID
		err = query.Validate()

		if err != nil {
			err = fmt.Errorf("list accounts: %w", err)
		}
	}

	if err == nil {
		_, err = authorizeMember(ctx, workspaceID, workspaces.ActionViewContent, workspaces.ErrWorkspaceNotFound)

		if err == nil {
			list.Total, err = s.repository.Count(ctx, query.Filter)
		}

		if err == nil {
			list.Accounts, err = s.repository.Find(ctx, query)
		}

		if err != nil {
			err = fmt.Errorf("list accounts of workspace %s: %w", uuid.UUID(workspaceID), err)
			list = AccountList{}
		}
	}

	return list, err
}

// The Account keeps its Workspace; only its User and name change. A User who
// is kept need not be a member any more.

func (s *AccountService) Update(
	ctx context.Context,
	request input.UpdateAccountInput,
) (*accounts.Account, error) {
	var (
		account  *accounts.Account
		identity *workspaces.Identity
		name     accounts.Name
		err      error
	)

	if err = s.check(); err == nil {
		var valuesErr error

		name, valuesErr = accountValues(request.UserID, request.Name)

		err = errors.Join(request.ID.Validate(), valuesErr)

		if err != nil {
			err = fmt.Errorf("update account: %w: %w", input.ErrUpdateAccountInputInvalid, err)
		}
	}

	if err == nil {
		identity, err = s.authorizeEdit(ctx)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				stored, err := s.findForUpdate(ctx, identity, request.ID)

				if err == nil && request.UserID != nil && (stored.UserID == nil || *stored.UserID != *request.UserID) {
					err = s.checkUser(ctx, stored.WorkspaceID, *request.UserID)
				}

				if err == nil {
					account = accounts.NewAccount(stored.ID, stored.WorkspaceID, request.UserID, name)

					if updateErr := s.repository.Update(ctx, account); updateErr != nil {
						err = updateErr
					}
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf("update account %s: %w", uuid.UUID(request.ID), err)
		}
	}

	if err != nil {
		account = nil
	}

	return account, err
}

// Delete

func (s *AccountService) Delete(ctx context.Context, id accounts.ID) error {
	var identity *workspaces.Identity

	err := s.check()

	if err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("delete account: %w", err)
		}
	}

	if err == nil {
		identity, err = s.authorizeEdit(ctx)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				_, err := s.findForUpdate(ctx, identity, id)

				if err == nil {
					if deleteErr := s.repository.Delete(ctx, id); deleteErr != nil {
						err = deleteErr
					}
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf("delete account %s: %w", uuid.UUID(id), err)
		}
	}

	return err
}

//
// Helpers
//

func (s *AccountService) check() error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrAccountRepositoryNil
	} else if s.transactor == nil {
		err = ErrTransactorNil
	} else if s.workspaces == nil {
		err = ErrWorkspaceRepositoryNil
	}

	return err
}

// The Account's Workspace is learned only inside the transaction, so the
// caller's identity is checked first for the Workspace it belongs to.

func (s *AccountService) authorizeEdit(ctx context.Context) (*workspaces.Identity, error) {
	identity, err := currentMember(ctx)

	if err == nil {
		identity, err = authorizeMember(ctx, identity.WorkspaceID, workspaces.ActionEditContent, accounts.ErrAccountNotFound)
	}

	return identity, err
}

// The Account is read only to learn its Workspace, which never changes, so
// locking the Workspace afterwards keeps the lock order of a Workspace
// deletion, which locks the Workspace before its Accounts. An Account of
// another Workspace than the identity's is reported as ErrAccountNotFound. It
// must run in a transaction.

func (s *AccountService) findForUpdate(
	ctx context.Context,
	identity *workspaces.Identity,
	id accounts.ID,
) (*accounts.Account, error) {
	account, err := s.repository.FindByID(ctx, id)

	if err == nil && account.WorkspaceID != identity.WorkspaceID {
		err = accounts.ErrAccountNotFound
	}

	if err == nil {
		_, err = reauthorizeMember(ctx, s.workspaces, identity, workspaces.ActionEditContent, accounts.ErrAccountNotFound)
	}

	if err != nil {
		account = nil
	}

	return account, err
}

// A User who is not a member of the Workspace is reported as
// users.ErrUserNotFound. It must run after the Workspace was locked, so the
// membership cannot end before the Account is written.

func (s *AccountService) checkUser(
	ctx context.Context,
	workspaceID workspaces.ID,
	userID users.UserID,
) error {
	_, err := s.workspaces.FindMember(ctx, workspaceID, userID)

	if errors.Is(err, workspaces.ErrMembershipNotFound) {
		err = fmt.Errorf("%w: %s is not a member", users.ErrUserNotFound, uuid.UUID(userID))
	}

	return err
}

// Build the values a create or update request supplies. Every invalid value is
// reported.

func accountValues(userID *users.UserID, rawName string) (accounts.Name, error) {
	var userIDErr error

	if userID != nil {
		userIDErr = userID.Validate()
	}

	name, nameErr := accounts.NewName(strings.TrimSpace(rawName))

	return name, errors.Join(userIDErr, nameErr)
}
