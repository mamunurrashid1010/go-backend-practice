package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Service struct {
	repo UserRepository
}

func NewService(repo UserRepository) *Service {
	return &Service{repo: repo}
}

// Register hashes the password and creates the user. Returns *ConflictError
// if the email is already registered.
func (s *Service) Register(ctx context.Context, in RegisterRequest) (User, error) {
	email := normalizeEmail(in.Email)

	hash, err := HashPassword(in.Password)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	u, err := s.repo.Create(ctx, email, hash)
	if err != nil {
		return User{}, err // already a *ConflictError or wrapped DB error
	}
	return u, nil
}

// Login verifies credentials. Returns ErrInvalidCredentials for BOTH a
// missing email and a wrong password — never reveal which one failed.
func (s *Service) Login(ctx context.Context, in LoginRequest) (User, error) {
	email := normalizeEmail(in.Email)

	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}

	if !CheckPassword(u.PasswordHash, in.Password) {
		return User{}, ErrInvalidCredentials
	}
	return u, nil
}

func normalizeEmail(e string) string {
	return strings.ToLower(strings.TrimSpace(e))
}
