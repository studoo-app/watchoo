-- Signaux declaratifs (phase 7.2, voir documentation/fr/12-supervision-diagnostic.md).
--
-- pull_requests.tool_ids : outils declares dans la description d'une PR (signal D2), encodes
-- « D2:claude », tries, separes par des virgules ; chaine vide = aucun ou non mesure. La description
-- elle-meme n'est jamais gardee. Remplie quand diagnostic.enabled est actif, videe par la purge.
--
-- repository_signals : contexte de diagnostic d'un depot. agent_config_since est la date (auteur) du
-- premier ajout d'un fichier de configuration d'agent (signal D4, affiche en contexte) ; NULL = aucun
-- fichier trouve. Une ligne par depot mesure, supprimee avec le depot.

-- +goose Up
ALTER TABLE pull_requests ADD COLUMN tool_ids TEXT NOT NULL DEFAULT '';
CREATE TABLE repository_signals (
    repo_id            INTEGER PRIMARY KEY REFERENCES repositories (id) ON DELETE CASCADE,
    agent_config_since TEXT
        CHECK (agent_config_since IS NULL
            OR agent_config_since GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    computed_at        TEXT    NOT NULL
        CHECK (computed_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z')
);

-- +goose Down
DROP TABLE repository_signals;
ALTER TABLE pull_requests DROP COLUMN tool_ids;
