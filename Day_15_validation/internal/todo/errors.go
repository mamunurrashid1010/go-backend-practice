package todo

import "errors"

// Domain errors. Day 15 dropped ErrEmptyTitle — "title is required" is now
// enforced by the validator at the boundary, not a business rule here.
//
// What remains are rules the validator CAN'T express:
//   - ErrNotFound        — needs the storage layer to know the id is missing
//   - ErrNothingToUpdate — a cross-field rule (all PATCH fields nil)
var (
	ErrNotFound        = errors.New("todo not found")
	ErrNothingToUpdate = errors.New("nothing to update")
)
