-- name: DeleteRepositoryLanguages :exec
-- Retire tous les langages du depot (avant de les remplacer).
DELETE FROM repository_languages WHERE repo_id = ?;

-- name: InsertRepositoryLanguage :exec
-- Ajoute un langage du depot.
INSERT INTO repository_languages (repo_id, language, bytes)
VALUES (?, ?, ?);

-- name: ListRepositoryLanguages :many
-- Langages filtres par depot, avec le total du depot : depot apres depot, plus gros langage d'abord,
-- puis nom croissant.
SELECT l.repo_id, l.language, l.bytes,
       r.owner AS repo_owner, r.name AS repo_name, r.label AS repo_label,
       CAST(SUM(l.bytes) OVER (PARTITION BY l.repo_id) AS INTEGER) AS total_bytes
FROM repository_languages l
JOIN repositories r ON r.id = l.repo_id
WHERE (sqlc.narg(repo_id) IS NULL OR l.repo_id = sqlc.narg(repo_id))
ORDER BY r.owner COLLATE NOCASE, r.name COLLATE NOCASE, l.bytes DESC, l.language;
