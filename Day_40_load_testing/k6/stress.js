// Stress — hold at high VU for a while. Two questions:
//   1) Does throughput STAY at the peak, or does it degrade over time?
//      (Slow degradation = memory leak, connection leak, GC ramp.)
//   2) When you ramp back down, does latency recover?
//      (No recovery = a piece of state didn't reset. Suspicious.)
//
// Run: k6 run k6/stress.js

import http from 'k6/http'
import { check } from 'k6'
import { setupOne, authHeaders } from './auth.js'

export const options = {
  stages: [
    { duration: '30s',  target: 200 }, // ramp up hard
    { duration: '2m',   target: 200 }, // hold — watch for degradation
    { duration: '30s',  target: 0   },
  ],
  thresholds: {
    // Under stress, higher tolerances. The point isn't "did it stay
    // fast", it's "did it stay UP and CORRECT."
    'http_req_failed': ['rate<0.10'],
  },
}

export function setup() { return setupOne() }

export default function (data) {
  const res = http.get(`${data.baseURL}/notes/1`, authHeaders(data.token))
  check(res, {
    'not 5xx': (r) => r.status < 500,
    // 404s and 429s (rate limit) are fine under stress; 5xx is a bug.
  })
}

// After the run, watch two things in the summary:
//   * `http_reqs` per second over time (in the console output, look
//     at the middle two minutes — should be flat, not sawtooth).
//   * `http_req_duration` p99 — if it climbs during the hold, you
//     have a leak somewhere (goroutines, memory, DB connections).
//
// Then hit /debug/pprof/goroutine to check the goroutine count —
// under sustained stress, it should plateau at some steady-state N,
// not climb linearly.
