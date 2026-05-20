DROP INDEX IF EXISTS idx_todos_user_id;

ALTER TABLE todos
    DROP COLUMN IF EXISTS user_id;
