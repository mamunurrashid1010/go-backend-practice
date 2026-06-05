package notes

import "errors"

// ErrNotFound — note doesn't exist OR isn't owned by the requesting user.
// The two cases collapse to the same response on purpose: revealing "the
// note exists but you can't see it" leaks information.
var (
	ErrNotFound        = errors.New("note not found")
	ErrNothingToUpdate = errors.New("nothing to update")
)
