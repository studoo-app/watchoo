package store

// Lectures et écriture d'une synchro de dépôt (branches, commits, commit_branches, contributors,
// contributor_emails, repositories, sync_runs).

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

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
		return nil, fmt.Errorf("branches du dépôt %d : %w", repoID, err)
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
		return 0, fmt.Errorf("nombre de commits du dépôt %d : %w", repoID, err)
	}
	return int(n), nil
}

// ExistingCommitSHAs renvoie l'ensemble des SHA de shas déjà enregistrés pour le dépôt repoID (map
// non nil ; SHA absents = nouveaux). Les doublons de shas sont permis. Les requêtes sont découpées par
// paquets pour rester sous la limite de paramètres de SQLite.
func (s *Store) ExistingCommitSHAs(ctx context.Context, repoID int64, shas []string) (map[string]bool, error) {
	known, err := existingCommitSHAs(ctx, s.queries, repoID, shas)
	if err != nil {
		return nil, fmt.Errorf("commits connus du dépôt %d : %w", repoID, err)
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

// ApplyRepoSync écrit une synchro réussie du dépôt w.RepoID dans UNE transaction (rien n'est écrit en
// cas d'erreur) :
//  1. repositories : owner, name, default_branch, url, stars, open_issues, archived (w.Metadata),
//     last_synced_at = w.SyncedAt ; violation d'unicité (owner/name pris par un autre dépôt) : erreur
//     enveloppant ErrAlreadyExists ;
//  2. branches : chaque w.Branches créée ou mise à jour (head_sha, tracked = 1) ; les autres branches
//     du dépôt passent à tracked = 0 ;
//  3. commits : chaque SHA absent de la base est inséré (INSERT … ON CONFLICT DO NOTHING : un commit
//     déjà en base n'est jamais modifié), avec author_id / committer_id résolus selon les règles
//     d'identité ci-dessous, les commits étant traités par CommittedAt croissant (puis SHA) pour que
//     le nom le plus récent l'emporte ;
//  4. commit_branches : une ligne par (commit, branche de CommitWrite.Branches), commits déjà connus
//     compris, sans doublon ; une branche absente de w.Branches est une erreur ;
//  5. last_commit_at = max(committed_at) des commits liés à une branche suivie (NULL si aucun) ;
//  6. pull requests (V2) : chaque w.PullRequests créée ou mise à jour en entier ; releases et tags
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
		return RepoSyncResult{}, fmt.Errorf("enregistrement de la synchro du dépôt %d : %w", w.RepoID, err)
	}

	var result RepoSyncResult
	err = s.writeTx(ctx, func(q *db.Queries) error {
		repo, err := q.GetRepository(ctx, w.RepoID)
		if err != nil {
			return repositoryLookupError(fmt.Sprintf("dépôt %d", w.RepoID), err)
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
			return fmt.Errorf("lecture des commits connus : %w", err)
		}

		ids := newIdentityResolver(q, repo.Provider)
		for _, c := range commits {
			if !known[c.SHA] {
				inserted, err := insertCommit(ctx, q, ids, w.RepoID, c)
				if err != nil {
					return err
				}
				result.NewCommits += inserted
			}
			for _, name := range c.Branches {
				if err := q.InsertCommitBranch(ctx, db.InsertCommitBranchParams{
					RepoID: w.RepoID, SHA: c.SHA, BranchID: branchIDs[name],
				}); err != nil {
					return fmt.Errorf("rattachement du commit %s à une branche : %w", shortSHA(c.SHA), err)
				}
			}
		}

		if err := writeExtras(ctx, q, w); err != nil {
			return err
		}

		last, err := q.MaxTrackedCommittedAt(ctx, w.RepoID)
		if err != nil {
			return fmt.Errorf("calcul du dernier commit : %w", err)
		}
		lastAt := sql.NullString{String: last, Valid: last != ""}
		if err := q.SetRepositoryLastCommitAt(ctx, db.SetRepositoryLastCommitAtParams{
			LastCommitAt: lastAt, ID: w.RepoID,
		}); err != nil {
			return fmt.Errorf("écriture du dernier commit : %w", err)
		}
		if result.LastCommitAt, err = parseNullTime(lastAt); err != nil {
			return fmt.Errorf("dernier commit : %w", err)
		}

		tracked, err := q.CountTrackedBranches(ctx, w.RepoID)
		if err != nil {
			return fmt.Errorf("nombre de branches suivies : %w", err)
		}
		result.TrackedBranches = int(tracked)

		n, err := q.FinishSyncRun(ctx, db.FinishSyncRunParams{
			Status:     string(model.SyncSuccess),
			FinishedAt: nullTime(w.SyncedAt),
			NewCommits: int64(result.NewCommits),
			ID:         w.RunID,
		})
		if err != nil {
			return fmt.Errorf("fin de synchro %d : %w", w.RunID, err)
		}
		if n == 0 {
			return fmt.Errorf("synchro en cours %d : %w", w.RunID, ErrNotFound)
		}
		return nil
	})
	if err != nil {
		return RepoSyncResult{}, fmt.Errorf("enregistrement de la synchro du dépôt %d : %w", w.RepoID, err)
	}
	return result, nil
}

