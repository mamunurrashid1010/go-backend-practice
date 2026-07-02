-- name: CreateNote :one
INSERT INTO notes (user_id, title, body)
VALUES ($1, $2, $3)
RETURNING id, user_id, title, body, created_at, updated_at;

-- name: GetNote :one
SELECT id, user_id, title, body, created_at, updated_at
FROM   notes
WHERE  id = $1 AND user_id = $2;

-- name: UpdateNote :one
UPDATE notes
SET    title = $3, body = $4, updated_at = now()
WHERE  id = $1 AND user_id = $2
RETURNING id, user_id, title, body, created_at, updated_at;

-- name: PatchNote :one
UPDATE notes
SET    title = COALESCE(sqlc.narg('title'), title),
       body  = COALESCE(sqlc.narg('body'),  body),
       updated_at = now()
WHERE  id = $1 AND user_id = $2
RETURNING id, user_id, title, body, created_at, updated_at;

-- name: DeleteNote :execrows
DELETE FROM notes
WHERE  id = $1 AND user_id = $2;
