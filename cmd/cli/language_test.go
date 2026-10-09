package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/languages/mock"
)

func TestParseLanguageCommand(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		want    languageCommand
		wantErr error
	}{
		{
			name: "create",
			args: []string{"languages", "create-language", "lv", "Latvian", "Latviešu"},
			want: languageCommand{
				action:      createLanguageCommand,
				createInput: input.CreateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: "Latviešu"},
			},
		},
		{
			name: "create fallback",
			args: []string{"languages", "create-language", "en", "English", "English", "--fallback"},
			want: languageCommand{
				action:      createLanguageCommand,
				createInput: input.CreateLanguageInput{Code: "en", EnglishName: "English", NativeName: "English", IsFallback: true},
			},
		},
		{
			name: "fallback shorthand before arguments",
			args: []string{"languages", "create-language", "-f", "en", "English", "English"},
			want: languageCommand{
				action:      createLanguageCommand,
				createInput: input.CreateLanguageInput{Code: "en", EnglishName: "English", NativeName: "English", IsFallback: true},
			},
		},
		{
			name: "options separator",
			args: []string{"languages", "create-language", "--", "xx", "-dash", "Name"},
			want: languageCommand{
				action:      createLanguageCommand,
				createInput: input.CreateLanguageInput{Code: "xx", EnglishName: "-dash", NativeName: "Name"},
			},
		},
		{
			name: "names with spaces",
			args: []string{"languages", "create-language", "zh", "Chinese (Simplified)", "简体中文"},
			want: languageCommand{
				action:      createLanguageCommand,
				createInput: input.CreateLanguageInput{Code: "zh", EnglishName: "Chinese (Simplified)", NativeName: "简体中文"},
			},
		},
		{
			name:    "unknown option",
			args:    []string{"languages", "create-language", "lv", "Latvian", "Latviešu", "--unknown"},
			wantErr: ErrCommandInvalid,
		},
		{
			name:    "missing native name",
			args:    []string{"languages", "create-language", "lv", "Latvian"},
			wantErr: ErrCommandInvalid,
		},
		{
			name:    "extra create argument",
			args:    []string{"languages", "create-language", "lv", "Latvian", "Latviešu", "extra"},
			wantErr: ErrCommandInvalid,
		},
		{
			name: "delete",
			args: []string{"languages", "delete-language", "lv"},
			want: languageCommand{action: deleteLanguageCommand, code: "lv"},
		},
		{
			name:    "delete without code",
			args:    []string{"languages", "delete-language"},
			wantErr: ErrCommandInvalid,
		},
		{
			name:    "extra delete argument",
			args:    []string{"languages", "delete-language", "lv", "extra"},
			wantErr: ErrCommandInvalid,
		},
		{
			name:    "wrong group",
			args:    []string{"language", "delete-language", "lv"},
			wantErr: ErrCommandInvalid,
		},
		{
			name:    "wrong action",
			args:    []string{"languages", "delete", "lv"},
			wantErr: ErrCommandInvalid,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command, err := parseLanguageCommand(tt.args)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) || command != tt.want {
				t.Fatalf("parseLanguageCommand(%v) = (%+v, %v), want (%+v, %v)", tt.args, command, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestExecuteCreateLanguageCommand(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		createErr  error
		wantOutput string
		wantErrs   []error
		wantText   string
	}{
		{
			name:       "created",
			args:       []string{"languages", "create-language", "en", "English", "English", "-f"},
			wantOutput: "created language en\n",
		},
		{
			name:     "invalid code",
			args:     []string{"languages", "create-language", "LV", "Latvian", "Latviešu"},
			wantErrs: []error{input.ErrCreateLanguageInputInvalid, languages.ErrCodeInvalid},
			wantText: "invalid language code",
		},
		{
			name:      "duplicate",
			args:      []string{"languages", "create-language", "lv", "Latvian", "Latviešu"},
			createErr: languages.ErrLanguageAlreadyExists,
			wantErrs:  []error{languages.ErrLanguageAlreadyExists},
			wantText:  "language already exists",
		},
		{
			name:      "infrastructure failure",
			args:      []string{"languages", "create-language", "lv", "Latvian", "Latviešu"},
			createErr: errors.New("connection reset by secret.internal"),
			wantErrs:  []error{},
			wantText:  "failed to create a language",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stored *languages.Language

			repository := &mock.LanguageRepository{
				CreateFunc: func(
					_ context.Context,
					language *languages.Language,
				) languages.ErrLanguageCreateFailed {
					var err languages.ErrLanguageCreateFailed

					stored = language

					if tt.createErr != nil {
						err = mock.NewErrLanguageCreateFailed(language, tt.createErr)
					}

					return err
				},
			}

			output, err := executeWithMock(t, tt.args, repository)

			assertCommandResult(t, output, err, tt.wantOutput, tt.wantErrs, tt.wantText)

			if tt.wantErrs == nil && (stored == nil || !stored.IsFallback) {
				t.Fatalf("stored language = %+v, want a fallback language", stored)
			}

			if strings.Contains(errorText(err), "secret.internal") {
				t.Fatalf("error %v exposes the infrastructure cause", err)
			}
		})
	}
}

