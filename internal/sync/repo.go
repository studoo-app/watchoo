package sync

// Synchronisation incrémentale d'un dépôt (documentation/03-architecture.md, « Synchronisation
// incrémentale ») : métadonnées, branches, commits, statistiques, puis une seule écriture en base.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/store"
)

// repoOutcome est le résultat d'une synchro de dépôt réussie.
type repoOutcome struct {
	fullName    string // owner/name actuels
	displayName string // label, sinon owner/name actuels
	result      store.RepoSyncResult
	stats       int // appels CommitStats faits
	pulls       int // pull requests lues (créées ou mises à jour)
	// warnings : listes (pull requests, releases, tags) non synchronisées, en français.
	warnings []string
}

// repoSync accumule les commits vus pendant la synchro d'un dépôt.
type repoSync struct {
	e       *Engine
	prov    provider.Provider
	owner   string // owner actuel (après RepositoryByID)
	name    string // nom actuel
	full    string // owner/name actuels, pour les logs
	commits []store.CommitWrite
	index   map[string]int // SHA → indice dans commits
	skipped int            // commits ignorés faute de date
}

// syncRepository synchronise repo avec le sync_run run déjà ouvert (verrou pris) ; en cas de succès,
// le sync_run est clos par store.ApplyRepoSync. Voir SyncRepo pour la stratégie.
func (e *Engine) syncRepository(ctx context.Context, prov provider.Provider, h *halt, repo model.Repository,
	run model.SyncRun) (repoOutcome, error) {
	meta, err := e.lookupRepository(ctx, prov, repo)
	if err != nil {
		return repoOutcome{}, err
	}
	s := &repoSync{
		e: e, prov: prov,
		owner: meta.Owner, name: meta.Name, full: meta.FullName(),
		index: make(map[string]int),
	}
	if s.full != repo.FullName() {
		e.logger.Info("dépôt renommé ou transféré", "repo", repo.FullName(), "nouveau_nom", s.full)
	}

	all, err := prov.Branches(ctx, meta.Owner, meta.Name)
	if err != nil {
		return repoOutcome{}, err
	}
	tracked := SelectBranches(all, repo.BranchPatterns, e.cfg.Branches.Exclude)
	if ref := ReferenceBranch(tracked, meta.DefaultBranch); ref > 0 {
		// La branche de référence est traitée en premier.
		tracked = slices.Concat(tracked[ref:ref+1], tracked[:ref], tracked[ref+1:])
	}

	knownList, err := e.store.ListBranches(ctx, repo.ID)
	if err != nil {
		return repoOutcome{}, err
	}
	known := make(map[string]model.Branch, len(knownList))
	var prevTracked []provider.Branch // branches suivies à la synchro précédente
	for _, b := range knownList {
		known[b.Name] = b
		if b.Tracked && b.HeadSHA != "" {
			prevTracked = append(prevTracked, provider.Branch{Name: b.Name, HeadSHA: b.HeadSHA})
		}
	}
	// Branche de référence de la synchro précédente (même règle, sur les branches suivies alors) ;
	// vide si aucune.
	prevRef := ""
	if i := ReferenceBranch(prevTracked, repo.DefaultBranch); i >= 0 {
		prevRef = prevTracked[i].Name
	}
	count, err := e.store.CountCommits(ctx, repo.ID)
	if err != nil {
		return repoOutcome{}, err
	}
	firstImport := count == 0

	for i, b := range tracked {
		// Premier import, ou branche de référence changée (renommée ou changée sur la forge, ancienne
		// référence supprimée ou exclue par les motifs) : la liste complète de la nouvelle référence.
		fullList := i == 0 && (firstImport || b.Name != prevRef)
		if err := s.fetchBranch(ctx, i == 0, fullList, b, tracked[0].HeadSHA, known); err != nil {
			return repoOutcome{}, err
		}
	}
	if s.skipped > 0 {
		e.logger.Warn("commits ignorés : date absente", "repo", s.full, "nombre", s.skipped)
	}

	stats := 0
	if !firstImport {
		if stats, err = s.fetchStats(ctx, h, repo.ID); err != nil {
			return repoOutcome{}, err
		}
	}

	x, err := s.fetchExtras(ctx, h, repo.ID)
	if err != nil {
		return repoOutcome{}, err
	}

	states := make([]store.BranchState, len(tracked))
	for i, b := range tracked {
		states[i] = store.BranchState{Name: b.Name, HeadSHA: b.HeadSHA}
	}
	res, err := e.store.ApplyRepoSync(ctx, store.RepoSyncWrite{
		RepoID: repo.ID,
		RunID:  run.ID,
		Metadata: store.RepositoryMetadata{
			Partial:       meta.Partial,
			ProviderID:    meta.ID,
			Owner:         meta.Owner,
			Name:          meta.Name,
			DefaultBranch: meta.DefaultBranch,
			URL:           meta.URL,
			Stars:         meta.Stars,
			OpenIssues:    meta.OpenIssues,
			Archived:      meta.Archived,
		},
		Branches:        states,
		Commits:         s.commits,
		PullRequests:    x.pulls,
		ReplaceReleases: x.replaceReleases,
		Releases:        x.releases,
		ReplaceTags:     x.replaceTags,
		Tags:            x.tags,
		SyncedAt:        e.now().UTC(),
	})
	if err != nil {
		return repoOutcome{}, err
	}
	display := repo.Label
	if display == "" {
		display = s.full
	}
	return repoOutcome{fullName: s.full, displayName: display, result: res, stats: stats,
		pulls: len(x.pulls), warnings: x.warnings}, nil
}

