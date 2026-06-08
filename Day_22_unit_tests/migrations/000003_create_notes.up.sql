CREATE TABLE notes (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT        NOT NULL,
    body       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_notes_user_id ON notes(user_id);
