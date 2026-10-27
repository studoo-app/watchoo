package github

// Appel d'API sur le quota (voir les règles communes en tête de repos.go).

import (
	"context"
	"log/slog"

	"github.com/studoo-app/watchoo/internal/provider"
)

// RateLimit implémente provider.Provider : GET /rate_limit (RateLimitService.Get), gratuit et permis
// même quand le quota est épuisé (le transport ne le refuse jamais préventivement).
//
// Résultat : resources.core → RateLimit{Limit, Remaining, Used, Reset (UTC), ObservedAt: heure de
// réception en UTC (p.tr.now)} ; core absent (ou sans date de remise à zéro) → p.unexpected. L'état
// est ensuite enregistré par p.quota.observe(p.fp, rl), pour LastRateLimit.
func (p *Provider) RateLimit(ctx context.Context) (provider.RateLimit, error) {
	const op = "lecture du quota de l'API"
	limits, resp, err := p.client.RateLimit.Get(ctx)
	if err != nil {
		return provider.RateLimit{}, p.convertError(ctx, op, resp, err)
	}
	core := limits.GetCore()
	if core == nil || core.Reset.IsZero() {
		return provider.RateLimit{}, p.unexpected(op, resp)
	}
	rl := provider.RateLimit{
		Limit:      core.Limit,
		Remaining:  core.Remaining,
		Used:       core.Used,
		Reset:      core.Reset.UTC(),
		ObservedAt: p.tr.now().UTC(),
	}
	p.quota.observe(p.fp, rl)
	p.logger.DebugContext(ctx, "quota de l'API lu",
		slog.Int("limite", rl.Limit),
		slog.Int("restant", rl.Remaining),
	)
	return rl, nil
}

// LastRateLimit implémente provider.Provider : p.quota.last(p.fp), sans requête. L'état est mis à jour
// par le transport à chaque réponse portant des en-têtes de quota « core » (304 comprises) et par
// RateLimit ; il est partagé entre les Provider construits avec le même Options.Quota et le même token.
func (p *Provider) LastRateLimit() (provider.RateLimit, bool) {
	return p.quota.last(p.fp)
}
