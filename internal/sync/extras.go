package sync

// Pull requests, releases et tags d'un dépôt (V2 ; documentation/fr/03-architecture.md, « Synchronisation
// incrémentale ») : lus après les commits, écrits dans la même transaction. Un échec sur l'une de ces
// listes ne fait pas échouer le dépôt : il devient un avertissement du rapport, et ce qui est en base
// pour cette liste est gardé. Seule l'annulation (contexte terminé) est une erreur.

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/diagnostic"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/store"
)

// Noms des listes dans les avertissements (« <liste> not synced: … »), identiques dans les deux
// langues.
const (
	listPulls    = "pull requests"
	listReleases = "releases"
	listTags     = "tags"
)

// extras est le résultat de la lecture des pull requests, releases et tags d'un dépôt.
type extras struct {
	pulls []model.PullRequest
	// prConventions (phase 8.5) sont les PR non modifiées depuis la synchro précédente, relues par la
	// relecture complète unique : seuls leur commit de fusion, leur origine et le masque des sections de
	// leur corps sont écrits.
	prConventions []store.PullRequestConventionsWrite
	// prsVersion (phase 8.5) : version des règles de PR à poser sur le dépôt quand la relecture complète
	// a réussi sans être tronquée ; 0 = marqueur inchangé.
	prsVersion      int64
	releases        []model.Release
	replaceReleases bool // releases lues : à remplacer en base
	tags            []model.Tag
	replaceTags     bool // tags lus : à remplacer en base
	// tagsComplete : liste des tags lue en entier (phase 8.4) : lecture réussie, aucun tag ignoré et
	// moins de provider.MaxTags tags (au plafond, la liste peut être tronquée). Seule une lecture
	// complète met à jour les observations des tags (un tag absent y serait fermé à tort).
	tagsComplete bool
	warnings     []model.SyncWarning
	skipped      int // éléments inexploitables ignorés (comptés dans un log)
}

