package store

// Relectures des pull requests (phase 8.6, PR4 des conventions) : sélection des PR dont les relectures
// sont à lire, comptes des auteurs de commits du dépôt et écriture des compteurs. Jamais de login de
// relecteur ni de corps de relecture en base.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// ReviewCandidate est une pull request dont les relectures sont à lire.
type ReviewCandidate struct {
	Number           int
	Merged           bool
	MergedAt         time.Time // zéro si non fusionnée
	CreatedAt        time.Time
	UpdatedAt        time.Time
	AuthorLogin      string // login de l'auteur (bots écartés par l'appelant) ; jamais journalisé
	AuthorProviderID int64  // 0 si inconnu
}

// PullRequestsToReview renvoie les pull requests du dépôt repoID dont les relectures sont à lire : les
// fusionnées jamais relues depuis leur fusion (reviews_checked_at NULL ou antérieur à merged_at), fusion
// la plus récente d'abord, puis les ouvertes hors brouillons modifiées depuis la dernière lecture,
// modification la plus récente d'abord ; jamais les PR issues d'un fork. Liste vide non nil si aucune.
func (s *Store) PullRequestsToReview(ctx context.Context, repoID int64) ([]ReviewCandidate, error) {
	rows, err := s.queries.ListPullRequestsToReview(ctx, repoID)
	if err != nil {
		return nil, wrapErr(i18n.T("store.pulls.to_review", repoID), err)
	}
	out := make([]ReviewCandidate, 0, len(rows))
	for _, row := range rows {
		c := ReviewCandidate{Number: int(row.Number), Merged: row.Merged, AuthorLogin: row.AuthorLogin,
			AuthorProviderID: row.AuthorProviderID}
		var err error
		if c.CreatedAt, err = ParseTime(row.CreatedAt); err != nil {
			return nil, wrapErr(i18n.T("store.pulls.number", row.Number), err)
		}
		if c.UpdatedAt, err = ParseTime(row.UpdatedAt); err != nil {
			return nil, wrapErr(i18n.T("store.pulls.number", row.Number), err)
		}
		if c.MergedAt, err = parseNullTime(row.MergedAt); err != nil {
			return nil, wrapErr(i18n.T("store.pulls.number", row.Number), err)
		}
		out = append(out, c)
	}
	return out, nil
}

// RepoCommitAuthorAccounts renvoie les comptes de forge des auteurs d'au moins un commit du dépôt repoID
// (conventions.pr.reviewer_must_contribute) : identifiants connus (> 0) et logins connus, en minuscules.
// Lus en mémoire seulement : jamais journalisés.
func (s *Store) RepoCommitAuthorAccounts(ctx context.Context, repoID int64) (map[int64]bool, map[string]bool, error) {
	rows, err := s.queries.ListRepoCommitAuthorAccounts(ctx, repoID)
	if err != nil {
		return nil, nil, wrapErr(i18n.T("store.pulls.commit_authors", repoID), err)
	}
	ids, logins := map[int64]bool{}, map[string]bool{}
	for _, r := range rows {
		if r.ProviderUserID > 0 {
			ids[r.ProviderUserID] = true
		}
		if l := strings.TrimSpace(r.Login); l != "" {
			logins[strings.ToLower(l)] = true
		}
	}
	return ids, logins, nil
}

// PullRequestReviewsWrite sont les compteurs de relectures d'une pull request (voir
// model.PullRequest.ReviewsByOthers).
type PullRequestReviewsWrite struct {
	RepoID                  int64
	Number                  int
	ReviewsByOthers         int
	ApprovalsByOthers       int
	ApprovalsByContributors int       // au plus ApprovalsByOthers
	CheckedAt               time.Time // date de la lecture (non nulle)
}

// SetPullRequestReviews écrit les compteurs de relectures d'une pull request déjà en base ; false si elle
// est absente. Compteur négatif, ApprovalsByContributors supérieur à ApprovalsByOthers ou date nulle :
// erreur, rien n'est écrit.
func (s *Store) SetPullRequestReviews(ctx context.Context, w PullRequestReviewsWrite) (bool, error) {
	if w.ReviewsByOthers < 0 || w.ApprovalsByOthers < 0 || w.ApprovalsByContributors < 0 ||
		w.ApprovalsByContributors > w.ApprovalsByOthers || w.CheckedAt.IsZero() {
		return false, errors.New(i18n.T("store.pulls.reviews_invalid", w.Number))
	}
	n, err := s.queries.SetPullRequestReviews(ctx, db.SetPullRequestReviewsParams{
		ReviewsByOthers:         sql.NullInt64{Int64: int64(w.ReviewsByOthers), Valid: true},
		ApprovalsByOthers:       sql.NullInt64{Int64: int64(w.ApprovalsByOthers), Valid: true},
		ApprovalsByContributors: sql.NullInt64{Int64: int64(w.ApprovalsByContributors), Valid: true},
		ReviewsCheckedAt:        nullTime(w.CheckedAt),
		RepoID:                  w.RepoID,
		Number:                  int64(w.Number),
	})
	if err != nil {
		return false, wrapErr(i18n.T("store.pulls.reviews", w.Number, w.RepoID), err)
	}
	return n > 0, nil
}
