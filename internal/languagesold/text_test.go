package languages

import (
	"errors"
	"testing"
)

func TestNewTranslation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		code    LanguageCode
		content string
		wantErr error
	}{
		{name: "English", code: "en", content: "Hello"},
		{name: "Unicode", code: "lv", content: "Sveiki"},
		{name: "invalid code", code: "EN", content: "Hello", wantErr: ErrInvalidLanguageCode},
		{name: "empty content", code: "en", wantErr: ErrInvalidTranslationContent},
		{name: "whitespace content", code: "en", content: " \t ", wantErr: ErrInvalidTranslationContent},
		{name: "invalid UTF-8", code: "en", content: "\xff", wantErr: ErrInvalidTranslationContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			translation, err := NewTranslation(tt.code, tt.content)

			if tt.wantErr != nil {
				if translation != (Translation{}) || !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewTranslation() = (%v, %v), want zero value and %v", translation, err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("NewTranslation() error = %v, want nil", err)
				}

				if translation.LanguageCode() != tt.code || translation.Content() != tt.content {
					t.Fatalf("NewTranslation() = %v, want (%q, %q)", translation, tt.code, tt.content)
				}
			}
		})
	}
}

func TestNewText(t *testing.T) {
	for _, tt := range []struct {
		name         string
		translations []Translation
		wantErr      error
	}{
		{name: "empty"},
		{name: "multilingual", translations: []Translation{{languageCode: "en", content: "Hello"}, {languageCode: "lv", content: "Sveiki"}}},
		{name: "duplicate code", translations: []Translation{{languageCode: "en", content: "Hello"}, {languageCode: "en", content: "Hi"}}, wantErr: ErrDuplicateTranslation},
		{name: "invalid code", translations: []Translation{{languageCode: "EN", content: "Hello"}}, wantErr: ErrInvalidLanguageCode},
		{name: "invalid content", translations: []Translation{{languageCode: "en", content: " "}}, wantErr: ErrInvalidTranslationContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			text, err := NewText(tt.translations)

			if tt.wantErr != nil {
				if len(text.Translations()) != 0 || !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewText() = (%v, %v), want empty text and %v", text, err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("NewText() error = %v, want nil", err)
				}

				if len(text.Translations()) != len(tt.translations) {
					t.Fatalf("NewText() has %d translations, want %d", len(text.Translations()), len(tt.translations))
				}
			}
		})
	}
}

func TestTextLookupAndCopies(t *testing.T) {
	input := []Translation{{languageCode: "en", content: "Hello"}, {languageCode: "lv", content: "Sveiki"}}
	text, err := NewText(input)

	if err != nil {
		t.Fatalf("NewText() error = %v, want nil", err)
	}

	input[0] = Translation{languageCode: "fr", content: "Bonjour"}
	output := text.Translations()
	output[1] = Translation{languageCode: "de", content: "Hallo"}

	for _, tt := range []struct {
		name        string
		code        LanguageCode
		wantContent string
		wantFound   bool
	}{
		{name: "original English", code: "en", wantContent: "Hello", wantFound: true},
		{name: "original Latvian", code: "lv", wantContent: "Sveiki", wantFound: true},
		{name: "missing French", code: "fr"},
		{name: "missing German", code: "de"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			translation, found := text.Translation(tt.code)

			if found != tt.wantFound || translation.Content() != tt.wantContent {
				t.Fatalf("Translation(%q) = (%v, %v), want content %q and found %v", tt.code, translation, found, tt.wantContent, tt.wantFound)
			}
		})
	}
}

func TestTextMapValidation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		text    Text
		wantErr error
	}{
		{name: "nil map"},
		{name: "valid map", text: Text{"en": {languageCode: "en", content: "Hello"}}},
		{name: "invalid key", text: Text{"EN": {languageCode: "EN", content: "Hello"}}, wantErr: ErrInvalidLanguageCode},
		{name: "mismatched key", text: Text{"en": {languageCode: "lv", content: "Sveiki"}}, wantErr: ErrTranslationLanguageMismatch},
		{name: "invalid content", text: Text{"en": {languageCode: "en", content: " "}}, wantErr: ErrInvalidTranslationContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.text.Validate()

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Text.Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestTextTranslationsAreSorted(t *testing.T) {
	text := Text{
		"lv": {languageCode: "lv", content: "Sveiki"},
		"en": {languageCode: "en", content: "Hello"},
	}
	translations := text.Translations()

	if len(translations) != 2 || translations[0].LanguageCode() != "en" || translations[1].LanguageCode() != "lv" {
		t.Fatalf("Translations() = %v, want English then Latvian", translations)
	}
}
