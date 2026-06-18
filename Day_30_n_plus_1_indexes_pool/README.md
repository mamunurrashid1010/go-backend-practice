# Day 30 — N+1, Indexes, EXPLAIN, Pool Tuning

> **Goal:** three things that separate "the API works on my laptop" from "the API works at 1000 req/s."
>
> 1. **N+1** — the most common performance bug in any ORM-shaped code. Spot it, fix it with a JOIN or a batched `IN (...)`.
> 2. **Indexes + EXPLAIN ANALYZE** — read Postgres's query plan, recognize Seq Scan vs Index Scan vs Bitmap Heap Scan, know when to add an index and when *not* to.
> 3. **`*sql.DB` pool tuning** — what `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime` actually do and how to pick numbers that don't fall over.

The day's teaching example: `GET /audit?include=notes` joins each audit entry with the current state of the note it references. We ship **three implementations** of that join (naive, `JOIN`, batched `IN`), selectable via `?strategy=`, so you can hit the same endpoint three ways and watch the SQL count.

---

## 1. The N+1 problem

**The classic shape.** You list parents, then for each parent you fetch a child:

```go
// 1 query
entries := repo.ListAudit(ctx, userID, 50)
// + 50 more queries — one per entry
for i := range entries {
    entries[i].Note, _ = notesRepo.Get(ctx, entries[i].TargetID)
}
```

For 50 entries that's **51 queries**, each with its own network round trip, planning, parsing. Locally on a Postgres docker container it's "fine" — total maybe 20 ms. In production with a 1 ms RTT to a managed DB, that's 51 ms per request — most of it idle network time.

Worse, it scales linearly with the page size. Bump `limit` to 200 and the request now hits the DB 201 times.

The fix lives in one of two patterns.

### Fix A — `JOIN` (one query)

```sql
SELECT a.id, a.action, a.target_id, a.created_at,
       n.id, n.title
FROM   audit_logs a
LEFT JOIN notes n
  ON   n.id = a.target_id
 AND   a.target_type = 'note'
WHERE  a.user_id = $1
ORDER  BY a.created_at DESC, a.id DESC
LIMIT  $2
```

One query, two tables, one network round trip. `LEFT JOIN` because a deleted note's audit entry should still appear (with `note = null`).

This is the default in the codebase — `audit.ListWithNotesJoin`.

### Fix B — batched `IN (...)` (two queries, sometimes cleaner)

```go
// 1. Load the audit entries
entries := repo.ListAudit(ctx, userID, 50)
// 2. Collect the note ids you need
ids := make([]int64, 0, len(entries))
for _, e := range entries {
    if e.TargetType == "note" {
        ids = append(ids, e.TargetID)
    }
}
// 3. ONE more query — fetch all the notes at once
notes, _ := notesRepo.GetMany(ctx, userID, ids)
// 4. Build a map and attach
notesByID := indexByID(notes)
for i := range entries {
    entries[i].Note = notesByID[entries[i].TargetID]
}
```

Two queries instead of N+1. Slightly slower than the JOIN (two round trips, more transferred data), but useful when:

