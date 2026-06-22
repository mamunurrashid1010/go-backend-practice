# Day 32 — Redis + Cache-Aside

> **Goal:** put Redis in front of `GET /notes/{id}`. Read path tries Redis first; on miss, fetches from Postgres and fills the cache. Write paths (`PUT` / `PATCH` / `DELETE`) invalidate the cached key after commit. Concurrent misses on the same key get coalesced via `singleflight` so a thousand readers don't all hit Postgres for the same row.

This is Day 32 of 105. The first time the API touches a second piece of infrastructure for performance rather than persistence.

---

## 1. Cache-aside, in 5 lines

```
read:   if (v in cache) return v
        v = db.read()
        cache.set(v, ttl)
        return v

write:  db.write()
        cache.delete(key)    # after a successful commit
```

The defining choice is **the application owns both**: cache and DB. The cache doesn't know about the DB and vice versa. Compare to:

- **Read-through**: the app talks only to the cache; the cache loads from DB on miss.
- **Write-through**: app writes to cache, cache writes to DB synchronously.
- **Write-behind**: app writes to cache, cache flushes to DB asynchronously.

Cache-aside is the simplest, the most flexible, and the default in every Go service I've worked on. The DB stays authoritative; the cache is a hint that you re-derive when wrong.

---

## 2. The ordering problem on writes

A write to a row demands two operations:

1. `UPDATE notes SET title = ... WHERE id = $1`
2. `DEL note:42` in Redis

The order matters. Three ways to do it, two are wrong:

### A — Delete cache **then** update DB (wrong)

- Concurrent reader misses cache.
- Reader queries DB (still old value).
- Reader writes old value to cache.
- Writer commits the new value.
- **Cache now has stale data and there's no event left to clear it.**

This is the standard "write-aside hazard." Don't do it.

### B — Update DB **then** delete cache (canonical)

- Writer commits.
- Window: a reader between commit and `DEL` reads from cache and sees the old value.
- Writer deletes the cache.
- Next reader misses, reads new value, refills.

Window size: tens of microseconds locally, single-digit milliseconds across the network. Acceptable for almost every workload.

### C — Update DB then update cache (tempting; subtly wrong under concurrency)

Two concurrent writers can finish their UPDATEs in order A→B but call `cache.Set` in order B→A. The cache now has the older value of two concurrent writes. With `DEL` only, the next reader rebuilds from the truth.

**Our service uses B.** The invalidation runs *after* `tx.InTx` returns nil — outside the transaction so a rollback doesn't drop the cache for no reason.

---

## 3. Concurrent misses — `singleflight`

Imagine 100 readers hit `GET /notes/42` simultaneously. Without protection:

- All 100 check Redis → miss.
- All 100 query Postgres for note 42.
- All 100 write the result to Redis.
- Postgres just did 100x the work for one row.

The pattern is called **thundering herd**. Fix: coalesce in-flight requests for the same key so only one actually hits the DB.

`golang.org/x/sync/singleflight` gives you this in 4 lines:

```go
type Service struct {
    sf singleflight.Group
}

func (s *Service) Get(ctx, userID, id) (Note, error) {
    key := fmt.Sprintf("note:%d:%d", userID, id)
    if n, ok := s.cache.GetJSON(ctx, key, &Note{}); ok { return n, nil }

    v, err, _ := s.sf.Do(key, func() (any, error) {
        return s.repo.Get(ctx, userID, id)
    })
    if err != nil { return Note{}, err }
    n := v.(Note)
    _ = s.cache.SetJSON(ctx, key, n, s.ttl)
    return n, nil
}
```

When 100 goroutines call `sf.Do(key, fn)` at once for the same `key`:

- One goroutine runs `fn`.
- The other 99 block, waiting.
- When `fn` returns, all 100 receive the same result.

`Do` returns `(value, err, shared)` — the third arg tells you whether you were one of the joiners (useful in metrics). The Service ignores it.

`singleflight` is **per-process**. With 10 instances of the API, you still get 10 concurrent fetches on a flash miss. Day 33's Redis-distributed lock improves this when N>10 matters.

---

## 4. Cache key naming

Three rules:

1. **Namespace the prefix.** `note:42` collides with anything else in the keyspace called `note:42`. We use `note:<userID>:<noteID>` — scoped by tenant.
2. **Don't include data that mutates.** Don't bake `updated_at` into the key — invalidation would be impossible.
3. **Don't include data that's user-controllable without a separator.** `note:alice` vs `note:al:ice` is the classic injection seam.

