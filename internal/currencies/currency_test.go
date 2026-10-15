package currencies

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// The constructor keeps the value as given, even when it is invalid.

func TestNewCode(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  error
	}{
		{"valid", "USD", nil},
		{"lower case", "usd", ErrCodeInvalidChars},
		{"too short", "US", ErrCodeWrongLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := NewCode(tt.value)

			if code != Code(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewCode(%q) = (%q, %v), want %v", tt.value, code, err, tt.want)
			}
		})
	}
}

func TestCodeValidate(t *testing.T) {
	tests := []struct {
		name string
		code Code
		want error
	}{
		{"valid", "EUR", nil},
		{"empty", "", ErrCodeWrongLength},
		{"too short", "EU", ErrCodeWrongLength},
		{"too long", "EURO", ErrCodeWrongLength},
		{"lower case", "eur", ErrCodeInvalidChars},
		{"digit", "EU1", ErrCodeInvalidChars},
		{"non-ASCII letter", "EÜR", ErrCodeInvalidChars},
		{"invalid UTF-8", "EU\xff", ErrCodeInvalidChars},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.code.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Code(%q).Validate() = %v, want %v", tt.code, err, tt.want)
			}

			if err != nil && !errors.Is(err, ErrCodeInvalid) {
				t.Fatalf("Code(%q).Validate() = %v, want it to wrap %v", tt.code, err, ErrCodeInvalid)
			}
		})
	}
}

// The constructor keeps the value as given, even when it is invalid.

func TestNewSymbol(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  error
	}{
		{"valid", "$", nil},
		{"only whitespace", " \t ", ErrSymbolEmpty},
		{"too long", strings.Repeat("$", MaxSymbolLengthChars+1), ErrSymbolTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			symbol, err := NewSymbol(tt.value)

			if symbol != Symbol(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewSymbol(%q) = (%q, %v), want %v", tt.value, symbol, err, tt.want)
			}
		})
	}
}

func TestSymbolValidate(t *testing.T) {
	tests := []struct {
		name   string
		symbol Symbol
		want   error
	}{
		{"dollar sign", "$", nil},
		{"euro sign", "€", nil},
		{"bitcoin sign", "₿", nil},
		{"letter and sign", "R$", nil},
		{"letters", "kr", nil},
		{"letters with diacritic", "zł", nil},
		{"cyrillic letters", "лв", nil},
		{"letters and punctuation", "S/.", nil},
		{"arabic letters and punctuation", "د.إ", nil},
		{"devanagari letters with marks", "रु", nil},
		{"letters and sign", "NT$", nil},
		{"limit in characters", "Bs.F.", nil},
		{"empty", "", ErrSymbolEmpty},
		{"only whitespace", " ", ErrSymbolEmpty},
		{"invalid UTF-8", "\xff", ErrSymbolInvalidChars},
		{"inner space", "R $", ErrSymbolInvalidChars},
		{"surrounding space", " $", ErrSymbolInvalidChars},
		{"control character", "$\n", ErrSymbolInvalidChars},
		{"digit", "$1", ErrSymbolInvalidChars},
		{"math symbol", "+", ErrSymbolInvalidChars},
		{"leading combining mark", "́a", ErrSymbolInvalidChars},
		{"only punctuation", "./", ErrSymbolNoSign},
		{"too long", "Bs.F.S", ErrSymbolTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.symbol.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Symbol(%q).Validate() = %v, want %v", tt.symbol, err, tt.want)
			}

			if err != nil && !errors.Is(err, ErrSymbolInvalid) {
				t.Fatalf("Symbol(%q).Validate() = %v, want it to wrap %v", tt.symbol, err, ErrSymbolInvalid)
			}
		})
	}
}

func TestCurrencyValidate(t *testing.T) {
	tests := []struct {
		name   string
		code   Code
		symbol Symbol
		want   []error
	}{
		{"valid", "USD", "$", nil},
		{"multi-character symbol", "BRL", "R$", nil},
		{"invalid code", "usd", "$", []error{ErrCodeInvalid}},
		{"invalid symbol", "USD", "", []error{ErrSymbolInvalid}},
		{"invalid code and symbol", "US", " ", []error{ErrCodeInvalid, ErrSymbolInvalid}},
	}

	if err := (*Currency)(nil).Validate(); !errors.Is(err, ErrCurrencyNil) || !errors.Is(err, ErrCurrencyInvalid) {
		t.Fatalf("Validate() on nil = %v, want %v wrapped in %v", err, ErrCurrencyNil, ErrCurrencyInvalid)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currency := NewCurrency(tt.code, tt.symbol)

			if currency.Code != tt.code || currency.Symbol != tt.symbol {
				t.Fatalf("NewCurrency() = %+v, want the given code and symbol", currency)
			}

			err := currency.Validate()

			if (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if err != nil && !errors.Is(err, ErrCurrencyInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrCurrencyInvalid)
			}

			for _, want := range tt.want {
				if !errors.Is(err, want) {
					t.Fatalf("Validate() = %v, want it to include %v", err, want)
				}
			}
		})
	}
}
