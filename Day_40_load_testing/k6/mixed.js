// Mixed — realistic per-endpoint distribution. This is what you'd
// defend in an SLA.
//
// 90% GET /notes/{id}   — the hot read path (Redis cache + PG fallback)
//  5% GET /notes         — cursor pagination (Postgres only, no cache)
//  5% POST /notes        — write path (tx + audit + cache invalidate)
//
// Run: k6 run k6/mixed.js

import http from 'k6/http'
import { check, sleep } from 'k6'
import { setupOne, authHeaders, jsonAuthHeaders } from './auth.js'

export const options = {
  vus: 50,
  duration: '2m',
  thresholds: {
    'http_req_duration{name:GetNote}':   ['p(95)<100'],
    'http_req_duration{name:ListNotes}': ['p(95)<200'],
    'http_req_duration{name:CreateNote}': ['p(95)<250'],
    'http_req_failed':                   ['rate<0.01'],
  },
}

export function setup() {
  const d = setupOne()
  // Seed a few notes so GetNote has real targets.
  const bodies = [
    { title: 'load-test seed 1', body: 'x' },
    { title: 'load-test seed 2', body: 'y' },
    { title: 'load-test seed 3', body: 'z' },
  ]
  const ids = []
  for (const b of bodies) {
    const res = http.post(
      `${d.baseURL}/notes`, JSON.stringify(b), jsonAuthHeaders(d.token),
    )
    if (res.status === 201) ids.push(res.json('id'))
  }
  return { ...d, seedIds: ids.length ? ids : [1] }
}

export default function (data) {
  const r = Math.random()

  if (r < 0.90) {
    // GET /notes/{id} — the hot path
    const id = data.seedIds[Math.floor(Math.random() * data.seedIds.length)]
    const res = http.get(
      `${data.baseURL}/notes/${id}`,
      { ...authHeaders(data.token), tags: { name: 'GetNote' } },
    )
    check(res, { 'get 200': (r) => r.status === 200 })
  } else if (r < 0.95) {
    // GET /notes — cursor pagination
    const res = http.get(
      `${data.baseURL}/notes?limit=20`,
      { ...authHeaders(data.token), tags: { name: 'ListNotes' } },
    )
    check(res, { 'list 200': (r) => r.status === 200 })
  } else {
    // POST /notes — write. Send Idempotency-Key to exercise that
    // middleware's write path.
    const key = `k6-${__VU}-${__ITER}`
    const res = http.post(
      `${data.baseURL}/notes`,
      JSON.stringify({ title: `k6 note ${__ITER}`, body: '' }),
      {
        headers: {
          'Authorization':    `Bearer ${data.token}`,
          'Content-Type':     'application/json',
          'Idempotency-Key':  key,
        },
        tags: { name: 'CreateNote' },
      },
    )
    check(res, { 'create 201': (r) => r.status === 201 })
  }

  // Small think-time — real users don't hammer.
  sleep(0.05)
}
