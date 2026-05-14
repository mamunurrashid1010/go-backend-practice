# Day 5 — Practice Tasks

Each task pushes one chi feature into your fingers. Type it.

> **Before you start:**
>
> ```powershell
> go mod init day05
> go get github.com/go-chi/chi/v5
> go run .
> ```
>
> Then hit every existing route once with `curl.exe -i` and confirm behavior matches Day 4 exactly.

---

## Warm-up (observe what already exists)

- [ ] `GET /users` and `GET /api/v1/users/` — same response, two paths. Confirm the sub-router works.
- [ ] `GET /users/abc` — `400 BAD_REQUEST` JSON.
- [ ] `GET /banana` — `404 NOT_FOUND` JSON (now via `r.NotFound`, not a `/` catch-all).
- [ ] `PATCH /users/1` — `405 METHOD_NOT_ALLOWED` JSON with `Allow: DELETE, GET, PUT` (chi sets it for you).
- [ ] Compare [main.go](main.go) line counts to Day 4's. Routing is shorter; everything else (decoding, validation, store) is identical.

---

## Task 1 — Use `r.Route` to group all `/users` routes

Right now the root-level routes are listed one by one:

```go
r.Get("/users", listUsersHandler)
r.Post("/users", createUserHandler)
r.Get("/users/{id}", getUserHandler)
r.Put("/users/{id}", updateUserHandler)
r.Delete("/users/{id}", deleteUserHandler)
```

- [ ] Replace those five lines with one `r.Route("/users", func(r chi.Router) { ... })` block. Inside the block, paths become `/` and `/{id}` (the `/users` prefix is implicit).
- [ ] Run the same `curl` commands and confirm nothing changed externally.

**Why:** route groups are the chi feature that scales to 100-route APIs without `main.go` becoming unreadable.

---

## Task 2 — `PATCH /users/{id}` (pointer-field DTO)

- [ ] Reuse the Day 3 pointer-field PATCH pattern: `Name *string`, `Email *string`. Nil → not provided; non-nil → update.
- [ ] Use `respond.*` helpers everywhere. No raw `http.Error`.
- [ ] Add a new method to `userStore`: `patch(id int, name, email *string) (User, bool)`.
- [ ] Test:
  - `PATCH /users/1` with `{"name":"X"}` updates only name.
  - `PATCH /users/1` with `{}` returns `400 "nothing to update"`.
  - `PATCH /users/999` with `{"name":"X"}` returns `404`.

**Why:** chi changes nothing about the JSON or store work — the PATCH pattern from Day 3 lifts cleanly. That's a feature, not a bug.

---

## Task 3 — Nested resource: `GET /users/{userID}/posts/{postID}`

Posts don't exist yet — that's fine, just fake one.

- [ ] Register `r.Get("/users/{userID}/posts/{postID}", userPostHandler)` (or do it inside the `Route("/users", ...)` block as `r.Get("/{userID}/posts/{postID}", ...)`).
- [ ] In the handler, read **both** params:
  ```go
  userID := chi.URLParam(r, "userID")
  postID := chi.URLParam(r, "postID")
  ```
- [ ] Validate both are positive ints. Return `{"user_id": ..., "post_id": ...}`.

**Why:** nested resources (`/users/{id}/notes/{id}`) show up in every real API. Chi's `URLParam` makes it boring, which is what you want.

---

## Task 4 — Mount a sub-router under `/api/v2`

- [ ] Write a new `usersV2Subrouter()` that returns the **same** routes but adds a fake `X-API-Version: v2` response header to each.
- [ ] `r.Mount("/api/v2/users", usersV2Subrouter())`.
- [ ] Hit `curl.exe -i http://localhost:8080/api/v2/users` and confirm the header appears.

**Why:** real-world API versioning. You mount different versions side by side; clients pick by URL.

---

## Task 5 — `r.Group` with a shared "API-key required" check (no real auth yet)

- [ ] Add a helper middleware:
  ```go
  func requireAPIKey(next http.Handler) http.Handler {
      return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          if r.Header.Get("X-API-Key") != "shhh" {
              respond.Unauthorized(w, "missing or invalid X-API-Key")
              return
          }
          next.ServeHTTP(w, r)
      })
  }
  ```
- [ ] Wrap a `r.Group(func(r chi.Router) { r.Use(requireAPIKey); ... })` around `POST /users`, `PUT /users/{id}`, and `DELETE /users/{id}`. Reads can stay public.
- [ ] Confirm `GET /users` works without a header, but `POST /users` returns `401` without `X-API-Key: shhh`.

**Why:** previews tomorrow's middleware day. The pattern (middleware applied per group) is the heart of real auth setups.

---

## Task 6 — Custom 404 with the requested method + path

- [ ] Day 4's catch-all already did this. Make sure your `r.NotFound` handler still returns:
  ```json
  { "error": { "code": "NOT_FOUND", "message": "route not found: GET /banana" } }
  ```
- [ ] Also test that `r.MethodNotAllowed` returns a JSON 405 (curl with `-X PATCH /users/1` *before* you implement Task 2).

---

## Stretch — only if you're flying

- [ ] Read the chi source for `Mux.ServeHTTP` (about 30 lines): <https://github.com/go-chi/chi/blob/master/mux.go>. You'll see it's "find the route, call the handler". No magic.
- [ ] Use a regex-constrained param: `r.Get("/users/{id:[0-9]+}", getUserHandler)` — see how `chi` returns 404 (not 400) when `/users/abc` no longer matches.
- [ ] Add `r.Use(panicRecovery)` at the top of the root router (panic middleware copied from Day 4). Hit a route that panics and confirm chi's request continues to return clean JSON.

