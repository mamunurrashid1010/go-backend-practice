# Day 3 — Practice Tasks

Each task adds one concept on top of [main.go](main.go). Type the code yourself — the muscle memory is the point.

> **Before you start:** run the server (`go mod init day03 && go run .`) and hit every existing route.

---

## Warm-up (observe what already exists)

- [ ] `GET /users` — should return a JSON array with two seed users.
- [ ] `POST /users` with a body — should return `201` and a `Location` header.
  ```powershell
  curl.exe -i -H "Content-Type: application/json" -d "{\"name\":\"Riad\",\"email\":\"r@x.dev\"}" http://localhost:8080/users
  ```
- [ ] `POST /users` with an **unknown field** — should `400` with `unknown field "role"`. This is `DisallowUnknownFields` doing its job.
  ```powershell
  curl.exe -i -H "Content-Type: application/json" -d "{\"name\":\"X\",\"role\":\"admin\"}" http://localhost:8080/users
  ```
- [ ] `POST /users` with **malformed JSON** (`{"name":}`) — should `400` with a clear "malformed JSON" message.
- [ ] `GET /users/999` — should `404`. `GET /users/abc` — should `400`.
- [ ] `GET /me` — note that the name contains double quotes and apostrophes, yet the JSON is valid. *That* is what `fmt.Sprintf` couldn't do for you in Day 1.

---

## Task 1 — `PUT /users/{id}` (full replacement)

- [ ] Add `PUT /users/{id}` that accepts a JSON body with `name` and `email`.
- [ ] If the user doesn't exist, return `404`. If `id` isn't a positive integer, `400`.
- [ ] On success, return `200` with the updated user.
- [ ] Reject unknown fields and oversized bodies, same as `POST`.

**Why:** `PUT` replaces. Internalising "PUT = replace, PATCH = modify" is half of REST.

---

## Task 2 — `DELETE /users/{id}`

- [ ] Add `DELETE /users/{id}`.
- [ ] On success, return `204 No Content` with **no body**. (Don't write JSON for 204.)
- [ ] On missing user, return `404` with a JSON error.

**Why:** trains "right status code, right body". A 204 with a JSON body is a bug.

---

## Task 3 — `omitempty` in action

- [ ] Add a `Bio string \`json:"bio,omitempty"\`` field to `User`.
- [ ] Allow it on `POST /users`.
- [ ] Confirm with `curl`: a user **without** a bio has no `"bio"` key in the response; a user **with** a bio shows it.

**Why:** demonstrates the difference `omitempty` makes on the wire.

---

## Task 4 — Nullable update with `PATCH /users/{id}`

- [ ] Add `PATCH /users/{id}` that updates only the fields the client sent.
- [ ] Use **pointer fields** in your input DTO: `Name *string`, `Email *string`. Nil → not provided; non-nil → update.
- [ ] If all fields are nil, return `400` ("nothing to update").
- [ ] Confirm: `PATCH` with `{"email":""}` clears the email; `PATCH` with `{}` errors; `PATCH` with `{"name":"x"}` only updates the name.

**Why:** this is the canonical pattern for partial updates. You will use it in every real API.

---

## Task 5 — A computed JSON field

- [ ] Add a method on `User`: `func (u User) DisplayName() string` that returns `"Name <email>"` if email is set, else just `Name`.
- [ ] Add a wrapper type for the response:
  ```go
  type userResponse struct {
      User
      DisplayName string `json:"display_name"`
  }
  ```
- [ ] Build a `userResponse` in `GET /users/{id}` and `GET /users`.

**Why:** real APIs add derived fields. Struct embedding + wrapper types is the cleanest way.

---

## Task 6 — Hide internal fields, expose curated ones

- [ ] Add a `Password string` field to `User` and set it on `POST /users` (accept it in the input DTO).
- [ ] Confirm `Password` does **not** appear in any response thanks to the `json:"-"` tag.
- [ ] Bonus: log on the server that you received a password (do NOT log the value).

**Why:** every backend leaks secrets eventually unless you build the habit of explicitly hiding them. The `json:"-"` tag is your first line of defense.

---

## Stretch — only if you're flying

- [ ] Move `respondJSON` and `respondError` into a tiny internal package: `internal/respond/respond.go`. Import it from `main.go`. This previews Day 4.
- [ ] Add a `Validate() error` method on the input DTO and call it from the handler. (`POST` requires `name`, `email` must contain `@` if provided.)
- [ ] Read the `encoding/json` package doc: <https://pkg.go.dev/encoding/json>. Skim — the bits on `Marshaler` and `Unmarshaler` interfaces are worth it.

