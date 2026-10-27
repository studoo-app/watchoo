package git

// Clones de Options.Dir (le répertoire repos/ à la racine de l'espace) : emplacement (règle des
// homonymes), création, mise à jour (« git fetch » puis copie de travail remise sur la branche par
// défaut) et suppression.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	gosync "sync"

	"github.com/studoo-app/watchoo/internal/provider"
)

// dirMode est le mode du répertoire des clones (repos/) et de chaque clone à sa création.
const dirMode = 0o700

// remotePrefix est l'espace des branches du dépôt distant dans un clone (« origin/main »).
const remotePrefix = "refs/remotes/origin/"

// remoteHEAD désigne la branche par défaut du dépôt distant dans un clone.
const remoteHEAD = remotePrefix + "HEAD"

// markerFile, dans le répertoire .git d'un clone, contient « owner/name » (en minuscules) : il signe
// les clones créés par Watchoo. Un répertoire sans ce fichier, ou signé pour un autre dépôt, n'est
// jamais mis à jour, déplacé ni supprimé.
const markerFile = "watchoo-repository"

// Préfixes des répertoires temporaires (clone en cours, déplacement) : « + » n'est permis ni dans un
// nom de dépôt ni dans un propriétaire, aucun clone ne peut donc porter ce nom.
const (
	tmpClonePattern = ".clone+*"
	tmpMovePattern  = ".move+*"
)

// fetchRefspecs sont les références mises à jour par « git fetch » : branches du distant dans
// refs/remotes/origin/ et tags, remplacés de force (historique réécrit) ; « --prune » retire ceux qui
// ont disparu du dépôt distant.
var fetchRefspecs = []string{"+refs/heads/*:" + remotePrefix + "*", "+refs/tags/*:refs/tags/*"}

// Ref désigne un dépôt suivi (owner/name), pour la règle des homonymes (NewLayout).
type Ref struct {
	Owner, Name string
}

// Layout range les clones dans un répertoire selon la règle des homonymes
// (documentation/03-architecture.md, « Dépôts Git locaux ») : les dépôts sont pris dans l'ordre donné
// (celui de leur ajout, identifiant croissant) et chacun reçoit le premier nom libre parmi
//
//	<name>, <owner>-<name>, <owner>-<name>-2, <owner>-<name>-3…
//
// tout en minuscules (GitHub, comme les systèmes de fichiers de macOS et Windows, ignore la casse) ;
// « libre » : pas déjà donné à un dépôt précédent, sans tenir compte de la casse. Le nom « .git »
// n'est jamais donné. Un dépôt absent de la liste est rangé comme s'il venait à la suite (ajout).
//
// La règle ne dépend que de la liste : ajouter un dépôt ne change l'emplacement d'aucun autre ;
// supprimer un dépôt peut libérer un nom (RemoveClone déplace alors les clones concernés). Sûr pour
// un usage concurrent.
type Layout struct {
	root  string
	mu    gosync.Mutex
	dirs  map[string]string // clé : owner/name en minuscules ; valeur : nom du répertoire
	taken map[string]bool   // noms de répertoires donnés
}

// NewLayout calcule les emplacements des clones des dépôts repos (dans l'ordre de leur ajout) sous
// root. Les références invalides sont ignorées.
func NewLayout(root string, repos []Ref) *Layout {
	l := &Layout{root: root, dirs: make(map[string]string), taken: make(map[string]bool)}
	for _, r := range repos {
		if provider.ValidateRepo(r.Owner, r.Name) == nil {
			l.assign(r.Owner, r.Name)
		}
	}
	return l
}

// assign renvoie le nom du répertoire du clone de owner/name (déjà validés), en le donnant s'il ne
// l'est pas encore. Appelé avec l.mu tenu, ou pendant NewLayout.
func (l *Layout) assign(owner, name string) string {
	owner, name = strings.ToLower(owner), strings.ToLower(name)
	key := owner + "/" + name
	if d, ok := l.dirs[key]; ok {
		return d
	}
	for i := 0; ; i++ {
		var c string
		switch i {
		case 0:
			c = name
		case 1:
			c = owner + "-" + name
		default:
			c = owner + "-" + name + "-" + strconv.Itoa(i)
		}
		if c == ".git" || l.taken[c] {
			continue
		}
		l.taken[c] = true
		l.dirs[key] = c
		return c
	}
}

