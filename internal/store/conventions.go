package store

// Lectures pour les conventions (phases 8.1 et 8.2) : commits, pull requests, tags et releases. Le calcul
// est fait par internal/conventions, à la lecture ; rien n'est stocké. Le message de commit et le titre de
// PR ne sont lus qu'en mémoire et ne sont jamais journalisés. Phase 8.3 : cache des critères de fichiers
// (table repository_conventions), écrit par la synchro : codes, états et compteurs seulement.

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
		committed, err := ParseTime(r.CommittedAt)
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
			CommittedAt:     committed,
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
			MergeCommitSHA:   row.MergeCommitSha.String,
			FromFork:         boolPtr(row.FromFork),
			BodySections:     int64Ptr(row.BodySections),

			ReviewsByOthers:         int64Ptr(row.ReviewsByOthers),
			ApprovalsByOthers:       int64Ptr(row.ApprovalsByOthers),
			ApprovalsByContributors: int64Ptr(row.ApprovalsByContributors),
		}
		if err := parseTimes(i18n.T("store.pulls.number", row.Number),
			[]string{row.CreatedAt, row.UpdatedAt}, []*time.Time{&pr.CreatedAt, &pr.UpdatedAt},
			[]sql.NullString{row.ClosedAt, row.MergedAt, row.ReviewsCheckedAt},
			[]*time.Time{&pr.ClosedAt, &pr.MergedAt, &pr.ReviewsCheckedAt}); err != nil {
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

// SaveRepositoryConventions enregistre le résultat des critères de fichiers d'une révision de dépôt
// (phase 8.3) ; il remplace celui de la même révision (tête, ou tag du même nom). La base refuse un type
// de révision inconnu, un SHA mal formé ou un résultat qui n'est pas un objet JSON.
func (s *Store) SaveRepositoryConventions(ctx context.Context, r model.RepositoryConventions) error {
	err := s.queries.UpsertRepositoryConventions(ctx, db.UpsertRepositoryConventionsParams{
		RepoID: r.RepoID, RevKind: r.RevKind, TagName: r.TagName, RevSha: r.RevSHA,
		RulesVersion: int64(r.RulesVersion), ConfigHash: r.ConfigHash, Results: r.Results,
		ComputedAt: FormatTime(r.ComputedAt),
	})
	if err != nil {
		return wrapErr(i18n.T("store.conventions.files", r.RepoID), err)
	}
	return nil
}

// RepositoryConventions renvoie les résultats en cache des critères de fichiers du dépôt repoID (0 = tous
// les dépôts), par dépôt, la tête avant les tags, les tags par nom ; liste vide non nil si aucun.
func (s *Store) RepositoryConventions(ctx context.Context, repoID int64) ([]model.RepositoryConventions, error) {
	rows, err := s.queries.ListRepositoryConventions(ctx, optionalID(repoID))
	if err != nil {
		return nil, wrapErr(i18n.T("store.conventions.files", repoID), err)
	}
	out := make([]model.RepositoryConventions, 0, len(rows))
	for _, r := range rows {
		at, err := ParseTime(r.ComputedAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.conventions.files", r.RepoID), err)
		}
		out = append(out, model.RepositoryConventions{
			RepoID: r.RepoID, RevKind: r.RevKind, TagName: r.TagName, RevSHA: r.RevSha,
			RulesVersion: int(r.RulesVersion), ConfigHash: r.ConfigHash, Results: r.Results, ComputedAt: at,
		})
	}
	return out, nil
}

// DeleteRepositoryConventionTags supprime les résultats des tags du dépôt repoID qui ne sont pas dans
// keep (tags disparus, jalons retirés) ; keep vide : tous les tags. La ligne de la tête reste. Renvoie
// le nombre de lignes supprimées.
func (s *Store) DeleteRepositoryConventionTags(ctx context.Context, repoID int64, keep []string) (int, error) {
	var (
		n   int64
		err error
	)
	if len(keep) == 0 {
		n, err = s.queries.DeleteRepositoryConventionTags(ctx, repoID)
	} else {
		n, err = s.queries.DeleteRepositoryConventionTagsExcept(ctx, db.DeleteRepositoryConventionTagsExceptParams{
			RepoID: repoID, Keep: keep})
	}
	if err != nil {
		return 0, wrapErr(i18n.T("store.conventions.files", repoID), err)
	}
	return int(n), nil
}

// DeleteRepositoryConventionHead supprime le résultat de la tête du dépôt repoID (branche par défaut
// absente ou dépôt vide : le résultat précédent ne vaut plus). Renvoie le nombre de lignes supprimées.
func (s *Store) DeleteRepositoryConventionHead(ctx context.Context, repoID int64) (int, error) {
	n, err := s.queries.DeleteRepositoryConventionHead(ctx, repoID)
	if err != nil {
		return 0, wrapErr(i18n.T("store.conventions.files", repoID), err)
	}
	return int(n), nil
}
