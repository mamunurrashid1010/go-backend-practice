// Package cache — small JSON wrapper around go-redis/v9.
//
// Three methods: GetJSON, SetJSON, Delete. Nothing knows about specific
// domain types; callers pass anything json-serializable. TTL jitter is
// applied automatically so a million keys written together don't all
// expire on the same tick.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	rdb    *redis.Client
	jitter float64 // 0..1; 0.1 == "expire up to 10% later than ttl"
}

func New(rdb *redis.Client, jitter float64) *Cache {
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 1 {
		jitter = 1
	}
	return &Cache{rdb: rdb, jitter: jitter}
}

// GetJSON tries to fetch and unmarshal the value for key into dst.
// Returns (true, nil) on a hit, (false, nil) on a clean miss, and
// (false, err) only on an unexpected Redis or unmarshal error.
//
// Important: a JSON decode error is a "miss + log it" condition for
// the caller — bad cache content shouldn't break the request, so we
// return (false, err). The notes service then falls back to Postgres.
func (c *Cache) GetJSON(ctx context.Context, key string, dst any) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, nil
	}
	b, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cache get %s: %w", key, err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return false, fmt.Errorf("cache unmarshal %s: %w", key, err)
	}
	return true, nil
}

// SetJSON writes value at key with the supplied TTL plus a random
// jitter so coordinated expiries don't all land at the same moment.
func (c *Cache) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache marshal %s: %w", key, err)
	}
	if err := c.rdb.Set(ctx, key, b, c.jitterTTL(ttl)).Err(); err != nil {
		return fmt.Errorf("cache set %s: %w", key, err)
	}
	return nil
}

// Delete removes a key. A missing key is not an error.
func (c *Cache) Delete(ctx context.Context, key string) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	if err := c.rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("cache del %s: %w", key, err)
	}
	return nil
}

func (c *Cache) jitterTTL(ttl time.Duration) time.Duration {
	if c.jitter == 0 || ttl <= 0 {
		return ttl
	}
	// Add 0..jitter*ttl, never subtract. Better to over-keep than
	// silently shorten the TTL the operator picked.
	extra := time.Duration(float64(ttl) * c.jitter * rand.Float64())
	return ttl + extra
}
