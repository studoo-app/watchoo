package store

// Vues de lecture (CLI, TUI, plus tard le web) : derniers commits, classement des contributeurs,
// résumé par dépôt. Filtres facultatifs avec sqlc.narg (voir sqlc.yaml).

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// ListRecentCommits renvoie les commits qui satisfont f, du plus récent au plus ancien (committed_at
// décroissant, puis SHA), chaque commit une seule fois même s'il est sur plusieurs branches ; liste
// vide non nil si aucun.
//
// Le filtre Author compare le login et le nom sans tenir compte de la casse des lettres ASCII
// (COLLATE NOCASE de SQLite), et l'e-mail après mise en minuscules.
func (s *Store) ListRecentCommits(ctx context.Context, f CommitFilter) ([]CommitView, error) {
	params := db.ListRecentCommitsParams{
		RepoID:    optionalID(f.RepoID),
		Since:     optionalTime(f.Since),
		Branch:    optionalString(f.Branch),
		RowLimit:  rowLimit(f.Limit),
		RowOffset: int64(max(f.Offset, 0)),
	}
	if author := strings.TrimSpace(f.Author); author != "" {
		params.Author = author
		params.AuthorEmail = sql.NullString{String: normalizeEmail(author), Valid: true}
	}
	rows, err := s.queries.ListRecentCommits(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("liste des commits : %w", err)
	}
	views, err := commitViewsFromRows(rows)
	if err != nil {
		return nil, err
	}
	return views, nil
}

