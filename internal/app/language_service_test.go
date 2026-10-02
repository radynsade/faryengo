package app

import (
	"context"
	"errors"
	"testing"

	appinput "github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
)

type fakeLanguageRepository struct {
	context           context.Context
	language          *languages.Language
	err               error
	calls             int
	updateCalls       int
	deleteCode        languages.LanguageCode
	deleteErr         error
	deleteCalls       int
	fallback          *languages.Language
	findFallbackErr   error
	findFallbackCalls int
}

func (f *fakeLanguageRepository) Create(ctx context.Context, language *languages.Language) error {
	f.context = ctx
	f.language = language
	f.calls++
	return f.err
}

func (f *fakeLanguageRepository) Update(context.Context, *languages.Language) error {
	f.updateCalls++
	return nil
}

func (f *fakeLanguageRepository) Delete(ctx context.Context, code languages.LanguageCode) error {
	f.context = ctx
	f.deleteCode = code
	f.deleteCalls++
	return f.deleteErr
}

func (f *fakeLanguageRepository) FindByCode(context.Context, languages.LanguageCode) (*languages.Language, error) {
	return nil, languages.ErrLanguageNotFound
}

func (f *fakeLanguageRepository) FindFallback(ctx context.Context) (*languages.Language, error) {
	f.context = ctx
	f.findFallbackCalls++
	var err error

	if f.findFallbackErr != nil {
		err = f.findFallbackErr
	} else if f.fallback == nil {
		err = languages.ErrLanguageNotFound
	}

	return f.fallback, err
}

func TestLanguageServiceCreateLanguage(t *testing.T) {
	ctx := context.Background()
	valid := appinput.CreateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: "Latviešu"}

	for _, tt := range []struct {
		name      string
		input     appinput.CreateLanguageInput
		createErr error
		wantErr   error
		wantCalls int
	}{
		{name: "created", input: valid, wantCalls: 1},
		{name: "invalid code", input: appinput.CreateLanguageInput{Code: "LV", EnglishName: "Latvian", NativeName: "Latviešu"}, wantErr: languages.ErrInvalidLanguageCode},
		{name: "invalid English name", input: appinput.CreateLanguageInput{Code: "lv", NativeName: "Latviešu"}, wantErr: languages.ErrInvalidLanguageEnglishName},
		{name: "invalid native name", input: appinput.CreateLanguageInput{Code: "lv", EnglishName: "Latvian"}, wantErr: languages.ErrInvalidLanguageNativeName},
		{name: "duplicate", input: valid, createErr: languages.ErrLanguageAlreadyExists, wantErr: languages.ErrLanguageAlreadyExists, wantCalls: 1},
		{name: "repository error", input: valid, createErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantCalls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repository := &fakeLanguageRepository{err: tt.createErr}
			service, err := NewLanguageService(repository)
			if err != nil {
				t.Fatalf("NewLanguageService() error = %v", err)
			}

			language, err := service.Create(ctx, tt.input)
			if !errors.Is(err, tt.wantErr) || repository.calls != tt.wantCalls || repository.updateCalls != 0 {
				t.Fatalf("CreateLanguage() = (%v, %v), calls = %d; want error %v and %d calls", language, err, repository.calls, tt.wantErr, tt.wantCalls)
			}

			if tt.wantErr != nil && tt.wantCalls == 0 && !errors.Is(err, appinput.ErrInvalidCreateLanguageInput) {
				t.Fatalf("CreateLanguage() error = %v, want ErrInvalidCreateLanguageInput", err)
			}

			if tt.wantCalls == 1 && (repository.context != ctx || repository.language.Code() != languages.LanguageCode(tt.input.Code) || repository.language.EnglishName() != languages.LanguageEnglishName(tt.input.EnglishName) || repository.language.NativeName() != languages.LanguageNativeName(tt.input.NativeName)) {
				t.Fatalf("CreateLanguage() forwarded context %v and language %v", repository.context, repository.language)
			}

			if tt.wantErr == nil {
				if language != repository.language {
					t.Fatalf("CreateLanguage() returned %v, want created language %v", language, repository.language)
				}
			} else if language != nil {
				t.Fatalf("CreateLanguage() returned %v on error, want nil", language)
			}
		})
	}
}

