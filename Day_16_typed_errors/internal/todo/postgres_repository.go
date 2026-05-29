package todo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// pgUniqueViolation is Postgres' SQLSTATE for a unique-constraint violation.
const pgUniqueViolation = "23505"

// asConflict inspects a driver error and, if it's a unique violation,
// returns a domain *ConflictError. Otherwise returns nil. This is the one
// place that knows about pgconn — the handler never imports it.
func asConflict(err error, field, value string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return &ConflictError{Field: field, Value: value}
	}
	return nil
}

func (r *PostgresRepository) List(ctx context.Context, f ListFilter) ([]Todo, error) {
	var (
		conds []string
		args  []any
	)
	if f.Done != nil {
		args = append(args, *f.Done)
		conds = append(conds, fmt.Sprintf("done = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
		conds = append(conds, fmt.Sprintf("LOWER(title) LIKE $%d", len(args)))
	}

	q := `SELECT id, title, done, created_at, updated_at FROM todos`
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY id"
	if f.Limit > 0 {
		args = append(args, f.Limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()

	out := make([]Todo, 0)
	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.Title, &t.Done, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list scan: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) Get(ctx context.Context, id int64) (Todo, error) {
	const q = `SELECT id, title, done, created_at, updated_at FROM todos WHERE id = $1`
	var t Todo
	err := r.db.QueryRowContext(ctx, q, id).
		Scan(&t.ID, &t.Title, &t.Done, &t.CreatedAt, &t.UpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Todo{}, ErrNotFound
	case err != nil:
		return Todo{}, fmt.Errorf("get id=%d: %w", id, err)
	}
	return t, nil
}

func (r *PostgresRepository) Create(ctx context.Context, in CreateRequest) (Todo, error) {
	const q = `
		INSERT INTO todos (title, done) VALUES ($1, $2)
		RETURNING id, title, done, created_at, updated_at
	`
	var t Todo
	err := r.db.QueryRowContext(ctx, q, in.Title, in.Done).
		Scan(&t.ID, &t.Title, &t.Done, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if conflict := asConflict(err, "title", in.Title); conflict != nil {
			return Todo{}, conflict
		}
		return Todo{}, fmt.Errorf("create: %w", err)
	}
	return t, nil
}

func (r *PostgresRepository) Update(ctx context.Context, id int64, in UpdateRequest) (Todo, error) {
	const q = `
		UPDATE todos
		SET    title = $2, done = $3, updated_at = now()
		WHERE  id = $1
		RETURNING id, title, done, created_at, updated_at
	`
	var t Todo
	err := r.db.QueryRowContext(ctx, q, id, in.Title, in.Done).
		Scan(&t.ID, &t.Title, &t.Done, &t.CreatedAt, &t.UpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Todo{}, ErrNotFound
	case err != nil:
		if conflict := asConflict(err, "title", in.Title); conflict != nil {
			return Todo{}, conflict
		}
		return Todo{}, fmt.Errorf("update id=%d: %w", id, err)
	}
	return t, nil
}

func (r *PostgresRepository) Patch(ctx context.Context, id int64, in PatchRequest) (Todo, error) {
	const q = `
		UPDATE todos
		SET    title      = COALESCE($2, title),
		       done       = COALESCE($3, done),
		       updated_at = now()
		WHERE  id = $1
		RETURNING id, title, done, created_at, updated_at
	`
	var t Todo
	err := r.db.QueryRowContext(ctx, q, id, in.Title, in.Done).
		Scan(&t.ID, &t.Title, &t.Done, &t.CreatedAt, &t.UpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Todo{}, ErrNotFound
	case err != nil:
		title := ""
		if in.Title != nil {
			title = *in.Title
		}
		if conflict := asConflict(err, "title", title); conflict != nil {
			return Todo{}, conflict
		}
		return Todo{}, fmt.Errorf("patch id=%d: %w", id, err)
	}
	return t, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM todos WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
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
