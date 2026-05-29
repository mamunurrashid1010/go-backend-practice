# Day 15 — Practice Tasks

Validation is easy to add and easy to do shallowly. These tasks push past "I added `required` tags".

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day15
> go get github.com/go-chi/chi/v5
> go get github.com/jackc/pgx/v5/stdlib
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

- [ ] `POST /todos` with `{"title":""}` → `422` with `details: [{"field":"title","issue":"is required"}]`.
- [ ] `POST /todos` with a 201-char title → `422` `"must be at most 200 characters"`.
- [ ] `PATCH /todos/1` with `{"title":""}` → `422` (the `min=1` rule on the pointer field).
- [ ] `PATCH /todos/1` with `{}` → `400 "nothing to update"` (a *service* rule, not validation).
- [ ] `POST /todos` with `{"title":"ok"}` → `201`.

Note the two different failure paths: **422** from the validator (format), **400** from the service (business rule). Both are correct; they mean different things.

---

## Task 1 — Add a `priority` field with `oneof`

- [ ] Add to `Todo`, `CreateRequest`, `UpdateRequest`, `PatchRequest`:
  ```go
  // Create/Update:
  Priority string  `json:"priority" validate:"required,oneof=low medium high"`
  // Patch:
  Priority *string `json:"priority,omitempty" validate:"omitempty,oneof=low medium high"`
  ```
- [ ] Add a migration: `000002_add_priority.up.sql`:
  ```sql
  ALTER TABLE todos ADD COLUMN priority TEXT NOT NULL DEFAULT 'low'
      CHECK (priority IN ('low','medium','high'));
  ```
- [ ] Update the Postgres repo SQL (SELECT/INSERT/UPDATE/COALESCE PATCH) + the `Todo` struct + scans to include `priority`.
- [ ] Test: `{"title":"x","priority":"urgent"}` → `422 "must be one of: low medium high"`.

**Why:** `oneof` is the enum-validation workhorse. And you get to feel the full-stack change: DTO tag + migration + repo SQL.

---

## Task 2 — Multiple errors at once

- [ ] `POST /todos` with `{"title":"","priority":"urgent"}`.
- [ ] Confirm the `details` array has **both** failures, not just the first:
  ```json
  "details":[
    {"field":"title","issue":"is required"},
    {"field":"priority","issue":"must be one of: low medium high"}
  ]
  ```

**Why:** this is the whole UX argument for a validation library. Hand-rolled `if` returns one error at a time; the validator returns them all.

---

## Task 3 — A registration DTO (auth preview)

You'll need this for Day 17. Build it now.

- [ ] Create `internal/auth/dto.go` (new package) with:
  ```go
  type RegisterRequest struct {
      Email    string `json:"email"    validate:"required,email"`
      Password string `json:"password" validate:"required,min=8,max=72"`
      Confirm  string `json:"confirm"  validate:"required,eqfield=Password"`
  }
  ```
- [ ] Write a throwaway test or a temporary handler that decodes + validates it.
- [ ] Test the messages: bad email → "must be a valid email"; short password → "must be at least 8 characters"; mismatched confirm → "must match Password".

**Why:** `email`, `min`, and `eqfield` are exactly the validations a register endpoint needs. (The `max=72` is bcrypt's input limit — Day 17 explains why.)

---

## Task 4 — A custom validator

The built-ins don't cover everything. Register one.

- [ ] In `internal/validate/validate.go`, register a custom rule `notblank` (a string that isn't just whitespace):
  ```go
  val.RegisterValidation("notblank", func(fl validator.FieldLevel) bool {
      return strings.TrimSpace(fl.Field().String()) != ""
  })
  ```
- [ ] Add an `issue` case for it: `"must not be blank"`.
- [ ] Change `Title` to `validate:"required,notblank,max=200"`.
- [ ] Test: `{"title":"   "}` → `422 "must not be blank"`. (Plain `required` passes whitespace; `notblank` catches it.)

**Why:** `required` only checks "not the zero value". `"   "` is non-zero. Real apps need `notblank`. Writing a custom validator once shows you the extension point.

---

## Task 5 — Validate query params too

`parseListFilter` hand-checks `limit` (1..100). Could the validator do it?

- [ ] Define a struct for the list query:
  ```go
  type ListQuery struct {
      Limit int `validate:"omitempty,gte=1,lte=100"`
  }
  ```
- [ ] Parse the query string into it, then `validate.Struct`.
- [ ] Compare to the hand-rolled version. Which reads better? (There's no single right answer — query params are sometimes easier hand-checked. Form an opinion.)

**Why:** validation isn't only for JSON bodies. Knowing you *can* validate any struct is the point.

---

## Task 6 — Decide: where should validation live?

A thinking task.

- [ ] Right now `validate.Struct` is called in the handler. Imagine adding a CLI that calls `svc.Create` directly. The CLI bypasses the handler — so it bypasses validation.
- [ ] Two fixes:
  - (a) Call `validate.Struct` in the **service** too (defense in depth).
  - (b) Call `validate.Struct` in the **CLI** (validation at every boundary).
- [ ] Which would you choose, and why? Write 3 lines in your "What I learned" section. (Hint: there's a reason most teams validate at the boundary AND keep critical invariants in the service. Day 16 makes the service's errors rich enough to carry field details.)

---

## Stretch — only if you're flying

- [ ] Read the validator docs' "Baked-in Validations" list: <https://github.com/go-playground/validator#baked-in-validations>. There are ~100. Skim — you'll reach for `e164` (phone), `datetime`, `dive` (validate slice elements) eventually.
- [ ] Use `dive` to validate a slice: a `BulkCreateRequest { Items []CreateRequest \`validate:"required,dive"\` }`.
- [ ] Localise messages: validator has a `universal-translator` integration for multi-language error messages. Overkill for now, good to know it exists.

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
