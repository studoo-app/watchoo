-- Groupes de contributeurs (1.9.0, voir documentation/fr/04-modele-donnees.md) : le superviseur
-- rattache les comptes d'une personne a un contributeur principal (un seul niveau, reversible).
--
-- contributors.primary_id : principal auquel ce contributeur est rattache ; NULL = principal lui-meme.
-- La regle « un seul niveau » est verifiee par le store dans la transaction du rattachement.
-- commits.person_id et commit_signals.person_id : principal du groupe de author_id (author_id lui-meme
-- s'il n'est pas rattache) ; NULL exactement quand author_id l'est. Copie denormalisee, tenue par le
-- store (rattachement, detachement, synchro, fusion), pour que les comptages par personne lisent un
-- index sans jointure.

-- +goose Up
ALTER TABLE contributors ADD COLUMN primary_id INTEGER REFERENCES contributors (id) ON DELETE SET NULL
    CHECK (primary_id IS NULL OR primary_id <> id);
ALTER TABLE commits ADD COLUMN person_id INTEGER REFERENCES contributors (id) ON DELETE SET NULL;
ALTER TABLE commit_signals ADD COLUMN person_id INTEGER REFERENCES contributors (id) ON DELETE SET NULL;
UPDATE commits SET person_id = author_id;
UPDATE commit_signals SET person_id = (SELECT c.person_id FROM commits c
                                       WHERE c.repo_id = commit_signals.repo_id AND c.sha = commit_signals.sha);
CREATE INDEX commits_person_id_committed_at ON commits (person_id, committed_at DESC);
DROP INDEX commit_signals_author_date;
CREATE INDEX commit_signals_person_date ON commit_signals (repo_id, person_id, authored_at);
CREATE INDEX contributors_primary_id ON contributors (primary_id);

-- +goose Down
DROP INDEX contributors_primary_id;
DROP INDEX commit_signals_person_date;
CREATE INDEX commit_signals_author_date ON commit_signals (repo_id, author_id, authored_at);
DROP INDEX commits_person_id_committed_at;
ALTER TABLE commit_signals DROP COLUMN person_id;
ALTER TABLE commits DROP COLUMN person_id;
ALTER TABLE contributors DROP COLUMN primary_id;
