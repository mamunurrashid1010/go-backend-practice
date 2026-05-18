# Day 7 — Extensions

This is the Week 1 mini-project, not a teaching day. The "tasks" below are **ideas to push the project further** if you want more practice before Week 2. Pick whichever feel useful; skip the rest.

> **Sanity check before extending:** run the server, exercise every endpoint from the [README curl list](README.md#run-it). If any returns the wrong status code or shape, fix that first.

---

## Quick wins (15–30 min each)

- [ ] **Validate title length.** Reject `title` longer than 200 chars with a clean 400 `"title too long (max 200)"`.
- [ ] **Add `priority`** (`"low" | "medium" | "high"`) to `Todo`, `CreateRequest`, `UpdateRequest`, `PatchRequest`. Reject anything else.
- [ ] **`HEAD /todos/{id}`** — same logic as `GET`, but no body. (Hint: chi has `r.Head(...)`.)
- [ ] **`X-Total-Count` header** on `GET /todos` — total *before* the `limit` is applied. Useful for paginated UIs.
- [ ] **Reject empty body** on `POST` with a clean message (right now Day 3's decoder catches it, but the message isn't great).

---

## Medium (30–90 min each)

- [ ] **Cursor pagination.** Add `?after=<id>&limit=20` to `GET /todos`. Return an `X-Next-Cursor` header pointing at the next page. Cursor pagination scales; offset doesn't.
- [ ] **Soft delete.** Add `DeletedAt *time.Time` (omitempty). `DELETE` sets it instead of removing; `GET` skips soft-deleted by default; add `?include_deleted=true` to opt in.
- [ ] **Bulk create.** `POST /todos/bulk` accepts an array of `CreateRequest` and returns the created todos. Return 207 Multi-Status if you want to be fancy.
- [ ] **`PATCH /todos/{id}/done`** — a tiny convenience endpoint that flips just the `done` field. Use it from a "complete" button in a frontend.

---

## Bigger (a real afternoon each)

- [ ] **Unit-test the handlers** with `net/http/httptest`. Day 23 covers this in depth; doing one or two now will make that day land faster.
  ```go
  req := httptest.NewRequest("GET", "/todos/1", nil)
  rr := httptest.NewRecorder()
  h.Router().ServeHTTP(rr, req)
  // assert on rr.Code and rr.Body
  ```
- [ ] **Add the `chi/middleware` versions** of Recover, Logger, RequestID side-by-side. Pick which you'd ship and write 2 lines of "why" in your notes.
- [ ] **Add a `requireAPIKey` middleware** on a `r.Group` covering only write methods. Reads stay public.
- [ ] **Mount the same handler at `/api/v1/todos`** in addition to `/todos`. Same code, different path. Sets up Day 11's package layout.

---

## Reflection — do this one

Open the project tree:

```
internal/
├── middleware/
├── respond/
└── todo/
```

Compare to where you were after Day 1 (a single `main.go` with everything inline). **What changed?** Write 5 bullets in your own words — not what each package does, but *why* the split exists. If you can answer "why is X in its own package", Day 11 (handler/service/repository layering) will feel obvious instead of forced.

-
-
-
-
-


