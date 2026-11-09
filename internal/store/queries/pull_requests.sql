-- name: UpsertPullRequest :exec
-- Cree la pull request (repo_id, number) ou la met a jour en entier. merge_commit_sha est remplace
-- (NULL : non fusionnee) ; from_fork et body_sections inconnus (NULL) gardent la valeur connue.
INSERT INTO pull_requests (repo_id, number, title, state, merged, draft, author_login, author_provider_id,
                           url, base_branch, head_branch, created_at, updated_at, closed_at, merged_at, tool_ids,
                           merge_commit_sha, from_fork, body_sections)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (repo_id, number) DO UPDATE SET
    title = excluded.title,
    state = excluded.state,
    merged = excluded.merged,
    draft = excluded.draft,
    author_login = excluded.author_login,
    author_provider_id = excluded.author_provider_id,
    url = excluded.url,
    base_branch = excluded.base_branch,
    head_branch = excluded.head_branch,
    created_at = excluded.created_at,
    updated_at = excluded.updated_at,
    closed_at = excluded.closed_at,
    merged_at = excluded.merged_at,
    tool_ids = excluded.tool_ids,
    merge_commit_sha = excluded.merge_commit_sha,
    from_fork = COALESCE(excluded.from_fork, pull_requests.from_fork),
    body_sections = COALESCE(excluded.body_sections, pull_requests.body_sections);

-- name: SetPullRequestConventions :execrows
-- Relecture complete des PR (phase 8.5) : ecrit le commit de fusion, l'origine (fork) et le masque des
-- sections du corps d'une PR deja en base, sans toucher aux autres colonnes (tool_ids compris) ; from_fork
-- et body_sections inconnus (NULL) gardent la valeur connue. 0 ligne : PR absente.
UPDATE pull_requests
SET merge_commit_sha = sqlc.narg(merge_commit_sha),
    from_fork = COALESCE(sqlc.narg(from_fork), from_fork),
    body_sections = COALESCE(sqlc.narg(body_sections), body_sections)
WHERE repo_id = sqlc.arg(repo_id) AND number = sqlc.arg(number);

-- name: SetPullRequestReviews :execrows
-- Relectures d'une PR deja en base (phase 8.6, PR4) : compteurs seulement, jamais de login de relecteur
-- ni de corps ; reviews_checked_at = date de la lecture. 0 ligne : PR absente.
UPDATE pull_requests
SET reviews_by_others = sqlc.arg(reviews_by_others),
    approvals_by_others = sqlc.arg(approvals_by_others),
    approvals_by_contributors = sqlc.arg(approvals_by_contributors),
    reviews_checked_at = sqlc.arg(reviews_checked_at)
WHERE repo_id = sqlc.arg(repo_id) AND number = sqlc.arg(number);

