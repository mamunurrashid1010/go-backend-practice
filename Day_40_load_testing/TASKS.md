# Day 40 — Practice Tasks

The scripts give you shapes to run. These tasks force the loop: measure → profile → hypothesize → fix → re-measure. The whole day is that loop.

> **Prerequisites:**
>
> 1. Day 35's hardened notes API running: `cd ..\Day_35_hardened_notes_api ; docker compose up -d ; go run .`
> 2. `net/http/pprof` mounted on the API (Day 39 Task 5 walked adding it).
> 3. `k6` installed (`winget install k6` or `docker run --rm -i grafana/k6 run - < script.js`).
> 4. Register a load-test user:
>    ```powershell
>    curl.exe -H "Content-Type: application/json" `
>      -d "{\"email\":\"loadtest@example.com\",\"password\":\"hunter2pass\"}" `
>      http://localhost:8080/auth/register
>    ```

---

## Warm-up — run the three scripts

- [ ] `k6 run k6/baseline.js` — 10 VUs, 30s of `/healthz`. Save the summary. Expected: p99 < 20ms.
- [ ] `k6 run k6/ramp.js` — 0 → 200 VUs over 5 minutes. **Watch the live p95** in the k6 progress line. Note the VU count where it crosses 300ms.
- [ ] `k6 run k6/mixed.js` — the realistic scenario. Read the per-tag `http_req_duration` breakdown at the bottom of the summary.

---

## Task 1 — Bump the rate limit so it doesn't dominate

Day 33's default is 60 req/min *per user*. `mixed.js` at 50 VUs will trip that in seconds. Either:

- [ ] Set `RATE_LIMIT_GLOBAL_MAX_PER_MINUTE=100000` in the notes API's `.env` and restart. You're now measuring the API, not the limiter.
- [ ] Or set `RATE_LIMIT_BACKEND=memory` with a big burst — same effect.

Rerun the mixed script. Latencies should be genuinely lower now (the limiter was itself adding a Redis round-trip per request).

---

## Task 2 — Find the knee in the ramp

- [ ] Run `k6/ramp.js`. In the k6 output stages, note the VU count where `http_req_duration...p(95)` starts climbing sharply.
- [ ] On your laptop against docker-compose Postgres, expect the knee somewhere in the 100–300 VU range. Wildly different numbers on different hardware — that's fine.
- [ ] Reflect: what saturates first? Postgres connections (`db.Stats().InUse` at limit)? CPU? Redis? Go's scheduler?

The knee tells you your **capacity** with today's config. Anything you change (pool size, cache TTL, index, algorithm) should move the knee.

---

## Task 3 — Profile at the knee

While a ramp is running and holding NEAR (not above) the knee, capture a CPU profile:

```powershell
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
(pprof) top
(pprof) list <top function>
```

- [ ] Write down the top 5 by cum time.
- [ ] Predictions to check:
  - Postgres driver (pgx) in the top 5.
  - JSON encoding around `respond.JSON`.
  - Redis client for `GET /notes/{id}` cache path.
  - `crypto/sha256` from idempotency middleware on POSTs.
  - `runtime.mallocgc` — if this is huge you have an allocation problem; grab a heap profile next.

---

## Task 4 — Grab a heap profile too

Same load, different profile:

```powershell
go tool pprof -alloc_space http://localhost:8080/debug/pprof/heap
(pprof) top
```

- [ ] Top allocators — expect `encoding/json` and the pgx driver.
- [ ] If a *handler* function is near the top, that handler is doing something wasteful; go read it.

Compare `-alloc_space` (churn) vs `-inuse_space` (currently held):

```powershell
go tool pprof -inuse_space http://localhost:8080/debug/pprof/heap
(pprof) top
```

- [ ] `inuse_space` should be *small* on a healthy service — most allocations are short-lived and collected. If it's growing during the run, you have a leak (probably a goroutine holding a big buffer — see Day 38).

---

## Task 5 — Fix ONE thing, re-measure

Pick one hypothesis from Tasks 3–4. Change ONE thing. Re-run `mixed.js` with the same seed and count. Compare summaries.

Examples of one-thing fixes:

- **Notes cache TTL up.** `REDIS_NOTES_TTL=1h`. Hit rate goes up; PG load goes down.
- **Pool bigger.** `DB_MAX_OPEN_CONNS=50`. If you were connection-starved, throughput climbs.
- **Pre-size a common allocation.** Find a spot in the notes handler chain where a `make([]X)` doesn't set capacity; set it.
- **Skip a middleware.** Turn off the idempotency middleware on GET-only workloads (it already does; verify with the profile).

Then: **did the p95 in the mixed run actually move?** If not, you fixed the wrong thing. Revert. Try a different hypothesis.

**Never leave a fix in place because it "should" help.** The load test tells you if it did.

---

## Task 6 — CI-fail on a threshold

- [ ] Take your best mixed-run p95 and set it as the threshold in `mixed.js`:
  ```javascript
  thresholds: {
    'http_req_duration{name:GetNote}': ['p(95)<80'],
  }
  ```
- [ ] Deliberately regress the API — add `time.Sleep(50 * time.Millisecond)` in `GetNote`'s handler. Rerun `mixed.js`. The threshold breach shows up as `FAILED` in the summary and `k6` exits non-zero.
- [ ] Undo the sleep. Day 42's GitHub Actions CI runs this exact check on every PR.

---

## Task 7 — Same test in vegeta

- [ ] Install: `go install github.com/tsenart/vegeta@latest`.
- [ ] Same GET /healthz baseline in vegeta at 1000 req/s for 30s:
  ```powershell
  echo "GET http://localhost:8080/healthz" | vegeta attack -rate=1000 -duration=30s | vegeta report
  ```
- [ ] Compare the latency histogram to k6's baseline. Different tools, same numbers within noise.
- [ ] Try `vegeta plot` to save an HTML latency chart.

---

## Stretch — only if you're flying

- [ ] **Coordinated omission.** `k6`'s closed-model default suffers from CO — a slow request makes the next VU issue *later*, hiding the tail. Rerun with `constant-arrival-rate` executor (open model) and compare the p99. It'll be worse — that's the truth.
- [ ] **Compare backends.** Run `mixed.js` twice: once with `RATE_LIMIT_BACKEND=memory`, once with `redis`. Difference is the Redis round-trip cost per request, at the p50 and at the p99. Redis under load will show up.
- [ ] **`k6` cloud output.** Grafana Cloud has a free tier for k6. Same script, run against your local API, results uploaded and stored. Nice charts, historical view, PR comment integration.
- [ ] **Profile a benchmark, not just live traffic.** Take the *strings* or *heap* benchmark from Day 39, capture a CPU profile, drive `pprof` — same skill, offline reproducible.

---

## What I learned (Day 40)

> 3 bullets in your own words.

-
-
-
