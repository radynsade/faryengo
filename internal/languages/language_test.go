package languages

import (
	"errors"
	"strings"
	"testing"
)

func TestNewLanguageCode(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		valid bool
	}{
		{name: "English", value: "en", valid: true},
		{name: "Latvian", value: "lv", valid: true},
		{name: "empty", value: ""},
		{name: "one letter", value: "e"},
		{name: "three letters", value: "eng"},
		{name: "uppercase", value: "EN"},
		{name: "region tag", value: "en-US"},
		{name: "digit", value: "e1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, err := NewLanguageCode(tt.value)
			if tt.valid {
				if err != nil || string(code) != tt.value {
					t.Fatalf("NewLanguageCode() = (%q, %v), want (%q, nil)", string(code), err, tt.value)
				}
			} else if code != (LanguageCode("")) || !errors.Is(err, ErrInvalidLanguageCode) {
				t.Fatalf("NewLanguageCode() = (%v, %v), want zero value and ErrInvalidLanguageCode", code, err)
			}
		})
	}
}

func TestNewLanguageNames(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		valid bool
	}{
		{name: "English", value: "English", valid: true},
		{name: "Unicode", value: "Latviešu", valid: true},
		{name: "100 characters", value: strings.Repeat("é", 100), valid: true},
		{name: "empty", value: ""},
		{name: "whitespace", value: "  "},
		{name: "101 characters", value: strings.Repeat("é", 101)},
		{name: "invalid UTF-8", value: "\xff"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, field := range []struct {
				name     string
				newValue func(string) (string, error)
				invalid  error
			}{
				{
					name: "English name",
					newValue: func(value string) (string, error) {
						name, err := NewLanguageEnglishName(value)
						return string(name), err
					},
					invalid: ErrInvalidLanguageEnglishName,
				},
				{
					name: "native name",
					newValue: func(value string) (string, error) {
						name, err := NewLanguageNativeName(value)
						return string(name), err
					},
					invalid: ErrInvalidLanguageNativeName,
				},
			} {
				t.Run(field.name, func(t *testing.T) {
					value, err := field.newValue(tt.value)
					if tt.valid {
						if err != nil || value != tt.value {
							t.Fatalf("constructor = (%q, %v), want (%q, nil)", value, err, tt.value)
						}
					} else if value != "" || !errors.Is(err, field.invalid) {
						t.Fatalf("constructor = (%q, %v), want empty value and %v", value, err, field.invalid)
					}
				})
			}
		})
	}
}

func TestNewLanguage(t *testing.T) {
	code, err := NewLanguageCode("lv")
	if err != nil {
		t.Fatal(err)
	}
	english, err := NewLanguageEnglishName("Latvian")
	if err != nil {
		t.Fatal(err)
	}
	native, err := NewLanguageNativeName("Latviešu")
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name    string
		code    LanguageCode
		english LanguageEnglishName
		native  LanguageNativeName
		want    error
	}{
		{name: "valid", code: code, english: english, native: native},
		{name: "zero code", english: english, native: native, want: ErrInvalidLanguageCode},
		{name: "zero English name", code: code, native: native, want: ErrInvalidLanguageEnglishName},
		{name: "zero native name", code: code, english: english, want: ErrInvalidLanguageNativeName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			language, err := NewLanguage(tt.code, tt.english, tt.native)
			if tt.want != nil {
				if language != nil || !errors.Is(err, tt.want) {
					t.Fatalf("NewLanguage() = (%v, %v), want (nil, %v)", language, err, tt.want)
				}
				return
			}
			if err != nil || language == nil {
				t.Fatalf("NewLanguage() = (%v, %v), want language and nil", language, err)
			}
			if language.Code() != code || language.EnglishName() != english || language.NativeName() != native {
				t.Fatal("NewLanguage() did not retain the supplied values")
			}
		})
	}
}
