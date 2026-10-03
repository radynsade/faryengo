package input

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidStringTranslations = errors.New("invalid string translations")

// ParseStringTranslations parses "en:Some text|lv:Kāds teksts" into input values.
// It trims keys and content, and the last entry for a language wins. Each entry
// must contain exactly one colon; delimiters cannot be escaped. Language codes
// and content are left for the caller to validate.
func ParseStringTranslations(encoded string) (map[string]string, error) {
	entries := strings.Split(encoded, "|")
	translations := make(map[string]string, len(entries))
	var err error

	for index, entry := range entries {
		code, content, found := strings.Cut(entry, ":")

		if !found || strings.Contains(content, ":") {
			err = fmt.Errorf("parse translation %d: %w: expected format \"en:Some text\"", index+1, ErrInvalidStringTranslations)
			break
		}

		translations[strings.TrimSpace(code)] = strings.TrimSpace(content)
	}

	if err != nil {
		translations = nil
	}

	return translations, err
}
