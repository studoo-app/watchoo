-- name: CreateRepository :one
-- Ajoute un depot et renvoie la ligne creee (id compris). Viole UNIQUE si le depot existe deja.
INSERT INTO repositories (
    provider, provider_id, owner, name, label, default_branch, url, stars, open_issues,
    last_synced_at, last_commit_at, archived, flemme_after_days, dead_after_days, enabled
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetRepository :one
-- Renvoie le depot d'identifiant interne id (sql.ErrNoRows s'il n'existe pas).
SELECT * FROM repositories
WHERE id = ?;

-- name: GetRepositoryByName :one
-- owner et name sont compares sans tenir compte de la casse (COLLATE NOCASE dans le schema).
SELECT * FROM repositories
WHERE provider = ? AND owner = ? AND name = ?;

-- name: GetRepositoryByProviderID :one
-- Renvoie le depot par son identifiant numerique sur la forge (stable malgre les renommages).
SELECT * FROM repositories
WHERE provider = ? AND provider_id = ?;

-- name: ListRepositories :many
-- Liste tous les depots, tries par owner puis name (sans tenir compte de la casse).
SELECT * FROM repositories
ORDER BY owner, name;

-- name: DeleteRepository :execrows
-- Supprime le depot ; branches, commits, commit_branches et sync_runs suivent (ON DELETE CASCADE).
DELETE FROM repositories
WHERE id = ?;
