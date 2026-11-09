package store

// Lectures et écriture d'une synchro de dépôt (branches, commits, commit_branches, contributors,
// contributor_emails, repositories, sync_runs).

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// shaBatchSize est la taille des paquets de SHA d'une requête « IN (…) » : bien en dessous de la
// limite de paramètres de SQLite (32 766 par défaut).
const shaBatchSize = 500

// ListBranches renvoie toutes les branches connues du dépôt repoID (suivies ou non), triées par nom ;
// liste vide non nil si aucune.
func (s *Store) ListBranches(ctx context.Context, repoID int64) ([]model.Branch, error) {
	rows, err := s.queries.ListBranches(ctx, repoID)
	if err != nil {
		return nil, wrapErr(i18n.T("store.sync.branches", repoID), err)
	}
	branches := make([]model.Branch, 0, len(rows))
	for _, row := range rows {
		branches = append(branches, model.Branch{
			ID:      row.ID,
			RepoID:  row.RepoID,
			Name:    row.Name,
			HeadSHA: row.HeadSHA,
			Tracked: row.Tracked,
		})
	}
	return branches, nil
}

// CountCommits renvoie le nombre de commits en base du dépôt repoID (0 = premier import à venir).
func (s *Store) CountCommits(ctx context.Context, repoID int64) (int, error) {
	n, err := s.queries.CountCommits(ctx, repoID)
	if err != nil {
		return 0, wrapErr(i18n.T("store.sync.commit_count", repoID), err)
	}
	return int(n), nil
}

// ExistingCommitSHAs renvoie l'ensemble des SHA de shas déjà enregistrés pour le dépôt repoID (map
// non nil ; SHA absents = nouveaux). Les doublons de shas sont permis. Les requêtes sont découpées par
// paquets pour rester sous la limite de paramètres de SQLite.
func (s *Store) ExistingCommitSHAs(ctx context.Context, repoID int64, shas []string) (map[string]bool, error) {
	known, err := existingCommitSHAs(ctx, s.queries, repoID, shas)
	if err != nil {
		return nil, wrapErr(i18n.T("store.sync.known_commits", repoID), err)
	}
	return known, nil
}

// existingCommitSHAs est ExistingCommitSHAs sur q (connexion ou transaction).
func existingCommitSHAs(ctx context.Context, q *db.Queries, repoID int64, shas []string) (map[string]bool, error) {
	known := make(map[string]bool)
	unique := slices.Compact(slices.Sorted(slices.Values(shas)))
	for batch := range slices.Chunk(unique, shaBatchSize) {
		found, err := q.ExistingCommitSHAs(ctx, db.ExistingCommitSHAsParams{RepoID: repoID, Shas: batch})
		if err != nil {
			return nil, err
		}
		for _, sha := range found {
			known[sha] = true
		}
	}
	return known, nil
}

// UnknownParentSHAs renvoie, par ordre croissant, au plus limit SHA (limit > 0) des commits du dépôt
// repoID dont le nombre de parents est inconnu (parent_count NULL), strictement après after (vide =
// depuis le début) : pagination du rattrapage par le clone Git local (8.1).
func (s *Store) UnknownParentSHAs(ctx context.Context, repoID int64, after string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, errors.New(i18n.T("store.sync.no_parents_limit", repoID, limit))
	}
	shas, err := s.queries.ListUnknownParentSHAs(ctx, db.ListUnknownParentSHAsParams{
		RepoID: repoID, AfterSha: after, RowLimit: int64(limit),
	})
	if err != nil {
		return nil, wrapErr(i18n.T("store.sync.no_parents", repoID), err)
	}
	return shas, nil
}

// UnknownStatsSHAs renvoie, par ordre croissant, au plus limit SHA (limit > 0) des commits du dépôt
// repoID dont les statistiques sont inconnues (stats_known = 0) ou dont les langages ont été lus avec
// une version de règles inférieure à languagesVersion (0 : langages ignorés), strictement après after
// (vide = depuis le début) : pagination des commits à calculer par le clone Git local.
func (s *Store) UnknownStatsSHAs(ctx context.Context, repoID, languagesVersion int64, after string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, errors.New(i18n.T("store.sync.no_stats_limit", repoID, limit))
	}
	shas, err := s.queries.ListUnknownStatsSHAs(ctx, db.ListUnknownStatsSHAsParams{
		RepoID: repoID, LanguagesVersion: languagesVersion, AfterSha: after, RowLimit: int64(limit),
	})
	if err != nil {
		return nil, wrapErr(i18n.T("store.sync.no_stats", repoID), err)
	}
	return shas, nil
}

