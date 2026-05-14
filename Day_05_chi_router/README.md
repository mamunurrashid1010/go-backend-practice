# Day 5 — The `chi` Router

> **Goal:** install `chi`, rebuild yesterday's API with it, and learn the two features that made everyone switch from `ServeMux`: **route groups** and **URL parameters** (`chi.URLParam`).

---

## 1. Why `chi` (and not just `ServeMux`)?

Go 1.22 made `ServeMux` good enough for a lot of small APIs. So why does the Go community still reach for `chi` 95% of the time? Three reasons:

| Thing                       | `ServeMux` (stdlib)                      | `chi` |
| --------------------------- | ---------------------------------------- | ----- |
| Method routing              | `mux.HandleFunc("GET /x", ...)` (1.22+)  | `r.Get("/x", ...)` |
| URL params                  | `r.PathValue("id")`                      | `chi.URLParam(r, "id")` |
| **Route groups**            | none — repeat patterns by hand           | `r.Route("/users", func(r chi.Router) { ... })` |
| **Per-group middleware**    | none — wrap handlers manually            | `r.Use(authMW)` inside a group |
| Sub-routers / mounting      | manual                                   | `r.Mount("/api/v1", apiRouter())` |
| Trailing-slash redirects    | manual / strict                          | configurable |
| `404` / `405` customisation | manual                                   | `r.NotFound(...)`, `r.MethodNotAllowed(...)` |

`chi` is also **tiny** (~1k lines of Go), **zero non-stdlib deps**, and any `chi.Router` is still an `http.Handler` — so everything you learned about `net/http`, `ResponseWriter`, `Request`, and middleware on Days 1–4 still applies. `chi` doesn't replace `net/http`; it gives you nicer routing on top.

> If you've used Express (Node), Flask blueprints (Python), or Sinatra (Ruby), `chi` will feel familiar.

---

## 2. Install

```powershell
go mod init day05
go get github.com/go-chi/chi/v5
```

`chi` lives at `github.com/go-chi/chi/v5` — note the `/v5`. v1 docs still float around online and use the old import path.

---

## 3. Method-based route registration

This is the part that makes handlers read like documentation:

```go
r := chi.NewRouter()

r.Get("/users",      listUsers)        // GET    /users
r.Post("/users",     createUser)       // POST   /users
r.Get("/users/{id}", getUser)          // GET    /users/{id}
r.Put("/users/{id}", updateUser)       // PUT    /users/{id}
r.Patch("/users/{id}", patchUser)      // PATCH  /users/{id}
r.Delete("/users/{id}", deleteUser)    // DELETE /users/{id}
```

Compared to Day 4's `mux.HandleFunc("GET /users/{id}", ...)`, you save almost nothing on a single route — but on a typical API with 30+ routes, it adds up to a much more scannable `main.go`.

> **`chi`'s 405 is free too.** A `POST /users/42` (with no `POST` registered) returns `405 Method Not Allowed` with an `Allow` header automatically.

---

## 4. URL parameters: `chi.URLParam`

```go
r.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
    idStr := chi.URLParam(r, "id")          // "42"
    id, err := strconv.Atoi(idStr)
    if err != nil || id <= 0 {
        respond.BadRequest(w, "id must be a positive integer")
        return
    }
    // ...
})
```

That's the entire feature. `chi.URLParam(r, "id")` is the exact equivalent of `r.PathValue("id")` from Day 2 — just spelled in `chi`'s convention.

### Wildcards

```go
r.Get("/files/*", serveFile)        // matches /files/anything/here/at/all
// inside the handler: chi.URLParam(r, "*") -> "anything/here/at/all"
```

### Regex constraints (chi-specific)

```go
r.Get("/users/{id:[0-9]+}", getUser)     // id must be digits, else 404
```

Handy, but use sparingly — validation in the handler reads more obviously and gives you a chance to return a *good* error message.

---

## 5. Route groups — the real reason to use `chi`

