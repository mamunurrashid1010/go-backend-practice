# Day 30 — Practice Tasks

The implementation gives you three switchable strategies for the same audit-with-notes view. The tasks make you *measure* the difference, then walk EXPLAIN ANALYZE on the queries you've shipped since Day 26 and finally pick concrete pool-tuning numbers.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day30
> go mod tidy
> go run .
> ```

---

## Warm-up — feel the difference

Register a user, log in, save `$tok`. Seed 50 notes (each writes a note + an audit row in one tx):

```powershell
1..50 | ForEach-Object {
    curl.exe -s -H "Authorization: Bearer $tok" -H "Content-Type: application/json" `
      -d "{\"title\":\"note $_\"}" http://localhost:8080/notes | Out-Null
}
```

- [ ] Hit each strategy and count "http_request" log lines on the **server** side per request — actually, since slog only emits one http_request line per HTTP request, instead use psql + `pg_stat_statements`:

  ```sql
  CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
  SELECT pg_stat_statements_reset();
  ```

  Then from your shell:
  ```powershell
  curl.exe -s -H "Authorization: Bearer $tok" "http://localhost:8080/audit?include=notes&strategy=join&limit=50" | Out-Null
  ```
  ```sql
  SELECT calls, query FROM pg_stat_statements WHERE query LIKE '%audit_logs%' OR query LIKE '%notes%' ORDER BY calls DESC;
  ```

  Expect **2 calls** for `join` (the audit list + the JOIN — actually it's 1 SQL statement, but Postgres counts the SELECT). Now reset and try `in_batch` → expect **2 calls** (list + IN). Then `naive` → expect **51 calls** (list + 50 individual gets).

- [ ] Time each one. The difference on localhost is small. On a real network it's not.

---

## Task 1 — EXPLAIN the cursor query

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, user_id, title, body, created_at, updated_at
FROM   notes
WHERE  user_id = 1
ORDER  BY id DESC
LIMIT  21;
```

- [ ] Confirm the plan uses `Index Scan Backward using idx_notes_user_id_id_desc`.
- [ ] Note: no `Sort` step, low row count actually read.
- [ ] Now drop the composite index:
  ```sql
  DROP INDEX idx_notes_user_id_id_desc;
  CREATE INDEX idx_notes_user_id ON notes(user_id);
  ```
- [ ] Re-run EXPLAIN. Now you'll likely see `Sort` because the `(user_id)` index doesn't carry ordering.
- [ ] Restore the composite index (re-run migration 000004 manually or just bounce the server).

---

## Task 2 — EXPLAIN the JOIN

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT a.id, a.action, a.target_id, a.created_at, n.id, n.title
FROM   audit_logs a
LEFT JOIN notes n
  ON   n.id = a.target_id
 AND   a.target_type = 'note'
WHERE  a.user_id = 1
ORDER  BY a.created_at DESC, a.id DESC
LIMIT  50;
```

- [ ] Expect: `Index Scan Backward using idx_audit_logs_user_id_created_at on audit_logs` (the outer table), `Index Scan using notes_pkey on notes` (per row in the inner). No `Seq Scan`, no `Sort`.
- [ ] If you see `Hash Join` or `Merge Join` with `Seq Scan`s, Postgres decided the data was small enough to scan. That's fine for tiny tables — the plan changes with size.

---

## Task 3 — Seed 100k notes and re-EXPLAIN

A 50-row table is a toy. Try with serious volume:

```sql
INSERT INTO notes (user_id, title)
SELECT 1, 'note ' || s
FROM   generate_series(1, 100000) s;
```

- [ ] Re-run the JOIN EXPLAIN. Postgres might now switch from `Nested Loop` to `Hash Join` (because the inner table is large). Either is fine; the question is whether it's using indexes.
- [ ] Run `?strategy=naive` against this. Watch the wall-clock balloon — 50 individual queries instead of 1 join.
- [ ] Compare the timings. Write 1 line in "What I learned" with the actual numbers.

---

## Task 4 — Trace a missing index

- [ ] Force a Seq Scan by querying something unindexed — `WHERE title = 'note 42'`:
  ```sql
  EXPLAIN ANALYZE SELECT * FROM notes WHERE user_id = 1 AND title = 'note 42';
  ```
  Expect `Seq Scan` filtered by `user_id` (the index helps) but the title filter is post-index.
- [ ] Add a partial index:
  ```sql
  CREATE INDEX idx_notes_user_id_lower_title ON notes (user_id, LOWER(title));
  ```
  Re-EXPLAIN with `LOWER(title) = 'note 42'`. Expect `Bitmap Index Scan` or `Index Scan`.
- [ ] Drop the index. Some indexes look attractive but only pay off if a real query uses them.

---

## Task 5 — Tune pool against your actual workload

Default config: `MaxOpenConns=25, MaxIdleConns=25`. Try the extremes:

- [ ] Set `DB_MAX_OPEN_CONNS=2` in `.env`, restart, and burst 50 quick requests at `/audit?include=notes&limit=50`. Latencies will spike because requests are queuing on connections.
- [ ] Set back to 25. Same test — should be flat.
- [ ] Set `DB_MAX_OPEN_CONNS=200`. Postgres default max is 100, so depending on your other connections this may fail at startup. Watch the error.
- [ ] Pick a number you'd actually use in prod. Write it in "What I learned" with the reasoning.

---

## Task 6 — Confirm SetConnMaxLifetime works

`DB_CONN_MAX_LIFETIME=5m` means a connection lives at most 5 minutes before being recycled.

- [ ] Set it to `30s` for the test.
- [ ] In psql: `SELECT pid, query_start FROM pg_stat_activity WHERE application_name = 'pgx';` — note the pids.
- [ ] Hit `/healthz` to force a connection. Wait 30+ seconds. Hit it again. The pid will be different.
- [ ] Set it back to `5m`.

This is what protects you against silent stale-connection bugs after a load-balancer rebalances.

---

## Task 7 — The "two-strategy" test

Add a test to `internal/audit/repository_test.go` (build tag `integration`, like Day 24):

- [ ] Insert 5 notes via SQL, 5 audit rows referencing them.
- [ ] Call `ListWithNotesJoin` → expect 5 entries, all with notes.
- [ ] Call `ListWithNotesInBatch` → expect 5 entries, all with notes.
- [ ] Call `ListWithNotesNaive` → expect 5 entries, all with notes.
- [ ] DELETE one of the notes.
- [ ] All three strategies must return 5 entries (the audit row still exists) with one having `note == nil`. This is the test that catches a regression where someone "fixes" the JOIN to be an INNER JOIN.

---

## Stretch — only if you're flying

- [ ] **`pg_stat_statements` in dev only**: enable it via `postgresql.conf` in your docker-compose, restart, and use it to surface the top-3 queries by total time after a load test.
- [ ] **Adaptive strategy**: make the audit handler pick `join` for `limit ≤ 50` and `in_batch` for `limit > 50`. Justify the cutoff with EXPLAIN.
- [ ] **`EXPLAIN (FORMAT JSON)`**: parse the JSON in Go, write a startup self-check that confirms the query uses an index. Fail-fast if not.
- [ ] **Pool metrics**: expose `db.Stats()` (open, idle, in-use, wait count, wait duration) via a tiny `/db-stats` debug endpoint.

---

## What I learned (Day 30)

> 3 bullets in your own words.

-
-
-
