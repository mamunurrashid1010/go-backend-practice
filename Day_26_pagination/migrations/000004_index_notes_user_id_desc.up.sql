-- Composite index for the canonical cursor-paginated scan:
--   WHERE user_id = $1 [AND id < $cursor] ORDER BY id DESC
-- With user_id as the leading column, Postgres locates one user's rows;
-- with id as the second column, the rows come out already ordered.
-- The redundant idx_notes_user_id is dropped — this index covers both.
DROP INDEX IF EXISTS idx_notes_user_id;
CREATE INDEX idx_notes_user_id_id_desc ON notes (user_id, id DESC);
