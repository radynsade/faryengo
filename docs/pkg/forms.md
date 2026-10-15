# forms

`pkg/forms` reads URL-encoded form bodies and rejects malformed ones before
any field is validated. It knows nothing about the fields' meaning; callers
validate the values themselves.

Import `github.com/radynsade/faryengo/pkg/forms`.

## Functions

| Function | Purpose |
| --- | --- |
| `Parse(writer, request, maxBytes)` | Reads a form body of at most `maxBytes` into `request.PostForm`. |
| `CheckFields(values, single, multiple)` | Accepts only the listed fields: each `single` field at most once, each `multiple` field any number of times. |

`Parse` requires the `application/x-www-form-urlencoded` content type and
limits the body with `http.MaxBytesReader`, so an oversized body fails instead
of being read in full.

Both functions report a malformed form as `ErrInvalid`, wrapping the parse
failure when there is one. Compare with `errors.Is`.

## Usage conventions

A malformed form, one with the wrong content type, an oversized body, an
unknown field, or a repeated single field, is not a user mistake: answer it
with a general error rather than field errors. Choose `maxBytes` per form,
large enough for its longest valid submission, and call `CheckFields` with
every field the form may send.
