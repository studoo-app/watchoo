-- Signatures d'outils d'IA declarees dans les commits (1.1.0) : une ligne par outil et par type de
-- signature ; aucune ligne = aucune signature, jamais une preuve de travail sans IA. ai_rules_version
-- est la version des regles (internal/aisignature) avec laquelle le commit a ete analyse ; 0 = pas encore
-- analyse. La synchro reprend les commits dont la version est inferieure a la version courante, sans
-- aucune requete a la forge : le message et l'auteur sont deja en base.

-- +goose Up
ALTER TABLE commits ADD COLUMN ai_rules_version INTEGER NOT NULL DEFAULT 0 CHECK (ai_rules_version >= 0);
CREATE INDEX commits_ai_rules_version ON commits (repo_id, ai_rules_version);
CREATE TABLE commit_ai_signatures (
    repo_id INTEGER NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    sha     TEXT    NOT NULL,
    tool    TEXT    NOT NULL CHECK (tool <> ''),
    kind    TEXT    NOT NULL CHECK (kind <> ''),
    PRIMARY KEY (repo_id, sha, tool, kind),
    FOREIGN KEY (repo_id, sha) REFERENCES commits (repo_id, sha) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE commit_ai_signatures;
DROP INDEX commits_ai_rules_version;
ALTER TABLE commits DROP COLUMN ai_rules_version;
