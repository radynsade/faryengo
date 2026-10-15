package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/budgets"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/period"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Budget list
//

type BudgetList struct {
	Budgets []*budgets.Budget
	Total   int
}

//
// Budget service
//

// Every method acts for the member in the context: the caller's
// workspaces.Identity must be for the Budget's Workspace, and its Role must
// allow the operation: viewing content to read, editing content to write. A
// caller without the identity is reported as ErrUnauthenticated. A User who is
// not a member cannot tell the Workspace or its Budgets from missing ones, so
// they are reported as workspaces.ErrWorkspaceNotFound or
// budgets.ErrBudgetNotFound.
//
// A Budget covers Categories of its own Workspace; a Category that is missing
// or belongs to another Workspace is reported as categories.ErrCategoryNotFound,
// so a member learns nothing about other Workspaces. The Currency must exist;
// a missing one is reported as currencies.ErrCurrencyNotFound. The repository
// enforces both when it writes.
//
// The identity's Role was read before the operation began. Every write runs in
// a transaction, locks the Workspace, and reads the caller's Role again under
// that lock, so a Role change and a write that relies on it apply one after
// another.

var ErrBudgetRepositoryNil = errors.New("budget repository is nil")

type BudgetService struct {
	transactor Transactor
	repository budgets.BudgetRepository
	workspaces workspaces.WorkspaceRepository
}

func NewBudgetService(
	transactor Transactor,
	repository budgets.BudgetRepository,
	workspaceRepository workspaces.WorkspaceRepository,
) (*BudgetService, error) {
	var (
		service *BudgetService
		err     error
	)

	if transactor == nil {
		err = ErrTransactorNil
	} else if repository == nil {
		err = ErrBudgetRepositoryNil
	} else if workspaceRepository == nil {
		err = ErrWorkspaceRepositoryNil
	} else {
		service = &BudgetService{transactor: transactor, repository: repository, workspaces: workspaceRepository}
	}

	return service, err
}

// Create

func (s *BudgetService) Create(
	ctx context.Context,
	request input.CreateBudgetInput,
) (*budgets.Budget, error) {
	var (
		budget   *budgets.Budget
		identity *workspaces.Identity
		err      error
	)

	if err = s.check(); err == nil {
		categoryIDs, currencyCode, budgetPeriod, amount, valuesErr := budgetValues(
			request.CategoryIDs,
			request.CurrencyCode,
			request.Period,
			request.Amount,
		)

		err = errors.Join(request.WorkspaceID.Validate(), valuesErr)

		if err != nil {
			err = fmt.Errorf("create budget: %w: %w", input.ErrCreateBudgetInputInvalid, err)
		} else {
			var (
				id       uuid.UUID
				budgetID budgets.ID
			)

			id, err = uuid.NewV7()

			if err == nil {
				budgetID, err = budgets.NewID(id)
			}

			if err != nil {
				err = fmt.Errorf("generate budget ID: %w", err)
			} else {
				budget = budgets.NewBudget(budgetID, request.WorkspaceID, categoryIDs, currencyCode, budgetPeriod, amount)
			}
		}
	}

	if err == nil {
		identity, err = authorizeMember(ctx, budget.WorkspaceID, workspaces.ActionEditContent, workspaces.ErrWorkspaceNotFound)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				_, err := reauthorizeMember(ctx, s.workspaces, identity, workspaces.ActionEditContent, workspaces.ErrWorkspaceNotFound)

				if err == nil {
					if createErr := s.repository.Create(ctx, budget); createErr != nil {
						err = createErr
					}
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf(
				"create budget %s in workspace %s: %w",
				uuid.UUID(budget.ID),
				uuid.UUID(request.WorkspaceID),
				err,
			)
		}
	}

	if err != nil {
		budget = nil
	}

	return budget, err
}

// The caller's identity is checked before the Budget is read, so a caller
// without one learns nothing about it.

func (s *BudgetService) FindByID(
	ctx context.Context,
	id budgets.ID,
) (*budgets.Budget, error) {
	var (
		budget *budgets.Budget
		err    error
	)

	if err = s.check(); err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("find budget: %w", err)
		}
	}

	if err == nil {
		_, err = currentMember(ctx)

		if err == nil {
			budget, err = s.repository.FindByID(ctx, id)
		}

		if err == nil {
			_, err = authorizeMember(ctx, budget.WorkspaceID, workspaces.ActionViewContent, budgets.ErrBudgetNotFound)
		}

		if err != nil {
			err = fmt.Errorf("find budget %s: %w", uuid.UUID(id), err)
			budget = nil
		}
	}

	return budget, err
}

// The Workspace criterion of the query is replaced by the given Workspace, so
// a member lists only that Workspace's Budgets.

func (s *BudgetService) List(
	ctx context.Context,
	workspaceID workspaces.ID,
	query budgets.BudgetQuery,
) (BudgetList, error) {
	var (
		list BudgetList
		err  error
	)

	if err = s.check(); err == nil {
		query.Filter.WorkspaceID = &workspaceID
		err = query.Validate()

		if err != nil {
			err = fmt.Errorf("list budgets: %w", err)
		}
	}

	if err == nil {
		_, err = authorizeMember(ctx, workspaceID, workspaces.ActionViewContent, workspaces.ErrWorkspaceNotFound)

		if err == nil {
			list.Total, err = s.repository.Count(ctx, query.Filter)
		}

		if err == nil {
			list.Budgets, err = s.repository.Find(ctx, query)
		}

		if err != nil {
			err = fmt.Errorf("list budgets of workspace %s: %w", uuid.UUID(workspaceID), err)
			list = BudgetList{}
		}
	}

	return list, err
}

