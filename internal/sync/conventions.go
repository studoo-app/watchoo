package sync

// Conventions (phase 8.1) : accès en lecture aux clones Git locaux pour le périmètre précis du rapport
// (conventions.Build, calculé à la lecture par la CLI et le web). Aucun fetch, aucune écriture.

import (
	"context"

	"github.com/studoo-app/watchoo/internal/conventions"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
)

// Vérifie à la compilation que le provider des clones sait lire le périmètre des conventions.
var _ conventions.CloneReader = (*gitprov.Provider)(nil)

// ConventionsClones renvoie l'accès en lecture aux clones Git locaux existants de l'espace (périmètre
// des conventions : git rev-list des branches suivies), sans les mettre à jour ; nil si les clones sont
// désactivés ou si le provider ne sait pas les lire (repli sur la base). Erreur seulement pour la base.
func (e *Engine) ConventionsClones(ctx context.Context) (conventions.CloneReader, error) {
	all, err := e.store.ListRepositories(ctx)
	if err != nil {
		return nil, err
	}
	gp, err := e.newGit(CloneRefs(all))
	if err != nil || gp == nil {
		if err != nil {
			e.logger.Debug("clones Git locaux indisponibles (conventions)", "erreur", err.Error())
		}
		return nil, nil
	}
	cr, ok := gp.(conventions.CloneReader)
	if !ok {
		return nil, nil
	}
	return cr, nil
}

// ConventionsLocalOnly indique un espace en clone seul (synchro sans gh) : les pull requests et releases
// en base ne sont plus mises à jour, PR1, PR2 et SV6 sont donc « inconnus » (BuildOptions.LocalOnly).
// source lit la source des synchros (Engine.Source) ; si elle ne peut pas être lue, le mode API est
// supposé. Seul un contexte terminé est une erreur. Règle commune à la CLI et au web.
func ConventionsLocalOnly(ctx context.Context, source func(context.Context) (Source, error)) (bool, error) {
	src, err := source(ctx)
	if err != nil {
		return false, ctx.Err()
	}
	return src.Kind == SourceGit, nil
}
