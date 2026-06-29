package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// slidingWindowScript — atomic ZSET-based sliding window log. See
// README §4. Returns {allowed (0/1), remaining, retry_after_ms}.
//
// KEYS[1] = bucket key
// ARGV[1] = now (ms)
// ARGV[2] = window (ms)
// ARGV[3] = limit
// ARGV[4] = unique member id for this request
var slidingWindowScript = redis.NewScript(`
local key    = KEYS[1]
local now    = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit  = tonumber(ARGV[3])
local member = ARGV[4]

redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window)
local count = redis.call('ZCARD', key)

if count >= limit then
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local retry = 0
    if oldest[2] then
        retry = (tonumber(oldest[2]) + window) - now
        if retry < 0 then retry = 0 end
    end
    return {0, 0, retry}
end

redis.call('ZADD', key, now, member)
redis.call('PEXPIRE', key, window)
return {1, limit - (count + 1), 0}
`)

// Redis is a distributed sliding-window-log limiter. The bucket is a
// ZSET keyed `<prefix>:<clientKey>`; entries are pruned and counted
// atomically by a Lua script every request.
type Redis struct {
	rdb    *redis.Client
	prefix string
	window time.Duration
	limit  int
}

func NewRedis(rdb *redis.Client, prefix string, window time.Duration, limit int) *Redis {
	return &Redis{rdb: rdb, prefix: prefix, window: window, limit: limit}
}

func (l *Redis) Limit() int { return l.limit }

func (l *Redis) Allow(ctx context.Context, key string) Decision {
	bucket := l.prefix + ":" + key
	now := time.Now()
	member := uniqueMember(now)

	res, err := slidingWindowScript.Run(
		ctx, l.rdb, []string{bucket},
		now.UnixMilli(),
		l.window.Milliseconds(),
		l.limit,
		member,
	).Result()
	if err != nil {
		// Fail CLOSED: deny on Redis trouble. Fail-open here turns the
		// limiter off under partial outage — exactly when you want it
		// most. Day 33 README discusses the tradeoff.
		slog.Default().Error("rate limit redis call failed", slog.Any("err", err))
		return Decision{Allowed: false, Remaining: 0, RetryAfter: l.window}
	}

	arr, ok := res.([]any)
	if !ok || len(arr) != 3 {
		slog.Default().Error("rate limit redis bad reply", slog.Any("res", res))
		return Decision{Allowed: false, Remaining: 0, RetryAfter: l.window}
	}
	allowed := arr[0].(int64) == 1
	remaining := int(arr[1].(int64))
	retryMs := time.Duration(arr[2].(int64)) * time.Millisecond

	return Decision{
		Allowed:    allowed,
		Remaining:  remaining,
		RetryAfter: retryMs,
		ResetAt:    now.Add(retryMs),
	}
}

func uniqueMember(now time.Time) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%d:%s", now.UnixNano(), hex.EncodeToString(b[:]))
}
