-- Page de profil des contributeurs (0.5.0) : adresse web du compte de forge (html_url du compte GitHub),
-- donnee par le provider et affichee par l'interface web comme lien secondaire. Chaine vide = aucun
-- compte connu, ou adresse pas encore lue : un contributeur deja en base la recoit avec son prochain
-- commit lu par l'API (aucune adresse n'est devinee ici).

-- +goose Up
ALTER TABLE contributors ADD COLUMN profile_url TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE contributors DROP COLUMN profile_url;