// Path renvoie l'emplacement du clone de owner/name. Erreur enveloppant provider.ErrInvalidRef si
// owner ou name est invalide (aucun chemin hors du répertoire des clones).
func (l *Layout) Path(owner, name string) (string, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return "", err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return filepath.Join(l.root, l.assign(owner, name)), nil
}

// marker renvoie la signature d'un clone de owner/name.
func marker(owner, name string) string {
	return strings.ToLower(owner + "/" + name)
}

// cloneState décrit ce qui se trouve à l'emplacement d'un clone.
type cloneState int

const (
	stateAbsent   cloneState = iota // rien
	stateEmpty                      // répertoire vide (remplaçable)
	stateOwned                      // clone créé par Watchoo pour ce dépôt
	stateOccupied                   // autre chose : jamais touché
)

// inspect dit ce qui se trouve à path pour le dépôt owner/name. Un lien symbolique, un fichier, un
// répertoire non vide sans signature ou signé pour un autre dépôt est « occupé ».
func inspect(path, owner, name string) (cloneState, error) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return stateAbsent, nil
	case err != nil:
		return stateOccupied, err
	case !info.IsDir():
		return stateOccupied, nil
	}
	if isClone(path) {
		got, err := os.ReadFile(filepath.Join(path, ".git", markerFile))
		if err == nil && strings.TrimSpace(string(got)) == marker(owner, name) {
			return stateOwned, nil
		}
		return stateOccupied, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return stateOccupied, err
	}
	if len(entries) == 0 {
		return stateEmpty, nil
	}
	return stateOccupied, nil
}

// isClone indique si path ressemble à une copie de travail Git : un vrai répertoire .git (ni lien,
// ni fichier « gitdir: » vers un autre dépôt) avec un fichier HEAD et un répertoire objects.
func isClone(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Lstat(gitDir)
	if err != nil || !info.IsDir() {
		return false
	}
	head, err := os.Lstat(filepath.Join(gitDir, "HEAD"))
	if err != nil || !head.Mode().IsRegular() {
		return false
	}
	objects, err := os.Lstat(filepath.Join(gitDir, "objects"))
	return err == nil && objects.IsDir()
}

// RemoveClone supprime le clone du dépôt owner/name de dir, s'il existe et a été créé par Watchoo
// pour ce dépôt (« repo remove »). repos est la liste des dépôts suivis AVANT le retrait, dans l'ordre
// de leur ajout (owner/name compris) : le nom libéré peut revenir à un homonyme (règle de Layout),
// dont le clone est alors déplacé. Ne touche à rien hors de dir, ni à un répertoire qui n'est pas un
// clone de Watchoo.
func RemoveClone(dir string, repos []Ref, owner, name string) error {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return err
	}
	before := NewLayout(dir, repos)
	path, err := before.Path(owner, name)
	if err != nil {
		return err
	}
	switch state, err := inspect(path, owner, name); {
	case err != nil:
		return fmt.Errorf("suppression du clone de %s/%s : %w", owner, name, err)
	case state == stateOwned:
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("suppression du clone de %s/%s : %w", owner, name, err)
		}
	}
	var rest []Ref
	for _, r := range repos {
		if !strings.EqualFold(r.Owner, owner) || !strings.EqualFold(r.Name, name) {
			rest = append(rest, r)
		}
	}
	if err := relocate(dir, before, NewLayout(dir, rest), rest); err != nil {
		return fmt.Errorf("suppression du clone de %s/%s : %w", owner, name, err)
	}
	return nil
}

