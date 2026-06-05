# Day 21 — Ship It

This is the Week 3 closer. The boring part is done — the interesting part is exercising it end to end and pushing to GitHub.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day21
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
> go run .
> ```

---

## Smoke test — the IDOR walk

Walk the curl sequence from the [README](README.md#full-curl-walkthrough) end to end. You're done when:

- [ ] A and B can both register + log in.
- [ ] A creates a note → 201, Location header, body contains `user_id` matching A.
- [ ] B's `GET /notes/<A's note id>` → **404 NOT_FOUND**.
- [ ] B's `DELETE /notes/<A's note id>` → 404.
- [ ] B's `PATCH /notes/<A's note id>` → 404.
- [ ] A's `GET /notes` and B's `GET /notes` each return only their own notes.
- [ ] Hitting any `/notes` route with no `Authorization` header → 401.
- [ ] In `psql`: `SELECT id, user_id, title FROM notes;` shows both users' rows, even though neither user can see the other's via the API.

That last bullet is the lesson: **the data exists in one table, but the API enforces tenant boundaries at the query level.**

---

## The full token lifecycle

- [ ] Login → save access + refresh.
- [ ] Create a note with the access token.
- [ ] Use `/auth/refresh` with the refresh token → new pair.
- [ ] Use the OLD refresh token again → 401 AND the chain dies (reuse detection from Day 20).
- [ ] Use the new access token → still works on /notes.
- [ ] Logout with the latest refresh token → 204.
- [ ] Try refresh again → 401 (the token is revoked).
- [ ] Access token still works until its `exp` — set `JWT_ACCESS_TTL=20s` if you want to feel that boundary.

---

## Polish ideas (pick a few)

- [ ] **Tests** (preview of Week 4):
  - `internal/notes/service_test.go` with a mock repository, table-driven cases for ErrNotFound and ErrNothingToUpdate.
  - `internal/notes/handler_test.go` with `httptest` — pass a fake-token-issuing setup, then check status codes.
- [ ] **Index `(user_id, id)`** on notes for the canonical "my notes by id" lookup. Day 30 covers index design properly.
- [ ] **Soft delete** — add `deleted_at TIMESTAMPTZ`, filter `WHERE deleted_at IS NULL` in every query. The schema change is one migration; the repo changes are tiny.
- [ ] **`/auth/sessions`** — list a user's active refresh tokens (so they can see "logged in from these devices"). Add a `device_label` column and surface it.
- [ ] **`/notes/{id}/share`** — a `note_shares(note_id, user_id)` table. Change Get's WHERE to `(user_id = $1 OR id IN (SELECT note_id FROM note_shares WHERE user_id = $1))`. The shape of the API doesn't change.
- [ ] **Search by body too** — change `LOWER(title) LIKE $n` to also include `LOWER(body) LIKE $n` with `OR`.
- [ ] **`/notes/count`** — `GET /notes/count` returns `{"count": n}` for the current user. Trivial endpoint; test it returns 0 for a fresh user.

---

## Reflection — Week 3 summary

Write 5 bullets answering "what's different about my project now vs. Day 14?":

-
-
-
-
-

(Hint: multi-user data isolation, refresh tokens, validator, typed errors, the security primitives you can never NOT have once you've shipped one.)

---

## What I learned (Week 3 summary)

> 5 bullets in your own words. Focus on what *compounded* this week.

-
-
-
-
-
