package git

// Nombre de parents des commits (phase 8.1) : rattrapage de commits.parent_count pour les commits
// enregistrés avant la migration 00016, lu dans le clone sans le mettre à jour.

import (
	"context"
	"slices"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// ParentCounts renvoie le nombre de parents (0 = racine, 1 = ordinaire, 2 ou plus = fusion) des commits
// shas (SHA complets) du clone EXISTANT de owner/name, lu tel quel, sans mise à jour (aucun accès
// réseau) : le moteur l'appelle après Prepare. Un SHA absent du clone est omis du résultat ; les doublons
// sont permis. Les SHA sont traités par paquets de StatsBatchSize, un processus git par paquet :
//
//	git rev-list --no-walk=unsorted --parents --ignore-missing --stdin
//
// Sans clone : erreur provider.ErrNotFound. Le calcul s'arrête dès que ctx est terminé (erreur
// ctx.Err()).
func (p *Provider) ParentCounts(ctx context.Context, owner, name string, shas []string) (map[string]int, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	for _, sha := range shas {
		if err := provider.ValidateSHA(sha); err != nil {
			return nil, err
		}
	}
	op := i18n.T("provider.git.op.parent_counts", owner, name)
	if !p.HasClone(owner, name) {
		return nil, &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(shas))
	unique := slices.Compact(slices.Sorted(slices.Values(shas)))
	for batch := range slices.Chunk(unique, StatsBatchSize) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		commits, err := p.revListParents(ctx, op, path, batch)
		if err != nil {
			return nil, err
		}
		for _, fields := range commits {
			out[fields[0]] = len(fields) - 1
		}
	}
	return out, nil
}
