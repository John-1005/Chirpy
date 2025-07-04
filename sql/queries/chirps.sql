-- name: AddChirp :one
INSERT INTO chirps (id, created_at, updated_at, body, user_id)
VALUES (
  gen_random_uuid(),
  NOW(),
  NOW(),
  $1,
  $2
)
RETURNING *;



-- name: GetChirps :many
SELECT * FROM chirps
ORDER BY created_at;



-- name: GetChirpByID :one
SELECT * FROM CHIRPS
WHERE ID = $1;


-- name: DeleteChirpByID :exec
DELETE from chirps
WHERE id = $1 and user_id = $2;


-- name: GetChirpsByID :many
SELECT * FROM CHIRPS
WHERE user_id = $1;

