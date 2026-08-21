// Baseline — constant low load. What does the API do when nothing
// is wrong? Compare EVERY future run to this.
//
// Run: k6 run k6/baseline.js
//
// Change BASE_URL via env: k6 run -e BASE_URL=http://api.dev.local k6/baseline.js

import http from 'k6/http'
import { check, sleep } from 'k6'

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080'

export const options = {
  vus: 10,
  duration: '30s',
  thresholds: {
    'http_req_duration': ['p(95)<50', 'p(99)<200'],
    'http_req_failed':   ['rate<0.01'],
  },
}

export default function () {
  const res = http.get(`${BASE_URL}/healthz`)
  check(res, {
    'status is 200':      (r) => r.status === 200,
    'body has status ok': (r) => r.body && r.body.includes('ok'),
  })
  // A tiny think-time keeps each VU under 100 req/s. Otherwise the
  // baseline turns into a stress test at low VU count.
  sleep(0.1)
}
