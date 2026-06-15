// Package ratelimit — per-key token-bucket limiter with TTL eviction.
//
// One *rate.Limiter per key (an IP, a user_id, etc). The limiter is kept
// in a map so each client has its own bucket; a background goroutine
// drops entries whose lastSeen is older than ttl, bounding memory.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Limiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     rate.Limit
	burst    int
	ttl      time.Duration
	stop     chan struct{}
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// New builds a Limiter and starts its eviction goroutine. Use Stop to
// release the goroutine in tests; production keeps it running.
func New(rps float64, burst int, ttl time.Duration) *Limiter {
	l := &Limiter{
		visitors: make(map[string]*visitor),
		rate:     rate.Limit(rps),
		burst:    burst,
		ttl:      ttl,
		stop:     make(chan struct{}),
	}
	go l.cleanup()
	return l
}

func (l *Limiter) get(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.visitors[key]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.visitors[key] = v
	}
	v.lastSeen = time.Now()
	return v.limiter
}

// Allow consumes a token; returns false if the bucket is empty.
func (l *Limiter) Allow(key string) bool {
	return l.get(key).Allow()
}

// Tokens reports how many whole tokens are currently in the bucket for
// key. Useful for the X-RateLimit-Remaining header. A non-existent key
// returns the full burst capacity.
func (l *Limiter) Tokens(key string) int {
	l.mu.Lock()
	v, ok := l.visitors[key]
	l.mu.Unlock()
	if !ok {
		return l.burst
	}
	t := v.limiter.Tokens()
	if t < 0 {
		return 0
	}
	return int(t)
}

func (l *Limiter) Burst() int { return l.burst }

// Stop ends the cleanup goroutine. Idempotent.
func (l *Limiter) Stop() {
	select {
	case <-l.stop:
	default:
		close(l.stop)
	}
}

func (l *Limiter) cleanup() {
	interval := l.ttl / 2
	if interval < time.Second {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case now := <-t.C:
			cutoff := now.Add(-l.ttl)
			l.mu.Lock()
			for k, v := range l.visitors {
				if v.lastSeen.Before(cutoff) {
					delete(l.visitors, k)
				}
			}
			l.mu.Unlock()
		}
	}
}

// Size returns the number of tracked clients. Test helper.
func (l *Limiter) Size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.visitors)
}
