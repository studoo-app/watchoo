package git

// Lignes ajoutées des commits (phase 7.5 du diagnostic, signaux de forme) : lues en flux sur le clone
// existant, sans les garder ni les écrire dans les logs (code d'un dépôt privé).

import (
	"bytes"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// LinesBatchSize est le nombre de SHA traités par paquet dans AddedLines.
const LinesBatchSize = 100

// maxDiffLine : au-delà, la fin d'une ligne de diff est ignorée (code minifié, données) ; la ligne est
// transmise tronquée, et l'appelant l'écarte sur sa longueur.
const maxDiffLine = 4096

// AddedLines appelle fn pour chaque ligne AJOUTÉE des commits shas (SHA complets) du clone EXISTANT de
// owner/name (sans le créer ni le mettre à jour : aucun accès réseau), au premier parent (arbre vide pour
// un commit racine), renommages détectés, sans contexte (-U0), fichiers binaires ignorés. fn reçoit le
// commit, le chemin du fichier, la ligne (sans le « + » ni la fin de ligne ; au plus maxDiffLine octets)
// et hunk, vrai à la première ligne de chaque bloc de changement. Les commits de fusion sont lus au
// premier parent comme les autres ; un SHA absent du clone est ignoré. Erreur provider.ErrNotFound sans
// clone. Les lignes ne sont ni gardées ni journalisées.
func (p *Provider) AddedLines(ctx context.Context, owner, name string, shas []string,
	fn func(sha, path, line string, hunk bool)) error {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return err
	}
	for _, sha := range shas {
		if err := provider.ValidateSHA(sha); err != nil {
			return err
		}
	}
	op := i18n.T("provider.git.op.added_lines", owner, name)
	if !p.HasClone(owner, name) {
		return &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return err
	}
	unique := slices.Compact(slices.Sorted(slices.Values(shas)))
	for batch := range slices.Chunk(unique, LinesBatchSize) {
		if err := ctx.Err(); err != nil {
			return err
		}
		input, _, err := p.firstParents(ctx, op, path, batch)
		if err != nil {
			return err
		}
		if input == nil {
			continue
		}
		w := &patchWriter{fn: fn}
		res, err := p.runTo(ctx, p.localTimeout, gitDir(path), "diff-tree", input, w,
			"diff-tree", "--stdin", "--root", "-r", "-p", "-U0", "-M", "--no-color", "--no-ext-diff",
			"--no-textconv", "--src-prefix=a/", "--dst-prefix=b/")
		if err != nil {
			return &provider.APIError{Op: op, Err: err}
		}
		if res.exitCode != 0 {
			return localError(op, res)
		}
		w.flush()
	}
	return nil
}

// patchWriter lit la sortie de « diff-tree --stdin -p -U0 » ligne par ligne, au fil de l'eau : chaque
// commit commence par son SHA seul sur une ligne, puis un diff par fichier (en-têtes « diff --git »,
// « +++ b/chemin », blocs « @@ »).
type patchWriter struct {
	fn  func(sha, path, line string, hunk bool)
	buf []byte // début de la ligne en cours (au plus maxDiffLine octets)

	sha    string
	path   string // fichier en cours ; vide : supprimé, ou en-têtes pas encore lus
	inHunk bool
	first  bool // prochaine ligne ajoutée : première de son bloc
}

func (w *patchWriter) Write(b []byte) (int, error) {
	n := len(b)
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		chunk := b
		if i >= 0 {
			chunk = b[:i]
		}
		if room := maxDiffLine - len(w.buf); room > 0 {
			w.buf = append(w.buf, chunk[:min(len(chunk), room)]...)
		}
		if i < 0 {
			break
		}
		w.line(w.buf)
		w.buf = w.buf[:0]
		b = b[i+1:]
	}
	return n, nil
}

// flush traite une dernière ligne sans fin de ligne.
func (w *patchWriter) flush() {
	if len(w.buf) > 0 {
		w.line(w.buf)
		w.buf = w.buf[:0]
	}
}

// line traite une ligne complète de la sortie.
func (w *patchWriter) line(l []byte) {
	if w.inHunk {
		switch {
		case len(l) > 0 && l[0] == '+':
			if w.path != "" {
				w.fn(w.sha, w.path, strings.TrimSuffix(string(l[1:]), "\r"), w.first)
				w.first = false
			}
			return
		case len(l) > 0 && (l[0] == '-' || l[0] == '\\' || l[0] == ' '):
			return
		case bytes.HasPrefix(l, []byte("@@")):
			w.first = true
			return
		}
		w.inHunk = false
	}
	s := string(l)
	switch {
	case bytes.HasPrefix(l, []byte("@@")):
		w.inHunk, w.first = true, true
	case strings.HasPrefix(s, "diff --git "):
		w.path = ""
	case strings.HasPrefix(s, "+++ "):
		w.path = diffPath(strings.TrimPrefix(s, "+++ "))
	case provider.ValidateSHA(s) == nil:
		w.sha, w.path = s, ""
	}
}

// diffPath renvoie le chemin d'un en-tête « +++ b/chemin » (guillemets à la manière du C si le chemin
// contient des caractères spéciaux, tabulation finale si le chemin contient une espace) ; vide pour
// « /dev/null » (fichier supprimé) ou un en-tête illisible.
func diffPath(h string) string {
	h = strings.TrimSuffix(h, "\t")
	if strings.HasPrefix(h, `"`) {
		u, err := strconv.Unquote(h)
		if err != nil {
			return ""
		}
		h = u
	}
	p, ok := strings.CutPrefix(h, "b/")
	if !ok {
		return ""
	}
	return p
}
