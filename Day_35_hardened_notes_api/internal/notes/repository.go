package notes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"day35/internal/db"
	"day35/internal/dbtx"
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

func NewPostgresRepository(d *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: d}
}

func (r *PostgresRepository) q(ctx context.Context) *db.Queries {
	return db.New(dbtx.RunnerFor(ctx, r.db).(db.DBTX))
}

func (r *PostgresRepository) run(ctx context.Context) dbtx.DBTX {
	return dbtx.RunnerFor(ctx, r.db)
}

func (r *PostgresRepository) Get(ctx context.Context, userID, id int64) (Note, error) {
	row, err := r.q(ctx).GetNote(ctx, db.GetNoteParams{ID: id, UserID: userID})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Note{}, ErrNotFound
	case err != nil:
		return Note{}, fmt.Errorf("get id=%d: %w", id, err)
	}
	return fromDB(row), nil
}

func (r *PostgresRepository) Create(ctx context.Context, userID int64, in CreateRequest) (Note, error) {
	row, err := r.q(ctx).CreateNote(ctx, db.CreateNoteParams{UserID: userID, Title: in.Title, Body: in.Body})
	if err != nil {
		return Note{}, fmt.Errorf("create: %w", err)
	}
	return fromDB(row), nil
}

func (r *PostgresRepository) Update(ctx context.Context, userID, id int64, in UpdateRequest) (Note, error) {
	row, err := r.q(ctx).UpdateNote(ctx, db.UpdateNoteParams{ID: id, UserID: userID, Title: in.Title, Body: in.Body})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Note{}, ErrNotFound
	case err != nil:
		return Note{}, fmt.Errorf("update id=%d: %w", id, err)
	}
	return fromDB(row), nil
}

func (r *PostgresRepository) Patch(ctx context.Context, userID, id int64, in PatchRequest) (Note, error) {
	row, err := r.q(ctx).PatchNote(ctx, db.PatchNoteParams{
		ID: id, UserID: userID, Title: nullString(in.Title), Body: nullString(in.Body),
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Note{}, ErrNotFound
	case err != nil:
		return Note{}, fmt.Errorf("patch id=%d: %w", id, err)
	}
	return fromDB(row), nil
}

func (r *PostgresRepository) Delete(ctx context.Context, userID, id int64) error {
	n, err := r.q(ctx).DeleteNote(ctx, db.DeleteNoteParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("delete id=%d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
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

func fromDB(n db.Note) Note {
	return Note{
		ID: n.ID, UserID: n.UserID, Title: n.Title, Body: n.Body,
		CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
	}
}

func nullString(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}
