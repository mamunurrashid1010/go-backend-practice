# Day 26 — Pagination, Filtering, Sorting

> **Goal:** turn `GET /notes` into a real list endpoint. **Cursor pagination** for scale, sorting + filter via query params, the `+1` trick for "is there a next page?", an opaque base64 cursor, and a wrapper response shape `{ data, next_cursor }`.

The plan says **"know when to use each"** — so this README covers both cursor *and* offset pagination, even though we ship the cursor version. The Tasks make you add the offset variant.

---

## 1. Why `LIMIT N OFFSET M` is a trap

The shape every tutorial teaches:

```sql
SELECT ... FROM notes ORDER BY id LIMIT 20 OFFSET 1000
```

Two problems:

**Performance.** Postgres has to *fetch and discard* the first 1000 rows to reach OFFSET 1000. At OFFSET 10,000,000 the query crawls. The index doesn't save you — the rows still get materialised, just to be thrown away.

**Stability.** Between page 1 and page 2, a new row is inserted at the top. The user clicks "next" and sees row 20 twice. Or someone deletes a row and the user skips one. Offset pagination is a **stateless guess** about row positions in a table that's actively changing.

Offset has its place:
- Small, mostly-static datasets (admin tables with ~500 rows).
- UIs that genuinely need "go to page 5" / total page count.

For everything else — feeds, infinite scroll, large lists — cursor wins.

---

## 2. Cursor pagination — the right default

The mental model: **keep a bookmark, ask for the next batch after it.**

```sql
-- page 1: no bookmark
SELECT ... FROM notes
WHERE user_id = $1
ORDER BY id DESC
LIMIT 21;          -- 20 + 1 (the "+1" trick)

-- page 2: bookmark = id of the last row on page 1
SELECT ... FROM notes
WHERE user_id = $1 AND id < $cursor
ORDER BY id DESC
LIMIT 21;
```

**`LIMIT 21` is the trick.** We ask for one more than the user wanted:

- Returned 21 rows → there's another page. Drop row 21, emit `next_cursor = items[19].id`.
- Returned ≤ 20 → last page. No cursor.

You learn whether there's a next page **without a second count(*) query**.

Performance is `O(limit)` instead of `O(offset + limit)`. A query at "page 10,000" is the same speed as page 1.

Stability is automatic. Rows added at the top don't change the bookmark. Rows after the bookmark added between pages are seen on the next page, which is what you want.

---

## 3. What makes a good cursor field

Three properties:

| Property | Why |
| --- | --- |
| **Unique** | Two rows with the same cursor value → undefined "which one am I after?" |
| **Sortable** | The cursor is "everything before/after this value" |
| **Indexed** | We're doing range scans on every request |

Common picks:
- **Primary key** — almost always good. Monotonic, unique, indexed for free.
- **`(created_at, id)` composite** — when you want chronological order even if IDs are non-monotonic (UUIDs).
- **Never a column users can edit.** Editing a row would change its cursor — pagination becomes unstable.

For our notes, **`id` is the cursor.** BIGINT IDENTITY guarantees monotonic + unique, and we already have the index.

---

## 4. Encoding — make it opaque

Don't put the raw ID on the wire:

```
?after=1234         # tells the client "this is a primary key" — they'll guess, enumerate, hack
```

Wrap it in base64 (no padding) — the value is unchanged, but it looks opaque:

```
?after=MTIzNA       # base64.RawURLEncoding("1234")
```

Two reasons:

1. **Discourage enumeration.** If the cursor is literally an integer, clients build URLs by hand and rely on the format. Then you can't change it without breaking them.
2. **Future-proofing.** Maybe tomorrow your cursor becomes `id:1234:created_at:2026-06-09`. The base64 wrapper hides that change.

It is **not encryption.** Anyone can base64-decode `MTIzNA` back to `1234`. We're hiding the *intent to expose IDs*, not the IDs themselves.

```go
func encodeCursor(id int64) string {
    return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}
```

---

## 5. The response shape

Before today, `GET /notes` returned a bare JSON array: `[{...}, {...}]`.

Today it returns a wrapper:

```json
{
  "data": [
    {"id": 27, "title": "...", ...},
    {"id": 26, "title": "...", ...}
  ],
  "next_cursor": "MjY"
}
```

When there's no next page, `next_cursor` is omitted (`omitempty`).

The client's loop:

```javascript
let cursor = null;
while (true) {
    const url = `/notes?limit=20` + (cursor ? `&after=${cursor}` : "");
    const r = await fetch(url, { headers: { Authorization: `Bearer ${tok}` } }).then(r => r.json());
    for (const note of r.data) render(note);
    if (!r.next_cursor) break;
    cursor = r.next_cursor;
}
```

That's "infinite scroll" in 6 lines.

---

## 6. Query parameters today

```
GET /notes?search=foo&limit=20&after=MjY&sort=desc
```