// fetchExtras lit les pull requests (depuis la plus récente date de modification en base), les
// releases et les tags du dépôt repoID :
//   - quota restant sous QuotaReservePercent : rien n'est demandé (avertissement) ;
//   - provider.ErrUnsupported (source sans cette notion) : liste ignorée, sans avertissement ;
//   - *provider.RateLimitError : les dépôts suivants sont reportés (h), cette liste et les suivantes
//     ne sont pas lues (avertissement) ;
//   - autre erreur : avertissement, la liste suivante est tout de même lue.
//
// Pour chaque PR lue, la description n'est lue qu'en mémoire : outils déclarés (D2, si le diagnostic
// est actif) et masque des sections (PR5, conventions.CheckPRBody, toujours : il ne coûte rien) ; elle
// n'est jamais gardée ni journalisée.
//
// Relecture complète unique (phase 8.5, documentation/fr/13-conventions.md) : conventions actives et
// dépôt dont repositories.conventions_prs_version est absent ou inférieur à conventions.PRRulesVersion.
// La liste est alors demandée sans UpdatedSince (au plus provider.MaxPullRequests PR, soit 10 pages),
// à la place de la lecture incrémentale qu'elle contient : les PR modifiées depuis la plus récente date
// en base sont écrites en entier comme d'habitude, les autres ne reçoivent que merge_commit_sha,
// from_fork et body_sections. Le marqueur n'est posé que si la liste a été lue sans erreur, sans PR
// ignorée et sans atteindre le plafond ; sinon la relecture est retentée à la synchro suivante (ce qui
// a été rempli reste).
//
// Erreur seulement si le contexte est terminé ou si la base ne peut être lue.
func (s *repoSync) fetchExtras(ctx context.Context, h *halt, repo model.Repository) (extras, error) {
	repoID := repo.ID
	var x extras
	if rl, ok := s.prov.LastRateLimit(); ok && QuotaLow(rl, s.e.now()) {
		x.warn(DeferLowQuota.Text(), listPulls, listReleases, listTags)
		s.e.logger.Info("pull requests, releases et tags non demandés : quota bas", "repo", s.full)
		return x, nil
	}

	since, err := s.e.store.LatestPullRequestUpdate(ctx, repoID)
	if err != nil {
		return x, err
	}
	reread := s.e.cfg.Conventions.Enabled && repo.ConventionsPRsVersion < conventions.PRRulesVersion
	opts := provider.PullRequestsOptions{UpdatedSince: since}
	if reread {
		opts.UpdatedSince = time.Time{}
	}
	pulls, err := s.prov.PullRequests(ctx, s.owner, s.name, opts)
	if stop, ferr := s.extraFailed(ctx, h, &x, err, listPulls, listReleases, listTags); stop || ferr != nil {
		return x, ferr
	}
	if err == nil {
		since = since.UTC().Truncate(time.Second)
		pullsSkipped := 0
		for _, p := range pulls {
			m, ok := pullRequestModel(p)
			if !ok {
				pullsSkipped++
				continue
			}
			if reread && p.UpdatedAt.UTC().Truncate(time.Second).Before(since) {
				// Déjà en base et non modifiée : seuls les champs de la phase 8.5 sont complétés.
				x.prConventions = append(x.prConventions, store.PullRequestConventionsWrite{
					Number: m.Number, MergeCommitSHA: m.MergeCommitSHA, FromFork: m.FromFork,
					BodySections: m.BodySections,
				})
				continue
			}
			if s.e.diag != nil {
				// Signal D2 : la description n'est lue qu'ici, en mémoire ; seuls les outils sont gardés.
				m.ToolIDs = diagnostic.EncodeDeclarations(diagnostic.PullRequestDeclarations(p.Body))
			}
			x.pulls = append(x.pulls, m)
		}
		x.skipped += pullsSkipped
		truncated := len(pulls) >= provider.MaxPullRequests
		if truncated {
			s.e.logger.Warn("pull requests tronquées", "repo", s.full, "plafond", provider.MaxPullRequests)
		}
		if reread {
			if !truncated && pullsSkipped == 0 {
				x.prsVersion = conventions.PRRulesVersion
			}
			s.e.logger.Info("liste complète des pull requests relue (conventions)", "repo", s.full,
				"relues", len(x.prConventions), "complète", x.prsVersion > 0)
		}
	}

	releases, err := s.prov.Releases(ctx, s.owner, s.name)
	if stop, ferr := s.extraFailed(ctx, h, &x, err, listReleases, listTags); stop || ferr != nil {
		return x, ferr
	}
	if err == nil {
		x.replaceReleases = true
		x.releases = make([]model.Release, 0, len(releases))
		for _, r := range releases {
			if m, ok := releaseModel(r); ok {
				x.releases = append(x.releases, m)
			} else {
				x.skipped++
			}
		}
	}

	tags, err := s.prov.Tags(ctx, s.owner, s.name)
	if stop, ferr := s.extraFailed(ctx, h, &x, err, listTags); stop || ferr != nil {
		return x, ferr
	}
	if err == nil {
		x.replaceTags = true
		x.tags = make([]model.Tag, 0, len(tags))
		tagsSkipped := 0 // tags ignorés : propres aux tags, contrairement à x.skipped
		for _, t := range tags {
			if t.Name == "" || provider.ValidateSHA(t.SHA) != nil {
				tagsSkipped++
				continue
			}
			x.tags = append(x.tags, model.Tag{Name: t.Name, SHA: t.SHA})
		}
		x.skipped += tagsSkipped
		x.tagsComplete = tagsSkipped == 0 && len(tags) < provider.MaxTags
	}
	if x.skipped > 0 {
		s.e.logger.Warn("pull requests, releases ou tags ignorés : données incomplètes", "repo", s.full,
			"nombre", x.skipped)
	}
	return x, nil
}

// extraFailed traite l'erreur err de la lecture de la liste lists[0] (lists : cette liste et celles
// qui restent à lire). Elle renvoie stop = vrai quand les listes restantes ne doivent pas être lues
// (quota épuisé), et une erreur seulement si le contexte est terminé. Sans erreur : (false, nil).
// L'appelant n'utilise le résultat d'une liste que si err est nil : une liste en échec ou non
// disponible ne remplace rien en base.
func (s *repoSync) extraFailed(ctx context.Context, h *halt, x *extras, err error, lists ...string) (bool, error) {
	if err == nil {
		return false, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return true, ctxErr
	}
	if errors.Is(err, provider.ErrUnsupported) {
		s.e.logger.Debug("liste non disponible pour cette source", "repo", s.full, "liste", lists[0])
		return false, nil
	}
	if rle, ok := errors.AsType[*provider.RateLimitError](err); ok {
		h.set(rateLimitResume(rle, s.e.now()), DeferQuotaExhausted)
		x.warn(errorMessage(err), lists...)
		s.e.logger.Info("pull requests, releases ou tags interrompus : quota épuisé", "repo", s.full,
			"liste", lists[0])
		return true, nil
	}
	x.warn(errorMessage(err), lists[0])
	s.e.logger.Warn("liste non synchronisée", "repo", s.full, "liste", lists[0], "erreur", errorMessage(err))
	return false, nil
}

