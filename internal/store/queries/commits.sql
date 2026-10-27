-- name: CountCommits :one
-- Nombre de commits en base du depot.
SELECT count(*) FROM commits
WHERE repo_id = ?;

-- name: ExistingCommitSHAs :many
-- SHA deja enregistres parmi ceux donnes (un paquet a la fois, voir ExistingCommitSHAs en Go).
SELECT sha FROM commits
WHERE repo_id = sqlc.arg(repo_id) AND sha IN (sqlc.slice(shas));

-- name: InsertCommit :execrows
-- Insere un commit ; un commit deja en base n'est jamais modifie (0 ligne inseree).
INSERT INTO commits (
    repo_id, sha, author_id, committer_id, message, authored_at, committed_at,
    additions, deletions, url
) VALUES (
    ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?
)
ON CONFLICT (repo_id, sha) DO NOTHING;

-- name: InsertCommitBranch :exec
-- Rattache un commit a une branche, sans doublon.
INSERT INTO commit_branches (repo_id, sha, branch_id)
VALUES (?, ?, ?)
ON CONFLICT (repo_id, sha, branch_id) DO NOTHING;

-- name: MaxTrackedCommittedAt :one
-- Date du commit le plus recent vu sur une branche suivie du depot ; chaine vide si aucun.
SELECT CAST(COALESCE(max(c.committed_at), '') AS TEXT) AS last_commit_at
FROM commits c
WHERE c.repo_id = sqlc.arg(repo_id)
  AND EXISTS (
    SELECT 1 FROM commit_branches cb
    JOIN branches b ON b.id = cb.branch_id
    WHERE cb.repo_id = c.repo_id AND cb.sha = c.sha AND b.tracked = 1
  );

-- name: ReassignCommitAuthors :exec
-- Reattribue a to_id les commits dont from_id est l'auteur (fusion de contributeurs).
UPDATE commits
SET author_id = sqlc.arg(to_id)
WHERE author_id = sqlc.arg(from_id);

-- name: ReassignCommitCommitters :exec
-- Reattribue a to_id les commits dont from_id est le committer (fusion de contributeurs).
UPDATE commits
SET committer_id = sqlc.arg(to_id)
WHERE committer_id = sqlc.arg(from_id);
