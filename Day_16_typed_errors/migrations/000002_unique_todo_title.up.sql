-- Artificial for teaching: real todos can repeat, but a UNIQUE constraint
-- is the simplest way to demonstrate 409 Conflict handling before we have
-- users (Day 21). Postgres returns SQLSTATE 23505 on a duplicate insert.
ALTER TABLE todos ADD CONSTRAINT todos_title_unique UNIQUE (title);
