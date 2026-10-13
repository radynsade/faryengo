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
		{"text", func() (any, error) {
			return constructed(NewText(map[string]string{"en": "Editor", "lv": "Redaktors"}))
		}, Text{"en": "Editor", "lv": "Redaktors"}, nil},
		{"missing text", func() (any, error) { return constructed(NewText(nil)) }, Text(nil), ErrTextNil},
		{"text without translations", func() (any, error) {
			return constructed(NewText(map[string]string{}))
		}, Text{}, ErrTextWithoutTranslations},
		{"text with a blank translation", func() (any, error) {
			return constructed(NewText(map[string]string{"en": " "}))
		}, Text{"en": " "}, ErrTranslationEmpty},
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
