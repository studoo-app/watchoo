-- name: InsertCommitSignals :execrows
-- Enregistre les signaux d'un commit deja en base ; author_id et authored_at sont recopies du commit.
-- Une ligne deja presente n'est jamais modifiee (0 ligne inseree), un SHA absent de la base est ignore.
INSERT INTO commit_signals (
    repo_id, sha, author_id, authored_at, files_kept, files_excluded, additions_kept, deletions_kept,
    tool_ids, agent_config, bot_author, computed_at
)
SELECT c.repo_id, c.sha, c.author_id, c.authored_at,
    sqlc.arg(files_kept), sqlc.arg(files_excluded), sqlc.arg(additions_kept), sqlc.arg(deletions_kept),
    sqlc.arg(tool_ids), sqlc.arg(agent_config), sqlc.arg(bot_author), sqlc.arg(computed_at)
FROM commits c
WHERE c.repo_id = sqlc.arg(repo_id) AND c.sha = sqlc.arg(sha)
ON CONFLICT (repo_id, sha) DO NOTHING;

-- name: PurgeCommitSignals :execrows
-- Supprime les signaux des commits ecrits avant before (tous les depots de l'espace).
DELETE FROM commit_signals
WHERE authored_at < sqlc.arg(before);

-- name: ListCommitSignals :many
-- Signaux des commits du depot, du plus recent au plus ancien (date d'ecriture, puis SHA).
SELECT repo_id, sha, author_id, authored_at, files_kept, files_excluded, additions_kept, deletions_kept,
    tool_ids, agent_config, bot_author, computed_at
FROM commit_signals
WHERE repo_id = sqlc.arg(repo_id)
ORDER BY authored_at DESC, sha;

-- name: CountCommitSignals :one
-- Nombre de commits du depot qui ont des signaux.
SELECT count(*) FROM commit_signals
WHERE repo_id = sqlc.arg(repo_id);

-- name: ListDeclaredCommitSignals :many
-- Signaux des commits qui ont au moins un signal declaratif (outil declare ou auteur bot), filtres
-- (depot, auteur, periode sur la date d'ecriture), du plus recent au plus ancien. author_email doit
-- etre deja en minuscules ; author est compare sans casse (ASCII) au login et au nom.
SELECT cs.repo_id, cs.sha, cs.author_id, cs.authored_at, cs.files_kept, cs.files_excluded,
       cs.additions_kept, cs.deletions_kept, cs.tool_ids, cs.agent_config, cs.bot_author, cs.computed_at,
       c.url,
       r.owner AS repo_owner, r.name AS repo_name, r.label AS repo_label,
       CAST(COALESCE(a.name, '') AS TEXT) AS author_name,
       CAST(COALESCE(a.provider_login, '') AS TEXT) AS author_login
FROM commit_signals cs
JOIN commits c ON c.repo_id = cs.repo_id AND c.sha = cs.sha
JOIN repositories r ON r.id = cs.repo_id
LEFT JOIN contributors a ON a.id = cs.author_id
WHERE (cs.tool_ids <> '' OR cs.bot_author = 1)
  AND (sqlc.narg(repo_id) IS NULL OR cs.repo_id = sqlc.narg(repo_id))
  AND (sqlc.narg(since) IS NULL OR cs.authored_at >= sqlc.narg(since))
  AND (sqlc.narg(author) IS NULL OR (cs.author_id IS NOT NULL AND (
        a.provider_login = sqlc.narg(author) COLLATE NOCASE
        OR a.name = sqlc.narg(author) COLLATE NOCASE
        OR EXISTS (
          SELECT 1 FROM contributor_emails e
          WHERE e.contributor_id = cs.author_id AND e.email = sqlc.narg(author_email)))))
ORDER BY cs.authored_at DESC, cs.repo_id, cs.sha;

-- name: UpsertRepositorySignals :exec
-- Enregistre le contexte de diagnostic du depot (remplace le precedent).
INSERT INTO repository_signals (repo_id, agent_config_since, computed_at)
VALUES (sqlc.arg(repo_id), sqlc.narg(agent_config_since), sqlc.arg(computed_at))
ON CONFLICT (repo_id) DO UPDATE SET
    agent_config_since = excluded.agent_config_since,
    computed_at = excluded.computed_at;

-- name: ListAgentConfigContexts :many
-- Depots ou un fichier de configuration d'agent a ete trouve (repo_id NULL = tous), par date.
SELECT rs.repo_id, CAST(rs.agent_config_since AS TEXT) AS agent_config_since,
       r.owner AS repo_owner, r.name AS repo_name, r.label AS repo_label
FROM repository_signals rs
JOIN repositories r ON r.id = rs.repo_id
WHERE rs.agent_config_since IS NOT NULL
  AND (sqlc.narg(repo_id) IS NULL OR rs.repo_id = sqlc.narg(repo_id))
ORDER BY rs.agent_config_since, rs.repo_id;

-- name: PurgePullRequestTools :execrows
-- Efface les outils declares des pull requests ouvertes avant before (tous les depots).
UPDATE pull_requests SET tool_ids = ''
WHERE tool_ids <> '' AND created_at < sqlc.arg(before);
