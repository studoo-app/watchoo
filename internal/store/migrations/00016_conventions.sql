-- Conventions de nommage (phase 8.1, voir documentation/fr/13-conventions.md).
--
-- commits.parent_count : nombre de parents du commit (0 = racine, 1 = ordinaire, 2 ou plus = fusion),
-- ecrit a l'insertion ; NULL = inconnu (commits anterieurs a la migration), rattrape par le clone Git
-- local a la synchro. L'index partiel sert a retrouver les commits inconnus d'un depot.
--
-- repositories.conventions_since : date de debut des conventions propre au depot, au format fixe
-- (minuit UTC de la date du calendrier) ; NULL = date de l'espace (conventions.since).

-- +goose Up
ALTER TABLE commits ADD COLUMN parent_count INTEGER CHECK (parent_count IS NULL OR parent_count >= 0);
CREATE INDEX commits_parent_count_unknown ON commits (repo_id, sha) WHERE parent_count IS NULL;
ALTER TABLE repositories ADD COLUMN conventions_since TEXT
    CHECK (conventions_since IS NULL
        OR conventions_since GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T00:00:00Z');

-- +goose Down
ALTER TABLE repositories DROP COLUMN conventions_since;
DROP INDEX commits_parent_count_unknown;
ALTER TABLE commits DROP COLUMN parent_count;