// ApplyRepoSync écrit une synchro réussie du dépôt w.RepoID dans UNE transaction (rien n'est écrit en
// cas d'erreur) :
//  1. repositories : owner, name, default_branch, url, stars, open_issues, archived (w.Metadata),
//     last_synced_at = w.SyncedAt ; violation d'unicité (owner/name pris par un autre dépôt) : erreur
//     enveloppant ErrAlreadyExists ;
//  2. branches : chaque w.Branches créée ou mise à jour (head_sha, tracked = 1) ; les autres branches
//     du dépôt passent à tracked = 0 ;
//  3. commits : chaque SHA absent de la base est inséré (INSERT … ON CONFLICT DO NOTHING : un commit
//     déjà en base n'est jamais modifié), parent_count compris (NULL si inconnu), avec author_id / committer_id résolus selon les règles
//     d'identité ci-dessous, les commits étant traités par CommittedAt croissant (puis SHA) pour que
//     le nom le plus récent l'emporte ;
//  4. commit_branches : une ligne par (commit, branche de CommitWrite.Branches), commits déjà connus
//     compris, sans doublon ; une branche absente de w.Branches est une erreur ; puis statistiques
//     w.Stats (additions, deletions, stats_known = 1) des commits en base dont elles sont inconnues,
//     SHA inconnus ignorés ;
//     langages (commit_languages remplacées, languages_version = w.LanguagesVersion) des nouveaux
//     commits et des w.Stats qui en ont (1.2.0) ; puis nombre de parents w.ParentCounts (8.1) des
//     commits en base dont il est inconnu, SHA inconnus ignorés ;
//  5. last_commit_at = max(committed_at) des commits liés à une branche suivie (NULL si aucun) ;
//  6. pull requests (V2) : chaque w.PullRequests créée ou mise à jour en entier ; puis (phase 8.5)
//     merge_commit_sha, from_fork et body_sections de chaque w.PullRequestConventions déjà en base, et
//     repositories.conventions_prs_version = w.PullRequestsVersion s'il est positif ; releases et tags
//     remplacés par w.Releases et w.Tags si w.ReplaceReleases / w.ReplaceTags (doublons ignorés) ;
//  7. sync_run w.RunID clos : status « success », finished_at = w.SyncedAt, new_commits.
//
// Règles d'identité (auteur et committer) :
//   - ProviderUserID != 0 : contributeur (provider du dépôt, provider_user_id) créé ou mis à jour
//     (provider_login, avatar_url, et name si Identity.Name n'est pas vide). Son e-mail (minuscules,
//     s'il n'est pas vide) lui est rattaché ; si l'adresse appartenait à un contributeur SANS compte,
//     celui-ci est fusionné dans le contributeur avec compte (commits réattribués en author_id et
//     committer_id, autres e-mails déplacés, ancien contributeur supprimé) ; si elle appartenait à un
//     autre contributeur AVEC compte, elle est simplement réattribuée ;
//   - ProviderUserID == 0 et e-mail non vide : contributeur trouvé par e-mail (minuscules), sinon créé
//     sans compte (provider et provider_user_id NULL, name = nom Git) avec cet e-mail ;
//   - ProviderUserID == 0 et e-mail vide : aucun contributeur (author_id / committer_id NULL), le nom
//     Git est ignoré (aucune identité fiable pour regrouper ces commits).
//
// Précisions : un login ou un avatar vide ne remplace pas la valeur connue ; le nom Git d'un commit sans
// compte met à jour le nom d'un contributeur SANS compte trouvé par e-mail, jamais celui d'un
// contributeur avec compte ; lors d'une fusion, le contributeur avec compte reprend le nom de l'absorbé
// s'il n'en a pas. Seuls les commits NOUVEAUX passent par ces règles.
func (s *Store) ApplyRepoSync(ctx context.Context, w RepoSyncWrite) (RepoSyncResult, error) {
	commits, err := prepareRepoSync(w)
	if err != nil {
		return RepoSyncResult{}, wrapErr(i18n.T("store.sync.record", w.RepoID), err)
	}

	var result RepoSyncResult
	err = s.writeTx(ctx, func(q *db.Queries) error {
		repo, err := q.GetRepository(ctx, w.RepoID)
		if err != nil {
			return repositoryLookupError(i18n.T("store.repo.id", w.RepoID), err)
		}
		if err := writeRepoMetadata(ctx, q, w); err != nil {
			return err
		}
		branchIDs, err := writeBranches(ctx, q, w.RepoID, w.Branches)
		if err != nil {
			return err
		}

		shas := make([]string, len(commits))
		for i, c := range commits {
			shas[i] = c.SHA
		}
		known, err := existingCommitSHAs(ctx, q, w.RepoID, shas)
		if err != nil {
			return wrapErr(i18n.T("store.sync.read_known"), err)
		}

		ids := newIdentityResolver(q, repo.Provider)
		for _, c := range commits {
			if !known[c.SHA] {
				inserted, err := insertCommit(ctx, q, ids, w.RepoID, c)
				if err != nil {
					return err
				}
				result.NewCommits += inserted
				if inserted > 0 && c.Languages != nil {
					if err := writeCommitLanguages(ctx, q, w.RepoID, w.LanguagesVersion, c.SHA, c.Languages); err != nil {
						return err
					}
				}
			}
			for _, name := range c.Branches {
				if err := q.InsertCommitBranch(ctx, db.InsertCommitBranchParams{
					RepoID: w.RepoID, SHA: c.SHA, BranchID: branchIDs[name],
				}); err != nil {
					return wrapErr(i18n.T("store.sync.link_branch", shortSHA(c.SHA)), err)
				}
			}
		}

		for _, st := range w.Stats {
			if _, err := q.SetCommitStats(ctx, db.SetCommitStatsParams{
				Additions: int64(st.Additions), Deletions: int64(st.Deletions), RepoID: w.RepoID, SHA: st.SHA,
			}); err != nil {
				return wrapErr(i18n.T("store.sync.commit_stats", shortSHA(st.SHA)), err)
			}
			if st.Languages != nil {
				if err := writeCommitLanguages(ctx, q, w.RepoID, w.LanguagesVersion, st.SHA, st.Languages); err != nil {
					return err
				}
			}
		}

		for _, pc := range w.ParentCounts {
			if _, err := q.SetCommitParentCount(ctx, db.SetCommitParentCountParams{
				ParentCount: sql.NullInt64{Int64: int64(pc.Parents), Valid: true}, RepoID: w.RepoID, SHA: pc.SHA,
			}); err != nil {
				return wrapErr(i18n.T("store.sync.commit_parents", shortSHA(pc.SHA)), err)
			}
		}

		if err := writeExtras(ctx, q, w); err != nil {
			return err
		}

		last, err := q.MaxTrackedCommittedAt(ctx, w.RepoID)
		if err != nil {
			return wrapErr(i18n.T("store.sync.compute_last"), err)
		}
		lastAt := sql.NullString{String: last, Valid: last != ""}
		if err := q.SetRepositoryLastCommitAt(ctx, db.SetRepositoryLastCommitAtParams{
			LastCommitAt: lastAt, ID: w.RepoID,
		}); err != nil {
			return wrapErr(i18n.T("store.sync.write_last"), err)
		}
		if result.LastCommitAt, err = parseNullTime(lastAt); err != nil {
			return wrapErr(i18n.T("store.last_commit"), err)
		}

		tracked, err := q.CountTrackedBranches(ctx, w.RepoID)
		if err != nil {
			return wrapErr(i18n.T("store.tracked_branches"), err)
		}
		result.TrackedBranches = int(tracked)

		n, err := q.FinishSyncRun(ctx, db.FinishSyncRunParams{
			Status:     string(model.SyncSuccess),
			FinishedAt: nullTime(w.SyncedAt),
			NewCommits: int64(result.NewCommits),
			ID:         w.RunID,
		})
		if err != nil {
			return wrapErr(i18n.T("store.runs.finish", w.RunID), err)
		}
		if n == 0 {
			return wrapErr(i18n.T("store.runs.running", w.RunID), ErrNotFound)
		}
		return nil
	})
	if err != nil {
		return RepoSyncResult{}, wrapErr(i18n.T("store.sync.record", w.RepoID), err)
	}
	return result, nil
}

