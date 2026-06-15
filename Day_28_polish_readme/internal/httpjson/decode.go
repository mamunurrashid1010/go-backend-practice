// Package httpjson holds the small JSON helpers every handler needs.
//
// Before Day 28 the auth and notes handlers each had their own copy of
// DecodeJSON and decodeErrorMessage — same code, same MaxBytes limit,
// same error mapping. Three is "extract it" territory; two was already.
package httpjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"day28/internal/respond"
)

// MaxBody is the largest request body we'll attempt to decode. Anything
// larger fails fast with 400.
const MaxBody = 1 << 20 // 1 MiB

// DecodeJSON reads a single JSON object from r.Body into v. On success
// it returns true; on failure it has already written the response and
// the caller should return.
//
// Guarantees:
//   - rejects non-JSON Content-Type (415)
//   - bounds the body to MaxBody (400 on overflow)
//   - DisallowUnknownFields — typos in the request fail loudly
//   - dec.More() — a trailing object after the first one is rejected
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if ct := r.Header.Get("Content-Type"); ct != "" && ct != "application/json" {
		respond.UnsupportedMediaType(w, "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		respond.BadRequest(w, errorMessage(err))
		return false
	}
	if dec.More() {
		respond.BadRequest(w, "body must contain a single JSON object")
		return false
	}
	return true
}

func errorMessage(err error) string {
	var (
		syntaxErr    *json.SyntaxError
		unmarshalErr *json.UnmarshalTypeError
		maxBytesErr  *http.MaxBytesError
	)
	switch {
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("malformed JSON at byte %d", syntaxErr.Offset)
	case errors.As(err, &unmarshalErr):
		return fmt.Sprintf("field %q has wrong type", unmarshalErr.Field)
	case errors.As(err, &maxBytesErr):
		return "request body too large"
	default:
		return "invalid JSON request body"
	}
}
