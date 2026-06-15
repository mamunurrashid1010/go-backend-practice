# Day 27 — Practice Tasks

The code gives you working rate limiting + CORS. The tasks make you *feel* them tripping, then tighten the parts that are easy to get wrong in production.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day27
> go get github.com/go-chi/chi/v5
> go get github.com/jackc/pgx/v5/stdlib
> go get github.com/jackc/pgx/v5/pgconn
> go get -tags 'postgres' github.com/golang-migrate/migrate/v4
> go get github.com/golang-migrate/migrate/v4/database/postgres
> go get github.com/golang-migrate/migrate/v4/source/file
> go get github.com/joho/godotenv
> go get github.com/caarlos0/env/v11
> go get github.com/go-playground/validator/v10
> go get golang.org/x/crypto/bcrypt
> go get github.com/golang-jwt/jwt/v5
> go get golang.org/x/time/rate
> go run .
> ```

---

## Warm-up — make the global limit trip

- [ ] With the defaults (`GLOBAL_RPS=10`, `GLOBAL_BURST=30`), fire 60 requests fast at `/healthz`:
  ```powershell
  1..60 | ForEach-Object {
      $r = curl.exe -s -o $null -w "%{http_code}`n" http://localhost:8080/healthz
      Write-Host "$_  $r"
  }
  ```
- [ ] Expect ~30 × `200`, then `429` until the bucket refills. Confirm the 429 body is the standard envelope:
  ```json
  { "error": { "code": "RATE_LIMITED", "message": "too many requests" } }
  ```
- [ ] Read the response headers on a successful call — confirm `X-RateLimit-Limit: 30` and a decreasing `X-RateLimit-Remaining`:
  ```powershell
  curl.exe -s -D - -o $null http://localhost:8080/healthz | Select-String -Pattern "X-RateLimit"
  ```

---

## Task 1 — Tight auth limiter

- [ ] With defaults (`AUTH_BURST=5`), fire 10 logins for the same user:
  ```powershell
  1..10 | ForEach-Object {
      $r = curl.exe -s -o $null -w "%{http_code}`n" -H "Content-Type: application/json" `
        -d "{\"email\":\"x@y.dev\",\"password\":\"wrong\"}" http://localhost:8080/auth/login
      Write-Host "$_  $r"
  }
  ```
- [ ] Expect at most ~5 attempts to even reach the handler (and return 401) before `429`s start.
- [ ] Read the README §6 again and explain to yourself why a single login attempt consumes one token from BOTH limiters — the global one *and* the auth one.

---

## Task 2 — Independence of keys

The limiter is keyed per-IP (unauthed) or per-user (authed). Two users at the same machine should be independent once they're logged in.

- [ ] Register and log in user A and user B.
- [ ] Hammer `/notes` as A → A hits 429.
- [ ] Immediately make a request as B → B is fine (B has their own bucket).

Confirm this is true. If B is also throttled, the limiter is keying by IP for authenticated requests — which would be wrong. (It shouldn't be; the code keys by user_id when authed. This task is just verification.)

---

## Task 3 — CORS preflight

Two requests, same path, different `Origin`:

- [ ] **Allowed origin:**
  ```powershell
  curl.exe -i -X OPTIONS http://localhost:8080/notes `
    -H "Origin: http://localhost:3000" `
    -H "Access-Control-Request-Method: PATCH" `
    -H "Access-Control-Request-Headers: Authorization"
  ```
  Expect `204 No Content`, `Access-Control-Allow-Origin: http://localhost:3000`, `Allow-Methods`, `Allow-Headers`, `Max-Age: 300`.
- [ ] **Disallowed origin:**
  ```powershell
  curl.exe -i -X OPTIONS http://localhost:8080/notes `
    -H "Origin: https://evil.example.com" `
    -H "Access-Control-Request-Method: PATCH"
  ```
  Expect `204`, but **no** `Access-Control-Allow-Origin` header. The browser will block the actual request.
- [ ] Confirm every response (allowed or not) carries `Vary: Origin`. (Why? See README §7.)

---

## Task 4 — Trust X-Forwarded-For (the right way)

`r.RemoteAddr` is the TCP peer. Behind nginx/Cloudflare, that's the proxy — every real client looks the same to your limiter, so a tight per-IP limit becomes a global limit.

- [ ] Add `TrustedProxies []string` to `RateLimitConfig` — a comma-separated list of CIDRs (e.g. `10.0.0.0/8,172.16.0.0/12`).
- [ ] In `middleware/ratelimit.go`, change `clientIP`:
  ```go
  func clientIP(r *http.Request, trusted []*net.IPNet) string {
      host, _, _ := net.SplitHostPort(r.RemoteAddr)
      ip := net.ParseIP(host)
      if !inTrusted(ip, trusted) {
          return host // direct connection — RemoteAddr is the real client
      }
      // We trust this hop. Walk X-Forwarded-For RIGHT to LEFT,
      // stopping at the first IP that isn't itself trusted.
      xff := r.Header.Get("X-Forwarded-For")
      parts := strings.Split(xff, ",")
      for i := len(parts) - 1; i >= 0; i-- {
          candidate := strings.TrimSpace(parts[i])
          if cip := net.ParseIP(candidate); cip != nil && !inTrusted(cip, trusted) {
              return candidate
          }
      }
      return host
  }
  ```
- [ ] Test by sending `X-Forwarded-For: 1.2.3.4, 10.0.0.1` from `127.0.0.1`:
  - If `127.0.0.1` is in `TrustedProxies`, the limiter keys by `1.2.3.4`.
  - If it isn't, the limiter keys by `127.0.0.1` and ignores XFF entirely.

**Why this matters:** if you blindly trust `X-Forwarded-For` from anyone, a malicious client just sends `X-Forwarded-For: 8.8.8.8` and gets a fresh bucket on every request. Rate limit defeated.

---

## Task 5 — Add a per-route override

A single API often wants different limits per resource (`POST /notes` is cheap, `POST /uploads` is expensive).

- [ ] Build a *third* limiter with a strict `(1 rps, 3 burst)` and apply it only to `POST /notes`:
  ```go
  notesPostLimiter := ratelimit.New(1, 3, cfg.RateLimit.TTL)
  defer notesPostLimiter.Stop()
  // inside the /notes group:
  r.With(mw.RateLimit(notesPostLimiter)).Post("/", notesHandler.create)
  ```
- [ ] Verify `GET /notes` is still loose but `POST /notes` is tight.

---

## Task 6 — Why the middleware order matters

Reorder middleware to put `RateLimit` *before* `CORS`, restart, and try the preflight as a disallowed origin:

```powershell
1..100 | ForEach-Object {
    curl.exe -s -o $null -w "%{http_code} " -X OPTIONS http://localhost:8080/notes `
      -H "Origin: https://evil.example.com" `
      -H "Access-Control-Request-Method: PATCH"
}
```

- [ ] Notice each preflight burns a token. After ~30 requests you get `429`s. Now a *legit* user behind the same IP — who shares the bucket — gets locked out by traffic from a malicious cross-origin browser.
- [ ] Put the order back (CORS first). The preflight now short-circuits at 204 *before* the limiter sees it.

Write one bullet in "What I learned" about why CORS belongs in front of RateLimit.

---

## Task 7 — Tests

- [ ] `go test ./internal/ratelimit/...` — confirm the 5 tests pass.
- [ ] `go test ./internal/middleware/...` — confirm the 4 CORS tests pass.
- [ ] Add a test for `middleware.RateLimit`:
  - Build a limiter with `(rate=10, burst=2)`.
  - Make 3 quick `httptest.NewRequest` calls through a stack of `[RateLimit, ok handler]`.
  - Assert the third response is `429` with the standard error envelope and `X-RateLimit-Remaining: 0`.

---

## Stretch — only if you're flying

- [ ] **Distributed limiter** (preview of Day 33): swap the in-process map for a Redis-backed limiter. The shape of the middleware doesn't change — only the implementation behind the `Limiter` type.
- [ ] **Wait, don't deny**: `rate.Limiter` has `Wait(ctx)` which blocks until a token is available. Build a "slow but accept" variant for low-importance endpoints, and confirm the 429 path still trips when `ctx` is cancelled.
- [ ] **Burnt-token logging**: when the limiter denies, log `slog.Warn("rate_limit_exceeded", "key", key)`. Now you can spot abusers in your structured logs.
- [ ] **Static asset bypass**: skip rate limiting for `GET /healthz` so a noisy load balancer can't make you 429 yourself.

---

## What I learned (Day 27)

> 3 bullets in your own words.

-
-
-