// prepareRepoSync vérifie w avant toute écriture et renvoie ses commits réunis par SHA (branches
// fusionnées, sans doublon), triés par CommittedAt croissant puis SHA. Les messages d'erreur ne citent
// que des SHA courts et des noms de branches, jamais de donnée personnelle.
func prepareRepoSync(w RepoSyncWrite) ([]CommitWrite, error) {
	switch {
	case w.RepoID == 0:
		return nil, errors.New(i18n.T("store.sync.no_repo"))
	case w.RunID == 0:
		return nil, errors.New(i18n.T("store.sync.no_run"))
	case w.SyncedAt.IsZero():
		return nil, errors.New(i18n.T("store.sync.no_finished_at"))
	case !w.Metadata.Partial && (w.Metadata.Owner == "" || w.Metadata.Name == ""):
		return nil, errors.New(i18n.T("store.sync.no_owner_name"))
	case w.ObserveTags && !w.ReplaceTags:
		return nil, errors.New(i18n.T("store.sync.tags_observe_unread"))
	}

	branches := make(map[string]bool, len(w.Branches))
	for _, b := range w.Branches {
		if b.Name == "" {
			return nil, errors.New(i18n.T("store.sync.branch_no_name"))
		}
		if branches[b.Name] {
			return nil, errors.New(i18n.T("store.sync.branch_duplicate", b.Name))
		}
		branches[b.Name] = true
	}

	index := make(map[string]int, len(w.Commits))
	merged := make([]CommitWrite, 0, len(w.Commits))
	for _, c := range w.Commits {
		if c.SHA == "" {
			return nil, errors.New(i18n.T("store.sync.commit_no_sha"))
		}
		if c.AuthoredAt.IsZero() || c.CommittedAt.IsZero() {
			return nil, errors.New(i18n.T("store.sync.commit_no_date", shortSHA(c.SHA)))
		}
		if c.ParentCount != nil && *c.ParentCount < 0 {
			return nil, errors.New(i18n.T("store.sync.parents_negative", shortSHA(c.SHA)))
		}
		for _, name := range c.Branches {
			if !branches[name] {
				return nil, errors.New(i18n.T("store.sync.commit_branch", shortSHA(c.SHA), name))
			}
		}
		if i, ok := index[c.SHA]; ok {
			merged[i].Branches = append(merged[i].Branches, c.Branches...)
			continue
		}
		index[c.SHA] = len(merged)
		c.Branches = slices.Clone(c.Branches)
		merged = append(merged, c)
	}
	languages := false
	for _, c := range merged {
		languages = languages || c.Languages != nil
	}
	for _, st := range w.Stats {
		languages = languages || st.Languages != nil
		switch {
		case st.SHA == "":
			return nil, errors.New(i18n.T("store.sync.stats_no_sha"))
		case st.Additions < 0 || st.Deletions < 0:
			return nil, errors.New(i18n.T("store.sync.stats_negative", shortSHA(st.SHA)))
		}
	}
	for _, pc := range w.ParentCounts {
		switch {
		case pc.SHA == "":
			return nil, errors.New(i18n.T("store.sync.parents_no_sha"))
		case pc.Parents < 0:
			return nil, errors.New(i18n.T("store.sync.parents_negative", shortSHA(pc.SHA)))
		}
	}
	if languages && w.LanguagesVersion <= 0 {
		return nil, errors.New(i18n.T("store.sync.no_languages_version"))
	}
	if err := checkExtras(w); err != nil {
		return nil, err
	}
	for i := range merged {
		slices.Sort(merged[i].Branches)
		merged[i].Branches = slices.Compact(merged[i].Branches)
	}
	slices.SortFunc(merged, func(a, b CommitWrite) int {
		return cmp.Or(a.CommittedAt.Compare(b.CommittedAt), strings.Compare(a.SHA, b.SHA))
	})
	return merged, nil
}

