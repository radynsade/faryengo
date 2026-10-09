package utils

import (
	"context"
	"errors"
	"net/http"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Authentication
//

var ErrUnauthenticated = errors.New("not authenticated")

// A missing, invalid, revoked, or expired Session, and a deleted User, mean
// the visitor is not signed in. Any other failure is operational.

func IsUnauthenticated(err error) bool {
	return errors.Is(err, ErrUnauthenticated) ||
		errors.Is(err, security.ErrCredentialsInvalid) ||
		errors.Is(err, security.ErrSessionInvalid) ||
		errors.Is(err, security.ErrSessionRevoked) ||
		errors.Is(err, users.ErrUserNotFound)
}

//
// Request state
//

// Request state is filled in while a request is handled: the identity once
// authentication succeeds, and the guest flash session once it is known.

type stateKey struct{}

type State struct {
	Identity *security.Identity
	GuestID  string
}

func WithState(ctx context.Context) context.Context {
	return context.WithValue(ctx, stateKey{}, &State{})
}

func StateOf(request *http.Request) *State {
	state, found := request.Context().Value(stateKey{}).(*State)

	if !found {
		state = &State{}
	}

	return state
}
