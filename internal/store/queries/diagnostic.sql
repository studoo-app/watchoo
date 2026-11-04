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
