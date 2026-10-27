package git

// Clones nus de Options.Dir : emplacement, création (« git clone --bare »), mise à jour
// (« git fetch ») et suppression.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/studoo-app/watchoo/internal/provider"
)

// dirMode est le mode des répertoires de clones (.watchoo/repos et ses sous-répertoires).
const dirMode = 0o700

// fetchRefspecs sont les références mises à jour par « git fetch » : branches et tags, remplacés de
// force (historique réécrit) ; « --prune » retire ceux qui ont disparu du dépôt distant.
var fetchRefspecs = []string{"+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*"}

// ClonePath renvoie l'emplacement du clone nu du dépôt owner/name dans dir :
// dir/<owner en minuscules>/<name en minuscules>.git (GitHub ne tient pas compte de la casse). Erreur
// enveloppant provider.ErrInvalidRef si owner ou name est invalide (aucun chemin hors de dir).
func ClonePath(dir, owner, name string) (string, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return "", err
	}
	return filepath.Join(dir, strings.ToLower(owner), strings.ToLower(name)+".git"), nil
}

// RemoveClone supprime le clone du dépôt owner/name de dir, s'il existe (« repo remove »), puis le
// répertoire du propriétaire s'il est vide. Ne touche à rien hors de dir.
func RemoveClone(dir, owner, name string) error {
	path, err := ClonePath(dir, owner, name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("suppression du clone de %s/%s : %w", owner, name, err)
	}
	// Répertoire du propriétaire : supprimé seulement s'il est vide.
	parent := filepath.Dir(path)
	entries, err := os.ReadDir(parent)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("suppression du clone de %s/%s : %w", owner, name, err)
	case len(entries) == 0:
		if err := os.Remove(parent); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("suppression du clone de %s/%s : %w", owner, name, err)
		}
	}
	return nil
}

// state renvoie l'état de mise à jour du dépôt owner/name pour ce Provider.
func (p *Provider) state(owner, name string) *repoState {
	key := strings.ToLower(owner + "/" + name)
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.repos[key]
	if !ok {
		st = &repoState{}
		p.repos[key] = st
	}
	return st
}

// ensure renvoie le chemin du clone de owner/name (déjà validés), après l'avoir créé (clone) ou mis à
// jour (fetch) une fois pour ce Provider. Les appels concurrents pour un même dépôt attendent la même
// mise à jour ; un échec sera retenté au prochain appel.
func (p *Provider) ensure(ctx context.Context, owner, name string) (string, error) {
	path, err := ClonePath(p.dir, owner, name)
	if err != nil {
		return "", err
	}
	st := p.state(owner, name)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.fetched {
		return path, nil
	}
	if isBareRepo(path) {
		err = p.fetch(ctx, owner, name, path)
	} else {
		err = p.clone(ctx, owner, name, path)
	}
	if err != nil {
		return "", err
	}
	st.fetched = true
	return path, nil
}

// isBareRepo indique si path ressemble à un dépôt nu (fichier HEAD et répertoire objects).
func isBareRepo(path string) bool {
	head, err := os.Stat(filepath.Join(path, "HEAD"))
	if err != nil || head.IsDir() {
		return false
	}
	objects, err := os.Stat(filepath.Join(path, "objects"))
	return err == nil && objects.IsDir()
}

// clone crée le clone nu de owner/name à path : clonage dans un répertoire temporaire voisin, puis
// renommage (un clone interrompu ne laisse jamais un clone à moitié fait à path).
func (p *Provider) clone(ctx context.Context, owner, name, path string) error {
	op := fmt.Sprintf("clone du dépôt %s/%s", owner, name)
	parent := filepath.Dir(path)
	if err := mkdirPrivate(p.dir); err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	if err := mkdirPrivate(parent); err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	// Un reste de dépôt invalide (dossier vide, clone cassé) est remplacé.
	if _, err := os.Lstat(path); err == nil {
		if err := os.RemoveAll(path); err != nil {
			return &provider.APIError{Op: op, Err: err}
		}
	}
	tmp, err := os.MkdirTemp(parent, ".clone-*")
	if err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	defer func() { _ = os.RemoveAll(tmp) }() // sans effet après le renommage
	res, err := p.run(ctx, p.networkTimeout, "", "clone",
		"clone", "--bare", "--quiet", "--no-tags", "--", p.remoteURL(owner, name), tmp)
	if err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return remoteError(op, res)
	}
	// clone --bare ne lit que les branches ; les tags viennent par le fetch, avec les mêmes règles
	// qu'aux synchros suivantes.
	if err := p.fetch(ctx, owner, name, tmp); err != nil {
		return err
	}
	if err := os.Chmod(tmp, dirMode); err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	if err := os.Rename(tmp, path); err != nil {
		if isBareRepo(path) {
			return nil // un autre processus vient de le cloner
		}
		return &provider.APIError{Op: op, Err: err}
	}
	return nil
}

// fetch met à jour les branches et les tags du clone path depuis le dépôt distant (l'URL est passée
// à chaque fois : aucun remote enregistré n'est utilisé, et un changement de git.protocol s'applique).
func (p *Provider) fetch(ctx context.Context, owner, name, path string) error {
	op := fmt.Sprintf("mise à jour du dépôt %s/%s", owner, name)
	args := []string{"fetch", "--quiet", "--prune", "--no-tags", "--no-write-fetch-head", "--",
		p.remoteURL(owner, name)}
	args = append(args, fetchRefspecs...)
	res, err := p.run(ctx, p.networkTimeout, path, "fetch", args...)
	if err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return remoteError(op, res)
	}
	return nil
}

// mkdirPrivate crée dir (et ses parents) en 0700 et lui remet ce mode s'il existait : il ne sert
// qu'aux répertoires des clones (.watchoo/repos et ses sous-répertoires), à l'utilisateur seul.
func mkdirPrivate(dir string) error {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	return os.Chmod(dir, dirMode)
}