// ContributorRanking renvoie le classement des contributeurs AUTEURS de commits satisfaisant f :
// commits décroissants, puis dernier commit le plus récent, puis id. Les commits sans auteur
// (author_id NULL) ne sont pas comptés. Liste vide non nil si aucun.
func (s *Store) ContributorRanking(ctx context.Context, f ContributorFilter) ([]ContributorStats, error) {
	rows, err := s.queries.ContributorRanking(ctx, db.ContributorRankingParams{
		RepoID:   optionalID(f.RepoID),
		Since:    optionalTime(f.Since),
		RowLimit: rowLimit(f.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("classement des contributeurs : %w", err)
	}
	stats := make([]ContributorStats, 0, len(rows))
	for _, row := range rows {
		last, err := ParseTime(row.LastCommitAt)
		if err != nil {
			return nil, fmt.Errorf("contributeur %d, dernier commit : %w", row.ID, err)
		}
		stats = append(stats, ContributorStats{
			Contributor: contributorFromDB(db.Contributor{
				ID: row.ID, Provider: row.Provider, ProviderUserID: row.ProviderUserID,
				ProviderLogin: row.ProviderLogin, Name: row.Name, AvatarURL: row.AvatarURL,
			}),
			Commits:      int(row.Commits),
			Additions:    int(row.Additions),
			Deletions:    int(row.Deletions),
			Repos:        int(row.Repos),
			LastCommitAt: last,
		})
	}
	return stats, nil
}

// RepositorySummaries renvoie un résumé par dépôt (tous, suivi arrêté compris), trié comme
// ListRepositories (owner puis name) ; liste vide non nil si aucun dépôt.
func (s *Store) RepositorySummaries(ctx context.Context) ([]RepositorySummary, error) {
	summaries, err := s.repositorySummaries(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("résumé des dépôts : %w", err)
	}
	return summaries, nil
}

// RepositorySummary renvoie le résumé du dépôt id, ou une erreur enveloppant ErrNotFound.
func (s *Store) RepositorySummary(ctx context.Context, id int64) (RepositorySummary, error) {
	summaries, err := s.repositorySummaries(ctx, id)
	if err != nil {
		return RepositorySummary{}, fmt.Errorf("résumé du dépôt %d : %w", id, err)
	}
	if len(summaries) == 0 {
		return RepositorySummary{}, fmt.Errorf("dépôt %d : %w", id, ErrNotFound)
	}
	return summaries[0], nil
}

// repositorySummaries construit les résumés de tous les dépôts (id = 0) ou du seul dépôt id, à partir
// de requêtes agrégées lues dans une même transaction (état cohérent même pendant une synchro).
func (s *Store) repositorySummaries(ctx context.Context, id int64) ([]RepositorySummary, error) {
	var summaries []RepositorySummary
	err := s.readTx(ctx, func(q *db.Queries) error {
		var repos []db.Repository
		if id == 0 {
			rows, err := q.ListRepositories(ctx)
			if err != nil {
				return fmt.Errorf("liste des dépôts : %w", err)
			}
			repos = rows
		} else {
			row, err := q.GetRepository(ctx, id)
			if err != nil {
				return repositoryLookupError(fmt.Sprintf("dépôt %d", id), err)
			}
			repos = []db.Repository{row}
		}
		filter := optionalID(id)

		commitCounts, err := q.CountCommitsByRepo(ctx, filter)
		if err != nil {
			return fmt.Errorf("nombre de commits : %w", err)
		}
		commits := make(map[int64]int, len(commitCounts))
		for _, c := range commitCounts {
			commits[c.RepoID] = int(c.Commits)
		}

		branchCounts, err := q.CountTrackedBranchesByRepo(ctx, filter)
		if err != nil {
			return fmt.Errorf("nombre de branches suivies : %w", err)
		}
		branches := make(map[int64]int, len(branchCounts))
		for _, b := range branchCounts {
			branches[b.RepoID] = int(b.Branches)
		}

		lastCommitRows, err := q.LastCommitByRepo(ctx, filter)
		if err != nil {
			return fmt.Errorf("dernier commit : %w", err)
		}
		lastCommits := make(map[int64]db.LastCommitByRepoRow, len(lastCommitRows))
		for _, c := range lastCommitRows {
			lastCommits[c.RepoID] = c
		}

		runRows, err := q.LastSyncRunByRepo(ctx, filter)
		if err != nil {
			return fmt.Errorf("dernière synchro : %w", err)
		}
		runs := make(map[int64]db.SyncRun, len(runRows))
		for _, r := range runRows {
			runs[r.RepoID] = r
		}

		summaries = make([]RepositorySummary, 0, len(repos))
		for _, row := range repos {
			repo, err := repositoryFromDB(row)
			if err != nil {
				return err
			}
			summary := RepositorySummary{
				Repository:      repo,
				Commits:         commits[repo.ID],
				TrackedBranches: branches[repo.ID],
			}
			if c, ok := lastCommits[repo.ID]; ok {
				commit, err := commitFromDB(db.Commit{
					RepoID: c.RepoID, SHA: c.SHA, AuthorID: c.AuthorID, CommitterID: c.CommitterID,
					Message: c.Message, AuthoredAt: c.AuthoredAt, CommittedAt: c.CommittedAt,
					Additions: c.Additions, Deletions: c.Deletions, URL: c.URL,
				})
				if err != nil {
					return err
				}
				summary.LastCommit = CommitView{
					Commit:      commit,
					RepoOwner:   repo.Owner,
					RepoName:    repo.Name,
					RepoLabel:   repo.Label,
					AuthorName:  c.AuthorName,
					AuthorLogin: c.AuthorLogin,
				}
			}
			if r, ok := runs[repo.ID]; ok {
				if summary.LastRun, err = syncRunFromDB(r); err != nil {
					return err
				}
			}
			summaries = append(summaries, summary)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return summaries, nil
}

// commitFromDB convertit une ligne sqlc en model.Commit (auteur ou committer NULL = 0).
func commitFromDB(row db.Commit) (model.Commit, error) {
	authoredAt, err := ParseTime(row.AuthoredAt)
	if err != nil {
		return model.Commit{}, fmt.Errorf("commit %s, authored_at : %w", shortSHA(row.SHA), err)
	}
	committedAt, err := ParseTime(row.CommittedAt)
	if err != nil {
		return model.Commit{}, fmt.Errorf("commit %s, committed_at : %w", shortSHA(row.SHA), err)
	}
	return model.Commit{
		RepoID:      row.RepoID,
		SHA:         row.SHA,
		AuthorID:    row.AuthorID.Int64,
		CommitterID: row.CommitterID.Int64,
		Message:     row.Message,
		AuthoredAt:  authoredAt,
		CommittedAt: committedAt,
		Additions:   int(row.Additions),
		Deletions:   int(row.Deletions),
		URL:         row.URL,
	}, nil
}

// contributorFromDB convertit une ligne sqlc en model.Contributor (Emails reste nil).
func contributorFromDB(row db.Contributor) model.Contributor {
	return model.Contributor{
		ID:             row.ID,
		Provider:       row.Provider.String,
		ProviderUserID: row.ProviderUserID.Int64,
		ProviderLogin:  row.ProviderLogin,
		Name:           row.Name,
		AvatarURL:      row.AvatarURL,
	}
}

// optionalID convertit un identifiant facultatif (0 = absent) pour un filtre sqlc.narg.
func optionalID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// optionalTime convertit une borne de date facultative (zéro = absente) pour un filtre sqlc.narg.
func optionalTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return FormatTime(t)
}

// optionalString convertit un filtre texte facultatif (vide = absent) pour un filtre sqlc.narg.
func optionalString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// rowLimit convertit une limite (0 ou négative = aucune) pour LIMIT : -1 signifie « sans limite »
// pour SQLite.
func rowLimit(n int) int64 {
	if n <= 0 {
		return -1
	}
	return int64(n)
}
