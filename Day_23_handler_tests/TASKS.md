# Day 23 — Practice Tasks

The handler tests give you a way to feel the contract from the outside.

> **Before you start:**
>
> ```powershell
> go mod init day23
> go get github.com/go-chi/chi/v5
> go get github.com/jackc/pgx/v5/stdlib
> go get github.com/jackc/pgx/v5/pgconn
> go get -tags 'postgres' github.com/golang-migrate/migrate/v4
> go get github.com/golang-migrate/migrate/v4/database/postgres
> go get github.com/golang-migrate/migrate/v4/source/file
> go get github.com/joho/godotenv
> go get github.com/caarlos0/env/v11
> go get github.com/go-playground/validator/v10
> go get golang.org/x/crypto/bcrypt
> go get github.com/golang-jwt/jwt/v5
>
> go test -v -race ./internal/notes/
> ```

You should see all Day 22 service tests + all Day 23 handler tests pass.

---

## Warm-up

- [ ] All tests green.
- [ ] `go test -cover ./internal/notes/` — should be higher than Day 22's. The handler functions are now exercised; only the Postgres SQL methods remain uncovered (Day 24 fixes that).

---

## Task 1 — A handler test that proves the body's shape

The existing tests check the status code. Add one that asserts the **body's structure**.

- [ ] Add:
  ```go
  func TestHandler_Get_BodyShape(t *testing.T) {
      repo := &fakeRepository{GetNote: Note{ID: 7, UserID: 1, Title: "x", Body: "y"}}
      router := newHandler(repo).Router()
      
      rec := httptest.NewRecorder()
      router.ServeHTTP(rec, newAuthedRequest("GET", "/7", "", 1))
      
      var got Note
      if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
          t.Fatal(err)
      }
      if got.ID != 7 || got.Title != "x" || got.Body != "y" {
          t.Errorf("body: got %+v", got)
      }
  }
  ```
- [ ] **Decode into a typed struct, not a `map[string]any`.** When you have 5+ fields to check, the typed version is cleaner.

---

## Task 2 — A request-builder helper

You'll write `newAuthedRequest("POST", "/", body, userID)` a lot. Make it ergonomic.

- [ ] Add a builder you can chain:
  ```go
  type req struct{ r *http.Request }
  
  func newReq(method, target string) *req {
      return &req{r: httptest.NewRequest(method, target, nil)}
  }
  func (r *req) auth(uid int64) *req     { r.r = r.r.WithContext(auth.WithUserID(r.r.Context(), uid)); return r }
  func (r *req) json(body string) *req {
      r.r.Header.Set("Content-Type", "application/json")
      r.r.Body = io.NopCloser(strings.NewReader(body))
      r.r.ContentLength = int64(len(body))
      return r
  }
  func (r *req) build() *http.Request { return r.r }
  ```
- [ ] Use it in one test:
  ```go
  router.ServeHTTP(rec, newReq("POST", "/").auth(1).json(`{"title":"x"}`).build())
  ```

**Why:** test code accumulates the same way prod code does. A fluent builder beats 8 lines of plumbing on every test.

---

## Task 3 — A 405 test

The notes router doesn't register `PATCH /` (only `PATCH /{id}`). Hit it.

- [ ] Add:
  ```go
  func TestHandler_Patch_OnCollection_Is405(t *testing.T) {
      router := newHandler(&fakeRepository{}).Router()
      rec := httptest.NewRecorder()
      router.ServeHTTP(rec, newAuthedRequest("PATCH", "/", `{"title":"x"}`, 1))
      // chi returns 405 Method Not Allowed for known paths with unknown methods
      if rec.Code != http.StatusMethodNotAllowed {
          t.Errorf("want 405, got %d", rec.Code)
      }
  }
  ```
- [ ] This is a chi-router behavior test. Useful when you wonder "what *exactly* does the framework do?"

---

## Task 4 — Full end-to-end with a real JWT (advanced)

Test through the **whole** stack: real `RequireAuth`, real `TokenIssuer`, real JWT in the header.

- [ ] In a new file `handler_e2e_test.go`:
  ```go
  func TestHandler_E2E_WithRealJWT(t *testing.T) {
      // 1. Build the issuer + verifier with a test secret.
      secret := "test-secret-at-least-32-chars-long-zzzz"
      iss := auth.NewTokenIssuer(secret, time.Minute, "test")
      ver := auth.NewTokenVerifier(secret, "test")
      
      // 2. Mint a token for user 1.
      tok, _, err := iss.Issue(auth.User{ID: 1, Email: "a@b.dev"})
      if err != nil { t.Fatal(err) }
      
      // 3. Build the full router as main.go does.
      h := &Handler{Svc: NewService(&fakeRepository{GetNote: Note{ID: 5, UserID: 1, Title: "x"}})}
      root := chi.NewRouter()
      root.Group(func(r chi.Router) {
          r.Use(auth.RequireAuth(ver))
          r.Mount("/notes", h.Router())
      })
      
      // 4. Make the request through the FULL path, with the real header.
      rec := httptest.NewRecorder()
      req := httptest.NewRequest("GET", "/notes/5", nil)
      req.Header.Set("Authorization", "Bearer " + tok)
      root.ServeHTTP(rec, req)
      
      if rec.Code != 200 {
          t.Fatalf("status: want 200, got %d", rec.Code)
      }
  }
  ```
- [ ] This is closer to an integration test. It's slower (~5ms vs ~50µs) but proves the *whole* stack actually fits together. Worth one per protected route group.

---

## Task 5 — A test that catches a regression

Imagine someone "refactors" the handler and accidentally writes `respond.JSON(w, http.StatusOK, ...)` instead of `respond.JSON(w, http.StatusCreated, ...)` on `POST /`.

- [ ] Confirm `TestHandler_Create_201` would catch it (it does — it asserts `rec.Code == 201`).
- [ ] Make the change in `handler.go` (don't commit). Run the test. Watch it fail. Revert.

**Why:** **a test you've never seen fail is a test you don't fully trust.** Every test should have a "what change does this catch?" answer in your head.

---

## Stretch — only if you're flying

- [ ] Read the `httptest` source — it's <500 lines: <https://pkg.go.dev/net/http/httptest>.
- [ ] Try `httptest.NewServer` — spins up a real local HTTP server on a random port. Useful when you need to test against a tool that *actually* makes HTTP requests (a Go HTTP client testing its retry logic, for example).
- [ ] Look up the difference between `httptest.NewServer` and `httptest.NewTLSServer`. Day 75 (HTTPS ingress) will land on this.

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
