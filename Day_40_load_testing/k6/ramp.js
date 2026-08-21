// Ramp — 0 -> N VUs over T. Find the knee in the p95/p99 curve.
//
// Ramps are the ONLY way to see the shape of your capacity limit.
// A constant-N load tells you "what happens at N VUs"; a ramp tells
// you "where things start to hurt."
//
// Run: k6 run k6/ramp.js
//
// While it runs, in another shell:
//   go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
// (start pprof around the middle of the ramp, when things get interesting)

import http from 'k6/http'
import { check, sleep } from 'k6'
import { setupOne, authHeaders } from './auth.js'

export const options = {
  // Stages replace vus + duration. Each stage: ramp to `target` VUs
  // over `duration`. VUs hold at target until the next stage.
  stages: [
    { duration: '30s',  target: 20  }, // warm-up
    { duration: '1m',   target: 50  },
    { duration: '1m',   target: 100 },
    { duration: '1m',   target: 200 },
    { duration: '30s',  target: 0   }, // graceful ramp-down
  ],
  thresholds: {
    // These are aspirational — a ramp test WILL breach the p99 near
    // the knee. Leaving them so you see the FAIL line in output.
    'http_req_duration': ['p(95)<300', 'p(99)<1000'],
    'http_req_failed':   ['rate<0.05'],
  },
}

export function setup() { return setupOne() }

// The workload: create a note first so we have an id, then GET it
// repeatedly. The GET is what we're measuring — it exercises the
// Redis cache + Postgres fallback path from Day 32.
export default function (data) {
  // Once per iteration, pick a random note id in [1..100].
  // (The API's IDOR check will 404 anything the user doesn't own —
  // that IS still a measurable path. If you want only-owns, seed
  // 100 notes for this user in setup instead.)
  const id = Math.floor(Math.random() * 100) + 1

  const res = http.get(`${data.baseURL}/notes/${id}`, authHeaders(data.token))
  check(res, {
    'status is 200 or 404': (r) => r.status === 200 || r.status === 404,
  })

  // No sleep in a ramp — we WANT to saturate. Each VU issues
  // requests as fast as the API replies.
}
