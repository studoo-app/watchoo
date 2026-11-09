-- Observations des tags par la synchro (phase 8.4, voir documentation/fr/13-conventions.md) : SV3 (tag
-- reecrit ou supprime) et echeances de SV2 (pose du tag du jalon comparee a son echeance). Une ligne par
-- periode d'observation d'un couple (nom du tag, SHA du commit vise) : ouverte (ended_at NULL) tant que le
-- couple est vu par une lecture complete des tags, fermee quand le nom designe un autre commit
-- (end_reason 'rewritten') ou disparait (end_reason 'deleted'). Un couple qui reapparait ouvre une
-- nouvelle ligne : l'historique des periodes reste.
--
-- Alimentee seulement si conventions.enabled, et seulement par une lecture des tags reussie et non
-- tronquee (sinon rien n'est ferme). first_seen_at : premiere lecture qui a vu le couple ; last_seen_at :
-- derniere ; ended_at : lecture qui a constate la fin. Aucune attribution a une personne. Supprimer le
-- depot supprime ses lignes.

-- +goose Up
CREATE TABLE tag_observations (
    id            INTEGER PRIMARY KEY,
    repo_id       INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    name          TEXT    NOT NULL CHECK (name <> ''),
    sha           TEXT    NOT NULL CHECK (length(sha) IN (40, 64) AND sha NOT GLOB '*[^0-9a-f]*'),
    first_seen_at TEXT    NOT NULL
        CHECK (first_seen_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    last_seen_at  TEXT    NOT NULL
        CHECK (last_seen_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    ended_at      TEXT
        CHECK (ended_at IS NULL
            OR ended_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    end_reason    TEXT CHECK (end_reason IS NULL OR end_reason IN ('rewritten', 'deleted')),
    CHECK ((ended_at IS NULL) = (end_reason IS NULL)),
    CHECK (last_seen_at >= first_seen_at),
    CHECK (ended_at IS NULL OR ended_at >= last_seen_at)
);

-- Au plus une periode ouverte par nom de tag et par depot.
CREATE UNIQUE INDEX tag_observations_open ON tag_observations (repo_id, name) WHERE ended_at IS NULL;

-- Historique d'un depot, par nom puis premiere observation.
CREATE INDEX tag_observations_repo_id_name ON tag_observations (repo_id, name, first_seen_at);

-- +goose Down
DROP TABLE tag_observations;
