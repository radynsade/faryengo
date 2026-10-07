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
		code languages.Code
		want []error
	}{
		{name: "English", code: "en"},
		{name: "Latvian", code: "lv"},
		{name: "lowercase pair", code: "zz"},
		{name: "empty", want: []error{languages.ErrInvalidCode}},
		{name: "one letter", code: "e", want: []error{languages.ErrInvalidCode}},
		{name: "three letters", code: "eng", want: []error{languages.ErrInvalidCode}},
		{name: "uppercase", code: "EN", want: []error{languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "mixed case", code: "eN", want: []error{languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "region tag", code: "en-US", want: []error{languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "digit", code: "e1", want: []error{languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "punctuation", code: "e-", want: []error{languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "leading whitespace", code: " en", want: []error{languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "trailing whitespace", code: "en ", want: []error{languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "trailing newline", code: "en\n", want: []error{languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "non ASCII letters", code: "éñ", want: []error{languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "invalid UTF-8", code: "e\xff", want: []error{languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
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
		empty      error
		tooLong    error
	}{
		{
			name: "English name",
			validate: func(value string) error {
				return languages.EnglishName(value).Validate()
			},
			invalid:    languages.ErrInvalidEnglishName,
			characters: languages.ErrInvalidEnglishNameCharacters,
			empty:      languages.ErrEmptyEnglishName,
			tooLong:    languages.ErrTooLongEnglishName,
		},
		{
			name: "native name",
			validate: func(value string) error {
				return languages.NativeName(value).Validate()
			},
			invalid:    languages.ErrInvalidNativeName,
			characters: languages.ErrInvalidNativeNameCharacters,
			empty:      languages.ErrEmptyNativeName,
			tooLong:    languages.ErrTooLongNativeName,
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
				{name: "empty", cause: field.empty},
				{name: "whitespace", value: " \t\r\n", cause: field.empty},
				{name: "Unicode whitespace", value: "\u00a0\u2003", cause: field.empty},
				{name: "101 ASCII characters", value: strings.Repeat("a", 101), cause: field.tooLong},
				{name: "101 Unicode characters", value: strings.Repeat("界", 101), cause: field.tooLong},
				{name: "whitespace counts toward length", value: " " + strings.Repeat("a", 100), cause: field.tooLong},
				{name: "invalid UTF-8", value: "Eng\xfflish", cause: field.characters},
				{name: "invalid UTF-8 before length", value: strings.Repeat("a", 101) + "\xff", cause: field.characters},
				{name: "blank before length", value: strings.Repeat(" ", 101), cause: field.empty},
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
		code     languages.Code
		english  languages.EnglishName
		native   languages.NativeName
		fallback bool
	}{
		{name: "English", code: "en", english: "English", native: "English"},
		{name: "Latvian fallback", code: "lv", english: "Latvian", native: "Latviešu", fallback: true},
		{name: "preserves whitespace", code: "en", english: " English ", native: " English "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			language := languages.NewLanguage(tt.code, tt.english, tt.native, tt.fallback)

			if language == nil {
				t.Fatal("NewLanguage() returned nil")
			}

			if language.Code != tt.code || language.EnglishName != tt.english || language.NativeName != tt.native || language.IsFallback != tt.fallback {
				t.Fatal("NewLanguage() did not retain the supplied values")
			}

			assertValidationErrors(t, language.Validate())
		})
	}
}

func TestLanguageValidateFieldChanges(t *testing.T) {
	for _, tt := range []struct {
		name            string
		initialFallback bool
		change          func(*languages.Language)
		want            []error
	}{
		{name: "valid code", change: func(language *languages.Language) { language.Code = "en" }},
		{
			name:   "invalid code",
			change: func(language *languages.Language) { language.Code = "LV" },
			want:   []error{languages.ErrInvalidLanguage, languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters},
		},
		{name: "valid English name", change: func(language *languages.Language) { language.EnglishName = " Latvian " }},
		{
			name:   "invalid English name",
			change: func(language *languages.Language) { language.EnglishName = "" },
			want:   []error{languages.ErrInvalidLanguage, languages.ErrInvalidEnglishName, languages.ErrEmptyEnglishName},
		},
		{name: "valid native name", change: func(language *languages.Language) { language.NativeName = " Latviešu " }},
		{
			name:   "invalid native name",
			change: func(language *languages.Language) { language.NativeName = "" },
			want:   []error{languages.ErrInvalidLanguage, languages.ErrInvalidNativeName, languages.ErrEmptyNativeName},
		},
		{name: "enable fallback", change: func(language *languages.Language) { language.IsFallback = true }},
		{name: "disable fallback", initialFallback: true, change: func(language *languages.Language) { language.IsFallback = false }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			language := languages.NewLanguage("lv", "Latvian", "Latviešu", tt.initialFallback)

			if language == nil {
				t.Fatal("NewLanguage() returned nil")
			}

			tt.change(language)
			before := *language
			assertValidationErrors(t, language.Validate(), tt.want...)

			if *language != before {
				t.Fatal("Validate() changed the language state")
			}
		})
	}
}

func TestLanguageValidate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		code    languages.Code
		english languages.EnglishName
		native  languages.NativeName
		want    []error
		exclude error
	}{
		{name: "valid", code: "lv", english: "Latvian", native: "Latviešu"},
		{name: "zero values", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}, exclude: languages.ErrInvalidEnglishName},
		{name: "invalid code", code: "LV", english: "Latvian", native: "Latviešu", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "empty English name", code: "lv", native: "Latviešu", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidEnglishName, languages.ErrEmptyEnglishName}},
		{name: "invalid English UTF-8", code: "lv", english: "\xff", native: "Latviešu", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidEnglishName, languages.ErrInvalidEnglishNameCharacters}},
		{name: "long English name", code: "lv", english: languages.EnglishName(strings.Repeat("界", 101)), native: "Latviešu", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidEnglishName, languages.ErrTooLongEnglishName}},
		{name: "empty native name", code: "lv", english: "Latvian", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidNativeName, languages.ErrEmptyNativeName}},
		{name: "invalid native UTF-8", code: "lv", english: "Latvian", native: "\xff", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidNativeName, languages.ErrInvalidNativeNameCharacters}},
		{name: "long native name", code: "lv", english: "Latvian", native: languages.NativeName(strings.Repeat("界", 101)), want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidNativeName, languages.ErrTooLongNativeName}},
		{name: "English validated before native", code: "lv", want: []error{languages.ErrInvalidLanguage, languages.ErrInvalidEnglishName, languages.ErrEmptyEnglishName}, exclude: languages.ErrInvalidNativeName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			language := languages.NewLanguage(tt.code, tt.english, tt.native, false)

			if language == nil {
				t.Fatal("NewLanguage() returned nil before validation")
			}

			err := language.Validate()
			assertValidationErrors(t, err, tt.want...)

			if tt.exclude != nil && errors.Is(err, tt.exclude) {
				t.Errorf("Validate() error = %v, must stop before %v", err, tt.exclude)
			}

			if language.Code != tt.code || language.EnglishName != tt.english || language.NativeName != tt.native || language.IsFallback {
				t.Fatal("Validate() changed the language state")
			}
		})
	}
}