// checkExtras vérifie les pull requests, releases et tags de w (numéros, états, identifiants, dates).
// Les messages ne citent que des numéros et des identifiants, jamais un titre ni un login.
func checkExtras(w RepoSyncWrite) error {
	for _, pr := range w.PullRequests {
		switch {
		case pr.Number <= 0:
			return errors.New(i18n.T("store.sync.pr_number", pr.Number))
		case pr.State != model.PullRequestOpen && pr.State != model.PullRequestClosed:
			return errors.New(i18n.T("store.sync.pr_state", pr.Number, pr.State))
		case pr.CreatedAt.IsZero() || pr.UpdatedAt.IsZero():
			return errors.New(i18n.T("store.sync.pr_no_date", pr.Number))
		case !validMergeSHA(pr.MergeCommitSHA) || (pr.BodySections != nil && *pr.BodySections < 0):
			return errors.New(i18n.T("store.sync.pr_conventions", pr.Number))
		}
	}
	for _, pr := range w.PullRequestConventions {
		switch {
		case pr.Number <= 0:
			return errors.New(i18n.T("store.sync.pr_number", pr.Number))
		case !validMergeSHA(pr.MergeCommitSHA) || (pr.BodySections != nil && *pr.BodySections < 0):
			return errors.New(i18n.T("store.sync.pr_conventions", pr.Number))
		}
	}
	if w.PullRequestsVersion < 0 {
		return errors.New(i18n.T("store.sync.prs_version", w.PullRequestsVersion))
	}
	for _, r := range w.Releases {
		if r.ProviderID <= 0 || r.TagName == "" || r.CreatedAt.IsZero() {
			return errors.New(i18n.T("store.sync.release_invalid", r.ProviderID))
		}
	}
	for _, t := range w.Tags {
		if t.Name == "" || t.SHA == "" {
			return errors.New(i18n.T("store.sync.tag_invalid", shortSHA(t.SHA)))
		}
	}
	for _, l := range w.Languages {
		if l.Language == "" || l.Bytes <= 0 {
			return errors.New(i18n.T("store.sync.language_invalid", l.Language))
		}
	}
	return nil
}

