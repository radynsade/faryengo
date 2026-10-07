package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
)

func TestCreateLanguageServiceDomainInputs(t *testing.T) {
	for _, tt := range []struct {
		name       string
		request    input.CreateLanguageInput
		wantErrors []error
		wantFields []string
	}{
		{
			name:    "valid",
			request: input.CreateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: "Latviešu"},
		},
		{
			name:    "valid fallback",
			request: input.CreateLanguageInput{Code: "en", EnglishName: "English", NativeName: "English", IsFallback: true},
		},
		{
			name:       "invalid code",
			request:    input.CreateLanguageInput{Code: "LV", EnglishName: "Latvian", NativeName: "Latviešu"},
			wantErrors: []error{languages.ErrCodeInvalidChars},
			wantFields: []string{"code"},
		},
		{
			name:       "blank English name",
			request:    input.CreateLanguageInput{Code: "lv", EnglishName: " ", NativeName: "Latviešu"},
			wantErrors: []error{languages.ErrEnglishNameInvalidChars},
			wantFields: []string{"english name"},
		},
		{
			name:       "long native name",
			request:    input.CreateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: strings.Repeat("a", 101)},
			wantErrors: []error{languages.ErrNativeNameInvalid},
			wantFields: []string{"native name"},
		},
		{
			name:       "all fields invalid",
			request:    input.CreateLanguageInput{Code: "", EnglishName: "", NativeName: ""},
			wantErrors: []error{languages.ErrCodeInvalidChars, languages.ErrEnglishNameInvalidChars, languages.ErrNativeNameInvalid},
			wantFields: []string{"code", "english name", "native name"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, _ := NewLanguageService(&fakeLanguageRepository{})

			_, err := service.Create(t.Context(), tt.request)
			if (err == nil) != (len(tt.wantErrors) == 0) {
				t.Fatalf("Service() error = %v, want errors %v", err, tt.wantErrors)
			}

			if len(tt.wantErrors) > 0 && !errors.Is(err, input.ErrInvalidCreateLanguageInput) {
				t.Fatalf("Service() error = %v, want input.ErrInvalidCreateLanguageInput", err)
			}

			for _, wantErr := range tt.wantErrors {
				if !errors.Is(err, wantErr) {
					t.Fatalf("Service() error = %v, want %v", err, wantErr)
				}
			}

			for _, field := range tt.wantFields {
				if !strings.Contains(err.Error(), field+":") {
					t.Fatalf("Service() error = %v, want field %q", err, field)
				}
			}
		})
	}
}
