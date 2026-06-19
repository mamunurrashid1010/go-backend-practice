package notes

import "errors"

var (
	ErrNotFound        = errors.New("note not found")
	ErrNothingToUpdate = errors.New("nothing to update")
	ErrInvalidCursor   = errors.New("invalid cursor")
)
