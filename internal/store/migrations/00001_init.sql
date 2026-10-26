-- Schéma initial du MVP : les 7 tables de documentation/04-modele-donnees.md.
--
-- Conventions :
--   - dates en TEXT au format fixe « 2006-01-02T15:04:05Z » (UTC, à la seconde), imposé par une
--     contrainte CHECK ; la conversion vers time.Time est faite dans internal/store (FormatTime / ParseTime) ;
--   - booléens en BOOLEAN (0 / 1) ;
--   - owner et name sont comparés sans tenir compte de la casse (COLLATE NOCASE), comme sur GitHub ;
--   - les clés étrangères sont actives grâce au pragma foreign_keys(1) passé dans le DSN.
--
-- Ce fichier sert aussi de schéma à sqlc : après toute modification, relancer « sqlc generate ».

-- +goose Up

-- Dépôts suivis.
CREATE TABLE repositories (
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
    UNIQUE (provider, provider_id),
    UNIQUE (provider, owner, name)
);

-- Branches des dépôts. Une branche supprimée sur la forge passe à tracked = 0, son historique reste.
CREATE TABLE branches (
    id       INTEGER PRIMARY KEY,
    repo_id  INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    head_sha TEXT    NOT NULL DEFAULT '',
    tracked  BOOLEAN NOT NULL DEFAULT 1 CHECK (tracked IN (0, 1)),
    UNIQUE (repo_id, name)
);

-- Personnes identifiées. provider et provider_user_id sont NULL ensemble pour une personne connue
-- seulement par son e-mail Git ; SQLite accepte plusieurs NULL dans une contrainte UNIQUE.
CREATE TABLE contributors (
    id               INTEGER PRIMARY KEY,
    provider         TEXT,
    provider_user_id INTEGER,
    provider_login   TEXT    NOT NULL DEFAULT '',
    name             TEXT    NOT NULL DEFAULT '',
    avatar_url       TEXT    NOT NULL DEFAULT '',
    UNIQUE (provider, provider_user_id),
    CHECK ((provider IS NULL) = (provider_user_id IS NULL))
);

-- Rattachement des e-mails Git aux contributeurs. Une adresse (en minuscules) désigne une seule personne.
CREATE TABLE contributor_emails (
    email          TEXT    PRIMARY KEY CHECK (email <> '' AND email = lower(email)),
    contributor_id INTEGER NOT NULL REFERENCES contributors (id) ON DELETE CASCADE
);

-- Historique des commits (table centrale) : un commit est enregistré une seule fois par dépôt.
-- Supprimer un contributeur ne supprime pas ses commits : author_id / committer_id passent à NULL.
CREATE TABLE commits (
    repo_id      INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    sha          TEXT    NOT NULL CHECK (sha <> ''),
    author_id    INTEGER REFERENCES contributors (id) ON DELETE SET NULL,
    committer_id INTEGER REFERENCES contributors (id) ON DELETE SET NULL,
    message      TEXT    NOT NULL DEFAULT '',
    authored_at  TEXT    NOT NULL
        CHECK (authored_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    committed_at TEXT    NOT NULL
        CHECK (committed_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    additions    INTEGER NOT NULL DEFAULT 0,
    deletions    INTEGER NOT NULL DEFAULT 0,
    url          TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (repo_id, sha)
);

-- Appartenance des commits aux branches suivies (un commit peut être vu sur plusieurs branches).
CREATE TABLE commit_branches (
    repo_id   INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    sha       TEXT    NOT NULL,
    branch_id INTEGER NOT NULL REFERENCES branches (id) ON DELETE CASCADE,
    PRIMARY KEY (repo_id, sha, branch_id),
    FOREIGN KEY (repo_id, sha) REFERENCES commits (repo_id, sha) ON DELETE CASCADE
);

-- Journal des synchronisations. status : 'running', 'success' ou 'error' (voir model.SyncStatus).
CREATE TABLE sync_runs (
    id          INTEGER PRIMARY KEY,
    repo_id     INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    started_at  TEXT    NOT NULL
        CHECK (started_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    finished_at TEXT
        CHECK (finished_at IS NULL
            OR finished_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    new_commits INTEGER NOT NULL DEFAULT 0,
    status      TEXT    NOT NULL,
    error       TEXT    NOT NULL DEFAULT ''
);

-- Index documentés (04-modele-donnees.md).
CREATE INDEX commits_repo_id_committed_at ON commits (repo_id, committed_at DESC);
CREATE INDEX commits_author_id_committed_at ON commits (author_id, committed_at DESC);
CREATE INDEX commit_branches_branch_id ON commit_branches (branch_id);

-- Index techniques : clés étrangères fréquemment parcourues.
CREATE INDEX contributor_emails_contributor_id ON contributor_emails (contributor_id);
CREATE INDEX sync_runs_repo_id_started_at ON sync_runs (repo_id, started_at DESC);

-- +goose Down
DROP INDEX sync_runs_repo_id_started_at;
DROP INDEX contributor_emails_contributor_id;
DROP INDEX commit_branches_branch_id;
DROP INDEX commits_author_id_committed_at;
DROP INDEX commits_repo_id_committed_at;
DROP TABLE sync_runs;
DROP TABLE commit_branches;
DROP TABLE commits;
DROP TABLE contributor_emails;
DROP TABLE contributors;
DROP TABLE branches;
DROP TABLE repositories;
