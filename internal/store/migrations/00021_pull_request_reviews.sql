-- Relectures des pull requests (phase 8.6, voir documentation/fr/13-conventions.md) : PR4. Seuls des
-- compteurs sont gardes : jamais le login d'un relecteur, ni le corps d'une relecture.
--
-- pull_requests.reviews_by_others : relectures deposees par d'autres comptes que l'auteur, ni bots ni
-- exclus (conventions.excluded_accounts), tous etats sauf PENDING.
-- pull_requests.approvals_by_others : relecteurs distincts de ces comptes dont le dernier etat parmi
-- APPROVED, CHANGES_REQUESTED et DISMISSED, depose au plus tard a merged_at, est APPROVED.
-- pull_requests.approvals_by_contributors : parmi eux, ceux qui sont auteurs d'au moins un commit du
-- depot (connus de la base au moment de la lecture).
-- pull_requests.reviews_checked_at : date de la derniere lecture des relectures (format fixe).
-- NULL partout = relectures jamais lues. conventions.pr.min_reviews et reviewer_must_contribute sont
-- appliques a la lecture, depuis ces compteurs.

-- +goose Up
ALTER TABLE pull_requests ADD COLUMN reviews_by_others INTEGER
    CHECK (reviews_by_others IS NULL OR reviews_by_others >= 0);
ALTER TABLE pull_requests ADD COLUMN approvals_by_others INTEGER
    CHECK (approvals_by_others IS NULL OR approvals_by_others >= 0);
ALTER TABLE pull_requests ADD COLUMN approvals_by_contributors INTEGER
    CHECK (approvals_by_contributors IS NULL
        OR (approvals_by_contributors >= 0 AND approvals_by_contributors <= approvals_by_others));
ALTER TABLE pull_requests ADD COLUMN reviews_checked_at TEXT
    CHECK (reviews_checked_at IS NULL
        OR reviews_checked_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z');

-- +goose Down
ALTER TABLE pull_requests DROP COLUMN reviews_checked_at;
ALTER TABLE pull_requests DROP COLUMN approvals_by_contributors;
ALTER TABLE pull_requests DROP COLUMN approvals_by_others;
ALTER TABLE pull_requests DROP COLUMN reviews_by_others;
