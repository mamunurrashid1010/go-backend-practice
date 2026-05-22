// Package todo — same domain model as Day 11. UNCHANGED.
//
//   - todo.go                : domain model + DTOs
//   - errors.go              : typed domain errors
//   - repository.go          : Repository interface + InMemoryRepository
//   - postgres_repository.go : Postgres-backed Repository (NEW today)
//   - service.go             : business rules
//   - handler.go             : HTTP transport
package todo

import "time"

type Todo struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateRequest struct {
	Title string `json:"title"`
	Done  bool   `json:"done,omitempty"`
}

type UpdateRequest struct {
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type PatchRequest struct {
	Title *string `json:"title,omitempty"`
	Done  *bool   `json:"done,omitempty"`
}

type ListFilter struct {
	Done   *bool
	Search string
	Limit  int
}
