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

// TestLoadRefusesUntrustedWorkspace vérifie qu'un espace accessible à d'autres que l'utilisateur, ou
// qui appartient à un autre utilisateur, est refusé avant toute lecture de ses réglages : c'est le cas
// d'un .watchoo/ commité dans un dépôt d'étudiant cloné (Git crée 0755 et 0644).
func TestLoadRefusesUntrustedWorkspace(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(t *testing.T, ws *Workspace)
		target func(ws *Workspace) string
		want   []string
	}{
		{"répertoire en 0755", func(t *testing.T, ws *Workspace) { chmod(t, ws.Dir, 0o755) },
			func(ws *Workspace) string { return ws.Dir }, []string{"0755", "chmod 700", "-C"}},
		{"répertoire en 0770", func(t *testing.T, ws *Workspace) { chmod(t, ws.Dir, 0o770) },
			func(ws *Workspace) string { return ws.Dir }, []string{"0770", "chmod 700"}},
		{"config.yaml en 0644", func(t *testing.T, ws *Workspace) { chmod(t, ws.ConfigPath(), 0o644) },
			func(ws *Workspace) string { return ws.ConfigPath() }, []string{"0644", "chmod 600", "-C"}},
		{"autre propriétaire", func(t *testing.T, ws *Workspace) {
			uid := os.Getuid() + 1
			old := currentUID
			currentUID = func() int { return uid }
			t.Cleanup(func() { currentUID = old })
		}, func(ws *Workspace) string { return ws.Dir }, []string{"autre utilisateur", "-C", EnvDir}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateEnv(t)
			ws := mustInit(t, t.TempDir())
			// Réglage qui ferait échouer Validate s'il était lu : le refus doit venir avant.
			writeFile(t, ws.ConfigPath(), "github:\n  gh_path: .watchoo/gh\n")
			tc.setup(t, ws)
			ws.Config = Config{}
			err := Load(ws)
			if !errors.Is(err, ErrUntrusted) {
				t.Fatalf("Load = %v, attendu ErrUntrusted", err)
			}
			assertErrorContains(t, err, append([]string{tc.target(ws)}, tc.want...)...)
			if ws.Config.GitHub.GHPath != "" {
				t.Errorf("réglages chargés malgré le refus : %+v", ws.Config.GitHub)
			}
		})
	}
}

// TestResolveRefusesCommittedWorkspaceInClone reproduit le scénario d'attaque : un enseignant lance
// une commande depuis le clone d'un dépôt d'étudiant rangé dans son espace, et ce clone contient un
// .watchoo/ commité (droits posés par Git). Resolve doit refuser ce faux espace au lieu de l'utiliser,
// et -C vers le vrai espace doit fonctionner.
func TestResolveRefusesCommittedWorkspaceInClone(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()
	requireNoWorkspaceAbove(t, base)
	teacher := mustInit(t, base)
	clone := newDir(t, base, "alice-projet")
	fake := filepath.Join(clone, DirName)
	if err := os.Mkdir(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fake, ConfigFileName),
		[]byte("github:\n  gh_path: /tmp/piege\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws, err := ResolveFrom("", clone)
	if !errors.Is(err, ErrUntrusted) {
		t.Fatalf("ResolveFrom depuis le clone = %+v, %v ; attendu ErrUntrusted", ws, err)
	}
	assertErrorContains(t, err, fake, "chmod 700", "-C")

	ws, err = ResolveFrom(teacher.Root, clone)
	if err != nil {
		t.Fatalf("ResolveFrom avec -C : %v", err)
	}
	if ws.Root != teacher.Root || ws.Config.GitHub.GHPath != DefaultGHPath {
		t.Errorf("espace = %s (gh_path %q), attendu %s et le gh par défaut", ws.Root, ws.Config.GitHub.GHPath, teacher.Root)
	}
}

func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
