package store

// Lectures pour les conventions (phases 8.1 et 8.2) : commits, pull requests, tags et releases. Le calcul
// est fait par internal/conventions, à la lecture ; rien n'est stocké. Le message de commit et le titre de
// PR ne sont lus qu'en mémoire et ne sont jamais journalisés.

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// ConventionCommits renvoie les commits du dépôt repoID, de l'auteur authorID (0 = tous les auteurs),
// du plus récent au plus ancien, avec leur rattachement aux branches suivies et à la branche par défaut ;
// liste vide non nil si aucun.
func (s *Store) ConventionCommits(ctx context.Context, repoID, authorID int64) ([]model.ConventionCommit, error) {
	rows, err := s.queries.ListConventionCommits(ctx, db.ListConventionCommitsParams{
		RepoID: repoID, AuthorID: optionalID(authorID)})
	if err != nil {
		return nil, wrapErr(i18n.T("store.conventions.commits", repoID), err)
	}
	out := make([]model.ConventionCommit, 0, len(rows))
	for _, r := range rows {
		at, err := ParseTime(r.AuthoredAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.conventions.commits", repoID), err)
		}
		c := model.ConventionCommit{
			SHA:             r.SHA,
			Message:         r.Message,
			AuthoredAt:      at,
			AuthorID:        r.AuthorID,
			AuthorName:      r.AuthorName,
			AuthorLogin:     r.AuthorLogin,
			AuthorEmails:    strings.Fields(r.AuthorEmails),
			CommitterLogin:  r.CommitterLogin,
			CommitterEmails: strings.Fields(r.CommitterEmails),
			OnTracked:       r.OnTracked != 0,
			OnDefault:       r.OnDefault != 0,
		}
		if r.ParentCount.Valid {
			n := int(r.ParentCount.Int64)
			c.ParentCount = &n
		}
		out = append(out, c)
	}
	return out, nil
}

// ConventionPullRequests renvoie les pull requests du dépôt repoID, numéro décroissant, avec le
// contributeur du même compte de forge que l'auteur (AuthorID 0 si aucun) ; liste vide non nil si aucune.
// Le titre n'est lu qu'en mémoire (PR1, PR2).
func (s *Store) ConventionPullRequests(ctx context.Context, repoID int64) ([]model.ConventionPullRequest, error) {
	rows, err := s.queries.ListConventionPullRequests(ctx, repoID)
	if err != nil {
		return nil, wrapErr(i18n.T("store.conventions.pulls", repoID), err)
	}
	out := make([]model.ConventionPullRequest, 0, len(rows))
	for _, row := range rows {
		pr := model.PullRequest{
			RepoID:           row.RepoID,
			Number:           int(row.Number),
			Title:            row.Title,
			State:            row.State,
			Merged:           row.Merged,
			Draft:            row.Draft,
			AuthorLogin:      row.AuthorLogin,
			AuthorProviderID: row.AuthorProviderID,
			URL:              row.URL,
			BaseBranch:       row.BaseBranch,
			HeadBranch:       row.HeadBranch,
			ToolIDs:          splitList(row.ToolIds),
		}
		if err := parseTimes(i18n.T("store.pulls.number", row.Number),
			[]string{row.CreatedAt, row.UpdatedAt}, []*time.Time{&pr.CreatedAt, &pr.UpdatedAt},
			[]sql.NullString{row.ClosedAt, row.MergedAt}, []*time.Time{&pr.ClosedAt, &pr.MergedAt}); err != nil {
			return nil, err
		}
		out = append(out, model.ConventionPullRequest{PullRequest: pr, AuthorID: row.AuthorID, AuthorName: row.AuthorName})
	}
	return out, nil
}

// ConventionTags renvoie les tags du dépôt repoID (nom et SHA du commit visé), par nom ; liste vide non
// nil si aucun. La synchro les écrit dans les deux modes (API et clone).
func (s *Store) ConventionTags(ctx context.Context, repoID int64) ([]model.Tag, error) {
	views, err := s.ListTags(ctx, ReleaseFilter{RepoID: repoID})
	if err != nil {
		return nil, err
	}
	out := make([]model.Tag, 0, len(views))
	for _, v := range views {
		out = append(out, v.Tag)
	}
	slices.SortFunc(out, func(a, b model.Tag) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// ConventionReleases renvoie les releases du dépôt repoID (brouillons compris), de la plus récente à la
// plus ancienne ; liste vide non nil si aucune.
func (s *Store) ConventionReleases(ctx context.Context, repoID int64) ([]model.Release, error) {
	views, err := s.ListReleases(ctx, ReleaseFilter{RepoID: repoID})
	if err != nil {
		return nil, err
	}
	out := make([]model.Release, 0, len(views))
	for _, v := range views {
		out = append(out, v.Release)
	}
	return out, nil
}
