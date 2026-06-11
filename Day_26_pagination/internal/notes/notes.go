// Package notes — Day 26 adds cursor pagination.
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

// ListFilter — Day 26 grows AfterID and SortDesc.
type ListFilter struct {
	Search   string
	Limit    int   // 1..100
	AfterID  int64 // 0 = first page
	SortDesc bool  // true = newest first (default)
}

// ListPage — the repository's paged result.
// NextID is 0 when there's no next page.
type ListPage struct {
	Items  []Note
	NextID int64
}