// prepareRepoSync vérifie w avant toute écriture et renvoie ses commits réunis par SHA (branches
// fusionnées, sans doublon), triés par CommittedAt croissant puis SHA. Les messages d'erreur ne citent
// que des SHA courts et des noms de branches, jamais de donnée personnelle.
func prepareRepoSync(w RepoSyncWrite) ([]CommitWrite, error) {
	switch {
	case w.RepoID == 0:
		return nil, errors.New("dépôt non précisé")
	case w.RunID == 0:
		return nil, errors.New("synchro non précisée")
	case w.SyncedAt.IsZero():
		return nil, errors.New("date de fin de synchro absente")
	case w.Metadata.Owner == "" || w.Metadata.Name == "":
		return nil, errors.New("propriétaire ou nom du dépôt absent")
	}

	branches := make(map[string]bool, len(w.Branches))
	for _, b := range w.Branches {
		if b.Name == "" {
			return nil, errors.New("branche sans nom")
		}
		if branches[b.Name] {
			return nil, fmt.Errorf("branche %q en double", b.Name)
		}
		branches[b.Name] = true
	}

	index := make(map[string]int, len(w.Commits))
	merged := make([]CommitWrite, 0, len(w.Commits))
	for _, c := range w.Commits {
		if c.SHA == "" {
			return nil, errors.New("commit sans SHA")
		}
		if c.AuthoredAt.IsZero() || c.CommittedAt.IsZero() {
			return nil, fmt.Errorf("commit %s sans date", shortSHA(c.SHA))
		}
		for _, name := range c.Branches {
			if !branches[name] {
				return nil, fmt.Errorf("commit %s : branche %q absente des branches suivies", shortSHA(c.SHA), name)
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
			return fmt.Errorf("pull request de numéro %d invalide", pr.Number)
		case pr.State != model.PullRequestOpen && pr.State != model.PullRequestClosed:
			return fmt.Errorf("pull request n° %d : état %q invalide", pr.Number, pr.State)
		case pr.CreatedAt.IsZero() || pr.UpdatedAt.IsZero():
			return fmt.Errorf("pull request n° %d sans date", pr.Number)
		}
	}
	for _, r := range w.Releases {
		if r.ProviderID <= 0 || r.TagName == "" || r.CreatedAt.IsZero() {
			return fmt.Errorf("release %d invalide (identifiant, tag ou date absent)", r.ProviderID)
		}
	}
	for _, t := range w.Tags {
		if t.Name == "" || t.SHA == "" {
			return fmt.Errorf("tag sans nom ou sans SHA (%s)", shortSHA(t.SHA))
		}
	}
	return nil
}

// writeExtras écrit les pull requests (créées ou mises à jour), puis remplace releases et tags si
// demandé.
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
		}); err != nil {
			return fmt.Errorf("enregistrement de la pull request n° %d : %w", pr.Number, err)
		}
	}
	if w.ReplaceReleases {
		if err := q.DeleteReleases(ctx, w.RepoID); err != nil {
			return fmt.Errorf("remplacement des releases : %w", err)
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
				return fmt.Errorf("enregistrement de la release %d : %w", r.ProviderID, err)
			}
		}
	}
	if w.ReplaceTags {
		if err := q.DeleteTags(ctx, w.RepoID); err != nil {
			return fmt.Errorf("remplacement des tags : %w", err)
		}
		for _, t := range w.Tags {
			if err := q.InsertTag(ctx, db.InsertTagParams{RepoID: w.RepoID, Name: t.Name, SHA: t.SHA}); err != nil {
				return fmt.Errorf("enregistrement d'un tag (%s) : %w", shortSHA(t.SHA), err)
			}
		}
	}
	return nil
}

