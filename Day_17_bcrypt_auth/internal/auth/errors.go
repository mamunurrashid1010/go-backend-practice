package auth

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound — no user with the given id/email. Stays internal to the
	// service: login translates it into ErrInvalidCredentials so the API
	// never reveals whether an email exists.
	ErrNotFound = errors.New("user not found")

	// ErrInvalidCredentials — the public, deliberately-vague login failure.
	// Returned for BOTH "no such email" and "wrong password" so an attacker
	// can't enumerate registered emails.
	ErrInvalidCredentials = errors.New("invalid email or password")
)

// ConflictError — same typed error pattern as Day 16. Returned when the
// email is already registered (Postgres 23505 on the unique constraint).
type ConflictError struct {
	Field string
	Value string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s %q already exists", e.Field, e.Value)
}
