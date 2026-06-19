package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"
)

const refreshTokenBytes = 32

type RefreshToken struct {
	ID           int64
	UserID       int64
	TokenHash    string
	ExpiresAt    time.Time
	RevokedAt    *time.Time
	ReplacedByID *int64
	CreatedAt    time.Time
}

func (rt *RefreshToken) IsActive(now time.Time) bool {
	return rt.RevokedAt == nil && now.Before(rt.ExpiresAt)
}

func generateRefreshToken() (plain, hashed string, err error) {
	b := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("rand: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(b)
	hashed = hashRefreshToken(plain)
	return plain, hashed, nil
}

func hashRefreshToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}
