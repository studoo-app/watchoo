-- Langages des commits (1.2.0) : lignes ajoutees par langage dans chaque commit, lues dans le clone Git
-- local dans le meme passage que les lignes +/- ; une ligne par langage avec au moins une ligne ajoutee.
-- languages_version est la version des regles de langages (git.LanguageRulesVersion) avec laquelle le
-- commit a ete analyse ; 0 = pas encore. La synchro reprend les commits dont la version est inferieure a
-- la version courante, dans le clone, sans requete a la forge.

-- +goose Up
ALTER TABLE commits ADD COLUMN languages_version INTEGER NOT NULL DEFAULT 0 CHECK (languages_version >= 0);
CREATE INDEX commits_languages_version ON commits (repo_id, languages_version);
CREATE TABLE commit_languages (
    repo_id   INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    sha       TEXT    NOT NULL,
    language  TEXT    NOT NULL CHECK (language <> ''),
    additions INTEGER NOT NULL CHECK (additions > 0),
    PRIMARY KEY (repo_id, sha, language),
    FOREIGN KEY (repo_id, sha) REFERENCES commits (repo_id, sha) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE commit_languages;
DROP INDEX commits_languages_version;
ALTER TABLE commits DROP COLUMN languages_version;
