-- name: InsertCommitSignals :execrows
-- Enregistre les signaux d'un commit deja en base ; author_id et authored_at sont recopies du commit.
-- Une ligne deja presente n'est remplacee que si sa forme n'etait pas mesuree (form_known = 0, ligne
-- d'avant la phase 7.5) ; sinon 0 ligne. Un SHA absent de la base est ignore.
INSERT INTO commit_signals (
    repo_id, sha, author_id, authored_at, files_kept, files_excluded, additions_kept, deletions_kept,
    tool_ids, agent_config, bot_author, computed_at, form_known, source_lines, comment_lines, functions,
    documented_functions, identifiers, lexical_diversity, test_lines, message_length, message_bullets,
    message_conventional
)
SELECT c.repo_id, c.sha, c.author_id, c.authored_at,
    sqlc.arg(files_kept), sqlc.arg(files_excluded), sqlc.arg(additions_kept), sqlc.arg(deletions_kept),
    sqlc.arg(tool_ids), sqlc.arg(agent_config), sqlc.arg(bot_author), sqlc.arg(computed_at),
    sqlc.arg(form_known), sqlc.arg(source_lines), sqlc.arg(comment_lines), sqlc.arg(functions),
    sqlc.arg(documented_functions), sqlc.arg(identifiers), sqlc.narg(lexical_diversity),
    sqlc.arg(test_lines), sqlc.arg(message_length), sqlc.arg(message_bullets),
    sqlc.arg(message_conventional)
FROM commits c
WHERE c.repo_id = sqlc.arg(repo_id) AND c.sha = sqlc.arg(sha)
ON CONFLICT (repo_id, sha) DO UPDATE SET
    files_kept = excluded.files_kept,
    files_excluded = excluded.files_excluded,
    additions_kept = excluded.additions_kept,
    deletions_kept = excluded.deletions_kept,
    tool_ids = excluded.tool_ids,
    agent_config = excluded.agent_config,
    bot_author = excluded.bot_author,
    computed_at = excluded.computed_at,
    form_known = excluded.form_known,
    source_lines = excluded.source_lines,
    comment_lines = excluded.comment_lines,
    functions = excluded.functions,
    documented_functions = excluded.documented_functions,
    identifiers = excluded.identifiers,
    lexical_diversity = excluded.lexical_diversity,
    test_lines = excluded.test_lines,
    message_length = excluded.message_length,
    message_bullets = excluded.message_bullets,
    message_conventional = excluded.message_conventional
WHERE commit_signals.form_known = 0 AND excluded.form_known = 1;

-- name: PurgeCommitSignals :execrows
-- Supprime les signaux des commits ecrits avant before (tous les depots de l'espace).
DELETE FROM commit_signals
WHERE authored_at < sqlc.arg(before);

-- name: ListCommitSignals :many
-- Signaux des commits du depot, du plus recent au plus ancien (date d'ecriture, puis SHA) ; auteur du commit.
SELECT cs.repo_id, cs.sha, c.author_id, cs.authored_at, cs.files_kept, cs.files_excluded, cs.additions_kept,
    cs.deletions_kept, cs.tool_ids, cs.agent_config, cs.bot_author, cs.computed_at,
    cs.form_known, cs.source_lines, cs.comment_lines, cs.functions, cs.documented_functions,
    cs.identifiers, cs.lexical_diversity, cs.test_lines, cs.message_length, cs.message_bullets,
    cs.message_conventional
FROM commit_signals cs
JOIN commits c ON c.repo_id = cs.repo_id AND c.sha = cs.sha
WHERE cs.repo_id = sqlc.arg(repo_id)
ORDER BY cs.authored_at DESC, cs.sha;

-- name: ReassignSignalAuthors :exec
-- Reattribue a to_id les signaux dont from_id est l'auteur (fusion de contributeurs, comme les commits).
UPDATE commit_signals
SET author_id = sqlc.arg(to_id)
WHERE author_id = sqlc.arg(from_id);

-- name: CountCommitSignals :one
-- Nombre de commits du depot qui ont des signaux.
SELECT count(*) FROM commit_signals
WHERE repo_id = sqlc.arg(repo_id);

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

-- name: ListCommitSignalViews :many
-- Signaux de tous les commits mesures (repo_id NULL = tous les depots), avec de quoi les afficher, du
-- plus ancien au plus recent (date d'ecriture, puis SHA) : historique des signaux de rythme. L'auteur est
-- celui du commit (a jour apres une fusion de contributeurs).
SELECT cs.repo_id, cs.sha, c.author_id, cs.authored_at, cs.files_kept, cs.files_excluded,
       cs.additions_kept, cs.deletions_kept, cs.tool_ids, cs.agent_config, cs.bot_author, cs.computed_at,
       cs.form_known, cs.source_lines, cs.comment_lines, cs.functions, cs.documented_functions,
       cs.identifiers, cs.lexical_diversity, cs.test_lines, cs.message_length, cs.message_bullets,
       cs.message_conventional,
       c.url,
       r.owner AS repo_owner, r.name AS repo_name, r.label AS repo_label,
       CAST(COALESCE(a.name, '') AS TEXT) AS author_name,
       CAST(COALESCE(a.provider_login, '') AS TEXT) AS author_login
FROM commit_signals cs
JOIN commits c ON c.repo_id = cs.repo_id AND c.sha = cs.sha
JOIN repositories r ON r.id = cs.repo_id
LEFT JOIN contributors a ON a.id = c.author_id
WHERE (sqlc.narg(repo_id) IS NULL OR cs.repo_id = sqlc.narg(repo_id))
ORDER BY cs.authored_at, cs.sha;

-- name: MatchingContributorIDs :many
-- Contributeurs dont le login ou le nom vaut author (sans casse, ASCII), ou qui ont l'e-mail
-- author_email (deja en minuscules).
SELECT DISTINCT a.id
FROM contributors a
WHERE a.provider_login = sqlc.arg(author) COLLATE NOCASE
   OR a.name = sqlc.arg(author) COLLATE NOCASE
   OR EXISTS (SELECT 1 FROM contributor_emails e WHERE e.contributor_id = a.id AND e.email = sqlc.arg(author_email))
ORDER BY a.id;

-- name: ListCommitsWithoutSignals :many
-- Commits du depot sans signaux, ou dont la forme n'est pas mesuree (form_known = 0), ecrits a partir de since, par SHA croissant apres after_sha (pagination ;
-- chaine vide = depuis le debut), au plus row_limit : reprise de l'historique.
SELECT c.sha, c.message,
       CAST(COALESCE(a.name, '') AS TEXT) AS author_name,
       CAST(COALESCE(a.provider_login, '') AS TEXT) AS author_login
FROM commits c
LEFT JOIN contributors a ON a.id = c.author_id
WHERE c.repo_id = sqlc.arg(repo_id)
  AND c.authored_at >= sqlc.arg(since)
  AND c.sha > sqlc.arg(after_sha)
  AND NOT EXISTS (SELECT 1 FROM commit_signals cs
                  WHERE cs.repo_id = c.repo_id AND cs.sha = c.sha AND cs.form_known = 1)
ORDER BY c.sha
LIMIT sqlc.arg(row_limit);

-- name: UpsertDiagnosticReview :exec
-- Enregistre la revue d'une piste (remplace la precedente).
INSERT INTO diagnostic_reviews (repo_id, lead_key, kind, lead_date, signals, decision, cause, confidence,
                                note, reviewer, reviewed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (repo_id, lead_key) DO UPDATE SET
    kind = excluded.kind,
    lead_date = excluded.lead_date,
    signals = excluded.signals,
    decision = excluded.decision,
    cause = excluded.cause,
    confidence = excluded.confidence,
    note = excluded.note,
    reviewer = excluded.reviewer,
    reviewed_at = excluded.reviewed_at;

-- name: ListDiagnosticReviews :many
-- Revues des pistes (repo_id NULL = tous les depots).
SELECT repo_id, lead_key, kind, lead_date, signals, decision, cause, confidence, note, reviewer, reviewed_at
FROM diagnostic_reviews
WHERE (sqlc.narg(repo_id) IS NULL OR repo_id = sqlc.narg(repo_id))
ORDER BY reviewed_at DESC, repo_id, lead_key;

-- name: PurgeDiagnosticReviews :execrows
-- Supprime les revues des pistes datees d'avant before (tous les depots).
DELETE FROM diagnostic_reviews
WHERE lead_date < sqlc.arg(before);
