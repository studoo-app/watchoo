package store

// Lectures des pull requests, releases et tags (V2) : point de reprise de la synchro des PR, vues de
// la CLI (« watchoo prs », « watchoo releases »), du web et du TUI.

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// LatestPullRequestUpdate renvoie la date de modification la plus récente des pull requests du dépôt
// repoID (point de reprise de la synchro suivante) ; zéro si aucune PR n'est en base.
func (s *Store) LatestPullRequestUpdate(ctx context.Context, repoID int64) (time.Time, error) {
	v, err := s.queries.MaxPullRequestUpdatedAt(ctx, repoID)
	if err != nil {
		return time.Time{}, fmt.Errorf("dernière pull request du dépôt %d : %w", repoID, err)
	}
	if v == "" {
		return time.Time{}, nil
	}
	t, err := ParseTime(v)
	if err != nil {
		return time.Time{}, fmt.Errorf("dernière pull request du dépôt %d : %w", repoID, err)
	}
	return t, nil
}

// ListPullRequests renvoie les pull requests qui satisfont f, de la plus récemment modifiée à la plus
// ancienne (puis dépôt, puis numéro décroissant) ; liste vide non nil si aucune. Un Status inconnu
// est une erreur.
func (s *Store) ListPullRequests(ctx context.Context, f PullRequestFilter) ([]PullRequestView, error) {
	params := db.ListPullRequestsParams{RepoID: optionalID(f.RepoID),
		AuthorProviderID: optionalID(f.AuthorProviderID), RowLimit: rowLimit(f.Limit)}
	switch f.Status {
	case "":
	case model.PullRequestStatusOpen:
		params.State = model.PullRequestOpen
	case model.PullRequestStatusMerged:
		params.Merged = true
	case model.PullRequestStatusClosed:
		params.State, params.Merged = model.PullRequestClosed, false
	default:
		return nil, fmt.Errorf("liste des pull requests : état %q inconnu", f.Status)
	}
	rows, err := s.queries.ListPullRequests(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("liste des pull requests : %w", err)
	}
	views := make([]PullRequestView, 0, len(rows))
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
		}
		if err := parseTimes(fmt.Sprintf("pull request n° %d", row.Number),
			[]string{row.CreatedAt, row.UpdatedAt}, []*time.Time{&pr.CreatedAt, &pr.UpdatedAt},
			[]sql.NullString{row.ClosedAt, row.MergedAt}, []*time.Time{&pr.ClosedAt, &pr.MergedAt}); err != nil {
			return nil, err
		}
		views = append(views, PullRequestView{
			PullRequest: pr,
			RepoRef:     RepoRef{RepoOwner: row.RepoOwner, RepoName: row.RepoName, RepoLabel: row.RepoLabel},
		})
	}
	return views, nil
}

// PullRequestCounts compte les pull requests du dépôt repoID par état affiché.
func (s *Store) PullRequestCounts(ctx context.Context, repoID int64) (PullRequestCounts, error) {
	row, err := s.queries.PullRequestCounts(ctx, repoID)
	if err != nil {
		return PullRequestCounts{}, fmt.Errorf("nombre de pull requests du dépôt %d : %w", repoID, err)
	}
	return PullRequestCounts{Open: int(row.OpenCount), Merged: int(row.MergedCount), Closed: int(row.ClosedCount)}, nil
}

// ListReleases renvoie les releases qui satisfont f, de la plus récente (publication, sinon
// création) à la plus ancienne ; liste vide non nil si aucune.
func (s *Store) ListReleases(ctx context.Context, f ReleaseFilter) ([]ReleaseView, error) {
	rows, err := s.queries.ListReleases(ctx, db.ListReleasesParams{RepoID: optionalID(f.RepoID), RowLimit: rowLimit(f.Limit)})
	if err != nil {
		return nil, fmt.Errorf("liste des releases : %w", err)
	}
	views := make([]ReleaseView, 0, len(rows))
	for _, row := range rows {
		r, err := releaseFromDB(db.Release{
			RepoID: row.RepoID, ProviderID: row.ProviderID, TagName: row.TagName, Name: row.Name,
			Draft: row.Draft, Prerelease: row.Prerelease, AuthorLogin: row.AuthorLogin, URL: row.URL,
			CreatedAt: row.CreatedAt, PublishedAt: row.PublishedAt,
		})
		if err != nil {
			return nil, err
		}
		views = append(views, ReleaseView{
			Release: r,
			RepoRef: RepoRef{RepoOwner: row.RepoOwner, RepoName: row.RepoName, RepoLabel: row.RepoLabel},
		})
	}
	return views, nil
}

// ListTags renvoie les tags qui satisfont f, avec la date et le lien du commit visé s'il est en base :
// commit le plus récent d'abord, tags dont le commit est inconnu à la fin, puis nom décroissant ;
// liste vide non nil si aucun.
func (s *Store) ListTags(ctx context.Context, f ReleaseFilter) ([]TagView, error) {
	rows, err := s.queries.ListTags(ctx, db.ListTagsParams{RepoID: optionalID(f.RepoID), RowLimit: rowLimit(f.Limit)})
	if err != nil {
		return nil, fmt.Errorf("liste des tags : %w", err)
	}
	views := make([]TagView, 0, len(rows))
	for _, row := range rows {
		v := TagView{
			Tag:       model.Tag{RepoID: row.RepoID, Name: row.Name, SHA: row.SHA},
			RepoRef:   RepoRef{RepoOwner: row.RepoOwner, RepoName: row.RepoName, RepoLabel: row.RepoLabel},
			CommitURL: row.CommitURL,
		}
		if row.CommittedAt != "" {
			if v.CommittedAt, err = ParseTime(row.CommittedAt); err != nil {
				return nil, fmt.Errorf("tag du commit %s : %w", shortSHA(row.SHA), err)
			}
		}
		views = append(views, v)
	}
	return views, nil
}

// releaseFromDB convertit une ligne sqlc en model.Release.
func releaseFromDB(row db.Release) (model.Release, error) {
	r := model.Release{
		RepoID: row.RepoID, ProviderID: row.ProviderID, TagName: row.TagName, Name: row.Name,
		Draft: row.Draft, Prerelease: row.Prerelease, AuthorLogin: row.AuthorLogin, URL: row.URL,
	}
	if err := parseTimes(fmt.Sprintf("release %d", row.ProviderID),
		[]string{row.CreatedAt}, []*time.Time{&r.CreatedAt},
		[]sql.NullString{row.PublishedAt}, []*time.Time{&r.PublishedAt}); err != nil {
		return model.Release{}, err
	}
	return r, nil
}

// parseTimes convertit les dates obligatoires required (vers dst) et facultatives optional (vers
// optDst) d'une ligne désignée par what.
func parseTimes(what string, required []string, dst []*time.Time, optional []sql.NullString, optDst []*time.Time) error {
	for i, v := range required {
		t, err := ParseTime(v)
		if err != nil {
			return fmt.Errorf("%s : %w", what, err)
		}
		*dst[i] = t
	}
	for i, v := range optional {
		t, err := parseNullTime(v)
		if err != nil {
			return fmt.Errorf("%s : %w", what, err)
		}
		*optDst[i] = t
	}
	return nil
}
