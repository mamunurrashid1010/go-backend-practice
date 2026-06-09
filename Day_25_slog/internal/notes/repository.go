package notes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Repository interface {
	List(ctx context.Context, userID int64, f ListFilter) ([]Note, error)
	Get(ctx context.Context, userID, id int64) (Note, error)
	Create(ctx context.Context, userID int64, in CreateRequest) (Note, error)
	Update(ctx context.Context, userID, id int64, in UpdateRequest) (Note, error)
	Patch(ctx context.Context, userID, id int64, in PatchRequest) (Note, error)
	Delete(ctx context.Context, userID, id int64) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) List(ctx context.Context, userID int64, f ListFilter) ([]Note, error) {
	args := []any{userID}
	conds := []string{"user_id = $1"}
	if f.Search != "" {
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
		conds = append(conds, fmt.Sprintf("LOWER(title) LIKE $%d", len(args)))
	}
	q := "SELECT id, user_id, title, body, created_at, updated_at FROM notes WHERE " +
		strings.Join(conds, " AND ") + " ORDER BY id"
	if f.Limit > 0 {
		args = append(args, f.Limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()
	out := make([]Note, 0)
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list scan: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) Get(ctx context.Context, userID, id int64) (Note, error) {
	const q = `SELECT id, user_id, title, body, created_at, updated_at FROM notes WHERE id = $1 AND user_id = $2`
	var n Note
	err := r.db.QueryRowContext(ctx, q, id, userID).
		Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Note{}, ErrNotFound
	case err != nil:
		return Note{}, fmt.Errorf("get id=%d: %w", id, err)
	}
	return n, nil
}

func (r *PostgresRepository) Create(ctx context.Context, userID int64, in CreateRequest) (Note, error) {
	const q = `
		INSERT INTO notes (user_id, title, body) VALUES ($1, $2, $3)
		RETURNING id, user_id, title, body, created_at, updated_at
	`
	var n Note
	err := r.db.QueryRowContext(ctx, q, userID, in.Title, in.Body).
		Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return Note{}, fmt.Errorf("create: %w", err)
	}
	return n, nil
}

func (r *PostgresRepository) Update(ctx context.Context, userID, id int64, in UpdateRequest) (Note, error) {
	const q = `
		UPDATE notes SET title = $3, body = $4, updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, title, body, created_at, updated_at
	`
	var n Note
	err := r.db.QueryRowContext(ctx, q, id, userID, in.Title, in.Body).
		Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Note{}, ErrNotFound
	case err != nil:
		return Note{}, fmt.Errorf("update id=%d: %w", id, err)
	}
	return n, nil
}

func (r *PostgresRepository) Patch(ctx context.Context, userID, id int64, in PatchRequest) (Note, error) {
	const q = `
		UPDATE notes
		SET    title      = COALESCE($3, title),
		       body       = COALESCE($4, body),
		       updated_at = now()
		WHERE  id = $1 AND user_id = $2
		RETURNING id, user_id, title, body, created_at, updated_at
	`
	var n Note
	err := r.db.QueryRowContext(ctx, q, id, userID, in.Title, in.Body).
		Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Note{}, ErrNotFound
	case err != nil:
		return Note{}, fmt.Errorf("patch id=%d: %w", id, err)
	}
	return n, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, userID, id int64) error {
	const q = `DELETE FROM notes WHERE id = $1 AND user_id = $2`
	res, err := r.db.ExecContext(ctx, q, id, userID)
	if err != nil {
		return fmt.Errorf("delete id=%d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete id=%d rows-affected: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
