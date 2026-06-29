// Package idempotency — Redis-backed dedup for retried POST/PUT/PATCH
// requests via the Idempotency-Key header. See README §6-§9.
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

// Record is what we cache per (userID, key). Captured after the
// handler runs; replayed verbatim on retry.
type Record struct {
	State    string      `json:"state"`     // "pending" while the handler runs; "done" once captured
	BodyHash string      `json:"body_hash"` // sha256 of the request body — for mismatch detection
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
	ttl      time.Duration // how long a "done" record survives
	leaseTTL time.Duration // how long a "pending" record survives before becoming reclaimable
}

func NewStore(rdb *redis.Client, ttl, leaseTTL time.Duration) *Store {
	return &Store{rdb: rdb, ttl: ttl, leaseTTL: leaseTTL}
}

// Reserve attempts to claim a slot for (key, bodyHash). Possible
// outcomes:
//
//   - (rec, nil)         — a "done" record exists with a matching body
//     hash; caller should replay it.
//   - (nil, nil)         — slot acquired; caller should run the
//     handler then call Save().
//   - (nil, ErrPending)  — another request is mid-flight; caller
//     should reply 409.
//   - (nil, ErrMismatch) — same key, different body; caller should
//     reply 422.
//   - (nil, otherErr)    — Redis error; caller decides (fail-open
//     by passing through is reasonable for idempotency).
func (s *Store) Reserve(ctx context.Context, key, bodyHash string) (*Record, error) {
	// Try to claim by SETNX with a "pending" sentinel.
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
		// Won the race; caller runs the handler.
		return nil, nil
	}

	// Lost the race; inspect what's there.
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		// The lease expired between SETNX and GET — retry one more
		// time at the caller's discretion. Bail; treat as transient.
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

// Save writes the captured response and flips the state to "done".
// Called after the handler returns.
func (s *Store) Save(ctx context.Context, key, bodyHash string, status int, headers http.Header, body []byte) error {
	rec := Record{
		State:    "done",
		BodyHash: bodyHash,
		Status:   status,
		Headers:  headers,
		Body:     body,
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}
	if err := s.rdb.Set(ctx, key, payload, s.ttl).Err(); err != nil {
		return fmt.Errorf("set done: %w", err)
	}
	return nil
}

// Release clears the pending slot — useful if the handler panicked
// and we still want to allow retry without waiting for the lease.
func (s *Store) Release(ctx context.Context, key string) error {
	if err := s.rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("del: %w", err)
	}
	return nil
}