Our key:

```go
func noteKey(userID, id int64) string {
    return fmt.Sprintf("note:%d:%d", userID, id)
}
```

For a fancier deployment you'd add a version prefix (`v1:note:...`) so you can roll a schema change by bumping the prefix — instant zero-downtime cache flush.

---

## 5. TTL strategy

Two things to tune:

- **TTL** — how long a cached entry survives without invalidation. Our default: **5 minutes.**
- **TTL jitter** — randomize ±10% so a million keys written together don't all expire at the same instant.

Why have a TTL at all when you invalidate on writes? Three reasons:

1. **The invalidate can fail.** Redis is unreachable for 200ms. Writes commit; deletes fail. The TTL is the upper bound on staleness.
2. **Out-of-band updates.** Someone runs an UPDATE in psql for a fix. No Go code, no invalidate. The TTL is the only thing that ever expires that.
3. **Memory pressure.** Redis caps total memory. Without TTLs you fight Redis's eviction policy directly; with them, cold keys leave on their own.

5 minutes is a safe default for our notes — small read load, low write rate. A high-traffic endpoint might want 60s; a slow-mutating reference table might want 24h.

---

## 6. What we cache and what we don't

In this day:

- **Cached:** `GET /notes/{id}`. The classic single-row read.
- **Not cached:** `GET /notes` (list with cursor). Cache-aside on lists is hard — you'd need to invalidate the cache on *any* note write, which makes the cache barely useful.
- **Not cached:** `GET /audit`. Append-only, read mostly once.

A common production pattern is to cache the list *page* keyed by `(userID, after_cursor, limit, sort)`. Day 33 covers patterns where that pays off.

---

## 7. What changed from Day 31

| File | Change |
| --- | --- |
| `docker-compose.yml` | + Redis service on `localhost:6379` |
| `internal/config/config.go` | + `Redis` sub-config (URL, TTL, jitter) |
| `internal/cache/cache.go` | **NEW** — go-redis wrapper: `GetJSON`, `SetJSON`, `Delete` |
| `internal/cache/cache_test.go` | **NEW** — smoke tests |
| `internal/notes/service.go` | `Get` is cache-aside + singleflight; `Update`/`Patch`/`Delete` invalidate after commit |
| `main.go` | builds the Redis client and `*cache.Cache`, hands to `notes.NewService(...)` |

The `notes.PostgresRepository` (sqlc + hand-written List) and the audit JOIN strategies are unchanged — caching is a service-layer concern.

---

## 8. Run it

```powershell
cd Day_32_redis_cache
docker compose up -d                    # postgres + redis
go mod init day32
go mod tidy
go run .
```

Confirm Redis is reachable:

```powershell
docker exec -it day32-redis redis-cli ping
# PONG
```

Walk a cache-aside loop:

```powershell
$tok = (curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"a@b.dev\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json).access_token

# Create a note
$id = (curl.exe -s -H "Authorization: Bearer $tok" -H "Content-Type: application/json" `
  -d "{\"title\":\"cached\"}" http://localhost:8080/notes | ConvertFrom-Json).id

# 1st GET — MISS, populates the cache
curl.exe -s -H "Authorization: Bearer $tok" http://localhost:8080/notes/$id | Out-Null

# 2nd GET — HIT (no DB query). Watch your structured logs — only the
# first request emits the SELECT.
curl.exe -s -H "Authorization: Bearer $tok" http://localhost:8080/notes/$id | Out-Null

# See the key directly
docker exec -it day32-redis redis-cli get "note:1:$id"

# Invalidate via PATCH
curl.exe -s -H "Authorization: Bearer $tok" -X PATCH -H "Content-Type: application/json" `
  -d "{\"title\":\"changed\"}" http://localhost:8080/notes/$id | Out-Null

# Key is gone
docker exec -it day32-redis redis-cli get "note:1:$id"
# (nil)
```

---

## 9. What's next

**Day 33 — Redis distributed rate limiting + idempotency keys.** The in-process rate limiter from Day 27 doesn't scale across replicas (each replica has its own bucket). Redis-backed limiter solves it. Idempotency keys (`Idempotency-Key` header) dedupe POST retries.
