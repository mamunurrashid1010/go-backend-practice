// Package todo holds the To-Do feature. DTOs unchanged from Day 15.
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
	Title string `json:"title" validate:"required,max=200"`
	Done  bool   `json:"done"`
}

type UpdateRequest struct {
	Title string `json:"title" validate:"required,max=200"`
	Done  bool   `json:"done"`
}

type PatchRequest struct {
	Title *string `json:"title,omitempty" validate:"omitempty,min=1,max=200"`
	Done  *bool   `json:"done,omitempty"`
}

type ListFilter struct {
	Done   *bool
	Search string
	Limit  int
}
