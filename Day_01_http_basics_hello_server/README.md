# Day 1 — HTTP Basics & Hello World Server (`net/http`)

> **Goal**: Understand how HTTP works (methods, status codes, headers, request/response cycle), the core REST principles, and write your first Go HTTP server using only the standard library.

---

## 1. How HTTP works (the mental model)

HTTP is a **request/response, text-based protocol** that runs over TCP. A client (browser, `curl`, mobile app) opens a connection to a server and sends a **request**; the server processes it and returns a **response**.

```
CLIENT                                SERVER
  | ---- HTTP Request -------------->   |
  |   (method, path, headers, body)     |
  |                                     |   <-- handler runs
  | <---- HTTP Response --------------- |
  |   (status code, headers, body)      |
```

### Anatomy of a request

```
GET /users/42?verbose=true HTTP/1.1      <-- start line: METHOD PATH VERSION
Host: api.example.com                    <-- headers
User-Agent: curl/8.0
Accept: application/json
Authorization: Bearer abc123

(optional body — used in POST/PUT/PATCH)
```

### Anatomy of a response

```
HTTP/1.1 200 OK                          <-- status line: VERSION CODE TEXT
Content-Type: application/json           <-- headers
Content-Length: 47
Date: Wed, 06 May 2026 10:00:00 GMT

{"id":42,"name":"Mamun","role":"admin"}  <-- body
```

**Stateless**: every request stands on its own. The server does not remember the previous request unless you give it state (cookies, tokens, DB).

---

## 2. HTTP methods (verbs)

| Method   | Purpose                       | Idempotent? | Has body? | Typical use                  |
| -------- | ----------------------------- | ----------- | --------- | ---------------------------- |
| `GET`    | Read a resource               | Yes         | No        | `GET /users/42`              |
| `POST`   | Create a resource / action    | No          | Yes       | `POST /users`                |
| `PUT`    | Replace a resource entirely   | Yes         | Yes       | `PUT /users/42`              |
| `PATCH`  | Partially update a resource   | No*         | Yes       | `PATCH /users/42`            |
| `DELETE` | Remove a resource             | Yes         | No        | `DELETE /users/42`           |
| `HEAD`   | Like GET, but headers only    | Yes         | No        | Health checks                |
| `OPTIONS`| Discover allowed methods/CORS | Yes         | No        | CORS preflight               |

> **Idempotent** = calling it 1 time or 100 times has the same effect. Important for retries.

---

## 3. Status codes (the ones you must know)

| Range | Meaning            | Common codes                                                        |
| ----- | ------------------ | ------------------------------------------------------------------- |
| 1xx   | Informational      | `100 Continue`                                                      |
| 2xx   | Success            | `200 OK`, `201 Created`, `204 No Content`                           |
| 3xx   | Redirection        | `301 Moved`, `302 Found`, `304 Not Modified`                        |
| 4xx   | Client error       | `400 Bad Request`, `401 Unauthorized`, `403 Forbidden`, `404 Not Found`, `409 Conflict`, `422 Unprocessable Entity`, `429 Too Many Requests` |
| 5xx   | Server error       | `500 Internal Server Error`, `502 Bad Gateway`, `503 Service Unavailable`, `504 Gateway Timeout` |

**Rules of thumb**
- `200` for "got it, here's the data".
- `201` for "I just created something" — usually with a `Location` header.
- `204` for "done, no body to return" (e.g., `DELETE`).
- `400` = the client's request is malformed (bad JSON, missing field).
- `401` = no/bad credentials. `403` = credentials fine, you're not allowed.
- `404` = resource doesn't exist.
- `500` = your server screwed up. Don't leak the stack trace to the client.

---

## 4. Headers you'll see every day

| Header              | Direction | Purpose                                              |
| ------------------- | --------- | ---------------------------------------------------- |
| `Content-Type`      | both      | Body MIME type (`application/json`, `text/html`)     |
| `Content-Length`    | both      | Body size in bytes                                   |
| `Accept`            | request   | "I can read these MIME types"                        |
| `Authorization`     | request   | `Bearer <token>` or `Basic <base64>`                 |
| `User-Agent`        | request   | Who is calling (browser, curl, app)                  |
| `Host`              | request   | The hostname being addressed                         |
| `Cache-Control`     | both      | Caching policy                                       |
| `Set-Cookie` / `Cookie` | both  | Cookies                                              |
| `Location`          | response  | Where to redirect / where the new resource lives     |
| `X-Request-ID`      | both      | Correlation ID for tracing a request through systems |

---

## 5. REST principles (the short version)

REST = **RE**presentational **S**tate **T**ransfer. It's a style of API design. Treat it as a useful guideline, not a religion.

1. **Resources are nouns, not verbs.**
   - Good: `GET /users/42`, `POST /users`
   - Bad:  `GET /getUser?id=42`, `POST /createUser`
