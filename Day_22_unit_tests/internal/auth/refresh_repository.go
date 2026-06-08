package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type RefreshTokenRepository interface {
	Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (*RefreshToken, error)
	GetByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	Revoke(ctx context.Context, id int64, replacedByID *int64) error
	RevokeFamilyDescendants(ctx context.Context, startID int64) error
}

type PostgresRefreshTokenRepository struct {
	db *sql.DB
}

func NewPostgresRefreshTokenRepository(db *sql.DB) *PostgresRefreshTokenRepository {
	return &PostgresRefreshTokenRepository{db: db}
}

func (r *PostgresRefreshTokenRepository) Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (*RefreshToken, error) {
	const q = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token_hash, expires_at, revoked_at, replaced_by_id, created_at
	`
	var rt RefreshToken
	err := r.db.QueryRowContext(ctx, q, userID, tokenHash, expiresAt).
		Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.RevokedAt, &rt.ReplacedByID, &rt.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}
	return &rt, nil
}

func (r *PostgresRefreshTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	const q = `
		SELECT id, user_id, token_hash, expires_at, revoked_at, replaced_by_id, created_at
		FROM   refresh_tokens
		WHERE  token_hash = $1
	`
	var rt RefreshToken
	err := r.db.QueryRowContext(ctx, q, tokenHash).
		Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.RevokedAt, &rt.ReplacedByID, &rt.CreatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("get refresh token: %w", err)
	}
	return &rt, nil
}

func (r *PostgresRefreshTokenRepository) Revoke(ctx context.Context, id int64, replacedByID *int64) error {
	const q = `
		UPDATE refresh_tokens
		SET    revoked_at = now(), replaced_by_id = $2
		WHERE  id = $1 AND revoked_at IS NULL
	`
	if _, err := r.db.ExecContext(ctx, q, id, replacedByID); err != nil {
		return fmt.Errorf("revoke refresh token id=%d: %w", id, err)
	}
	return nil
}

func (r *PostgresRefreshTokenRepository) RevokeFamilyDescendants(ctx context.Context, startID int64) error {
	const q = `
		WITH RECURSIVE family AS (
		    SELECT id, replaced_by_id FROM refresh_tokens WHERE id = $1
		    UNION ALL
		    SELECT rt.id, rt.replaced_by_id
		    FROM   refresh_tokens rt
		    JOIN   family f ON rt.id = f.replaced_by_id
		)
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE id IN (SELECT id FROM family) AND revoked_at IS NULL
	`
	if _, err := r.db.ExecContext(ctx, q, startID); err != nil {
		return fmt.Errorf("revoke family descendants of id=%d: %w", startID, err)
	}
	return nil
}
