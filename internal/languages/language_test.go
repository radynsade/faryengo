package languages_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/languages"
)

func TestLanguageCodeValidate(t *testing.T) {
	for _, tt := range []struct {
		name string
		code languages.LanguageCode
		want []error
	}{
		{name: "English", code: "en"},
		{name: "Latvian", code: "lv"},
		{name: "lowercase pair", code: "zz"},
		{name: "empty", want: []error{languages.ErrInvalidLanguageCode}},
		{name: "one letter", code: "e", want: []error{languages.ErrInvalidLanguageCode}},
		{name: "three letters", code: "eng", want: []error{languages.ErrInvalidLanguageCode}},
		{name: "uppercase", code: "EN", want: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}},
		{name: "mixed case", code: "eN", want: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}},
		{name: "region tag", code: "en-US", want: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}},
		{name: "digit", code: "e1", want: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}},
		{name: "punctuation", code: "e-", want: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}},
		{name: "leading whitespace", code: " en", want: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}},
		{name: "trailing whitespace", code: "en ", want: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}},
		{name: "trailing newline", code: "en\n", want: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}},
		{name: "non ASCII letters", code: "éñ", want: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}},
		{name: "invalid UTF-8", code: "e\xff", want: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assertValidationErrors(t, tt.code.Validate(), tt.want...)
		})
	}
}

func TestLanguageNamesValidate(t *testing.T) {
	for _, field := range []struct {
		name       string
		validate   func(string) error
		invalid    error
		characters error
		tooLong    error
	}{
		{
			name: "English name",
			validate: func(value string) error {
				return languages.LanguageEnglishName(value).Validate()
			},
			invalid:    languages.ErrInvalidLanguageEnglishName,
			characters: languages.ErrInvalidLanguageEnglishNameCharacters,
			tooLong:    languages.ErrTooLongLanguageEnglishName,
		},
		{
			name: "native name",
			validate: func(value string) error {
				return languages.LanguageNativeName(value).Validate()
			},
			invalid:    languages.ErrInvalidLanguageNativeName,
			characters: languages.ErrInvalidLanguageNativeNameCharacters,
			tooLong:    languages.ErrTooLongLanguageNativeName,
		},
	} {
		t.Run(field.name, func(t *testing.T) {
			for _, tt := range []struct {
				name  string
				value string
				cause error
			}{
				{name: "English", value: "English"},
				{name: "Unicode", value: "Latviešu"},
				{name: "non Latin script", value: "日本語"},
				{name: "surrounding whitespace", value: " English \t"},
				{name: "100 ASCII characters", value: strings.Repeat("a", 100)},
				{name: "100 Unicode characters", value: strings.Repeat("界", 100)},
				{name: "empty", cause: field.characters},
				{name: "whitespace", value: " \t\r\n", cause: field.characters},
				{name: "Unicode whitespace", value: "\u00a0\u2003", cause: field.characters},
				{name: "101 ASCII characters", value: strings.Repeat("a", 101), cause: field.tooLong},
				{name: "101 Unicode characters", value: strings.Repeat("界", 101), cause: field.tooLong},
				{name: "whitespace counts toward length", value: " " + strings.Repeat("a", 100), cause: field.tooLong},
				{name: "invalid UTF-8", value: "Eng\xfflish", cause: field.characters},
				{name: "invalid UTF-8 before length", value: strings.Repeat("a", 101) + "\xff", cause: field.characters},
				{name: "blank before length", value: strings.Repeat(" ", 101), cause: field.characters},
			} {
				t.Run(tt.name, func(t *testing.T) {
					err := field.validate(tt.value)

					if tt.cause == nil {
						assertValidationErrors(t, err)
					} else {
						assertValidationErrors(t, err, field.invalid, tt.cause)
					}
				})
			}
		})
	}
}

