# Day 33 — Distributed Rate Limit + Idempotency Keys

Two independent things, both wired through Redis:

1. **Distributed rate limiter** — Day 27's token-bucket lives in process memory. At 10 replicas, a "100 rps" limit becomes "1000 rps" in aggregate. Today we swap the backend for a Redis **sliding window log** so the limit holds across replicas.
2. **`Idempotency-Key` header** on POST/PUT/PATCH — clients send a unique key with each retry. Redis caches the original response keyed by `(userID, key)`. Same key + same body → replay. Same key + different body → 422. Hyphen of Stripe/HTTP-style retry safety.

---

## 1. Why the in-process limiter doesn't scale

```
┌─ replica 1 ─┐    bucket: 10/s    ┐
├─ replica 2 ─┤    bucket: 10/s    ├─ total ceiling: 30/s, not 10/s
└─ replica 3 ─┘    bucket: 10/s    ┘
```

Each Go process holds its own token bucket. Three replicas, "10/s per IP" becomes "30/s per IP" — the limit you advertised isn't real. Two ways out:

- **Shared store** (Redis here). All replicas decrement the same counter.
- **Per-replica budgets** divided up-front (`MaxOpenConns / replicas`). Works when replica count is static; falls apart with autoscaling.

We pick shared-store.

---

## 2. Sliding window log — the algorithm

Three competing distributed algorithms:

| Algorithm | Accuracy | Cost per req | Memory per bucket |
| --- | --- | --- | --- |
| **Fixed window** (`INCR` + `EXPIRE`) | bad at edges (2x spike around boundary) | O(1) | O(1) |
| **Sliding window counter** | ~1% off | O(1) | O(1) |
| **Sliding window log** (`ZADD` + `ZREMRANGEBYSCORE`) | exact | O(log N) | O(N) per bucket |

We use the log — it's the algorithmic flagship and small N makes the cost negligible. Each bucket is a Redis sorted set; the score is the timestamp, the member is a unique-per-request id.

The wire shape per request:

1. `ZREMRANGEBYSCORE key -inf (now - window)` — drop expired entries.
2. `ZCARD key` — count surviving entries.
3. If `count >= limit` → deny, peek the oldest with `ZRANGE 0 0 WITHSCORES` to compute Retry-After.
4. Else → `ZADD key now uniqueID`, `PEXPIRE key window`, return allowed.

All four steps must be **atomic** across replicas. The naive Go-side loop has a race where two replicas both see `count < limit` and both ZADD. Solution: one Lua script run with `EVAL` — Redis executes the whole script under its single thread.

The full script lives in [internal/ratelimit/redis.go](internal/ratelimit/redis.go). It returns `[allowed, remaining, retryAfterMs]` so the Go side just decodes a 3-element array.

---

## 3. The Limiter interface — letting both implementations coexist

```go
// internal/ratelimit/limiter.go
type Decision struct {
    Allowed    bool
    Remaining  int
    RetryAfter time.Duration
}

type Limiter interface {
    Allow(ctx context.Context, key string) Decision
    Limit() int            // for the X-RateLimit-Limit header
}
```

The Day 27 in-process limiter satisfies the same interface — same `Allow`, same `Limit()`. So the middleware doesn't care which backend it's wired to:

```go
func RateLimit(l ratelimit.Limiter) func(http.Handler) http.Handler { ... }
```

`main.go` picks the implementation:

```go
switch cfg.RateLimit.Backend {
case "redis":
    globalLimiter = ratelimit.NewRedis(rdb, "rl:global", time.Minute, cfg.RateLimit.GlobalMaxPerMinute)
case "memory":
    globalLimiter = ratelimit.NewMemory(cfg.RateLimit.GlobalRPS, cfg.RateLimit.GlobalBurst, cfg.RateLimit.TTL)
}
```

This is the "boring interface, plural implementations" pattern. Same shape as the `Repository` interface from Day 11 — it has paid for itself again.

### Why two algorithms with different shapes

The in-process limiter is **token bucket** (rps + burst). The Redis one is **sliding window log** (max-per-window). Why not match them?

Token bucket is much cheaper in-process — refill is just `tokens += rps * elapsed`, capped at burst. Implementing token bucket on Redis is doable but requires multiple Lua scripts or a Lua script that uses `TIME` for the refill arithmetic. Sliding window log maps to native Redis ops (`ZSET`, `ZADD`, `ZCARD`) so it's the cheapest distributed primitive.

You get the cleaner implementation each side. It's also true to production — most teams pick a different algorithm for the distributed limiter than the in-process one.

---

## 4. The Lua script, walked

