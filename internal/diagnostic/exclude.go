package diagnostic

import (
	"fmt"
	"path"
	"strings"
)

// Excluder applique les motifs d'exclusion de fichiers (réglage diagnostic.exclude), syntaxe de
// path.Match (« * », « ? », « [...] », sans traverser « / »). Formes :
//
//   - « nom/ » (barre finale, sans autre barre) : un répertoire de ce nom n'importe où dans le chemin
//     (« vendor/ » exclut « vendor/a.go » et « web/vendor/b.js » ; « *.egg-info/ » est accepté) ;
//   - « a/b/ » : ce qui est sous le répertoire a/b, à la racine ou plus bas ;
//   - « /nom/ », « /a/b/ » (barre initiale) : le répertoire à la racine du dépôt seulement ;
//   - motif sans « / » : comparé au nom du fichier (« *.pb.go », « go.sum ») ;
//   - motif avec « / » (sans barre finale) : comparé au chemin complet depuis la racine
//     (« web/static/*.js », « /go.sum »).
//
// La valeur zéro n'exclut rien.
type Excluder struct {
	anyDirs  []string   // « nom/ » : un élément de répertoire, n'importe où
	subDirs  [][]string // « a/b/ » : suite d'éléments de répertoire, n'importe où
	rootDirs [][]string // « /a/b/ » : suite d'éléments de répertoire, à la racine
	names    []string   // motifs sur le nom du fichier
	paths    []string   // motifs sur le chemin complet
}

// ValidatePattern vérifie un motif d'exclusion : non vide (espaces exclus), pas seulement « / », et
// chaque élément accepté par path.Match.
func ValidatePattern(p string) error {
	if strings.TrimSpace(p) == "" || strings.Trim(p, "/") == "" {
		return fmt.Errorf("empty pattern")
	}
	for _, seg := range strings.Split(strings.Trim(p, "/"), "/") {
		if seg == "" {
			return fmt.Errorf("empty path element")
		}
		if _, err := path.Match(seg, ""); err != nil {
			return err
		}
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
		root := strings.HasPrefix(p, "/")
		segs := strings.Split(strings.Trim(p, "/"), "/")
		switch {
		case strings.HasSuffix(p, "/") && root:
			e.rootDirs = append(e.rootDirs, segs)
		case strings.HasSuffix(p, "/") && len(segs) == 1:
			e.anyDirs = append(e.anyDirs, segs[0])
		case strings.HasSuffix(p, "/"):
			e.subDirs = append(e.subDirs, segs)
		case strings.Contains(p, "/"):
			e.paths = append(e.paths, strings.TrimPrefix(p, "/"))
		default:
			e.names = append(e.names, p)
		}
	}
	return e, nil
}

// Excluded indique si le fichier de chemin p (séparé par « / », relatif à la racine) est exclu.
func (e *Excluder) Excluded(p string) bool {
	if e == nil {
		return false
	}
	segs := strings.Split(p, "/")
	dirs := segs[:len(segs)-1] // éléments de répertoire du fichier
	for _, d := range e.anyDirs {
		for _, s := range dirs {
			if ok, _ := path.Match(d, s); ok {
				return true
			}
		}
	}
	for _, seq := range e.subDirs {
		for i := 0; i+len(seq) <= len(dirs); i++ {
			if matchSegs(seq, dirs[i:i+len(seq)]) {
				return true
			}
		}
	}
	for _, seq := range e.rootDirs {
		if len(seq) <= len(dirs) && matchSegs(seq, dirs[:len(seq)]) {
			return true
		}
	}
	base := segs[len(segs)-1]
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

// matchSegs indique si chaque élément de got correspond au motif de même rang de pattern.
func matchSegs(pattern, got []string) bool {
	for i := range pattern {
		if ok, _ := path.Match(pattern[i], got[i]); !ok {
			return false
		}
	}
	return true
}
