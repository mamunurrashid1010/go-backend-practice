DROP INDEX IF EXISTS idx_todos_alive;

ALTER TABLE todos
    DROP COLUMN IF EXISTS deleted_at;