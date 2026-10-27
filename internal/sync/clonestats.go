package sync

// Statistiques de lignes par le clone Git local (« toujours cloner », 0.5.0,
// documentation/03-architecture.md, « Synchronisation incrémentale » et « Dépôts Git locaux ») : avec
// ou sans gh, chaque dépôt suivi a son clone dans repos/, qui donne les lignes ajoutées et supprimées
// de tous ses commits (premier import compris), sans plafond ni quota. Clone indisponible : repli sur
// les statistiques de l'API (fetchStats : premier import exclu, MaxStatsPerRepo, garde-fou de quota)
// et avertissement, sans faire échouer le dépôt.

import (
	"context"
	"errors"
	"fmt"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/store"
)

// unknownStatsPage est le nombre de SHA à statistiques inconnues lus en base à la fois.
const unknownStatsPage = 2000

// CloneStats est ce que le moteur demande au provider des clones Git locaux pour les statistiques
// de lignes (implémenté par *git.Provider) : créer ou mettre à jour le clone d'un dépôt en extrayant
// sa branche par défaut, puis calculer les statistiques d'un paquet de commits (SHA absents du clone
// omis du résultat).
type CloneStats interface {
	Prepare(ctx context.Context, owner, name, defaultBranch string) error
	CommitStatsBatch(ctx context.Context, owner, name string, shas []string) (map[string]provider.CommitStats, error)
}

// clones est l'accès aux clones Git locaux pendant une synchro, pour les statistiques.
type clones struct {
	stats CloneStats // nil : pas de clone (voir err)
	// err : provider des clones impossible à construire (avec gh) ; chaque dépôt reçoit alors un
	// avertissement et se replie sur l'API. nil avec stats nil : clones désactivés (tests sans
	// répertoire des clones), repli sur l'API sans avertissement.
	err error
}

// openClones renvoie l'accès aux clones d'une synchro de source src faite avec le Provider prov.
// Sans gh, prov est lui-même le provider des clones. Avec gh, le provider des clones est construit en
// plus (Options.NewGitProvider, mêmes clones que sans gh) ; une fabrique qui renvoie nil désactive
// les clones.
func (e *Engine) openClones(ctx context.Context, prov provider.Provider, src Source) clones {
	if src.Kind == SourceGit {
		cs, _ := prov.(CloneStats)
		return clones{stats: cs}
	}
	all, err := e.store.ListRepositories(ctx)
	if err != nil {
		return clones{err: err}
	}
	gp, err := e.newGit(CloneRefs(all))
	switch {
	case err != nil:
		e.logger.Debug("clones Git locaux indisponibles", "erreur", err.Error())
		return clones{err: err}
	case gp == nil:
		return clones{}
	}
	cs, _ := gp.(CloneStats)
	return clones{stats: cs}
}

// lineStats renseigne les statistiques de lignes des commits du dépôt : par le clone si possible
// (cloneStats), sinon par l'API (fetchStats, sauf au premier import). Renvoie le nombre de
// statistiques calculées ou demandées, les statistiques des commits déjà en base à écrire, et un
// avertissement (clone indisponible) ; erreur seulement si le contexte est terminé ou la base en
// échec.
func (s *repoSync) lineStats(ctx context.Context, h *halt, repoID int64, firstImport bool, defaultBranch string,
	cl clones, src Source) (int, []store.CommitStatsWrite, string, error) {
	cloneErr := cl.err
	if cl.stats != nil {
		n, writes, err := s.cloneStats(ctx, repoID, defaultBranch, cl.stats)
		if err == nil {
			return n, writes, "", nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, nil, "", fmt.Errorf("statistiques des commits : %w", ctxErr)
		}
		var storeErr *storeError
		if errors.As(err, &storeErr) {
			return 0, nil, "", storeErr.err
		}
		cloneErr = err
	}
	warning := ""
	if cloneErr != nil {
		s.e.logger.Info("clone Git local indisponible", "repo", s.full, "erreur", errorMessage(cloneErr))
		if src.Kind == SourceGit {
			return 0, nil, "statistiques de lignes non calculées : " + errorMessage(cloneErr), nil
		}
		warning = "clone Git local indisponible, statistiques de lignes lues par l'API (limitées) : " +
			errorMessage(cloneErr)
	}
	if firstImport {
		return 0, nil, warning, nil
	}
	n, err := s.fetchStats(ctx, h, repoID)
	return n, nil, warning, err
}

// storeError distingue un échec de la base (qui fait échouer le dépôt) d'un échec du clone (repli).
type storeError struct{ err error }

func (e *storeError) Error() string { return e.err.Error() }

// cloneStats crée ou met à jour le clone (branche par défaut defaultBranch) puis calcule les
// statistiques des nouveaux commits (renseignées dans s.commits) et de tous les commits du dépôt déjà
// en base dont les statistiques sont inconnues (renvoyées pour ApplyRepoSync), page par page.
func (s *repoSync) cloneStats(ctx context.Context, repoID int64, defaultBranch string, cs CloneStats) (int, []store.CommitStatsWrite, error) {
	if err := cs.Prepare(ctx, s.owner, s.name, defaultBranch); err != nil {
		return 0, nil, err
	}
	computed := 0

	// Nouveaux commits : enregistrés directement avec leurs statistiques.
	shas := make([]string, len(s.commits))
	for i, c := range s.commits {
		shas[i] = c.SHA
	}
	existing, err := s.e.store.ExistingCommitSHAs(ctx, repoID, shas)
	if err != nil {
		return 0, nil, &storeError{err}
	}
	fresh := make([]string, 0, len(shas))
	for _, sha := range shas {
		if !existing[sha] {
			fresh = append(fresh, sha)
		}
	}
	if len(fresh) > 0 {
		got, err := cs.CommitStatsBatch(ctx, s.owner, s.name, fresh)
		if err != nil {
			return 0, nil, err
		}
		for i := range s.commits {
			c := &s.commits[i]
			if st, ok := got[c.SHA]; ok && !existing[c.SHA] {
				c.Additions, c.Deletions, c.StatsKnown = st.Additions, st.Deletions, true
				computed++
			}
		}
	}

	// Commits déjà en base sans statistiques (base d'avant la 0.5.0, synchro sans clone…).
	var writes []store.CommitStatsWrite
	after := ""
	for {
		page, err := s.e.store.UnknownStatsSHAs(ctx, repoID, after, unknownStatsPage)
		if err != nil {
			return 0, nil, &storeError{err}
		}
		if len(page) == 0 {
			break
		}
		got, err := cs.CommitStatsBatch(ctx, s.owner, s.name, page)
		if err != nil {
			return 0, nil, err
		}
		for _, sha := range page {
			if st, ok := got[sha]; ok {
				writes = append(writes, store.CommitStatsWrite{SHA: sha, Additions: st.Additions, Deletions: st.Deletions})
			}
		}
		if len(page) < unknownStatsPage {
			break
		}
		after = page[len(page)-1]
	}
	computed += len(writes)
	if computed > 0 {
		s.e.logger.Debug("statistiques calculées par le clone", "repo", s.full, "commits", computed)
	}
	return computed, writes, nil
}
