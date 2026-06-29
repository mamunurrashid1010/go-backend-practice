// Package ratelimit — pluggable limiter interface with two backends:
// in-process token bucket (NewMemory) and Redis sliding window log
// (NewRedis). Both satisfy Limiter so middleware code is identical.
package ratelimit

import (
	"context"
	"time"
)

// Decision is what every Allow call returns.
//   - Allowed     : true to pass, false to deny
//   - Remaining   : tokens / slots left in this window
//   - RetryAfter  : on deny only, how long until the bucket is unblocked
//   - ResetAt     : when the bucket would refill to Limit (UNIX epoch)
type Decision struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
	ResetAt    time.Time
}

type Limiter interface {
	// Allow decides whether the request with the given client key
	// passes. Implementations may use ctx for cancellation; deny on
	// timeout (don't fail-open, that defeats the limiter under attack).
	Allow(ctx context.Context, key string) Decision

	// Limit is the value exposed as X-RateLimit-Limit. For the in-
	// process token bucket this is burst; for the Redis sliding window
	// it's max-per-window.
	Limit() int
}
