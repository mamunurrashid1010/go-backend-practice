package notes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"day29/internal/dbtx"
)

type Repository interface {
	List(ctx context.Context, userID int64, f ListFilter) (ListPage, error)
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

// run resolves the runner once per call. If the caller put a *sql.Tx
// on the ctx via dbtx.WithTx, that's what we get; otherwise the *sql.DB
// pool. Identical SQL either way.
func (r *PostgresRepository) run(ctx context.Context) dbtx.DBTX {
	return dbtx.RunnerFor(ctx, r.db)
}

func (r *PostgresRepository) List(ctx context.Context, userID int64, f ListFilter) (ListPage, error) {
	args := []any{userID}
	conds := []string{"user_id = $1"}

	if f.Search != "" {
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
		conds = append(conds, fmt.Sprintf("LOWER(title) LIKE $%d", len(args)))
	}
	if f.AfterID > 0 {
		args = append(args, f.AfterID)
		if f.SortDesc {
			conds = append(conds, fmt.Sprintf("id < $%d", len(args)))
		} else {
			conds = append(conds, fmt.Sprintf("id > $%d", len(args)))
		}
	}
	order := "ORDER BY id"
	if f.SortDesc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	args = append(args, f.Limit+1)

	q := fmt.Sprintf(
		"SELECT id, user_id, title, body, created_at, updated_at FROM notes WHERE %s %s LIMIT $%d",
		strings.Join(conds, " AND "), order, len(args),
	)

	rows, err := r.run(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return ListPage{}, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()

	items := make([]Note, 0, f.Limit+1)
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return ListPage{}, fmt.Errorf("list scan: %w", err)
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		return ListPage{}, err
	}

	var nextID int64
	if len(items) > f.Limit {
		items = items[:f.Limit]
		nextID = items[len(items)-1].ID
	}
	return ListPage{Items: items, NextID: nextID}, nil
}

func (r *PostgresRepository) Get(ctx context.Context, userID, id int64) (Note, error) {
	const q = `SELECT id, user_id, title, body, created_at, updated_at FROM notes WHERE id = $1 AND user_id = $2`
	var n Note
	err := r.run(ctx).QueryRowContext(ctx, q, id, userID).
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
	err := r.run(ctx).QueryRowContext(ctx, q, userID, in.Title, in.Body).
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
	err := r.run(ctx).QueryRowContext(ctx, q, id, userID, in.Title, in.Body).
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
		SET    title = COALESCE($3, title),
		       body  = COALESCE($4, body),
		       updated_at = now()
		WHERE  id = $1 AND user_id = $2
		RETURNING id, user_id, title, body, created_at, updated_at
	`
	var n Note
	err := r.run(ctx).QueryRowContext(ctx, q, id, userID, in.Title, in.Body).
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
	res, err := r.run(ctx).ExecContext(ctx, q, id, userID)
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
