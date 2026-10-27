-- name: CreateRepository :one
-- Ajoute un depot et renvoie la ligne creee (id compris). Viole UNIQUE si le depot existe deja.
INSERT INTO repositories (
    provider, provider_id, owner, name, label, default_branch, url, stars, open_issues,
    last_synced_at, last_commit_at, archived, flemme_after_days, dead_after_days, enabled,
    branch_patterns
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?
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

-- name: UpdateRepositorySettings :one
-- Remplace le label et les motifs de branches du depot et renvoie la ligne a jour.
UPDATE repositories
SET label = ?, branch_patterns = ?
WHERE id = ?
RETURNING *;

-- name: SetRepositoryEnabled :execrows
-- Active (1) ou arrete (0) le suivi du depot, sans toucher a son historique.
UPDATE repositories
SET enabled = ?
WHERE id = ?;

-- name: UpdateRepositoryMetadata :execrows
-- Ecrit les metadonnees lues sur la forge et la date de fin de la synchro reussie.
UPDATE repositories
SET owner = ?, name = ?, default_branch = ?, url = ?, stars = ?, open_issues = ?, archived = ?,
    last_synced_at = ?
WHERE id = ?;

-- name: SetRepositoryLastCommitAt :exec
-- Ecrit la date du dernier commit connu sur les branches suivies (NULL si aucun).
UPDATE repositories
SET last_commit_at = ?
WHERE id = ?;
