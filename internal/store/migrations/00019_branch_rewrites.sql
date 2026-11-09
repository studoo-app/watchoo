-- Reecritures observees de la branche par defaut (phase 8.4, BR4, voir
-- documentation/fr/13-conventions.md) : a la synchro, l'ancienne tete connue de la branche par defaut
-- n'etait plus ancetre de la nouvelle (historique reecrit). Une ligne par evenement observe.
--
-- Alimentee seulement si conventions.enabled ; un ancien commit absent du clone donne un resultat inconnu,
-- jamais enregistre, sauf en mode API ou l'erreur ErrUnknownBase de la synchro sur l'ancienne tete sert de
-- signal (voir 13-conventions.md, section 5.2). branch : nom de la branche par defaut au moment de l'observation ; old_sha et new_sha :
-- ancienne et nouvelle tete ; observed_at : fin de la synchro qui l'a observee. Aucune attribution a une
-- personne (jamais "a pousse"). Supprimer le depot supprime ses lignes.

-- +goose Up
CREATE TABLE branch_rewrites (
    id          INTEGER PRIMARY KEY,
    repo_id     INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    branch      TEXT    NOT NULL CHECK (branch <> ''),
    old_sha     TEXT    NOT NULL CHECK (length(old_sha) IN (40, 64) AND old_sha NOT GLOB '*[^0-9a-f]*'),
    new_sha     TEXT    NOT NULL CHECK (length(new_sha) IN (40, 64) AND new_sha NOT GLOB '*[^0-9a-f]*'),
    observed_at TEXT    NOT NULL
        CHECK (observed_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    CHECK (old_sha <> new_sha)
);

-- Evenements d'un depot par date.
CREATE INDEX branch_rewrites_repo_id_observed_at ON branch_rewrites (repo_id, observed_at);

-- +goose Down
DROP TABLE branch_rewrites;
