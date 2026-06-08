// Package notes — UNCHANGED from Day 22.
package notes

import "time"

type Note struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateRequest struct {
	Title string `json:"title" validate:"required,max=200"`
	Body  string `json:"body"  validate:"max=10000"`
}

type UpdateRequest struct {
	Title string `json:"title" validate:"required,max=200"`
	Body  string `json:"body"  validate:"max=10000"`
}

type PatchRequest struct {
	Title *string `json:"title,omitempty" validate:"omitempty,min=1,max=200"`
	Body  *string `json:"body,omitempty"  validate:"omitempty,max=10000"`
}

type ListFilter struct {
	Search string
	Limit  int
}
