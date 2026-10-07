package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
)

func TestUserServiceRejectsDomainPrimitivesWithoutTransport(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*input.CreateUserInput)
		want   error
	}{
		{name: "email", change: func(r *input.CreateUserInput) { r.Email = "not-an-email" }, want: security.ErrInvalidEmail},
		{name: "phone", change: func(r *input.CreateUserInput) { r.Phone = "37123456789" }, want: security.ErrInvalidPhone},
		{name: "first name", change: func(r *input.CreateUserInput) { r.FirstName = " " }, want: security.ErrInvalidFirstName},
		{name: "last name", change: func(r *input.CreateUserInput) { r.LastName = strings.Repeat("ā", 101) }, want: security.ErrInvalidLastName},
		{name: "role ID", change: func(r *input.CreateUserInput) { r.RoleID = security.RoleID{} }, want: security.ErrInvalidRoleID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stored := validStoredUser(t)
			store := &fakeUserStore{stored: stored}
			hasher := &fakePasswordHasher{hash: "hash"}
			service, err := NewUserService(store, hasher)

			if err != nil {
				t.Fatal(err)
			}

			request := validCreateUserInput()
			tt.change(&request)
			created, createErr := service.Create(t.Context(), request)
			updated, updateErr := service.Update(t.Context(), input.UpdateUserInput{
				ID: stored.ID(), RoleID: request.RoleID, Email: request.Email, Phone: request.Phone,
				FirstName: request.FirstName, LastName: request.LastName,
			})

			if created != nil || updated != nil || !errors.Is(createErr, tt.want) || !errors.Is(updateErr, tt.want) {
				t.Fatalf("Create/Update errors = %v / %v, want %v", createErr, updateErr, tt.want)
			}

			if store.findCalls != 0 || store.createCalls != 0 || store.updateCalls != 0 || hasher.calls != 0 || stored.Email() != "old@example.com" {
				t.Fatal("invalid input accessed persistence or changed loaded state")
			}
		})
	}
}
