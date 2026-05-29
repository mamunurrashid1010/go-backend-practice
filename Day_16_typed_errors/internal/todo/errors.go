package todo

import (
	"errors"
	"fmt"
)

// ---- Sentinel errors (identity only, checked with errors.Is) -----------

var (
	// ErrNotFound — no record with the requested id.
	ErrNotFound = errors.New("todo not found")

	// ErrNothingToUpdate — PATCH body had no non-nil fields.
	ErrNothingToUpdate = errors.New("nothing to update")
)

// ---- Typed errors (carry data, checked with errors.As) -----------------

// ConflictError means a uniqueness constraint was violated. It carries the
// field and value that conflicted so the handler can build a useful message
// without parsing strings.
//
// Created by the repository when it sees a Postgres 23505 (unique violation),
// or by the in-memory repo on a duplicate title. Inspected by the handler
// with errors.As.
type ConflictError struct {
	Field string
	Value string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s %q already exists", e.Field, e.Value)
}
