# Day 32 — Practice Tasks

The code ships a working cache-aside on `GET /notes/{id}`. The tasks make you **see** the hit/miss/invalidate paths, force a thundering herd to feel singleflight work, then explore the ordering trap and TTL math.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day32
> go mod tidy
> go run .
> ```

You'll want `redis-cli` handy:

```powershell
docker exec -it day32-redis redis-cli
# MONITOR — watch every command the server sees (great for the demo).
```

---

## Warm-up — see hit, miss, invalidate

Log in, save `$tok`, save a note id `$id`.

- [ ] Open a `redis-cli MONITOR` in a second terminal.
- [ ] `curl GET /notes/$id` once. MONITOR shows a `GET note:1:$id` (miss) and a `SET note:1:$id "{...}" PX ...`.
- [ ] `curl GET /notes/$id` again. MONITOR shows only a `GET` (no SET; hit).
- [ ] `curl PATCH /notes/$id` with a new title. MONITOR shows a `DEL`.
- [ ] `curl GET /notes/$id` — back to miss + SET.

If you don't see the SET on the first request, your TTL is 0 or Redis isn't connected — check `redis-cli PING`.

---

## Task 1 — Confirm what's actually in Redis

- [ ] `docker exec -it day32-redis redis-cli get "note:1:$id"` should print the JSON.
- [ ] `redis-cli ttl "note:1:$id"` should print a number close to but slightly above 300 (5min + jitter).
- [ ] After a PATCH: `redis-cli get "note:1:$id"` returns `(nil)`.

---

## Task 2 — Provoke a thundering herd

`hey` or `vegeta` will do this, but PowerShell can fake it with parallel jobs:

```powershell
1..50 | ForEach-Object -Parallel {
    curl.exe -s -H "Authorization: Bearer $using:tok" -o $null "http://localhost:8080/notes/$using:id"
} -ThrottleLimit 50
```

- [ ] First, manually clear the cache: `redis-cli del "note:1:$id"`.
- [ ] Run the 50-request burst.
- [ ] Look at the server logs. With singleflight: **one** DB SELECT, the other 49 wait for it. Without singleflight: 50 SELECTs, all racing.

To prove this is singleflight and not the cache catching up: temporarily replace `s.sf.Do(...)` in [service.go](internal/notes/service.go) with a direct `s.repo.Get(ctx, ...)`, rerun, watch 50 selects fly past, then revert.

---

## Task 3 — Feel the ordering trap

This is the "why we delete AFTER commit" lesson made concrete.

- [ ] In [service.go](internal/notes/service.go) `Patch`, move `s.invalidate(...)` to BEFORE `s.tx.InTx(...)`. Run the warm-up: `GET`, `PATCH`, immediately `GET`. With clever timing, you can sometimes catch a stale cached value. (It's racy and hard to provoke locally — the README walks the reasoning.)
- [ ] Move `s.invalidate(...)` INSIDE `s.tx.InTx(...)`. The cache gets blown away even when the tx rolls back. Force a rollback by panicking inside the audit insert (set `audit.Log` to `panic("test")` temporarily) — watch the cache get cleared and the row stay unchanged. Compare to "after commit only" which would keep the cache *and* the DB consistent.
- [ ] Revert to the canonical after-commit form.

---

## Task 4 — A real integration test for the cache

The unit tests in [cache_test.go](internal/cache/cache_test.go) cover the nil-Cache path. Write a real test:

- [ ] Add a build-tagged test file `cache_pg_test.go` (or use a similar pattern from Day 24 integration tests):
  ```go
  //go:build integration
  package cache_test
  ```
- [ ] Use `testcontainers-go` to spin up Redis, connect, and verify SetJSON+GetJSON round-trips a struct, Delete removes it, and TTL is approximately respected (allow ±20% slop).

---

## Task 5 — Cache the list query (decide whether to)

`GET /notes?limit=20&after=...` is **not** cached. Should it be?

- [ ] Sketch the key shape: `note-list:<userID>:<sortDesc>:<search>:<limit>:<after>`. Notice how every variant makes a new key.
- [ ] Sketch the invalidation: any note write must invalidate **all** list keys for that user. The simplest approach is a per-user version counter:
  ```
  key := fmt.Sprintf("note-list:%d:v%d:%s", userID, versionCounter, args)
  // On any write: redis.INCR("note-list-version:<userID>")
  ```
- [ ] Don't actually implement it — just answer in "What I learned": at what request rate does this pay off? When would you not bother?

---

## Task 6 — Replace JSON with a binary codec

JSON is fine; it's also the slowest part of the cache round trip:

- [ ] Try `msgpack` (`github.com/vmihailenco/msgpack/v5`). Replace `json.Marshal`/`Unmarshal` in [cache.go](internal/cache/cache.go) with msgpack equivalents.
- [ ] Benchmark with `go test -bench=. ./internal/cache/...` — small numbers locally, real numbers under load.

Not always worth it (operational cost: now you can't `redis-cli get` and see a human-readable value), but worth knowing.

---

## Stretch — only if you're flying

- [ ] **Stale-while-revalidate**: return the cached value AND kick off a background refresh when the value is older than half its TTL. Smooths the latency spike on the request that catches the miss.
- [ ] **Redis pipelining**: when invalidating multiple keys (e.g. tag-based cache), use `redis.Pipelined(ctx, func(p Pipeliner) ...)` to send N DELs in one round trip.
- [ ] **Cache stampede protection in Redis** (Day 33 preview): `SET key val NX PX <ttl>` — only the first writer succeeds; others see a "loading" sentinel and back off.
- [ ] **Negative caching**: cache the *miss* for `GET /notes/{nonExistent}` with a short TTL so 1000 reqs for an invalid id don't hammer Postgres. Mark with a sentinel like `null` or `{tombstone:true}`.

---

## What I learned (Day 32)

> 3 bullets in your own words.

-
-
-
