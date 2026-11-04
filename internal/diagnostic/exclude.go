package diagnostic

import (
	"fmt"
	"path"
	"strings"
)

// Excluder applique les motifs d'exclusion de fichiers (réglage diagnostic.exclude). Trois formes :
//
//   - « nom/ » (barre finale) : un répertoire, n'importe où dans le chemin (« vendor/ » exclut
//     « vendor/a.go » et « web/vendor/b.js ») ; « a/b/ » exclut ce qui est sous « a/b/ », à la racine
//     ou plus bas ;
//   - motif sans « / » : comparé au nom du fichier, syntaxe de path.Match (« *.pb.go », « go.sum ») ;
//   - motif avec « / » : comparé au chemin complet, syntaxe de path.Match (« web/static/*.js »).
//
// La valeur zéro n'exclut rien.
type Excluder struct {
	dirs  []string // « nom/ » sans la barre finale
	names []string // motifs sur le nom du fichier
	paths []string // motifs sur le chemin complet
}

// ValidatePattern vérifie un motif d'exclusion : non vide (espaces exclus) et accepté par path.Match.
func ValidatePattern(p string) error {
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("empty pattern")
	}
	if _, err := path.Match(strings.TrimSuffix(p, "/"), ""); err != nil {
		return err
	}
	return nil
}

// NewExcluder construit un Excluder ; erreur au premier motif invalide (voir ValidatePattern).
func NewExcluder(patterns []string) (*Excluder, error) {
	e := &Excluder{}
	for _, p := range patterns {
		if err := ValidatePattern(p); err != nil {
			return nil, fmt.Errorf("pattern %q: %w", p, err)
		}
		switch {
		case strings.HasSuffix(p, "/"):
			e.dirs = append(e.dirs, strings.Trim(p, "/"))
		case strings.Contains(p, "/"):
			e.paths = append(e.paths, p)
		default:
			e.names = append(e.names, p)
		}
	}
	return e, nil
}

// Excluded indique si le fichier de chemin p (séparé par « / ») est exclu.
func (e *Excluder) Excluded(p string) bool {
	if e == nil {
		return false
	}
	for _, d := range e.dirs {
		if strings.HasPrefix(p, d+"/") || strings.Contains(p, "/"+d+"/") {
			return true
		}
	}
	base := path.Base(p)
	for _, n := range e.names {
		if ok, _ := path.Match(n, base); ok {
			return true
		}
	}
	for _, full := range e.paths {
		if ok, _ := path.Match(full, p); ok {
			return true
		}
	}
	return false
}
