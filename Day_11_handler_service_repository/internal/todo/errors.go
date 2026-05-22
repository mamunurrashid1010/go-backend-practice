package todo

import "errors"

// Domain errors. Sentinel values — callers check with errors.Is.
//
// The handler maps these to HTTP status codes at the edge; the service
// returns them directly; the repository returns ErrNotFound and lets the
// service decide what to do.
//
// Day 16 will formalise this — typed error structs with extra fields,
// a single "to HTTP" mapping function.
var (
	// ErrNotFound — no record with the requested ID.
	ErrNotFound = errors.New("todo not found")

	// ErrEmptyTitle — Create or Update sent an empty title.
	ErrEmptyTitle = errors.New("title is required")

	// ErrNothingToUpdate — PATCH body had no non-nil fields.
	ErrNothingToUpdate = errors.New("nothing to update")
)
