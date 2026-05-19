-- Day 8 — guided walkthrough.
--
-- Don't run this whole file. Open psql, then copy each labelled block,
-- read the output, modify a value, run again. The mistakes are the lesson.
--
--   docker compose exec postgres psql -U app -d appdb


-- ============================================================
-- 1. Inspect what we have
-- ============================================================

\dt                          -- list tables
\d todos                     -- describe the todos table

SELECT * FROM todos;         -- the two seeded rows


-- ============================================================
-- 2. SELECT — projections, filters, ordering
-- ============================================================

SELECT id, title FROM todos;

SELECT * FROM todos WHERE done = false;
SELECT * FROM todos WHERE id = 1;

SELECT * FROM todos WHERE title ILIKE '%sql%';        -- case-insensitive contains
SELECT * FROM todos WHERE title LIKE 'learn%';        -- case-sensitive starts-with

SELECT * FROM todos ORDER BY created_at DESC;
SELECT * FROM todos ORDER BY id LIMIT 1 OFFSET 1;     -- pagination, the bad way

SELECT COUNT(*)                  FROM todos;
SELECT done, COUNT(*) AS n       FROM todos GROUP BY done;


-- ============================================================
-- 3. INSERT — single, multi, with RETURNING
-- ============================================================

INSERT INTO todos (title) VALUES ('practice psql');

INSERT INTO todos (title, done) VALUES
    ('ship the day',  false),
    ('grab coffee',   true),
    ('write SQL day', false);

-- RETURNING is Postgres-specific and absolutely essential later in Go.
INSERT INTO todos (title) VALUES ('learn RETURNING') RETURNING id, created_at;


-- ============================================================
-- 4. UPDATE — with WHERE, with RETURNING
-- ============================================================

UPDATE todos SET done = true WHERE id = 1;

UPDATE todos
SET    title = 'learn SQL slowly',
       updated_at = now()
WHERE  id = 1
RETURNING *;

-- DANGER: no WHERE means every row.
-- UPDATE todos SET done = true;     -- DO NOT RUN UNLESS YOU MEAN IT


-- ============================================================
-- 5. DELETE
-- ============================================================

DELETE FROM todos WHERE title = 'grab coffee';

-- DANGER: no WHERE means every row.
-- DELETE FROM todos;
-- TRUNCATE todos;     -- same effect, faster, also resets the IDENTITY counter


-- ============================================================
-- 6. NULL — feel the trap
-- ============================================================

SELECT NULL = NULL;          -- NULL, not true
SELECT 1 = NULL;             -- NULL
SELECT NULL IS NULL;         -- true — use IS / IS NOT for NULL checks

-- COUNT(*) counts rows; COUNT(col) ignores NULLs in that column.
SELECT COUNT(*) FROM todos;


-- ============================================================
-- 7. Transactions — feel rollback
-- ============================================================

BEGIN;
    UPDATE todos SET done = true WHERE id = 2;
    SELECT id, done FROM todos WHERE id = 2;   -- see the change
ROLLBACK;
SELECT id, done FROM todos WHERE id = 2;       -- back to original

BEGIN;
    UPDATE todos SET done = true WHERE id = 2;
COMMIT;
SELECT id, done FROM todos WHERE id = 2;       -- the change persists


-- ============================================================
-- 8. JOIN preview (Day 11 will use this)
-- ============================================================

-- Don't run this yet — we don't have a users table until Day 11.
-- It's here so the shape feels familiar when you see it again.
--
-- SELECT t.id, t.title, u.name AS owner
-- FROM   todos t
-- JOIN   users u ON u.id = t.user_id
-- ORDER  BY t.id;


-- ============================================================
-- 9. Indexes — preview (Day 30)
-- ============================================================

EXPLAIN ANALYZE SELECT * FROM todos WHERE done = false;

CREATE INDEX IF NOT EXISTS idx_todos_done ON todos(done);

EXPLAIN ANALYZE SELECT * FROM todos WHERE done = false;

-- Compare the two EXPLAIN plans. With this few rows, Postgres may still
-- prefer a sequential scan (it's faster on tiny tables). The point is to
-- see EXPLAIN ANALYZE output and learn to read it.
