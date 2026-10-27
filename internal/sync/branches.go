package sync

// Sélection des branches suivies et intervalle de synchro automatique.

import (
	"fmt"
	"path"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// SelectBranches renvoie, dans l'ordre reçu, les branches à suivre : celles dont le nom correspond à
// au moins un motif de patterns (path.Match ; patterns vide = toutes) et à aucun motif de exclude
// (branches.exclude). Un motif invalide ne correspond à rien. Liste vide non nil si aucune.
func SelectBranches(branches []provider.Branch, patterns, exclude []string) []provider.Branch {
	out := make([]provider.Branch, 0, len(branches))
	for _, b := range branches {
		if len(patterns) > 0 && !matchAny(patterns, b.Name) {
			continue
		}
		if matchAny(exclude, b.Name) {
			continue
		}
		out = append(out, b)
	}
	return out
}

// matchAny indique si name correspond à l'un des motifs (path.Match). Un motif invalide ou vide ne
// correspond à rien.
func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if ok, err := path.Match(p, name); err == nil && ok {
			return true
		}
	}
	return false
}

// ReferenceBranch renvoie l'indice dans tracked de la branche de référence : la branche par défaut
// si elle est suivie, sinon la première branche suivie ; -1 si tracked est vide.
func ReferenceBranch(tracked []provider.Branch, defaultBranch string) int {
	if len(tracked) == 0 {
		return -1
	}
	if defaultBranch != "" {
		for i, b := range tracked {
			if b.Name == defaultBranch {
				return i
			}
		}
	}
	return 0
}

// AutoInterval renvoie l'intervalle de synchro automatique de « watchoo tui » (et plus tard
// « watchoo serve ») : override s'il est fourni (option --interval, overridden vrai), sinon
// cfg.Interval si cfg.Auto, sinon 0. 0 = synchro manuelle seule. Une valeur négative, ou non nulle et
// inférieure à workspace.MinSyncInterval, donne une erreur en français (« --interval : 10m0s est
// inférieur au minimum de 15 min (0 pour la synchro manuelle seule) »).
func AutoInterval(cfg workspace.SyncConfig, override time.Duration, overridden bool) (time.Duration, error) {
	name, d := "sync.interval", cfg.Interval
	switch {
	case overridden:
		name, d = "--interval", override
	case !cfg.Auto:
		return 0, nil
	}
	minutes := int(workspace.MinSyncInterval / time.Minute)
	switch {
	case d < 0:
		return 0, fmt.Errorf("%s : %s est négatif (0 pour la synchro manuelle seule, sinon au moins %d min)",
			name, d, minutes)
	case d > 0 && d < workspace.MinSyncInterval:
		return 0, fmt.Errorf("%s : %s est inférieur au minimum de %d min (0 pour la synchro manuelle seule)",
			name, d, minutes)
	}
	return d, nil
}
