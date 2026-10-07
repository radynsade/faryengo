package languages_test

import (
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/languages"
)

func TestTranslationContentValidate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content languages.TranslationContent
		want    []error
	}{
		{name: "English", content: "Hello"},
		{name: "Unicode", content: "Sveiki, pasaule! 世界 🌍"},
		{name: "surrounding whitespace", content: " \tHello\n "},
		{name: "multiline", content: "Hello\nWorld"},
		{name: "long content", content: languages.TranslationContent(strings.Repeat("界", 1000))},
		{name: "empty", want: []error{languages.ErrInvalidTranslationContent, languages.ErrInvalidTranslationContentCharacters}},
		{name: "whitespace", content: " \t\r\n", want: []error{languages.ErrInvalidTranslationContent, languages.ErrInvalidTranslationContentCharacters}},
		{name: "Unicode whitespace", content: "\u00a0\u2003", want: []error{languages.ErrInvalidTranslationContent, languages.ErrInvalidTranslationContentCharacters}},
		{name: "invalid UTF-8", content: "Hello\xff", want: []error{languages.ErrInvalidTranslationContent, languages.ErrInvalidTranslationContentCharacters}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assertValidationErrors(t, tt.content.Validate(), tt.want...)
		})
	}
}

func TestTextValidate(t *testing.T) {
	for _, tt := range []struct {
		name string
		text languages.Text
		want []error
	}{
		{name: "nil map", want: []error{languages.ErrInvalidText, languages.ErrTextHasNoTranslations}},
		{name: "empty map", text: languages.Text{}, want: []error{languages.ErrInvalidText, languages.ErrTextHasNoTranslations}},
		{name: "single translation", text: languages.Text{"en": "Hello"}},
		{name: "multilingual", text: languages.Text{"lv": "Sveiki", "en": "Hello", "ja": "こんにちは"}},
		{name: "empty code", text: languages.Text{"": "Hello"}, want: []error{languages.ErrInvalidText, languages.ErrInvalidCode}},
		{name: "invalid code", text: languages.Text{"EN": "Hello"}, want: []error{languages.ErrInvalidText, languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "invalid UTF-8 code", text: languages.Text{"e\xff": "Hello"}, want: []error{languages.ErrInvalidText, languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters}},
		{name: "empty content", text: languages.Text{"en": ""}, want: []error{languages.ErrInvalidText, languages.ErrInvalidTranslationContent, languages.ErrInvalidTranslationContentCharacters}},
		{name: "whitespace content", text: languages.Text{"en": " \t"}, want: []error{languages.ErrInvalidText, languages.ErrInvalidTranslationContent, languages.ErrInvalidTranslationContentCharacters}},
		{name: "invalid UTF-8 content", text: languages.Text{"en": "\xff"}, want: []error{languages.ErrInvalidText, languages.ErrInvalidTranslationContent, languages.ErrInvalidTranslationContentCharacters}},
		{name: "invalid after valid entry", text: languages.Text{"en": "Hello", "lv": " "}, want: []error{languages.ErrInvalidText, languages.ErrInvalidTranslationContent, languages.ErrInvalidTranslationContentCharacters}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := maps.Clone(tt.text)
			assertValidationErrors(t, tt.text.Validate(), tt.want...)

			if !maps.Equal(tt.text, before) || (tt.text == nil) != (before == nil) {
				t.Fatal("Validate() changed the translations")
			}
		})
	}
}

func TestTextValidateErrorOrder(t *testing.T) {
	for _, tt := range []struct {
		name    string
		text    languages.Text
		want    []error
		exclude error
	}{
		{
			name:    "code before content for the same entry",
			text:    languages.Text{"EN": " "},
			want:    []error{languages.ErrInvalidText, languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters},
			exclude: languages.ErrInvalidTranslationContent,
		},
		{
			name:    "earlier invalid code before later invalid content",
			text:    languages.Text{"EN": "Hello", "lv": " "},
			want:    []error{languages.ErrInvalidText, languages.ErrInvalidCode, languages.ErrInvalidCodeCharacters},
			exclude: languages.ErrInvalidTranslationContent,
		},
		{
			name:    "earlier invalid content before later invalid code",
			text:    languages.Text{"en": " ", "lV": "Sveiki"},
			want:    []error{languages.ErrInvalidText, languages.ErrInvalidTranslationContent, languages.ErrInvalidTranslationContentCharacters},
			exclude: languages.ErrInvalidCode,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Repeated checks exercise randomized map iteration order.
			for range 32 {
				err := tt.text.Validate()
				assertValidationErrors(t, err, tt.want...)

				if errors.Is(err, tt.exclude) {
					t.Fatalf("Validate() error = %v, must stop before %v", err, tt.exclude)
				}
			}
		})
	}
}