// writeRepoMetadata écrit les métadonnées du dépôt et last_synced_at.
func writeRepoMetadata(ctx context.Context, q *db.Queries, w RepoSyncWrite) error {
	m := w.Metadata
	n, err := q.UpdateRepositoryMetadata(ctx, db.UpdateRepositoryMetadataParams{
		Owner:         m.Owner,
		Name:          m.Name,
		DefaultBranch: m.DefaultBranch,
		URL:           m.URL,
		Stars:         int64(m.Stars),
		OpenIssues:    int64(m.OpenIssues),
		Archived:      m.Archived,
		LastSyncedAt:  nullTime(w.SyncedAt),
		ID:            w.RepoID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("dépôt %s/%s : %w", m.Owner, m.Name, ErrAlreadyExists)
		}
		return fmt.Errorf("métadonnées du dépôt : %w", err)
	}
	if n == 0 {
		return fmt.Errorf("dépôt %d : %w", w.RepoID, ErrNotFound)
	}
	return nil
}

// writeBranches passe toutes les branches du dépôt à tracked = 0, puis crée ou met à jour les branches
// suivies (tracked = 1), et renvoie leurs identifiants par nom.
func writeBranches(ctx context.Context, q *db.Queries, repoID int64, branches []BranchState) (map[string]int64, error) {
	if err := q.UntrackBranches(ctx, repoID); err != nil {
		return nil, fmt.Errorf("branches du dépôt : %w", err)
	}
	ids := make(map[string]int64, len(branches))
	for _, b := range branches {
		id, err := q.UpsertTrackedBranch(ctx, db.UpsertTrackedBranchParams{
			RepoID: repoID, Name: b.Name, HeadSHA: b.HeadSHA,
		})
		if err != nil {
			return nil, fmt.Errorf("branche %q : %w", b.Name, err)
		}
		ids[b.Name] = id
	}
	return ids, nil
}