// validMergeSHA indique un commit de fusion vide (inconnu) ou de la forme stockée : 40 ou 64 chiffres
// hexadécimaux minuscules (contrainte CHECK de pull_requests.merge_commit_sha).
func validMergeSHA(sha string) bool {
	if sha == "" {
		return true
	}
	if len(sha) != 40 && len(sha) != 64 {
		return false
	}
	for _, c := range sha {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// writeExtras écrit les pull requests (créées ou mises à jour), puis remplace releases, tags et
// langages si demandé.
func writeExtras(ctx context.Context, q *db.Queries, w RepoSyncWrite) error {
	for _, pr := range w.PullRequests {
		if err := q.UpsertPullRequest(ctx, db.UpsertPullRequestParams{
			RepoID:           w.RepoID,
			Number:           int64(pr.Number),
			Title:            pr.Title,
			State:            pr.State,
			Merged:           pr.Merged,
			Draft:            pr.Draft,
			AuthorLogin:      pr.AuthorLogin,
			AuthorProviderID: pr.AuthorProviderID,
			URL:              pr.URL,
			BaseBranch:       pr.BaseBranch,
			HeadBranch:       pr.HeadBranch,
			CreatedAt:        FormatTime(pr.CreatedAt),
			UpdatedAt:        FormatTime(pr.UpdatedAt),
			ClosedAt:         nullTime(pr.ClosedAt),
			MergedAt:         nullTime(pr.MergedAt),
			ToolIds:          strings.Join(pr.ToolIDs, ","),
			MergeCommitSha:   nullSHA(pr.MergeCommitSHA),
			FromFork:         nullBool(pr.FromFork),
			BodySections:     nullInt64(pr.BodySections),
		}); err != nil {
			return wrapErr(i18n.T("store.sync.pr_record", pr.Number), err)
		}
	}
	for _, pr := range w.PullRequestConventions {
		if _, err := q.SetPullRequestConventions(ctx, db.SetPullRequestConventionsParams{
			MergeCommitSha: nullSHA(pr.MergeCommitSHA),
			FromFork:       nullBool(pr.FromFork),
			BodySections:   nullInt64(pr.BodySections),
			RepoID:         w.RepoID,
			Number:         int64(pr.Number),
		}); err != nil {
			return wrapErr(i18n.T("store.sync.pr_record", pr.Number), err)
		}
	}
	if w.PullRequestsVersion > 0 {
		if _, err := q.SetRepositoryConventionsPRsVersion(ctx, db.SetRepositoryConventionsPRsVersionParams{
			ConventionsPrsVersion: sql.NullInt64{Int64: w.PullRequestsVersion, Valid: true}, ID: w.RepoID,
		}); err != nil {
			return wrapErr(i18n.T("store.sync.prs_version_record"), err)
		}
	}
	if w.ReplaceReleases {
		if err := q.DeleteReleases(ctx, w.RepoID); err != nil {
			return wrapErr(i18n.T("store.sync.releases_replace"), err)
		}
		for _, r := range w.Releases {
			if err := q.InsertRelease(ctx, db.InsertReleaseParams{
				RepoID:      w.RepoID,
				ProviderID:  r.ProviderID,
				TagName:     r.TagName,
				Name:        r.Name,
				Draft:       r.Draft,
				Prerelease:  r.Prerelease,
				AuthorLogin: r.AuthorLogin,
				URL:         r.URL,
				CreatedAt:   FormatTime(r.CreatedAt),
				PublishedAt: nullTime(r.PublishedAt),
			}); err != nil {
				return wrapErr(i18n.T("store.sync.release_record", r.ProviderID), err)
			}
		}
	}
	if w.ReplaceTags {
		if err := q.DeleteTags(ctx, w.RepoID); err != nil {
			return wrapErr(i18n.T("store.sync.tags_replace"), err)
		}
		for _, t := range w.Tags {
			if err := q.InsertTag(ctx, db.InsertTagParams{RepoID: w.RepoID, Name: t.Name, SHA: t.SHA}); err != nil {
				return wrapErr(i18n.T("store.sync.tag_record", shortSHA(t.SHA)), err)
			}
		}
		if w.ObserveTags {
			if err := observeTags(ctx, q, w.RepoID, w.Tags, w.SyncedAt); err != nil {
				return err
			}
		}
	}
	for _, r := range w.BranchRewrites {
		if err := insertBranchRewrite(ctx, q, w.RepoID, r, w.SyncedAt); err != nil {
			return err
		}
	}
	if w.ReplaceLanguages {
		if err := q.DeleteRepositoryLanguages(ctx, w.RepoID); err != nil {
			return wrapErr(i18n.T("store.sync.languages_replace"), err)
		}
		for _, l := range w.Languages {
			if err := q.InsertRepositoryLanguage(ctx, db.InsertRepositoryLanguageParams{RepoID: w.RepoID,
				Language: l.Language, Bytes: l.Bytes}); err != nil {
				return wrapErr(i18n.T("store.sync.language_record", l.Language), err)
			}
		}
	}
	return nil
}

// writeRepoMetadata écrit les métadonnées du dépôt et last_synced_at.
func writeRepoMetadata(ctx context.Context, q *db.Queries, w RepoSyncWrite) error {
	m := w.Metadata
	if m.Partial {
		n, err := q.UpdateRepositoryLocalMetadata(ctx, db.UpdateRepositoryLocalMetadataParams{
			DefaultBranch: sql.NullString{String: m.DefaultBranch, Valid: m.DefaultBranch != ""},
			LastSyncedAt:  nullTime(w.SyncedAt),
			ID:            w.RepoID,
		})
		if err != nil {
			return wrapErr(i18n.T("store.sync.metadata"), err)
		}
		if n == 0 {
			return wrapErr(i18n.T("store.repo.id", w.RepoID), ErrNotFound)
		}
		return nil
	}
	n, err := q.UpdateRepositoryMetadata(ctx, db.UpdateRepositoryMetadataParams{
		Owner:            m.Owner,
		Name:             m.Name,
		DefaultBranch:    m.DefaultBranch,
		URL:              m.URL,
		Stars:            int64(m.Stars),
		OpenIssues:       int64(m.OpenIssues),
		Archived:         m.Archived,
		LastSyncedAt:     nullTime(w.SyncedAt),
		ProviderID:       nullID(m.ProviderID),
		AllowSquashMerge: nullBool(m.AllowSquashMerge),
		AllowMergeCommit: nullBool(m.AllowMergeCommit),
		AllowRebaseMerge: nullBool(m.AllowRebaseMerge),
		ID:               w.RepoID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return wrapErr(i18n.T("store.repo.full_name", m.Owner, m.Name), ErrAlreadyExists)
		}
		return wrapErr(i18n.T("store.sync.metadata"), err)
	}
	if n == 0 {
		return wrapErr(i18n.T("store.repo.id", w.RepoID), ErrNotFound)
	}
	return nil
}

// writeBranches passe toutes les branches du dépôt à tracked = 0, puis crée ou met à jour les branches
// suivies (tracked = 1), et renvoie leurs identifiants par nom.
func writeBranches(ctx context.Context, q *db.Queries, repoID int64, branches []BranchState) (map[string]int64, error) {
	if err := q.UntrackBranches(ctx, repoID); err != nil {
		return nil, wrapErr(i18n.T("store.sync.repo_branches"), err)
	}
	ids := make(map[string]int64, len(branches))
	for _, b := range branches {
		id, err := q.UpsertTrackedBranch(ctx, db.UpsertTrackedBranchParams{
			RepoID: repoID, Name: b.Name, HeadSHA: b.HeadSHA,
		})
		if err != nil {
			return nil, wrapErr(i18n.T("store.sync.branch", b.Name), err)
		}
		ids[b.Name] = id
	}
	return ids, nil
}

// writeCommitLanguages note la version de règles avec laquelle les langages du commit sha ont été lus et
// les remplace (langages sans ligne ignorés) ; un commit absent de la base est ignoré.
func writeCommitLanguages(ctx context.Context, q *db.Queries, repoID, version int64, sha string, langs map[string]int) error {
	n, err := q.SetCommitLanguagesVersion(ctx, db.SetCommitLanguagesVersionParams{
		LanguagesVersion: version, RepoID: repoID, SHA: sha,
	})
	if err != nil {
		return wrapErr(i18n.T("store.sync.commit_languages", shortSHA(sha)), err)
	}
	if n == 0 {
		return nil
	}
	if err := q.DeleteCommitLanguages(ctx, db.DeleteCommitLanguagesParams{RepoID: repoID, SHA: sha}); err != nil {
		return wrapErr(i18n.T("store.sync.commit_languages", shortSHA(sha)), err)
	}
	for _, l := range model.SortedLanguageLines(langs) {
		if err := q.InsertCommitLanguage(ctx, db.InsertCommitLanguageParams{
			RepoID: repoID, SHA: sha, Language: l.Language, Additions: int64(l.Lines),
		}); err != nil {
			return wrapErr(i18n.T("store.sync.commit_languages", shortSHA(sha)), err)
		}
	}
	return nil
}

// insertCommit résout l'auteur et le committer de c puis l'insère ; renvoie 1 si le commit a été
// inséré, 0 s'il existait déjà.
func insertCommit(ctx context.Context, q *db.Queries, ids *identityResolver, repoID int64, c CommitWrite) (int, error) {
	authorID, err := ids.resolve(ctx, c.Author)
	if err != nil {
		return 0, wrapErr(i18n.T("store.sync.author", shortSHA(c.SHA)), err)
	}
	committerID, err := ids.resolve(ctx, c.Committer)
	if err != nil {
		return 0, wrapErr(i18n.T("store.sync.committer", shortSHA(c.SHA)), err)
	}
	n, err := q.InsertCommit(ctx, db.InsertCommitParams{
		RepoID:      repoID,
		SHA:         c.SHA,
		AuthorID:    authorID,
		CommitterID: committerID,
		Message:     c.Message,
		AuthoredAt:  FormatTime(c.AuthoredAt),
		CommittedAt: FormatTime(c.CommittedAt),
		Additions:   int64(c.Additions),
		Deletions:   int64(c.Deletions),
		StatsKnown:  c.StatsKnown,
		URL:         c.URL,
		ParentCount: nullInt(c.ParentCount),
	})
	if err != nil {
		return 0, wrapErr(i18n.T("store.sync.commit_record", shortSHA(c.SHA)), err)
	}
	return int(n), nil
}

// identityResolver applique les règles d'identité d'ApplyRepoSync dans une transaction. Il retient les
// identités déjà traitées pour ne pas refaire les mêmes requêtes à chaque commit d'une même personne.
type identityResolver struct {
	q        *db.Queries
	provider string
	// seen associe une identité (tous ses champs) au contributeur résolu ; vidé à chaque fusion, qui
	// supprime un contributeur.
	seen map[Identity]int64
}

func newIdentityResolver(q *db.Queries, provider string) *identityResolver {
	return &identityResolver{q: q, provider: provider, seen: make(map[Identity]int64)}
}

// resolve renvoie le contributeur de id (NULL si ni compte ni e-mail), en le créant, le mettant à jour
// ou en fusionnant au besoin.
func (r *identityResolver) resolve(ctx context.Context, id Identity) (sql.NullInt64, error) {
	id.Email = normalizeEmail(id.Email)
	if id.ProviderUserID == 0 && id.Email == "" {
		return sql.NullInt64{}, nil
	}
	if cid, ok := r.seen[id]; ok {
		return sql.NullInt64{Int64: cid, Valid: true}, nil
	}
	var cid int64
	var err error
	if id.ProviderUserID != 0 {
		cid, err = r.resolveAccount(ctx, id)
	} else {
		cid, err = r.resolveEmail(ctx, id)
	}
	if err != nil {
		return sql.NullInt64{}, err
	}
	r.seen[id] = cid
	return sql.NullInt64{Int64: cid, Valid: true}, nil
}

// resolveAccount traite une identité liée à un compte de forge.
func (r *identityResolver) resolveAccount(ctx context.Context, id Identity) (int64, error) {
	row, err := r.q.GetContributorByAccount(ctx, db.GetContributorByAccountParams{
		Provider:       sql.NullString{String: r.provider, Valid: true},
		ProviderUserID: sql.NullInt64{Int64: id.ProviderUserID, Valid: true},
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		row.ID, err = r.q.CreateContributor(ctx, db.CreateContributorParams{
			Provider:       sql.NullString{String: r.provider, Valid: true},
			ProviderUserID: sql.NullInt64{Int64: id.ProviderUserID, Valid: true},
			ProviderLogin:  id.Login,
			Name:           id.Name,
			AvatarURL:      id.AvatarURL,
			ProfileURL:     id.ProfileURL,
		})
		if err != nil {
			return 0, wrapErr(i18n.T("store.contrib.create"), err)
		}
		row.ProviderLogin, row.Name, row.AvatarURL, row.ProfileURL = id.Login, id.Name, id.AvatarURL, id.ProfileURL
	case err != nil:
		return 0, wrapErr(i18n.T("store.contrib.read"), err)
	default:
		next := profile{login: cmp.Or(id.Login, row.ProviderLogin), name: cmp.Or(id.Name, row.Name),
			avatar: cmp.Or(id.AvatarURL, row.AvatarURL), url: cmp.Or(id.ProfileURL, row.ProfileURL)}
		if err := r.updateProfile(ctx, row, next); err != nil {
			return 0, err
		}
		row.ProviderLogin, row.Name, row.AvatarURL, row.ProfileURL = next.login, next.name, next.avatar, next.url
	}
	if id.Email == "" {
		return row.ID, nil
	}

	holder, err := r.q.GetContributorByEmail(ctx, id.Email)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Adresse inconnue : rattachée au compte.
	case err != nil:
		return 0, wrapErr(i18n.T("store.contrib.read_email"), err)
	case holder.ID == row.ID:
		return row.ID, nil
	case !holder.ProviderUserID.Valid:
		// Contributeur sans compte : fusionné dans le compte (ses adresses suivent, celle-ci comprise).
		if err := r.merge(ctx, holder, row); err != nil {
			return 0, err
		}
		return row.ID, nil
	default:
		// Adresse tenue par un autre compte : des identités retenues la rattachent encore à celui-ci.
		clear(r.seen)
	}
	// Adresse inconnue, ou tenue par un autre compte : (ré)attribuée à ce compte.
	if err := r.q.AttachContributorEmail(ctx, db.AttachContributorEmailParams{
		Email: id.Email, ContributorID: row.ID,
	}); err != nil {
		return 0, wrapErr(i18n.T("store.contrib.link_email"), err)
	}
	return row.ID, nil
}

// resolveEmail traite une identité sans compte, avec un e-mail (déjà en minuscules).
func (r *identityResolver) resolveEmail(ctx context.Context, id Identity) (int64, error) {
	row, err := r.q.GetContributorByEmail(ctx, id.Email)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		cid, err := r.q.CreateContributor(ctx, db.CreateContributorParams{Name: id.Name})
		if err != nil {
			return 0, wrapErr(i18n.T("store.contrib.create"), err)
		}
		if err := r.q.AttachContributorEmail(ctx, db.AttachContributorEmailParams{
			Email: id.Email, ContributorID: cid,
		}); err != nil {
			return 0, wrapErr(i18n.T("store.contrib.link_email"), err)
		}
		return cid, nil
	case err != nil:
		return 0, wrapErr(i18n.T("store.contrib.read_email"), err)
	}
	// Le nom Git le plus récent l'emporte, mais seulement pour un contributeur sans compte : le nom d'un
	// compte vient des commits liés à ce compte.
	if !row.ProviderUserID.Valid && id.Name != "" {
		next := profileOf(row)
		next.name = id.Name
		if err := r.updateProfile(ctx, row, next); err != nil {
			return 0, err
		}
	}
	return row.ID, nil
}

// profile regroupe les champs d'un contributeur que met à jour updateProfile.
type profile struct {
	login, name, avatar, url string
}

// profileOf renvoie les champs actuels du contributeur row.
func profileOf(row db.Contributor) profile {
	return profile{login: row.ProviderLogin, name: row.Name, avatar: row.AvatarURL, url: row.ProfileURL}
}

// updateProfile écrit login, nom, avatar et page de profil du contributeur row s'ils ont changé.
func (r *identityResolver) updateProfile(ctx context.Context, row db.Contributor, p profile) error {
	if profileOf(row) == p {
		return nil
	}
	if err := r.q.UpdateContributorProfile(ctx, db.UpdateContributorProfileParams{
		ProviderLogin: p.login, Name: p.name, AvatarURL: p.avatar, ProfileURL: p.url, ID: row.ID,
	}); err != nil {
		return wrapErr(i18n.T("store.contrib.update"), err)
	}
	// Les identités retenues ne reflètent plus le dernier nom, login ou avatar écrit : sans cela, une
	// identité déjà vue après une autre qui l'a modifiée ne serait pas réappliquée.
	clear(r.seen)
	return nil
}

// merge fusionne le contributeur sans compte from dans le contributeur avec compte into : commits
// réattribués (auteur et committer), signaux de diagnostic aussi, adresses déplacées, from supprimé. into reprend le nom de from
// s'il n'en a pas.
func (r *identityResolver) merge(ctx context.Context, from, into db.Contributor) error {
	toID := sql.NullInt64{Int64: into.ID, Valid: true}
	fromID := sql.NullInt64{Int64: from.ID, Valid: true}
	if err := r.q.ReassignCommitAuthors(ctx, db.ReassignCommitAuthorsParams{ToID: toID, FromID: fromID}); err != nil {
		return wrapErr(i18n.T("store.contrib.merge_authors"), err)
	}
	if err := r.q.ReassignCommitCommitters(ctx, db.ReassignCommitCommittersParams{ToID: toID, FromID: fromID}); err != nil {
		return wrapErr(i18n.T("store.contrib.merge_committers"), err)
	}
	if err := r.q.ReassignSignalAuthors(ctx, db.ReassignSignalAuthorsParams{ToID: toID, FromID: fromID}); err != nil {
		return wrapErr(i18n.T("store.contrib.merge_authors"), err)
	}
	if err := r.q.MoveContributorEmails(ctx, db.MoveContributorEmailsParams{ToID: into.ID, FromID: from.ID}); err != nil {
		return wrapErr(i18n.T("store.contrib.merge_emails"), err)
	}
	if into.Name == "" && from.Name != "" {
		next := profileOf(into)
		next.name = from.Name
		if err := r.updateProfile(ctx, into, next); err != nil {
			return err
		}
	}
	if err := r.q.DeleteContributor(ctx, from.ID); err != nil {
		return wrapErr(i18n.T("store.contrib.merge_delete"), err)
	}
	// Des identités retenues peuvent désigner le contributeur supprimé.
	clear(r.seen)
	return nil
}

// normalizeEmail met une adresse e-mail Git sous sa forme enregistrée : sans espaces autour, en
// minuscules.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// shortSHA renvoie les 7 premiers caractères d'un SHA (messages d'erreur et logs).
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// BackfillProfileURLs complète la page de profil des contributeurs de la forge provider dont le login
// est connu mais l'adresse vide (contributeurs enregistrés avant la migration 00005, que la synchro
// incrémentale ne revoit pas) : profile_url = build(login), sauf si build renvoie une chaîne vide. Les
// adresses déjà connues et les contributeurs sans login ne sont pas touchés. Une transaction ; renvoie
// le nombre de contributeurs complétés. Le login n'apparaît dans aucune erreur.
func (s *Store) BackfillProfileURLs(ctx context.Context, provider string, build func(login string) string) (int, error) {
	if build == nil {
		return 0, nil
	}
	filled := 0
	err := s.writeTx(ctx, func(q *db.Queries) error {
		rows, err := q.ListContributorsWithoutProfileURL(ctx, sql.NullString{String: provider, Valid: true})
		if err != nil {
			return wrapErr(i18n.T("store.contrib.no_profile"), err)
		}
		for _, row := range rows {
			url := build(row.ProviderLogin)
			if url == "" {
				continue
			}
			n, err := q.SetContributorProfileURL(ctx, db.SetContributorProfileURLParams{ProfileURL: url, ID: row.ID})
			if err != nil {
				return wrapErr(i18n.T("store.contrib.profile", row.ID), err)
			}
			filled += int(n)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return filled, nil
}
