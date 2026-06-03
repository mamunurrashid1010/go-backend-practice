package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type LoginResult struct {
	User         User
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
}

type RefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
}

type Service struct {
	repo         UserRepository
	refreshRepo  RefreshTokenRepository
	tokens       *TokenIssuer
	refreshTTL   time.Duration
}

func NewService(repo UserRepository, refreshRepo RefreshTokenRepository, tokens *TokenIssuer, refreshTTL time.Duration) *Service {
	return &Service{
		repo:        repo,
		refreshRepo: refreshRepo,
		tokens:      tokens,
		refreshTTL:  refreshTTL,
	}
}

func (s *Service) Register(ctx context.Context, in RegisterRequest) (User, error) {
	email := normalizeEmail(in.Email)
	hash, err := HashPassword(in.Password)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	return s.repo.Create(ctx, email, hash)
}

// Login checks the password and issues BOTH an access token and a refresh
// token. The plaintext refresh token is returned to the client once; only
// its hash lives in the DB.
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

	access, ttl, err := s.tokens.Issue(u)
	if err != nil {
		return LoginResult{}, err
	}

	plain, hashed, err := generateRefreshToken()
	if err != nil {
		return LoginResult{}, err
	}
	if _, err := s.refreshRepo.Create(ctx, u.ID, hashed, time.Now().UTC().Add(s.refreshTTL)); err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		User:         u,
		AccessToken:  access,
		RefreshToken: plain,
		ExpiresIn:    ttl,
	}, nil
}

// Refresh rotates the refresh token and issues a new access token.
//
// Reuse detection: if the presented token is already revoked, that's proof
// the chain has been compromised. We revoke the whole family descending
// from the reused token to force a re-login.
func (s *Service) Refresh(ctx context.Context, presented string) (RefreshResult, error) {
	hashed := hashRefreshToken(presented)
	rt, err := s.refreshRepo.GetByHash(ctx, hashed)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RefreshResult{}, ErrInvalidRefreshToken
		}
		return RefreshResult{}, err
	}

	// Reuse detection — token already revoked means somebody else may have
	// already used it. Kill the chain and refuse.
	if rt.RevokedAt != nil {
		if rerr := s.refreshRepo.RevokeFamilyDescendants(ctx, rt.ID); rerr != nil {
			// Log-worthy but we still refuse the request.
			return RefreshResult{}, fmt.Errorf("reuse detected, revoke chain: %w", rerr)
		}
		return RefreshResult{}, ErrInvalidRefreshToken
	}

	// Expired?
	if !rt.IsActive(time.Now().UTC()) {
		return RefreshResult{}, ErrInvalidRefreshToken
	}

	// Token's good — issue new pair.
	u, err := s.repo.GetByID(ctx, rt.UserID)
	if err != nil {
		// User was deleted but refresh token still exists. Refuse.
		if errors.Is(err, ErrNotFound) {
			_ = s.refreshRepo.Revoke(ctx, rt.ID, nil)
			return RefreshResult{}, ErrInvalidRefreshToken
		}
		return RefreshResult{}, err
	}

	access, ttl, err := s.tokens.Issue(u)
	if err != nil {
		return RefreshResult{}, err
	}

	newPlain, newHash, err := generateRefreshToken()
	if err != nil {
		return RefreshResult{}, err
	}
	newRT, err := s.refreshRepo.Create(ctx, u.ID, newHash, time.Now().UTC().Add(s.refreshTTL))
	if err != nil {
		return RefreshResult{}, err
	}

	// Mark the old one revoked + linked to the new one.
	if err := s.refreshRepo.Revoke(ctx, rt.ID, &newRT.ID); err != nil {
		return RefreshResult{}, err
	}

	return RefreshResult{
		AccessToken:  access,
		RefreshToken: newPlain,
		ExpiresIn:    ttl,
	}, nil
}

// Logout revokes the presented refresh token. Idempotent — logging out a
// missing or already-revoked token is a no-op (still 204) so the client can
// safely retry.
func (s *Service) Logout(ctx context.Context, presented string) error {
	hashed := hashRefreshToken(presented)
	rt, err := s.refreshRepo.GetByHash(ctx, hashed)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	if rt.RevokedAt != nil {
		return nil
	}
	return s.refreshRepo.Revoke(ctx, rt.ID, nil)
}

// GetByID — unchanged, used by /me.
func (s *Service) GetByID(ctx context.Context, id int64) (User, error) {
	return s.repo.GetByID(ctx, id)
}

func normalizeEmail(e string) string {
	return strings.ToLower(strings.TrimSpace(e))
}
