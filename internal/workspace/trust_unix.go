//go:build unix

package workspace

import (
	"fmt"
	"os"
	"syscall"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// currentUID renvoie l'identifiant de l'utilisateur courant. C'est une variable pour que les tests
// puissent simuler un espace appartenant à un autre utilisateur.
var currentUID = os.Getuid

// checkTrusted vérifie que l'espace ws appartient à l'utilisateur courant et n'est accessible qu'à
// lui : .watchoo/ à son nom et sans droit pour le groupe ni les autres (0700 au plus), et de même pour
// config.yaml s'il existe (0600 au plus). Watchoo pose toujours ces droits, Git jamais : un .watchoo/
// commité dans un dépôt cloné (git add -f) est donc refusé avant que ses réglages soient lus, comme le
// safe.directory de Git. Le message cite le chemin et le remède (chmod, ou -C vers son propre espace).
func checkTrusted(ws *Workspace) error {
	if err := checkOwnedPrivate(ws.Dir, true); err != nil {
		return err
	}
	return checkOwnedPrivate(ws.ConfigPath(), false)
}

// checkOwnedPrivate contrôle un élément de l'espace (le répertoire si dir, sinon un fichier). Un élément
// absent n'est pas une erreur : Load fonctionne aussi avant Init, et config.yaml est facultatif.
func checkOwnedPrivate(path string, dir bool) error {
	info, err := os.Stat(path)
	switch {
	case isNotExist(err):
		return nil // espace pas encore créé (Load avant Init) ou config.yaml absent : rien à lire
	case err != nil:
		return errWrapped(i18n.T("workspace.trust.stat"), err)
	case info.IsDir() != dir:
		return nil // type inattendu : la lecture des réglages le signalera
	}
	mode, want := "700", os.FileMode(DirMode)
	if !dir {
		mode, want = "600", os.FileMode(FileMode)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != currentUID() {
		return fmt.Errorf("%w%s%s", ErrUntrusted, i18n.T("workspace.err.sep"),
			i18n.T("workspace.trust.other_uid", path, st.Uid, EnvDir))
	}
	if perm := info.Mode().Perm(); perm&^want != 0 {
		return fmt.Errorf("%w%s%s", ErrUntrusted, i18n.T("workspace.err.sep"),
			i18n.T("workspace.trust.perms", path, perm, want, mode, path, EnvDir))
	}
	return nil
}
