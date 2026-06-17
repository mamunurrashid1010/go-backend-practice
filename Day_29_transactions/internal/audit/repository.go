package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"day29/internal/dbtx"
)

// Repository — small surface. Log appends an entry; List reads recent
// entries for a user. Both go through dbtx.RunnerFor so they honour
// any tx attached to ctx.
type Repository interface {
	Log(ctx context.Context, userID int64, action, targetType string, targetID int64, metadata any) error
	List(ctx context.Context, userID int64, limit int) ([]Entry, error)
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
		FROM   audit_logs
		WHERE  user_id = $1
		ORDER  BY created_at DESC, id DESC
		LIMIT  $2
	`
	rows, err := dbtx.RunnerFor(ctx, r.db).QueryContext(ctx, q, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()

	out := make([]Entry, 0, limit)
	for rows.Next() {
		var (
			e   Entry
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
