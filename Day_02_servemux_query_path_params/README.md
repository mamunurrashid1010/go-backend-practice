# Day 2 — `http.HandlerFunc`, `ServeMux`, Query & Path Params

> **Goal:** understand the two interfaces that power every Go web server (`http.Handler` and `http.HandlerFunc`), how `http.ServeMux` actually routes requests, and how to read query and path parameters cleanly.

---

## 1. The two types you'll see everywhere: `Handler` and `HandlerFunc`

### `http.Handler` — the interface

Anything that satisfies this interface can serve HTTP:

```go
type Handler interface {
    ServeHTTP(w ResponseWriter, r *Request)
}
```

You can implement it on a struct when you need state:

```go
type pingHandler struct{ msg string }

func (h *pingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintln(w, h.msg)
}
```

### `http.HandlerFunc` — the adapter

It's a tiny adapter that lets a *plain function* satisfy the `Handler` interface. Its definition is one of the most elegant tricks in the Go stdlib:

```go
type HandlerFunc func(ResponseWriter, *Request)

// HandlerFunc itself implements http.Handler:
func (f HandlerFunc) ServeHTTP(w ResponseWriter, r *Request) {
    f(w, r)
}
```

That's it. Because `HandlerFunc` has a `ServeHTTP` method on it, *any function with the right signature* can be cast to a `Handler`.

So both of these are equivalent:

```go
mux.HandleFunc("/ping", pingHandler)        // function-style
mux.Handle("/ping", http.HandlerFunc(pingHandler))  // explicit cast
```

`HandleFunc` (no `r`) is just a shortcut around `Handle` + `HandlerFunc`. Use it 99% of the time.

### Why this matters

When you see middleware later (Day 6), you'll see code like:

```go
func logging(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Println(r.Method, r.URL.Path)
        next.ServeHTTP(w, r)
    })
}
```

That code only makes sense once you've internalised the adapter pattern above. Today's the day to internalise it.

---

## 2. `http.ServeMux` — Go's built-in router

`ServeMux` is a multiplexer: it looks at the request's method + path and forwards the request to the registered handler. It's the *router* that ships with Go.

### Two flavors of pattern

`ServeMux` was upgraded in **Go 1.22** (Feb 2024) to support method-aware patterns and path parameters. You'll see both styles in the wild.

| Pattern style | Example | Meaning |
| --- | --- | --- |
| Exact path | `/about` | matches `/about` only |
| Subtree (legacy) | `/static/` | matches `/static/` and everything below it |
| Method + path (1.22+) | `GET /users` | only `GET /users` |
| Path param (1.22+) | `GET /users/{id}` | captures `{id}`, available via `r.PathValue("id")` |
| Wildcard suffix (1.22+) | `GET /files/{path...}` | captures everything after `/files/` |

> **Trailing slash gotcha**: `/foo` and `/foo/` are different patterns. A request to `/foo/bar` only matches `/foo/` (subtree), not `/foo` (exact).

### Two routing styles in this lesson

The Day 2 task in the learning plan says **"path params (manual parsing)"** because that's how every Go HTTP tutorial under 2 years old does it, and reading legacy code is a real skill. We'll do it both ways:

- **Manual** — split `r.URL.Path` by `/` and parse the segment yourself.
- **Modern (1.22+)** — let `ServeMux` capture `{id}` and read it with `r.PathValue("id")`.

Knowing the manual version makes you appreciate `r.PathValue` later, and means you can read code from any era of Go.

---

## 3. Query parameters

The URL after `?` is the **query string**. Go gives you a parser:

```go
// URL: /search?q=golang&limit=10&tag=web&tag=api
vals := r.URL.Query()      // url.Values, which is a map[string][]string

q := vals.Get("q")         // "golang" — first value or "" if missing
limit := vals.Get("limit") // "10" — always a string, parse it yourself
tags := vals["tag"]        // []string{"web", "api"} — multi-values

if vals.Has("debug") {     // Go 1.17+: present even when the value is empty
    // ...
}
```

Rules of thumb:

- **`Get(key)`** returns `""` for both "missing" and "present but empty". If you need to tell them apart, use `Has(key)` (1.17+) or check `vals[key]` directly.
- **Numbers come in as strings.** Always parse with `strconv.Atoi` and return `400 Bad Request` on failure.
- **Multi-values are common** for filters: `?tag=web&tag=api`. Don't assume a single string.
- **Bools**: there's no built-in. Treat `?debug=true`, `?debug=1`, or just the presence (`?debug`) as truthy — pick one and document it.

---

## 4. Path parameters — the manual way

Before Go 1.22, `ServeMux` didn't support captured path segments, so the pattern was: register a **subtree pattern** (note the trailing slash) and parse `r.URL.Path` yourself.

```go
mux.HandleFunc("/users/", usersHandler)   // matches /users/, /users/42, /users/42/posts, ...

func usersHandler(w http.ResponseWriter, r *http.Request) {
    // r.URL.Path is the full path: "/users/42"
    // strings.TrimPrefix removes "/users/" → "42"
    rest := strings.TrimPrefix(r.URL.Path, "/users/")
    if rest == "" {
        listUsers(w, r)
        return
    }
    parts := strings.Split(rest, "/")
    id := parts[0]      // "42"
    // optionally route on parts[1] for /users/42/posts, etc.
    showUser(w, r, id)
}
```

This works but is fiddly: you handle the trailing slash, the empty case, and any deeper segments yourself.

### The Go 1.22+ way (much nicer)

```go
mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    fmt.Fprintf(w, "user %s", id)
})
```

- The `GET ` prefix means non-GET requests get an automatic `405 Method Not Allowed` — no boilerplate.
- `r.PathValue("id")` returns the captured segment.
- Returns `404 Not Found` automatically if no pattern matches.

You'll see both styles all the time. Day 5 introduces `chi`, which has its own (different again) syntax.

---

## 5. The `/echo?msg=...` and `/ping` endpoints

The plan asks you to build:

- `GET /ping` → returns `pong` as plain text.
- `GET /echo?msg=hello` → returns the value of `msg`.

These look trivial, but doing them right covers the whole topic:

- Method check (or use the 1.22 `GET ` pattern prefix).
- Read a query param with `r.URL.Query().Get("msg")`.
- Decide what `?msg=` (empty) should return — probably `400 Bad Request`.
- Set `Content-Type` and write the body.

[main.go](main.go) shows both styles side by side.

---

## 6. How to run today's code

From inside this folder:

```powershell
go mod init day02
go run .
```

Then in another terminal:

```powershell
curl.exe http://localhost:8080/ping
curl.exe "http://localhost:8080/echo?msg=hello"
curl.exe "http://localhost:8080/echo"          # should 400
curl.exe http://localhost:8080/users/42         # manual path param style
curl.exe http://localhost:8080/v2/users/42      # modern 1.22 style
curl.exe "http://localhost:8080/search?q=go&tag=web&tag=api"
```

---

## 7. What to take away

- A **handler** is anything with `ServeHTTP(w, r)`. `HandlerFunc` is just a function-to-handler adapter.
- `ServeMux` is a real router — small, but enough for most apps. Method-aware patterns landed in Go 1.22.
- Trailing slashes change pattern semantics. `"/foo"` and `"/foo/"` are not the same pattern.
- Query params are always strings; you parse and validate.
- Path params: the **manual way** trains intuition; the **1.22 way** is what you'll actually write today.

---

## 8. What's next

**Day 3**: `encoding/json` — decoding request bodies, encoding responses, struct tags, and why the hand-built JSON in Day 1 (`fmt.Sprintf` of a JSON string) is a bug waiting to happen.
