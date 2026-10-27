-- Dépôts Git locaux (V2) : repositories.provider_id devient facultatif. Un dépôt ajouté sans gh (clone
-- Git local) n'a pas encore d'identifiant de forge : NULL jusqu'à la première synchro par l'API
-- GitHub, qui le résout par owner/name et l'enregistre. SQLite accepte plusieurs NULL dans la
-- contrainte UNIQUE (provider, provider_id) ; 0 et les valeurs négatives sont refusés.
--
-- SQLite ne sait pas retirer un NOT NULL : la table est reconstruite selon la procédure documentée
-- (https://www.sqlite.org/lang_altertable.html#otheralter), hors transaction goose (NO TRANSACTION)
-- pour pouvoir couper les clés étrangères : sans cela, DROP TABLE repositories supprimerait en cascade
-- branches, commits, sync_runs, pull requests, releases et tags. Tout se fait sur une seule connexion
-- (goose exécute une migration NO TRANSACTION sur un *sql.Conn), dans une transaction explicite ; les
-- clés étrangères sont réactivées à la fin. Les tables filles gardent leur « REFERENCES repositories »,
-- qui désigne la nouvelle table après le renommage.

-- +goose NO TRANSACTION

-- +goose Up
PRAGMA foreign_keys = OFF;

BEGIN;

CREATE TABLE repositories_new (
    id                INTEGER PRIMARY KEY,
    provider          TEXT    NOT NULL,
    provider_id       INTEGER CHECK (provider_id IS NULL OR provider_id > 0),
    owner             TEXT    NOT NULL COLLATE NOCASE,
    name              TEXT    NOT NULL COLLATE NOCASE,
    label             TEXT    NOT NULL DEFAULT '',
    default_branch    TEXT    NOT NULL DEFAULT '',
    url               TEXT    NOT NULL DEFAULT '',
    stars             INTEGER NOT NULL DEFAULT 0,
    open_issues       INTEGER NOT NULL DEFAULT 0,
    last_synced_at    TEXT
        CHECK (last_synced_at IS NULL
            OR last_synced_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    last_commit_at    TEXT
        CHECK (last_commit_at IS NULL
            OR last_commit_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    archived          BOOLEAN NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    flemme_after_days INTEGER CHECK (flemme_after_days IS NULL OR flemme_after_days >= 1),
    dead_after_days   INTEGER CHECK (dead_after_days IS NULL OR dead_after_days >= 1),
    enabled           BOOLEAN NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    branch_patterns   TEXT    NOT NULL DEFAULT '',
    UNIQUE (provider, provider_id),
    UNIQUE (provider, owner, name)
);

INSERT INTO repositories_new (
    id, provider, provider_id, owner, name, label, default_branch, url, stars, open_issues,
    last_synced_at, last_commit_at, archived, flemme_after_days, dead_after_days, enabled, branch_patterns
)
SELECT
    id, provider, CASE WHEN provider_id > 0 THEN provider_id END, owner, name, label, default_branch, url,
    stars, open_issues, last_synced_at, last_commit_at, archived, flemme_after_days, dead_after_days,
    enabled, branch_patterns
FROM repositories;

DROP TABLE repositories;

ALTER TABLE repositories_new RENAME TO repositories;

COMMIT;

PRAGMA foreign_keys = ON;

-- +goose Down
-- Retour au NOT NULL : un dépôt sans identifiant de forge reçoit l'opposé de son identifiant interne
-- (négatif, donc unique et reconnaissable), que la première synchro par l'API remplacera.
PRAGMA foreign_keys = OFF;

BEGIN;

CREATE TABLE repositories_old (
    id                INTEGER PRIMARY KEY,
    provider          TEXT    NOT NULL,
    provider_id       INTEGER NOT NULL,
    owner             TEXT    NOT NULL COLLATE NOCASE,
    name              TEXT    NOT NULL COLLATE NOCASE,
    label             TEXT    NOT NULL DEFAULT '',
    default_branch    TEXT    NOT NULL DEFAULT '',
    url               TEXT    NOT NULL DEFAULT '',
    stars             INTEGER NOT NULL DEFAULT 0,
    open_issues       INTEGER NOT NULL DEFAULT 0,
    last_synced_at    TEXT
        CHECK (last_synced_at IS NULL
            OR last_synced_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    last_commit_at    TEXT
        CHECK (last_commit_at IS NULL
            OR last_commit_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    archived          BOOLEAN NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    flemme_after_days INTEGER CHECK (flemme_after_days IS NULL OR flemme_after_days >= 1),
    dead_after_days   INTEGER CHECK (dead_after_days IS NULL OR dead_after_days >= 1),
    enabled           BOOLEAN NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    branch_patterns   TEXT    NOT NULL DEFAULT '',
    UNIQUE (provider, provider_id),
    UNIQUE (provider, owner, name)
);

INSERT INTO repositories_old (
    id, provider, provider_id, owner, name, label, default_branch, url, stars, open_issues,
    last_synced_at, last_commit_at, archived, flemme_after_days, dead_after_days, enabled, branch_patterns
)
SELECT
    id, provider, COALESCE(provider_id, -id), owner, name, label, default_branch, url, stars, open_issues,
    last_synced_at, last_commit_at, archived, flemme_after_days, dead_after_days, enabled, branch_patterns
FROM repositories;

DROP TABLE repositories;

ALTER TABLE repositories_old RENAME TO repositories;

COMMIT;

PRAGMA foreign_keys = ON;
