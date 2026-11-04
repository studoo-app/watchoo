-- Revues humaines des pistes de diagnostic (phase 7.4, voir documentation/fr/12-supervision-diagnostic.md,
-- section 6.2). Une revue par piste (lead_key, cle stable calculee par internal/diagnostic : commit,
-- pull request ou jour d'un auteur), remplacee si elle est refaite. signals garde les identifiants des
-- signaux presents lors de la revue (calibration, phase 7.6). note est une donnee personnelle : la
-- revue suit la duree de conservation des signaux, a partir de lead_date (date de la piste). Supprimer
-- le depot supprime ses revues.

-- +goose Up
CREATE TABLE diagnostic_reviews (
    repo_id     INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    lead_key    TEXT    NOT NULL CHECK (lead_key <> ''),
    kind        TEXT    NOT NULL CHECK (kind IN ('commit', 'pull_request', 'day')),
    lead_date   TEXT    NOT NULL
        CHECK (lead_date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    signals     TEXT    NOT NULL DEFAULT '',
    decision    TEXT    NOT NULL CHECK (decision IN ('assisted', 'not_assisted', 'undetermined')),
    cause       TEXT    NOT NULL DEFAULT ''
        CHECK (cause IN ('', 'internal_copy', 'tool_generated', 'existing_import', 'refactoring', 'other')),
    confidence  TEXT    NOT NULL CHECK (confidence IN ('low', 'medium', 'high')),
    note        TEXT    NOT NULL DEFAULT '',
    reviewer    TEXT    NOT NULL DEFAULT '',
    reviewed_at TEXT    NOT NULL
        CHECK (reviewed_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    PRIMARY KEY (repo_id, lead_key)
);
CREATE INDEX diagnostic_reviews_lead_date ON diagnostic_reviews (lead_date);

-- +goose Down
DROP INDEX diagnostic_reviews_lead_date;
DROP TABLE diagnostic_reviews;
