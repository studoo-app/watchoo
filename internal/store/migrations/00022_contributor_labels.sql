-- Noms donnes aux contributeurs (1.8.0, voir documentation/fr/04-modele-donnees.md) : le superviseur
-- donne a un contributeur un nom de son choix dans l'interface web (/settings/contributors), affiche
-- partout a la place du pseudo des commits. Chaine vide = aucun. La synchro n'ecrit jamais cette
-- colonne (UpdateContributorProfile ne la liste pas) ; le partage de configuration ne l'exporte pas.

-- +goose Up
ALTER TABLE contributors ADD COLUMN label TEXT NOT NULL DEFAULT '' CHECK (length(label) <= 100);

-- +goose Down
ALTER TABLE contributors DROP COLUMN label;
