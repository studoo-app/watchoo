-- Avertissements d'une synchro (1.3.0) : tableau JSON de {"kind": ..., "message": ...}, '[]' si aucun.
-- kind est un code stable (model.SyncWarningKind), traduit a l'affichage ; message est le texte complet,
-- dans la langue active lors de la synchro, sans donnee personnelle. Ils survivent ainsi a un redemarrage
-- de watchoo serve ; seul le dernier sync_run d'un depot est affiche.

-- +goose Up
ALTER TABLE sync_runs ADD COLUMN warnings TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(warnings) AND json_type(warnings) = 'array');

-- +goose Down
ALTER TABLE sync_runs DROP COLUMN warnings;
