-- name: ListRecentCommits :many
-- Derniers commits filtres (depot, auteur, branche, periode), du plus recent au plus ancien.
-- author_email doit etre deja en minuscules ; author est compare sans casse (ASCII) au login et au nom.
-- row_limit negatif = pas de limite.
SELECT c.repo_id, c.sha, c.author_id, c.committer_id, c.message, c.authored_at, c.committed_at,
       c.additions, c.deletions, c.url,
       r.owner AS repo_owner, r.name AS repo_name, r.label AS repo_label,
       CAST(COALESCE(a.name, '') AS TEXT) AS author_name,
       CAST(COALESCE(a.provider_login, '') AS TEXT) AS author_login
FROM commits c
JOIN repositories r ON r.id = c.repo_id
LEFT JOIN contributors a ON a.id = c.author_id
WHERE (sqlc.narg(repo_id) IS NULL OR c.repo_id = sqlc.narg(repo_id))
  AND (sqlc.narg(since) IS NULL OR c.committed_at >= sqlc.narg(since))
  AND (sqlc.narg(branch) IS NULL OR EXISTS (
        SELECT 1 FROM commit_branches cb
        JOIN branches b ON b.id = cb.branch_id
        WHERE cb.repo_id = c.repo_id AND cb.sha = c.sha AND b.name = sqlc.narg(branch)))
  AND (sqlc.narg(author) IS NULL OR (c.author_id IS NOT NULL AND (
        a.provider_login = sqlc.narg(author) COLLATE NOCASE
        OR a.name = sqlc.narg(author) COLLATE NOCASE
        OR EXISTS (
          SELECT 1 FROM contributor_emails e
          WHERE e.contributor_id = c.author_id AND e.email = sqlc.narg(author_email)))))
ORDER BY c.committed_at DESC, c.sha
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountRecentCommits :one
-- Nombre de commits qui satisfont les filtres de ListRecentCommits (memes regles), sans limite.
SELECT count(*)
FROM commits c
LEFT JOIN contributors a ON a.id = c.author_id
WHERE (sqlc.narg(repo_id) IS NULL OR c.repo_id = sqlc.narg(repo_id))
  AND (sqlc.narg(since) IS NULL OR c.committed_at >= sqlc.narg(since))
  AND (sqlc.narg(branch) IS NULL OR EXISTS (
        SELECT 1 FROM commit_branches cb
        JOIN branches b ON b.id = cb.branch_id
        WHERE cb.repo_id = c.repo_id AND cb.sha = c.sha AND b.name = sqlc.narg(branch)))
  AND (sqlc.narg(author) IS NULL OR (c.author_id IS NOT NULL AND (
        a.provider_login = sqlc.narg(author) COLLATE NOCASE
        OR a.name = sqlc.narg(author) COLLATE NOCASE
        OR EXISTS (
          SELECT 1 FROM contributor_emails e
          WHERE e.contributor_id = c.author_id AND e.email = sqlc.narg(author_email)))));

-- name: ContributorRanking :many
-- Classement des auteurs : commits decroissants, puis dernier commit le plus recent, puis id.
-- row_limit negatif = pas de limite.
SELECT ct.id, ct.provider, ct.provider_user_id, ct.provider_login, ct.name, ct.avatar_url,
       count(*) AS commits,
       CAST(COALESCE(sum(c.additions), 0) AS INTEGER) AS additions,
       CAST(COALESCE(sum(c.deletions), 0) AS INTEGER) AS deletions,
       count(DISTINCT c.repo_id) AS repos,
       CAST(max(c.committed_at) AS TEXT) AS last_commit_at
FROM commits c
JOIN contributors ct ON ct.id = c.author_id
WHERE (sqlc.narg(repo_id) IS NULL OR c.repo_id = sqlc.narg(repo_id))
  AND (sqlc.narg(since) IS NULL OR c.committed_at >= sqlc.narg(since))
GROUP BY ct.id
ORDER BY commits DESC, last_commit_at DESC, ct.id
LIMIT sqlc.arg(row_limit);

