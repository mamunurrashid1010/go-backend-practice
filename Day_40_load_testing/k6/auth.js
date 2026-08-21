// Shared helper — login once per VU, return the access token.
// Every authed script imports this and calls loginOnce() from setup
// (or from the top of default() and caches on globalThis).

import http from 'k6/http'
import { check, fail } from 'k6'

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080'
const EMAIL    = __ENV.LOAD_EMAIL || 'loadtest@example.com'
const PASSWORD = __ENV.LOAD_PASSWORD || 'hunter2pass'

// Login and return an access token. Call this once per VU from setup;
// don't hit /auth/login every request — it does bcrypt (intentionally
// slow) and would dominate every profile.
export function login() {
  const res = http.post(
    `${BASE_URL}/auth/login`,
    JSON.stringify({ email: EMAIL, password: PASSWORD }),
    { headers: { 'Content-Type': 'application/json' } },
  )
  check(res, { 'login 200': (r) => r.status === 200 })
    || fail(`login failed: ${res.status} ${res.body}`)
  return res.json('access_token')
}

// Setup runs ONCE for the whole test; the return value is passed as
// the first arg to default(). Use it for anything that shouldn't be
// timed as part of the workload.
export function setupOne() {
  const token = login()
  return { token, baseURL: BASE_URL }
}

export function authHeaders(token) {
  return { headers: { 'Authorization': `Bearer ${token}` } }
}

export function jsonAuthHeaders(token) {
  return {
    headers: {
      'Authorization':   `Bearer ${token}`,
      'Content-Type':    'application/json',
    },
  }
}
