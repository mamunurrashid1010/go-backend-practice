ALTER TABLE todos
    ADD COLUMN priority TEXT NOT NULL DEFAULT 'low'
    CHECK (priority IN('low', 'medium','high'));