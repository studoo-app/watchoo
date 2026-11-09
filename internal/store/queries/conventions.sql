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

-- name: ListTagObservations :many
-- Periodes d'observation des tags d'un depot (repo_id NULL = tous les depots), par depot, nom, premiere
-- observation.
SELECT id, repo_id, name, sha, first_seen_at, last_seen_at, ended_at, end_reason
FROM tag_observations
WHERE (sqlc.narg(repo_id) IS NULL OR repo_id = sqlc.narg(repo_id))
ORDER BY repo_id, name, first_seen_at, id;

-- name: ListOpenTagObservations :many
-- Periodes d'observation ouvertes des tags d'un depot.
SELECT id, repo_id, name, sha, first_seen_at, last_seen_at, ended_at, end_reason
FROM tag_observations
WHERE repo_id = ? AND ended_at IS NULL
ORDER BY name, id;

-- name: OpenTagObservation :exec
-- Ouvre une periode d'observation d'un couple (nom, SHA) : premiere et derniere observation a la date de
-- la lecture.
INSERT INTO tag_observations (repo_id, name, sha, first_seen_at, last_seen_at)
VALUES (sqlc.arg(repo_id), sqlc.arg(name), sqlc.arg(sha), sqlc.arg(seen_at), sqlc.arg(seen_at));

-- name: SeeTagObservation :execrows
-- Reporte la derniere observation d'une periode ouverte (jamais en arriere).
UPDATE tag_observations
SET last_seen_at = max(last_seen_at, sqlc.arg(seen_at))
WHERE repo_id = sqlc.arg(repo_id) AND name = sqlc.arg(name) AND sha = sqlc.arg(sha) AND ended_at IS NULL;

-- name: EndTagObservation :execrows
-- Ferme la periode ouverte d'un couple (nom, SHA) : reecrit ou supprime.
UPDATE tag_observations
SET ended_at = max(last_seen_at, sqlc.arg(ended_at)), end_reason = sqlc.arg(end_reason)
WHERE repo_id = sqlc.arg(repo_id) AND name = sqlc.arg(name) AND sha = sqlc.arg(sha) AND ended_at IS NULL;

-- name: InsertBranchRewrite :exec
-- Enregistre une reecriture observee de la branche par defaut.
INSERT INTO branch_rewrites (repo_id, branch, old_sha, new_sha, observed_at)
VALUES (?, ?, ?, ?, ?);

-- name: ListBranchRewrites :many
-- Reecritures observees de la branche par defaut (repo_id NULL = tous les depots), par depot, date.
SELECT id, repo_id, branch, old_sha, new_sha, observed_at
FROM branch_rewrites
WHERE (sqlc.narg(repo_id) IS NULL OR repo_id = sqlc.narg(repo_id))
ORDER BY repo_id, observed_at, id;
