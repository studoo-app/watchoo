package sync

// Conventions (phase 8.1) : accès en lecture aux clones Git locaux pour le périmètre précis du rapport
// (conventions.Build, calculé à la lecture par la CLI et le web). Aucun fetch, aucune écriture dans le clone.
//
// Phase 8.3 : critères de fichiers (CHANGELOG, README, documentation, outillage) mesurés à la synchro sur
// le clone qui vient d'être mis à jour, par ses objets Git, et mis en cache (repository_conventions).

import (
	"context"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/model"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
)

// Vérifie à la compilation que le provider des clones sait lire le périmètre des conventions.
var _ conventions.CloneReader = (*gitprov.Provider)(nil)

// Vérifie à la compilation que le provider des clones sait lire les fichiers d'une révision (phase 8.3).
var _ conventions.FileReader = (*gitprov.Provider)(nil)

// Vérifie à la compilation que le provider des clones sait lire l'historique (phase 8.4 : SV4, SV5, SV7,
// CL10).
var _ conventions.HistoryReader = (*gitprov.Provider)(nil)

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

// updateConventionFiles met à jour le cache des critères de fichiers des conventions du dépôt (phase 8.3),
// seulement si les conventions sont activées et que le clone est à jour pour cette synchro : la tête
// headSHA de la branche par défaut et le tag de chaque jalon configuré (tags en base), recalculés
// seulement si le commit, la version des règles de fichiers ou l'empreinte des réglages ont changé ; les
// lignes des tags qui ne désignent plus de jalon sont supprimées, celle de la tête aussi quand la tête est
// inconnue (headSHA vide). Aucun appel à l'API. Un échec, de lecture comme d'accès à la base, est
// seulement journalisé (Debug, sans chemin, nom de tag ni contenu) : la synchro du dépôt réussit.
func (e *Engine) updateConventionFiles(ctx context.Context, s *repoSync, repoID int64, headSHA string) {
	if !e.cfg.Conventions.Enabled || s.clone == nil || ctx.Err() != nil {
		return
	}
	fr, ok := s.clone.(conventions.FileReader)
	if !ok {
		return
	}
	tags, err := e.store.ConventionTags(ctx, repoID)
	if err != nil {
		e.logger.Debug("critères de fichiers des conventions non calculés", "repo", s.full, "erreur", errorMessage(err))
		return
	}
	ms, err := e.cfg.Conventions.Milestones()
	if err != nil {
		ms = nil
	}
	rules := e.cfg.Conventions.FileRules()
	hash := conventions.FileConfigHash(rules, s.owner+"/"+s.name, tags)
	targets := conventions.FileTargets(headSHA, tags, ms)
	rows, err := e.store.RepositoryConventions(ctx, repoID)
	if err != nil {
		e.logger.Debug("critères de fichiers des conventions non calculés", "repo", s.full, "erreur", errorMessage(err))
		return
	}
	cached := make(map[string]model.RepositoryConventions, len(rows))
	for _, r := range rows {
		cached[r.RevKind+"/"+r.TagName] = r
	}
	keep := []string{}
	for _, t := range targets {
		if t.Kind == model.ConventionRevTag {
			keep = append(keep, t.Tag)
		}
	}
	if _, err := e.store.DeleteRepositoryConventionTags(ctx, repoID, keep); err != nil {
		e.logger.Debug("critères de fichiers des conventions non enregistrés", "repo", s.full, "erreur", errorMessage(err))
		return
	}
	// Tête inconnue (dépôt vide, branche par défaut absente) : le résultat précédent ne vaut plus.
	if headSHA == "" {
		if _, err := e.store.DeleteRepositoryConventionHead(ctx, repoID); err != nil {
			e.logger.Debug("critères de fichiers des conventions non enregistrés", "repo", s.full, "erreur", errorMessage(err))
			return
		}
	}

	now := e.now().UTC()
	var computed, failed int
	var firstErr error
	for _, t := range targets {
		if r, ok := cached[t.Kind+"/"+t.Tag]; ok && conventions.FileCacheFresh(r, t.SHA, hash) {
			continue
		}
		raw, err := e.computeConventionFiles(ctx, fr, s, t, tags, rules, now)
		if err != nil {
			failed++
			firstErr = firstError(firstErr, err)
			continue
		}
		err = e.store.SaveRepositoryConventions(ctx, model.RepositoryConventions{
			RepoID: repoID, RevKind: t.Kind, TagName: t.Tag, RevSHA: t.SHA,
			RulesVersion: conventions.FileRulesVersion, ConfigHash: hash, Results: raw, ComputedAt: now,
		})
		if err != nil {
			e.logger.Debug("critères de fichiers des conventions non enregistrés", "repo", s.full, "erreur", errorMessage(err))
			return
		}
		computed++
	}
	if failed > 0 {
		e.logger.Debug("critères de fichiers des conventions non calculés", "repo", s.full, "revisions", failed,
			"erreur", errorMessage(firstErr))
	}
	if computed > 0 {
		e.logger.Debug("critères de fichiers des conventions calculés", "repo", s.full, "revisions", computed)
	}
}

// computeConventionFiles lit l'arbre de la révision t dans le clone et en mesure les critères de fichiers ;
// renvoie le résultat encodé (codes, états, compteurs). Les chemins et contenus ne restent qu'en mémoire.
func (e *Engine) computeConventionFiles(ctx context.Context, fr conventions.FileReader, s *repoSync,
	t conventions.FileTarget, tags []model.Tag, rules conventions.FileRules, now time.Time) (string, error) {
	tree, err := conventions.ReadFileTree(ctx, fr, s.owner, s.name, t.SHA, rules)
	if err != nil {
		return "", err
	}
	res, _ := conventions.CheckFiles(tree, conventions.FileOptionsFor(t, s.owner+"/"+s.name, tags, rules,
		now.In(e.conventionsLocation())))
	return conventions.EncodeFileResults(res)
}

// conventionsLocation renvoie le fuseau de l'espace (jour des dates futures de CL4) ; UTC s'il ne se lit pas.
func (e *Engine) conventionsLocation() *time.Location {
	loc, err := e.cfg.Location()
	if err != nil || loc == nil {
		return time.UTC
	}
	return loc
}

// firstError garde la première erreur.
func firstError(first, err error) error {
	if first != nil {
		return first
	}
	return err
}
