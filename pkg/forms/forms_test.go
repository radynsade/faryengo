package forms

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		maxBytes    int64
		want        error
	}{
		{"form", "application/x-www-form-urlencoded", "email=a%40b.c", 64, nil},
		{"form with charset", "application/x-www-form-urlencoded; charset=utf-8", "email=x", 64, nil},
		{"wrong content type", "multipart/form-data", "email=x", 64, ErrInvalid},
		{"missing content type", "", "email=x", 64, ErrInvalid},
		{"oversized body", "application/x-www-form-urlencoded", "email=" + strings.Repeat("x", 64), 16, ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)

			err := Parse(httptest.NewRecorder(), request, test.maxBytes)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if test.want == nil && request.PostForm.Get("email") == "" {
				t.Fatalf("got form %v, want the email field", request.PostForm)
			}
		})
	}
}

func TestCheckFields(t *testing.T) {
	tests := []struct {
		name   string
		values url.Values
		want   error
	}{
		{"allowed fields", url.Values{"name": {"a"}, "tags": {"x", "y"}}, nil},
		{"no fields", url.Values{}, nil},
		{"unknown field", url.Values{"name": {"a"}, "extra": {"1"}}, ErrInvalid},
		{"repeated single field", url.Values{"name": {"a", "b"}}, ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := CheckFields(test.values, []string{"name"}, []string{"tags"}); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}
