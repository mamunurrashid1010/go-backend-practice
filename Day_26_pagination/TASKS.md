# Day 26 — Practice Tasks

The implementation gives you cursor pagination. The tasks make you prove it works at scale, compare it against offset, and feel the pitfalls.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day26
> go get github.com/go-chi/chi/v5
> go get github.com/jackc/pgx/v5/stdlib
> go get github.com/jackc/pgx/v5/pgconn
> go get -tags 'postgres' github.com/golang-migrate/migrate/v4
> go get github.com/golang-migrate/migrate/v4/database/postgres
> go get github.com/golang-migrate/migrate/v4/source/file
> go get github.com/joho/godotenv
> go get github.com/caarlos0/env/v11
> go get github.com/go-playground/validator/v10
> go get golang.org/x/crypto/bcrypt
> go get github.com/golang-jwt/jwt/v5
> go run .
> ```

---

## Warm-up — register a user and seed 50 notes

- [ ] Register + login, save `$tok = (access_token)`.
- [ ] Seed:
  ```powershell
  1..50 | ForEach-Object {
      curl.exe -s -H "Authorization: Bearer $tok" -H "Content-Type: application/json" `
        -d "{\"title\":\"note $_\",\"body\":\"body $_\"}" http://localhost:8080/notes | Out-Null
  }
  ```

---

## Task 1 — Walk all 50 with cursor pagination

- [ ] `GET /notes?limit=10` — expect 10 items + a `next_cursor`.
- [ ] Loop: pass `?after=<cursor>` each time, until `next_cursor` is empty.
- [ ] Total items collected = 50, no duplicates, IDs strictly decreasing.

PowerShell loop you can paste:

```powershell
$cursor = ""; $all = @()
do {
    $url = "http://localhost:8080/notes?limit=10" + $(if ($cursor) { "&after=$cursor" } else { "" })
    $r = curl.exe -s -H "Authorization: Bearer $tok" $url | ConvertFrom-Json
    $all += $r.data; $cursor = $r.next_cursor
} while ($cursor)
$all.Count                    # expect 50
($all.id | Get-Unique).Count  # expect 50
```

---

## Task 2 — Prove cursor is stable under inserts

The whole point of cursor pagination.

- [ ] `GET /notes?limit=10` → save the cursor.
- [ ] Insert one new note (it gets a higher id, lands "above" the page).
- [ ] `GET /notes?limit=10&after=<cursor>` → still returns the next 10 in the original sequence. Nothing is skipped or repeated.
- [ ] Now try the same dance with offset (in your head): `LIMIT 10 OFFSET 10`. The new note is in page 1, so when you ask for page 2 you'd skip the old item that just got pushed off page 1.

Write 2 lines under "What I learned" about why this matters for feeds.

---

## Task 3 — Add an offset-paginated endpoint for comparison

Build `GET /notes/page?page=N&size=M` as a separate route.

- [ ] In `internal/notes/repository.go`, add:
  ```go
  type OffsetPage struct {
      Items []Note
      Total int
  }
  func (r *PostgresRepository) ListOffset(ctx context.Context, userID int64, search string, page, size int) (OffsetPage, error) {
      // Two queries: one for the page, one for COUNT(*).
      // Then math out total_pages in the handler.
  }
  ```
- [ ] Wire `GET /notes/page` in `handler.Router()`. Response:
  ```json
  { "data":[...], "page":2, "size":10, "total":50, "total_pages":5 }
  ```
- [ ] Now `EXPLAIN ANALYZE` both:
  ```sql
  EXPLAIN ANALYZE SELECT * FROM notes WHERE user_id=1 ORDER BY id DESC LIMIT 10 OFFSET 40;
  EXPLAIN ANALYZE SELECT * FROM notes WHERE user_id=1 AND id < 11 ORDER BY id DESC LIMIT 11;
  ```
  At 50 rows the difference is invisible. Seed 100k rows (use `generate_series`) and look again — offset slows down linearly, cursor stays flat.

---

## Task 4 — Verify the +1 trick

The trick is subtle. Verify it directly:

- [ ] Seed exactly 20 notes.
- [ ] `GET /notes?limit=20` → returns 20 items, **no** `next_cursor` (because the +1 query returned 20, not 21).
- [ ] Add 1 more note (21 total). `GET /notes?limit=20` → 20 items, **has** `next_cursor`.
- [ ] Print the SQL by adding a temporary `log.Println(q, args)` in `repository.go` `List`. Confirm the query ends in `LIMIT 21`.

---

## Task 5 — Cursor decode failures

- [ ] `GET /notes?after=not-a-cursor` → 400.
- [ ] `GET /notes?after=MA` → 400 (base64 of "0" is rejected — ids must be positive).
- [ ] `GET /notes?after=YQ` → 400 (base64 of "a" — not an integer).
- [ ] Test for it: add a row in `service_test.go`'s `TestCursor_InvalidIsRejected`.

---

## Task 6 — Switch sort direction

- [ ] `GET /notes?sort=asc&limit=5` — oldest first.
- [ ] Paginate forward with `?after=<cursor>&sort=asc` — should walk in ascending id order.
- [ ] `GET /notes?sort=banana` → 400.

This is why the SortDesc lives on `ListFilter` — the repo flips both the comparison (`id > $cursor`) and the `ORDER BY`.

---

## Task 7 — Confirm the new index gets used

- [ ] In `psql`:
  ```sql
  \d+ notes
  ```
  Confirm `idx_notes_user_id_id_desc` is on `(user_id, id DESC)` and the old `idx_notes_user_id` is gone (migration 4 dropped it).
- [ ] Seed 10k notes (`INSERT ... SELECT generate_series(...)`).
- [ ] Run `EXPLAIN ANALYZE SELECT ... FROM notes WHERE user_id = 1 ORDER BY id DESC LIMIT 21;` — expect `Index Scan using idx_notes_user_id_id_desc`. If you see `Sort`, the index isn't being used.

---

## Stretch — only if you're flying

- [ ] **Composite cursor**: change the cursor to encode `(created_at, id)` so notes still paginate stably even if you swap from BIGINT IDENTITY to UUIDs. Hint: encode `RFC3339|id` then base64. The WHERE becomes `(created_at, id) < ($cursor_ts, $cursor_id)`.
- [ ] **Configurable max limit**: move `maxLimit = 100` into `config.Config`.
- [ ] **`X-Total-Approx` header** on cursor responses: include `(SELECT reltuples FROM pg_class WHERE relname='notes')::bigint` — a *fast* estimate of total rows without `COUNT(*)`. Document that it's an estimate, not exact.
- [ ] **Search by body too**: change `LOWER(title) LIKE $n` to `(LOWER(title) LIKE $n OR LOWER(body) LIKE $n)`.

---

## What I learned (Day 26)

> 3 bullets in your own words.

-
-
-
