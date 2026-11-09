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
    additions, deletions, stats_known, url, parent_count
) VALUES (
    ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?
)
ON CONFLICT (repo_id, sha) DO NOTHING;

-- name: SetCommitStats :execrows
-- Enregistre les statistiques d'un commit deja en base dont elles sont inconnues et les marque connues
-- (des statistiques connues ne sont jamais recalculees).
UPDATE commits
SET additions = sqlc.arg(additions), deletions = sqlc.arg(deletions), stats_known = 1
WHERE repo_id = sqlc.arg(repo_id) AND sha = sqlc.arg(sha) AND stats_known = 0;

-- name: ListUnknownStatsSHAs :many
-- SHA des commits du depot dont les statistiques sont inconnues, ou les langages lus avec une version de
-- regles inferieure a languages_version, par ordre croissant, apres after_sha (pagination ; chaine vide =
-- depuis le debut), au plus row_limit.
SELECT sha FROM commits
WHERE repo_id = sqlc.arg(repo_id)
  AND (stats_known = 0 OR languages_version < sqlc.arg(languages_version))
  AND sha > sqlc.arg(after_sha)
ORDER BY sha
LIMIT sqlc.arg(row_limit);

-- name: SetCommitParentCount :execrows
-- Enregistre le nombre de parents d'un commit deja en base dont il est inconnu (un nombre connu n'est
-- jamais reecrit).
UPDATE commits
SET parent_count = sqlc.arg(parent_count)
WHERE repo_id = sqlc.arg(repo_id) AND sha = sqlc.arg(sha) AND parent_count IS NULL;

-- name: ListUnknownParentSHAs :many
-- SHA des commits du depot dont le nombre de parents est inconnu, par ordre croissant, apres after_sha
-- (pagination ; chaine vide = depuis le debut), au plus row_limit.
SELECT sha FROM commits
WHERE repo_id = sqlc.arg(repo_id)
  AND parent_count IS NULL
  AND sha > sqlc.arg(after_sha)
ORDER BY sha
LIMIT sqlc.arg(row_limit);

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

-- name: ListCommitsToScanForAI :many
-- Commits du depot pas encore analyses avec la version de regles current_version (au plus row_limit),
-- avec le login et les adresses de leur auteur (separees par un saut de ligne).
SELECT c.sha, c.message,
       CAST(COALESCE(a.provider_login, '') AS TEXT) AS author_login,
       CAST(COALESCE((SELECT group_concat(e.email, char(10)) FROM contributor_emails e
                      WHERE e.contributor_id = c.author_id), '') AS TEXT) AS author_emails
FROM commits c
LEFT JOIN contributors a ON a.id = c.author_id
WHERE c.repo_id = sqlc.arg(repo_id) AND c.ai_rules_version < sqlc.arg(current_version)
ORDER BY c.sha
LIMIT sqlc.arg(row_limit);

-- name: DeleteCommitAISignatures :exec
-- Retire les signatures d'un commit avant de les reecrire.
DELETE FROM commit_ai_signatures
WHERE repo_id = ? AND sha = ?;

-- name: InsertCommitAISignature :exec
-- Enregistre une signature d'un commit, sans doublon.
INSERT INTO commit_ai_signatures (repo_id, sha, tool, kind)
VALUES (?, ?, ?, ?)
ON CONFLICT (repo_id, sha, tool, kind) DO NOTHING;

-- name: SetCommitAIRulesVersion :exec
-- Marque un commit analyse avec cette version de regles.
UPDATE commits
SET ai_rules_version = sqlc.arg(rules_version)
WHERE repo_id = sqlc.arg(repo_id) AND sha = sqlc.arg(sha);

-- name: DeleteCommitLanguages :exec
-- Retire les langages d'un commit avant de les reecrire.
DELETE FROM commit_languages
WHERE repo_id = ? AND sha = ?;

-- name: InsertCommitLanguage :exec
-- Enregistre les lignes ajoutees dans un langage par un commit.
INSERT INTO commit_languages (repo_id, sha, language, additions)
VALUES (?, ?, ?, ?);

-- name: SetCommitLanguagesVersion :execrows
-- Marque les langages d'un commit lus avec cette version de regles (0 ligne : commit absent).
UPDATE commits
SET languages_version = sqlc.arg(languages_version)
WHERE repo_id = sqlc.arg(repo_id) AND sha = sqlc.arg(sha);

-- name: ListConventionCommits :many
-- Commits du depot pour les conventions (phase 8.1), du plus recent au plus ancien : message (lu en
-- memoire seulement, jamais garde), nombre de parents (NULL = inconnu), auteur et committer (logins et
-- adresses separees par des espaces), date du committer (phase 8.5, PR3), rattachement a une branche suivie et a la branche par defaut
-- (commit_branches, repli sans clone). author_id NULL = tous les auteurs.
SELECT c.sha, c.message, c.parent_count, c.authored_at, c.committed_at,
       CAST(COALESCE(c.author_id, 0) AS INTEGER) AS author_id,
       CAST(COALESCE(a.name, '') AS TEXT) AS author_name,
       CAST(COALESCE(a.provider_login, '') AS TEXT) AS author_login,
       CAST(COALESCE((SELECT group_concat(ae.email, ' ') FROM contributor_emails ae
                      WHERE ae.contributor_id = c.author_id), '') AS TEXT) AS author_emails,
       CAST(COALESCE(cm.provider_login, '') AS TEXT) AS committer_login,
       CAST(COALESCE((SELECT group_concat(ce.email, ' ') FROM contributor_emails ce
                      WHERE ce.contributor_id = c.committer_id), '') AS TEXT) AS committer_emails,
       CAST(EXISTS (SELECT 1 FROM commit_branches cb JOIN branches b ON b.id = cb.branch_id
                    WHERE cb.repo_id = c.repo_id AND cb.sha = c.sha AND b.tracked = 1) AS INTEGER) AS on_tracked,
       CAST(EXISTS (SELECT 1 FROM commit_branches cb JOIN branches b ON b.id = cb.branch_id
                    JOIN repositories r ON r.id = c.repo_id
                    WHERE cb.repo_id = c.repo_id AND cb.sha = c.sha AND b.name = r.default_branch) AS INTEGER) AS on_default
FROM commits c
LEFT JOIN contributors a ON a.id = c.author_id
LEFT JOIN contributors cm ON cm.id = c.committer_id
WHERE c.repo_id = sqlc.arg(repo_id)
  AND (sqlc.narg(author_id) IS NULL OR c.author_id = sqlc.narg(author_id))
ORDER BY c.authored_at DESC, c.sha;
