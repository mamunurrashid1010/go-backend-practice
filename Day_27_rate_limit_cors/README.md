# Day 27 — Rate Limiting + CORS

> **Goal:** stop abusive clients from drowning the API (rate limit), and let trusted browsers talk to it cross-origin without opening it to the entire web (CORS). Two small middlewares that are easy to get *almost* right and dangerous when wrong.

---

## 1. Why rate limit

Three real reasons:

1. **Brute force.** A login endpoint with no limiter is a credentials guesser's dream — millions of attempts a day cost the attacker pennies.
2. **Accidental abuse.** A frontend loop, a misbehaving worker, a forgotten retry — your own systems often DDOS you before strangers do.
3. **Cost / fairness.** One client mining your API shouldn't make the others slow.

The limiter isn't a security feature on its own — it's a circuit breaker that buys you time and lowers the blast radius.

---

## 2. Token bucket — the algorithm

`golang.org/x/time/rate` implements **token bucket**:

- A bucket holds at most `burst` tokens.
- Tokens are refilled at `rate` tokens per second.
- Each request consumes 1 token. If the bucket is empty, the request is denied.

The shape:
- `burst` = how big a sudden spike you tolerate.
- `rate` = the long-run average.

Example: `rate = 10/s`, `burst = 30`. A client can fire 30 requests *right now*, then 10/s after that. That handles a real human clicking through a UI without throttling, but stops a runaway script.

Compare to alternatives:
- **Fixed window** (`X requests per minute`): simple but suffers from the "edge surge" — a burst at 11:59:59 + 12:00:00 can deliver 2X in a second.
- **Sliding window log**: accurate but memory-heavy (one timestamp per request).
- **Token bucket**: O(1) state per client, allows bursts, smooths long-run. The winner for in-process limiting.

---

## 3. Per-client state — the map + TTL trick

You need one limiter **per client**. But the set of clients grows forever — if you naively keep them all, you leak memory.

```go
type Limiter struct {
    mu       sync.Mutex
    visitors map[string]*visitor
    ttl      time.Duration
}
type visitor struct {
    limiter  *rate.Limiter
    lastSeen time.Time
}
```

A background goroutine sweeps `visitors` periodically and drops anything not touched in `ttl`. The token bucket refills itself, so an evicted client just starts fresh — same as a brand-new one. No correctness loss.

In `internal/ratelimit/ratelimit.go`:

- `Allow(key)` — get or create, update `lastSeen`, return `limiter.Allow()`.
- `Tokens(key)` — read-only peek at remaining tokens for the `X-RateLimit-Remaining` header.
- A `cleanup()` goroutine started in `New(...)`.

---

## 4. What's the key?

Per **IP**? Per **user**? Both, depending on context.

**Unauthenticated routes** (`/auth/login`, `/auth/register`) — there's no user yet. Key by IP. Use a *tight* limit (e.g. 5 burst, 1 rps) because each attempt is a credential guess.

**Authenticated routes** — key by `user_id`. One user with 100 devices behind one office NAT shouldn't share a limit with the other 99 users at the same gateway.

A simple helper:

```go
func ClientKey(r *http.Request) string {
    if id, ok := auth.GetUserID(r.Context()); ok {
        return "user:" + strconv.FormatInt(id, 10)
    }
    return "ip:" + clientIP(r)
}
```

