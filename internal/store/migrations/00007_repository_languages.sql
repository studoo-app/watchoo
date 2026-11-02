-- Langages d'un depot (1.1.0) : taille en octets de chaque langage sur la branche par defaut, lue dans
-- le clone Git local. La liste d'un depot est remplacee a chaque synchro qui a pu lire son clone ; les
-- pourcentages sont calcules a l'affichage.

-- +goose Up
CREATE TABLE repository_languages (
    repo_id  INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    language TEXT    NOT NULL CHECK (language <> ''),
    bytes    INTEGER NOT NULL CHECK (bytes > 0),
    PRIMARY KEY (repo_id, language)
);

-- +goose Down
DROP TABLE repository_languages;
