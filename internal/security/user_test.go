package security

import (
	"errors"
	"strings"
	"testing"
)

func TestValueConstructors(t *testing.T) {
	tests := []struct {
		name     string
		newValue func(string) (string, error)
		invalid  error
		cases    []struct {
			name  string
			value string
			valid bool
		}
	}{
		{
			name: "email",
			newValue: func(value string) (string, error) {
				email, err := NewEmail(value)
				return string(email), err
			},
			invalid: ErrInvalidEmail,
			cases: []struct {
				name, value string
				valid       bool
			}{
				{name: "plain address", value: "person@example.com", valid: true},
				{name: "tagged address", value: "person+tag@mail.example.com", valid: true},
				{name: "empty", value: ""},
				{name: "missing at sign", value: "person.example.com"},
				{name: "display name", value: "Person <person@example.com>"},
				{name: "whitespace", value: " person@example.com "},
			},
		},
		{
			name: "phone",
			newValue: func(value string) (string, error) {
				phone, err := NewPhone(value)
				return string(phone), err
			},
			invalid: ErrInvalidPhone,
			cases: []struct {
				name, value string
				valid       bool
			}{
				{name: "international number", value: "+37123456789", valid: true},
				{name: "fifteen digits", value: "+123456789012345", valid: true},
				{name: "empty", value: ""},
				{name: "missing plus", value: "37123456789"},
				{name: "leading zero", value: "+012345"},
				{name: "too long", value: "+1234567890123456"},
				{name: "punctuation", value: "+371 23456789"},
			},
		},
		{
			name: "password hash",
			newValue: func(value string) (string, error) {
				hash, err := NewPasswordHash(value)
				return string(hash), err
			},
			invalid: ErrInvalidPasswordHash,
			cases: []struct {
				name, value string
				valid       bool
			}{
				{name: "hash", value: "hash", valid: true},
				{name: "empty", value: ""},
				{name: "whitespace", value: "  "},
			},
		},
		{
			name: "first name",
			newValue: func(value string) (string, error) {
				name, err := NewFirstName(value)
				return string(name), err
			},
			invalid: ErrInvalidFirstName,
			cases:   nameCases(),
		},
		{
			name: "last name",
			newValue: func(value string) (string, error) {
				name, err := NewLastName(value)
				return string(name), err
			},
			invalid: ErrInvalidLastName,
			cases:   nameCases(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, tc := range tt.cases {
				t.Run(tc.name, func(t *testing.T) {
					value, err := tt.newValue(tc.value)
					if tc.valid {
						if err != nil || value != tc.value {
							t.Fatalf("constructor = (%q, %v), want (%q, nil)", value, err, tc.value)
						}
					} else if value != "" || !errors.Is(err, tt.invalid) {
						t.Fatalf("constructor = (%q, %v), want empty value and %v", value, err, tt.invalid)
					}
				})
			}
		})
	}
}

func nameCases() []struct {
	name, value string
	valid       bool
} {
	return []struct {
		name, value string
		valid       bool
	}{
		{name: "one character", value: "A", valid: true},
		{name: "100 Unicode characters", value: strings.Repeat("é", 100), valid: true},
		{name: "empty", value: ""},
		{name: "whitespace", value: "   "},
		{name: "101 Unicode characters", value: strings.Repeat("é", 101)},
	}
}

func TestUserRejectsZeroValues(t *testing.T) {
	email, _ := NewEmail("person@example.com")
	phone, _ := NewPhone("+37123456789")
	hash, _ := NewPasswordHash("hash")
	first, _ := NewFirstName("First")
	last, _ := NewLastName("Last")

	for _, tt := range []struct {
		name  string
		email Email
		phone Phone
		hash  PasswordHash
		first FirstName
		last  LastName
		want  error
	}{
		{name: "email", phone: phone, hash: hash, first: first, last: last, want: ErrInvalidEmail},
		{name: "phone", email: email, hash: hash, first: first, last: last, want: ErrInvalidPhone},
		{name: "password hash", email: email, phone: phone, first: first, last: last, want: ErrInvalidPasswordHash},
		{name: "first name", email: email, phone: phone, hash: hash, last: last, want: ErrInvalidFirstName},
		{name: "last name", email: email, phone: phone, hash: hash, first: first, want: ErrInvalidLastName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			user, err := NewUser(UserID{}, tt.email, tt.phone, tt.hash, tt.first, tt.last)

			if user != nil || !errors.Is(err, tt.want) {
				t.Fatalf("NewUser() = (%v, %v), want (nil, %v)", user, err, tt.want)
			}
		})
	}

	user, err := NewUser(UserID{}, email, phone, hash, first, last)

	if err != nil {
		t.Fatalf("NewUser() = %v, want nil", err)
	}

	for _, tt := range []struct {
		name string
		set  func() error
		want error
	}{
		{name: "email", set: func() error { return user.SetEmail(Email("")) }, want: ErrInvalidEmail},
		{name: "phone", set: func() error { return user.SetPhone(Phone("")) }, want: ErrInvalidPhone},
		{name: "password hash", set: func() error { return user.SetPasswordHash(PasswordHash("")) }, want: ErrInvalidPasswordHash},
		{name: "first name", set: func() error { return user.SetFirstName(FirstName("")) }, want: ErrInvalidFirstName},
		{name: "last name", set: func() error { return user.SetLastName(LastName("")) }, want: ErrInvalidLastName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.set(); !errors.Is(err, tt.want) {
				t.Fatalf("setter = %v, want %v", err, tt.want)
			}
		})
	}

	if user.Email() != email || user.Phone() != phone || user.PasswordHash() != hash || user.FirstName() != first || user.LastName() != last {
		t.Fatal("invalid setters changed user")
	}

	newEmail, _ := NewEmail("new@example.com")
	newPhone, _ := NewPhone("+15551234567")
	newHash, _ := NewPasswordHash("newhash")
	newFirst, _ := NewFirstName("New")
	newLast, _ := NewLastName("Name")

	for _, set := range []func() error{
		func() error { return user.SetEmail(newEmail) },
		func() error { return user.SetPhone(newPhone) },
		func() error { return user.SetPasswordHash(newHash) },
		func() error { return user.SetFirstName(newFirst) },
		func() error { return user.SetLastName(newLast) },
	} {
		if err := set(); err != nil {
			t.Fatalf("valid setter = %v, want nil", err)
		}
	}

	if user.Email() != newEmail || user.Phone() != newPhone || user.PasswordHash() != newHash || user.FirstName() != newFirst || user.LastName() != newLast {
		t.Fatal("valid setters did not update user")
	}
}