**On clientIP**: `r.RemoteAddr` is the TCP peer. Behind nginx / CloudFront / Cloudflare it's the proxy's IP — every client looks the same to you. Production deployments need to trust `X-Forwarded-For` (or the proxy's custom header) **only if the connection came from a trusted source**. Day 27's code uses `RemoteAddr` directly with a TODO for the proxy case; the TASKS make you wire `X-Forwarded-For` correctly.

---

## 5. Headers — talking to good clients

A polite limiter helps clients back off gracefully:

| Header | When | Meaning |
| --- | --- | --- |
| `X-RateLimit-Limit` | every request | the burst capacity |
| `X-RateLimit-Remaining` | every request | tokens left in the bucket |
| `Retry-After` | only on 429 | seconds until the next request would succeed |

The 429 body:

```json
{ "error": { "code": "RATE_LIMITED", "message": "too many requests" } }
```

These exist so a well-written client (or its SDK) can pause and retry instead of hammering harder when refused.

---

## 6. Two limiters, two routes

`main.go` wires **two** limiters with different config:

```go
globalLimiter := ratelimit.New(cfg.RateLimit.GlobalRPS, cfg.RateLimit.GlobalBurst, cfg.RateLimit.TTL)
authLimiter   := ratelimit.New(cfg.RateLimit.AuthRPS,   cfg.RateLimit.AuthBurst,   cfg.RateLimit.TTL)

r.Use(mw.RateLimit(globalLimiter))              // every route gets the broad limit
authH.Router(verifier, mw.RateLimit(authLimiter)) // /auth/login and /auth/register get the tight one too
```

So `/auth/login` is hit by **both** limiters — tight on the login route specifically, loose on the API generally. Credit gets consumed in both buckets — that's fine and intended.

---

## 7. CORS — the part everyone gets wrong

**The problem CORS solves.** A browser at `https://app.example.com` wants to call `https://api.example.com/notes` with the user's cookies/tokens. Without explicit permission from the API, the browser blocks the response. CORS is the API's way of saying *"yes, this origin is allowed."*

**What everyone gets wrong:** `Access-Control-Allow-Origin: *` everywhere. It works in development, looks fine in browser DevTools, and silently breaks the moment you set `Access-Control-Allow-Credentials: true` (the spec forbids wildcard + credentials). So people set wildcard, see the bug, and rather than fix it, do something worse — reflect any `Origin` header back. Now your API trusts *every* site on the internet.

**The right shape:**

1. Maintain an **allowlist** of origins (`http://localhost:3000`, `https://app.example.com`).
2. If the request's `Origin` is in the list, echo it in `Access-Control-Allow-Origin`. Otherwise, send nothing — the browser blocks.
3. Always set `Vary: Origin` so caches don't serve a response made for origin A back to origin B.
4. For preflight (`OPTIONS` with `Access-Control-Request-Method`), respond with the allowed methods/headers/max-age and `204 No Content`. Browsers cache the preflight for `Max-Age` seconds.

In code:

```go
if origin != "" && allowed[origin] {
    w.Header().Set("Access-Control-Allow-Origin", origin)
    if cfg.AllowCredentials {
        w.Header().Set("Access-Control-Allow-Credentials", "true")
    }
}
w.Header().Add("Vary", "Origin")
```

`Access-Control-Allow-Credentials: true` is what lets the browser send/receive cookies and `Authorization` headers cross-origin. It pairs with a specific origin only — never with `*`.

---

## 8. Preflight, briefly

A browser sends a preflight before a "non-simple" request — anything with custom headers (like `Authorization`), or methods like `PUT`/`PATCH`/`DELETE`. The preflight is `OPTIONS` with these headers:

```
Origin: http://localhost:3000
Access-Control-Request-Method: PATCH
Access-Control-Request-Headers: Authorization, Content-Type
```

The server replies with what it allows:

```
Access-Control-Allow-Origin: http://localhost:3000
Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
Access-Control-Allow-Headers: Authorization, Content-Type, X-Request-ID
Access-Control-Max-Age: 300
```

`Max-Age` is how long the browser caches the preflight result. 300s = 5 minutes — the browser won't preflight again until then.

Our middleware short-circuits preflight: if it sees `OPTIONS` + `Access-Control-Request-Method`, it writes the headers and returns `204` without invoking the next handler. The actual route is never touched on preflight.

---

## 9. Middleware order matters

```
RequestID  →  Logger  →  CORS  →  RateLimit  →  Recover  →  routes
```

Why this order:
- **RequestID first** — everything downstream needs to log it.
- **CORS before RateLimit** — preflight OPTIONS should answer instantly without spending tokens.
- **CORS before Recover** — CORS headers should still be on a 500 response, otherwise the browser hides the body.
- **RateLimit before Recover** — a 429 returns fast without touching handler code.

If you put rate limiting at the top, a flood of preflight OPTIONS counts against the bucket and a tiny burst from a real client trips it.

---

## 10. What changed from Day 26

| File | Change |
| --- | --- |
| `internal/ratelimit/ratelimit.go` | **NEW** — token bucket per key, with TTL eviction |
| `internal/ratelimit/ratelimit_test.go` | **NEW** — Allow / refill / eviction tests |
| `internal/middleware/ratelimit.go` | **NEW** — sets `X-RateLimit-*`, returns 429 on deny |
| `internal/middleware/cors.go` | **NEW** — allowlist-based, handles preflight |
| `internal/config/config.go` | + `RateLimit` and `CORS` sub-configs |
| `main.go` | builds two limiters, wires middleware in the right order |

Auth, notes, logging, respond, validate, migrations — unchanged.

---

## 11. Run it

```powershell
cd Day_27_rate_limit_cors
docker compose up -d
go mod init day27
# usual go gets, plus:
go get golang.org/x/time/rate
go run .
```

**Watch rate limiting trip:**

```powershell
# burst is 30, rate is 10/s — fire 50 quick requests
1..50 | ForEach-Object {
    $r = curl.exe -s -o $null -w "%{http_code}`n" http://localhost:8080/healthz
    "$_  $r"
}
# Expect ~30 x 200, then 429s, then occasional 200s as tokens refill.
```

**Watch CORS preflight:**

```powershell
curl.exe -i -X OPTIONS http://localhost:8080/notes `
  -H "Origin: http://localhost:3000" `
  -H "Access-Control-Request-Method: PATCH" `
  -H "Access-Control-Request-Headers: Authorization"
# 204 No Content, with Access-Control-Allow-* headers.

curl.exe -i -X OPTIONS http://localhost:8080/notes `
  -H "Origin: https://evil.example.com" `
  -H "Access-Control-Request-Method: PATCH"
# 204 No Content, but NO Access-Control-Allow-Origin — browser blocks.
```

---

## 12. What's next

**Day 28** — Week 4 close: a README + repo polish day. That's the final cleanup of "is this clonable by a stranger?" before Week 5 dives into transactions, indexes, sqlc, Redis, and OpenAPI docs.
