# Day 8 — Practice Tasks

All tasks run inside `psql`. No Go today.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> docker compose exec postgres psql -U app -d appdb
> \timing on
> ```

---

## Warm-up

- [ ] `\l` — list databases. Confirm `appdb` exists.
- [ ] `\dt` — list tables. Confirm `todos` is there.
- [ ] `\d todos` — read the column types and the `DEFAULT now()` on the timestamps.
- [ ] `SELECT * FROM todos;` — see the two seeded rows.

---

## Task 1 — Insert 5 todos, half done, half not

- [ ] Use a **single** `INSERT` with multiple `VALUES` rows.
- [ ] Verify with `SELECT COUNT(*) FROM todos;` (should be 7 — 2 seed + 5 new).

---

## Task 2 — Find things

- [ ] All open todos: `SELECT * FROM todos WHERE done = false;`
- [ ] Todos whose title contains "sql" (case-insensitive): `ILIKE '%sql%'`.
- [ ] The 3 newest: `ORDER BY created_at DESC LIMIT 3`.
- [ ] How many of each state: `SELECT done, COUNT(*) FROM todos GROUP BY done;`

---

## Task 3 — Update with `RETURNING`

- [ ] Pick one open todo. Mark it `done = true` AND update `updated_at = now()` in the same statement.
- [ ] Use `RETURNING *` to confirm the new row shape in one round-trip.
- [ ] Note: this is exactly the pattern Day 12 uses from Go.

---

## Task 4 — Soft delete (without changing the schema)

- [ ] Add a `deleted_at TIMESTAMPTZ` column: `ALTER TABLE todos ADD COLUMN deleted_at TIMESTAMPTZ;`
- [ ] "Soft delete" todo id=1: `UPDATE todos SET deleted_at = now() WHERE id = 1;`
- [ ] Write a SELECT that **excludes** soft-deleted: `WHERE deleted_at IS NULL`.
- [ ] Restore it: `UPDATE todos SET deleted_at = NULL WHERE id = 1;`

**Why:** soft-delete is the single most common schema decision after MVP. Knowing the `IS NULL` filter pattern is half of working with it.

---

## Task 5 — Transaction discipline

- [ ] Run this and observe the state at each step:
  ```sql
  BEGIN;
  UPDATE todos SET title = 'TEMP' WHERE id = 1;
  SELECT id, title FROM todos WHERE id = 1;
  ROLLBACK;
  SELECT id, title FROM todos WHERE id = 1;
  ```
- [ ] Now do the same with `COMMIT;` and confirm the change persists.

**Why:** Day 29 (transactions in Go) is just this with `BeginTx`. The mental model is the same.

---

## Task 6 — Build an index, read the plan

- [ ] `EXPLAIN ANALYZE SELECT * FROM todos WHERE done = false;` — note `Seq Scan` and the duration.
- [ ] `CREATE INDEX idx_todos_done ON todos(done);`
- [ ] Run `EXPLAIN ANALYZE` again. With ~7 rows Postgres may still prefer the seq scan; that's fine — the goal is to read the plan output, not beat the planner.
- [ ] Insert 10,000 rows and re-run:
  ```sql
  INSERT INTO todos (title, done)
  SELECT 'gen-' || g, (g % 2 = 0)
  FROM   generate_series(1, 10000) AS g;
  ```
- [ ] Now `EXPLAIN ANALYZE` should switch to an index scan (or bitmap heap scan). **That** is what the index does.

**Why:** Day 30 covers this. Seeing it once today means it won't be surprising.

---

## Task 7 — A users table + a foreign key

- [ ] Create a `users` table:
  ```sql
  CREATE TABLE users (
      id    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
      name  TEXT NOT NULL,
      email TEXT NOT NULL UNIQUE
  );
  ```
- [ ] Insert 2 users.
- [ ] Add a `user_id` column to `todos`:
  ```sql
  ALTER TABLE todos ADD COLUMN user_id BIGINT REFERENCES users(id) ON DELETE CASCADE;
  ```
- [ ] Assign every existing todo to user 1: `UPDATE todos SET user_id = 1;`
- [ ] Make it `NOT NULL`: `ALTER TABLE todos ALTER COLUMN user_id SET NOT NULL;`
- [ ] Write a JOIN: list every todo with its owner's name.
  ```sql
  SELECT t.id, t.title, u.name AS owner
  FROM   todos t
  JOIN   users u ON u.id = t.user_id
  ORDER  BY t.id;
  ```
- [ ] Try `DELETE FROM users WHERE id = 1;` — confirm the `ON DELETE CASCADE` removes their todos too.

**Why:** this is the schema Day 11 + Day 12 will use. You're doing it once by hand now so the Go code feels like instructions, not magic.

---

## Stretch

- [ ] Add a `priority TEXT CHECK (priority IN ('low','medium','high'))` column. Try inserting a row with `priority = 'urgent'` and watch Postgres reject it. This is a **CHECK constraint** — domain rules enforced by the DB.
- [ ] Read the Postgres tutorial section "[Concepts](https://www.postgresql.org/docs/16/tutorial-concepts.html)" — 5 minutes, sets a lot of context.
- [ ] Try `pgAdmin` or `DBeaver` instead of `psql` for one task — many people prefer a GUI for exploring schemas.


