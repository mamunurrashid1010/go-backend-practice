package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

type Record struct {
	State    string      `json:"state"`
	BodyHash string      `json:"body_hash"`
	Status   int         `json:"status,omitempty"`
	Headers  http.Header `json:"headers,omitempty"`
	Body     []byte      `json:"body,omitempty"`
}

var (
	ErrPending  = errors.New("idempotency: another request with the same key is in flight")
	ErrMismatch = errors.New("idempotency: request body differs from the cached one")
)

type Store struct {
	rdb      *redis.Client
	ttl      time.Duration
	leaseTTL time.Duration
}

func NewStore(rdb *redis.Client, ttl, leaseTTL time.Duration) *Store {
	return &Store{rdb: rdb, ttl: ttl, leaseTTL: leaseTTL}
}

func (s *Store) Reserve(ctx context.Context, key, bodyHash string) (*Record, error) {
	pending := Record{State: "pending", BodyHash: bodyHash}
	payload, err := json.Marshal(pending)
	if err != nil {
		return nil, fmt.Errorf("marshal pending: %w", err)
	}
	ok, err := s.rdb.SetNX(ctx, key, payload, s.leaseTTL).Result()
	if err != nil {
		return nil, fmt.Errorf("setnx: %w", err)
	}
	if ok {
		return nil, nil
	}
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("idempotency: lease vanished mid-reserve")
	}
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if rec.BodyHash != bodyHash {
		return nil, ErrMismatch
	}
	if rec.State == "pending" {
		return nil, ErrPending
	}
	return &rec, nil
}

func (s *Store) Save(ctx context.Context, key, bodyHash string, status int, headers http.Header, body []byte) error {
	rec := Record{State: "done", BodyHash: bodyHash, Status: status, Headers: headers, Body: body}
	payload, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}
	if err := s.rdb.Set(ctx, key, payload, s.ttl).Err(); err != nil {
		return fmt.Errorf("set done: %w", err)
	}
	return nil
}

func (s *Store) Release(ctx context.Context, key string) error {
	if err := s.rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("del: %w", err)
	}
	return nil
}