// lookupRepository lit les métadonnées actuelles du dépôt, par son identifiant de forge (suit
// renommage et transfert), ou par owner/name s'il n'est pas connu (dépôt ajouté par clone Git local :
// la première synchro par l'API résout alors l'identifiant) ou si la source ne connaît pas les
// identifiants (clones Git locaux : ErrUnsupported).
func (e *Engine) lookupRepository(ctx context.Context, prov provider.Provider, repo model.Repository) (provider.Repository, error) {
	if repo.ProviderID > 0 {
		meta, err := prov.RepositoryByID(ctx, repo.ProviderID)
		if !errors.Is(err, provider.ErrUnsupported) {
			return meta, err
		}
	}
	return prov.Repository(ctx, repo.Owner, repo.Name)
}

// fetchBranch récupère les commits nouveaux de la branche b (isRef : branche de référence, dont la
// tête est refHead) selon la stratégie de SyncRepo. fullList (branche de référence seulement) : liste
// complète des commits de la branche, sans Stop (premier import, ou nouvelle branche de référence,
// qui doit être rattachée à tout son historique, y compris aux commits déjà en base).
func (s *repoSync) fetchBranch(ctx context.Context, isRef, fullList bool, b provider.Branch, refHead string,
	known map[string]model.Branch) error {
	prev, hasPrev := known[b.Name]
	var (
		commits []provider.Commit
		err     error
	)
	switch {
	case fullList:
		// Premier import : rien n'est connu, la liste complète (bornée par MaxCommits) est exacte.
		// Nouvelle branche de référence d'un dépôt déjà importé (branche par défaut renommée ou
		// changée, motifs modifiés) : une comparaison depuis l'ancienne référence ne donnerait que les
		// commits absents de celle-ci, et les commits partagés ne seraient rattachés à aucune branche
		// suivie si l'ancienne ne l'est plus. Les commits déjà en base ne sont pas réenregistrés :
		// seul commit_branches est complété.
		commits, err = s.list(ctx, b.HeadSHA)
	case hasPrev && prev.HeadSHA == b.HeadSHA:
		return nil // tête inchangée : aucune requête
	case hasPrev && prev.HeadSHA != "":
		commits, err = s.compare(ctx, prev.HeadSHA, b.HeadSHA)
		if errors.Is(err, provider.ErrUnknownBase) {
			s.e.logger.Debug("ancienne tête introuvable (historique réécrit ?), repli", "repo", s.full,
				"base", provider.ShortSHA(prev.HeadSHA), "tete", provider.ShortSHA(b.HeadSHA))
			base := refHead
			if isRef {
				base = ""
			}
			commits, err = s.fallback(ctx, base, b.HeadSHA)
		}
	default:
		// Nouvelle branche (ou autre branche du premier import) : commits propres à la branche.
		commits, err = s.fallback(ctx, refHead, b.HeadSHA)
	}
	if err != nil {
		return err
	}
	s.add(commits, b.Name)
	return nil
}

// fallback compare base...head si base est fournie et différente de head ; si la base est inconnue
// (ErrUnknownBase), ou sans base, relit la liste des commits de head sans Stop.
func (s *repoSync) fallback(ctx context.Context, base, head string) ([]provider.Commit, error) {
	if base != "" {
		commits, err := s.compare(ctx, base, head)
		if !errors.Is(err, provider.ErrUnknownBase) {
			return commits, err
		}
		s.e.logger.Debug("comparaison impossible, relecture de la liste des commits", "repo", s.full,
			"base", provider.ShortSHA(base), "tete", provider.ShortSHA(head))
	}
	return s.list(ctx, head)
}

