# Day 8 — Postgres, `psql`, and Basic SQL

> **Goal:** get Postgres running on your laptop, connect with `psql`, and write enough SQL by hand to make Day 9's Go code feel like "wiring", not magic.

No Go today. This is a database day. Spend the time **in `psql`** — once SQL is comfortable, the Go side is fast.

---

## 1. Two install paths — pick one

| Option | Best when… | Setup |
| --- | --- | --- |
| **A. Docker** (recommended on Windows) | You have Docker Desktop, want zero-config, easy reset | `docker compose up -d` in this folder |
| **B. Native installer** | You want a GUI (`pgAdmin`) and an always-on service | <https://www.postgresql.org/download/windows/> |

We'll use the **Docker path** in this README. If you go native, every command works the same — just skip Section 2 and use your installed `psql.exe` against `localhost:5432` with the password you set during install.

---

## 2. Run Postgres in Docker

The [docker-compose.yml](docker-compose.yml) in this folder defines a Postgres 16 service with sane defaults and auto-loads the [init.sql](init.sql) schema on first start.

```powershell
# from this folder
docker compose up -d
docker compose ps         # confirm "postgres" is "running"
docker compose logs -f postgres   # Ctrl+C to exit — useful for debugging
```

What got created:

- A Postgres 16 server on `localhost:5432`
- Database: `appdb`
- Superuser: `app` / password: `app`
- A `todos` table seeded with two rows (see [init.sql](init.sql))

> The data lives in a named Docker volume (`pgdata`) so it survives `docker compose down`. To **wipe and start fresh**: `docker compose down -v` (the `-v` deletes the volume).

---

## 3. Connect with `psql`

`psql` is the Postgres CLI. With Docker, the easiest way to get one is to use the client inside the container:

```powershell
docker compose exec postgres psql -U app -d appdb
```

You'll see a prompt:

```
appdb=#
```

That's where you spend today. Type `\q` to quit.

> If you installed Postgres natively, just run `psql -U app -d appdb` from PowerShell.

---

## 4. Essential `psql` meta-commands

Everything starting with a backslash is a `psql` command, not SQL.

