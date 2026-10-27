package store

// Vues de lecture ajoutées en phase 4 pour l'interface web (internal/web) : historique paginé avec
// total, dates de commit pour la série d'activité, commits de chaque contributeur par dépôt, fin de la
// dernière synchro (première synchro automatique de « watchoo serve »). Comme les autres vues, elles
// ne renvoient que des types de la bibliothèque standard et de internal/model.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/store/db"
)

// MaxPerPage borne la taille d'une page de CommitPage.
const MaxPerPage = 200

// CommitPage est une page de l'historique des commits (CommitPage).
type CommitPage struct {
	Commits []CommitView // commits de la page, du plus récent au plus ancien ; vide non nil
	Total   int          // nombre total de commits qui satisfont le filtre (toutes pages)
	Page    int          // numéro de la page renvoyée, à partir de 1 (ramené dans [1, Pages])
	PerPage int          // taille d'une page
	Pages   int          // nombre de pages (au moins 1, même sans commit)
}

// HasPrev indique s'il existe une page précédente.
func (p CommitPage) HasPrev() bool { return p.Page > 1 }

// HasNext indique s'il existe une page suivante.
func (p CommitPage) HasNext() bool { return p.Page < p.Pages }

// CommitPage renvoie la page page (à partir de 1) de l'historique filtré par f, perPage commits par
// page (ramené entre 1 et MaxPerPage), dans l'ordre de ListRecentCommits (committed_at décroissant,
// puis SHA ; chaque commit une seule fois). f.Limit et f.Offset sont ignorés : ils sont calculés à
// partir de page et perPage. Une page hors bornes est ramenée à la première ou à la dernière. Le total
// et la page sont lus dans une même transaction (état cohérent pendant une synchro).
func (s *Store) CommitPage(ctx context.Context, f CommitFilter, page, perPage int) (CommitPage, error) {
	perPage = min(max(perPage, 1), MaxPerPage)
	var result CommitPage
	err := s.readTx(ctx, func(q *db.Queries) error {
		countParams := db.CountRecentCommitsParams{
			RepoID: optionalID(f.RepoID),
			Since:  optionalTime(f.Since),
			Branch: optionalString(f.Branch),
		}
		if author := strings.TrimSpace(f.Author); author != "" {
			countParams.Author = author
			countParams.AuthorEmail = sql.NullString{String: normalizeEmail(author), Valid: true}
		}
		total, err := q.CountRecentCommits(ctx, countParams)
		if err != nil {
			return fmt.Errorf("nombre de commits : %w", err)
		}
		pages := max(1, (int(total)+perPage-1)/perPage)
		page = min(max(page, 1), pages)
		result = CommitPage{Total: int(total), Page: page, PerPage: perPage, Pages: pages}

		params := db.ListRecentCommitsParams{
			RepoID:      countParams.RepoID,
			Since:       countParams.Since,
			Branch:      countParams.Branch,
			Author:      countParams.Author,
			AuthorEmail: countParams.AuthorEmail,
			RowLimit:    int64(perPage),
			RowOffset:   int64((page - 1) * perPage),
		}
		rows, err := q.ListRecentCommits(ctx, params)
		if err != nil {
			return fmt.Errorf("liste des commits : %w", err)
		}
		result.Commits, err = commitViewsFromRows(rows)
		return err
	})
	if err != nil {
		return CommitPage{}, fmt.Errorf("historique des commits : %w", err)
	}
	return result, nil
}

// commitViewsFromRows convertit les lignes de ListRecentCommits (liste vide non nil si aucune).
func commitViewsFromRows(rows []db.ListRecentCommitsRow) ([]CommitView, error) {
	views := make([]CommitView, 0, len(rows))
	for _, row := range rows {
		commit, err := commitFromDB(db.Commit{
			RepoID: row.RepoID, SHA: row.SHA, AuthorID: row.AuthorID, CommitterID: row.CommitterID,
			Message: row.Message, AuthoredAt: row.AuthoredAt, CommittedAt: row.CommittedAt,
			Additions: row.Additions, Deletions: row.Deletions, URL: row.URL,
		})
		if err != nil {
			return nil, err
		}
		views = append(views, CommitView{
			Commit:      commit,
			RepoOwner:   row.RepoOwner,
			RepoName:    row.RepoName,
			RepoLabel:   row.RepoLabel,
			AuthorName:  row.AuthorName,
			AuthorLogin: row.AuthorLogin,
		})
	}
	return views, nil
}

