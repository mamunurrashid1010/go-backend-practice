# Day 40 — Load Testing with `k6` (and `vegeta`)

> **Goal:** point `k6` at the Day 35 hardened notes API, run a baseline → ramp → mixed scenario, read the p50/p95/p99 curves, use Day 38/39's `pprof` to explain the bottleneck, fix it, re-measure. This is the skill: **measure → profile → hypothesize → fix → re-measure.**
>
> **The frame:** Days 36–39 taught you concurrency, ctx, pprof, benchmarks. Everything so far worked on tiny programs. Today it all points at a real service.

---

## 1. Install `k6`

Pick one:

```powershell
# Windows package manager (recommended)
winget install k6 --source=winget

# macOS
brew install k6

# Docker (no local install)
docker run --rm -i grafana/k6 run - < .\k6\baseline.js

# Or a static binary from https://k6.io/docs/get-started/installation/
```

Verify: `k6 version` → `k6 vX.Y.Z`.

Scripts are JavaScript (ES6 modules). The k6 runtime is written in Go; scripts are a thin DSL over an HTTP client + a metrics pipeline. There's zero JS at runtime cost — assertions and metrics happen in Go.

---

## 2. `k6` in three minutes

A script is one exported `default function`. Each **VU** (virtual user) runs that function in a tight loop for the test's duration.

```javascript
// baseline.js — 10 VUs pounding /healthz for 30s
import http from 'k6/http'
import { check } from 'k6/check'

export const options = {
  vus: 10,
  duration: '30s',
}

export default function () {
  const res = http.get('http://localhost:8080/healthz')
  check(res, { 'is 200': (r) => r.status === 200 })
}
```

Run:

```powershell
k6 run k6/baseline.js
```

k6 prints a live progress line, then a summary table:

```
     http_req_duration..............: avg=1.31ms  min=0.42ms  med=1.02ms  max=53ms   p(90)=2.11ms  p(95)=3.04ms
     http_reqs......................: 62823    2094/s
     iterations.....................: 62823    2094/s
     vus............................: 10       min=10  max=10
     vus_max........................: 10
     ✓ is 200
```

Six numbers to internalise:

- **`http_req_duration`** — the latency distribution. `avg`/`med`/`p(90)`/`p(95)` — the tail is where problems hide.
- **`http_reqs`** — throughput. `62823    2094/s` means 62823 total requests, 2094 per second.
- **`checks`** — how many assertions passed vs failed. A failed check is *usually* a broken response, not a slow one.
- **`http_req_failed`** — the transport-level error rate. TCP errors, 4xx/5xx if you configure it that way.

---

## 3. The three scenarios you actually run

### Baseline

```
Constant, low load. "What is this service doing when nothing is wrong?"
```

10 VUs, 30 seconds, one endpoint. Compare to *any* future run — regressions show up as ns/op-style slowdowns even at low load.

See [k6/baseline.js](k6/baseline.js).

### Ramp

```
0 → N VUs over T minutes, hold, ramp down. "Where does p99 turn upward?"
```

The **knee** in the p99 curve is your capacity. Below it: linear. Above it: exponential. Almost every real system has one.

See [k6/ramp.js](k6/ramp.js).

### Mixed

```
Realistic per-endpoint distribution. Reads dominate; occasional writes.
```

90% `GET /notes/{id}`, 5% `GET /notes`, 5% `POST /notes`. Roughly what a healthy CRUD app looks like. This is the number you'd defend in an SLA.

See [k6/mixed.js](k6/mixed.js).

---

## 4. Thresholds — build/fail the run on metrics

k6's real trick is `thresholds`. Attach them to any metric; the run fails if any threshold is breached.

```javascript
export const options = {
  vus: 50, duration: '1m',
  thresholds: {
    'http_req_duration{status:200}': ['p(95)<200', 'p(99)<500'],
    'http_req_failed':               ['rate<0.01'],
  },
}
```

CI-friendly: `k6 run` exits non-zero on threshold breach. Wire it into GitHub Actions (Day 42) and every PR gets checked against these SLOs.

---

## 5. The methodology — measure, profile, hypothesize, fix, re-measure

The whole day is this loop:

1. **Baseline.** `k6 run baseline.js`. Save the summary.
2. **Ramp.** `k6 run ramp.js`. Find the knee — the VU count where p95 crosses your target.
3. **Profile at the knee.** Start a ramp that holds *just below* the knee. In another shell:
   ```powershell
   go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
   (pprof) top
   ```
4. **Hypothesize** based on what pprof shows. "JSON encoding is 40% of CPU" → try `json.Encoder` over `json.Marshal`. "Postgres driver is the top allocator" → check for missing prepared statements or a hot cache miss.
5. **Fix ONE thing.**
6. **Re-run the same load test.** Compare `k6` summaries. If p95 didn't move, you fixed the wrong thing.

Never skip step 6. Every fix looks great in isolation; only the load test tells you if it moved the number that matters.

---

## 6. Reading a k6 run — what's normal, what's a smell