// insertCommit résout l'auteur et le committer de c puis l'insère ; renvoie 1 si le commit a été
// inséré, 0 s'il existait déjà.
func insertCommit(ctx context.Context, q *db.Queries, ids *identityResolver, repoID int64, c CommitWrite) (int, error) {
	authorID, err := ids.resolve(ctx, c.Author)
	if err != nil {
		return 0, fmt.Errorf("auteur du commit %s : %w", shortSHA(c.SHA), err)
	}
	committerID, err := ids.resolve(ctx, c.Committer)
	if err != nil {
		return 0, fmt.Errorf("committer du commit %s : %w", shortSHA(c.SHA), err)
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
		URL:         c.URL,
	})
	if err != nil {
		return 0, fmt.Errorf("enregistrement du commit %s : %w", shortSHA(c.SHA), err)
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
		})
		if err != nil {
			return 0, fmt.Errorf("création d'un contributeur : %w", err)
		}
		row.ProviderLogin, row.Name, row.AvatarURL = id.Login, id.Name, id.AvatarURL
	case err != nil:
		return 0, fmt.Errorf("lecture d'un contributeur : %w", err)
	default:
		if err := r.updateProfile(ctx, row, cmp.Or(id.Login, row.ProviderLogin),
			cmp.Or(id.Name, row.Name), cmp.Or(id.AvatarURL, row.AvatarURL)); err != nil {
			return 0, err
		}
		row.ProviderLogin = cmp.Or(id.Login, row.ProviderLogin)
		row.Name = cmp.Or(id.Name, row.Name)
		row.AvatarURL = cmp.Or(id.AvatarURL, row.AvatarURL)
	}
	if id.Email == "" {
		return row.ID, nil
	}

	holder, err := r.q.GetContributorByEmail(ctx, id.Email)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Adresse inconnue : rattachée au compte.
	case err != nil:
		return 0, fmt.Errorf("lecture d'une adresse : %w", err)
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
		return 0, fmt.Errorf("rattachement d'une adresse : %w", err)
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
			return 0, fmt.Errorf("création d'un contributeur : %w", err)
		}
		if err := r.q.AttachContributorEmail(ctx, db.AttachContributorEmailParams{
			Email: id.Email, ContributorID: cid,
		}); err != nil {
			return 0, fmt.Errorf("rattachement d'une adresse : %w", err)
		}
		return cid, nil
	case err != nil:
		return 0, fmt.Errorf("lecture d'une adresse : %w", err)
	}
	// Le nom Git le plus récent l'emporte, mais seulement pour un contributeur sans compte : le nom d'un
	// compte vient des commits liés à ce compte.
	if !row.ProviderUserID.Valid && id.Name != "" {
		if err := r.updateProfile(ctx, row, row.ProviderLogin, id.Name, row.AvatarURL); err != nil {
			return 0, err
		}
	}
	return row.ID, nil
}

// updateProfile écrit login, nom et avatar du contributeur row s'ils ont changé.
func (r *identityResolver) updateProfile(ctx context.Context, row db.Contributor, login, name, avatar string) error {
	if row.ProviderLogin == login && row.Name == name && row.AvatarURL == avatar {
		return nil
	}
	if err := r.q.UpdateContributorProfile(ctx, db.UpdateContributorProfileParams{
		ProviderLogin: login, Name: name, AvatarURL: avatar, ID: row.ID,
	}); err != nil {
		return fmt.Errorf("mise à jour d'un contributeur : %w", err)
	}
	// Les identités retenues ne reflètent plus le dernier nom, login ou avatar écrit : sans cela, une
	// identité déjà vue après une autre qui l'a modifiée ne serait pas réappliquée.
	clear(r.seen)
	return nil
}

// merge fusionne le contributeur sans compte from dans le contributeur avec compte into : commits
// réattribués (auteur et committer), adresses déplacées, from supprimé. into reprend le nom de from
// s'il n'en a pas.
func (r *identityResolver) merge(ctx context.Context, from, into db.Contributor) error {
	toID := sql.NullInt64{Int64: into.ID, Valid: true}
	fromID := sql.NullInt64{Int64: from.ID, Valid: true}
	if err := r.q.ReassignCommitAuthors(ctx, db.ReassignCommitAuthorsParams{ToID: toID, FromID: fromID}); err != nil {
		return fmt.Errorf("fusion de contributeurs (auteurs) : %w", err)
	}
	if err := r.q.ReassignCommitCommitters(ctx, db.ReassignCommitCommittersParams{ToID: toID, FromID: fromID}); err != nil {
		return fmt.Errorf("fusion de contributeurs (committers) : %w", err)
	}
	if err := r.q.MoveContributorEmails(ctx, db.MoveContributorEmailsParams{ToID: into.ID, FromID: from.ID}); err != nil {
		return fmt.Errorf("fusion de contributeurs (adresses) : %w", err)
	}
	if into.Name == "" && from.Name != "" {
		if err := r.updateProfile(ctx, into, into.ProviderLogin, from.Name, into.AvatarURL); err != nil {
			return err
		}
	}
	if err := r.q.DeleteContributor(ctx, from.ID); err != nil {
		return fmt.Errorf("fusion de contributeurs (suppression) : %w", err)
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
