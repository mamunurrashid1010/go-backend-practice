# Day 2 — Practice Tasks

Do these in order. Each task adds one concept on top of [main.go](main.go). Don't peek — break it, read the error, fix it.

> **Before you start:** run the server (`go mod init day02 && go run .`) and hit every existing route with `curl` so you have a working baseline.

---

## Warm-up (observe what already exists)

- [ ] Hit `GET /ping` and `GET /echo?msg=hi`. Confirm both work.
- [ ] Hit `GET /echo` (no query) — should be `400 Bad Request`. Hit `GET /echo?msg=` — also `400`. Notice the two different error messages.
- [ ] Hit `GET /search?q=golang&tag=web&tag=api` — see how `tag` becomes a multi-value `[]string`.
- [ ] Hit `GET /users/`, `GET /users/42`, `GET /users/42/posts`, `GET /users/42/banana` (the last should 404).
- [ ] Hit `GET /v2/users/42` and `GET /v2/users/42/posts/7`. Try `POST /v2/users/42` and notice you get an automatic `405 Method Not Allowed` — that's Go 1.22's pattern matcher rejecting the method for free.

---

## Task 1 — `GET /greet` with required + optional params

- [ ] Add `GET /greet?name=Mamun&lang=bn`.
- [ ] `name` is **required** — return `400` if missing or empty.
- [ ] `lang` is **optional** — default to `en`. Support at least `en` and `bn` (Bengali). Anything else → `400`.
- [ ] Body should be `Hello, Mamun!` for `en` or `Salam, Mamun!` for `bn`.

**Why:** required vs optional vs validated-enum is the entire shape of query-param API design.

---

## Task 2 — `GET /sum` with multiple integer params

- [ ] Add `GET /sum?n=1&n=2&n=3&n=4` — return the total (`10`).
- [ ] Use `r.URL.Query()["n"]` to get the slice of values.
- [ ] If any value isn't an integer, return `400` and name the offending value.
- [ ] If `n` is missing entirely, return `400`.

**Why:** trains the multi-value pattern. You'll use it for filters in Day 26.

---

## Task 3 — Manual path param: `GET /products/{id}`

- [ ] Register `mux.HandleFunc("/products/", productHandler)` (trailing slash, subtree match).
- [ ] Parse the ID from `r.URL.Path` yourself with `strings.TrimPrefix` + `strings.Split`.
- [ ] Validate the ID is a positive integer; otherwise `400`.
- [ ] If only `/products/` (no ID), return a list message.

**Why:** when you read older Go codebases or the source of `chi`/`gorilla/mux`, this is the pattern they're built on.

---

## Task 4 — Same thing the modern way: `GET /v2/products/{id}`

- [ ] Register `mux.HandleFunc("GET /v2/products/{id}", ...)`.
- [ ] Read the ID with `r.PathValue("id")`.
- [ ] Validate it's a positive integer.
- [ ] Compare line counts with Task 3. Notice how much boilerplate you skipped.

**Why:** by doing both, you understand *why* the 1.22 update was a big deal.

---

## Task 5 — Two path params: `GET /v2/products/{id}/reviews/{reviewID}`

- [ ] Read both with `r.PathValue`.
- [ ] Validate both are integers.
- [ ] Return: `product=42 review=7`.

**Why:** nested resources show up everywhere (`/users/{userID}/notes/{noteID}`).

---

## Task 6 — Pagination params with sane defaults

- [ ] Add `GET /list?page=2&size=20`.
- [ ] Defaults: `page=1`, `size=10`. Limits: `1 ≤ size ≤ 100`.
- [ ] Reject invalid input (`page=abc`, `size=0`, `size=999`) with `400`.
- [ ] Body: `page=2 size=20 offset=20` (where `offset = (page-1) * size`).

**Why:** this exact code shows up on Day 26 — you're building a real piece you'll keep.

---

## Stretch — only if you're flying

- [ ] Refactor: extract a tiny `queryInt(r, "size", 10)` helper that returns the value or a default. Use it in multiple handlers.
- [ ] Build a `requireMethod(method string, h http.HandlerFunc) http.HandlerFunc` wrapper. Replace all your `if r.Method != ...` lines with it. Congratulations — you just wrote your first middleware.
- [ ] Read the comments at the top of `net/http/server.go`'s `ServeMux` definition in the Go source. The pattern-matching algorithm is more clever than it looks.

---

## What I learned (fill this in at the end of the day)

> Write 3–5 bullets in your own words. No googling.

-
-
-
-
-
