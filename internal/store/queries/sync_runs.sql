-- name: CloseStaleSyncRuns :exec
-- Clot en erreur les synchros running du depot commencees avant stale_before (processus arrete).
UPDATE sync_runs
SET status = 'error', error = sqlc.arg(message), finished_at = sqlc.arg(finished_at)
WHERE repo_id = sqlc.arg(repo_id) AND status = 'running' AND started_at < sqlc.arg(stale_before);

-- name: CountRunningSyncRuns :one
-- Nombre de synchros running du depot (verrou).
SELECT count(*) FROM sync_runs
WHERE repo_id = ? AND status = 'running';

-- name: CreateSyncRun :one
-- Ouvre une synchro running du depot et renvoie la ligne creee.
INSERT INTO sync_runs (repo_id, started_at, status, new_commits)
VALUES (?, ?, 'running', 0)
RETURNING *;

-- name: FinishSyncRun :execrows
-- Clot une synchro encore running (0 ligne si elle n'existe pas ou est deja close).
UPDATE sync_runs
SET status = sqlc.arg(status), finished_at = sqlc.arg(finished_at), new_commits = sqlc.arg(new_commits),
    error = sqlc.arg(message)
WHERE id = sqlc.arg(id) AND status = 'running';

-- name: LastSyncRun :one
-- Derniere synchro du depot (started_at le plus recent, puis id le plus grand).
SELECT * FROM sync_runs
WHERE repo_id = ?
ORDER BY started_at DESC, id DESC
LIMIT 1;
