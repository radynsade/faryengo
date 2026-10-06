package requestvalidation

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestFieldErrors(t *testing.T) {
	type dto struct {
		ID       string  `json:"id" validate:"required,uuid"`
		Email    string  `form:"email" validate:"required,email"`
		Password *string `form:"password" validate:"omitempty,min=1,maxbytes=8"`
		Name     string  `form:"first_name" validate:"required,notblank,max=3"`
	}

	valid := dto{ID: "00000000-0000-0000-0000-000000000001", Email: "person@example.com", Name: "āāā"}
	for _, tt := range []struct {
		name   string
		change func(*dto)
		fields []FieldError
	}{
		{name: "optional password and Unicode length"},
		{name: "required", change: func(d *dto) { d.ID, d.Email = "", "" }, fields: []FieldError{{Field: "email", Rule: "required"}, {Field: "id", Rule: "required"}}},
		{name: "email", change: func(d *dto) { d.Email = "secret invalid email" }, fields: []FieldError{{Field: "email", Rule: "email"}}},
		{name: "UUID", change: func(d *dto) { d.ID = "malformed" }, fields: []FieldError{{Field: "id", Rule: "uuid"}}},
		{name: "name length", change: func(d *dto) { d.Name = "āāāā" }, fields: []FieldError{{Field: "first_name", Rule: "max", Param: "3"}}},
		{name: "blank name", change: func(d *dto) { d.Name = "   " }, fields: []FieldError{{Field: "first_name", Rule: "notblank"}}},
		{name: "empty password pointer", change: func(d *dto) { d.Password = new("") }, fields: []FieldError{{Field: "password", Rule: "min", Param: "1"}}},
		{name: "byte limit", change: func(d *dto) { d.Password = new("āāāāā") }, fields: []FieldError{{Field: "password", Rule: "maxbytes", Param: "8"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request := valid

			if tt.change != nil {
				tt.change(&request)
			}

			err := Validate(t.Context(), request)

			if len(tt.fields) == 0 {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				wrapped := fmt.Errorf("decode form: %w", err)
				var fields *Errors

				if !errors.Is(wrapped, ErrInvalidRequest) || !errors.As(wrapped, &fields) || !reflect.DeepEqual(fields.Fields, tt.fields) {
					t.Fatalf("validation = %v, want %v", wrapped, tt.fields)
				}

				if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "āāāāā") {
					t.Fatal("validation error exposed a submitted value")
				}
			}
		})
	}
}

func TestNestedFieldNames(t *testing.T) {
	type dto struct {
		Name map[string]string `form:"name" validate:"dive,required"`
	}

	err := Validate(t.Context(), dto{Name: map[string]string{"lv": "", "en": ""}})
	var fields *Errors

	if !errors.As(err, &fields) || len(fields.Fields) != 2 || fields.Fields[0].Field != "name[en]" || fields.Fields[1].Field != "name[lv]" {
		t.Fatalf("field errors = %v", err)
	}
}

func TestInvalidDTO(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
	}{{name: "nil"}, {name: "primitive", value: "value"}} {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(t.Context(), tt.value); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("Validate() = %v", err)
			}
		})
	}
}
