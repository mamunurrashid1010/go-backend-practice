# Day 33 — Practice Tasks

The code ships both the distributed limiter and the idempotency middleware fully wired. The tasks make you *see* both work, then poke the edges (Lua atomicity, body-mismatch, lease expiry).

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day33
> go mod tidy
> go run .
> ```

Keep a `redis-cli MONITOR` open in a second terminal for most of these:

```powershell
docker exec -it day33-redis redis-cli MONITOR
```

---

## Warm-up — see the limit hold

Default config: `RATE_LIMIT_BACKEND=redis`, 60 req/min global.

- [ ] Fire 70 healthchecks:
  ```powershell
  1..70 | ForEach-Object {
      $r = curl.exe -s -o $null -w "%{http_code} R=%{header_x-ratelimit-remaining}`n" http://localhost:8080/healthz
      "$_  $r"
  }
  ```
- [ ] Expect 60 x 200 with X-RateLimit-Remaining counting down to 0, then 429 with `Retry-After`. After ~60s the bucket reopens.

---

## Task 1 — Compare the two backends

- [ ] Set `RATE_LIMIT_BACKEND=memory` in `.env`, restart. Now you're using Day 27's token bucket (`burst=30`, `rps=10`).
- [ ] Fire the 70-burst again. Notice:
  - 30 succeed almost instantly (the burst).
  - The next ~30 trickle in at 10/s as the bucket refills.
  - Last 10 get 429 only briefly.
- [ ] Switch back to `redis`. Sliding-window log has *no* burst — 60 in the window, then a hard 429 until the window slides.

Both are "60 ish per minute," but the shape of the curve is different. Token bucket smooths a one-off spike; sliding-window-log keeps the in-window count strict.

---

## Task 2 — Idempotent POST round-trip

- [ ] Log in, save `$tok`.
- [ ] Generate a key: `$key = [guid]::NewGuid().ToString()`.
- [ ] First POST → 201:
  ```powershell
  curl.exe -i -H "Authorization: Bearer $tok" -H "Content-Type: application/json" -H "Idempotency-Key: $key" `
    -d "{\"title\":\"once\"}" http://localhost:8080/notes
  ```
- [ ] Same key, same body → replayed (`201`, header `Idempotent-Replayed: true`):
  ```powershell
  curl.exe -i -H "Authorization: Bearer $tok" -H "Content-Type: application/json" -H "Idempotency-Key: $key" `
    -d "{\"title\":\"once\"}" http://localhost:8080/notes
  ```
- [ ] Inspect Redis:
  ```powershell
  docker exec -it day33-redis redis-cli get "idem:user:1:$key"
  ```

---

## Task 3 — Body mismatch

- [ ] Reuse the same `$key` but a different body:
  ```powershell
  curl.exe -i -H "Authorization: Bearer $tok" -H "Content-Type: application/json" -H "Idempotency-Key: $key" `
    -d "{\"title\":\"different\"}" http://localhost:8080/notes
  ```
- [ ] Expect `422 IDEMPOTENCY_MISMATCH`.

Why 422 not 409? 409 is "you tried to do conflicting work" (use it for "in flight"). 422 is "the request is well-formed but semantically wrong" — and reusing an idempotency key for a different body is a client bug, full stop.

---

## Task 4 — Filter 5xx responses out of the cache

When a handler returns 500, today the middleware happily caches that response. The next retry sees the same 500 instantly — annoying because the failure might be transient.

- [ ] In [idempotency/middleware.go](internal/idempotency/middleware.go), wrap the `store.Save` call with a status check:
  ```go
  if status < 500 {
      store.Save(...)
  } else {
      // Release the pending slot so the client can retry afresh.
      store.Release(r.Context(), scoped)
  }
  ```
- [ ] Test by temporarily injecting an error in `notes.Service.Create` so POST /notes returns 500. Confirm a retry actually runs the handler again instead of replaying the 500.
- [ ] Revert the injected error and the change too — or keep the filter if you like it (it's a defensible default).

---

## Task 5 — Force the lease expiry

If the handler crashes mid-execution, the `pending` lease should let a retry reclaim the slot after `IDEMPOTENCY_LEASE_TTL`.

- [ ] Set `IDEMPOTENCY_LEASE_TTL=5s` in `.env`, restart.
- [ ] In `notes.Service.Create`, inject a `time.Sleep(15 * time.Second)` before the audit insert. Restart.
- [ ] Fire a POST with a fresh key. While it's sleeping, fire the *same* POST in another shell. Expect 409 (in-flight).
- [ ] Wait 10 seconds and retry. Expect 200/201 — the lease expired so the slot was reclaimed.
- [ ] Revert.

---

## Task 6 — Atomicity test for the Lua script

Open two `redis-cli` sessions. Run the sliding-window-log script manually:

```
EVAL "
  redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', tonumber(ARGV[1]) - tonumber(ARGV[2]))
  local count = redis.call('ZCARD', KEYS[1])
  if count >= tonumber(ARGV[3]) then return 0 end
  redis.call('ZADD', KEYS[1], tonumber(ARGV[1]), ARGV[4])
  return 1
" 1 testkey <NOW_MS> 60000 3 m1
```

- [ ] Fire it 3 times → all return 1. Fire again → 0 (over limit).
- [ ] In a tight loop from a script, fire concurrently — confirm Redis serializes them and the count is exact.

---

## Stretch — only if you're flying

- [ ] **Token bucket on Redis.** Implement a token-bucket Lua script for parity with the in-process limiter. Hint: store `(tokens, last_refill_ms)`; refill by `(now - last_refill) * rps / 1000`. Use Redis's `TIME` command.
- [ ] **Per-route limits.** Add a strict limiter on `POST /notes` only (e.g. 20/min) using a *third* `Redis` instance with a different prefix. Apply via `r.With(mw.RateLimit(...))`.
- [ ] **Idempotency on auth/register.** Currently we only scope by user — but `/register` has no user yet. Try scoping by IP + key for the register route. What's the security implication?
- [ ] **Replay metric.** Count `Idempotent-Replayed: true` responses in a counter (Day 90's Prometheus preview). High replay rate ≈ network is bad for clients.

---

## What I learned (Day 33)

> 3 bullets in your own words.

-
-
-
