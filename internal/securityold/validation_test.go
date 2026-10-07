package security

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIDConstructors(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		valid bool
	}{
		{name: "existing v4", value: "8e35a76b-cc06-4b5b-8d8c-2c4d9144ff63", valid: true},
		{name: "v7", value: "019a04cf-2200-7000-8000-000000000001", valid: true},
		{name: "empty"},
		{name: "malformed", value: "invalid"},
		{name: "zero", value: uuid.Nil.String()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			userID, userErr := NewUserID(tt.value)
			roleID, roleErr := NewRoleID(tt.value)

			if tt.valid {
				if userErr != nil || roleErr != nil || userID.Validate() != nil || roleID.Validate() != nil {
					t.Fatalf("ID construction = %v, %v", userErr, roleErr)
				}
			} else if !errors.Is(userErr, ErrInvalidUserID) || !errors.Is(roleErr, ErrInvalidRoleID) || userID != (UserID{}) || roleID != (RoleID{}) {
				t.Fatalf("invalid IDs = %v, %v, %v, %v", userID, userErr, roleID, roleErr)
			}
		})
	}
}

func TestUserIdentityCannotBeCleared(t *testing.T) {
	user, err := NewUser(UserID{}, RoleID{1}, "person@example.com", "+37123456789", "hash", "First", "Last")

	if user != nil || !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("zero identity accepted: %v, %v", user, err)
	}

	user, err = NewUser(UserID{1}, RoleID{1}, "person@example.com", "+37123456789", "hash", "First", "Last")

	if err != nil {
		t.Fatal(err)
	}

	if err := user.SetID(UserID{}); !errors.Is(err, ErrInvalidUserID) || user.ID() != (UserID{1}) {
		t.Fatalf("invalid identity change = %v, ID = %v", err, user.ID())
	}
}

func TestPasswordConstructors(t *testing.T) {
	for _, tt := range []struct {
		name                             string
		value                            string
		passwordValid, registrationValid bool
	}{
		{name: "empty"},
		{name: "replacement and candidate", value: "x", passwordValid: true},
		{name: "spaces retain existing credential policy", value: "        ", passwordValid: true},
		{name: "minimum Unicode characters", value: "āāāāāāāā", passwordValid: true, registrationValid: true},
		{name: "short Unicode", value: "āāāāāāā", passwordValid: true},
		{name: "maximum bytes", value: strings.Repeat("x", MaxPasswordBytes), passwordValid: true, registrationValid: true},
		{name: "too many bytes", value: strings.Repeat("ā", MaxPasswordBytes/2+1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, constructor := range []struct {
				name   string
				create func(string) (Password, error)
				valid  bool
			}{{"password", NewPassword, tt.passwordValid}, {"registration", NewRegistrationPassword, tt.registrationValid}} {
				t.Run(constructor.name, func(t *testing.T) {
					value, err := constructor.create(tt.value)

					if constructor.valid {
						if err != nil || value.Value() != tt.value {
							t.Fatalf("password = %v, err = %v", value != (Password{}), err)
						}
					} else if !errors.Is(err, ErrInvalidPassword) || value != (Password{}) {
						t.Fatalf("invalid password = %v, err = %v", value != (Password{}), err)
					}
				})
			}
		})
	}
}

func TestSessionInvariants(t *testing.T) {
	valid := Session{ID: uuid.UUID{1}, UserID: UserID{2}, CredentialVersion: uuid.UUID{3}, ExpiresAt: time.Now().Add(time.Hour)}

	for _, tt := range []struct {
		name   string
		change func(*Session)
	}{
		{name: "valid"},
		{name: "device identity", change: func(s *Session) { s.ID = uuid.Nil }},
		{name: "user identity", change: func(s *Session) { s.UserID = UserID{} }},
		{name: "credential version", change: func(s *Session) { s.CredentialVersion = uuid.Nil }},
		{name: "expiration", change: func(s *Session) { s.ExpiresAt = time.Time{} }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session := valid
			var want error

			if tt.change != nil {
				tt.change(&session)
				want = ErrInvalidSession
			}

			if err := session.Validate(); !errors.Is(err, want) {
				t.Fatalf("Session.Validate() = %v, want %v", err, want)
			}
		})
	}
}