| Command | What it does |
| --- | --- |
| `\l` | List databases |
| `\c appdb` | Connect to a database |
| `\dt` | List tables in the current schema |
| `\d todos` | Describe a table — columns, types, constraints, indexes |
| `\du` | List roles (users) |
| `\dn` | List schemas |
| `\x` | Toggle "expanded display" — readable rows when columns are wide |
| `\?` | Show all meta-commands |
| `\h CREATE TABLE` | Show SQL syntax help for a command |
| `\timing on` | Show how long each query took (you'll want this Day 30) |
| `\q` | Quit |
| `\i path/to/file.sql` | Run a SQL file |

Try them now — `\dt` should show `todos`; `\d todos` should show the columns.

---

## 5. The SQL you need today

### Data types (the ones you'll use 99% of the time)

| Type             | When to use                                |
| ---------------- | ------------------------------------------ |
| `BIGINT`         | IDs, counts (default to BIGINT, not INT)   |
| `TEXT`           | Strings — no length limit. Prefer over `VARCHAR(n)` unless you specifically need the limit |
| `BOOLEAN`        | true/false                                  |
| `TIMESTAMPTZ`    | Time + timezone. **Always prefer over `TIMESTAMP`** (no TZ → bugs) |
| `JSONB`          | Schema-less blobs you'll query into        |
| `UUID`           | If you don't want sequential IDs           |
| `NUMERIC(10,2)`  | Money. **Never `FLOAT`/`DOUBLE` for money.** |

### CREATE TABLE — read this carefully

```sql
CREATE TABLE todos (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title       TEXT      NOT NULL,
    done        BOOLEAN   NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Three habits the schema enforces:

1. **`NOT NULL` by default.** Adding `NOT NULL` everywhere makes invariants explicit. Nullable columns should be the exception, with a clear reason.
2. **`DEFAULT` instead of "the app will set it".** `created_at` is the server's job — let the DB own it.
3. **`GENERATED ALWAYS AS IDENTITY`** is the modern way to auto-increment (Postgres 10+). It replaces the older `SERIAL` you'll still see in old code. The "ALWAYS" stops apps from accidentally inserting their own IDs.

### INSERT

```sql
-- single row
INSERT INTO todos (title) VALUES ('learn SQL');

-- multiple rows
INSERT INTO todos (title, done) VALUES
  ('do laundry', false),
  ('walk the dog', true);

-- INSERT + return the row (Postgres-specific, very useful)
INSERT INTO todos (title) VALUES ('write Day 8') RETURNING id, created_at;
```

The `RETURNING` clause matters for Day 9: when Go inserts a row, you'll use `RETURNING id` to get back the new ID without a second query.

### SELECT

```sql
-- everything
SELECT * FROM todos;

-- specific columns
SELECT id, title, done FROM todos;

-- WHERE
SELECT * FROM todos WHERE done = false;
SELECT * FROM todos WHERE title ILIKE '%learn%';   -- case-insensitive LIKE
SELECT * FROM todos WHERE created_at > now() - interval '1 day';

-- ORDER, LIMIT, OFFSET
SELECT * FROM todos ORDER BY id DESC LIMIT 10 OFFSET 20;

-- COUNT
SELECT COUNT(*) FROM todos;
SELECT done, COUNT(*) FROM todos GROUP BY done;
```

### UPDATE

```sql
UPDATE todos SET done = true WHERE id = 1;

-- update + return the new row
UPDATE todos
SET    done = true, updated_at = now()
WHERE  id = 1
RETURNING *;
```

**Always include a `WHERE`.** `UPDATE todos SET done = true;` (no WHERE) updates **every row**. There's no "undo". If you're nervous, run `BEGIN;` first, do the UPDATE, then `SELECT` to verify, then `COMMIT;` or `ROLLBACK;`.

### DELETE

```sql
DELETE FROM todos WHERE id = 1;

-- "delete all but keep the table" — same risk as UPDATE without WHERE
DELETE FROM todos;
TRUNCATE todos;     -- faster equivalent, also resets identity counter
```

### NULL — the one footgun

`NULL` is **not** equal to anything, not even itself:

```sql
SELECT NULL = NULL;        -- NULL (not true!)
SELECT 1 = NULL;           -- NULL
SELECT NULL IS NULL;       -- true (use IS NULL / IS NOT NULL)
```

This is why many tutorials say "`WHERE col = NULL` always returns nothing" — it's not a bug, it's the spec.

---

## 6. Basic JOIN (you'll need this Day 11)

Imagine adding a `users` table:

```sql
CREATE TABLE users (
    id    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name  TEXT NOT NULL
);

ALTER TABLE todos
  ADD COLUMN user_id BIGINT REFERENCES users(id) ON DELETE CASCADE;
```

Then:

```sql
SELECT t.id, t.title, u.name AS owner
FROM   todos t
JOIN   users u ON u.id = t.user_id
ORDER BY t.id;
```

- `JOIN` (`INNER JOIN`) — only rows that match on both sides.
- `LEFT JOIN` — keep every row from the left table; right side is `NULL` if no match.
- `ON u.id = t.user_id` — the join condition.

Day 11 will introduce a `user_id` column for real; today you just need to recognise the shape.

---

## 7. Transactions — feel them now, use them Day 29

```sql
BEGIN;
UPDATE todos SET done = true WHERE id = 1;
SELECT * FROM todos WHERE id = 1;     -- see the change
ROLLBACK;                              -- undo it
SELECT * FROM todos WHERE id = 1;     -- back to original
```

A transaction is "a bunch of statements that succeed together or fail together". When you wrap a multi-step operation in `BEGIN ... COMMIT`, the DB guarantees atomicity. Day 29 (transactions in Go) builds on this.

---

## 8. Indexes — read this once, build them Day 30

```sql
CREATE INDEX idx_todos_done ON todos(done);
CREATE INDEX idx_todos_title_search ON todos (LOWER(title));
EXPLAIN ANALYZE SELECT * FROM todos WHERE done = false;
```

Indexes make `WHERE` and `ORDER BY` fast. They cost write speed and disk. **`EXPLAIN ANALYZE`** shows the query plan and whether the index was used. Day 30 covers this in depth.

---

## 9. How to actually practise

Open `psql` and walk through [queries.sql](queries.sql) line by line. Copy each block into the prompt, read the output, change a value, run it again. You'll learn more in 30 minutes of running real queries than 2 hours of reading.

Then do [TASKS.md](TASKS.md).

---

## 10. Common gotchas

- **Strings are single-quoted.** `'hello'`, not `"hello"`. Double quotes are for identifiers (column/table names with weird casing).
- **Statements end in `;`.** If `psql` shows `appdb-#` instead of `appdb=#`, it's waiting for a semicolon.
- **Booleans are `true` / `false`** — no quotes.
- **Timezone-aware time** — store everything in `TIMESTAMPTZ`. Postgres stores it as UTC internally and converts on display.
- **Comments** are `-- single line` or `/* multi line */`.
- **Case-insensitive keywords** — `select`, `SELECT`, `Select` are the same. Conventional style is upper-case keywords (`SELECT id FROM ...`), but it's just style.

---

## 11. What's next

**Day 9** connects Go to this Postgres with `database/sql` + the `pgx` driver. Today's `todos` table will be the target — and the queries you ran by hand will be the queries you write in Go.

If `docker compose up -d` worked and you can run `SELECT * FROM todos;` in `psql`, you're done for the day.
