//go:build unix

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestInitForcesModesWhateverTheUmask vérifie que les droits 0700 / 0600 sont imposés par chmod et ne
// dépendent pas du umask : avec un umask très restrictif (0777) comme très permissif (0), le résultat
// est le même. Le umask étant propre au processus, ce test ne doit pas être parallèle.
func TestInitForcesModesWhateverTheUmask(t *testing.T) {
	isolateEnv(t)
	for _, mask := range []int{0o777, 0o000} {
		root := t.TempDir()
		old := syscall.Umask(mask)
		res, err := Init(root)
		syscall.Umask(old)
		if err != nil {
			t.Fatalf("Init avec umask %#o : %v", mask, err)
		}
		ws := res.Workspace
		assertMode(t, ws.Dir, DirMode)
		assertMode(t, ws.ConfigPath(), FileMode)
		assertMode(t, filepath.Join(ws.Dir, GitignoreFileName), FileMode)
	}
}

// TestFindStopsOnAccessError vérifie qu'une erreur d'accès arrête la recherche au lieu de remonter
// vers un espace parent, qui ne serait peut-être pas celui voulu par l'utilisateur. Init, lui, ne
// perd que l'avertissement d'espace parent.
func TestFindStopsOnAccessError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignore les droits des répertoires")
	}
	isolateEnv(t)
	base := t.TempDir()
	mustInit(t, base)
	locked := newDir(t, base, "verrouille")
	start := newDir(t, locked, "sous")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	ws, err := Find(start)
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("Find = %v, %v ; attendu une erreur d'accès", ws, err)
	}
	assertErrorContains(t, err, "recherche de l'espace")
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("l'erreur d'origine doit être conservée : %v", err)
	}
}
