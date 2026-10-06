package input

import "errors"

var ErrInvalidSignInInput = errors.New("invalid sign-in input")

type SignInInput struct {
	Email    string
	Password string
}
