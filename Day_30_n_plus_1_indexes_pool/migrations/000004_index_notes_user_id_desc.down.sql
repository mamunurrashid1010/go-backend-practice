DROP INDEX IF EXISTS idx_notes_user_id_id_desc;
CREATE INDEX idx_notes_user_id ON notes(user_id);
