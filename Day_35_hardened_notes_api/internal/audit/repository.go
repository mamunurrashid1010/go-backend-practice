package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"day35/internal/dbtx"
)

type Repository interface {
	Log(ctx context.Context, userID int64, action, targetType string, targetID int64, metadata any) error
	List(ctx context.Context, userID int64, limit int) ([]Entry, error)
	ListWithNotesJoin(ctx context.Context, userID int64, limit int) ([]EntryWithNote, error)
	ListWithNotesInBatch(ctx context.Context, userID int64, limit int) ([]EntryWithNote, error)
	ListWithNotesNaive(ctx context.Context, userID int64, limit int) ([]EntryWithNote, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Log(ctx context.Context, userID int64, action, targetType string, targetID int64, metadata any) error {
	var metaJSON []byte
	if metadata != nil {
		b, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshal audit metadata: %w", err)
		}
		metaJSON = b
	}
	const q = `
		INSERT INTO audit_logs (user_id, action, target_type, target_id, metadata)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := dbtx.RunnerFor(ctx, r.db).ExecContext(ctx, q, userID, action, targetType, targetID, metaJSON)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

func (r *PostgresRepository) List(ctx context.Context, userID int64, limit int) ([]Entry, error) {
	const q = `
		SELECT id, user_id, action, target_type, target_id, metadata, created_at
		FROM   audit_logs WHERE user_id = $1
		ORDER  BY created_at DESC, id DESC LIMIT $2
	`
	rows, err := dbtx.RunnerFor(ctx, r.db).QueryContext(ctx, q, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()
	out := make([]Entry, 0, limit)
	for rows.Next() {
		var (
			e    Entry
			meta sql.NullString
		)
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.TargetType, &e.TargetID, &meta, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		if meta.Valid {
			e.Metadata = json.RawMessage(meta.String)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListWithNotesJoin(ctx context.Context, userID int64, limit int) ([]EntryWithNote, error) {
	const q = `
		SELECT a.id, a.user_id, a.action, a.target_type, a.target_id, a.metadata, a.created_at,
		       n.id, n.title
		FROM   audit_logs a
		LEFT JOIN notes n ON n.id = a.target_id AND a.target_type = 'note'
		WHERE  a.user_id = $1
		ORDER  BY a.created_at DESC, a.id DESC LIMIT $2
	`
	rows, err := dbtx.RunnerFor(ctx, r.db).QueryContext(ctx, q, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit join: %w", err)
	}
	defer rows.Close()
	out := make([]EntryWithNote, 0, limit)
	for rows.Next() {
		var (
			ewn  EntryWithNote
			meta sql.NullString
			nID  sql.NullInt64
			nTtl sql.NullString
		)
		if err := rows.Scan(&ewn.ID, &ewn.UserID, &ewn.Action, &ewn.TargetType, &ewn.TargetID, &meta, &ewn.CreatedAt, &nID, &nTtl); err != nil {
			return nil, fmt.Errorf("scan audit join: %w", err)
		}
		if meta.Valid {
			ewn.Metadata = json.RawMessage(meta.String)
		}
		if nID.Valid {
			ewn.Note = &NoteRef{ID: nID.Int64, Title: nTtl.String}
		}
		out = append(out, ewn)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListWithNotesInBatch(ctx context.Context, userID int64, limit int) ([]EntryWithNote, error) {
	entries, err := r.List(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	idSet := make(map[int64]struct{}, len(entries))
	for _, e := range entries {
		if e.TargetType == TargetNote {
			idSet[e.TargetID] = struct{}{}
		}
	}
	notesByID := map[int64]NoteRef{}
	if len(idSet) > 0 {
		args := make([]any, 0, len(idSet)+1)
		args = append(args, userID)
		placeholders := make([]string, 0, len(idSet))
		for id := range idSet {
			args = append(args, id)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		q := fmt.Sprintf("SELECT id, title FROM notes WHERE user_id = $1 AND id IN (%s)", strings.Join(placeholders, ", "))
		rows, err := dbtx.RunnerFor(ctx, r.db).QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("list audit batch fetch: %w", err)
		}
		for rows.Next() {
			var n NoteRef
			if err := rows.Scan(&n.ID, &n.Title); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan batched note: %w", err)
			}
			notesByID[n.ID] = n
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	out := make([]EntryWithNote, 0, len(entries))
	for _, e := range entries {
		ewn := EntryWithNote{Entry: e}
		if e.TargetType == TargetNote {
			if n, ok := notesByID[e.TargetID]; ok {
				ewn.Note = &n
			}
		}
		out = append(out, ewn)
	}
	return out, nil
}

func (r *PostgresRepository) ListWithNotesNaive(ctx context.Context, userID int64, limit int) ([]EntryWithNote, error) {
	entries, err := r.List(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]EntryWithNote, 0, len(entries))
	for _, e := range entries {
		ewn := EntryWithNote{Entry: e}
		if e.TargetType == TargetNote {
			n, err := r.fetchOneNote(ctx, userID, e.TargetID)
			if err != nil {
				out = append(out, ewn)
				continue
			}
			ewn.Note = &n
		}
		out = append(out, ewn)
	}
	return out, nil
}

func (r *PostgresRepository) fetchOneNote(ctx context.Context, userID, id int64) (NoteRef, error) {
	const q = `SELECT id, title FROM notes WHERE id = $1 AND user_id = $2`
	var n NoteRef
	err := dbtx.RunnerFor(ctx, r.db).QueryRowContext(ctx, q, id, userID).Scan(&n.ID, &n.Title)
	return n, err
}
