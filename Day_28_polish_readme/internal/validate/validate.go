// Package validate — thin wrapper around go-playground/validator with
// the JSON tag exposed as the field name in errors.
package validate

import (
	"errors"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

type FieldError struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

var v = newValidator()

func newValidator() *validator.Validate {
	val := validator.New(validator.WithRequiredStructEnabled())
	val.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return val
}

func Struct(s any) []FieldError {
	err := v.Struct(s)
	if err == nil {
		return nil
	}
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		out := make([]FieldError, 0, len(ve))
		for _, fe := range ve {
			out = append(out, FieldError{Field: fe.Field(), Issue: issue(fe)})
		}
		return out
	}
	return []FieldError{{Field: "", Issue: err.Error()}}
}

func issue(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "min":
		return "must be at least " + fe.Param() + " characters"
	case "max":
		return "must be at most " + fe.Param() + " characters"
	case "email":
		return "must be a valid email"
	default:
		return "is invalid"
	}
}