-- name: ListPullRequestsToReview :many
-- PR dont les relectures sont a lire (phase 8.6) : fusionnees jamais relues depuis leur fusion
-- (reviews_checked_at NULL ou anterieur a merged_at : une seule lecture apres la fusion suffit), puis
-- ouvertes hors brouillons modifiees depuis la derniere lecture ; jamais les PR issues d'un fork
-- (from_fork = 1). Fusionnees d'abord (fusion la plus recente d'abord), puis ouvertes (modification la
-- plus recente d'abord). L'appelant ecarte les auteurs bots, les PR ouvertes avant la date de debut des
-- conventions (created_at) et applique le plafond.
SELECT p.number, p.merged, p.merged_at, p.created_at, p.updated_at, p.author_login, p.author_provider_id
FROM pull_requests p
WHERE p.repo_id = ?
  AND (p.from_fork IS NULL OR p.from_fork = 0)
  AND ((p.merged = 1 AND p.merged_at IS NOT NULL
        AND (p.reviews_checked_at IS NULL OR p.reviews_checked_at < p.merged_at))
    OR (p.merged = 0 AND p.state = 'open' AND p.draft = 0
        AND (p.reviews_checked_at IS NULL OR p.reviews_checked_at < p.updated_at)))
ORDER BY p.merged DESC, p.merged_at DESC, p.updated_at DESC, p.number DESC;

-- name: MaxPullRequestUpdatedAt :one
-- Date de modification la plus recente des pull requests du depot (chaine vide si aucune).
SELECT CAST(COALESCE(max(updated_at), '') AS TEXT) AS updated_at
FROM pull_requests
WHERE repo_id = ?;

-- name: ListPullRequests :many
-- Pull requests filtrees (depot, etat, merge, compte de l'auteur), de la plus recemment modifiee a la plus ancienne.
-- row_limit negatif = pas de limite.
SELECT p.repo_id, p.number, p.title, p.state, p.merged, p.draft, p.author_login, p.author_provider_id,
       p.url, p.base_branch, p.head_branch, p.created_at, p.updated_at, p.closed_at, p.merged_at, p.tool_ids,
       p.merge_commit_sha, p.from_fork, p.body_sections,
       p.reviews_by_others, p.approvals_by_others, p.approvals_by_contributors, p.reviews_checked_at,
       r.owner AS repo_owner, r.name AS repo_name, r.label AS repo_label
FROM pull_requests p
JOIN repositories r ON r.id = p.repo_id
WHERE (sqlc.narg(repo_id) IS NULL OR p.repo_id = sqlc.narg(repo_id))
  AND (sqlc.narg(state) IS NULL OR p.state = sqlc.narg(state))
  AND (sqlc.narg(merged) IS NULL OR p.merged = sqlc.narg(merged))
  AND (sqlc.narg(author_provider_id) IS NULL OR p.author_provider_id = sqlc.narg(author_provider_id))
ORDER BY p.updated_at DESC, p.repo_id, p.number DESC
LIMIT sqlc.arg(row_limit);

-- name: PullRequestCounts :one
-- Pull requests du depot : ouvertes, mergees, fermees sans merge.
SELECT CAST(COALESCE(sum(state = 'open'), 0) AS INTEGER) AS open_count,
       CAST(COALESCE(sum(merged = 1), 0) AS INTEGER) AS merged_count,
       CAST(COALESCE(sum(state = 'closed' AND merged = 0), 0) AS INTEGER) AS closed_count
FROM pull_requests
WHERE repo_id = ?;

-- name: CountOpenPullRequestsByRepo :many
-- Nombre de pull requests ouvertes par depot (repo_id NULL = tous les depots).
SELECT repo_id, count(*) AS open_pulls
FROM pull_requests
WHERE state = 'open' AND (sqlc.narg(repo_id) IS NULL OR repo_id = sqlc.narg(repo_id))
GROUP BY repo_id;

-- name: DeleteReleases :exec
-- Retire toutes les releases du depot (avant de les remplacer).
DELETE FROM releases WHERE repo_id = ?;

-- name: InsertRelease :exec
-- Ajoute une release (une release deja presente avec le meme identifiant est ignoree).
INSERT INTO releases (repo_id, provider_id, tag_name, name, draft, prerelease, author_login, url,
                      created_at, published_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (repo_id, provider_id) DO NOTHING;

-- name: ListReleases :many
-- Releases filtrees par depot, de la plus recente (publication, sinon creation) a la plus ancienne.
-- row_limit negatif = pas de limite.
SELECT rl.repo_id, rl.provider_id, rl.tag_name, rl.name, rl.draft, rl.prerelease, rl.author_login, rl.url,
       rl.created_at, rl.published_at,
       r.owner AS repo_owner, r.name AS repo_name, r.label AS repo_label
FROM releases rl
JOIN repositories r ON r.id = rl.repo_id
WHERE (sqlc.narg(repo_id) IS NULL OR rl.repo_id = sqlc.narg(repo_id))
ORDER BY COALESCE(rl.published_at, rl.created_at) DESC, rl.repo_id, rl.provider_id DESC
LIMIT sqlc.arg(row_limit);

-- name: LatestReleaseByRepo :many
-- Release publiee (hors brouillons) la plus recente de chaque depot (egalite : identifiant le plus grand).
SELECT rl.repo_id, rl.provider_id, rl.tag_name, rl.name, rl.draft, rl.prerelease, rl.author_login, rl.url,
       rl.created_at, rl.published_at
FROM releases rl
WHERE rl.draft = 0
  AND (sqlc.narg(repo_id) IS NULL OR rl.repo_id = sqlc.narg(repo_id))
  AND NOT EXISTS (
        SELECT 1 FROM releases o
        WHERE o.repo_id = rl.repo_id AND o.draft = 0
          AND (COALESCE(o.published_at, o.created_at) > COALESCE(rl.published_at, rl.created_at)
            OR (COALESCE(o.published_at, o.created_at) = COALESCE(rl.published_at, rl.created_at)
                AND o.provider_id > rl.provider_id)));

-- name: DeleteTags :exec
-- Retire tous les tags du depot (avant de les remplacer).
DELETE FROM tags WHERE repo_id = ?;

-- name: InsertTag :exec
-- Ajoute un tag (un tag deja present avec le meme nom est ignore).
INSERT INTO tags (repo_id, name, sha)
VALUES (?, ?, ?)
ON CONFLICT (repo_id, name) DO NOTHING;

-- name: ListTags :many
-- Tags filtres par depot, avec la date et le lien du commit vise s'il est en base (chaines vides sinon) :
-- commit le plus recent d'abord, tags sans commit connu a la fin, puis nom decroissant.
-- row_limit negatif = pas de limite.
SELECT t.repo_id, t.name, t.sha,
       r.owner AS repo_owner, r.name AS repo_name, r.label AS repo_label,
       CAST(COALESCE(c.committed_at, '') AS TEXT) AS committed_at,
       CAST(COALESCE(c.url, '') AS TEXT) AS commit_url
FROM tags t
JOIN repositories r ON r.id = t.repo_id
LEFT JOIN commits c ON c.repo_id = t.repo_id AND c.sha = t.sha
WHERE (sqlc.narg(repo_id) IS NULL OR t.repo_id = sqlc.narg(repo_id))
ORDER BY c.committed_at IS NULL, c.committed_at DESC, t.name DESC
LIMIT sqlc.arg(row_limit);

-- name: ListConventionPullRequests :many
-- Pull requests du depot pour les conventions (phase 8.2), numero decroissant : titre (lu en memoire
-- seulement, jamais garde), etat, brouillon, auteur et branche de tete, commit de fusion, origine (fork)
-- et masque des sections du corps (phase 8.5) ; author_id = contributeur du
-- meme compte de forge (0 si aucun).
SELECT p.repo_id, p.number, p.title, p.state, p.merged, p.draft, p.author_login, p.author_provider_id,
       p.url, p.base_branch, p.head_branch, p.created_at, p.updated_at, p.closed_at, p.merged_at, p.tool_ids,
       p.merge_commit_sha, p.from_fork, p.body_sections,
       p.reviews_by_others, p.approvals_by_others, p.approvals_by_contributors, p.reviews_checked_at,
       CAST(COALESCE(c.id, 0) AS INTEGER) AS author_id,
       CAST(COALESCE(c.name, '') AS TEXT) AS author_name
FROM pull_requests p
JOIN repositories r ON r.id = p.repo_id
LEFT JOIN contributors c ON p.author_provider_id <> 0 AND c.provider = r.provider
                          AND c.provider_user_id = p.author_provider_id
WHERE p.repo_id = ?
ORDER BY p.number DESC;
