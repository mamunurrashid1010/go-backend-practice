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

type NoteRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type EntryWithNote struct {
	Entry
	Note *NoteRef `json:"note,omitempty"`
}

const (
	ActionNoteCreated = "note.created"
	ActionNoteUpdated = "note.updated"
	ActionNotePatched = "note.patched"
	ActionNoteDeleted = "note.deleted"
)

const TargetNote = "note"
