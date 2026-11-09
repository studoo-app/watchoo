-- Cache des criteres de fichiers des conventions (phase 8.3, voir documentation/fr/13-conventions.md) :
-- CHANGELOG, README, documentation et outillage, mesures par la synchro sur le clone Git local, pour la
-- tete de la branche par defaut (rev_kind 'head', tag_name vide) et pour chaque tag qui designe un jalon
-- configure (rev_kind 'tag', tag_name = nom du tag). Une ligne par revision mesuree, remplacee au
-- recalcul.
--
-- rev_sha : commit mesure ; rules_version : version des regles des criteres de fichiers ; config_hash :
-- empreinte des reglages (et des tags de version) qui influencent ces criteres. Une ligne d'un autre
-- commit, d'une autre version ou d'une autre empreinte est recalculee a la synchro suivante.
--
-- results : objet JSON d'identifiants de criteres, d'etats, de codes et de compteurs seulement ; jamais
-- de chemin, de contenu de fichier, de cellule d'INDEX ni de nom de personne. Supprimer le depot
-- supprime ses lignes.

-- +goose Up
CREATE TABLE repository_conventions (
    repo_id       INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    rev_kind      TEXT    NOT NULL CHECK (rev_kind IN ('head', 'tag')),
    tag_name      TEXT    NOT NULL DEFAULT '',
    rev_sha       TEXT    NOT NULL CHECK (length(rev_sha) IN (40, 64) AND rev_sha NOT GLOB '*[^0-9a-f]*'),
    rules_version INTEGER NOT NULL CHECK (rules_version >= 1),
    config_hash   TEXT    NOT NULL CHECK (config_hash <> ''),
    results       TEXT    NOT NULL CHECK (json_valid(results) AND json_type(results) = 'object'),
    computed_at   TEXT    NOT NULL
        CHECK (computed_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    CHECK ((rev_kind = 'head') = (tag_name = '')),
    PRIMARY KEY (repo_id, rev_kind, tag_name)
);

-- +goose Down
DROP TABLE repository_conventions;
