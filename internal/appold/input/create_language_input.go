package input

import "errors"

var ErrInvalidCreateLanguageInput = errors.New("invalid create language input")

type CreateLanguageInput struct {
	Code        string
	EnglishName string
	NativeName  string
	IsFallback  bool
}
