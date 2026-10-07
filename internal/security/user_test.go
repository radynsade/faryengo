package security_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/radynsade/faryengo/internal/security"
)

func domainUser() *security.User {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

	return security.NewUser(security.UserID{1}, security.RoleID{2}, "alice@example.com", now,
		"+37123456789", now, "hash", now, "Alice", "Example", now, now)
}

func TestUserValidate(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*security.User)
		want   error
	}{
		{name: "valid", change: func(*security.User) {}},
		{name: "ID", change: func(u *security.User) { u.ID = security.UserID{} }, want: security.ErrInvalidUserID},
		{name: "role", change: func(u *security.User) { u.RoleID = security.RoleID{} }, want: security.ErrInvalidRoleID},
		{name: "email", change: func(u *security.User) { u.Email = "Alice <alice@example.com>" }, want: security.ErrInvalidEmail},
		{name: "phone", change: func(u *security.User) { u.Phone = "+0123" }, want: security.ErrInvalidPhone},
		{name: "hash", change: func(u *security.User) { u.PasswordHash = " " }, want: security.ErrInvalidPasswordHash},
		{name: "first name", change: func(u *security.User) { u.FirstName = "" }, want: security.ErrEmptyFirstName},
		{name: "last name", change: func(u *security.User) { u.LastName = "" }, want: security.ErrEmptyLastName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			user := domainUser()
			tt.change(user)
			before := *user
			err := user.Validate()

			if !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if tt.want != nil && !errors.Is(err, security.ErrInvalidUser) {
				t.Fatalf("Validate() lost the user error: %v", err)
			}

			if *user != before {
				t.Fatal("Validate changed user state")
			}
		})
	}

	var user *security.User

	if !errors.Is(user.Validate(), security.ErrInvalidUser) {
		t.Fatal("nil user must be invalid")
	}
}

func TestUserNamesValidate(t *testing.T) {
	for _, field := range []struct {
		name                                string
		validate                            func(string) error
		invalid, empty, characters, tooLong error
	}{
		{name: "first", validate: func(s string) error { return security.FirstName(s).Validate() },
			invalid: security.ErrInvalidFirstName, empty: security.ErrEmptyFirstName,
			characters: security.ErrInvalidFirstNameCharacters, tooLong: security.ErrTooLongFirstName},
		{name: "last", validate: func(s string) error { return security.LastName(s).Validate() },
			invalid: security.ErrInvalidLastName, empty: security.ErrEmptyLastName,
			characters: security.ErrInvalidLastNameCharacters, tooLong: security.ErrTooLongLastName},
	} {
		t.Run(field.name, func(t *testing.T) {
			for _, tt := range []struct {
				name, value string
				cause       error
			}{
				{name: "Unicode", value: "日本語"},
				{name: "boundary", value: strings.Repeat("界", 100)},
				{name: "whitespace retained", value: " Alice "},
				{name: "empty", cause: field.empty},
				{name: "blank", value: " \t", cause: field.empty},
				{name: "too long", value: strings.Repeat("界", 101), cause: field.tooLong},
				{name: "invalid UTF-8", value: "a\xff", cause: field.characters},
				{name: "blank before length", value: strings.Repeat(" ", 101), cause: field.empty},
			} {
				t.Run(tt.name, func(t *testing.T) {
					err := field.validate(tt.value)

					if !errors.Is(err, tt.cause) || (tt.cause != nil && !errors.Is(err, field.invalid)) {
						t.Fatalf("Validate() = %v, want %v and %v", err, field.invalid, tt.cause)
					}
				})
			}
		})
	}
}

func TestUserContactValues(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want error
	}{
		{name: "mailbox", err: security.Email("a@example.com").Validate()},
		{name: "display name", err: security.Email("A <a@example.com>").Validate(), want: security.ErrInvalidEmail},
		{name: "email whitespace", err: security.Email(" a@example.com").Validate(), want: security.ErrInvalidEmail},
		{name: "phone", err: security.Phone("+37123456789").Validate()},
		{name: "maximum phone", err: security.Phone("+123456789012345").Validate()},
		{name: "long phone", err: security.Phone("+1234567890123456").Validate(), want: security.ErrInvalidPhone},
		{name: "phone punctuation", err: security.Phone("+371 23456789").Validate(), want: security.ErrInvalidPhone},
		{name: "zero identity", err: (security.UserID{}).Validate(), want: security.ErrInvalidUserID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", tt.err, tt.want)
			}
		})
	}
}
