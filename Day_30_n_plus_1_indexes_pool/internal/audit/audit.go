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

// NoteRef — slim projection of a note for embedding in an audit view.
// We don't import notes.Note here to avoid an import cycle (notes
// already imports audit). Only the columns actually shown in the audit
// list live here.
type NoteRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// EntryWithNote — Entry + the current state of the target note.
// Note is a pointer because the note may have been deleted (audit
// outlives the row).
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
