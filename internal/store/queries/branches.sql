-- name: ListBranches :many
-- Liste toutes les branches connues du depot (suivies ou non), triees par nom.
SELECT * FROM branches
WHERE repo_id = ?
ORDER BY name;

-- name: UntrackBranches :exec
-- Passe toutes les branches du depot a tracked = 0 (les branches suivies sont ensuite reecrites).
UPDATE branches
SET tracked = 0
WHERE repo_id = ?;

-- name: UpsertTrackedBranch :one
-- Cree la branche ou met a jour sa tete ; dans les deux cas elle devient suivie.
INSERT INTO branches (repo_id, name, head_sha, tracked)
VALUES (?, ?, ?, 1)
ON CONFLICT (repo_id, name) DO UPDATE SET head_sha = excluded.head_sha, tracked = 1
RETURNING id;

-- name: CountTrackedBranches :one
-- Nombre de branches suivies du depot.
SELECT count(*) FROM branches
WHERE repo_id = ? AND tracked = 1;
