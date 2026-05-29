// Package validate wraps go-playground/validator so the rest of the app
// gets a clean, response-shaped API:
//
//	fields := validate.Struct(dto)   // nil if ok, []FieldError if not
//
// It is NOT HTTP-coupled — any caller (handler, CLI, test) can use it.
package validate

import (
	"errors"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// FieldError is one failed field, ready to serialize into the error envelope.
type FieldError struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// A single shared validator. It's safe for concurrent use and caches
// struct reflection, so create it once.
var v = newValidator()

func newValidator() *validator.Validate {
	val := validator.New(validator.WithRequiredStructEnabled())

	// Report the JSON field name (e.g. "title") instead of the Go field
	// name (e.g. "Title") so error details match what the client sent.
	val.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	return val
}

// Struct validates s. Returns nil when valid, otherwise one FieldError per
// failed rule.
func Struct(s any) []FieldError {
	err := v.Struct(s)
	if err == nil {
		return nil
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		out := make([]FieldError, 0, len(ve))
		for _, fe := range ve {
			out = append(out, FieldError{
				Field: fe.Field(),
				Issue: issue(fe),
			})
		}
		return out
	}

	// Non-validation error (e.g. passed a non-struct). Surface generically.
	return []FieldError{{Field: "", Issue: err.Error()}}
}

// issue turns a validator tag + param into a human-readable message.
// Add cases here as you adopt new tags.
func issue(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "min":
		return "must be at least " + fe.Param() + " characters"
	case "max":
		return "must be at most " + fe.Param() + " characters"
	case "len":
		return "must be exactly " + fe.Param() + " characters"
	case "email":
		return "must be a valid email"
	case "url":
		return "must be a valid URL"
	case "oneof":
		return "must be one of: " + fe.Param()
	case "gte":
		return "must be >= " + fe.Param()
	case "lte":
		return "must be <= " + fe.Param()
	case "uuid":
		return "must be a valid UUID"
	case "eqfield":
		return "must match " + fe.Param()
	default:
		return "is invalid"
	}
}
