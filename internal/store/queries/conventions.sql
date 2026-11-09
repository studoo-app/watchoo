-- name: UpsertRepositoryConventions :exec
-- Enregistre les criteres de fichiers d'une revision d'un depot (remplace la ligne precedente).
INSERT INTO repository_conventions (repo_id, rev_kind, tag_name, rev_sha, rules_version, config_hash,
                                    results, computed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (repo_id, rev_kind, tag_name) DO UPDATE SET
    rev_sha = excluded.rev_sha,
    rules_version = excluded.rules_version,
    config_hash = excluded.config_hash,
    results = excluded.results,
    computed_at = excluded.computed_at;

-- name: ListRepositoryConventions :many
-- Criteres de fichiers en cache (repo_id NULL = tous les depots), la tete puis les tags par nom.
SELECT repo_id, rev_kind, tag_name, rev_sha, rules_version, config_hash, results, computed_at
FROM repository_conventions
WHERE (sqlc.narg(repo_id) IS NULL OR repo_id = sqlc.narg(repo_id))
ORDER BY repo_id, rev_kind, tag_name;

-- name: DeleteRepositoryConventionTagsExcept :execrows
-- Supprime les lignes des tags d'un depot hors de la liste keep (liste non vide).
DELETE FROM repository_conventions
WHERE repo_id = sqlc.arg(repo_id)
  AND rev_kind = 'tag'
  AND tag_name NOT IN (sqlc.slice(keep));

-- name: DeleteRepositoryConventionTags :execrows
-- Supprime toutes les lignes des tags d'un depot.
DELETE FROM repository_conventions
WHERE repo_id = sqlc.arg(repo_id)
  AND rev_kind = 'tag';

-- name: DeleteRepositoryConventionHead :execrows
-- Supprime la ligne de la tete d'un depot (branche par defaut absente ou depot vide).
DELETE FROM repository_conventions
WHERE repo_id = sqlc.arg(repo_id)
  AND rev_kind = 'head';
