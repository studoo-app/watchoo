package sync

// Relectures des pull requests (PR4 des conventions, phase 8.6) : après l'écriture de la synchro, les
// relectures des PR fusionnées jamais relues depuis leur fusion, puis des PR ouvertes modifiées depuis la
// dernière lecture, sont lues par l'API (GET /pulls/{n}/reviews) et réduites en mémoire à trois compteurs
// (conventions.ReviewTally). Ni le login d'un relecteur ni le corps d'une relecture ne sont gardés ou
// journalisés.

import (
	"context"
	"errors"

	"github.com/studoo-app/watchoo/internal/aisignature"
	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/store"
)

// updatePullRequestReviews lit les relectures des PR du dépôt dont les compteurs sont à (re)calculer
// (store.PullRequestsToReview, auteurs bots et PR ouvertes avant la date de début des conventions du dépôt
// écartés : ni lues ni évaluées), seulement si les conventions sont activées et en
// mode API, au plus MaxReviewsPerRepo requêtes (pagination comprise dans chaque appel), et s'arrête dès
// que le quota restant passe sous QuotaReservePercent : le reste est lu aux synchros suivantes. Un quota
// épuisé arrête aussi les dépôts suivants (h). Une PR disparue (ErrNotFound) est passée ; toute autre
// erreur, de la forge comme de la base, arrête la passe et n'est journalisée qu'en Debug (dépôt, numéro
// de PR, erreur) : la synchro du dépôt réussit toujours.
func (e *Engine) updatePullRequestReviews(ctx context.Context, s *repoSync, h *halt, src Source, repo model.Repository) {
	repoID := repo.ID
	if !e.cfg.Conventions.Enabled || src.Kind != SourceAPI || ctx.Err() != nil {
		return
	}
	if _, _, halted := h.get(); halted {
		return
	}
	all, err := e.store.PullRequestsToReview(ctx, repoID)
	if err != nil {
		e.logger.Debug("relectures des pull requests non lues", "repo", s.full, "erreur", errorMessage(err))
		return
	}
	since, err := conventions.EffectiveSinceAt(e.cfg.Conventions.Since, repo.ConventionsSince, e.conventionsLocation())
	if err != nil {
		e.logger.Debug("relectures des pull requests non lues", "repo", s.full, "erreur", errorMessage(err))
		return
	}
	cands := make([]store.ReviewCandidate, 0, len(all))
	for _, c := range all {
		if aisignature.IsBot(c.AuthorLogin) || (!since.IsZero() && c.CreatedAt.Before(since)) {
			continue // hors de PR4 (conventions.PRCategory, date de début) : aucune requête
		}
		cands = append(cands, c)
	}
	if len(cands) == 0 {
		return
	}
	ids, logins, err := e.store.RepoCommitAuthorAccounts(ctx, repoID)
	if err != nil {
		e.logger.Debug("relectures des pull requests non lues", "repo", s.full, "erreur", errorMessage(err))
		return
	}
	calls, written := 0, 0
	defer func() {
		if calls > 0 {
			e.logger.Debug("relectures des pull requests lues", "repo", s.full, "requetes", calls,
				"pr", written, "restantes", len(cands)-calls)
		}
	}()
	for _, c := range cands {
		if calls >= MaxReviewsPerRepo {
			return
		}
		if rl, ok := s.prov.LastRateLimit(); ok && QuotaLow(rl, e.now()) {
			e.logger.Debug("relectures des pull requests non demandées : quota bas", "repo", s.full)
			return
		}
		calls++
		list, err := s.prov.PullRequestReviews(ctx, s.owner, s.name, c.Number)
		if err != nil {
			switch {
			case ctx.Err() != nil:
				return
			case errors.Is(err, provider.ErrUnsupported):
				calls--
				return
			case errors.Is(err, provider.ErrNotFound):
				continue // PR disparue entre-temps : rien n'est écrit
			}
			if rle, ok := errors.AsType[*provider.RateLimitError](err); ok {
				h.set(rateLimitResume(rle, e.now()), DeferQuotaExhausted)
				e.logger.Info("relectures des pull requests interrompues : quota épuisé", "repo", s.full)
				return
			}
			e.logger.Debug("relectures des pull requests interrompues", "repo", s.full, "pr", c.Number,
				"erreur", errorMessage(err))
			return
		}
		counts := conventions.ReviewTally(toReviews(list), conventions.ReviewContext{
			AuthorID: c.AuthorProviderID, MergedAt: c.MergedAt, Excluded: e.cfg.Conventions.ExcludedAccounts,
			ContributorIDs: ids, ContributorLogins: logins,
		})
		// Date de lecture. PR fusionnée : au moins égale à la fusion, pour qu'une horloge locale en retard
		// sur la forge ne la fasse pas relire à chaque synchro. PR ouverte : la date de modification de la
		// forge, sans l'horloge locale, pour qu'une fusion ultérieure (merged_at > updated_at), même si
		// elle a lieu pendant cette synchro ou si l'horloge locale est en avance, fasse toujours relire la PR.
		checked := c.UpdatedAt
		if c.Merged {
			checked = e.now().UTC()
			if checked.Before(c.MergedAt) {
				checked = c.MergedAt
			}
		}
		ok, err := e.store.SetPullRequestReviews(ctx, store.PullRequestReviewsWrite{
			RepoID: repoID, Number: c.Number, ReviewsByOthers: counts.ReviewsByOthers,
			ApprovalsByOthers: counts.ApprovalsByOthers, ApprovalsByContributors: counts.ApprovalsByContributors,
			CheckedAt: checked,
		})
		if err != nil {
			e.logger.Debug("relectures des pull requests non enregistrées", "repo", s.full, "pr", c.Number,
				"erreur", errorMessage(err))
			return
		}
		if ok {
			written++
		}
	}
}

// toReviews convertit les relectures de la forge pour conventions.ReviewTally.
func toReviews(list []provider.PullRequestReview) []conventions.Review {
	out := make([]conventions.Review, 0, len(list))
	for _, r := range list {
		out = append(out, conventions.Review{
			ReviewerID: r.Reviewer.ID, ReviewerLogin: r.Reviewer.Login, State: r.State, SubmittedAt: r.SubmittedAt,
		})
	}
	return out
}
