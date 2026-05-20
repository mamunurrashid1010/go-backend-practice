ALTER TABLE todos
    ADD COLUMN deleted_at TIMESTAMPTZ;

-- partial index — only the "alive" rows
CREATE INDEX idx_todos_alive ON todos(id) WHERE deleted_at IS NULL;