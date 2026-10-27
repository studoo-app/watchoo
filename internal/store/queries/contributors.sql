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
INSERT INTO contributors (provider, provider_user_id, provider_login, name, avatar_url)
VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: UpdateContributorProfile :exec
-- Met a jour le login, le nom et l'avatar d'un contributeur.
UPDATE contributors
SET provider_login = ?, name = ?, avatar_url = ?
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