// relocate déplace les clones des dépôts repos dont l'emplacement passe de before à after, en deux
// temps (vers un nom temporaire, puis vers la destination) pour que deux clones puissent échanger
// leurs places. Une destination occupée par autre chose fait supprimer le clone déplacé : il sera
// refait à la synchro suivante, sans jamais écraser ce qui s'y trouve.
func relocate(dir string, before, after *Layout, repos []Ref) error {
	type move struct{ tmp, dst string }
	var moves []move
	var errs []error
	for _, r := range repos {
		src, err1 := before.Path(r.Owner, r.Name)
		dst, err2 := after.Path(r.Owner, r.Name)
		if err1 != nil || err2 != nil || src == dst {
			continue
		}
		if state, err := inspect(src, r.Owner, r.Name); err != nil || state != stateOwned {
			continue
		}
		tmp, err := os.MkdirTemp(dir, tmpMovePattern)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// MkdirTemp a créé tmp : le renommage d'un répertoire sur un répertoire vide est permis sous
		// Unix, pas sous Windows ; on le retire d'abord.
		if err := os.Remove(tmp); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.Rename(src, tmp); err != nil {
			errs = append(errs, err)
			continue
		}
		moves = append(moves, move{tmp: tmp, dst: dst})
	}
	for _, m := range moves {
		if _, err := os.Lstat(m.dst); err == nil {
			errs = append(errs, os.RemoveAll(m.tmp))
			continue
		}
		if err := os.Rename(m.tmp, m.dst); err != nil {
			errs = append(errs, err, os.RemoveAll(m.tmp))
		}
	}
	return errors.Join(errs...)
}

// state renvoie l'état de mise à jour du dépôt owner/name pour ce Provider.
func (p *Provider) state(owner, name string) *repoState {
	key := marker(owner, name)
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.repos[key]
	if !ok {
		st = &repoState{}
		p.repos[key] = st
	}
	return st
}

// ErrCloneDirOccupied : l'emplacement du clone est pris par autre chose qu'un clone de Watchoo pour
// ce dépôt (répertoire de l'utilisateur, clone d'un autre dépôt) ; Watchoo n'y touche pas.
var ErrCloneDirOccupied = errors.New("emplacement du clone occupé par un répertoire que Watchoo n'a pas créé pour ce dépôt")

// ensure renvoie le chemin du clone de owner/name (déjà validés), après l'avoir créé ou mis à jour
// une fois pour ce Provider. Les appels concurrents pour un même dépôt attendent la même mise à jour ;
// un échec sera retenté au prochain appel.
func (p *Provider) ensure(ctx context.Context, owner, name string) (string, error) {
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return "", err
	}
	st := p.state(owner, name)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.fetched {
		return path, nil
	}
	op := fmt.Sprintf("clone du dépôt %s/%s", owner, name)
	state, err := inspect(path, owner, name)
	if err != nil {
		return "", &provider.APIError{Op: op, Err: err}
	}
	switch state {
	case stateOwned:
		err = p.update(ctx, owner, name, path)
	case stateEmpty:
		if err := os.Remove(path); err != nil {
			return "", &provider.APIError{Op: op, Err: err}
		}
		err = p.clone(ctx, owner, name, path)
	case stateAbsent:
		err = p.clone(ctx, owner, name, path)
	default:
		return "", &provider.APIError{Op: fmt.Sprintf("%s dans %s", op, filepath.Base(path)), Err: ErrCloneDirOccupied}
	}
	if err != nil {
		return "", err
	}
	st.fetched = true
	return path, nil
}

// clone crée le clone de owner/name à path : dépôt créé dans un répertoire temporaire voisin, signé,
// rempli par update, puis renommé (un clone interrompu ne laisse jamais un clone à moitié fait à
// path). Le dépôt est créé par « git init » puis « git fetch » plutôt que par « git clone » : une
// seule façon de lire le distant, et aucun remote enregistré (l'URL est passée à chaque fetch).
func (p *Provider) clone(ctx context.Context, owner, name, path string) error {
	op := fmt.Sprintf("clone du dépôt %s/%s", owner, name)
	if err := mkdirPrivate(p.dir); err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	tmp, err := os.MkdirTemp(p.dir, tmpClonePattern)
	if err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	defer func() { _ = os.RemoveAll(tmp) }() // sans effet après le renommage
	res, err := p.run(ctx, p.localTimeout, "", "init", "init", "--quiet", "--", tmp)
	if err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return localError(op, res)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".git", markerFile), []byte(marker(owner, name)+"\n"), 0o600); err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	if err := p.update(ctx, owner, name, tmp); err != nil {
		return err
	}
	if err := os.Chmod(tmp, dirMode); err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	if err := os.Rename(tmp, path); err != nil {
		if state, _ := inspect(path, owner, name); state == stateOwned {
			return nil // un autre processus vient de le cloner
		}
		return &provider.APIError{Op: op, Err: err}
	}
	return nil
}

