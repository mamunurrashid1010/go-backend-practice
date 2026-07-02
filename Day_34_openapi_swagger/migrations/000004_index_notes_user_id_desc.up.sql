DROP INDEX IF EXISTS idx_notes_user_id;
CREATE INDEX idx_notes_user_id_id_desc ON notes (user_id, id DESC);
