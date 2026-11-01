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

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
)

// Noms des listes dans les avertissements (« <liste> non synchronisées : … »).
const (
	listPulls    = "pull requests"
	listReleases = "releases"
	listTags     = "tags"
)

// extras est le résultat de la lecture des pull requests, releases et tags d'un dépôt.
type extras struct {
	pulls           []model.PullRequest
	releases        []model.Release
	replaceReleases bool // releases lues : à remplacer en base
	tags            []model.Tag
	replaceTags     bool // tags lus : à remplacer en base
	warnings        []string
	skipped         int // éléments inexploitables ignorés (comptés dans un log)
}

// fetchExtras lit les pull requests (depuis la plus récente date de modification en base), les
// releases et les tags du dépôt repoID :
//   - quota restant sous QuotaReservePercent : rien n'est demandé (avertissement) ;
//   - provider.ErrUnsupported (source sans cette notion) : liste ignorée, sans avertissement ;
//   - *provider.RateLimitError : les dépôts suivants sont reportés (h), cette liste et les suivantes
//     ne sont pas lues (avertissement) ;
//   - autre erreur : avertissement, la liste suivante est tout de même lue.
//
// Erreur seulement si le contexte est terminé ou si la base ne peut être lue.
func (s *repoSync) fetchExtras(ctx context.Context, h *halt, repoID int64) (extras, error) {
	var x extras
	if rl, ok := s.prov.LastRateLimit(); ok && QuotaLow(rl, s.e.now()) {
		x.warn(reasonLow, listPulls, listReleases, listTags)
		s.e.logger.Info("pull requests, releases et tags non demandés : quota bas", "repo", s.full)
		return x, nil
	}

	since, err := s.e.store.LatestPullRequestUpdate(ctx, repoID)
	if err != nil {
		return x, err
	}
	pulls, err := s.prov.PullRequests(ctx, s.owner, s.name, provider.PullRequestsOptions{UpdatedSince: since})
	if stop, ferr := s.extraFailed(ctx, h, &x, err, listPulls, listReleases, listTags); stop || ferr != nil {
		return x, ferr
	}
	if err == nil {
		for _, p := range pulls {
			if m, ok := pullRequestModel(p); ok {
				x.pulls = append(x.pulls, m)
			} else {
				x.skipped++
			}
		}
		if len(pulls) >= provider.MaxPullRequests {
			s.e.logger.Warn("pull requests tronquées", "repo", s.full, "plafond", provider.MaxPullRequests)
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
		for _, t := range tags {
			if t.Name == "" || provider.ValidateSHA(t.SHA) != nil {
				x.skipped++
				continue
			}
			x.tags = append(x.tags, model.Tag{Name: t.Name, SHA: t.SHA})
		}
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
		h.set(rateLimitResume(rle, s.e.now()), reasonSpent)
		x.warn(errorMessage(err), lists...)
		s.e.logger.Info("pull requests, releases ou tags interrompus : quota épuisé", "repo", s.full,
			"liste", lists[0])
		return true, nil
	}
	x.warn(errorMessage(err), lists[0])
	s.e.logger.Warn("liste non synchronisée", "repo", s.full, "liste", lists[0], "erreur", errorMessage(err))
	return false, nil
}

// warn ajoute l'avertissement « <listes> non synchronisées : <raison> » (« synchronisés » dès que
// les tags en sont).
func (x *extras) warn(reason string, lists ...string) {
	agreed := " non synchronisées : "
	if slices.Contains(lists, listTags) {
		agreed = " non synchronisés : "
	}
	x.warnings = append(x.warnings, joinLists(lists)+agreed+reason)
}

// joinLists écrit « a », « a et b », « a, b et c ».
func joinLists(lists []string) string {
	switch len(lists) {
	case 0:
		return ""
	case 1:
		return lists[0]
	}
	return strings.Join(lists[:len(lists)-1], ", ") + " et " + lists[len(lists)-1]
}

// pullRequestModel convertit une pull request de la forge ; false si elle est inexploitable (numéro,
// état ou date de modification absents). Une date d'ouverture absente prend la date de modification ;
// une PR mergée sans date de merge prend sa date de fermeture, sinon de modification.
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