func TestLanguageServiceRejectsNilRepository(t *testing.T) {
	service, err := NewLanguageService(nil)
	if service != nil || !errors.Is(err, ErrNilLanguageRepository) {
		t.Fatalf("NewLanguageService(nil) = (%v, %v), want nil and ErrNilLanguageRepository", service, err)
	}

	var nilService *LanguageService
	language, err := nilService.Create(context.Background(), appinput.CreateLanguageInput{})
	if language != nil || !errors.Is(err, ErrNilLanguageRepository) {
		t.Fatalf("CreateLanguage(nil receiver) = (%v, %v), want nil and ErrNilLanguageRepository", language, err)
	}
}

func TestLanguageServiceDeleteLanguage(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name      string
		code      string
		deleteErr error
		wantErr   error
		wantCalls int
	}{
		{name: "deleted", code: "lv", wantCalls: 1},
		{name: "invalid code", code: "LV", wantErr: languages.ErrInvalidLanguageCode},
		{name: "missing code", wantErr: languages.ErrInvalidLanguageCode},
		{name: "not found", code: "lv", deleteErr: languages.ErrLanguageNotFound, wantErr: languages.ErrLanguageNotFound, wantCalls: 1},
		{name: "repository error", code: "lv", deleteErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantCalls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repository := &fakeLanguageRepository{deleteErr: tt.deleteErr}
			service, err := NewLanguageService(repository)
			if err != nil {
				t.Fatalf("NewLanguageService() error = %v", err)
			}

			err = service.Delete(ctx, tt.code)
			if !errors.Is(err, tt.wantErr) || repository.deleteCalls != tt.wantCalls || repository.calls != 0 || repository.updateCalls != 0 {
				t.Fatalf("DeleteLanguage() error = %v, calls = %d, want %v and %d calls", err, repository.deleteCalls, tt.wantErr, tt.wantCalls)
			}

			if tt.wantCalls == 1 && (repository.context != ctx || repository.deleteCode != languages.LanguageCode(tt.code)) {
				t.Fatalf("DeleteLanguage() context = %v, code = %q", repository.context, repository.deleteCode)
			}
		})
	}

	var nilService *LanguageService
	if err := nilService.Delete(ctx, "lv"); !errors.Is(err, ErrNilLanguageRepository) {
		t.Fatalf("DeleteLanguage(nil receiver) error = %v, want ErrNilLanguageRepository", err)
	}
}

func TestLanguageServiceCreateFallback(t *testing.T) {
	ctx := context.Background()
	existing, err := languages.NewLanguage("en", "English", "English", true)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name      string
		fallback  bool
		existing  *languages.Language
		findErr   error
		createErr error
		wantErr   error
		wantFind  int
		wantWrite int
	}{
		{name: "ordinary language", existing: existing, wantWrite: 1},
		{name: "first fallback", fallback: true, wantFind: 1, wantWrite: 1},
		{name: "second fallback", fallback: true, existing: existing, wantErr: languages.ErrFallbackLanguageAlreadyExists, wantFind: 1},
		{name: "lookup failure", fallback: true, findErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantFind: 1},
		{name: "concurrent fallback creation", fallback: true, createErr: languages.ErrFallbackLanguageAlreadyExists, wantErr: languages.ErrFallbackLanguageAlreadyExists, wantFind: 1, wantWrite: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repository := &fakeLanguageRepository{fallback: tt.existing, findFallbackErr: tt.findErr, err: tt.createErr}
			service, err := NewLanguageService(repository)
			if err != nil {
				t.Fatal(err)
			}

			language, err := service.Create(ctx, appinput.CreateLanguageInput{
				Code: "lv", EnglishName: "Latvian", NativeName: "Latviešu", IsFallback: tt.fallback,
			})
			if !errors.Is(err, tt.wantErr) || repository.findFallbackCalls != tt.wantFind || repository.calls != tt.wantWrite || repository.context != ctx {
				t.Fatalf("Create() = (%v, %v), repository = %+v", language, err, repository)
			}

			if tt.wantErr == nil {
				if language == nil || language != repository.language || language.IsFallback() != tt.fallback {
					t.Fatalf("Create() language = %v, want fallback %v", language, tt.fallback)
				}
			} else if language != nil {
				t.Fatalf("Create() language = %v, want nil on error", language)
			}
		})
	}
}
