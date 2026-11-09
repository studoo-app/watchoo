package git

// Lectures des fichiers d'une révision dans le clone existant, par les objets Git seulement (jamais la
// copie de travail de repos/) : critères de fichiers des conventions (CHANGELOG, README, documentation,
// outillage ; phase 8.3). Aucun accès réseau, aucune écriture.

import (
	"bytes"
	"context"
	"strconv"
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// Plafonds des lectures de fichiers.
const (
	// MaxTreePaths borne le nombre de chemins renvoyés par TreePaths ; au-delà, la liste est tronquée.
	MaxTreePaths = 20000
	// MaxBlobBytes borne la taille d'un fichier lu par ReadBlobs ; au-delà, il est « trop gros ».
	MaxBlobBytes = 1 << 20
	// MaxBlobsTotalBytes borne le total lu par un appel à ReadBlobs ; les fichiers au-delà ne sont pas lus.
	MaxBlobsTotalBytes = 32 << 20
	// maxTreeRecord borne un enregistrement de « ls-tree » gardé en mémoire (chemin démesuré : ignoré).
	maxTreeRecord = 64 << 10
)

// treeEntry est un fichier ordinaire de l'arbre : son objet et sa taille.
type treeEntry struct {
	oid  string
	size int64
}

// TreePaths renvoie les chemins des fichiers ordinaires (ni liens symboliques ni sous-modules) de l'arbre
// du commit rev (SHA complet) dans le clone EXISTANT de owner/name, relatifs à la racine et séparés par
// « / », dans l'ordre de git, sans mise à jour (aucun accès réseau) :
//
//	git ls-tree -r -l -z --end-of-options <rev>
//
// Au-delà de MaxTreePaths chemins, la liste est coupée et truncated est vrai. Un chemin qui n'est pas de
// l'UTF-8 valide ou qui contient un caractère de contrôle est ignoré. Sans clone, ou si le commit n'est
// pas dans le clone : provider.ErrNotFound. Les chemins ne sont jamais écrits dans les logs ni les erreurs.
func (p *Provider) TreePaths(ctx context.Context, owner, name, rev string) (paths []string, truncated bool, err error) {
	list := &pathList{limit: MaxTreePaths, paths: []string{}}
	if err := p.walkTree(ctx, owner, name, rev, list.add); err != nil {
		return nil, false, err
	}
	return list.paths, list.truncated, nil
}

// pathList garde au plus limit chemins ; au-delà, truncated est vrai et la lecture s'arrête.
type pathList struct {
	limit     int
	paths     []string
	truncated bool
}

func (l *pathList) add(file string, _ treeEntry) bool {
	if len(l.paths) >= l.limit {
		l.truncated = true
		return false
	}
	l.paths = append(l.paths, file)
	return true
}

// ReadBlobs lit, dans l'arbre du commit rev (SHA complet) du clone EXISTANT de owner/name, le contenu des
// fichiers ordinaires paths, par leurs objets, sans filtre ni conversion et sans mise à jour (aucun accès
// réseau) :
//
//	git ls-tree -r -l -z --end-of-options <rev>
//	git cat-file --batch   (entrée : les identifiants des objets, jamais les chemins)
//
// fn est appelé une fois par fichier lu, avec son contenu ; un fichier de plus de MaxBlobBytes octets
// est signalé trop gros (tooLarge vrai, data nil), sans être lu. Un chemin absent de l'arbre, un lien
// symbolique, un sous-module, ou un fichier au-delà de MaxBlobsTotalBytes lus en tout ne donnent aucun
// appel. Un chemin vide, qui n'est pas de l'UTF-8 valide ou qui contient un caractère de contrôle :
// provider.ErrInvalidRef (sans citer le chemin). Sans clone, ou si le commit n'est pas dans le clone :
// provider.ErrNotFound.
func (p *Provider) ReadBlobs(ctx context.Context, owner, name, rev string, paths []string,
	fn func(path string, data []byte, tooLarge bool)) error {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return err
	}
	if err := provider.ValidateSHA(rev); err != nil {
		return err
	}
	want := make(map[string]bool, len(paths))
	for _, file := range paths {
		if err := provider.ValidatePath(file); err != nil {
			return err
		}
		want[file] = true
	}
	found := make(map[string]treeEntry, len(want))
	if len(want) > 0 {
		err := p.walkTree(ctx, owner, name, rev, func(file string, e treeEntry) bool {
			if want[file] {
				found[file] = e
			}
			return len(found) < len(want)
		})
		if err != nil {
			return err
		}
	} else if err := p.walkTree(ctx, owner, name, rev, func(string, treeEntry) bool { return false }); err != nil {
		return err
	}

	// Fichiers à lire, dans l'ordre demandé, dans la limite du total ; un objet partagé est lu une fois.
	byOID := map[string][]string{}
	var oids []string
	var total int64
	for _, file := range paths {
		e, ok := found[file]
		if !ok || !want[file] {
			continue
		}
		want[file] = false // chemin demandé deux fois : un seul appel
		if e.size > MaxBlobBytes {
			fn(file, nil, true)
			continue
		}
		if total+e.size > MaxBlobsTotalBytes {
			continue
		}
		total += e.size
		if _, seen := byOID[e.oid]; !seen {
			oids = append(oids, e.oid)
		}
		byOID[e.oid] = append(byOID[e.oid], file)
	}
	if len(oids) == 0 {
		return nil
	}
	op := i18n.T("provider.git.op.files", owner, name)
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return err
	}
	res, err := p.runInput(ctx, p.localTimeout, gitDir(path), "cat-file",
		[]byte(strings.Join(oids, "\n")+"\n"), "cat-file", "--batch")
	if err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return localError(op, res)
	}
	blobs, ok := parseCatFile(res.stdout)
	if !ok {
		return localError(op, res)
	}
	for _, oid := range oids {
		data, ok := blobs[oid]
		if !ok {
			continue
		}
		for _, file := range byOID[oid] {
			fn(file, data, false)
		}
	}
	return nil
}

