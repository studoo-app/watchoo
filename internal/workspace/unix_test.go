//go:build unix

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestInitForcesModesWhateverTheUmask checks that the 0700 / 0600 permissions are imposed by chmod and
// do not depend on the umask: with a very restrictive umask (0777) as with a very permissive one (0),
// the result is the same. The umask being specific to the process, this test must not be parallel.
func TestInitForcesModesWhateverTheUmask(t *testing.T) {
	isolateEnv(t)
	for _, mask := range []int{0o777, 0o000} {
		root := t.TempDir()
		old := syscall.Umask(mask)
		res, err := Init(root)
		syscall.Umask(old)
		if err != nil {
			t.Fatalf("Init with umask %#o: %v", mask, err)
		}
		ws := res.Workspace
		assertMode(t, ws.Dir, DirMode)
		assertMode(t, ws.ConfigPath(), FileMode)
		assertMode(t, filepath.Join(ws.Dir, GitignoreFileName), FileMode)
	}
}

// TestFindStopsOnAccessError checks that an access error stops the search instead of going up to a
// parent workspace, which might not be the one the user wanted. Init, for its part, only loses the
// parent workspace warning.
func TestFindStopsOnAccessError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permissions of directories")
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
		t.Fatalf("Find = %v, %v; want an access error", ws, err)
	}
	assertErrorContains(t, err, "looking for the workspace")
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("the original error must be kept: %v", err)
	}
}

// TestLoadRefusesUntrustedWorkspace checks that a workspace accessible to others than the user, or
// owned by another user, is refused before any read of its settings: this is the case of a .watchoo/
// committed in a cloned student repository (Git creates 0755 and 0644).
func TestLoadRefusesUntrustedWorkspace(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(t *testing.T, ws *Workspace)
		target func(ws *Workspace) string
		want   []string
	}{
		{"directory in 0755", func(t *testing.T, ws *Workspace) { chmod(t, ws.Dir, 0o755) },
			func(ws *Workspace) string { return ws.Dir }, []string{"0755", "chmod 700", "-C"}},
		{"directory in 0770", func(t *testing.T, ws *Workspace) { chmod(t, ws.Dir, 0o770) },
			func(ws *Workspace) string { return ws.Dir }, []string{"0770", "chmod 700"}},
		{"config.yaml in 0644", func(t *testing.T, ws *Workspace) { chmod(t, ws.ConfigPath(), 0o644) },
			func(ws *Workspace) string { return ws.ConfigPath() }, []string{"0644", "chmod 600", "-C"}},
		{"other owner", func(t *testing.T, ws *Workspace) {
			uid := os.Getuid() + 1
			old := currentUID
			currentUID = func() int { return uid }
			t.Cleanup(func() { currentUID = old })
		}, func(ws *Workspace) string { return ws.Dir }, []string{"another user", "-C", EnvDir}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateEnv(t)
			ws := mustInit(t, t.TempDir())
			// Setting that would make Validate fail if it were read: the refusal must come before.
			writeFile(t, ws.ConfigPath(), "github:\n  gh_path: .watchoo/gh\n")
			tc.setup(t, ws)
			ws.Config = Config{}
			err := Load(ws)
			if !errors.Is(err, ErrUntrusted) {
				t.Fatalf("Load = %v, want ErrUntrusted", err)
			}
			assertErrorContains(t, err, append([]string{tc.target(ws)}, tc.want...)...)
			if ws.Config.GitHub.GHPath != "" {
				t.Errorf("settings loaded despite the refusal: %+v", ws.Config.GitHub)
			}
		})
	}
}

// TestResolveRefusesCommittedWorkspaceInClone reproduces the attack scenario: a teacher runs a
// command from the clone of a student repository stored in their workspace, and this clone holds a
// committed .watchoo/ (permissions set by Git). Resolve must refuse this fake workspace instead of
// using it, and -C to the real workspace must work.
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
		t.Fatalf("ResolveFrom from the clone = %+v, %v; want ErrUntrusted", ws, err)
	}
	assertErrorContains(t, err, fake, "chmod 700", "-C")

	ws, err = ResolveFrom(teacher.Root, clone)
	if err != nil {
		t.Fatalf("ResolveFrom with -C: %v", err)
	}
	if ws.Root != teacher.Root || ws.Config.GitHub.GHPath != DefaultGHPath {
		t.Errorf("workspace = %s (gh_path %q), want %s and the default gh", ws.Root, ws.Config.GitHub.GHPath, teacher.Root)
	}
}

func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