-- name: CountCommitsByRepo :many
-- Nombre de commits par depot (repo_id NULL = tous les depots).
SELECT repo_id, count(*) AS commits
FROM commits
WHERE (sqlc.narg(repo_id) IS NULL OR repo_id = sqlc.narg(repo_id))
GROUP BY repo_id;

-- name: CountTrackedBranchesByRepo :many
-- Nombre de branches suivies par depot (repo_id NULL = tous les depots).
SELECT repo_id, count(*) AS branches
FROM branches
WHERE tracked = 1 AND (sqlc.narg(repo_id) IS NULL OR repo_id = sqlc.narg(repo_id))
GROUP BY repo_id;

-- name: LastCommitByRepo :many
-- Commit le plus recent de chaque depot sur ses branches suivies (egalite : SHA croissant), avec son auteur.
SELECT c.repo_id, c.sha, c.author_id, c.committer_id, c.message, c.authored_at, c.committed_at,
       c.additions, c.deletions, c.url,
       CAST(COALESCE(a.name, '') AS TEXT) AS author_name,
       CAST(COALESCE(a.provider_login, '') AS TEXT) AS author_login
FROM repositories r
JOIN commits c ON c.repo_id = r.id AND c.sha = (
    SELECT c2.sha FROM commits c2
    WHERE c2.repo_id = r.id AND EXISTS (
        SELECT 1 FROM commit_branches cb
        JOIN branches b ON b.id = cb.branch_id
        WHERE cb.repo_id = c2.repo_id AND cb.sha = c2.sha AND b.tracked = 1)
    ORDER BY c2.committed_at DESC, c2.sha
    LIMIT 1)
LEFT JOIN contributors a ON a.id = c.author_id
WHERE (sqlc.narg(repo_id) IS NULL OR r.id = sqlc.narg(repo_id));

-- name: LastSyncRunByRepo :many
-- Derniere synchro de chaque depot (started_at le plus recent, puis id le plus grand).
SELECT s.id, s.repo_id, s.started_at, s.finished_at, s.new_commits, s.status, s.error
FROM repositories r
JOIN sync_runs s ON s.id = (
    SELECT s2.id FROM sync_runs s2
    WHERE s2.repo_id = r.id
    ORDER BY s2.started_at DESC, s2.id DESC
    LIMIT 1)
WHERE (sqlc.narg(repo_id) IS NULL OR r.id = sqlc.narg(repo_id));

-- name: CommitTimesSince :many
-- Dates de commit (committed_at) des commits du depot (repo_id NULL = tous) depuis since, sans ordre garanti.
SELECT committed_at
FROM commits
WHERE (sqlc.narg(repo_id) IS NULL OR repo_id = sqlc.narg(repo_id))
  AND committed_at >= sqlc.arg(since);

-- name: AuthorCommitTimesSince :many
-- Auteur (author_id) et date de commit (committed_at) des commits d'auteur connu depuis since, sans ordre garanti.
SELECT author_id, committed_at
FROM commits
WHERE author_id IS NOT NULL
  AND committed_at >= sqlc.arg(since);

-- name: ContributorRepoBreakdown :many
-- Commits de chaque auteur par depot : nombre et dernier commit ; tri par auteur, commits decroissants, depot.
SELECT c.author_id AS contributor_id, c.repo_id,
       r.owner AS repo_owner, r.name AS repo_name, r.label AS repo_label,
       count(*) AS commits,
       CAST(max(c.committed_at) AS TEXT) AS last_commit_at
FROM commits c
JOIN repositories r ON r.id = c.repo_id
WHERE c.author_id IS NOT NULL
GROUP BY c.author_id, c.repo_id
ORDER BY c.author_id, commits DESC, c.repo_id;

-- name: LastSyncFinishedAt :one
-- Fin de la synchro la plus recente, tous depots et tous statuts confondus ; chaine vide si aucune.
SELECT CAST(COALESCE(max(finished_at), '') AS TEXT) AS last_finished_at
FROM sync_runs
WHERE finished_at IS NOT NULL;
