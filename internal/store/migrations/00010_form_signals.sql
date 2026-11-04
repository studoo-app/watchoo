-- Mesures de forme par commit (phase 7.5, voir documentation/fr/12-supervision-diagnostic.md, section 9.5) :
-- signaux F1 a F5, calcules en memoire sur les lignes ajoutees du commit (clone Git local) et sur son
-- message. Seules les mesures sont gardees : ni ligne de code, ni identifiant, ni chemin, ni message.
--
-- form_known = 0 : mesures absentes (ligne ecrite avant la phase 7.5, ou lignes ajoutees illisibles) ;
-- les autres colonnes valent alors 0 et la reprise de l'historique (watchoo diagnostic backfill) les
-- mesure. lexical_diversity est NULL avec moins de 50 identifiants (MATTR, fenetre de 50).

-- +goose Up
ALTER TABLE commit_signals ADD COLUMN form_known BOOLEAN NOT NULL DEFAULT 0 CHECK (form_known IN (0, 1));
ALTER TABLE commit_signals ADD COLUMN source_lines INTEGER NOT NULL DEFAULT 0 CHECK (source_lines >= 0);
ALTER TABLE commit_signals ADD COLUMN comment_lines INTEGER NOT NULL DEFAULT 0 CHECK (comment_lines >= 0);
ALTER TABLE commit_signals ADD COLUMN functions INTEGER NOT NULL DEFAULT 0 CHECK (functions >= 0);
ALTER TABLE commit_signals ADD COLUMN documented_functions INTEGER NOT NULL DEFAULT 0
    CHECK (documented_functions >= 0);
ALTER TABLE commit_signals ADD COLUMN identifiers INTEGER NOT NULL DEFAULT 0 CHECK (identifiers >= 0);
ALTER TABLE commit_signals ADD COLUMN lexical_diversity REAL
    CHECK (lexical_diversity IS NULL OR (lexical_diversity >= 0 AND lexical_diversity <= 1));
ALTER TABLE commit_signals ADD COLUMN test_lines INTEGER NOT NULL DEFAULT 0 CHECK (test_lines >= 0);
ALTER TABLE commit_signals ADD COLUMN message_length INTEGER NOT NULL DEFAULT 0 CHECK (message_length >= 0);
ALTER TABLE commit_signals ADD COLUMN message_bullets INTEGER NOT NULL DEFAULT 0 CHECK (message_bullets >= 0);
ALTER TABLE commit_signals ADD COLUMN message_conventional BOOLEAN NOT NULL DEFAULT 0
    CHECK (message_conventional IN (0, 1));

-- +goose Down
ALTER TABLE commit_signals DROP COLUMN message_conventional;
ALTER TABLE commit_signals DROP COLUMN message_bullets;
ALTER TABLE commit_signals DROP COLUMN message_length;
ALTER TABLE commit_signals DROP COLUMN test_lines;
ALTER TABLE commit_signals DROP COLUMN lexical_diversity;
ALTER TABLE commit_signals DROP COLUMN identifiers;
ALTER TABLE commit_signals DROP COLUMN documented_functions;
ALTER TABLE commit_signals DROP COLUMN functions;
ALTER TABLE commit_signals DROP COLUMN comment_lines;
ALTER TABLE commit_signals DROP COLUMN source_lines;
ALTER TABLE commit_signals DROP COLUMN form_known;
