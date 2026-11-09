-- Fusions et corps des pull requests, methodes de fusion du depot (phase 8.5, voir
-- documentation/fr/13-conventions.md) : PR3, PR5, PR6, BR1 sans les forks, BR2, BR5.
--
-- pull_requests.merge_commit_sha : commit cree par la fusion (squash, merge commit ou dernier commit
-- rebase), rempli seulement pour une PR fusionnee ; NULL = non fusionnee ou inconnu.
-- pull_requests.from_fork : 1 si le depot de tete differe du depot de base ou a ete supprime, 0 sinon ;
-- NULL = inconnu (PR lue avant la migration, ou reponse sans depot de base).
-- pull_requests.body_sections : masque de bits des sections reconnues dans le corps de la PR et du
-- mot-cle de fermeture (conventions.CheckPRBody), calcule en memoire a la synchro ; le corps n'est jamais
-- stocke. NULL = inconnu (PR lue avant la migration).
--
-- repositories.allow_squash_merge, allow_merge_commit, allow_rebase_merge : methodes de fusion
-- autorisees (1/0), lues dans GET /repos ; GitHub ne les renvoie qu'a un compte qui a le droit push.
-- NULL = inconnu (sans ce droit, ou en mode clone).
-- repositories.conventions_prs_version : version des regles de PR (conventions.PRRulesVersion) pour
-- laquelle la liste complete des PR du depot a ete relue une fois (remplissage des colonnes ci-dessus
-- pour les PR deja en base) ; NULL = jamais relue.

-- +goose Up
ALTER TABLE pull_requests ADD COLUMN merge_commit_sha TEXT
    CHECK (merge_commit_sha IS NULL
        OR (length(merge_commit_sha) IN (40, 64) AND merge_commit_sha NOT GLOB '*[^0-9a-f]*'));
ALTER TABLE pull_requests ADD COLUMN from_fork INTEGER CHECK (from_fork IS NULL OR from_fork IN (0, 1));
ALTER TABLE pull_requests ADD COLUMN body_sections INTEGER CHECK (body_sections IS NULL OR body_sections >= 0);
ALTER TABLE repositories ADD COLUMN allow_squash_merge INTEGER
    CHECK (allow_squash_merge IS NULL OR allow_squash_merge IN (0, 1));
ALTER TABLE repositories ADD COLUMN allow_merge_commit INTEGER
    CHECK (allow_merge_commit IS NULL OR allow_merge_commit IN (0, 1));
ALTER TABLE repositories ADD COLUMN allow_rebase_merge INTEGER
    CHECK (allow_rebase_merge IS NULL OR allow_rebase_merge IN (0, 1));
ALTER TABLE repositories ADD COLUMN conventions_prs_version INTEGER
    CHECK (conventions_prs_version IS NULL OR conventions_prs_version >= 1);

-- +goose Down
ALTER TABLE repositories DROP COLUMN conventions_prs_version;
ALTER TABLE repositories DROP COLUMN allow_rebase_merge;
ALTER TABLE repositories DROP COLUMN allow_merge_commit;
ALTER TABLE repositories DROP COLUMN allow_squash_merge;
ALTER TABLE pull_requests DROP COLUMN body_sections;
ALTER TABLE pull_requests DROP COLUMN from_fork;
ALTER TABLE pull_requests DROP COLUMN merge_commit_sha;
