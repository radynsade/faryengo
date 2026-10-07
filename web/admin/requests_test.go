package admin

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
)

func TestRequestDTOs(t *testing.T) {
	for _, tt := range []struct {
		name    string
		request any
		field   string
	}{
		{name: "valid sign in", request: signInRequest{Email: "person@example.com", Password: "secret"}},
		{name: "domain mailbox format", request: signInRequest{Email: "person@localhost", Password: "secret"}},
		{name: "missing email", request: signInRequest{Password: "secret"}, field: "email"},
		{name: "malformed email", request: signInRequest{Email: "Person <person@example.com>", Password: "secret"}, field: "email"},
		{name: "missing password", request: signInRequest{Email: "person@example.com"}, field: "password"},
		{name: "Unicode password bytes", request: signInRequest{Email: "person@example.com", Password: strings.Repeat("ā", security.MaxPasswordBytes/2+1)}, field: "password"},
		{name: "valid role", request: roleFormRequest{Name: map[string]string{"en": "Role"}}},
		{name: "missing name", request: roleFormRequest{}, field: "name"},
		{name: "malformed language", request: roleFormRequest{Name: map[string]string{"EN": "Role"}}, field: "name[EN]"},
		{name: "blank translation", request: roleFormRequest{Name: map[string]string{"en": " "}}, field: "name[en]"},
		{name: "invalid UTF8", request: roleFormRequest{Name: map[string]string{"en": "\xff"}}, field: "name[en]"},
		{name: "unknown permission", request: roleFormRequest{Name: map[string]string{"en": "Role"}, Permissions: []string{"unknown"}}, field: "permissions[0]"},
		{name: "malformed UUID", request: roleIDRequest{ID: "bad"}, field: "role"},
		{name: "missing UUID", request: roleIDRequest{}, field: "role"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := requestvalidation.Validate(t.Context(), tt.request)
			var fields *requestvalidation.Errors

			if tt.field == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.As(err, &fields) || len(fields.Fields) != 1 || fields.Fields[0].Field != tt.field {
				t.Fatalf("validation = %v, want field %q", err, tt.field)
			}
		})
	}
}

func TestLocalizedFieldErrors(t *testing.T) {
	for _, tt := range []struct {
		language string
		want     string
	}{{"en", "at least one language"}, {"lv", "vismaz vienā valodā"}, {"ru", "хотя бы на одном языке"}} {
		t.Run(tt.language, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/admin/"+tt.language+"/roles", nil)
			request.SetPathValue("language", tt.language)
			err := requestvalidation.Validate(t.Context(), roleFormRequest{})
			fields := requestFieldErrors(admini18n.WithRequest(request), err)
			message := strings.Join(fields["name"], " ")

			if len(fields) != 1 || !strings.Contains(message, tt.want) {
				t.Fatalf("localized validation = %q", message)
			}
		})
	}
}

func TestRoleIDTransportEncodings(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		valid bool
	}{
		{name: "canonical", value: "8e35a76b-cc06-4b5b-8d8c-2c4d9144ff63", valid: true},
		{name: "uppercase", value: "8E35A76B-CC06-4B5B-8D8C-2C4D9144FF63", valid: true},
		{name: "compact", value: "8e35a76bcc064b5b8d8c2c4d9144ff63", valid: true},
		{name: "URN", value: "urn:uuid:8e35a76b-cc06-4b5b-8d8c-2c4d9144ff63", valid: true},
		{name: "malformed", value: "bad"},
		{name: "zero", value: "00000000-0000-0000-0000-000000000000"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/admin/en/roles/id/view", nil)
			request.SetPathValue("role", tt.value)
			id, err := roleID(request)

			if tt.valid {
				if err != nil || id.Validate() != nil {
					t.Fatalf("roleID() = %v, %v", id, err)
				}
			} else if !errors.Is(err, security.ErrRoleIDInvalid) || id != (security.RoleID{}) {
				t.Fatalf("invalid roleID() = %v, %v", id, err)
			}
		})
	}
}