// The Budget keeps its Workspace; its Categories, Currency, Period, and Amount
// are replaced.

func (s *BudgetService) Update(
	ctx context.Context,
	request input.UpdateBudgetInput,
) (*budgets.Budget, error) {
	var (
		budget       *budgets.Budget
		identity     *workspaces.Identity
		categoryIDs  budgets.CategoryIDs
		currencyCode currencies.Code
		budgetPeriod period.Period
		amount       budgets.Amount
		err          error
	)

	if err = s.check(); err == nil {
		var valuesErr error

		categoryIDs, currencyCode, budgetPeriod, amount, valuesErr = budgetValues(
			request.CategoryIDs,
			request.CurrencyCode,
			request.Period,
			request.Amount,
		)

		err = errors.Join(request.ID.Validate(), valuesErr)

		if err != nil {
			err = fmt.Errorf("update budget: %w: %w", input.ErrUpdateBudgetInputInvalid, err)
		}
	}

	if err == nil {
		identity, err = s.authorizeEdit(ctx)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				stored, err := s.findForUpdate(ctx, identity, request.ID)

				if err == nil {
					budget = budgets.NewBudget(stored.ID, stored.WorkspaceID, categoryIDs, currencyCode, budgetPeriod, amount)

					if updateErr := s.repository.Update(ctx, budget); updateErr != nil {
						err = updateErr
					}
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf("update budget %s: %w", uuid.UUID(request.ID), err)
		}
	}

	if err != nil {
		budget = nil
	}

	return budget, err
}

// Delete

func (s *BudgetService) Delete(ctx context.Context, id budgets.ID) error {
	var identity *workspaces.Identity

	err := s.check()

	if err == nil {
		err = id.Validate()

		if err != nil {
			err = fmt.Errorf("delete budget: %w", err)
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
			err = fmt.Errorf("delete budget %s: %w", uuid.UUID(id), err)
		}
	}

	return err
}

//
// Helpers
//

func (s *BudgetService) check() error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrBudgetRepositoryNil
	} else if s.transactor == nil {
		err = ErrTransactorNil
	} else if s.workspaces == nil {
		err = ErrWorkspaceRepositoryNil
	}

	return err
}

// The Budget's Workspace is learned only inside the transaction, so the
// caller's identity is checked first for the Workspace it belongs to.

func (s *BudgetService) authorizeEdit(ctx context.Context) (*workspaces.Identity, error) {
	identity, err := currentMember(ctx)

	if err == nil {
		identity, err = authorizeMember(ctx, identity.WorkspaceID, workspaces.ActionEditContent, budgets.ErrBudgetNotFound)
	}

	return identity, err
}

// The Budget is read only to learn its Workspace, which never changes, so
// locking the Workspace afterwards keeps the lock order of a Workspace
// deletion, which locks the Workspace before its Budgets. A Budget of another
// Workspace than the identity's is reported as ErrBudgetNotFound. It must run
// in a transaction.

func (s *BudgetService) findForUpdate(
	ctx context.Context,
	identity *workspaces.Identity,
	id budgets.ID,
) (*budgets.Budget, error) {
	budget, err := s.repository.FindByID(ctx, id)

	if err == nil && budget.WorkspaceID != identity.WorkspaceID {
		err = budgets.ErrBudgetNotFound
	}

	if err == nil {
		_, err = reauthorizeMember(ctx, s.workspaces, identity, workspaces.ActionEditContent, budgets.ErrBudgetNotFound)
	}

	if err != nil {
		budget = nil
	}

	return budget, err
}

// Build the values a create or update request supplies. Every invalid value is
// reported. The Categories and the Period's times are copied, so the caller
// cannot change the Budget afterwards.

func budgetValues(
	rawCategoryIDs []categories.ID,
	rawCurrencyCode string,
	rawPeriod input.PeriodInput,
	rawAmount int64,
) (budgets.CategoryIDs, currencies.Code, period.Period, budgets.Amount, error) {
	categoryIDs := budgets.NewCategoryIDs(slices.Clone(rawCategoryIDs))
	currencyCode, currencyErr := currencies.NewCode(rawCurrencyCode)
	budgetPeriod, periodErr := periodValue(rawPeriod)
	amount, amountErr := budgets.NewAmount(rawAmount)

	return categoryIDs, currencyCode, budgetPeriod, amount, errors.Join(categoryIDs.Validate(), currencyErr, periodErr, amountErr)
}

func periodValue(raw input.PeriodInput) (period.Period, error) {
	kind, err := period.NewRepeatKind(raw.Repeat)
	value := period.NewPeriod(
		copyTime(raw.From),
		copyTime(raw.To),
		period.NewRepeat(kind, raw.Every, copyTime(raw.Until)),
	)

	if err == nil {
		err = value.Validate()
	}

	return value, err
}

func copyTime(value *time.Time) *time.Time {
	var copied *time.Time

	if value != nil {
		copied = new(*value)
	}

	return copied
}
