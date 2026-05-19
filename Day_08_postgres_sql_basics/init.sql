-- Day 8 — initial schema.
--
-- Runs ONCE on first start of a fresh Postgres data volume (because of how
-- the official postgres image's /docker-entrypoint-initdb.d/ works). To
-- re-run after changes, wipe the volume:  docker compose down -v
--
-- The shape of the `todos` table matches Day 7's Go Todo struct so we can
-- swap the in-memory store for Postgres on Day 12 without touching the API.

CREATE TABLE IF NOT EXISTS todos (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title       TEXT        NOT NULL,
    done        BOOLEAN     NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Two seed rows so SELECT * FROM todos isn't empty on first run.
INSERT INTO todos (title, done) VALUES
  ('learn SQL by typing it', false),
  ('walk through queries.sql in psql', true);
