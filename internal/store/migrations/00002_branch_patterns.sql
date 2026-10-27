-- Branches suivies par dépôt (phase 3) : motifs de « watchoo repo add --branch » et de la colonne
-- branches de « watchoo repo import ».
--
-- branch_patterns : noms et motifs (syntaxe de path.Match, ex. « release/* ») séparés par des
-- virgules, sans espace (ex. « main,dev,release/* ») ; chaîne vide = toutes les branches du dépôt.
-- La conversion vers []string (model.Repository.BranchPatterns) est faite dans internal/store.

-- +goose Up
ALTER TABLE repositories ADD COLUMN branch_patterns TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE repositories DROP COLUMN branch_patterns;
