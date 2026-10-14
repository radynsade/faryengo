package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/app/input"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/languages/mock"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestNewLanguageService(t *testing.T) {
	for _, tt := range []struct {
		name       string
		transactor Transactor
		repository languages.LanguageRepository
		wantErr    error
	}{
		{name: "created", transactor: &appmock.Transactor{}, repository: &mock.LanguageRepository{}},
		{name: "nil transactor", repository: &mock.LanguageRepository{}, wantErr: ErrTransactorNil},
		{name: "nil repository", transactor: &appmock.Transactor{}, wantErr: ErrLanguageRepositoryNil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewLanguageService(tt.transactor, tt.repository)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) || (service == nil) != (tt.wantErr != nil) {
				t.Fatalf("NewLanguageService() = (%v, %v), want error %v", service, err, tt.wantErr)
			}
		})
	}
}

func TestLanguageServiceNilReceiver(t *testing.T) {
	ctx := systemContext(t)

	var service *LanguageService

	language, createErr := service.Create(ctx, input.CreateLanguageInput{})
	found, findErr := service.FindByCode(ctx, "lv")
	list, listErr := service.List(ctx)
	updated, updateErr := service.Update(ctx, input.UpdateLanguageInput{})
	deleteErr := service.Delete(ctx, "lv")

	for _, err := range []error{createErr, findErr, listErr, updateErr, deleteErr} {
		if !errors.Is(err, ErrLanguageRepositoryNil) {
			t.Fatalf("nil receiver error = %v, want ErrLanguageRepositoryNil", err)
		}
	}

	if language != nil || found != nil || list != nil || updated != nil {
		t.Fatalf("nil receiver returned values: %v, %v, %v, %v", language, found, list, updated)
	}
}

func TestLanguageServiceCreate(t *testing.T) {
	valid := input.CreateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: "Latviešu"}

	for _, tt := range []struct {
		name         string
		request      input.CreateLanguageInput
		beginErr     error
		createErr    error
		wantErrs     []error
		wantTxCalls  int
		wantWrites   int
		wantFallback bool
	}{
		{name: "created", request: valid, wantTxCalls: 1, wantWrites: 1},
		{
			name:         "created fallback",
			request:      input.CreateLanguageInput{Code: "en", EnglishName: "English", NativeName: "English", IsFallback: true},
			wantTxCalls:  1,
			wantWrites:   1,
			wantFallback: true,
		},
		{
			name:     "invalid code",
			request:  input.CreateLanguageInput{Code: "LV", EnglishName: "Latvian", NativeName: "Latviešu"},
			wantErrs: []error{input.ErrCreateLanguageInputInvalid, languages.ErrCodeInvalid},
		},
		{
			name:     "long native name",
			request:  input.CreateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: strings.Repeat("a", 101)},
			wantErrs: []error{input.ErrCreateLanguageInputInvalid, languages.ErrNativeNameTooLong},
		},
		{
			name:    "all fields invalid",
			request: input.CreateLanguageInput{},
			wantErrs: []error{
				input.ErrCreateLanguageInputInvalid,
				languages.ErrCodeInvalid,
				languages.ErrEnglishNameEmpty,
				languages.ErrNativeNameEmpty,
			},
		},
		{
			name:        "transaction not started",
			request:     valid,
			beginErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
		},
		{
			name:        "duplicate",
			request:     valid,
			createErr:   languages.ErrLanguageAlreadyExists,
			wantErrs:    []error{languages.ErrLanguageAlreadyExists},
			wantTxCalls: 1,
			wantWrites:  1,
		},
		{
			name:        "second fallback",
			request:     input.CreateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: "Latviešu", IsFallback: true},
			createErr:   languages.ErrFallbackLanguageAlreadyExists,
			wantErrs:    []error{languages.ErrFallbackLanguageAlreadyExists},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0

			var stored *languages.Language

			repository := &mock.LanguageRepository{
				CreateFunc: func(
					ctx context.Context,
					language *languages.Language,
				) languages.ErrLanguageCreateFailed {
					var err languages.ErrLanguageCreateFailed

					writes++
					stored = language

					if !appmock.InTransaction(ctx) {
						t.Error("Create() ran outside a transaction")
					}

					if tt.createErr != nil {
						err = mock.NewErrLanguageCreateFailed(language, tt.createErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{BeginErr: tt.beginErr}
			service, err := NewLanguageService(transactor, repository)

			if err != nil {
				t.Fatal(err)
			}

			language, err := service.Create(userContext(t, testUserID, users.PermissionManageLanguage), tt.request)

			if transactor.Calls != tt.wantTxCalls || writes != tt.wantWrites {
				t.Fatalf("Create() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if language != nil {
					t.Fatalf("Create() language = %v, want nil on error", language)
				}
			} else if language == nil || language != stored ||
				language.Code != languages.Code(tt.request.Code) ||
				language.EnglishName != languages.EnglishName(tt.request.EnglishName) ||
				language.NativeName != languages.NativeName(tt.request.NativeName) ||
				language.IsFallback != tt.wantFallback {
				t.Fatalf("Create() language = %+v, stored = %+v, want %+v", language, stored, tt.request)
			}
		})
	}
}

