package security_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/security"
)

func TestPasswordValidate(t *testing.T) {
	for _, tt := range []struct {
		name, value string
		want        error
	}{
		{name: "minimum", value: "abcdef"},
		{name: "Unicode characters", value: "日本語日本語"},
		{name: "maximum bytes", value: strings.Repeat("a", security.MaxPasswordBytes)},
		{name: "empty", want: security.ErrEmptyPassword},
		{name: "whitespace", value: "      ", want: security.ErrEmptyPassword},
		{name: "short", value: "abcde", want: security.ErrTooShortPassword},
		{name: "short Unicode", value: "日本語", want: security.ErrTooShortPassword},
		{name: "oversized", value: strings.Repeat("a", security.MaxPasswordBytes+1), want: security.ErrTooLongPassword},
		{name: "invalid UTF-8", value: "abcde\xff", want: security.ErrInvalidPasswordCharacters},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := security.Password(tt.value).Validate()

			if !errors.Is(err, tt.want) || (tt.want != nil && !errors.Is(err, security.ErrInvalidPassword)) {
				t.Fatalf("Validate() = %v, want %v wrapped in ErrInvalidPassword", err, tt.want)
			}

			if errors.Is(err, security.ErrInvalidPasswordHash) {
				t.Fatal("plaintext error incorrectly wraps hash error")
			}
		})
	}
}

func TestPasswordHashValidate(t *testing.T) {
	for _, tt := range []struct {
		name string
		hash security.PasswordHash
		want error
	}{
		{name: "nonempty", hash: "encoded-hash"},
		{name: "empty", want: security.ErrEmptyPasswordHash},
		{name: "whitespace", hash: " \t", want: security.ErrEmptyPasswordHash},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.hash.Validate()

			if !errors.Is(err, tt.want) || (tt.want != nil && !errors.Is(err, security.ErrInvalidPasswordHash)) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}
