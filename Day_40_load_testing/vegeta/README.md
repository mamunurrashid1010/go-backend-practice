# vegeta — the pure-Go alternative

For "hammer this endpoint for N seconds and show me the histogram," `vegeta` is dead simple. No JS, no scenarios, pipes fine into shell.

## Install

```powershell
go install github.com/tsenart/vegeta@latest
vegeta -version
```

## Run

Simplest form — a single URL:

```powershell
echo "GET http://localhost:8080/healthz" | vegeta attack -rate=1000 -duration=30s | vegeta report
```

Sample output:

```
Requests      [total, rate, throughput]         30000, 1000.02, 1000.02
Duration      [total, attack, wait]             30.001s, 30s, 1.4ms
Latencies     [min, mean, 50, 90, 95, 99, max]  0.15ms, 0.87ms, 0.7ms, 1.4ms, 1.8ms, 3.4ms, 42.6ms
Bytes In      [total, mean]                     450000, 15.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:30000
```

Note: `vegeta` uses **open-model** load (fixed rate, requests keep issuing regardless of latency). `k6` defaults to **closed-model** (fixed VUs, each waits for the prior response). Both are valid; they measure different things:

- **Open** — "what latency does the service produce at X req/s?" More like real traffic.
- **Closed** — "what throughput can the service produce with X concurrent users?" More like a load test.

## Multi-endpoint targets

Put multiple URLs into a file, one per line:

```
# targets.txt
GET http://localhost:8080/healthz
GET http://localhost:8080/notes/1
Authorization: Bearer eyJhbGci...

POST http://localhost:8080/notes
Authorization: Bearer eyJhbGci...
Content-Type: application/json
@body.json
```

`vegeta` cycles through the entries at the given rate.

## Histograms and reports

```powershell
echo "GET http://localhost:8080/healthz" `
  | vegeta attack -rate=500 -duration=30s -output=results.bin

# Text report — same summary as before
vegeta report results.bin

# Latency histogram, bucketed
vegeta report -type='hist[0,10ms,50ms,100ms,500ms]' results.bin

# HTML report — save to file, open in browser
vegeta plot results.bin > report.html
```

## When to pick which

| Situation | Tool |
| --- | --- |
| "Give me a p99 for this URL, fast" | `vegeta` |
| "Simulate 90% GET / 10% POST across auth" | `k6` |
| "Fail the CI if p95 > 200ms" | `k6` (thresholds) |
| "Open-model traffic at a fixed rate" | `vegeta` |
| "Closed-model N concurrent VUs" | `k6` (default) or `vegeta` with `-workers` |
| "Latency histogram as HTML" | `vegeta plot` |

Most teams use both. `vegeta` for quick "how fast is this?" checks; `k6` for the actual test suite.
