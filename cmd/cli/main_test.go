package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/app"
	appinput "github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestParseCreateLanguage(t *testing.T) {
	for _, tt := range []struct {
		name         string
		args         []string
		wantCode     string
		wantEN       string
		wantName     string
		wantFallback bool
		wantError    error
	}{
		{name: "language", args: []string{"languages", "create-language", "lv", "Latvian", "Latviešu"}, wantCode: "lv", wantEN: "Latvian", wantName: "Latviešu"},
		{name: "fallback", args: []string{"languages", "create-language", "en", "English", "English", "--fallback"}, wantCode: "en", wantEN: "English", wantName: "English", wantFallback: true},
		{name: "fallback shorthand", args: []string{"languages", "create-language", "-f", "en", "English", "English"}, wantCode: "en", wantEN: "English", wantName: "English", wantFallback: true},
		{name: "fallback between arguments", args: []string{"languages", "create-language", "en", "--fallback", "English", "English"}, wantCode: "en", wantEN: "English", wantName: "English", wantFallback: true},
		{name: "options separator", args: []string{"languages", "create-language", "--", "en", "English", "English"}, wantCode: "en", wantEN: "English", wantName: "English"},
		{name: "unknown flag", args: []string{"languages", "create-language", "en", "English", "English", "--unknown"}, wantError: errInvalidCommand},
		{name: "flag without arguments", args: []string{"languages", "create-language", "-f"}, wantError: errInvalidCommand},
		{name: "quoted names", args: []string{"languages", "create-language", "zh", "Chinese (Simplified)", "简体中文"}, wantCode: "zh", wantEN: "Chinese (Simplified)", wantName: "简体中文"},
		{name: "missing command", wantError: errInvalidCommand},
		{name: "wrong group", args: []string{"language", "create-language", "lv", "Latvian", "Latviešu"}, wantError: errInvalidCommand},
		{name: "wrong operation", args: []string{"languages", "create", "lv", "Latvian", "Latviešu"}, wantError: errInvalidCommand},
		{name: "missing native name", args: []string{"languages", "create-language", "lv", "Latvian"}, wantError: errInvalidCommand},
		{name: "invalid code", args: []string{"languages", "create-language", "LV", "Latvian", "Latviešu"}, wantError: languages.ErrInvalidLanguageCode},
		{name: "extra argument", args: []string{"languages", "create-language", "lv", "Latvian", "Latviešu", "extra"}, wantError: errInvalidCommand},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input, err := parseCreateLanguage(tt.args)
			if !errors.Is(err, tt.wantError) || input.Code != tt.wantCode || input.EnglishName != tt.wantEN || input.NativeName != tt.wantName || input.IsFallback != tt.wantFallback {
				t.Fatalf("parseCreateLanguage(%v) = (%+v, %v), want (%q, %q, %q, %v)", tt.args, input, err, tt.wantCode, tt.wantEN, tt.wantName, tt.wantError)
			}

			if errors.Is(tt.wantError, languages.ErrInvalidLanguageCode) && !errors.Is(err, appinput.ErrInvalidCreateLanguageInput) {
				t.Fatalf("parseCreateLanguage() error = %v, want ErrInvalidCreateLanguageInput", err)
			}
		})
	}
}

func TestRunWithoutDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	for _, tt := range []struct {
		name       string
		args       []string
		wantOutput string
		wantError  string
	}{
		{name: "help", args: []string{"help"}, wantOutput: usage + "\n"},
		{name: "invalid command", args: []string{"languages"}, wantError: "invalid command"},
		{name: "missing connection for create", args: []string{"languages", "create-language", "lv", "Latvian", "Latviešu"}, wantError: "DATABASE_URL"},
		{name: "invalid language before connection", args: []string{"languages", "create-language", "LV", "Latvian", "Latviešu"}, wantError: "invalid language code"},
		{name: "missing connection for delete", args: []string{"languages", "delete-language", "lv"}, wantError: "DATABASE_URL"},
		{name: "delete with missing code", args: []string{"languages", "delete-language"}, wantError: "invalid command"},
		{name: "missing connection for user creation", args: createUserArguments(), wantError: "DATABASE_URL"},
		{name: "missing connection for user deletion", args: []string{"security", "delete-user", testUserUUID}, wantError: "DATABASE_URL"},
		{name: "invalid user before connection", args: changedUserArguments(2, "invalid-email"), wantError: "invalid email"},
		{name: "invalid user UUID before connection", args: []string{"security", "delete-user", "invalid-uuid"}, wantError: "invalid user ID"},
		{name: "missing connection for role creation", args: []string{"security", "create-role", "en:Administrator", "--super"}, wantError: "DATABASE_URL"},
		{name: "missing connection for role deletion", args: []string{"security", "delete-role", testRoleUUID}, wantError: "DATABASE_URL"},
		{name: "malformed role name before connection", args: []string{"security", "create-role", "broken"}, wantError: "invalid string translations"},
		{name: "invalid role code before connection", args: []string{"security", "create-role", "EN:Administrator"}, wantError: "invalid language code"},
		{name: "invalid permission before connection", args: []string{"security", "create-role", "en:Administrator", "-p", "unknown"}, wantError: "invalid permission"},
		{name: "invalid role UUID before connection", args: []string{"security", "delete-role", "invalid-uuid"}, wantError: "invalid role ID"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := run(context.Background(), tt.args, &stdout, &stderr)
			if stdout.String() != tt.wantOutput || (tt.wantError == "") != (err == nil) {
				t.Fatalf("run() = (%q, %v), want output %q and error containing %q", stdout.String(), err, tt.wantOutput, tt.wantError)
			}

			if tt.wantError != "" && !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("run() error = %v, want %q", err, tt.wantError)
			}

			if tt.wantError == "invalid command" && stderr.String() != usage+"\n" {
				t.Fatalf("run() stderr = %q, want usage", stderr.String())
			}
		})
	}
}

