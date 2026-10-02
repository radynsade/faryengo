package input

import (
	"errors"
	"fmt"

	"github.com/radynsade/faryengo/internal/security"
)

var ErrInvalidSignInInput = errors.New("invalid sign-in input")

type SignInInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (input SignInInput) Validate() error {
	var problems []error
	var err error

	if len(input.Email) > 254 || security.Email(input.Email).Validate() != nil {
		problems = append(problems, security.ErrInvalidEmail)
	}

	if len(input.Password) == 0 || len(input.Password) > security.MaxPasswordBytes {
		problems = append(problems, security.ErrInvalidPassword)
	}

	if len(problems) > 0 {
		err = fmt.Errorf("%w: %w", ErrInvalidSignInInput, errors.Join(problems...))
	}

	return err
}