func TestLanguageServiceFindByCode(t *testing.T) {
	latvian := languages.NewLanguage("lv", "Latvian", "Latviešu", false)

	for _, tt := range []struct {
		name      string
		code      string
		findErr   error
		wantErrs  []error
		wantCalls int
	}{
		{name: "found", code: "lv", wantCalls: 1},
		{name: "invalid code", code: "LV", wantErrs: []error{languages.ErrCodeInvalid}},
		{
			name:      "not found",
			code:      "lv",
			findErr:   languages.ErrLanguageNotFound,
			wantErrs:  []error{languages.ErrLanguageNotFound},
			wantCalls: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := userContext(t, testUserID)
			calls := 0

			repository := &mock.LanguageRepository{
				FindByCodeFunc: func(
					callCtx context.Context,
					code languages.Code,
				) (*languages.Language, error) {
					var language *languages.Language

					calls++

					if callCtx != ctx || code != languages.Code(tt.code) {
						t.Errorf("FindByCode() called with (%v, %q)", callCtx, code)
					}

					if tt.findErr == nil {
						language = latvian
					}

					return language, tt.findErr
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewLanguageService(transactor, repository)

			if err != nil {
				t.Fatal(err)
			}

			language, err := service.FindByCode(ctx, tt.code)

			if calls != tt.wantCalls || transactor.Calls != 0 {
				t.Fatalf("FindByCode() calls = (repository %d, transaction %d), want (%d, 0)", calls, transactor.Calls, tt.wantCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && language != nil {
				t.Fatalf("FindByCode() language = %v, want nil on error", language)
			} else if len(tt.wantErrs) == 0 && language != latvian {
				t.Fatalf("FindByCode() language = %v, want %v", language, latvian)
			}
		})
	}
}

func TestLanguageServiceList(t *testing.T) {
	all := []*languages.Language{
		languages.NewLanguage("en", "English", "English", true),
		languages.NewLanguage("lv", "Latvian", "Latviešu", false),
	}

	for _, tt := range []struct {
		name     string
		findErr  error
		wantErrs []error
	}{
		{name: "listed"},
		{name: "repository error", findErr: context.DeadlineExceeded, wantErrs: []error{context.DeadlineExceeded}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := userContext(t, testUserID)

			repository := &mock.LanguageRepository{
				FindAllFunc: func(callCtx context.Context) ([]*languages.Language, error) {
					if callCtx != ctx {
						t.Errorf("FindAll() context = %v, want %v", callCtx, ctx)
					}

					return all, tt.findErr
				},
			}

			service, err := NewLanguageService(&appmock.Transactor{}, repository)

			if err != nil {
				t.Fatal(err)
			}

			result, err := service.List(ctx)

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && result != nil {
				t.Fatalf("List() = %v, want nil on error", result)
			} else if len(tt.wantErrs) == 0 && len(result) != len(all) {
				t.Fatalf("List() = %v, want %v", result, all)
			}
		})
	}
}

func TestLanguageServiceUpdate(t *testing.T) {
	valid := input.UpdateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: "Latviešu", IsFallback: true}

	for _, tt := range []struct {
		name        string
		request     input.UpdateLanguageInput
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
			request:  input.UpdateLanguageInput{Code: "lv", EnglishName: " ", NativeName: ""},
			wantErrs: []error{input.ErrUpdateLanguageInputInvalid, languages.ErrEnglishNameEmpty, languages.ErrNativeNameEmpty},
		},
		{
			name:        "not found",
			request:     valid,
			lockErr:     languages.ErrLanguageNotFound,
			wantErrs:    []error{languages.ErrLanguageNotFound},
			wantTxCalls: 1,
			wantLocks:   1,
		},
		{
			name:        "fallback in use",
			request:     input.UpdateLanguageInput{Code: "en", EnglishName: "English", NativeName: "English"},
			updateErr:   languages.ErrFallbackLanguageAlreadyInUse,
			wantErrs:    []error{languages.ErrFallbackLanguageAlreadyInUse},
			wantTxCalls: 1,
			wantLocks:   1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			locks, writes := 0, 0

			var stored *languages.Language

			repository := &mock.LanguageRepository{
				FindByCodeForUpdateFunc: func(
					ctx context.Context,
					code languages.Code,
				) (*languages.Language, error) {
					var language *languages.Language

					locks++

					if !appmock.InTransaction(ctx) || code != languages.Code(tt.request.Code) {
						t.Errorf("FindByCodeForUpdate() called with (%v, %q), want a transaction", ctx, code)
					}

					if tt.lockErr == nil {
						language = languages.NewLanguage(code, "Old", "Old", false)
					}

					return language, tt.lockErr
				},
				UpdateFunc: func(
					ctx context.Context,
					language *languages.Language,
				) languages.ErrLanguageUpdateFailed {
					var err languages.ErrLanguageUpdateFailed

					writes++
					stored = language

					if !appmock.InTransaction(ctx) {
						t.Error("Update() ran outside a transaction")
					}

					if tt.updateErr != nil {
						err = mock.NewErrLanguageUpdateFailed(language, tt.updateErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewLanguageService(transactor, repository)

			if err != nil {
				t.Fatal(err)
			}

			language, err := service.Update(userContext(t, testUserID, users.PermissionManageLanguage), tt.request)

			if transactor.Calls != tt.wantTxCalls || locks != tt.wantLocks || writes != tt.wantWrites {
				t.Fatalf(
					"Update() calls = (transaction %d, lock %d, write %d), want (%d, %d, %d)",
					transactor.Calls, locks, writes, tt.wantTxCalls, tt.wantLocks, tt.wantWrites,
				)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if language != nil {
					t.Fatalf("Update() language = %v, want nil on error", language)
				}
			} else if language == nil || language != stored ||
				language.Code != languages.Code(tt.request.Code) ||
				language.EnglishName != languages.EnglishName(tt.request.EnglishName) ||
				language.NativeName != languages.NativeName(tt.request.NativeName) ||
				language.IsFallback != tt.request.IsFallback {
				t.Fatalf("Update() language = %+v, stored = %+v, want %+v", language, stored, tt.request)
			}
		})
	}
}

func TestLanguageServiceDelete(t *testing.T) {
	for _, tt := range []struct {
		name        string
		code        string
		deleteErr   error
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{name: "deleted", code: "lv", wantTxCalls: 1, wantWrites: 1},
		{name: "invalid code", code: "LV", wantErrs: []error{languages.ErrCodeInvalid}},
		{name: "missing code", wantErrs: []error{languages.ErrCodeInvalid}},
		{
			name:        "not found",
			code:        "lv",
			deleteErr:   languages.ErrLanguageNotFound,
			wantErrs:    []error{languages.ErrLanguageNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0

			repository := &mock.LanguageRepository{
				DeleteFunc: func(
					ctx context.Context,
					code languages.Code,
				) languages.ErrLanguageDeleteFailed {
					var err languages.ErrLanguageDeleteFailed

					writes++

					if !appmock.InTransaction(ctx) || code != languages.Code(tt.code) {
						t.Errorf("Delete() called with (%v, %q), want a transaction", ctx, code)
					}

					if tt.deleteErr != nil {
						err = mock.NewErrLanguageDeleteFailed(code, tt.deleteErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewLanguageService(transactor, repository)

			if err != nil {
				t.Fatal(err)
			}

			err = service.Delete(userContext(t, testUserID, users.PermissionManageLanguage), tt.code)

			if transactor.Calls != tt.wantTxCalls || writes != tt.wantWrites {
				t.Fatalf("Delete() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)
		})
	}
}

// Managing languages needs its own permission; any identity reads them. Every
// refusal comes before any repository call.

func TestLanguageServiceAccess(t *testing.T) {
	operations := []struct {
		name  string
		write bool
		call  func(ctx context.Context, service *LanguageService) error
	}{
		{"create", true, func(ctx context.Context, service *LanguageService) error {
			_, err := service.Create(ctx, input.CreateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: "Latviešu"})
			return err
		}},
		{"find", false, func(ctx context.Context, service *LanguageService) error {
			_, err := service.FindByCode(ctx, "lv")
			return err
		}},
		{"list", false, func(ctx context.Context, service *LanguageService) error {
			_, err := service.List(ctx)
			return err
		}},
		{"update", true, func(ctx context.Context, service *LanguageService) error {
			_, err := service.Update(ctx, input.UpdateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: "Latviešu"})
			return err
		}},
		{"delete", true, func(ctx context.Context, service *LanguageService) error {
			return service.Delete(ctx, "lv")
		}},
	}

	for _, tt := range []struct {
		name     string
		ctx      context.Context
		wantErrs func(write bool) []error
	}{
		{"missing identity", t.Context(), func(bool) []error { return []error{ErrUnauthenticated, security.ErrIdentityMissing} }},
		{"without the permission", userContext(t, testUserID, users.PermissionManageRole), func(write bool) []error {
			var wantErrs []error

			if write {
				wantErrs = []error{users.ErrPermissionDenied}
			}

			return wantErrs
		}},
	} {
		for _, operation := range operations {
			t.Run(tt.name+" "+operation.name, func(t *testing.T) {
				transactor := &appmock.Transactor{}
				calls := 0
				repository := &mock.LanguageRepository{
					FindByCodeFunc: func(context.Context, languages.Code) (*languages.Language, error) {
						calls++
						return languages.NewLanguage("lv", "Latvian", "Latviešu", false), nil
					},
					FindAllFunc: func(context.Context) ([]*languages.Language, error) {
						calls++
						return nil, nil
					},
				}

				service, err := NewLanguageService(transactor, repository)

				if err != nil {
					t.Fatal(err)
				}

				err = operation.call(tt.ctx, service)
				wantErrs := tt.wantErrs(operation.write)

				if len(wantErrs) == 0 {
					if calls != 1 || err != nil {
						t.Fatalf("%s = %v after %d repository calls, want it allowed", operation.name, err, calls)
					}
				} else if calls != 0 || transactor.Calls != 0 {
					t.Fatalf("%s called the repository %d and the transactor %d times, want none", operation.name, calls, transactor.Calls)
				}

				assertErrors(t, err, wantErrs)
			})
		}
	}
}

func assertErrors(t *testing.T, err error, wantErrs []error) {
	t.Helper()

	if (err == nil) != (len(wantErrs) == 0) {
		t.Fatalf("error = %v, want %v", err, wantErrs)
	}

	for _, wantErr := range wantErrs {
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	}
}
