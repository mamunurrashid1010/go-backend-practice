// Package audit — audit-log domain. Entries describe a user-scoped
// action against some target. Entries are written via a transactional
// Log() call alongside the action they record; they're read via List().
package audit

import (
	"encoding/json"
	"time"
)

type Entry struct {
	ID         int64           `json:"id"`
	UserID     int64           `json:"user_id"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type"`
	TargetID   int64           `json:"target_id"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Action constants — keep these stable; downstream tools (dashboards,
// alerts, log queries) match on the string. New actions should be added
// here, not invented inline.
const (
	ActionNoteCreated = "note.created"
	ActionNoteUpdated = "note.updated"
	ActionNotePatched = "note.patched"
	ActionNoteDeleted = "note.deleted"
)

const (
	TargetNote = "note"
)