// CommitTimes renvoie la date de commit (committed_at, UTC) de chaque commit du dépôt repoID (0 = tous
// les dépôts) depuis since inclus, sans ordre garanti ; chaque commit une seule fois. Sert à la série
// d'activité par jour (activity.DailyCounts), calculée en Go dans le fuseau d'affichage.
func (s *Store) CommitTimes(ctx context.Context, repoID int64, since time.Time) ([]time.Time, error) {
	rows, err := s.queries.CommitTimesSince(ctx, db.CommitTimesSinceParams{
		RepoID: optionalID(repoID),
		Since:  FormatTime(since),
	})
	if err != nil {
		return nil, fmt.Errorf("dates des commits : %w", err)
	}
	times := make([]time.Time, 0, len(rows))
	for _, row := range rows {
		t, err := ParseTime(row)
		if err != nil {
			return nil, fmt.Errorf("dates des commits : %w", err)
		}
		times = append(times, t)
	}
	return times, nil
}

// AuthorCommitTime est la date de commit (committed_at, UTC) d'un commit dont l'auteur est connu.
type AuthorCommitTime struct {
	ContributorID int64     // auteur (contributors.id)
	CommittedAt   time.Time // date du commit, en UTC
}

// AuthorCommitTimes renvoie l'auteur et la date de chaque commit d'auteur connu depuis since inclus,
// tous dépôts confondus, sans ordre garanti ; chaque commit une seule fois. Sert à la série
// d'activité par contributeur (graphique de la page des contributeurs), calculée en Go dans le
// fuseau d'affichage.
func (s *Store) AuthorCommitTimes(ctx context.Context, since time.Time) ([]AuthorCommitTime, error) {
	rows, err := s.queries.AuthorCommitTimesSince(ctx, FormatTime(since))
	if err != nil {
		return nil, fmt.Errorf("dates des commits par auteur : %w", err)
	}
	out := make([]AuthorCommitTime, 0, len(rows))
	for _, row := range rows {
		t, err := ParseTime(row.CommittedAt)
		if err != nil {
			return nil, fmt.Errorf("dates des commits par auteur : %w", err)
		}
		out = append(out, AuthorCommitTime{ContributorID: row.AuthorID.Int64, CommittedAt: t})
	}
	return out, nil
}

// ContributorRepoStats est le nombre de commits d'un contributeur (AUTEUR) dans un dépôt.
type ContributorRepoStats struct {
	ContributorID int64     // contributeur (contributors.id)
	RepoID        int64     // dépôt
	RepoOwner     string    // propriétaire actuel du dépôt
	RepoName      string    // nom actuel du dépôt
	RepoLabel     string    // nom affiché du dépôt (vide = owner/name)
	Commits       int       // commits dont il est l'auteur dans ce dépôt
	LastCommitAt  time.Time // son commit le plus récent dans ce dépôt (committed_at)
}

// RepoDisplayName renvoie le nom affiché du dépôt (RepoLabel, sinon owner/name).
func (c ContributorRepoStats) RepoDisplayName() string {
	if c.RepoLabel != "" {
		return c.RepoLabel
	}
	return c.RepoOwner + "/" + c.RepoName
}

// ContributorRepoBreakdown renvoie, pour chaque contributeur auteur d'au moins un commit, ses commits
// par dépôt : trié par ContributorID, puis commits décroissants, puis RepoID. Les commits sans auteur
// ne sont pas comptés. Liste vide non nil si aucun. La page des contributeurs regroupe ces lignes par
// ContributorID pour compléter ContributorRanking.
func (s *Store) ContributorRepoBreakdown(ctx context.Context) ([]ContributorRepoStats, error) {
	rows, err := s.queries.ContributorRepoBreakdown(ctx)
	if err != nil {
		return nil, fmt.Errorf("commits des contributeurs par dépôt : %w", err)
	}
	out := make([]ContributorRepoStats, 0, len(rows))
	for _, row := range rows {
		last, err := ParseTime(row.LastCommitAt)
		if err != nil {
			return nil, fmt.Errorf("contributeur %d, dépôt %d, dernier commit : %w",
				row.ContributorID.Int64, row.RepoID, err)
		}
		out = append(out, ContributorRepoStats{
			ContributorID: row.ContributorID.Int64,
			RepoID:        row.RepoID,
			RepoOwner:     row.RepoOwner,
			RepoName:      row.RepoName,
			RepoLabel:     row.RepoLabel,
			Commits:       int(row.Commits),
			LastCommitAt:  last,
		})
	}
	return out, nil
}

// LastSyncFinishedAt renvoie la fin de la synchro la plus récente (sync_runs.finished_at, tous dépôts
// et tous statuts confondus), ou le temps zéro si aucune synchro n'est terminée. « watchoo serve » s'en
// sert pour ne lancer une synchro au démarrage que si la dernière date de plus d'un intervalle.
func (s *Store) LastSyncFinishedAt(ctx context.Context) (time.Time, error) {
	raw, err := s.queries.LastSyncFinishedAt(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("dernière synchro : %w", err)
	}
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := ParseTime(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("dernière synchro : %w", err)
	}
	return t, nil
}
