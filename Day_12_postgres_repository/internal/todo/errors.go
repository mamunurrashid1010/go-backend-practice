package todo

import "errors"

// Domain errors — UNCHANGED from Day 11. Both repositories return these.
var (
	ErrNotFound        = errors.New("todo not found")
	ErrEmptyTitle      = errors.New("title is required")
	ErrNothingToUpdate = errors.New("nothing to update")
)
