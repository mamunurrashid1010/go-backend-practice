package ratelimit

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Memory struct {
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

func NewMemory(rps float64, burst int, ttl time.Duration) *Memory {
	l := &Memory{
		visitors: make(map[string]*visitor),
		rate:     rate.Limit(rps),
		burst:    burst,
		ttl:      ttl,
		stop:     make(chan struct{}),
	}
	go l.cleanup()
	return l
}

func (l *Memory) get(key string) *rate.Limiter {
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

func (l *Memory) Allow(_ context.Context, key string) Decision {
	lim := l.get(key)
	ok := lim.Allow()
	tokens := lim.Tokens()
	if tokens < 0 {
		tokens = 0
	}
	d := Decision{Allowed: ok, Remaining: int(tokens)}
	if !ok && l.rate > 0 {
		d.RetryAfter = time.Duration(float64(time.Second) / float64(l.rate))
	}
	d.ResetAt = time.Now().Add(time.Duration(float64(l.burst-int(tokens)) / float64(l.rate) * float64(time.Second)))
	return d
}

func (l *Memory) Limit() int { return l.burst }

func (l *Memory) Stop() {
	select {
	case <-l.stop:
	default:
		close(l.stop)
	}
}

func (l *Memory) cleanup() {
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

func (l *Memory) Size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.visitors)
}