```lua
-- KEYS[1] = bucket key
-- ARGV[1] = now (ms since epoch)
-- ARGV[2] = window (ms)
-- ARGV[3] = limit
-- ARGV[4] = unique member id for this request

local key    = KEYS[1]
local now    = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit  = tonumber(ARGV[3])
local member = ARGV[4]

-- 1. Prune anything that fell out of the window.
redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window)

-- 2. Count what's left.
local count = redis.call('ZCARD', key)

-- 3. Over limit? Compute retry-after from the oldest surviving entry.
if count >= limit then
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local retry = 0
    if oldest[2] then
        retry = (tonumber(oldest[2]) + window) - now
        if retry < 0 then retry = 0 end
    end
    return {0, 0, retry}
end

-- 4. Allowed. Record this request and refresh TTL.
redis.call('ZADD', key, now, member)
redis.call('PEXPIRE', key, window)
return {1, limit - (count + 1), 0}
```

Two things worth flagging:

- **`member` must be unique** within the set, else `ZADD` is a no-op and the counter stays. We pass a random per-request id from Go.
- **`PEXPIRE`** on every request means an idle bucket eventually expires — Redis cleans it up. Without it, a one-time visitor's bucket lives forever.

---

## 5. The middleware emits richer headers

Day 27 set `X-RateLimit-Limit` and `X-RateLimit-Remaining`. Day 33 keeps those plus adds:

- `X-RateLimit-Reset` — UNIX timestamp (seconds) when remaining returns to limit.
- `Retry-After` — only on 429, computed by the Lua script.

```
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 47
X-RateLimit-Reset: 1729000060   # only sent when remaining < limit
Retry-After: 12                  # only on 429
```

A polite client can pause for `Retry-After` seconds instead of mashing harder.

---

## 6. Idempotency keys — the protocol

A client retries a POST because the network blipped. Did the server already commit? Two outcomes if we don't think about it:

- **Server committed, response lost.** Client retries → server commits again → duplicate row.
- **Server didn't commit.** Client retries → fine.

The fix: clients send an `Idempotency-Key` header per attempt. Same key on retry → server returns the *original* response without re-running the handler.

```
POST /notes HTTP/1.1
Authorization: Bearer <tok>
Content-Type: application/json
Idempotency-Key: 5a2c1f3e-...

{"title":"buy milk"}
```

First attempt → server runs the handler, stores `(status, headers, body)` in Redis keyed by `idem:<userID>:<key>`, returns 201.

Second attempt with the same key → server fetches the stored response, replays it byte-for-byte, adds `Idempotent-Replayed: true`.

Third attempt with the same key but a *different body* → server returns **422** (the client is doing something wrong — same key should mean the same intent).

---

## 7. The Redis state for one idempotency key

```
idem:user:42:5a2c1f3e-...   (JSON, PX 24h)
{
  "state":     "done",            // "pending" while the handler runs
  "body_hash": "sha256:...",      // for mismatch detection
  "status":    201,
  "headers":   { "Content-Type": ["application/json"], "Location": ["/notes/7"] },
  "body":      "...base64..."
}
```

Three states matter:

| State | Meaning | Action |
| --- | --- | --- |
| `(missing)` | first time we see this key | `SET ... NX PX leaseTTL`; if NX succeeds, run the handler |
| `pending` | another request claimed the slot and is still running | 409 Conflict, "in flight" |
| `done` | response is cached | replay it |

The "race between two concurrent first-attempts" is solved by `SET key value NX PX leaseTTL`. Only one wins; the other sees `pending` and returns 409.

If the handler crashes mid-execution, the lease (default 60s) lets the slot reclaim. Without a lease, a crashed request would lock the key forever and the client could never retry.

---

## 8. The middleware shape

```go
func Middleware(store *Store) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Only mutating methods. GET retries are already idempotent.
            if !mutating(r.Method) || r.Header.Get("Idempotency-Key") == "" {
                next.ServeHTTP(w, r)
                return
            }

            // Read & restore body so the handler can still decode it.
            body, _ := readAndRestore(r)
            hash := sha256Hex(body)
            key  := scope(r.Context(), r.Header.Get("Idempotency-Key"))

            rec, err := store.Reserve(r.Context(), key, hash)
            // ErrPending  -> 409
            // ErrMismatch -> 422
            // rec != nil  -> replay
            // rec == nil  -> run handler, capture, store
        })
    }
}
```

The full code lives at [internal/idempotency/middleware.go](internal/idempotency/middleware.go).

Two implementation notes:

