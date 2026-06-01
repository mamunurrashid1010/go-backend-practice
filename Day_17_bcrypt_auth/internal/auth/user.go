// Package auth holds registration + login: the user model, DTOs, password
// hashing, the user repository, the service, and the HTTP handlers.
package auth

import "time"

// User — the domain model.
//
// PasswordHash has `json:"-"` so it NEVER appears in any JSON response.
// This is the single most important struct tag in the package.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// RegisterRequest — POST /auth/register body.
// password max=72 because bcrypt ignores bytes past 72.
type RegisterRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// LoginRequest — POST /auth/login body. No min/max on password here: we
// don't reveal our password rules to an attacker probing the login form,
// and an old password might predate a rule change.
type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}
