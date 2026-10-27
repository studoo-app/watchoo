-- Pull requests, releases et tags (V2) : les tables pull_requests, releases et tags de
-- documentation/04-modele-donnees.md (le modèle passe de 7 à 10 tables).
--
-- Conventions de 00001_init.sql : dates en TEXT au format fixe « 2006-01-02T15:04:05Z » (contrainte
-- CHECK), booléens en BOOLEAN (0 / 1), suppression en cascade avec le dépôt.
--
--   - pull_requests : une ligne par PR et par dépôt, mise à jour à chaque synchro qui la voit modifiée ;
--     l'auteur est gardé tel quel (login, identifiant du compte), sans lien vers contributors ;
--   - releases et tags : remplacés en entier à chaque synchro réussie de leur liste. Un tag n'a pas de
--     date : l'affichage reprend celle du commit visé s'il est en base (sans clé étrangère, le commit
--     pouvant être sur une branche non suivie).

-- +goose Up
CREATE TABLE pull_requests (
    repo_id            INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    number             INTEGER NOT NULL CHECK (number > 0),
    title              TEXT    NOT NULL DEFAULT '',
    state              TEXT    NOT NULL CHECK (state IN ('open', 'closed')),
    merged             BOOLEAN NOT NULL DEFAULT 0 CHECK (merged IN (0, 1)),
    draft              BOOLEAN NOT NULL DEFAULT 0 CHECK (draft IN (0, 1)),
    author_login       TEXT    NOT NULL DEFAULT '',
    author_provider_id INTEGER NOT NULL DEFAULT 0,
    url                TEXT    NOT NULL DEFAULT '',
    base_branch        TEXT    NOT NULL DEFAULT '',
    head_branch        TEXT    NOT NULL DEFAULT '',
    created_at         TEXT    NOT NULL
        CHECK (created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    updated_at         TEXT    NOT NULL
        CHECK (updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    closed_at          TEXT
        CHECK (closed_at IS NULL
            OR closed_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    merged_at          TEXT
        CHECK (merged_at IS NULL
            OR merged_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    PRIMARY KEY (repo_id, number)
);

CREATE TABLE releases (
    repo_id      INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    provider_id  INTEGER NOT NULL CHECK (provider_id > 0),
    tag_name     TEXT    NOT NULL CHECK (tag_name <> ''),
    name         TEXT    NOT NULL DEFAULT '',
    draft        BOOLEAN NOT NULL DEFAULT 0 CHECK (draft IN (0, 1)),
    prerelease   BOOLEAN NOT NULL DEFAULT 0 CHECK (prerelease IN (0, 1)),
    author_login TEXT    NOT NULL DEFAULT '',
    url          TEXT    NOT NULL DEFAULT '',
    created_at   TEXT    NOT NULL
        CHECK (created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    published_at TEXT
        CHECK (published_at IS NULL
            OR published_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    PRIMARY KEY (repo_id, provider_id)
);

CREATE TABLE tags (
    repo_id INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    name    TEXT    NOT NULL CHECK (name <> ''),
    sha     TEXT    NOT NULL CHECK (sha <> ''),
    PRIMARY KEY (repo_id, name)
);

-- Point de reprise de la synchro des PR (max(updated_at) d'un dépôt) et liste des PR d'un dépôt.
CREATE INDEX pull_requests_repo_id_updated_at ON pull_requests (repo_id, updated_at DESC);

-- +goose Down
DROP INDEX pull_requests_repo_id_updated_at;
DROP TABLE tags;
DROP TABLE releases;
DROP TABLE pull_requests;