- **Body hash before the handler reads.** We read the body, hash it, and put a `bytes.Reader` back so the handler's `httpjson.DecodeJSON` still works.
- **Response capture is full buffering.** We use a `responseRecorder` that holds the body in a `bytes.Buffer` until the handler finishes; then we flush it to the client AND save it to Redis. For tiny JSON responses this is fine. For streaming responses (file downloads), idempotency makes less sense anyway.

---

## 9. Scoping by user

The idempotency key is generated by the client, not by us. Two users could pick the same value (UUIDs make this very unlikely, but you can't assume). So the Redis key is scoped by user:

```go
func scope(ctx context.Context, key string) string {
    if id, ok := auth.GetUserID(ctx); ok {
        return fmt.Sprintf("idem:user:%d:%s", id, key)
    }
    return fmt.Sprintf("idem:anon:%s", key)
}
```

We apply the middleware **after** the auth middleware, so by the time it runs `userID` is on the context. Unauthenticated POSTs (`/auth/login`) don't get idempotency in this codebase — they're rare enough that we don't bother.

---

## 10. What about transactions?

If the handler runs `tx.InTx` and the tx commits → the cache stores the response. Retry → replay. Correct.

If the tx **rolls back** → the response is still saved (it's a 500 with an error envelope). Retry → replays the 500. Slightly weird: the client retries and sees the same error, not a fresh attempt.

For most error categories (validation, conflict, not found) this is fine — they're deterministic on the body. For transient errors (DB blip) you might want to *not* cache 5xx responses. Day 33's middleware doesn't filter, but TASKS Task 4 has you add the filter.

---

## 11. What changed from Day 32

| File | Change |
| --- | --- |
| `internal/ratelimit/limiter.go` | **NEW** — `Limiter` interface + `Decision` |
| `internal/ratelimit/ratelimit.go` | renamed to `NewMemory(...)`, returns `Decision` |
| `internal/ratelimit/redis.go` | **NEW** — sliding window log w/ Lua script |
| `internal/middleware/ratelimit.go` | takes the interface; emits `X-RateLimit-Reset` |
| `internal/idempotency/store.go` | **NEW** — Redis store + Record + sentinel errors |
| `internal/idempotency/middleware.go` | **NEW** |
| `internal/config/config.go` | + `RateLimit.Backend`, `RateLimit.GlobalMaxPerMinute`, `RateLimit.AuthMaxPerMinute`, `Idempotency.TTL`, `Idempotency.LeaseTTL` |
| `main.go` | picks limiter backend; wires idempotency under the auth group |

Notes cache, sqlc, transactions, audit JOIN strategies — unchanged.

---

## 12. Run it

```powershell
cd Day_33_distributed_rate_limit_idempotency
docker compose up -d                  # postgres + redis
go mod init day33
go mod tidy
go run .
```

**Watch distributed rate limiting trip across replicas (using one process for the demo):**

```powershell
# RATE_LIMIT_BACKEND=redis is the default in .env; verify by reading the startup log
1..70 | ForEach-Object {
    $r = curl.exe -s -o $null -w "%{http_code} %{header_x-ratelimit-remaining}`n" http://localhost:8080/healthz
    Write-Host "$_  $r"
}
# Default: 60 req/minute. Expect 60 x 200 with remaining counting down, then 429s with Retry-After.
```

**Walk an idempotent POST:**

```powershell
$tok = (curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"a@b.dev\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json).access_token
$key = [guid]::NewGuid().ToString()

# First call: real handler runs, 201
curl.exe -i -H "Authorization: Bearer $tok" -H "Content-Type: application/json" -H "Idempotency-Key: $key" `
  -d "{\"title\":\"once\"}" http://localhost:8080/notes
# 201 Created, Location: /notes/<n>

# Same key, same body: replayed
curl.exe -i -H "Authorization: Bearer $tok" -H "Content-Type: application/json" -H "Idempotency-Key: $key" `
  -d "{\"title\":\"once\"}" http://localhost:8080/notes
# 201 Created, Idempotent-Replayed: true, Location: /notes/<same n>

# Same key, DIFFERENT body: 422
curl.exe -i -H "Authorization: Bearer $tok" -H "Content-Type: application/json" -H "Idempotency-Key: $key" `
  -d "{\"title\":\"different\"}" http://localhost:8080/notes
# 422 {"error":{"code":"IDEMPOTENCY_MISMATCH",...}}

# Inspect the saved record
docker exec -it day33-redis redis-cli get "idem:user:1:$key"
```

---

## 13. What's next

**Day 34 — OpenAPI / Swagger docs.** The API has grown to a real surface; documenting it as a machine-readable spec lets you generate SDKs, run contract tests, and stop the README walking from drifting out of sync.
