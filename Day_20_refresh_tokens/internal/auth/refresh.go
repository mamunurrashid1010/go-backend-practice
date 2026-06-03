package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"
)

// refreshTokenBytes — 32 bytes = 256 bits of entropy. Unguessable.
const refreshTokenBytes = 32

// RefreshToken is the server-side record of an issued refresh token.
// We never store the plaintext — only TokenHash. The hash is the lookup key.
type RefreshToken struct {
	ID           int64
	UserID       int64
	TokenHash    string
	ExpiresAt    time.Time
	RevokedAt    *time.Time // nil = active
	ReplacedByID *int64     // points at the rotated successor
	CreatedAt    time.Time
}

// IsActive reports whether the token can still be used.
func (rt *RefreshToken) IsActive(now time.Time) bool {
	return rt.RevokedAt == nil && now.Before(rt.ExpiresAt)
}

// generateRefreshToken returns (plaintext, hash) — the plaintext goes to
// the client (once), the hash goes to the DB.
func generateRefreshToken() (plain, hashed string, err error) {
	b := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("rand: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(b)
	hashed = hashRefreshToken(plain)
	return plain, hashed, nil
}

// hashRefreshToken — SHA-256 hex of the plaintext.
//
// Why SHA-256 instead of bcrypt? Bcrypt is slow on purpose to defeat
// brute-force on WEAK passwords. A 256-bit random secret can't be
// brute-forced regardless. Fast deterministic hash is the right primitive:
// we look up by hash on every refresh.
func hashRefreshToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}