2. **HTTP methods carry the intent.** Don't tunnel everything through `POST`.
3. **Statelessness** — each request contains everything the server needs (auth, params).
4. **Use status codes correctly.** A successful response with `{"error": "..."}` and a `200` is a smell.
5. **Consistent shape.** Your error JSON should have the same fields everywhere.
6. **Versioning.** Public APIs usually go in a `/v1/` prefix.

> Recommended read: Roy Fielding's dissertation, chapter 5. (Long.)
> Practical read: any "REST API design best practices" article — pick one and skim.

---

## 6. Go's `net/http` — the parts you need today

### `http.HandleFunc` and `http.ListenAndServe`

```go
http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintln(w, "Hello, world")
})
http.ListenAndServe(":8080", nil)
```

- `http.HandleFunc(pattern, handler)` registers a handler on the **default mux**.
- `http.ListenAndServe(addr, handler)` starts the server. `nil` means "use default mux".
- A **handler** is anything that satisfies `func(w http.ResponseWriter, r *http.Request)`.

### `http.ResponseWriter`

Three things you'll do with it:
1. **Set headers** — `w.Header().Set("Content-Type", "application/json")`. Must be done **before** writing the status or body.
2. **Set status code** — `w.WriteHeader(http.StatusCreated)`. If you skip this, Go writes `200` on the first `Write`.
3. **Write the body** — `w.Write([]byte("..."))` or `fmt.Fprint(w, ...)`.

### `*http.Request`

Useful fields:
- `r.Method` — `"GET"`, `"POST"`, …
- `r.URL.Path` — `"/users/42"`
- `r.URL.Query()` — query string as `url.Values` (a `map[string][]string`)
- `r.Header` — request headers (`http.Header`, also a `map[string][]string`)
- `r.Body` — `io.ReadCloser`. Always `defer r.Body.Close()` when you read it. (Tomorrow we'll JSON-decode this.)
- `r.Context()` — request-scoped context, cancelled when the client disconnects.

### Why you should outgrow `http.ListenAndServe(":8080", nil)`

The 1-line version uses Go's package-level default mux and has **no timeouts** — both are footguns later. From day one, prefer constructing your own `http.Server`:

```go
mux := http.NewServeMux()
mux.HandleFunc("/hello", helloHandler)

srv := &http.Server{
    Addr:              ":8080",
    Handler:           mux,
    ReadHeaderTimeout: 5 * time.Second,
    ReadTimeout:       10 * time.Second,
    WriteTimeout:      10 * time.Second,
    IdleTimeout:       60 * time.Second,
}
log.Fatal(srv.ListenAndServe())
```

---

## 7. How to actually run today's code

From inside this folder (`Day_01_http_basics_hello_server`):

```powershell
# one-time: turn this folder into a Go module
go mod init day01

# run the server
go run .
```

In **another terminal** (server keeps running in the first):

```powershell
# basic GET
curl http://localhost:8080/

# hello with name
curl "http://localhost:8080/hello?name=Mamun"

# see full request/response (very useful)
curl -v http://localhost:8080/time

# inspect only the headers
curl -I http://localhost:8080/

# try a POST (we just echo for now — real JSON is Day 3)
curl -X POST -d "hi server" http://localhost:8080/echo
```

> **Tip:** PowerShell aliases `curl` to `Invoke-WebRequest`, which behaves differently. Use `curl.exe` explicitly on Windows: `curl.exe -v http://localhost:8080/`.

Stop the server with **Ctrl+C**.

---

## 8. Practice plan for today (suggested order)

Work top-down. Don't skip ahead — each step adds *one* concept.

1. **Read & run** [main.go](main.go) as-is. Hit every route with `curl` and observe.
2. **Inspect with `curl -v`** so you see the actual request line, headers, and response status. This trains your eye.
3. **Tasks** — open [TASKS.md](TASKS.md) and do them one by one. They get progressively harder.
4. **Reflect** — at the end, fill in the short "What I learned" section at the bottom of TASKS.md. Writing it down forces real understanding.

### How I recommend you practice (general advice)
- **Type the code, don't paste it.** The friction is the point.
- **Break things on purpose.** Return the wrong status code and watch a browser's devtools complain. Skip `Content-Type` and see how the response looks.
- **Read one stdlib source file per day.** Today, skim `net/http/server.go` — even just the comments. Go's stdlib is famously readable.
- **Commit at the end of each day.** Even messy. `git commit -m "day 1: hello server"`.
- **If a "day" takes two real days, that's fine.** The plan is a guide.

---

## 9. What's next

**Day 2** introduces `http.ServeMux`, query params and (manual) path params more rigorously, and you'll build `/ping` + `/echo?msg=...` cleanly. Today's code already nudges you in that direction — tomorrow we formalize it.
