-- name: GetContributorByAccount :one
-- Contributeur d'un compte de forge (sql.ErrNoRows s'il n'existe pas).
SELECT * FROM contributors
WHERE provider = ? AND provider_user_id = ?;

-- name: GetContributorByEmail :one
-- Contributeur auquel l'adresse (en minuscules) est rattachee (sql.ErrNoRows sinon).
SELECT c.* FROM contributors c
JOIN contributor_emails e ON e.contributor_id = c.id
WHERE e.email = ?;

-- name: CreateContributor :one
-- Ajoute un contributeur ; provider et provider_user_id sont NULL ensemble pour un contributeur sans compte.
INSERT INTO contributors (provider, provider_user_id, provider_login, name, avatar_url, profile_url)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: UpdateContributorProfile :exec
-- Met a jour le login, le nom, l'avatar et la page de profil d'un contributeur.
UPDATE contributors
SET provider_login = ?, name = ?, avatar_url = ?, profile_url = ?
WHERE id = ?;

-- name: AttachContributorEmail :exec
-- Rattache l'adresse au contributeur, en la retirant a son ancien titulaire s'il y en a un.
INSERT INTO contributor_emails (email, contributor_id)
VALUES (?, ?)
ON CONFLICT (email) DO UPDATE SET contributor_id = excluded.contributor_id;

-- name: MoveContributorEmails :exec
-- Deplace toutes les adresses de from_id vers to_id (fusion de contributeurs).
UPDATE contributor_emails
SET contributor_id = sqlc.arg(to_id)
WHERE contributor_id = sqlc.arg(from_id);

-- name: DeleteContributor :exec
-- Supprime un contributeur (apres fusion : ses commits et ses adresses ont ete reattribues).
DELETE FROM contributors
WHERE id = ?;

-- name: GetContributorProfile :one
-- Contributeur et totaux de ses commits d'AUTEUR (zeros et chaines vides s'il n'en a aucun) ; le nombre
-- d'adresses e-mail rattachees, jamais les adresses. Depuis la 1.9.0, totaux et adresses de tout son
-- groupe (person_id). sql.ErrNoRows si le contributeur n'existe pas.
SELECT ct.id, ct.provider, ct.provider_user_id, ct.provider_login, ct.name, ct.label, ct.avatar_url, ct.profile_url,
       CAST((SELECT count(*) FROM contributor_emails e
             WHERE e.contributor_id IN (SELECT g.id FROM contributors g WHERE g.id = ct.id OR g.primary_id = ct.id))
            AS INTEGER) AS emails,
       count(c.sha) AS commits,
       CAST(COALESCE(sum(c.additions), 0) AS INTEGER) AS additions,
       CAST(COALESCE(sum(c.deletions), 0) AS INTEGER) AS deletions,
       CAST(COALESCE(sum(c.stats_known), 0) AS INTEGER) AS known_commits,
       CAST(COALESCE(sum(c.ai_rules_version > 0), 0) AS INTEGER) AS ai_scanned_commits,
       CAST(COALESCE(sum(c.languages_version > 0), 0) AS INTEGER) AS languages_scanned_commits,
       CAST(COALESCE(sum(EXISTS (SELECT 1 FROM commit_ai_signatures s
                                 WHERE s.repo_id = c.repo_id AND s.sha = c.sha)), 0) AS INTEGER) AS ai_signed_commits,
       count(DISTINCT c.repo_id) AS repos,
       CAST(COALESCE(min(c.committed_at), '') AS TEXT) AS first_commit_at,
       CAST(COALESCE(max(c.committed_at), '') AS TEXT) AS last_commit_at
FROM contributors ct
LEFT JOIN commits c ON c.person_id = ct.id
WHERE ct.id = sqlc.arg(id)
GROUP BY ct.id;

-- name: ListContributorsWithoutProfileURL :many
-- Contributeurs d'une forge dont le login est connu mais la page de profil vide (connus avant la 0.5.0).
SELECT id, provider_login FROM contributors
WHERE provider = ? AND provider_login <> '' AND profile_url = ''
ORDER BY id;

-- name: SetContributorProfileURL :execrows
-- Renseigne la page de profil d'un contributeur, seulement si elle est encore vide.
UPDATE contributors
SET profile_url = ?
WHERE id = ? AND profile_url = '';

