package forms

import (
	"errors"
	"fmt"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"slices"
)

//
// Form bodies
//

// A form with an unexpected content type, an oversized body, an unknown field
// or a repeated single-valued field is malformed rather than invalid, so it
// is rejected without field errors.

var ErrInvalid = errors.New("invalid form")

// Parse reads a URL-encoded form body of at most maxBytes into
// request.PostForm.

func Parse(writer http.ResponseWriter, request *http.Request, maxBytes int64) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))

	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		err = ErrInvalid
	} else {
		request.Body = http.MaxBytesReader(writer, request.Body, maxBytes)

		if parseErr := request.ParseForm(); parseErr != nil {
			err = fmt.Errorf("%w: %w", ErrInvalid, parseErr)
		}
	}

	return err
}

// CheckFields accepts only the given fields: each single field at most once,
// and each multiple field any number of times.

func CheckFields(values url.Values, single []string, multiple []string) error {
	var err error

	keys := slices.Sorted(maps.Keys(values))

	for index := 0; err == nil && index < len(keys); index++ {
		key := keys[index]

		if !slices.Contains(single, key) && !slices.Contains(multiple, key) {
			err = ErrInvalid
		} else if slices.Contains(single, key) && len(values[key]) > 1 {
			err = ErrInvalid
		}
	}

	return err
}