Rules of thumb for a small CRUD API on a laptop:

- **Baseline (10 VUs)**: p99 < 20ms. If it's 100ms+, your handler is doing something wrong at zero load.
- **Ramp knee**: expect it in the range 100–500 concurrent VUs against localhost Postgres. Below 50 is a red flag (per-request lock, external call, etc).
- **Failure rate**: `http_req_failed > 1%` under sustained normal load is a bug. Under overload, timeouts + 429s are expected.
- **Latency shape**: p95 close to avg = good; p95 5x avg = long-tail problem; p99 100x avg = you have a spike (GC, lock contention, upstream stall).

If p95/p99 are healthy but throughput is low, you're **bounded by concurrency** — not enough parallel work. Bump `MaxOpenConns`, add worker pools, remove hidden serialisation (a Mutex? one shared HTTP client?).

If throughput is high but p95/p99 balloon, you're **bounded by contention** — usually locks, GC, or the DB under load. Profile.

---

## 7. Common bottlenecks in the Day 35 API — before you even look

Predictions I'd make about the Day 35 notes API under 200 VUs:

- **JSON encoding at the response boundary.** The `respond.JSON` helper calls `json.NewEncoder(w).Encode(v)`. That's already the fast path (no throwaway `[]byte`), so it should stay below the top 5.
- **Postgres driver** on the read path. Even with the Redis cache from Day 32, cache misses go to Postgres. If the composite index from Day 26 isn't hit, ~everything shows up here.
- **The idempotency middleware** on POST. The Redis SETNX + JSON marshal/unmarshal of the record happens on every write. Bench.
- **`bcrypt.CompareHashAndPassword`** on `/auth/login`. Bcrypt is intentionally slow. If you load-test the login endpoint, this dominates. That's *by design* — but it means `POST /auth/login` has a completely different perf shape from `GET /notes/{id}`.
- **`sqlc`-generated code**. No overhead vs hand-written; if this shows up it's the *query* that's slow, not sqlc.

You won't know until you profile. But you'll be surprised how often "I know exactly why it's slow" is wrong.

---

## 8. `vegeta` — the pure-Go alternative

k6 wins for scenarios; `vegeta` wins for "hammer this URL for N seconds and give me a histogram." No JS, no scenarios, pipes fine into shell:

```powershell
echo "GET http://localhost:8080/healthz" | vegeta attack -rate=1000 -duration=30s | vegeta report
```

Output:

```
Requests      [total, rate, throughput]         30000, 1000.02, 1000.02
Duration      [total, attack, wait]             30.001s, 30s, 1.4ms
Latencies     [min, mean, 50, 90, 95, 99, max]  0.15ms, 0.87ms, 0.7ms, 1.4ms, 1.8ms, 3.4ms, 42.6ms
Bytes In      [total, mean]                     450000, 15.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:30000
```

Same numbers, cleaner CLI. Use it for one-off "how fast is this endpoint?" checks; use `k6` when you need scenarios, thresholds, per-endpoint mixes, and CI integration.

See [vegeta/README.md](vegeta/README.md).

---

## 9. What changed from Day 39

Nothing carries as Go code. Today is applying skills to the existing Day 35 hardened notes API. The k6 scripts here are the deliverable.

---

## 10. Run it

```powershell
# 1. Start the notes API (Day 35 folder)
cd ..\Day_35_hardened_notes_api
docker compose up -d
go run .
# API on http://localhost:8080, docs at /docs/, pprof at /debug/pprof/

# 2. Run the baseline in a second shell
cd ..\Day_40_load_testing
k6 run k6/baseline.js

# 3. Register a test user so authed scripts have credentials
curl.exe -H "Content-Type: application/json" `
  -d "{\"email\":\"loadtest@example.com\",\"password\":\"hunter2pass\"}" `
  http://localhost:8080/auth/register

# 4. Run the ramp and watch the p99 curve
k6 run k6/ramp.js

# 5. Run the mixed scenario — closest to reality
k6 run k6/mixed.js

# 6. During the mixed run, in a third shell, capture a CPU profile
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
(pprof) top
```

---

## 11. Prerequisite: `/debug/pprof/*` on the notes API

If your Day 35 notes API doesn't have pprof endpoints yet (Day 39 Task 5 walked adding them), do that first. The two-line version:

```go
import _ "net/http/pprof"

// ...somewhere in your router setup:
r.Mount("/debug", middleware.Profiler())   // chi-middleware helper
// or explicit:
r.Get("/debug/pprof/", pprof.Index)
r.Get("/debug/pprof/profile", pprof.Profile)
r.Get("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
r.Get("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
```

**Gate behind auth in prod.** These endpoints leak stack traces and can be DOS'd by an attacker holding `/debug/pprof/profile?seconds=3600`.

---

## 12. What's next

**Day 41 — `golangci-lint`, `staticcheck`, `gofumpt`, pre-commit hooks, `air` for hot reload.** The developer-experience layer that catches perf traps + bugs at edit time, not test time.