// list renvoie les commits atteignables depuis head (SHA complet de la tête, pour rester cohérent avec
// la liste des branches lue juste avant), sans Stop, bornés par provider.MaxCommits.
func (s *repoSync) list(ctx context.Context, head string) ([]provider.Commit, error) {
	commits, err := s.prov.ListCommits(ctx, s.owner, s.name, provider.ListCommitsOptions{Branch: head})
	if err == nil && len(commits) >= provider.MaxCommits {
		s.e.logger.Warn("historique tronqué", "repo", s.full, "tete", provider.ShortSHA(head),
			"plafond", provider.MaxCommits)
	}
	return commits, err
}

// compare renvoie les commits atteignables depuis head mais pas depuis base.
func (s *repoSync) compare(ctx context.Context, base, head string) ([]provider.Commit, error) {
	return s.prov.CompareCommits(ctx, s.owner, s.name, base, head)
}

// add ajoute les commits vus sur la branche branch (un commit déjà vu reçoit la branche en plus).
func (s *repoSync) add(commits []provider.Commit, branch string) {
	for _, c := range commits {
		if i, ok := s.index[c.SHA]; ok {
			if !slices.Contains(s.commits[i].Branches, branch) {
				s.commits[i].Branches = append(s.commits[i].Branches, branch)
			}
			continue
		}
		w, ok := commitWrite(c)
		if !ok {
			s.skipped++
			continue
		}
		w.Branches = []string{branch}
		s.index[c.SHA] = len(s.commits)
		s.commits = append(s.commits, w)
	}
}

// commitWrite convertit un commit de la forge ; false si le commit n'a aucune date (le store le
// refuserait et l'activité serait faussée). Une seule date manquante est remplacée par l'autre.
func commitWrite(c provider.Commit) (store.CommitWrite, bool) {
	authored, committed := c.AuthoredAt, c.CommittedAt
	switch {
	case authored.IsZero() && committed.IsZero():
		return store.CommitWrite{}, false
	case authored.IsZero():
		authored = committed
	case committed.IsZero():
		committed = authored
	}
	return store.CommitWrite{
		SHA:         c.SHA,
		Message:     c.Message,
		URL:         c.URL,
		Author:      identity(c.Author),
		Committer:   identity(c.Committer),
		AuthoredAt:  authored,
		CommittedAt: committed,
	}, true
}

// identity convertit une personne de la forge en identité à enregistrer.
func identity(p provider.Person) store.Identity {
	return store.Identity{
		Name:           p.Name,
		Email:          p.Email,
		Login:          p.Login,
		ProviderUserID: p.ID,
		AvatarURL:      p.AvatarURL,
		ProfileURL:     p.ProfileURL,
	}
}

// fetchStats demande les statistiques (CommitStats) des commits absents de la base, dans l'ordre de
// découverte (les plus récents de la branche de référence d'abord), au plus MaxStatsPerRepo, et
// s'arrête dès que le quota restant passe sous QuotaReservePercent. Un quota épuisé arrête aussi les
// dépôts suivants (h) sans faire échouer celui-ci ; une autre erreur arrête seulement les
// statistiques. Renvoie le nombre d'appels faits ; erreur seulement si le contexte est terminé.
func (s *repoSync) fetchStats(ctx context.Context, h *halt, repoID int64) (int, error) {
	if len(s.commits) == 0 {
		return 0, nil
	}
	shas := make([]string, len(s.commits))
	for i, c := range s.commits {
		shas[i] = c.SHA
	}
	existing, err := s.e.store.ExistingCommitSHAs(ctx, repoID, shas)
	if err != nil {
		return 0, err
	}
	calls := 0
	for i := range s.commits {
		c := &s.commits[i]
		if existing[c.SHA] {
			continue
		}
		if calls >= MaxStatsPerRepo {
			break
		}
		if rl, ok := s.prov.LastRateLimit(); ok && QuotaLow(rl, s.e.now()) {
			s.e.logger.Debug("statistiques non demandées : quota bas", "repo", s.full)
			break
		}
		calls++
		st, err := s.prov.CommitStats(ctx, s.owner, s.name, c.SHA)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return calls, fmt.Errorf("statistiques des commits : %w", ctxErr)
			}
			var rle *provider.RateLimitError
			if errors.As(err, &rle) {
				h.set(rateLimitResume(rle, s.e.now()), reasonSpent)
				s.e.logger.Info("statistiques interrompues : quota épuisé", "repo", s.full)
				break
			}
			if errors.Is(err, provider.ErrNotFound) {
				continue // commit disparu entre-temps : statistiques à 0
			}
			s.e.logger.Debug("statistiques interrompues", "repo", s.full, "sha", provider.ShortSHA(c.SHA),
				"erreur", err.Error())
			break
		}
		c.Additions, c.Deletions = st.Additions, st.Deletions
	}
	return calls, nil
}
