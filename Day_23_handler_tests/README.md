# Day 23 — Handler Tests with `net/http/httptest`

> **Goal:** test the notes HTTP handlers by building requests with `httptest.NewRequest`, serving them through the real chi router with `httptest.NewRecorder`, and asserting the **status code, body, and headers** — the full HTTP contract. Test the 200/201/204/400/401/404/422 paths.

The Day 22 `fakeRepository` carries over — we plug it behind a **real** `Service` and let the handler call through. We're testing the handler's wiring (decode → validate → call service → encode), not the service's logic (already done Day 22).

---

## 1. The stdlib tools

`net/http/httptest` ships with Go. Two functions are all you need:

- **`httptest.NewRequest(method, target, body)`** — builds a `*http.Request` suitable for handing to a handler. No network involved.
- **`httptest.NewRecorder()`** — returns a `*ResponseRecorder` that implements `http.ResponseWriter`. It captures everything the handler writes: status code, body, headers.

```go
rec := httptest.NewRecorder()
req := httptest.NewRequest("GET", "/7", nil)
handler.ServeHTTP(rec, req)

// rec.Code            -> int
// rec.Body            -> *bytes.Buffer
// rec.Header()        -> http.Header (the response headers the handler set)
```

That's it. No server, no port, no goroutine — just function calls.

---

## 2. The three layers we pick today

Most Go handler tests fall into one of three shapes. Pick deliberately.

| Shape | Service used | Trade-off |
| --- | --- | --- |
| **Pure handler unit test** | Mock the whole `Service` via an interface | Most isolated. Adds an interface you didn't need otherwise. |
| **Handler + real service + fake repo** ← we use this | Real `*Service`, fake `Repository` (Day 22) | Realistic. No extra interfaces. The handler test exercises the service too. |
| **End-to-end** | Real service, real Postgres | Most realistic, slowest. Day 24's `testcontainers`. |

The middle path is the right default for handler tests. The handler is **mostly plumbing** — decoding JSON, mapping errors. Testing it through the real service catches more bugs and writes no extra mocks.

---

## 3. Serving through the router — why it matters

The handler depends on `chi.URLParam(r, "id")` for the path param. That works only if the request reached the handler **through chi**. You can't fake it cleanly; just serve through the router:

```go
h := &Handler{Svc: NewService(&fakeRepository{...})}
router := h.Router()         // chi.Router

rec := httptest.NewRecorder()
req := httptest.NewRequest("GET", "/7", nil)  // path relative to /notes mount
router.ServeHTTP(rec, req)
```

The URL is `/7`, not `/notes/7`, because the test mounts the router directly — there's no `/notes` prefix in scope. (Day 23 stretch: mount through the *full* router with `RequireAuth` for a true end-to-end test.)

---

## 4. Bypassing `RequireAuth` for unit tests

Production: every `/notes` route requires a Bearer JWT. The middleware verifies it and puts `userID` on the context.

Tests: we don't want to mint JWTs for every test. Skip the middleware and put the userID on the context directly:

```go
func newAuthedRequest(method, target, body string, userID int64) *http.Request {
    var r *http.Request
    if body == "" {
        r = httptest.NewRequest(method, target, nil)
    } else {
        r = httptest.NewRequest(method, target, strings.NewReader(body))
        r.Header.Set("Content-Type", "application/json")
    }
    ctx := auth.WithUserID(r.Context(), userID)
    return r.WithContext(ctx)
}
```

The handler reads `auth.GetUserID(r.Context())` — and finds the userID we planted. The middleware is never exercised in these tests, and that's correct: testing `RequireAuth` is a *separate* concern (a JWT round-trip), not a notes-handler test.

The "no userID at all" case is also worth a test, though — it's the defense-in-depth `respond.Unauthorized` branch inside the handler:

```go
req := httptest.NewRequest("GET", "/7", nil) // NO auth.WithUserID
router.ServeHTTP(rec, req)
// want rec.Code == 401
```

---

## 5. The status-code matrix to cover

Every handler has the same handful of status paths. The Day 21 `writeServiceErr` decides them:

| Status | Trigger |
| --- | --- |
| `200 OK` | normal success |
| `201 Created` | `POST /notes` success (+ `Location` header) |
| `204 No Content` | `DELETE /notes/{id}` success |
| `400 Bad Request` | bad path param, bad JSON, `ErrNothingToUpdate` |
| `401 Unauthorized` | no userID on context (defense in depth) |
| `404 Not Found` | `ErrNotFound` from the repo / service |
| `415 Unsupported Media Type` | wrong `Content-Type` |
| `422 Unprocessable Entity` | validator rejected the DTO |

Each row deserves at least one test. The Day 23 `handler_test.go` has 13 of them.

---

## 6. Reading the response body

The body is JSON. Decode into `map[string]any` for ad-hoc inspection, or into a typed struct when you want type safety:

```go
func readBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
    t.Helper()
    var out map[string]any
    if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
        t.Fatalf("decode response: %v (body: %s)", err, rec.Body.String())
    }
    return out
}
```

`map[string]any` is "good enough" — clear at the call site (`body["error"].(map[string]any)["code"]`), no boilerplate types per test. Save typed decodes for cases where you'll compare 5+ fields.

---

## 7. What's in this folder

| File | Status |
| --- | --- |
| `internal/notes/handler_test.go` | **NEW** — today's lesson |
| `internal/notes/service_test.go` | carried from Day 22 (still part of the suite) |
| Everything else | unchanged from Day 22 |

The application code is **unchanged**. Tests get added without changing the code under test. That's the whole pitch of Days 11/22.

---

## 8. Run the tests

```powershell
cd Day_23_handler_tests
go mod init day23
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5/stdlib
go get github.com/jackc/pgx/v5/pgconn
go get -tags 'postgres' github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/postgres
go get github.com/golang-migrate/migrate/v4/source/file
go get github.com/joho/godotenv
go get github.com/caarlos0/env/v11
go get github.com/go-playground/validator/v10
go get golang.org/x/crypto/bcrypt
go get github.com/golang-jwt/jwt/v5

go test -v -race ./internal/notes/
```

Expected output: all Day 22 service tests + all Day 23 handler tests pass. Roughly 20 subtest lines, in milliseconds.

---

## 9. What's next

**Day 24** — integration tests with `testcontainers-go`. The real Postgres repository (the SQL methods we deliberately *didn't* unit-test) gets exercised against a real Postgres container spun up per test run. After that, your coverage numbers actually mean something — the slow but real path is covered, the fast in-memory path is covered, and you can move fast without breaking either.
