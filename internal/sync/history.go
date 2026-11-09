package sync

// Historique des tags et réécritures de la branche par défaut (conventions, phase 8.4,
// documentation/fr/13-conventions.md, §5.2 BR4, §5.4 SV2 et SV3, §9) : alimentés à la synchro,
// seulement si les conventions sont activées (minimisation des données), écrits dans la transaction de
// la synchro (store.RepoSyncWrite.ObserveTags, BranchRewrites). Aucun de ces calculs ne fait échouer la
// synchro du dépôt ; un échec est journalisé en Debug, sans nom de tag, de branche ni SHA. Aucune
// attribution à une personne.

import (
	"context"
	"slices"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
)

// AncestryReader est le test d'ascendance du clone Git local (implémenté par *git.Provider), lu tel quel
// après Prepare : vrai si ancestor est ancêtre de descendant ; un commit absent du clone est une erreur
// (provider.ErrNotFound), lue comme « inconnu ».
type AncestryReader interface {
	IsAncestor(ctx context.Context, owner, name, ancestor, descendant string) (bool, error)
}

// Vérifie à la compilation que le provider des clones sait tester l'ascendance (phase 8.4).
var _ AncestryReader = (*gitprov.Provider)(nil)

// observeTags indique si les observations des tags sont mises à jour par cette synchro : conventions
// activées et liste des tags lue en entier (réussie, aucun tag ignoré, sous le plafond). Sinon rien
// n'est ouvert ni fermé.
func (e *Engine) observeTags(x extras) bool {
	return e.cfg.Conventions.Enabled && x.replaceTags && x.tagsComplete
}

// headChange décrit la tête de la branche par défaut connue avant la synchro (repo.DefaultBranch,
// branches en base known) et lue par elle (defaultBranch, branches suivies tracked).
func headChange(repo model.Repository, known map[string]model.Branch, defaultBranch string,
	tracked []provider.Branch) conventions.HeadChange {
	h := conventions.HeadChange{PrevBranch: repo.DefaultBranch, Branch: defaultBranch}
	if b, ok := known[repo.DefaultBranch]; ok && repo.DefaultBranch != "" {
		h.PrevSHA, h.PrevTracked = b.HeadSHA, b.Tracked
	}
	if i := slices.IndexFunc(tracked, func(b provider.Branch) bool { return b.Name == defaultBranch }); i >= 0 {
		h.SHA, h.Tracked = tracked[i].HeadSHA, true
	}
	return h
}

// branchRewrites renvoie la réécriture de la branche par défaut observée par cette synchro (BR4) : au
// plus un événement, seulement si les conventions sont activées et les têtes comparables
// (conventions.HeadChange.Comparable). L'ascendance de l'ancienne tête vers la nouvelle est lue dans le
// clone mis à jour par cette synchro (s.clone), dans les deux modes ; un commit absent du clone, ou un
// clone illisible, donne « inconnu », jamais une réécriture. En mode API seulement, quand le clone ne
// tranche pas (pas de clone, ou commit absent du clone), la comparaison de la forge en échec sur
// l'ancienne tête (provider.ErrUnknownBase : ancienne tête qui n'existe plus dans le dépôt, ou
// historiques sans ancêtre commun ; la nouvelle tête vient d'être lue) compte comme réécriture. GitHub
// garde longtemps les commits orphelins : après un force-push, la comparaison réussit souvent
// (« diverged », « behind ») et l'API seule ne voit alors pas la réécriture. En clone seul, ErrUnknownBase
// signifie un commit absent du clone et n'est jamais un signal.
func (e *Engine) branchRewrites(ctx context.Context, s *repoSync, src Source, h conventions.HeadChange) []model.BranchRewrite {
	if !e.cfg.Conventions.Enabled || !h.Comparable() || ctx.Err() != nil {
		return nil
	}
	a := conventions.AncestryUnknown
	if ar, ok := s.clone.(AncestryReader); ok {
		yes, err := ar.IsAncestor(ctx, s.owner, s.name, h.PrevSHA, h.SHA)
		switch {
		case err != nil:
			e.logger.Debug("réécriture de la branche par défaut non vérifiée", "repo", s.full)
		case yes:
			a = conventions.AncestryYes
		default:
			a = conventions.AncestryNo
		}
	}
	if a == conventions.AncestryUnknown && src.Kind == SourceAPI && s.unknownBase[h.Branch] {
		a = conventions.AncestryNo
	}
	if !conventions.RewriteObserved(h, a) {
		return nil
	}
	e.logger.Debug("réécriture de la branche par défaut observée", "repo", s.full)
	return []model.BranchRewrite{{Branch: h.Branch, OldSHA: h.PrevSHA, NewSHA: h.SHA}}
}
