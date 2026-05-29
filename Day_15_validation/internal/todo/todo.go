// Package todo holds the To-Do feature.
//
// Day 15: DTOs gained `validate:"..."` tags. The validator runs at the
// handler boundary (see handler.go) before the service is called.
package todo

import "time"

type Todo struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateRequest — POST body. title is required and capped at 200 chars.
type CreateRequest struct {
	Title string `json:"title" validate:"required,max=200"`
	Done  bool   `json:"done"`
}

// UpdateRequest — PUT body (full replacement). Same rules as create.
type UpdateRequest struct {
	Title string `json:"title" validate:"required,max=200"`
	Done  bool   `json:"done"`
}

// PatchRequest — PATCH body (partial). Pointer fields + omitempty:
//   - nil           → field not provided, skip all rules
//   - non-nil       → validate the dereferenced value (min=1, max=200)
type PatchRequest struct {
	Title *string `json:"title,omitempty" validate:"omitempty,min=1,max=200"`
	Done  *bool   `json:"done,omitempty"`
}

type ListFilter struct {
	Done   *bool
	Search string
	Limit  int
}
