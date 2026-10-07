package input

import (
	"errors"
	"maps"
	"strings"
	"testing"
)

func TestParseStringTranslations(t *testing.T) {
	for _, tt := range []struct {
		name    string
		encoded string
		want    map[string]string
		wantErr error
		entry   string
	}{
		{name: "single translation", encoded: "en:Some text", want: map[string]string{"en": "Some text"}},
		{name: "multilingual Unicode", encoded: "en:Some text|lv:Kāds teksts|ru:Какой-то текст|ja:こんにちは 👋", want: map[string]string{"en": "Some text", "lv": "Kāds teksts", "ru": "Какой-то текст", "ja": "こんにちは 👋"}},
		{name: "trim whitespace", encoded: " \t en : Some text \n | lv : Kāds teksts \t ", want: map[string]string{"en": "Some text", "lv": "Kāds teksts"}},
		{name: "trim Unicode whitespace", encoded: "\u2003en\u2003:\u2003Hello\u2003", want: map[string]string{"en": "Hello"}},
		{name: "preserve internal whitespace", encoded: "en:First  line\nSecond line", want: map[string]string{"en": "First  line\nSecond line"}},
		{name: "last duplicate wins", encoded: "en:First|lv:Sveiki| en :Last", want: map[string]string{"en": "Last", "lv": "Sveiki"}},
		{name: "empty content is left for validation", encoded: "en: ", want: map[string]string{"en": ""}},
		{name: "empty code is left for validation", encoded: ":Some text", want: map[string]string{"": "Some text"}},
		{name: "code is left for validation", encoded: " EN :Hello", want: map[string]string{"EN": "Hello"}},
		{name: "empty fields", encoded: " : ", want: map[string]string{"": ""}},
		{name: "empty input", wantErr: ErrInvalidStringTranslations, entry: "translation 1"},
		{name: "blank input", encoded: " \t ", wantErr: ErrInvalidStringTranslations, entry: "translation 1"},
		{name: "missing separator", encoded: "en Some text", wantErr: ErrInvalidStringTranslations, entry: "translation 1"},
		{name: "multiple colons", encoded: "en:Some:text", wantErr: ErrInvalidStringTranslations, entry: "translation 1"},
		{name: "leading pipe", encoded: "|en:Hello", wantErr: ErrInvalidStringTranslations, entry: "translation 1"},
		{name: "trailing pipe", encoded: "en:Hello|", wantErr: ErrInvalidStringTranslations, entry: "translation 2"},
		{name: "empty middle entry", encoded: "en:Hello||lv:Sveiki", wantErr: ErrInvalidStringTranslations, entry: "translation 2"},
		{name: "malformed entry after valid input", encoded: "en:Hello|lv Sveiki", wantErr: ErrInvalidStringTranslations, entry: "translation 2"},
		{name: "colon in later content", encoded: "en:Hello|lv:Some:text", wantErr: ErrInvalidStringTranslations, entry: "translation 2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			translations, err := ParseStringTranslations(tt.encoded)

			if !errors.Is(err, tt.wantErr) || !maps.Equal(translations, tt.want) {
				t.Fatalf("ParseStringTranslations(%q) = (%v, %v), want (%v, %v)", tt.encoded, translations, err, tt.want, tt.wantErr)
			}

			if tt.wantErr != nil {
				if translations != nil {
					t.Fatal("malformed input returned partial translations")
				}

				if !strings.Contains(err.Error(), tt.entry) || !strings.Contains(err.Error(), "en:Some text") {
					t.Fatalf("parse error lacks the entry or expected format: %v", err)
				}
			}
		})
	}
}
