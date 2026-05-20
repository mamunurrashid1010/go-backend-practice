-- Adds an owner relationship between todos and users.
-- Pattern for "add a NOT NULL column to an existing table":
--   1. add the column nullable
--   2. backfill existing rows
--   3. promote to NOT NULL
-- We skip step 2 here only because we know the table is empty (fresh DB).

ALTER TABLE todos
    ADD COLUMN user_id BIGINT REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE todos
    ALTER COLUMN user_id SET NOT NULL;

CREATE INDEX idx_todos_user_id ON todos(user_id);