func TestNewLanguage(t *testing.T) {
	for _, tt := range []struct {
		name     string
		code     languages.LanguageCode
		english  languages.LanguageEnglishName
		native   languages.LanguageNativeName
		fallback bool
	}{
		{name: "English", code: "en", english: "English", native: "English"},
		{name: "Latvian fallback", code: "lv", english: "Latvian", native: "Latviešu", fallback: true},
		{name: "preserves whitespace", code: "en", english: " English ", native: " English "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			language, err := languages.NewLanguage(tt.code, tt.english, tt.native, tt.fallback)

			if err != nil || language == nil {
				t.Fatalf("NewLanguage() = (%v, %v), want language and nil error", language, err)
			}

			if language.Code() != tt.code || language.EnglishName() != tt.english || language.NativeName() != tt.native || language.IsFallback() != tt.fallback {
				t.Fatal("NewLanguage() did not retain the supplied values")
			}

			assertValidationErrors(t, language.Validate())
		})
	}
}

func TestLanguageSetIsFallback(t *testing.T) {
	for _, tt := range []struct {
		name    string
		initial bool
		updated bool
	}{
		{name: "enable", updated: true},
		{name: "disable", initial: true},
		{name: "keep enabled", initial: true, updated: true},
		{name: "keep disabled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			language, err := languages.NewLanguage("lv", "Latvian", "Latviešu", tt.initial)

			if err != nil || language == nil {
				t.Fatalf("NewLanguage() = (%v, %v), want language and nil error", language, err)
			}

			language.SetIsFallback(tt.updated)

			if language.IsFallback() != tt.updated {
				t.Errorf("IsFallback() = %v, want %v", language.IsFallback(), tt.updated)
			}

			if language.Code() != "lv" || language.EnglishName() != "Latvian" || language.NativeName() != "Latviešu" {
				t.Fatal("SetIsFallback() changed the language code or names")
			}

			assertValidationErrors(t, language.Validate())
		})
	}
}

func TestLanguageValidate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		code    languages.LanguageCode
		english languages.LanguageEnglishName
		native  languages.LanguageNativeName
		want    []error
		exclude error
	}{
		{name: "valid", code: "lv", english: "Latvian", native: "Latviešu"},
		{name: "zero values", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}, exclude: languages.ErrInvalidLanguageEnglishName},
		{name: "invalid code", code: "LV", english: "Latvian", native: "Latviešu", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidLanguageCode, languages.ErrInvalidLanguageCodeCharacters}},
		{name: "empty English name", code: "lv", native: "Latviešu", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidLanguageEnglishName, languages.ErrInvalidLanguageEnglishNameCharacters}},
		{name: "invalid English UTF-8", code: "lv", english: "\xff", native: "Latviešu", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidLanguageEnglishName, languages.ErrInvalidLanguageEnglishNameCharacters}},
		{name: "long English name", code: "lv", english: languages.LanguageEnglishName(strings.Repeat("界", 101)), native: "Latviešu", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidLanguageEnglishName, languages.ErrTooLongLanguageEnglishName}},
		{name: "empty native name", code: "lv", english: "Latvian", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidLanguageNativeName, languages.ErrInvalidLanguageNativeNameCharacters}},
		{name: "invalid native UTF-8", code: "lv", english: "Latvian", native: "\xff", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidLanguageNativeName, languages.ErrInvalidLanguageNativeNameCharacters}},
		{name: "long native name", code: "lv", english: "Latvian", native: languages.LanguageNativeName(strings.Repeat("界", 101)), want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidLanguageNativeName, languages.ErrTooLongLanguageNativeName}},
		{name: "English validated before native", code: "lv", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidLanguageEnglishName, languages.ErrInvalidLanguageEnglishNameCharacters}, exclude: languages.ErrInvalidLanguageNativeName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			language, err := languages.NewLanguage(tt.code, tt.english, tt.native, false)

			if err != nil || language == nil {
				t.Fatalf("NewLanguage() = (%v, %v), want language and nil error before validation", language, err)
			}

			err = language.Validate()
			assertValidationErrors(t, err, tt.want...)

			if tt.exclude != nil && errors.Is(err, tt.exclude) {
				t.Errorf("Validate() error = %v, must stop before %v", err, tt.exclude)
			}

			if language.Code() != tt.code || language.EnglishName() != tt.english || language.NativeName() != tt.native || language.IsFallback() {
				t.Fatal("Validate() changed the language state")
			}
		})
	}
}
