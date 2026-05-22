package todo

import "errors"

var (
	ErrNotFound        = errors.New("todo not found")
	ErrEmptyTitle      = errors.New("title is required")
	ErrNothingToUpdate = errors.New("nothing to update")
)
