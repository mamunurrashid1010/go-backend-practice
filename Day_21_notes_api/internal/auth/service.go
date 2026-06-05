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
	repo        UserRepository
	refreshRepo RefreshTokenRepository
	tokens      *TokenIssuer
	refreshTTL  time.Duration
}

func NewService(repo UserRepository, refreshRepo RefreshTokenRepository, tokens *TokenIssuer, refreshTTL time.Duration) *Service {
	return &Service{repo: repo, refreshRepo: refreshRepo, tokens: tokens, refreshTTL: refreshTTL}
}

func (s *Service) Register(ctx context.Context, in RegisterRequest) (User, error) {
	email := normalizeEmail(in.Email)
	hash, err := HashPassword(in.Password)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	return s.repo.Create(ctx, email, hash)
}

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
	return LoginResult{User: u, AccessToken: access, RefreshToken: plain, ExpiresIn: ttl}, nil
}

func (s *Service) Refresh(ctx context.Context, presented string) (RefreshResult, error) {
	hashed := hashRefreshToken(presented)
	rt, err := s.refreshRepo.GetByHash(ctx, hashed)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RefreshResult{}, ErrInvalidRefreshToken
		}
		return RefreshResult{}, err
	}
	if rt.RevokedAt != nil {
		if rerr := s.refreshRepo.RevokeFamilyDescendants(ctx, rt.ID); rerr != nil {
			return RefreshResult{}, fmt.Errorf("reuse detected, revoke chain: %w", rerr)
		}
		return RefreshResult{}, ErrInvalidRefreshToken
	}
	if !rt.IsActive(time.Now().UTC()) {
		return RefreshResult{}, ErrInvalidRefreshToken
	}
	u, err := s.repo.GetByID(ctx, rt.UserID)
	if err != nil {
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
	if err := s.refreshRepo.Revoke(ctx, rt.ID, &newRT.ID); err != nil {
		return RefreshResult{}, err
	}
	return RefreshResult{AccessToken: access, RefreshToken: newPlain, ExpiresIn: ttl}, nil
}

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

func (s *Service) GetByID(ctx context.Context, id int64) (User, error) {
	return s.repo.GetByID(ctx, id)
}

func normalizeEmail(e string) string {
	return strings.ToLower(strings.TrimSpace(e))
}
