package input

import "errors"

//
// Create currency input
//

var ErrCreateCurrencyInputInvalid = errors.New("invalid create currency input")

type CreateCurrencyInput struct {
	Code   string
	Symbol string
}

//
// Update currency input
//

var ErrUpdateCurrencyInputInvalid = errors.New("invalid update currency input")

type UpdateCurrencyInput struct {
	Code   string
	Symbol string
}
