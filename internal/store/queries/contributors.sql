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
-- d'adresses e-mail rattachees, jamais les adresses. sql.ErrNoRows si le contributeur n'existe pas.
SELECT ct.id, ct.provider, ct.provider_user_id, ct.provider_login, ct.name, ct.avatar_url, ct.profile_url,
       CAST((SELECT count(*) FROM contributor_emails e WHERE e.contributor_id = ct.id) AS INTEGER) AS emails,
       count(c.sha) AS commits,
       CAST(COALESCE(sum(c.additions), 0) AS INTEGER) AS additions,
       CAST(COALESCE(sum(c.deletions), 0) AS INTEGER) AS deletions,
       count(DISTINCT c.repo_id) AS repos,
       CAST(COALESCE(min(c.committed_at), '') AS TEXT) AS first_commit_at,
       CAST(COALESCE(max(c.committed_at), '') AS TEXT) AS last_commit_at
FROM contributors ct
LEFT JOIN commits c ON c.author_id = ct.id
WHERE ct.id = sqlc.arg(id)
GROUP BY ct.id;
