-- Signaux absolus, marqueurs de travail et contre-indicateurs (phase 7.7, voir
-- documentation/fr/12-supervision-diagnostic.md, section 9.6). Nouvelles mesures de forme par commit,
-- calculees en memoire comme celles de la phase 7.5 : des nombres seulement, ni ligne, ni mot, ni chemin.
--
-- Les formes deja mesurees n'ont pas ces mesures : form_known repasse a 0 et la reprise de l'historique
-- (watchoo diagnostic backfill) les mesure de nouveau.

-- +goose Up
ALTER TABLE commit_signals ADD COLUMN decorative_lines INTEGER NOT NULL DEFAULT 0 CHECK (decorative_lines >= 0);
ALTER TABLE commit_signals ADD COLUMN fr_words INTEGER NOT NULL DEFAULT 0 CHECK (fr_words >= 0);
ALTER TABLE commit_signals ADD COLUMN fr_words_unaccented INTEGER NOT NULL DEFAULT 0 CHECK (fr_words_unaccented >= 0);
ALTER TABLE commit_signals ADD COLUMN wip_markers INTEGER NOT NULL DEFAULT 0 CHECK (wip_markers >= 0);
ALTER TABLE commit_signals ADD COLUMN short_identifiers INTEGER NOT NULL DEFAULT 0 CHECK (short_identifiers >= 0);
ALTER TABLE commit_signals ADD COLUMN private_defs INTEGER NOT NULL DEFAULT 0 CHECK (private_defs >= 0);
ALTER TABLE commit_signals ADD COLUMN message_revert BOOLEAN NOT NULL DEFAULT 0 CHECK (message_revert IN (0, 1));
ALTER TABLE commit_signals ADD COLUMN message_fix BOOLEAN NOT NULL DEFAULT 0 CHECK (message_fix IN (0, 1));
UPDATE commit_signals SET form_known = 0;

-- +goose Down
ALTER TABLE commit_signals DROP COLUMN message_fix;
ALTER TABLE commit_signals DROP COLUMN message_revert;
ALTER TABLE commit_signals DROP COLUMN private_defs;
ALTER TABLE commit_signals DROP COLUMN short_identifiers;
ALTER TABLE commit_signals DROP COLUMN wip_markers;
ALTER TABLE commit_signals DROP COLUMN fr_words_unaccented;
ALTER TABLE commit_signals DROP COLUMN fr_words;
ALTER TABLE commit_signals DROP COLUMN decorative_lines;