- The "child" is in a different database / service.
- The JOIN would explode row counts (`one parent → many children` and you don't want the parent columns duplicated).
- Your data layer can't or doesn't want to express the JOIN (sqlc-style codegen, REST microservice).

In the audit code: `audit.ListWithNotesInBatch`.

### The antipattern — kept for comparison

`audit.ListWithNotesNaive` does the actual N+1 — one `Get` per row. **It's wired only to show the contrast.** Pick `?strategy=naive` and watch your structured logs spit out `len(entries) + 1` distinct `http_request`-level DB query events for one request.

### How to spot N+1 in a real codebase

- Read every `for ... range ... { repo.Get(...) }` with suspicion.
- Look at query logs / Postgres `pg_stat_statements` and sort by `calls`. The bug is usually the top entry.
- Track a single request's DB call count in middleware. If the per-request count scales with the response item count, you have N+1.

---

## 2. Indexes and `EXPLAIN ANALYZE`

`EXPLAIN ANALYZE` runs your query and prints the plan plus actual timings. Read it bottom-up.

### The plan you want for our cursor query

The Day 26 cursor query:
```sql
SELECT id, user_id, title, body, created_at, updated_at
FROM   notes
WHERE  user_id = $1 AND id < $2
ORDER  BY id DESC
LIMIT  21;
```

With the composite index `idx_notes_user_id_id_desc` on `(user_id, id DESC)`:

```
Limit  (cost=0.42..1.15 rows=21)
  ->  Index Scan Backward using idx_notes_user_id_id_desc on notes
        Index Cond: ((user_id = $1) AND (id < $2))
```

- `Index Scan` — reading the index in order.
- `Index Cond` — both filters serviced by the index.
- **No `Sort` step** — the index is already in the right order.

That's the gold plan: O(limit), not O(table size).

### The plan you don't want

Drop the index and run the same query:

```
Limit  (cost=2563.84..2563.89 rows=21)
  ->  Sort  (cost=2563.84..2588.84)
        Sort Key: id DESC
        ->  Seq Scan on notes  (cost=0..2063.00 rows=N)
              Filter: ((user_id = $1) AND (id < $2))
              Rows Removed by Filter: 99000
```

- `Seq Scan` — read every row.
- `Rows Removed by Filter` — scanned 99,000 rows to find your 21.
- `Sort` — materialized a sort because there was no index order to follow.

At 100 rows you don't notice. At 10 million you do.

### What `EXPLAIN ANALYZE` actually tells you

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT ...
```

| Field | Meaning |
| --- | --- |
| `Seq Scan` | full table scan — only OK on small or filter-heavy tables |
| `Index Scan` | walking an index in order |
| `Bitmap Heap Scan` | reading rows via a bitmap of index hits — good for medium-selectivity filters |
| `Sort` | materialising a sort in memory or on disk |
| `Hash Join` / `Merge Join` / `Nested Loop` | the three join algorithms — Hash is usually fastest for "many × many" |
| `actual time=… rows=… loops=…` | the truth |
| `Buffers: shared hit=… read=…` | how many 8KB pages came from cache (`hit`) vs disk (`read`) |

The two questions you ask of every plan:

1. **Is it using an index?** If "no" and the table is large, add one.
2. **Are the row estimates anywhere near actuals?** Big mismatches mean Postgres has stale statistics — `ANALYZE` the table.

### When NOT to add an index

Every index has a cost — every write also updates the index. A table with many low-selectivity indexes inserts at a fraction of the speed.

Rules of thumb:

- **Add an index for the hot read path.** Our notes pagination uses `WHERE user_id=$1 ORDER BY id DESC` constantly — index it.
- **Don't index by single columns when you have a composite query.** A single `(user_id)` index can't service `WHERE user_id=$1 ORDER BY id DESC` as well as `(user_id, id DESC)` does.
- **Don't index low-cardinality columns alone.** A boolean `done` column has two values; an index on it points at half the table.
- **Don't blindly add foreign-key indexes.** Postgres doesn't index FKs automatically — but you only need one when you actually query the table by that FK.

### The "audit join" plan

`audit.ListWithNotesJoin` joins audit_logs with notes. With:

- `idx_audit_logs_user_id_created_at` on `(user_id, created_at DESC)` (Day 29)
- `notes_pkey` on `(id)` (free)

You should see something like:

```
Limit
  ->  Nested Loop Left Join
        ->  Index Scan Backward using idx_audit_logs_user_id_created_at on audit_logs a
              Index Cond: (user_id = $1)
        ->  Index Scan using notes_pkey on notes n
              Index Cond: (id = a.target_id)
              Filter: (a.target_type = 'note')
```

Index scan on the outer, primary-key lookup on the inner per row — both O(1)-ish. No table scans, no sorts, no merges. This is what a healthy pagination join looks like.

---

## 3. `*sql.DB` pool tuning

`*sql.DB` is **a pool, not a connection.** Your code calls `Query` / `Exec`; under the hood the pool hands you a connection from a slice, you use it, you return it.

Four knobs, each protects against a different failure mode:

### `SetMaxOpenConns(n)` — the ceiling

How many TCP connections to Postgres your app will ever hold *open at once* (in use + idle). Default: unlimited.

- Too low → goroutines queue waiting for a connection; you see latency spikes shaped like a staircase under load.
- Too high → you exhaust `max_connections` on the Postgres side (default 100). Postgres rejects new connections; *every* app instance starts failing.

A rule of thumb: `MaxOpenConns ≈ Postgres max_connections / number_of_app_instances - margin`. For Postgres default 100 and 5 app instances, ~15 per app, leaving 25 for psql / migrations / backups.

The codebase default is 25 — fine for one app instance against a docker-local DB.

### `SetMaxIdleConns(n)` — the warm pool

How many connections to keep open even when no one's using them. Default: 2 (terrible for any real workload — every request after the second has to do a fresh TCP+TLS handshake).

Rule: **set `MaxIdleConns == MaxOpenConns`** for read-heavy services. Idle connections take negligible memory; the cost of churn is real.

### `SetConnMaxLifetime(d)` — the recycle

Maximum age of a connection before the pool closes and replaces it. Default: forever.

Why you want it:

- **Load balancer rebalances.** If Postgres is behind PgBouncer / RDS Proxy / a service mesh, long-lived connections never see the rebalanced traffic — new backends are starved while old ones are saturated.
- **DB server restart.** A connection opened against the old server keeps trying to use the dead TCP socket. Recycling fixes itself faster.
- **Server-side per-conn config drift.** Sessions accumulate state (settings, prepared statements). Recycling resets.

5–30 minutes is the typical range. The codebase uses 5m.

### `SetConnMaxIdleTime(d)` — the eviction

How long an *idle* connection sits in the pool before being closed. Default: forever.

Pairs with `MaxIdleConns` — `MaxIdleConns` says "we may keep up to N idle"; `ConnMaxIdleTime` says "but evict ones that've been idle for D". Without it, your pool keeps every connection it ever opened during a spike, even when traffic falls to zero.

Codebase uses 2m. Reasonable.

### Picking numbers

Don't reach for "the right values" — measure.

1. Load-test (Day 40 covers k6). Find the steady-state latency at your target throughput.
2. Crank `MaxOpenConns` up by 50%. If latency improves, you were waiting on connections; if it stays the same or gets worse, you weren't.
3. Watch `pg_stat_activity`. If `state = 'idle in transaction'` is non-trivial, you have leaked transactions — fix those before tuning anything.

---

## 4. What changed from Day 29

| File | Change |
| --- | --- |
| `internal/audit/audit.go` | + `NoteRef` (slim note projection) and `EntryWithNote` |
| `internal/audit/repository.go` | + `ListWithNotesJoin`, `ListWithNotesInBatch`, `ListWithNotesNaive` |
| `internal/audit/handler.go` | `GET /audit` honours `?include=notes&strategy=join|in_batch|naive` |
| `internal/notes/repository.go` | + `GetMany(ctx, userID, ids)` for the IN batch strategy |

No migration. No schema change. Pool config is already in `internal/config` — the README explains what it does; the TASKS make you tune it.

---

## 5. Run it

```powershell
cd Day_30_n_plus_1_indexes_pool
docker compose up -d
go mod init day30
go mod tidy
go run .
```

Seed a few notes (creates audit entries automatically), then watch the three strategies:

```powershell
$tok = (curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"a@b.dev\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json).access_token

# Create 5 notes — each writes a note + audit row in one tx
1..5 | ForEach-Object {
    curl.exe -s -H "Authorization: Bearer $tok" -H "Content-Type: application/json" `
      -d "{\"title\":\"note $_\"}" http://localhost:8080/notes | Out-Null
}

# Default: JOIN
curl.exe -s -H "Authorization: Bearer $tok" "http://localhost:8080/audit?include=notes" | ConvertFrom-Json

# Batched IN(...)
curl.exe -s -H "Authorization: Bearer $tok" "http://localhost:8080/audit?include=notes&strategy=in_batch" | ConvertFrom-Json

# Naive N+1 — same response, ~6x the DB calls. Watch server logs.
curl.exe -s -H "Authorization: Bearer $tok" "http://localhost:8080/audit?include=notes&strategy=naive" | ConvertFrom-Json
```

Then walk EXPLAIN ANALYZE in psql — TASKS.md has the queries laid out.

---

## 6. What's next

**Day 31 — `sqlc`.** Hand-written `rows.Scan` blocks are about to vanish. `sqlc` reads your SQL files and generates type-safe Go. After today's "is this query fast?" comes "are these query shapes correct and maintained without boilerplate?"
