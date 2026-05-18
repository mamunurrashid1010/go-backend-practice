// Package todo holds the To-Do domain: the model, the request DTOs,
// the in-memory store, and the HTTP handlers.
//
// Day 11 will split the handler off from the store with a service layer
// between them. Today's split (types + store + handler) is already enough
// to feel real — and it's exactly the right starting shape.
package todo

import "time"

// Todo is the domain model — what's stored and what's returned.
type Todo struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateRequest is the POST body shape.
// Only fields the client may set — no ID, no timestamps.
type CreateRequest struct {
	Title string `json:"title"`
	Done  bool   `json:"done,omitempty"`
}

// UpdateRequest is the PUT body shape: full replacement.
// Both fields are required.
type UpdateRequest struct {
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// PatchRequest is the PATCH body shape: partial update.
// Pointer fields distinguish "not provided" (nil) from "set to zero" (non-nil
// pointing at "" or false).
type PatchRequest struct {
	Title *string `json:"title,omitempty"`
	Done  *bool   `json:"done,omitempty"`
}

// ListFilter narrows GET /todos. Empty Done means "no filter".
type ListFilter struct {
	Done   *bool  // nil → no filter; true/false → match
	Search string // case-insensitive substring on Title; "" → no filter
	Limit  int    // 0 → no limit
}
