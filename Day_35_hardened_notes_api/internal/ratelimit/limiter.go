package ratelimit

import (
	"context"
	"time"
)

type Decision struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
	ResetAt    time.Time
}

type Limiter interface {
	Allow(ctx context.Context, key string) Decision
	Limit() int
}
