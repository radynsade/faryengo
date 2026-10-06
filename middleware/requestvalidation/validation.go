// Package requestvalidation implements structural validation at transport boundaries.
package requestvalidation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-playground/validator/v10"
)

var ErrInvalidRequest = errors.New("invalid request")

// FieldError contains transport names and rule information, never submitted values.
type FieldError struct {
	Field string
	Rule  string
	Param string
}

func (e FieldError) Error() string {
	message := "invalid value"

	switch e.Rule {
	case "required", "notblank":
		message = "required"
	case "email", "mailbox":
		message = "invalid email"
	case "uuid", "uuid_input":
		message = "invalid UUID"
	case "e164":
		message = "invalid phone number"
	case "max", "maxbytes":
		message = "exceeds maximum length"
	case "min":
		message = "below minimum length"
	case "oneof", "eq":
		message = "invalid choice"
	}

	return e.Field + ": " + message
}

type Errors struct {
	Fields []FieldError
}

func (e *Errors) Error() string {
	messages := make([]string, 0, len(e.Fields))

	for _, field := range e.Fields {
		messages = append(messages, field.Error())
	}

	return strings.Join(messages, "; ")
}

func (e *Errors) Unwrap() error {
	return ErrInvalidRequest
}

var shared = validator.New(validator.WithRequiredStructEnabled())

func init() {
	shared.RegisterTagNameFunc(func(field reflect.StructField) string {
		name, _, _ := strings.Cut(field.Tag.Get("form"), ",")

		if name == "" {
			name, _, _ = strings.Cut(field.Tag.Get("json"), ",")
		}

		if name == "" {
			name = field.Name
		}

		return name
	})

	for _, registration := range []struct {
		name  string
		check validator.Func
	}{
		{"notblank", func(field validator.FieldLevel) bool { return strings.TrimSpace(field.Field().String()) != "" }},
		{"utf8", func(field validator.FieldLevel) bool { return utf8.ValidString(field.Field().String()) }},
		{"maxbytes", func(field validator.FieldLevel) bool {
			limit, err := strconv.Atoi(field.Param())

			return err == nil && len(field.Field().String()) <= limit
		}},
	} {
		if err := shared.RegisterValidation(registration.name, registration.check); err != nil {
			panic(err)
		}
	}
}

// RegisterStringRule configures a format rule during package init, before
// concurrent validation. Callbacks can delegate to domain constructors when
// built-in formats differ from the project's invariants.
func RegisterStringRule(name string, check func(string) bool) error {
	return shared.RegisterValidation(name, func(field validator.FieldLevel) bool {
		return check(field.Field().String())
	})
}

// Validate converts validator errors to stable, sorted field errors. No validator
// types escape this package, including misconfigured DTO errors.
func Validate(ctx context.Context, request any) error {
	var result error
	err := shared.StructCtx(ctx, request)

	if err != nil {
		var failures validator.ValidationErrors

		if errors.As(err, &failures) {
			fields := make([]FieldError, 0, len(failures))

			for _, failure := range failures {
				_, field, _ := strings.Cut(failure.Namespace(), ".")
				fields = append(fields, FieldError{Field: field, Rule: failure.Tag(), Param: failure.Param()})
			}

			slices.SortFunc(fields, func(a, b FieldError) int { return strings.Compare(a.Field, b.Field) })
			result = &Errors{Fields: fields}
		} else {
			result = fmt.Errorf("validate request DTO: %w", ErrInvalidRequest)
		}
	}

	return result
}