func TestParseDeleteLanguage(t *testing.T) {
	for _, tt := range []struct {
		name      string
		args      []string
		wantCode  string
		wantError error
	}{
		{name: "language", args: []string{"languages", "delete-language", "lv"}, wantCode: "lv"},
		{name: "missing code", args: []string{"languages", "delete-language"}, wantError: errInvalidCommand},
		{name: "extra argument", args: []string{"languages", "delete-language", "lv", "extra"}, wantError: errInvalidCommand},
		{name: "wrong group", args: []string{"language", "delete-language", "lv"}, wantError: errInvalidCommand},
		{name: "wrong command", args: []string{"languages", "delete", "lv"}, wantError: errInvalidCommand},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, err := parseDeleteLanguage(tt.args)
			if code != tt.wantCode || !errors.Is(err, tt.wantError) {
				t.Fatalf("parseDeleteLanguage(%v) = (%q, %v), want (%q, %v)", tt.args, code, err, tt.wantCode, tt.wantError)
			}
		})
	}
}

type fakeCLIRepository struct {
	context    context.Context
	deleteCode languages.LanguageCode
	deleteErr  error
	calls      int
	created    *languages.Language
}

func (f *fakeCLIRepository) Create(ctx context.Context, language *languages.Language) error {
	f.context = ctx
	f.created = language
	return nil
}
func (f *fakeCLIRepository) Update(context.Context, *languages.Language) error { return nil }
func (f *fakeCLIRepository) Delete(ctx context.Context, code languages.LanguageCode) error {
	f.context = ctx
	f.deleteCode = code
	f.calls++
	return f.deleteErr
}

func (f *fakeCLIRepository) FindByCode(context.Context, languages.LanguageCode) (*languages.Language, error) {
	return nil, languages.ErrLanguageNotFound
}

func (f *fakeCLIRepository) FindFallback(context.Context) (*languages.Language, error) {
	return nil, languages.ErrLanguageNotFound
}

func TestDeleteLanguageCommand(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name       string
		deleteErr  error
		wantOutput string
		wantErr    error
	}{
		{name: "deleted", wantOutput: "deleted language lv\n"},
		{name: "not found", deleteErr: languages.ErrLanguageNotFound, wantErr: languages.ErrLanguageNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command, err := parseLanguageCommand([]string{"languages", "delete-language", "lv"})
			if err != nil {
				t.Fatalf("parseLanguageCommand() error = %v", err)
			}

			repository := &fakeCLIRepository{deleteErr: tt.deleteErr}
			service, err := app.NewLanguageService(repository)
			if err != nil {
				t.Fatalf("NewLanguageService() error = %v", err)
			}

			var output bytes.Buffer
			err = executeLanguageCommand(ctx, command, service, &output)
			if !errors.Is(err, tt.wantErr) || output.String() != tt.wantOutput || repository.calls != 1 || repository.context != ctx || repository.deleteCode != "lv" {
				t.Fatalf("executeLanguageCommand() = (%q, %v), repository = %+v; want output %q and error %v", output.String(), err, repository, tt.wantOutput, tt.wantErr)
			}
		})
	}
}

func TestCreateFallbackLanguageCommand(t *testing.T) {
	ctx := context.Background()
	command, err := parseLanguageCommand([]string{"languages", "create-language", "en", "English", "English", "--fallback"})
	if err != nil {
		t.Fatal(err)
	}

	repository := &fakeCLIRepository{}
	service, err := app.NewLanguageService(repository)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = executeLanguageCommand(ctx, command, service, &output)
	if err != nil || output.String() != "created language en\n" || repository.context != ctx || repository.created == nil || !repository.created.IsFallback() {
		t.Fatalf("executeLanguageCommand() = (%q, %v), repository = %+v", output.String(), err, repository)
	}
}

func (f *fakeCLIRepository) FindAll(context.Context) ([]*languages.Language, error) {
	return nil, nil
}
