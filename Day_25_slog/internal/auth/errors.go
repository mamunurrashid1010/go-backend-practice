package auth

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")
)

type ConflictError struct {
	Field string
	Value string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s %q already exists", e.Field, e.Value)
}
