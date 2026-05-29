# Day 16 — Practice Tasks

Error handling is where junior and senior Go code differ most visibly. These tasks drill the `Is`/`As`/`%w` muscle memory.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day16
> go get github.com/go-chi/chi/v5
> go get github.com/jackc/pgx/v5/stdlib
> go get github.com/jackc/pgx/v5/pgconn
> go get -tags 'postgres' github.com/golang-migrate/migrate/v4
> go get github.com/golang-migrate/migrate/v4/database/postgres
> go get github.com/golang-migrate/migrate/v4/source/file
> go get github.com/joho/godotenv
> go get github.com/caarlos0/env/v11
> go get github.com/go-playground/validator/v10
> go run .
> ```

---

## Warm-up

- [ ] Create a todo `{"title":"alpha"}` → `201`.
- [ ] Create `{"title":"alpha"}` again → `409 CONFLICT` `"title \"alpha\" already exists"`.
- [ ] Look at the server log on that 409 — nothing scary, because the repo returned a clean `*ConflictError` (not a wrapped pg error). Now temporarily break it (see Task 3) to see the difference.
- [ ] `GET /todos/9999` → `404` (sentinel `ErrNotFound` via `errors.Is`).
- [ ] `POST /todos` with `{"title":""}` → `422` (validator, not a domain error — different path).

---

## Task 1 — Trace the wrapping chain

- [ ] In `postgres_repository.go`'s `Get`, the not-found path returns the **bare** `ErrNotFound`. The error path returns `fmt.Errorf("get id=%d: %w", id, err)`.
- [ ] Temporarily change `Get` to wrap the not-found too: `return Todo{}, fmt.Errorf("get id=%d: %w", id, ErrNotFound)`.
- [ ] Confirm `GET /todos/9999` **still** returns 404 — because `errors.Is` walks the chain and finds `ErrNotFound` even when wrapped.
- [ ] Now change `%w` to `%v`. Confirm it now returns **500** — because `%v` flattens the error into a string and breaks the chain, so `errors.Is` can't find `ErrNotFound`.
- [ ] Revert to `%w`. **This is the single most important thing to internalise about Go errors.**

---

## Task 2 — Add a typed `ValidationError` from the service

The validator handles format at the handler. But some validation needs the DB (a *business* rule). Build one.

- [ ] Add a rule: a todo titled exactly `"admin"` is reserved.
- [ ] Add a typed error in `errors.go`:
  ```go
  type ReservedTitleError struct{ Title string }
  func (e *ReservedTitleError) Error() string {
      return fmt.Sprintf("title %q is reserved", e.Title)
  }
  ```
- [ ] In `service.Create`, return `&ReservedTitleError{in.Title}` when `in.Title == "admin"`.
- [ ] Add an `errors.As` case in `writeServiceErr` → `respond.Conflict` (or a new 422).
- [ ] Test: `{"title":"admin"}` → your chosen status with the reserved message.

**Why:** practising the typed-error + `errors.As` flow yourself, end to end.

---

## Task 3 — See what leaking looks like (then fix it)

- [ ] Temporarily change `writeServiceErr`'s `default` case to `respond.BadRequest(w, err.Error())` — i.e. leak the raw error.
- [ ] Drop the `asConflict` translation in `Create` so a duplicate insert returns the raw pg error.
- [ ] Create a duplicate title. Observe the client now sees:
  ```
  create: ERROR: duplicate key value violates unique constraint "todos_title_unique" (SQLSTATE 23505)
  ```
  That leaks your table name, constraint name, and that you use Postgres. **An attacker loves this.**
- [ ] Revert both changes. The `default → respond.Internal` + repo translation is what keeps internals internal.

**Why:** you have to *see* a leak once to take the discipline seriously.

---

## Task 4 — `errors.Is` with multiple targets (Go 1.20+)

- [ ] Go 1.20+ lets a single error wrap multiple errors via `errors.Join`. Try it:
  ```go
  err := errors.Join(ErrNotFound, ErrNothingToUpdate)
  fmt.Println(errors.Is(err, ErrNotFound))        // true
  fmt.Println(errors.Is(err, ErrNothingToUpdate)) // true
  ```
- [ ] Not something you'll use daily, but know it exists — useful when an operation fails for several independent reasons.

---

## Task 5 — A custom `Is` method (advanced)

- [ ] Give `ConflictError` an `Is` method so `errors.Is(err, &ConflictError{})` works (matches by type, ignoring fields):
  ```go
  func (e *ConflictError) Is(target error) bool {
      _, ok := target.(*ConflictError)
      return ok
  }
  ```
- [ ] Now both `errors.Is(err, &ConflictError{})` (any conflict) and `errors.As(err, &conflict)` (extract fields) work.
- [ ] Decide: do you prefer `Is` (just "was it a conflict?") or `As` (extract the field)? For the handler, `As` wins — you want the message. For a quick boolean check elsewhere, `Is` is handy.

---

## Task 6 — Map foreign-key violations too (preview Day 21)

When the Notes API arrives, notes will reference users. Deleting a user with notes, or creating a note for a missing user, triggers a foreign-key violation (`23503`).

- [ ] Sketch (don't fully build) an `asFKViolation(err) error` helper alongside `asConflict`, returning a new sentinel `ErrRelatedRecordMissing`.
- [ ] Think about which HTTP code that maps to: usually `400` or `409` or `422`, depending on whether the client could fix it. Write 2 lines of reasoning in your "What I learned".

---

## Stretch — only if you're flying

- [ ] Read the Go blog "Working with Errors in Go 1.13": <https://go.dev/blog/go1.13-errors>. It's the canonical explanation of `Is`/`As`/`%w`.
- [ ] Look at how a big project structures errors: skim `github.com/jackc/pgx/v5/pgconn`'s `PgError` type — note it's a typed error with a dozen fields, inspected with `errors.As`. That's the pattern at scale.
- [ ] Consider: should domain errors live in the `todo` package (where they are now) or a shared `apperror` package? Trade-offs: per-package keeps cohesion; shared avoids duplication when 10 packages all need `ErrNotFound`. Form an opinion.

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
