package cache

import (
	"context"
	"testing"
	"time"
)

// These tests cover the no-Redis "nil safe" path — handy for unit tests
// of services that depend on *Cache without a running container.
// Integration tests against a real Redis live behind a build tag (see
// TASKS.md Task 4).

func TestCache_NilCache_GetMiss(t *testing.T) {
	var c *Cache
	var dst struct{ X int }
	hit, err := c.GetJSON(context.Background(), "any", &dst)
	if err != nil {
		t.Fatalf("nil GetJSON err: %v", err)
	}
	if hit {
		t.Fatalf("nil GetJSON should be a miss")
	}
}

func TestCache_NilCache_SetNoop(t *testing.T) {
	var c *Cache
	if err := c.SetJSON(context.Background(), "k", 42, time.Minute); err != nil {
		t.Fatalf("nil SetJSON err: %v", err)
	}
	if err := c.Delete(context.Background(), "k"); err != nil {
		t.Fatalf("nil Delete err: %v", err)
	}
}

func TestJitterTTL_BoundedAbove(t *testing.T) {
	c := &Cache{jitter: 0.1}
	base := 10 * time.Second
	for i := 0; i < 100; i++ {
		got := c.jitterTTL(base)
		if got < base {
			t.Fatalf("jitterTTL shrunk below base: %v < %v", got, base)
		}
		if got > time.Duration(float64(base)*1.1)+time.Millisecond {
			t.Fatalf("jitterTTL exceeded max: %v > %v", got, time.Duration(float64(base)*1.1))
		}
	}
}

func TestJitterTTL_ZeroJitterPassThrough(t *testing.T) {
	c := &Cache{jitter: 0}
	if got := c.jitterTTL(7 * time.Second); got != 7*time.Second {
		t.Fatalf("jitterTTL with 0 jitter changed value: %v", got)
	}
}
