package input

import "errors"

//
// Create language input
//

var ErrCreateLanguageInputInvalid = errors.New("invalid create language input")

type CreateLanguageInput struct {
	Code        string
	EnglishName string
	NativeName  string
	IsFallback  bool
}

//
// Update language input
//

var ErrUpdateLanguageInputInvalid = errors.New("invalid update language input")

type UpdateLanguageInput struct {
	Code        string
	EnglishName string
	NativeName  string
	IsFallback  bool
}
