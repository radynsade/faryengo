package app

import (
	"errors"
	"fmt"

	"github.com/radynsade/faryengo/internal/security"
)

type userValues struct {
	email     security.Email
	phone     security.Phone
	firstName security.FirstName
	lastName  security.LastName
}

func userValuesFromInput(roleID security.RoleID, email, phone, firstName, lastName string) (userValues, error) {
	var values userValues
	var problems []error
	var err error

	if err = roleID.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("role ID: %w", err))
	}

	values.email, err = security.NewEmail(email)

	if err != nil {
		problems = append(problems, fmt.Errorf("email: %w", err))
	}

	values.phone, err = security.NewPhone(phone)

	if err != nil {
		problems = append(problems, fmt.Errorf("phone: %w", err))
	}

	values.firstName, err = security.NewFirstName(firstName)

	if err != nil {
		problems = append(problems, fmt.Errorf("first name: %w", err))
	}

	values.lastName, err = security.NewLastName(lastName)

	if err != nil {
		problems = append(problems, fmt.Errorf("last name: %w", err))
	}

	err = errors.Join(problems...)

	if err != nil {
		values = userValues{}
	}

	return values, err
}
