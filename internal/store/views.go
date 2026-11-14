package store

// Vues de lecture (CLI, TUI, plus tard le web) : derniers commits, classement des contributeurs,
// résumé par dépôt. Filtres facultatifs avec sqlc.narg (voir sqlc.yaml).

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

// ListRecentCommits renvoie les commits qui satisfont f, du plus récent au plus ancien (committed_at
// décroissant, puis SHA), chaque commit une seule fois même s'il est sur plusieurs branches ; liste
// vide non nil si aucun.
//
// Le filtre Author compare le login et le nom sans tenir compte de la casse des lettres ASCII
// (COLLATE NOCASE de SQLite), et l'e-mail après mise en minuscules.
func (s *Store) ListRecentCommits(ctx context.Context, f CommitFilter) ([]CommitView, error) {
	params := db.ListRecentCommitsParams{
		RepoID:    optionalID(f.RepoID),
		AuthorID:  optionalID(f.AuthorID),
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
		return nil, wrapErr(i18n.T("store.views.commits"), err)
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
	var rows []db.ContributorRankingRow
	var (
		tools map[int64][]model.AIToolCount
		langs contributorLanguageLines
	)
	err := s.readTx(ctx, func(q *db.Queries) error {
		var err error
		rows, err = q.ContributorRanking(ctx, db.ContributorRankingParams{
			RepoID:   optionalID(f.RepoID),
			Since:    optionalTime(f.Since),
			RowLimit: rowLimit(f.Limit),
		})
		if err != nil {
			return err
		}
		if tools, err = contributorAITools(ctx, q, 0, f.RepoID, f.Since); err != nil {
			return err
		}
		langs, err = contributorLanguages(ctx, q, 0, f.RepoID, f.Since)
		return err
	})
	if err != nil {
		return nil, wrapErr(i18n.T("store.views.ranking"), err)
	}
	stats := make([]ContributorStats, 0, len(rows))
	for _, row := range rows {
		last, err := ParseTime(row.LastCommitAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.views.contrib_last", row.ID), err)
		}
		stats = append(stats, ContributorStats{
			Contributor: contributorFromDB(db.Contributor{
				ID: row.ID, Provider: row.Provider, ProviderUserID: row.ProviderUserID,
				ProviderLogin: row.ProviderLogin, Name: row.Name, Label: row.Label, AvatarURL: row.AvatarURL,
				ProfileURL: row.ProfileURL,
			}),
			Commits:      int(row.Commits),
			Additions:    int(row.Additions),
			Deletions:    int(row.Deletions),
			KnownCommits: int(row.KnownCommits),
			Repos:        int(row.Repos),
			LastCommitAt: last,
			AI:           aiTotals(row.Commits, row.AiScannedCommits, row.AiSignedCommits, tools[row.ID]),
			Languages:    langs.totals(row.ID, row.Commits, row.LanguagesScannedCommits),
		})
	}
	return stats, nil
}

// RepositorySummaries renvoie un résumé par dépôt (tous, suivi arrêté compris), trié comme
// ListRepositories (owner puis name) ; liste vide non nil si aucun dépôt.
func (s *Store) RepositorySummaries(ctx context.Context) ([]RepositorySummary, error) {
	summaries, err := s.repositorySummaries(ctx, 0)
	if err != nil {
		return nil, wrapErr(i18n.T("store.views.summaries"), err)
	}
	return summaries, nil
}

// RepositorySummary renvoie le résumé du dépôt id, ou une erreur enveloppant ErrNotFound.
func (s *Store) RepositorySummary(ctx context.Context, id int64) (RepositorySummary, error) {
	summaries, err := s.repositorySummaries(ctx, id)
	if err != nil {
		return RepositorySummary{}, wrapErr(i18n.T("store.views.summary", id), err)
	}
	if len(summaries) == 0 {
		return RepositorySummary{}, wrapErr(i18n.T("store.repo.id", id), ErrNotFound)
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
				return wrapErr(i18n.T("store.repo.list"), err)
			}
			repos = rows
		} else {
			row, err := q.GetRepository(ctx, id)
			if err != nil {
				return repositoryLookupError(i18n.T("store.repo.id", id), err)
			}
			repos = []db.Repository{row}
		}
		filter := optionalID(id)

		commitCounts, err := q.CountCommitsByRepo(ctx, filter)
		if err != nil {
			return wrapErr(i18n.T("store.views.commit_count"), err)
		}
		lines := make(map[int64]model.LineTotals, len(commitCounts))
		for _, c := range commitCounts {
			lines[c.RepoID] = model.LineTotals{Commits: int(c.Commits), KnownCommits: int(c.KnownCommits),
				Additions: int(c.Additions), Deletions: int(c.Deletions)}
		}

		branchCounts, err := q.CountTrackedBranchesByRepo(ctx, filter)
		if err != nil {
			return wrapErr(i18n.T("store.tracked_branches"), err)
		}
		branches := make(map[int64]int, len(branchCounts))
		for _, b := range branchCounts {
			branches[b.RepoID] = int(b.Branches)
		}

		lastCommitRows, err := q.LastCommitByRepo(ctx, filter)
		if err != nil {
			return wrapErr(i18n.T("store.last_commit"), err)
		}
		lastCommits := make(map[int64]db.LastCommitByRepoRow, len(lastCommitRows))
		for _, c := range lastCommitRows {
			lastCommits[c.RepoID] = c
		}

		runRows, err := q.LastSyncRunByRepo(ctx, filter)
		if err != nil {
			return wrapErr(i18n.T("store.views.last_sync"), err)
		}
		runs := make(map[int64]db.SyncRun, len(runRows))
		for _, r := range runRows {
			runs[r.RepoID] = r
		}

		openRows, err := q.CountOpenPullRequestsByRepo(ctx, filter)
		if err != nil {
			return wrapErr(i18n.T("store.views.open_pulls"), err)
		}
		openPulls := make(map[int64]int, len(openRows))
		for _, o := range openRows {
			openPulls[o.RepoID] = int(o.OpenPulls)
		}

		releaseRows, err := q.LatestReleaseByRepo(ctx, filter)
		if err != nil {
			return wrapErr(i18n.T("store.views.last_release"), err)
		}
		latest := make(map[int64]model.Release, len(releaseRows))
		for _, row := range releaseRows {
			r, err := releaseFromDB(row)
			if err != nil {
				return err
			}
			latest[r.RepoID] = r
		}

		summaries = make([]RepositorySummary, 0, len(repos))
		for _, row := range repos {
			repo, err := repositoryFromDB(row)
			if err != nil {
				return err
			}
			summary := RepositorySummary{
				Repository:       repo,
				Commits:          lines[repo.ID].Commits,
				Lines:            lines[repo.ID],
				TrackedBranches:  branches[repo.ID],
				OpenPullRequests: openPulls[repo.ID],
				LatestRelease:    latest[repo.ID],
			}
			if c, ok := lastCommits[repo.ID]; ok {
				commit, err := commitFromDB(db.Commit{
					RepoID: c.RepoID, SHA: c.SHA, AuthorID: c.AuthorID, CommitterID: c.CommitterID,
					Message: c.Message, AuthoredAt: c.AuthoredAt, CommittedAt: c.CommittedAt,
					Additions: c.Additions, Deletions: c.Deletions, StatsKnown: c.StatsKnown, URL: c.URL,
					ParentCount: c.ParentCount,
				})
				if err != nil {
					return err
				}
				summary.LastCommit = CommitView{
					Commit:           commit,
					RepoOwner:        repo.Owner,
					RepoName:         repo.Name,
					RepoLabel:        repo.Label,
					AuthorName:       c.AuthorName,
					PersonID:         c.PersonID,
					PersonLabel:      c.PersonLabel,
					PersonName:       c.PersonName,
					PersonLogin:      c.PersonLogin,
					AuthorLogin:      c.AuthorLogin,
					AuthorProfileURL: c.AuthorProfileURL,
					AISignatures:     parseAISignatures(c.AiSignatures),
					AIScanned:        c.AiRulesVersion > 0,
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
		return model.Commit{}, wrapErr(i18n.T("store.views.authored_at", shortSHA(row.SHA)), err)
	}
	committedAt, err := ParseTime(row.CommittedAt)
	if err != nil {
		return model.Commit{}, wrapErr(i18n.T("store.views.committed_at", shortSHA(row.SHA)), err)
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
		StatsKnown:  row.StatsKnown,
		URL:         row.URL,
		ParentCount: intPtr(row.ParentCount),
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
		Label:          row.Label,
		AvatarURL:      row.AvatarURL,
		ProfileURL:     row.ProfileURL,
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

// aiTotals assemble les totaux de signatures d'IA d'un contributeur.
func aiTotals(commits, scanned, signed int64, tools []model.AIToolCount) model.AITotals {
	if tools == nil {
		tools = []model.AIToolCount{}
	}
	return model.AITotals{Commits: int(commits), Scanned: int(scanned), Signed: int(signed), Tools: tools}
}

// contributorAITools renvoie les outils signés dans les commits de chaque auteur (contributorID 0 = tous).
func contributorAITools(ctx context.Context, q *db.Queries, contributorID, repoID int64, since time.Time) (map[int64][]model.AIToolCount, error) {
	rows, err := q.ContributorAITools(ctx, db.ContributorAIToolsParams{
		ContributorID: optionalID(contributorID),
		RepoID:        optionalID(repoID),
		Since:         optionalTime(since),
	})
	if err != nil {
		return nil, err
	}
	tools := make(map[int64][]model.AIToolCount)
	for _, row := range rows {
		id := row.ContributorID.Int64
		tools[id] = append(tools[id], model.AIToolCount{Tool: row.Tool, Commits: int(row.Commits)})
	}
	return tools, nil
}

// contributorLanguageLines regroupe les lignes ajoutées par langage lues par ContributorLanguages : par
// contributeur (tous dépôts du filtre), et le langage principal de chaque (contributeur, dépôt).
type contributorLanguageLines struct {
	lines map[int64]map[string]int
	main  map[[2]int64]string
}

// totals renvoie les langages du contributeur id, qui a commits commits dont scanned analysés.
func (c contributorLanguageLines) totals(id, commits, scanned int64) model.LanguageTotals {
	return model.LanguageTotals{Commits: int(commits), Scanned: int(scanned), Lines: model.SortedLanguageLines(c.lines[id])}
}

// contributorLanguages lit les lignes ajoutées par langage dans les commits de chaque auteur
// (contributorID 0 = tous), avec les filtres du classement (repoID 0 = tous les dépôts, since zéro =
// sans borne).
func contributorLanguages(ctx context.Context, q *db.Queries, contributorID, repoID int64, since time.Time) (contributorLanguageLines, error) {
	rows, err := q.ContributorLanguages(ctx, db.ContributorLanguagesParams{
		ContributorID: optionalID(contributorID),
		RepoID:        optionalID(repoID),
		Since:         optionalTime(since),
	})
	if err != nil {
		return contributorLanguageLines{}, err
	}
	out := contributorLanguageLines{lines: make(map[int64]map[string]int), main: make(map[[2]int64]string)}
	for _, row := range rows {
		id := row.ContributorID.Int64
		if out.lines[id] == nil {
			out.lines[id] = make(map[string]int)
		}
		out.lines[id][row.Language] += int(row.Additions)
		// Lignes triées par dépôt puis lignes décroissantes : la première est le langage principal.
		if key := [2]int64{id, row.RepoID}; out.main[key] == "" {
			out.main[key] = row.Language
		}
	}
	return out, nil
}

// parseAISignatures lit la colonne agrégée « outil:type,outil:type » (valeurs du catalogue, sans
// séparateur) ; triée par outil puis type, liste vide non nil si aucune.
func parseAISignatures(raw string) []model.AISignature {
	signatures := []model.AISignature{}
	if raw == "" {
		return signatures
	}
	parts := strings.Split(raw, ",")
	slices.Sort(parts)
	for _, part := range parts {
		tool, kind, _ := strings.Cut(part, ":")
		signatures = append(signatures, model.AISignature{Tool: tool, Kind: kind})
	}
	return signatures
}

// CommitsToScanForAI renvoie au plus limit commits du dépôt pas encore analysés avec la version de
// règles version, par SHA croissant. Les messages et adresses lus ici ne vont jamais dans les journaux.
func (s *Store) CommitsToScanForAI(ctx context.Context, repoID, version int64, limit int) ([]AIScanCommit, error) {
	rows, err := s.queries.ListCommitsToScanForAI(ctx, db.ListCommitsToScanForAIParams{
		RepoID: repoID, CurrentVersion: version, RowLimit: int64(limit),
	})
	if err != nil {
		return nil, wrapErr(i18n.T("store.ai.read"), err)
	}
	commits := make([]AIScanCommit, len(rows))
	for i, row := range rows {
		commits[i] = AIScanCommit{SHA: row.SHA, Message: row.Message, AuthorLogin: row.AuthorLogin}
		if row.AuthorEmails != "" {
			commits[i].AuthorEmails = strings.Split(row.AuthorEmails, "\n")
		}
	}
	return commits, nil
}

// WriteAIScans remplace, dans une même transaction, les signatures des commits analysés et note la
// version de règles utilisée.
func (s *Store) WriteAIScans(ctx context.Context, repoID, version int64, results []AIScanResult) error {
	err := s.writeTx(ctx, func(q *db.Queries) error {
		for _, r := range results {
			if err := q.DeleteCommitAISignatures(ctx, db.DeleteCommitAISignaturesParams{RepoID: repoID, SHA: r.SHA}); err != nil {
				return err
			}
			for _, sig := range r.Signatures {
				if err := q.InsertCommitAISignature(ctx, db.InsertCommitAISignatureParams{
					RepoID: repoID, SHA: r.SHA, Tool: sig.Tool, Kind: sig.Kind,
				}); err != nil {
					return err
				}
			}
			if err := q.SetCommitAIRulesVersion(ctx, db.SetCommitAIRulesVersionParams{
				RulesVersion: version, RepoID: repoID, SHA: r.SHA,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return wrapErr(i18n.T("store.ai.write"), err)
	}
	return nil
}
