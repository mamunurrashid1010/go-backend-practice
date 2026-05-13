# Day 4 — Practice Tasks

Each task makes the error story tighter. Type the code; don't paste.

> **Before you start:** `go mod init day04 && go run .`, then hit each route once with `curl.exe -i` and confirm every response is JSON and has the same envelope.

---

## Warm-up (observe what already exists)

- [ ] `GET /users/abc` — `400 BAD_REQUEST` JSON.
- [ ] `GET /users/999` — `404 NOT_FOUND` JSON.
- [ ] `GET /banana` — `404 NOT_FOUND` JSON (the catch-all).
- [ ] `PUT /users/1` — `405 METHOD_NOT_ALLOWED` JSON with an `Allow` header (Go 1.22's free 405).
- [ ] `GET /panic` — `500 INTERNAL` JSON. Look at your server logs: the real panic is logged but never sent to the client.

If any of those returns plain text or a different shape, that's a bug to fix before moving on.

---

## Task 1 — Add `PATCH /users/{id}` using `respond.*` helpers

- [ ] Reuse the Day 3 pointer-field PATCH pattern. Use **only** the `respond.*` helpers — no `http.Error`, no hand-built envelopes.
- [ ] `404` if the user doesn't exist, `400` if no fields provided, `200` with the updated user on success.

**Why:** the proof that the helpers actually shrink handler code. Compare line count to your Day 3 version.

---

## Task 2 — `Conflict` on duplicate email

- [ ] In `POST /users`, return `409 CONFLICT` if a user with that email already exists.
- [ ] Add the lookup inside `store.create` under the mutex (atomic check-then-write).
- [ ] Verify with two `curl` calls using the same email.

**Why:** `409 Conflict` is one of the most under-used status codes. Practise picking it deliberately.

---

## Task 3 — Field-level validation errors (array form)

Right now a validation failure is one string: `"name is required"`. Real APIs report **all** problems at once so the client doesn't have to fix-and-retry per field.

- [ ] Extend the envelope to support a `details` array:
  ```json
  { "error": {
      "code": "VALIDATION",
      "message": "request is invalid",
      "details": [
        { "field": "name",  "issue": "is required" },
        { "field": "email", "issue": "must contain @" }
      ]
  } }
  ```
- [ ] Add a `respond.ValidationFailed(w, []respond.FieldError{...})` helper.
- [ ] Use it from `POST /users` so a request with no name AND no `@` in email returns BOTH issues in one response.

**Why:** the difference between "good error" and "great error" UX is almost entirely this.

---

## Task 4 — Stop leaking the JSON decoder error

`decodeErrorMessage` still returns the raw `err.Error()` in the default branch. That's an internal detail.

- [ ] Replace the `default:` branch with a generic message like `"invalid JSON request body"`.
- [ ] **Log** the real error server-side so you can still debug.
- [ ] Confirm `curl.exe -d "<<<not json>>>" -H "Content-Type: application/json" ...` returns the generic message, not the raw parser error.

**Why:** internal vs transport errors. The handler is your last gate before bytes go on the wire.

---

## Task 5 — Migrate the error envelope to RFC 7807 (Problem Details)

- [ ] Read the short version of [RFC 7807](https://www.rfc-editor.org/rfc/rfc7807) — it's mostly examples.
- [ ] Add a *second* helper function set (`respond.Problem(w, status, problem)`) that emits:
  ```json
  {
    "type": "https://example.com/probs/not-found",
    "title": "Not Found",
    "status": 404,
    "detail": "user not found",
    "instance": "/users/999"
  }
  ```
  with `Content-Type: application/problem+json`.
- [ ] Don't break the existing handlers. Pick one route (say `GET /users/{id}`) and convert it to use `respond.Problem` so you can compare side by side.
- [ ] At the end, write a 3-line note in your TASKS.md "What I learned" section: when would you pick RFC 7807 over the simple envelope, and when would you not?

**Why:** standardised error formats win on public APIs (especially if multiple teams or external partners consume them). Worth feeling the verbosity vs benefit tradeoff.

---

## Task 6 — Add a `request_id` to every response

A correlation ID is the single most useful debugging tool you'll ever add to an API.

- [ ] Write a middleware `requestID(next http.Handler) http.Handler` that:
  - Reads the incoming `X-Request-ID` header, or generates a UUID if missing. (Use the stdlib `crypto/rand` or just `time.Now().UnixNano()` for now.)
  - Stores it on the request `context.Context`.
  - Sets it on the response header `X-Request-ID`.
- [ ] Modify `respond.Error` (or the envelope itself) to include the request ID:
  ```json
  { "error": { "code": "...", "message": "...", "request_id": "..." } }
  ```
- [ ] When the user emails support with "your API broke for me", they quote the ID and you find their request in the logs in 5 seconds.

**Why:** Day 25 (slog) builds on exactly this. The plumbing you do today is the plumbing the logger uses there.

---

## Stretch — only if you're flying

- [ ] Replace `log.Printf` in `respond.Internal` with a `Logger` interface accepted by the package, so callers can inject `slog` later.
- [ ] Make the panic-recovery middleware also print the **stack trace** with `runtime/debug.Stack()` — useful for prod incidents.
- [ ] Read Go's `net/http`'s `recovery` middleware in `chi/middleware` source for comparison: <https://github.com/go-chi/chi/blob/master/middleware/recoverer.go>