// walkTree parcourt les fichiers ordinaires de l'arbre du commit rev du clone existant de owner/name, en
// flux, jusqu'à ce que fn renvoie faux. Chemins invalides (ValidatePath), liens symboliques et
// sous-modules sont ignorés.
func (p *Provider) walkTree(ctx context.Context, owner, name, rev string, fn func(file string, e treeEntry) bool) error {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return err
	}
	if err := provider.ValidateSHA(rev); err != nil {
		return err
	}
	op := i18n.T("provider.git.op.files", owner, name)
	if !p.HasClone(owner, name) {
		return &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return err
	}
	if _, ok, err := p.resolve(ctx, op, path, rev); err != nil {
		return err
	} else if !ok {
		return &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	w := &treeWriter{fn: fn}
	res, err := p.runTo(ctx, p.localTimeout, gitDir(path), "ls-tree", nil, w,
		"ls-tree", "-r", "-l", "-z", "--end-of-options", rev)
	if err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	// git peut finir sur une erreur d'écriture (tube) si la lecture s'est arrêtée tôt : sans effet.
	if res.exitCode != 0 && !w.stopped {
		return localError(op, res)
	}
	return nil
}

// treeWriter lit en flux la sortie de « ls-tree -r -l -z » : un enregistrement
// « mode type objet taille<TAB>chemin<NUL> » par entrée.
type treeWriter struct {
	fn       func(file string, e treeEntry) bool
	pending  []byte
	skipping bool // enregistrement démesuré : ignoré jusqu'au prochain NUL
	stopped  bool // fn a demandé l'arrêt : la suite est ignorée
}

func (w *treeWriter) Write(b []byte) (int, error) {
	n := len(b)
	for len(b) > 0 && !w.stopped {
		i := bytes.IndexByte(b, 0)
		if i < 0 {
			if !w.skipping {
				w.pending = append(w.pending, b...)
				if len(w.pending) > maxTreeRecord {
					w.pending, w.skipping = w.pending[:0], true
				}
			}
			break
		}
		if !w.skipping {
			w.pending = append(w.pending, b[:i]...)
			if file, e, ok := parseTreeRecord(w.pending); ok && !w.fn(file, e) {
				w.stopped = true
			}
		}
		w.pending, w.skipping = w.pending[:0], false
		b = b[i+1:]
	}
	return n, nil
}

// parseTreeRecord lit un enregistrement de « ls-tree -r -l -z » ; faux pour un lien symbolique, un
// sous-module, un enregistrement illisible ou un chemin invalide.
func parseTreeRecord(rec []byte) (string, treeEntry, bool) {
	meta, file, ok := strings.Cut(string(rec), "\t")
	if !ok {
		return "", treeEntry{}, false
	}
	fields := strings.Fields(meta)
	if len(fields) != 4 || fields[1] != "blob" || fields[0] == "120000" {
		return "", treeEntry{}, false
	}
	size, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil || size < 0 || provider.ValidatePath(file) != nil {
		return "", treeEntry{}, false
	}
	return file, treeEntry{oid: fields[2], size: size}, true
}

// parseCatFile lit la sortie de « cat-file --batch » : « <objet> blob <taille>\n<contenu>\n » par objet
// lu, « <objet> missing\n » pour un objet absent. Faux si la sortie est mal formée.
func parseCatFile(raw []byte) (map[string][]byte, bool) {
	out := map[string][]byte{}
	for len(raw) > 0 {
		nl := bytes.IndexByte(raw, '\n')
		if nl < 0 {
			return nil, false
		}
		fields := strings.Fields(string(raw[:nl]))
		raw = raw[nl+1:]
		switch {
		case len(fields) == 2 && fields[1] == "missing":
			continue
		case len(fields) != 3:
			return nil, false
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || len(raw) < size+1 || raw[size] != '\n' {
			return nil, false
		}
		if fields[1] == "blob" {
			out[fields[0]] = bytes.Clone(raw[:size])
		}
		raw = raw[size+1:]
	}
	return out, true
}
