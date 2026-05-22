// Package todo is the entire To-Do feature, split into four files:
//
//   - todo.go        : domain model + request DTOs
//   - errors.go      : typed domain errors (Day 16 expands these)
//   - repository.go  : data access — Repository interface + InMemoryRepository
//   - service.go     : business rules — Service struct (depends on Repository)
//   - handler.go     : HTTP transport — Handler struct (depends on *Service)
//
// API behaviour is identical to Day 7. Only the internal structure changes.
package todo

import "time"

// Todo — the domain model.
type Todo struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateRequest — POST body.
type CreateRequest struct {
	Title string `json:"title"`
	Done  bool   `json:"done,omitempty"`
}

// UpdateRequest — PUT body (full replacement).
type UpdateRequest struct {
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// PatchRequest — PATCH body. Pointer fields distinguish "not provided" (nil)
// from "set to zero" (non-nil pointing at "" or false).
type PatchRequest struct {
	Title *string `json:"title,omitempty"`
	Done  *bool   `json:"done,omitempty"`
}

// ListFilter — narrows GET /todos.
type ListFilter struct {
	Done   *bool
	Search string
	Limit  int
}
