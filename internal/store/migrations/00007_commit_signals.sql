-- Signaux de diagnostic par commit (phase 7.1, voir documentation/fr/12-supervision-diagnostic.md) :
-- donnees mesurees a la synchro quand le reglage diagnostic.enabled est actif, pour les signaux des
-- phases suivantes. Une ligne par commit dont le detail des fichiers est connu (clone Git local).
--
-- Minimisation : ni message, ni e-mail, ni chemin de fichier. tool_ids ne garde que les identifiants
-- d'outils reconnus (internal/diagnostic), separes par des virgules, tries ; chaine vide = aucun.
-- author_id et authored_at sont recopies de commits a l'ecriture, pour les fenetres glissantes par
-- auteur de la phase 7.3 (index ci-dessous) et la purge par anciennete (reglage
-- diagnostic.retention_days). Supprimer le commit (ou le depot) supprime sa ligne.

-- +goose Up
CREATE TABLE commit_signals (
    repo_id        INTEGER NOT NULL,
    sha            TEXT    NOT NULL,
    author_id      INTEGER REFERENCES contributors (id) ON DELETE SET NULL,
    authored_at    TEXT    NOT NULL
        CHECK (authored_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    files_kept     INTEGER NOT NULL DEFAULT 0 CHECK (files_kept >= 0),
    files_excluded INTEGER NOT NULL DEFAULT 0 CHECK (files_excluded >= 0),
    additions_kept INTEGER NOT NULL DEFAULT 0 CHECK (additions_kept >= 0),
    deletions_kept INTEGER NOT NULL DEFAULT 0 CHECK (deletions_kept >= 0),
    tool_ids       TEXT    NOT NULL DEFAULT '',
    agent_config   BOOLEAN NOT NULL DEFAULT 0 CHECK (agent_config IN (0, 1)),
    bot_author     BOOLEAN NOT NULL DEFAULT 0 CHECK (bot_author IN (0, 1)),
    computed_at    TEXT    NOT NULL
        CHECK (computed_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    PRIMARY KEY (repo_id, sha),
    FOREIGN KEY (repo_id, sha) REFERENCES commits (repo_id, sha) ON DELETE CASCADE
);
CREATE INDEX commit_signals_author_date ON commit_signals (repo_id, author_id, authored_at);
CREATE INDEX commit_signals_date ON commit_signals (authored_at);

-- +goose Down
DROP INDEX commit_signals_date;
DROP INDEX commit_signals_author_date;
DROP TABLE commit_signals;