A **group** is a set of routes that share a path prefix and/or middleware. Two ways to write them:

### `r.Route` (most common)

```go
r.Route("/users", func(r chi.Router) {
    r.Get("/",        listUsers)        // GET  /users
    r.Post("/",       createUser)       // POST /users
    r.Get("/{id}",    getUser)          // GET  /users/{id}
    r.Put("/{id}",    updateUser)       // PUT  /users/{id}
    r.Delete("/{id}", deleteUser)       // DELETE /users/{id}
})
```

Inside the closure, `r` is a *sub-router* that already has the `/users` prefix. You can add `r.Use(...)` middleware that only applies inside this group — gold for things like "require auth on every `/users` route". That's tomorrow's day.

### `r.Group` (no prefix, just shared middleware)

```go
r.Group(func(r chi.Router) {
    r.Use(authMW)                       // applies to the routes below only
    r.Get("/me",   getMe)
    r.Get("/feed", getFeed)
})

r.Get("/public", publicHandler)         // not affected by authMW
```

You'll see both patterns; pick whichever fits.

---

## 6. Mounting sub-routers (versioning)

```go
func usersRouter() chi.Router {
    r := chi.NewRouter()
    r.Get("/",      listUsers)
    r.Post("/",     createUser)
    r.Get("/{id}",  getUser)
    return r
}

r := chi.NewRouter()
r.Mount("/api/v1/users", usersRouter())     // every route inside lives under /api/v1/users
```

This is how big codebases split features into files: `internal/users/router.go`, `internal/notes/router.go`, each exports a `func Router() chi.Router`, and `main.go` mounts them. We'll do exactly this in Day 11 (handler/service/repository layering).

---

## 7. Custom 404 / 405

```go
r.NotFound(func(w http.ResponseWriter, r *http.Request) {
    respond.NotFound(w, "route not found: "+r.Method+" "+r.URL.Path)
})
r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
    respond.MethodNotAllowed(w, "")     // chi sets the Allow header for you
})
```

That replaces Day 4's catch-all `/` hack. Cleaner intent.

---

## 8. `chi.Router` IS an `http.Handler`

```go
r := chi.NewRouter()
// ...
srv := &http.Server{
    Addr:    ":8080",
    Handler: r,                         // just plug it in
    // ...
}
srv.ListenAndServe()
```

Nothing changes about the surrounding `http.Server`, the timeouts, the response writers, or your `internal/respond` package. `chi` is purely "the thing that decides which handler runs".

---

## 9. What `chi` is NOT

- **Not a framework.** Gin and Echo own the response (`c.JSON(...)`), wrap request parsing, have their own middleware signatures, etc. `chi` doesn't.
- **No request binding / validation.** You still `json.Decode` and validate yourself.
- **No built-in JSON helpers.** Your `respond` package keeps doing its job.

That's a *feature*: `chi` adds one thing (routing) and leaves the rest of the stdlib alone. When Day 7's mini-project ships, you'll feel why this matters — your code doesn't depend on a heavy framework you can't easily swap out.

---

## 10. How to run today's code

```powershell
go mod init day05
go get github.com/go-chi/chi/v5
go run .
```

```powershell
curl.exe -i http://localhost:8080/users
curl.exe -i http://localhost:8080/users/1
curl.exe -i -X POST -H "Content-Type: application/json" `
  -d '{"name":"Riad","email":"r@x.dev"}' http://localhost:8080/users
curl.exe -i -X DELETE http://localhost:8080/users/1
curl.exe -i http://localhost:8080/banana            # custom JSON 404
```

Compare today's [main.go](main.go) to Day 4's. Same API, same `respond` package, more readable routing.

---

## 11. What's next

**Day 6** introduces the **middleware** concept properly — logging, recovery (yours from Day 4 — but as `chi/middleware`), request ID. `chi`'s `r.Use(...)` is where today's groundwork pays off.