func TestExecuteDeleteLanguageCommand(t *testing.T) {
	for _, tt := range []struct {
		name       string
		code       string
		deleteErr  error
		wantOutput string
		wantErrs   []error
		wantText   string
	}{
		{name: "deleted", code: "lv", wantOutput: "deleted language lv\n"},
		{
			name:     "invalid code",
			code:     "LV",
			wantErrs: []error{languages.ErrCodeInvalid},
			wantText: "invalid language code",
		},
		{
			name:      "not found",
			code:      "lv",
			deleteErr: languages.ErrLanguageNotFound,
			wantErrs:  []error{languages.ErrLanguageNotFound},
			wantText:  "language not found",
		},
		{
			name:      "used by translations",
			code:      "lv",
			deleteErr: languages.ErrLanguageInUse,
			wantErrs:  []error{languages.ErrLanguageInUse},
			wantText:  "language is used by translations",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var deleted languages.Code

			repository := &mock.LanguageRepository{
				DeleteFunc: func(
					_ context.Context,
					code languages.Code,
				) languages.ErrLanguageDeleteFailed {
					var err languages.ErrLanguageDeleteFailed

					deleted = code

					if tt.deleteErr != nil {
						err = mock.NewErrLanguageDeleteFailed(code, tt.deleteErr)
					}

					return err
				},
			}

			output, err := executeWithMock(t, []string{"languages", "delete-language", tt.code}, repository)

			assertCommandResult(t, output, err, tt.wantOutput, tt.wantErrs, tt.wantText)

			if tt.wantErrs == nil && deleted != languages.Code(tt.code) {
				t.Fatalf("deleted code = %q, want %q", deleted, tt.code)
			}
		})
	}
}

func executeWithMock(
	t *testing.T,
	args []string,
	repository *mock.LanguageRepository,
) (string, error) {
	t.Helper()

	var output bytes.Buffer

	command, err := parseLanguageCommand(args)

	if err != nil {
		t.Fatal(err)
	}

	service, err := app.NewLanguageService(&appmock.Transactor{}, repository)

	if err != nil {
		t.Fatal(err)
	}

	err = executeLanguageCommand(t.Context(), command, service, &output)

	return output.String(), err
}

func assertCommandResult(
	t *testing.T,
	output string,
	err error,
	wantOutput string,
	wantErrs []error,
	wantText string,
) {
	t.Helper()

	if output != wantOutput || (err == nil) != (wantErrs == nil) {
		t.Fatalf("command = (%q, %v), want output %q and errors %v", output, err, wantOutput, wantErrs)
	}

	for _, wantErr := range wantErrs {
		if !errors.Is(err, wantErr) {
			t.Fatalf("command error = %v, want %v", err, wantErr)
		}
	}

	if !strings.Contains(errorText(err), wantText) {
		t.Fatalf("command error = %v, want text %q", err, wantText)
	}
}

func errorText(err error) string {
	var text string

	if err != nil {
		text = err.Error()
	}

	return text
}