// warn ajoute l'avertissement « <listes> not synced: <raison> », dans la langue du processus (en
// français « <listes> non synchronisées : <raison> », « synchronisés » dès que les tags en sont).
func (x *extras) warn(reason string, lists ...string) {
	key := "sync.extras.not_synced"
	if slices.Contains(lists, listTags) {
		key = "sync.extras.not_synced_masc"
	}
	x.warnings = append(x.warnings, model.SyncWarning{Kind: model.WarnListsNotSynced,
		Message: i18n.T(key, joinLists(lists), reason)})
}

// joinLists écrit « a », « a and b », « a, b and c » (« et » en français).
func joinLists(lists []string) string {
	switch len(lists) {
	case 0:
		return ""
	case 1:
		return lists[0]
	}
	return strings.Join(lists[:len(lists)-1], ", ") + i18n.T("sync.extras.and") + lists[len(lists)-1]
}

// pullRequestModel convertit une pull request de la forge ; false si elle est inexploitable (numéro,
// état ou date de modification absents). Une date d'ouverture absente prend la date de modification ;
// une PR mergée sans date de merge prend sa date de fermeture, sinon de modification. Phase 8.5 : le
// commit de fusion n'est gardé que pour une PR fusionnée et un SHA valide ; BodySections est le masque
// conventions.CheckPRBody de la description (lue en mémoire, jamais gardée).
func pullRequestModel(p provider.PullRequest) (model.PullRequest, bool) {
	if p.Number <= 0 || p.UpdatedAt.IsZero() ||
		(p.State != provider.PullRequestOpen && p.State != provider.PullRequestClosed) {
		return model.PullRequest{}, false
	}
	created := p.CreatedAt
	if created.IsZero() {
		created = p.UpdatedAt
	}
	merged, mergedAt := p.Merged || !p.MergedAt.IsZero(), p.MergedAt
	mergeSHA := ""
	if merged && provider.ValidateSHA(p.MergeCommitSHA) == nil {
		mergeSHA = p.MergeCommitSHA
	}
	sections := conventions.CheckPRBody(p.Body)
	if merged && mergedAt.IsZero() {
		mergedAt = p.ClosedAt
		if mergedAt.IsZero() {
			mergedAt = p.UpdatedAt
		}
	}
	return model.PullRequest{
		Number:           p.Number,
		Title:            p.Title,
		State:            p.State,
		Merged:           merged,
		Draft:            p.Draft,
		AuthorLogin:      p.Author.Login,
		AuthorProviderID: p.Author.ID,
		URL:              p.URL,
		BaseBranch:       p.BaseBranch,
		HeadBranch:       p.HeadBranch,
		CreatedAt:        created,
		UpdatedAt:        p.UpdatedAt,
		ClosedAt:         p.ClosedAt,
		MergedAt:         mergedAt,
		MergeCommitSHA:   mergeSHA,
		FromFork:         p.FromFork.Ptr(),
		// PR5 : la description n'est lue qu'ici, en mémoire ; seul le masque des sections est gardé.
		BodySections: &sections,
	}, true
}

// releaseModel convertit une release de la forge ; false si elle est inexploitable (identifiant, tag
// ou toute date absents). Une date de création absente prend la date de publication.
func releaseModel(r provider.Release) (model.Release, bool) {
	created := r.CreatedAt
	if created.IsZero() {
		created = r.PublishedAt
	}
	if r.ID <= 0 || r.TagName == "" || created.IsZero() {
		return model.Release{}, false
	}
	return model.Release{
		ProviderID:  r.ID,
		TagName:     r.TagName,
		Name:        r.Name,
		Draft:       r.Draft,
		Prerelease:  r.Prerelease,
		AuthorLogin: r.Author.Login,
		URL:         r.URL,
		CreatedAt:   created,
		PublishedAt: r.PublishedAt,
	}, true
}
