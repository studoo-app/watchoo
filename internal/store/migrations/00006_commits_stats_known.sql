-- Statistiques des commits connues ou non (0.5.0) : stats_known = 1 quand additions et deletions ont
-- ete calculees (clone Git local, ou API en repli), meme a 0 ; 0 tant qu'elles sont inconnues (affichees
-- par un tiret). Les commits deja en base dont additions ou deletions n'est pas nul ont ete lus par l'API :
-- marques connus. Les autres (0/0, impossibles a distinguer d'un commit jamais calcule) restent
-- inconnus et sont recalcules par le clone a la synchro suivante (sans cout). L'index partiel sert a
-- retrouver les commits inconnus d'un depot.

-- +goose Up
ALTER TABLE commits ADD COLUMN stats_known BOOLEAN NOT NULL DEFAULT 0 CHECK (stats_known IN (0, 1));
UPDATE commits SET stats_known = 1 WHERE additions <> 0 OR deletions <> 0;
CREATE INDEX commits_stats_unknown ON commits (repo_id, sha) WHERE stats_known = 0;

-- +goose Down
DROP INDEX commits_stats_unknown;
ALTER TABLE commits DROP COLUMN stats_known;
