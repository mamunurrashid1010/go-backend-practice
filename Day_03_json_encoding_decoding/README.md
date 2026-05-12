# Day 3 — JSON Encoding & Decoding (`encoding/json`)

> **Goal:** read JSON from a request body, write JSON to a response, understand struct tags, and stop hand-building JSON with `fmt.Sprintf` (the bug from Day 1's `/me`).

---

## 1. Why this day matters

In Day 1 you wrote this:

```go
jsonBody := fmt.Sprintf(`{"name":"%s","day":%d}`, "Mamun", 1)
```

That works — until the name is `Bobby "Drop Tables" O'Brien`. Then the quotes break the JSON, the response is invalid, and clients crash. Today fixes that the right way.

JSON is the de facto wire format for REST APIs. Go's `encoding/json` package (in the standard library, no install) does two things:

1. **Encode** Go values into JSON bytes (for responses).
2. **Decode** JSON bytes into Go values (for requests).

---

## 2. The four functions you'll use

There are two pairs that do "the same thing" but with different inputs/outputs:

| You have…                       | You want…                          | Use                                  |
| ------------------------------- | ---------------------------------- | ------------------------------------ |
| A Go value                      | JSON bytes (`[]byte`)              | `json.Marshal(v)`                    |
| A Go value                      | JSON written to an `io.Writer`     | `json.NewEncoder(w).Encode(v)`       |
| JSON bytes (`[]byte`)           | A Go value                         | `json.Unmarshal(data, &v)`           |
| JSON from an `io.Reader`        | A Go value                         | `json.NewDecoder(r).Decode(&v)`      |

In HTTP handlers, **always prefer the streaming pair** (`Encoder`/`Decoder`) because:
- The request body **is** an `io.Reader`. The response writer **is** an `io.Writer`. No intermediate `[]byte` allocation.
- `Decoder` lets you tweak behavior (`DisallowUnknownFields`, decoding token by token, etc.).

> **Gotcha:** `Encoder.Encode` appends a trailing newline (`\n`) after the JSON. Usually harmless. `Marshal` doesn't.

---

## 3. Struct tags — the field-by-field instructions

A struct tag is a string literal next to a struct field that the encoder reads:

```go
type User struct {
    ID        int       `json:"id"`
    Name      string    `json:"name"`
    Email     string    `json:"email,omitempty"`     // omit if zero value
    Password  string    `json:"-"`                   // never encode/decode
    Age       int       `json:"age,string"`          // encode as JSON string "30" not number 30
    CreatedAt time.Time `json:"created_at"`
}
```

| Tag form                   | Effect                                                                       |
| -------------------------- | ---------------------------------------------------------------------------- |
| `json:"name"`              | Use `"name"` as the JSON key instead of the Go field name.                   |
| `json:"-"`                 | Skip this field both ways. Useful for passwords, internal state.             |
| `json:"name,omitempty"`    | Omit from output if the field is the zero value (`0`, `""`, `nil`, `false`). |
| `json:"name,string"`       | Encode/decode as a JSON string even if the Go type is numeric.               |
| (no tag)                   | The encoder uses the Go field name **only if it's exported (capitalised).**  |

**Three rules to remember:**

1. Only **exported** fields (starting with a capital letter) are encoded/decoded. The encoder silently skips lowercase ones — a common "why is my field missing?" bug.
2. `omitempty` checks the **Go zero value**, not "user-provided". A field with `0` looks the same as "not set" — see the *pointers for nullability* section below.
3. Tags are just strings. A typo (`json:"nme"`) is silently wrong. Tests catch this; the compiler doesn't.

---

## 4. Decoding a request body — the right way

The lazy version:

```go
var u User
json.NewDecoder(r.Body).Decode(&u)  // works, but every line below is a real bug
```

The production version:

```go
func createUserHandler(w http.ResponseWriter, r *http.Request) {
    // 1. Reject anything that's not JSON.
    if ct := r.Header.Get("Content-Type"); ct != "" && ct != "application/json" {
        http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
        return
    }

    // 2. Cap the body size — an attacker shouldn't be able to send a 5 GB body.
    r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

    // 3. Strict decode: reject unknown fields. Catches client typos early.
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()

    var u User
    if err := dec.Decode(&u); err != nil {
        http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
        return
    }

    // 4. Reject trailing garbage after the JSON ("{...}{...}" or "{...}xyz").
    if dec.More() {
        http.Error(w, "request body must contain a single JSON object", http.StatusBadRequest)
        return
    }

    // 5. Validate the decoded value yourself — JSON parsing won't.
    if u.Name == "" {
        http.Error(w, "name is required", http.StatusBadRequest)
        return
    }
    // ...
}
```

> **All five steps matter.** Day 4 will turn them into a reusable helper.

---

## 5. Encoding a response — the helper pattern

You'll write JSON responses in dozens of handlers. Use one helper:

```go
func respondJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(status)
    if err := json.NewEncoder(w).Encode(v); err != nil {
        // Headers are already sent — log it and move on.
        log.Printf("respondJSON: %v", err)
    }
}
```

Use it everywhere:

```go
respondJSON(w, http.StatusCreated, user)
respondJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
```

Day 4 turns this into a tiny `respond` package alongside an `respondError` companion.

---

## 6. Common gotchas

### "Zero value" vs "not provided"

Given `type Patch struct { Verified bool }`, the JSON `{}` and the JSON `{"verified": false}` decode into **the same Go value**. You can't tell which the client sent.

**Fix:** use a pointer.

```go
type Patch struct {
    Verified *bool `json:"verified,omitempty"`
}
// {}                 → p.Verified == nil
// {"verified": false} → *p.Verified == false
// {"verified": true } → *p.Verified == true
```

This pattern matters for `PATCH` endpoints (Day 4 onward).

### Time

`time.Time` encodes to RFC 3339 by default (`"2026-05-11T10:00:00Z"`). For dates without time, parse manually or define a custom type with `MarshalJSON`/`UnmarshalJSON`.

### Numbers

JSON numbers decode into `float64` if you target `interface{}`. For integer fields, **decode into a typed struct, not a map** — otherwise `42` becomes `42.0`.

### Streaming bigger payloads

Don't `io.ReadAll` the body then `json.Unmarshal`. That doubles memory. Just `Decoder.Decode` directly off `r.Body`.

### `encoding/json` is slow-ish

For most APIs it's plenty fast. Once you're past 50k req/s, look at `goccy/go-json` or `json/v2` (proposal, partially landed). Don't optimise this on day 3.

---

## 7. Putting it together

Today's [main.go](main.go) implements:

- `GET  /users`        → list users (JSON array)
- `GET  /users/{id}`   → one user
- `POST /users`        → create a user from a JSON body; returns `201 Created` with the new user and a `Location` header
- `GET  /me`           → the Day 1 `/me` route, this time **done properly** with `json.Marshal`

The store is an in-memory `map` guarded by a `sync.Mutex` — the same shape you'll use in Day 7's mini-project.

---

## 8. How to run

```powershell
go mod init day03
go run .
```

```powershell
curl.exe http://localhost:8080/users

curl.exe -H "Content-Type: application/json" -d "{\"name\":\"Mamun\",\"email\":\"m@x.dev\"}" http://localhost:8080/users

curl.exe http://localhost:8080/users/1

curl.exe -H "Content-Type: application/json" -d "{\"name\":\"X\",\"role\":\"admin\"}" http://localhost:8080/users
# ↑ should 400 with "unknown field 'role'" (strict decode)
```

> On PowerShell, escape inner quotes with `\"` as above, or use a here-string + `Invoke-RestMethod`.

---

## 9. What's next

**Day 4** turns today's inline JSON handling into reusable `respondJSON` / `respondError` helpers and codifies error-response shape across the whole API.
