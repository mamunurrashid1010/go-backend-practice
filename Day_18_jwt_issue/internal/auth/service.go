package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// LoginResult bundles what the handler needs to write the response.
type LoginResult struct {
	User        User
	AccessToken string
	ExpiresIn   time.Duration
}

type Service struct {
	repo   UserRepository
	tokens *TokenIssuer
}

func NewService(repo UserRepository, tokens *TokenIssuer) *Service {
	return &Service{repo: repo, tokens: tokens}
}

func (s *Service) Register(ctx context.Context, in RegisterRequest) (User, error) {
	email := normalizeEmail(in.Email)

	hash, err := HashPassword(in.Password)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	u, err := s.repo.Create(ctx, email, hash)
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// Login verifies the password and issues an access token. The same
// ErrInvalidCredentials is returned for "no such email" and "wrong password"
// so the API can't be used to enumerate registered emails.
func (s *Service) Login(ctx context.Context, in LoginRequest) (LoginResult, error) {
	email := normalizeEmail(in.Email)

	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, err
	}

	if !CheckPassword(u.PasswordHash, in.Password) {
		return LoginResult{}, ErrInvalidCredentials
	}

	token, ttl, err := s.tokens.Issue(u)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{User: u, AccessToken: token, ExpiresIn: ttl}, nil
}

func normalizeEmail(e string) string {
	return strings.ToLower(strings.TrimSpace(e))
}
