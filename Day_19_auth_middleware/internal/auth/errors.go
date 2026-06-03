package auth

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound           = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

type ConflictError struct {
	Field string
	Value string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s %q already exists", e.Field, e.Value)
}
