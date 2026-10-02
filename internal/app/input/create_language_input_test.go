package input

import (
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/languages"
)

func TestCreateLanguageInputValidate(t *testing.T) {
	for _, tt := range []struct {
		name       string
		input      CreateLanguageInput
		wantErrors []error
		wantFields []string
	}{
		{
			name:  "valid",
			input: CreateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: "Latviešu"},
		},
		{
			name:  "valid fallback",
			input: CreateLanguageInput{Code: "en", EnglishName: "English", NativeName: "English", IsFallback: true},
		},
		{
			name:       "invalid code",
			input:      CreateLanguageInput{Code: "LV", EnglishName: "Latvian", NativeName: "Latviešu"},
			wantErrors: []error{languages.ErrInvalidLanguageCode},
			wantFields: []string{"code"},
		},
		{
			name:       "blank English name",
			input:      CreateLanguageInput{Code: "lv", EnglishName: " ", NativeName: "Latviešu"},
			wantErrors: []error{languages.ErrInvalidLanguageEnglishName},
			wantFields: []string{"english name"},
		},
		{
			name:       "long native name",
			input:      CreateLanguageInput{Code: "lv", EnglishName: "Latvian", NativeName: strings.Repeat("a", 101)},
			wantErrors: []error{languages.ErrInvalidLanguageNativeName},
			wantFields: []string{"native name"},
		},
		{
			name:       "all fields invalid",
			input:      CreateLanguageInput{Code: "", EnglishName: "", NativeName: ""},
			wantErrors: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageEnglishName, languages.ErrInvalidLanguageNativeName},
			wantFields: []string{"code", "english name", "native name"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if (err == nil) != (len(tt.wantErrors) == 0) {
				t.Fatalf("Validate() error = %v, want errors %v", err, tt.wantErrors)
			}

			if len(tt.wantErrors) > 0 && !errors.Is(err, ErrInvalidCreateLanguageInput) {
				t.Fatalf("Validate() error = %v, want ErrInvalidCreateLanguageInput", err)
			}

			for _, wantErr := range tt.wantErrors {
				if !errors.Is(err, wantErr) {
					t.Fatalf("Validate() error = %v, want %v", err, wantErr)
				}
			}

			for _, field := range tt.wantFields {
				if !strings.Contains(err.Error(), field+":") {
					t.Fatalf("Validate() error = %v, want field %q", err, field)
				}
			}
		})
	}
}