-- name: ListRepoCommitAuthorAccounts :many
-- Comptes de forge des auteurs d'au moins un commit du depot (phase 8.6, conventions.pr.reviewer_must_contribute) :
-- identifiant (0 si inconnu, pour un contributeur d'une autre forge ou sans compte) et login en minuscules
-- (vide si inconnu). Lus en memoire seulement, jamais journalises.
SELECT DISTINCT
       CAST(CASE WHEN c.provider = r.provider THEN COALESCE(c.provider_user_id, 0) ELSE 0 END AS INTEGER) AS provider_user_id,
       CAST(lower(c.provider_login) AS TEXT) AS login
FROM commits m
JOIN repositories r ON r.id = m.repo_id
JOIN contributors c ON c.id = m.author_id
WHERE m.repo_id = ?;

-- name: SetContributorLabel :execrows
-- Nom donne a un contributeur par le superviseur (1.8.0 ; vide = aucun). 0 ligne : contributeur inconnu.
UPDATE contributors
SET label = sqlc.arg(label)
WHERE id = sqlc.arg(id);

-- name: KeepContributorLabel :exec
-- Fusion de contributeurs : to_id reprend le nom donne de from_id s'il n'en a pas (1.8.0).
UPDATE contributors
SET label = (SELECT f.label FROM contributors AS f WHERE f.id = sqlc.arg(from_id))
WHERE contributors.id = sqlc.arg(to_id) AND contributors.label = ''
  AND EXISTS (SELECT 1 FROM contributors AS g WHERE g.id = sqlc.arg(from_id) AND g.label <> '');

-- name: ListContributorsForLabels :many
-- Contributeurs auteurs d'au moins un commit (page /settings/contributors, 1.8.0) : identite, nom donne,
-- nombre d'adresses e-mail rattachees (jamais les adresses) et nombre de commits.
SELECT ct.id, ct.provider, ct.provider_user_id, ct.provider_login, ct.name, ct.label, ct.avatar_url, ct.profile_url,
       ct.primary_id,
       CAST((SELECT count(*) FROM contributor_emails e WHERE e.contributor_id = ct.id) AS INTEGER) AS emails,
       count(*) AS commits
FROM contributors ct
JOIN commits c ON c.author_id = ct.id
GROUP BY ct.id
ORDER BY ct.id;

-- name: ListContributorLabels :many
-- Noms affiches qui remplacent le nom des commits dans les rapports du diagnostic et des conventions :
-- le nom donne (1.8.0) ; pour un principal de groupe sans nom donne (1.9.0), son nom, sinon son login
-- (ses lignes viennent aussi des commits de ses comptes rattaches).
SELECT ct.id,
       CAST(CASE WHEN ct.label <> '' THEN ct.label
                 WHEN ct.name <> '' THEN ct.name
                 ELSE ct.provider_login END AS TEXT) AS label
FROM contributors ct
WHERE ct.label <> '' OR EXISTS (SELECT 1 FROM contributors a WHERE a.primary_id = ct.id)
ORDER BY ct.id;

-- name: GetContributorGroup :one
-- Groupe d'un contributeur (1.9.0) : son principal (NULL s'il est principal) et le nombre de comptes qui
-- lui sont rattaches. sql.ErrNoRows si le contributeur n'existe pas.
SELECT ct.id, ct.primary_id,
       CAST((SELECT count(*) FROM contributors a WHERE a.primary_id = ct.id) AS INTEGER) AS attached
FROM contributors ct
WHERE ct.id = sqlc.arg(id);

-- name: SetContributorPrimary :exec
-- Rattache (primary_id) ou detache (NULL) un contributeur (1.9.0). Les regles sont verifiees par le store.
UPDATE contributors
SET primary_id = sqlc.narg(primary_id)
WHERE id = sqlc.arg(id);

-- name: MoveAttachedAccounts :exec
-- Fusion de contributeurs (1.9.0) : les comptes rattaches a from_id sont rattaches a to_id (sauf to_id
-- lui-meme, qui devient alors principal par ClearContributorPrimary).
UPDATE contributors
SET primary_id = sqlc.arg(to_id)
WHERE primary_id = sqlc.arg(from_id) AND id <> sqlc.arg(to_id);

-- name: RefreshCommitPersons :exec
-- Recalcule commits.person_id (principal du groupe de l'auteur) pour les commits des contributeurs des
-- groupes de a et b (1.9.0 : rattachement, detachement, fusion).
UPDATE commits
SET person_id = (SELECT COALESCE(ct.primary_id, ct.id) FROM contributors ct WHERE ct.id = commits.author_id)
WHERE author_id IN (SELECT g.id FROM contributors g
                    WHERE g.id IN (sqlc.arg(a), sqlc.arg(b)) OR g.primary_id IN (sqlc.arg(a), sqlc.arg(b)));

-- name: RefreshSignalPersons :exec
-- Recopie commits.person_id dans commit_signals pour les memes auteurs que RefreshCommitPersons.
UPDATE commit_signals
SET person_id = (SELECT c.person_id FROM commits c WHERE c.repo_id = commit_signals.repo_id AND c.sha = commit_signals.sha)
WHERE author_id IN (SELECT g.id FROM contributors g
                    WHERE g.id IN (sqlc.arg(a), sqlc.arg(b)) OR g.primary_id IN (sqlc.arg(a), sqlc.arg(b)));

-- name: FillCommitPersons :exec
-- Synchro (1.9.0) : person_id des commits du depot qui n'en ont pas encore (nouveaux commits), d'apres le
-- groupe de leur auteur.
UPDATE commits
SET person_id = (SELECT COALESCE(ct.primary_id, ct.id) FROM contributors ct WHERE ct.id = commits.author_id)
WHERE repo_id = sqlc.arg(repo_id) AND person_id IS NULL AND author_id IS NOT NULL;

-- name: ListAttachedAccounts :many
-- Comptes rattaches a un principal (1.9.0), avec leur nombre de commits d'auteur ; jamais d'adresse.
SELECT ct.id, ct.provider, ct.provider_user_id, ct.provider_login, ct.name, ct.label, ct.avatar_url, ct.profile_url,
       ct.primary_id,
       CAST((SELECT count(*) FROM commits c WHERE c.author_id = ct.id) AS INTEGER) AS commits
FROM contributors ct
WHERE ct.primary_id IS NOT NULL
ORDER BY ct.primary_id, ct.id;