| Param | Default | Notes |
| --- | --- | --- |
| `search` | `""` | case-insensitive substring on title |
| `limit` | `20` | 1..100 |
| `after` | `""` | opaque cursor from previous response |
| `sort` | `desc` | `desc` (newest first) or `asc` |

Bad `after` → `400`. Bad `limit` → `400`. The handler validates before reaching the service.

---

## 7. The SQL details

Two query shapes — first page vs paged:

**First page (no cursor):**
```sql
SELECT id, user_id, title, body, created_at, updated_at
FROM   notes
WHERE  user_id = $1 [AND LOWER(title) LIKE $2]
ORDER  BY id DESC
LIMIT  $3;     -- limit + 1
```

**Subsequent pages (with cursor):**
```sql
SELECT id, user_id, title, body, created_at, updated_at
FROM   notes
WHERE  user_id = $1 [AND LOWER(title) LIKE $2] AND id < $3   -- > for asc
ORDER  BY id DESC
LIMIT  $4;     -- limit + 1
```

The repo builds the WHERE dynamically (search + cursor are optional).

---

## 8. Indexing for the canonical scan

With `WHERE user_id = $1 AND id < $cursor ORDER BY id DESC`, Postgres wants an index where:

- `user_id` is the leading column (so we can find one user's rows).
- `id` is the next column, in the order we'll read it.

We already have `idx_notes_user_id` from Day 21. For the canonical scan, a composite is better:

```sql
CREATE INDEX idx_notes_user_id_id_desc ON notes (user_id, id DESC);
```

Migration `000004` adds it. Day 30 covers index design properly; today this is the obvious one.

---

## 9. Offset pagination — when you'd actually use it

If the plan says "know when to use each," here's *each*:

```sql
SELECT ... FROM notes WHERE user_id = $1
ORDER  BY id DESC
LIMIT  $size OFFSET $((page-1) * size);
```

Response shape:
```json
{ "data": [...], "page": 2, "size": 20, "total": 487, "total_pages": 25 }
```

| Use offset when… | Don't use offset when… |
| --- | --- |
| Total page count matters in the UI | List can grow large (>10k rows) |
| Static or near-static data | Underlying data changes |
| You need "go to page 7" jump | Performance matters |

Most production lists are cursor. Admin panels and reports are offset. Task 3 has you build the offset variant on a different endpoint.

---

## 10. What changed from Day 25

| File | Change |
| --- | --- |
| `internal/notes/notes.go` | `ListFilter` gains `AfterID`, `SortDesc`; new `ListPage` type |
| `internal/notes/cursor.go` | **NEW** — base64 encode/decode |
| `internal/notes/repository.go` | `List` returns `ListPage`; new SQL with cursor + LIMIT N+1 |
| `internal/notes/service.go` | `List` signature updated to return `ListPage` |
| `internal/notes/handler.go` | `parseListFilter` decodes cursor + sort; response wraps in `{data, next_cursor}` |
| `migrations/000004_index_notes_user_id_desc.up.sql` | composite index for the canonical scan |
| `internal/notes/service_test.go`, `handler_test.go` | fake repo + assertions updated for new return shape |

Auth, config, middleware, respond, logging — unchanged.

---

## 11. Run it

```powershell
cd Day_26_pagination
docker compose up -d
go mod init day26
# usual go gets
go run .
```

Walk a paginated read:

```powershell
# 1. register + login (as before)
$tok = (curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"a@b.dev\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json).access_token

# 2. seed 25 notes
1..25 | ForEach-Object {
    curl.exe -s -H "Authorization: Bearer $tok" -H "Content-Type: application/json" `
      -d "{\"title\":\"note $_\"}" http://localhost:8080/notes | Out-Null
}

# 3. page 1 (limit=10)
$r = curl.exe -s -H "Authorization: Bearer $tok" "http://localhost:8080/notes?limit=10" | ConvertFrom-Json
$r.data.Count                 # 10
$r.next_cursor                # base64 of an int — e.g. "MTY"

# 4. page 2
$r = curl.exe -s -H "Authorization: Bearer $tok" "http://localhost:8080/notes?limit=10&after=$($r.next_cursor)" | ConvertFrom-Json
$r.data.Count                 # 10
$r.next_cursor                # next

# 5. page 3 — should be smaller and have no next_cursor
$r = curl.exe -s -H "Authorization: Bearer $tok" "http://localhost:8080/notes?limit=10&after=$($r.next_cursor)" | ConvertFrom-Json
$r.data.Count                 # 5
$r.next_cursor                # empty
```

That's stable pagination — you can insert/delete rows between calls without page 2 shifting.

---

## 12. What's next

**Day 27** — rate limiting (`golang.org/x/time/rate`) + CORS middleware. Then **Day 28** — README polish + Week 4 close. After today, the API can serve large lists efficiently; tomorrow it can survive abuse.
