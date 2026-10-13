package languages

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// Every constructor returns the value it was given, even when the value is
// invalid, together with the error of its validation.

func TestValueConstructors(t *testing.T) {
	tests := []struct {
		name      string
		construct func() (any, error)
		want      any
		wantErr   error
	}{
		{"code", func() (any, error) { return constructed(NewCode("lv")) }, Code("lv"), nil},
		{"upper-case code", func() (any, error) { return constructed(NewCode("LV")) }, Code("LV"), ErrCodeInvalid},
		{"English name", func() (any, error) { return constructed(NewEnglishName("Latvian")) }, EnglishName("Latvian"), nil},
		{"blank English name", func() (any, error) { return constructed(NewEnglishName(" ")) }, EnglishName(" "), ErrEnglishNameEmpty},
		{"native name", func() (any, error) { return constructed(NewNativeName("Latviešu")) }, NativeName("Latviešu"), nil},
		{"too long native name", func() (any, error) {
			return constructed(NewNativeName(strings.Repeat("ā", MaxNativeNameLength+1)))
		}, NativeName(strings.Repeat("ā", MaxNativeNameLength+1)), ErrNativeNameTooLong},
		{"translation", func() (any, error) { return constructed(NewTranslation("Redaktors")) }, Translation("Redaktors"), nil},
		{"empty translation", func() (any, error) { return constructed(NewTranslation("")) }, Translation(""), ErrTranslationEmpty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.construct()

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) {
				t.Fatalf("got error %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// The text constructor only assigns, so it keeps invalid translations for
// its validation to report.

func TestTextValidate(t *testing.T) {
	tests := []struct {
		name         string
		translations map[Code]Translation
		want         error
	}{
		{"valid", map[Code]Translation{"en": "Editor", "lv": "Redaktors"}, nil},
		{"missing", nil, ErrTextNil},
		{"without translations", map[Code]Translation{}, ErrTextWithoutTranslations},
		{"blank translation", map[Code]Translation{"en": " "}, ErrTranslationEmpty},
		{"invalid code", map[Code]Translation{"english": "Editor"}, ErrCodeInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := NewText(tt.translations)

			if !reflect.DeepEqual(map[Code]Translation(text), tt.translations) {
				t.Fatalf("NewText() = %#v, want %#v", text, tt.translations)
			}

			err := text.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if tt.want != nil && !errors.Is(err, ErrTextInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrTextInvalid)
			}
		})
	}
}

func constructed[T any](value T, err error) (any, error) {
	return value, err
}

func TestLanguageValidate(t *testing.T) {
	tests := []struct {
		name     string
		language *Language
		wantErrs []error
	}{
		{"valid", NewLanguage("lv", "Latvian", "Latviešu", false), nil},
		{"nil", nil, []error{ErrLanguageNil}},
		{"invalid code", NewLanguage("LV", "Latvian", "Latviešu", false), []error{ErrCodeInvalid}},
		{"invalid code and blank names", NewLanguage("LV", " ", "", false), []error{ErrCodeInvalid, ErrEnglishNameEmpty, ErrNativeNameEmpty}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.language.Validate()

			if (err == nil) != (len(tt.wantErrs) == 0) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErrs)
			}

			for _, want := range tt.wantErrs {
				if !errors.Is(err, want) {
					t.Fatalf("Validate() = %v, want %v", err, want)
				}
			}

			if err != nil && !errors.Is(err, ErrLanguageInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrLanguageInvalid)
			}
		})
	}
}