// update met à jour le clone path : fetch, puis copie de travail remise sur la branche par défaut du
// distant (checkout).
func (p *Provider) update(ctx context.Context, owner, name, path string) error {
	if err := p.fetch(ctx, owner, name, path); err != nil {
		return err
	}
	return p.checkout(ctx, owner, name, path)
}

// fetch met à jour les branches (refs/remotes/origin/) et les tags du clone path depuis le dépôt
// distant (l'URL est passée à chaque fois : aucun remote enregistré n'est utilisé, et un changement de
// git.protocol s'applique), sans sous-module.
func (p *Provider) fetch(ctx context.Context, owner, name, path string) error {
	op := fmt.Sprintf("mise à jour du dépôt %s/%s", owner, name)
	args := []string{"fetch", "--quiet", "--prune", "--no-tags", "--no-write-fetch-head",
		"--no-recurse-submodules", "--", p.remoteURL(owner, name)}
	args = append(args, fetchRefspecs...)
	res, err := p.run(ctx, p.networkTimeout, gitDir(path), "fetch", args...)
	if err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return remoteError(op, res)
	}
	return nil
}

// checkout remet la copie de travail du clone path sur la branche par défaut du distant : la
// branche locale <défaut> est recréée sur origin/<défaut> (« checkout --force -B »), puis
// « reset --hard » efface toute modification des fichiers suivis (et un merge en cours). Les fichiers
// non suivis sont gardés (pas de « git clean »). origin/HEAD désigne ensuite origin/<défaut>. Dépôt
// vide (ou branche par défaut absente) : rien à extraire, la copie de travail reste telle quelle.
func (p *Provider) checkout(ctx context.Context, owner, name, path string) error {
	op := fmt.Sprintf("mise à jour de la copie de travail de %s/%s", owner, name)
	branch, err := p.defaultBranch(ctx, owner, name)
	if err != nil {
		return err
	}
	if branch == "" || provider.ValidateBranch(branch) != nil {
		return nil
	}
	remote := remotePrefix + branch
	if _, found, err := p.resolve(ctx, op, path, remote); err != nil || !found {
		return err
	}
	tree := "--work-tree=" + path
	for _, args := range [][]string{
		{"symbolic-ref", remoteHEAD, remote},
		{tree, "checkout", "--quiet", "--force", "--no-recurse-submodules", "-B", branch, remote, "--"},
		{tree, "reset", "--quiet", "--hard", "--no-recurse-submodules", remote, "--"},
	} {
		cmd := args[0]
		if cmd == tree {
			cmd = args[1]
		}
		res, err := p.run(ctx, p.localTimeout, gitDir(path), cmd, args...)
		if err != nil {
			return &provider.APIError{Op: op, Err: err}
		}
		if res.exitCode != 0 {
			return localError(op, res)
		}
	}
	return nil
}

// defaultBranch renvoie la branche par défaut du distant : celle lue par Repository pendant la vie
// de ce Provider, sinon un nouveau « ls-remote ».
func (p *Provider) defaultBranch(ctx context.Context, owner, name string) (string, error) {
	p.mu.Lock()
	branch, ok := p.defaults[marker(owner, name)]
	p.mu.Unlock()
	if ok {
		return branch, nil
	}
	meta, err := p.Repository(ctx, owner, name)
	if err != nil {
		return "", err
	}
	return meta.DefaultBranch, nil
}

// gitDir renvoie le répertoire .git du clone path, passé à git par --git-dir : git ne cherche
// jamais le dépôt lui-même (un clone abîmé ne le ferait pas remonter jusqu'à un dépôt parent).
func gitDir(path string) string {
	return filepath.Join(path, ".git")
}

// mkdirPrivate crée dir (et ses parents) en 0700 et lui remet ce mode s'il existait : il ne sert
// qu'au répertoire des clones (repos/), à l'utilisateur seul.
func mkdirPrivate(dir string) error {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	return os.Chmod(dir, dirMode)
}
