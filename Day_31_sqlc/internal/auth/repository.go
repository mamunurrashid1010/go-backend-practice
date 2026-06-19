package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"day31/internal/db"
	"day31/internal/dbtx"
)

type UserRepository interface {
	Create(ctx context.Context, email, passwordHash string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
	GetByID(ctx context.Context, id int64) (User, error)
}

// PostgresUserRepository — thin adapter over sqlc-generated *db.Queries.
// All three methods translate between domain types (User) and DB row
// types (db.User). One translation function at the bottom.
type PostgresUserRepository struct {
	db *sql.DB
}

func NewPostgresUserRepository(d *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: d}
}

func (r *PostgresUserRepository) q(ctx context.Context) *db.Queries {
	return db.New(dbtx.RunnerFor(ctx, r.db).(db.DBTX))
}

const pgUniqueViolation = "23505"

func (r *PostgresUserRepository) Create(ctx context.Context, email, passwordHash string) (User, error) {
	row, err := r.q(ctx).CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: passwordHash,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return User{}, &ConflictError{Field: "email", Value: email}
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return fromDB(row), nil
}

func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (User, error) {
	row, err := r.q(ctx).GetUserByEmail(ctx, email)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return User{}, ErrNotFound
	case err != nil:
		return User{}, fmt.Errorf("get user by email: %w", err)
	}
	return fromDB(row), nil
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id int64) (User, error) {
	row, err := r.q(ctx).GetUserByID(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return User{}, ErrNotFound
	case err != nil:
		return User{}, fmt.Errorf("get user by id: %w", err)
	}
	return fromDB(row), nil
}

func fromDB(u db.User) User {
	return User{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}
