package currencies

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

//
// ISO 4217 Code
//

const CodeLengthChars = 3

var (
	ErrCodeInvalid      = errors.New("invalid currency code")
	ErrCodeInvalidChars = errors.New("invalid characters")
	ErrCodeWrongLength  = fmt.Errorf("is not %d characters long", CodeLengthChars)
	codePattern         = regexp.MustCompile(`^[A-Z]*$`)
)

type Code string

func NewCode(value string) (Code, error) {
	var (
		err  error
		code Code
	)

	code = Code(value)
	err = code.Validate()
	return code, err
}

func (c Code) Validate() error {
	var err error

	if !utf8.ValidString(string(c)) || !codePattern.MatchString(string(c)) {
		err = ErrCodeInvalidChars
	} else if utf8.RuneCountInString(string(c)) != CodeLengthChars {
		err = ErrCodeWrongLength
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrCodeInvalid, err)
	}

	return err
}

//
// Symbol
//

// A symbol is either a single sign, such as "$" or "€", or a short notation of
// several characters, such as "R$", "kr", "zł", "S/." or "د.إ". It consists of
// letters, combining marks, currency signs and punctuation, contains no
// whitespace, and has at least one letter or currency sign. A combining mark
// cannot start a symbol, since it has no character to attach to.

const MaxSymbolLengthChars = 5

var (
	ErrSymbolInvalid      = errors.New("invalid currency symbol")
	ErrSymbolEmpty        = errors.New("is empty")
	ErrSymbolInvalidChars = errors.New("invalid characters")
	ErrSymbolNoSign       = errors.New("has no letter or currency sign")
	ErrSymbolTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxSymbolLengthChars)
)

type Symbol string

func NewSymbol(value string) (Symbol, error) {
	var (
		err    error
		symbol Symbol
	)

	symbol = Symbol(value)
	err = symbol.Validate()
	return symbol, err
}

func (s Symbol) Validate() error {
	var err error

	if strings.TrimSpace(string(s)) == "" {
		err = ErrSymbolEmpty
	} else if !utf8.ValidString(string(s)) || !s.hasValidChars() {
		err = ErrSymbolInvalidChars
	} else if !s.hasSign() {
		err = ErrSymbolNoSign
	} else if utf8.RuneCountInString(string(s)) > MaxSymbolLengthChars {
		err = ErrSymbolTooLong
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrSymbolInvalid, err)
	}

	return err
}

func (s Symbol) hasValidChars() bool {
	var (
		isValid bool
		first   rune
	)

	first, _ = utf8.DecodeRuneInString(string(s))
	isValid = !unicode.IsMark(first)

	for _, char := range string(s) {
		if !isValid {
			break
		}

		isValid = unicode.IsLetter(char) ||
			unicode.IsMark(char) ||
			unicode.Is(unicode.Sc, char) ||
			unicode.IsPunct(char)
	}

	return isValid
}

func (s Symbol) hasSign() bool {
	return strings.IndexFunc(string(s), func(char rune) bool {
		return unicode.IsLetter(char) || unicode.Is(unicode.Sc, char)
	}) >= 0
}

//
// Currency
//

var (
	ErrCurrencyInvalid = errors.New("invalid currency")
	ErrCurrencyNil     = errors.New("is nil")
)

type Currency struct {
	Code   Code
	Symbol Symbol
}

func NewCurrency(code Code, symbol Symbol) *Currency {
	return &Currency{
		Code:   code,
		Symbol: symbol,
	}
}

func (c *Currency) Validate() error {
	var err error

	if c == nil {
		err = ErrCurrencyNil
	} else {
		err = errors.Join(
			c.Code.Validate(),
			c.Symbol.Validate(),
		)
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrCurrencyInvalid, err)
	}

	return err
}
