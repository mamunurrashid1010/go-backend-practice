# Day 1 — Practice Tasks

Do these in order. Each task adds one concept on top of [main.go](main.go). Don't peek at solutions online — break it, read the error, fix it. That's where the learning happens.

> **Before you start:** run the server (`go run .`) and hit every existing route with `curl` so you have a working baseline.

---

## Warm-up (observe what already exists)

- [ ] Run the server. Hit `GET /` in a browser **and** with `curl -v http://localhost:8080/`. Note the differences in headers (browsers send `Accept`, `User-Agent`, etc.).
- [ ] Hit `GET /hello?name=Mamun`. Now hit `GET /hello` (no name). Confirm the default fallback works.
- [ ] Hit `POST /echo` with `curl -X POST -d "hi" http://localhost:8080/echo`. Now try `GET /echo` and observe the `405 Method Not Allowed` response and the `Allow` header.
- [ ] Run `curl.exe -I http://localhost:8080/time` and look at just the headers — note `X-Server: day01`.

---

## Task 1 — A new route: `GET /goodbye`

- [ ] Add a handler at `/goodbye` that returns `Goodbye, <name>!` using the same `?name=` query pattern as `/hello`.
- [ ] Reject any method other than `GET` with `405` and an `Allow: GET` header.

**Why:** repetition. Routes will all start to feel the same — that's good.

---

## Task 2 — Status codes on purpose

- [ ] Add `GET /teapot` that returns status `418 I'm a teapot` with body `short and stout`.
- [ ] Add `GET /redirect` that returns `302 Found` with a `Location: /` header. Hit it in a browser and watch it redirect.

**Why:** trains you that status codes are a tool you *choose*, not something the framework picks.

---

## Task 3 — Reading multiple query params

- [ ] Add `GET /add?a=2&b=3` that returns `5` as plain text.
- [ ] If `a` or `b` is missing or not a number, return `400 Bad Request` with a clear message.
- [ ] Try with `curl "http://localhost:8080/add?a=foo&b=3"` — does your error look helpful?

**Why:** input validation + correct 4xx codes is 80% of being a competent API author.

---

## Task 4 — Custom 404

- [ ] Modify the root handler so any path that isn't a known route returns a custom `404` body like `no route for GET /banana` (include the method + path).
- [ ] Make sure existing routes still work.

**Why:** good error pages are good UX, even for APIs.

---

## Task 5 — Read a header

- [ ] Add `GET /whoami` that reads the `User-Agent` header from the request and returns `you are: <user-agent>`.
- [ ] Test with `curl`, then with a browser. Compare.

**Why:** request headers are how clients tell servers about themselves; you'll use them constantly (auth, content-type, request IDs).

---

## Task 6 — A small JSON-by-hand response

> *(Day 3 covers the proper way with `encoding/json`. Today, do it the ugly way to feel why we want a real library.)*

- [ ] Add `GET /me` that returns this body, with `Content-Type: application/json`:
  ```json
  {"name":"Mamun","day":1}
  ```
- [ ] Build the string with `fmt.Sprintf` for now.
- [ ] Open it in a browser — many browsers pretty-print JSON. If yours doesn't, install a JSON viewer extension.

**Why:** this sets up Day 3 perfectly — you'll *want* a real JSON encoder by tomorrow.

---

## Stretch — only if you're flying

- [ ] Add a global request log: print `method path status duration` for every request. (Hint: wrap the mux in another handler. This is a sneak peek at "middleware" — Day 6.)
- [ ] Replace `http.Error(...)` calls with a tiny helper `writeError(w, status, msg)` so you have one place to change the format later.
- [ ] Read the comments at the top of `net/http/server.go` in the Go source. Just skim — this builds intuition for later.
