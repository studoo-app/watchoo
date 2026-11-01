package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

func TestInitCreatesWorkspace(t *testing.T) {
	isolateEnv(t)
	root := newDir(t, t.TempDir(), "licence-projet")
	requireNoWorkspaceAbove(t, root) // otherwise ParentWorkspace would be set

	res, err := Init(root)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	ws := res.Workspace
	if ws.Root != root {
		t.Errorf("Root = %q, want %q", ws.Root, root)
	}
	if want := filepath.Join(root, DirName); ws.Dir != want {
		t.Errorf("Dir = %q, want %q", ws.Dir, want)
	}
	if !reflect.DeepEqual(ws.Config, DefaultConfig("licence-projet")) {
		t.Errorf("Config = %+v, want DefaultConfig(\"licence-projet\")", ws.Config)
	}
	if res.ParentWorkspace != "" {
		t.Errorf("ParentWorkspace = %q, want empty", res.ParentWorkspace)
	}

	assertMode(t, ws.Dir, DirMode)
	assertMode(t, ws.ConfigPath(), FileMode)
	gitignore := filepath.Join(ws.Dir, GitignoreFileName)
	assertMode(t, gitignore, FileMode)

	if got := readFile(t, gitignore); got != "*\n" {
		t.Errorf(".gitignore = %q, want %q", got, "*\n")
	}
	if got := readFile(t, ws.ConfigPath()); got != ConfigTemplate(i18n.English, "licence-projet") {
		t.Errorf("config.yaml different from ConfigTemplate(i18n.English, \"licence-projet\"):\n%s", got)
	}

	// Init does not create the database: that is the role of store.
	if _, err := os.Stat(ws.DBPath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the database must not be created by Init (Stat: %v)", err)
	}
	entries, err := os.ReadDir(ws.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("content of .watchoo = %v, want config.yaml and .gitignore", entries)
	}

	// The created workspace loads again and gives back the default settings.
	loaded := &Workspace{Root: ws.Root, Dir: ws.Dir}
	if err := Load(loaded); err != nil {
		t.Fatalf("Load after Init: %v", err)
	}
	if !reflect.DeepEqual(loaded.Config, ws.Config) {
		t.Errorf("Load after Init = %+v, want %+v", loaded.Config, ws.Config)
	}
}

func TestInitRelativeRoot(t *testing.T) {
	isolateEnv(t)
	root := newDir(t, t.TempDir(), "promo")
	t.Chdir(root)

	res, err := Init(".")
	if err != nil {
		t.Fatalf("Init(\".\"): %v", err)
	}
	if !filepath.IsAbs(res.Workspace.Root) {
		t.Errorf("Root = %q, want an absolute path", res.Workspace.Root)
	}
	assertSameDir(t, res.Workspace.Root, root)
	if res.Workspace.Config.Name != "promo" {
		t.Errorf("Name = %q, want %q", res.Workspace.Config.Name, "promo")
	}
}

func TestInitRefusesExistingWorkspace(t *testing.T) {
	isolateEnv(t)
	root := t.TempDir()
	ws := mustInit(t, root)

	custom := "name: \"mes réglages\"\n"
	writeFile(t, ws.ConfigPath(), custom)

	_, err := Init(root)
	if !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("second Init: error %v, want ErrAlreadyInitialized", err)
	}
	assertErrorContains(t, err, "already a Watchoo workspace", root)
	if got := readFile(t, ws.ConfigPath()); got != custom {
		t.Errorf("config.yaml modified by the second Init: %q", got)
	}
}

func TestInitRefusesWatchooFile(t *testing.T) {
	isolateEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, DirName)
	writeFile(t, path, "pas un répertoire")

	_, err := Init(root)
	if err == nil || errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("Init with a .watchoo file: error %v, want a clear error", err)
	}
	assertErrorContains(t, err, path, "is not a directory")
	if got := readFile(t, path); got != "pas un répertoire" {
		t.Errorf("the .watchoo file was modified: %q", got)
	}
}

func TestInitRootMustBeADirectory(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()

	missing := filepath.Join(base, "absent")
	_, err := Init(missing)
	assertErrorContains(t, err, missing, "does not exist")
	if _, statErr := os.Stat(missing); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("Init must not create the root directory")
	}

	file := filepath.Join(base, "fichier")
	writeFile(t, file, "x")
	_, err = Init(file)
	assertErrorContains(t, err, file, "is not a directory")
}

func TestInitNestedWorkspace(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()
	requireNoWorkspaceAbove(t, base)
	parent := newDir(t, base, "licence-projet")
	mustInit(t, parent)
	child := newDir(t, parent, "groupes", "groupe-a")

	res, err := Init(child)
	if err != nil {
		t.Fatalf("nested Init: %v", err)
	}
	if res.ParentWorkspace != parent {
		t.Errorf("ParentWorkspace = %q, want %q", res.ParentWorkspace, parent)
	}
	if res.Workspace.Root != child || res.Workspace.Config.Name != "groupe-a" {
		t.Errorf("nested workspace = %+v", res.Workspace)
	}

	// Each workspace is found from its own subdirectories.
	deep := newDir(t, child, "src", "x")
	if ws, err := Find(deep); err != nil || ws.Root != child {
		t.Errorf("Find(%s) = %v, %v; want the nested workspace %s", deep, ws, err, child)
	}
	sibling := newDir(t, parent, "groupes", "groupe-b")
	if ws, err := Find(sibling); err != nil || ws.Root != parent {
		t.Errorf("Find(%s) = %v, %v; want the parent workspace %s", sibling, ws, err, parent)
	}

	// A workspace without parent reports nothing.
	other := newDir(t, base, "master-projet")
	res, err = Init(other)
	if err != nil {
		t.Fatal(err)
	}
	if res.ParentWorkspace != "" {
		t.Errorf("ParentWorkspace = %q, want empty", res.ParentWorkspace)
	}
}

func TestInitCleansUpOnFailure(t *testing.T) {
	isolateEnv(t)
	root := t.TempDir()

	failure := errors.New("disk full (simulated)")
	original := createFile
	t.Cleanup(func() { createFile = original })
	createFile = func(path string, content []byte) error {
		if filepath.Base(path) == GitignoreFileName {
			return failure
		}
		return original(path, content)
	}

	_, err := Init(root)
	if !errors.Is(err, failure) {
		t.Fatalf("Init: error %v, want the simulated failure", err)
	}
	if _, statErr := os.Lstat(filepath.Join(root, DirName)); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf(".watchoo must be deleted after a failure (Lstat: %v)", statErr)
	}

	// Once the cause is gone, Init succeeds.
	createFile = original
	if _, err := Init(root); err != nil {
		t.Fatalf("Init after cleanup: %v", err)
	}
}

func TestFind(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()
	requireNoWorkspaceAbove(t, base)
	root := newDir(t, base, "espace")
	mustInit(t, root)
	deep := newDir(t, root, "a", "b", "c")

	for _, start := range []string{root, deep, filepath.Join(root, DirName)} {
		ws, err := Find(start)
		if err != nil {
			t.Fatalf("Find(%s): %v", start, err)
		}
		if ws.Root != root || ws.Dir != filepath.Join(root, DirName) {
			t.Errorf("Find(%s) = {Root: %s, Dir: %s}, want the root %s", start, ws.Root, ws.Dir, root)
		}
		if !reflect.DeepEqual(ws.Config, Config{}) {
			t.Errorf("Find must not load the settings: %+v", ws.Config)
		}
	}
}

func TestFindRelativeStart(t *testing.T) {
	isolateEnv(t)
	root := t.TempDir()
	mustInit(t, root)
	newDir(t, root, "sous")
	t.Chdir(root)

	ws, err := Find("sous")
	if err != nil {
		t.Fatalf("Find(\"sous\"): %v", err)
	}
	assertSameDir(t, ws.Root, root)
}

func TestFindNotFound(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()
	requireNoWorkspaceAbove(t, base)
	start := newDir(t, base, "vide", "sous")

	ws, err := Find(start)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find: %v, %v; want ErrNotFound", ws, err)
	}
	assertErrorContains(t, err, `no Watchoo workspace here, run "watchoo init"`, start)
}

func TestFindIgnoresWatchooFile(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()
	requireNoWorkspaceAbove(t, base)
	mustInit(t, base)
	child := newDir(t, base, "enfant")
	writeFile(t, filepath.Join(child, DirName), "fichier, pas un espace")

	ws, err := Find(child)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if ws.Root != base {
		t.Errorf("Find = %s, want the workspace above (%s): a .watchoo file does not count", ws.Root, base)
	}
}

// A .watchoo/ committed in a student's repository, cloned by Watchoo into repos/ (V2), is never
// taken for a workspace, even with private permissions (umask 077): the search resumes at the
// workspace holding the clones. Init is refused there.
func TestFindSkipsWorkspaceInClones(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()
	requireNoWorkspaceAbove(t, base)
	root := newDir(t, base, "espace")
	mustInit(t, root)
	clone := newDir(t, root, ReposDirName, "projet-groupe-3")
	if err := os.Mkdir(filepath.Join(clone, DirName), DirMode); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(clone, DirName, ConfigFileName), "name: piege\n")
	deep := newDir(t, clone, "src")
	nested := newDir(t, clone, "sous", "projet")
	if err := os.Mkdir(filepath.Join(nested, DirName), DirMode); err != nil {
		t.Fatal(err)
	}
	for _, start := range []string{clone, deep, nested, filepath.Join(clone, DirName)} {
		ws, err := Find(start)
		if err != nil {
			t.Fatalf("Find(%s): %v", start, err)
		}
		if ws.Root != root {
			t.Errorf("Find(%s) = %s, want the workspace holding the clones (%s)", start, ws.Root, root)
		}
	}
	// Elsewhere than under repos/, a nested workspace is still found.
	other := newDir(t, root, "autre")
	mustInit(t, other)
	if ws, err := Find(other); err != nil || ws.Root != other {
		t.Errorf("Find(%s) = %v, %v; want the nested workspace", other, ws, err)
	}
	// Init in a clone: refused, nothing created.
	fresh := newDir(t, root, ReposDirName, "autre-projet")
	if _, err := Init(fresh); err == nil || !strings.Contains(err.Error(), "clones directory") {
		t.Errorf("Init in repos/: %v, want a refusal", err)
	}
	if _, err := os.Stat(filepath.Join(fresh, DirName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Init in repos/ created %s", DirName)
	}
}

func TestFindNearestWins(t *testing.T) {
	isolateEnv(t)
	outer := t.TempDir()
	mustInit(t, outer)
	inner := newDir(t, outer, "inner")
	mustInit(t, inner)

	ws, err := Find(newDir(t, inner, "x"))
	if err != nil {
		t.Fatal(err)
	}
	if ws.Root != inner {
		t.Errorf("Find = %s, want the nearest workspace %s", ws.Root, inner)
	}
}

func TestResolveFromPriority(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()
	requireNoWorkspaceAbove(t, base)
	fromFlag := newDir(t, base, "flag")
	fromEnv := newDir(t, base, "env")
	fromCwd := newDir(t, base, "cwd")
	for _, root := range []string{fromFlag, fromEnv, fromCwd} {
		mustInit(t, root)
	}
	cwd := newDir(t, fromCwd, "sous", "repertoire")

	t.Setenv(EnvDir, fromEnv)
	ws, err := ResolveFrom(fromFlag, cwd)
	if err != nil {
		t.Fatalf("ResolveFrom with -C: %v", err)
	}
	if ws.Root != fromFlag || ws.Config.Name != "flag" {
		t.Errorf("-C must take precedence over WATCHOO_DIR and the current directory: %s (%q)", ws.Root, ws.Config.Name)
	}

	ws, err = ResolveFrom("", cwd)
	if err != nil {
		t.Fatalf("ResolveFrom with WATCHOO_DIR: %v", err)
	}
	if ws.Root != fromEnv || ws.Config.Name != "env" {
		t.Errorf("WATCHOO_DIR must take precedence over the current directory: %s", ws.Root)
	}

	t.Setenv(EnvDir, "")
	ws, err = ResolveFrom("", cwd)
	if err != nil {
		t.Fatalf("ResolveFrom from the current directory: %v", err)
	}
	if ws.Root != fromCwd || ws.Config.Name != "cwd" {
		t.Errorf("the current directory (and its parents) must be used: %s", ws.Root)
	}
}

func TestResolveFromRelativePaths(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()
	root := newDir(t, base, "licence-projet")
	mustInit(t, root)
	newDir(t, root, "groupe-a")

	// Relative -C, interpreted against cwd; a subdirectory of a workspace is fine (like git -C).
	ws, err := ResolveFrom(filepath.Join("licence-projet", "groupe-a"), base)
	if err != nil {
		t.Fatalf("ResolveFrom(relative -C): %v", err)
	}
	if ws.Root != root {
		t.Errorf("Root = %s, want %s", ws.Root, root)
	}

	// Relative WATCHOO_DIR.
	t.Setenv(EnvDir, "licence-projet")
	ws, err = ResolveFrom("", base)
	if err != nil {
		t.Fatalf("ResolveFrom(relative WATCHOO_DIR): %v", err)
	}
	if ws.Root != root {
		t.Errorf("Root = %s, want %s", ws.Root, root)
	}
}

func TestResolveFromErrors(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()
	requireNoWorkspaceAbove(t, base)
	empty := newDir(t, base, "sans-espace")
	missing := filepath.Join(base, "absent")
	file := filepath.Join(base, "fichier")
	writeFile(t, file, "x")

	t.Run("-C without workspace", func(t *testing.T) {
		_, err := ResolveFrom(empty, base)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("error %v, want ErrNotFound", err)
		}
		assertErrorContains(t, err, "option -C", "no Watchoo workspace", empty)
	})
	t.Run("nonexistent -C", func(t *testing.T) {
		_, err := ResolveFrom(missing, base)
		assertErrorContains(t, err, "option -C", missing, "does not exist")
	})
	t.Run("-C file", func(t *testing.T) {
		_, err := ResolveFrom(file, base)
		assertErrorContains(t, err, "option -C", file, "is not a directory")
	})
	t.Run("nonexistent WATCHOO_DIR", func(t *testing.T) {
		t.Setenv(EnvDir, missing)
		_, err := ResolveFrom("", base)
		assertErrorContains(t, err, "variable WATCHOO_DIR", missing, "does not exist")
	})
	t.Run("WATCHOO_DIR without workspace", func(t *testing.T) {
		t.Setenv(EnvDir, empty)
		_, err := ResolveFrom("", base)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("error %v, want ErrNotFound", err)
		}
		assertErrorContains(t, err, "variable WATCHOO_DIR")
	})
	t.Run("current directory without workspace", func(t *testing.T) {
		_, err := ResolveFrom("", empty)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("error %v, want ErrNotFound", err)
		}
		if strings.Contains(err.Error(), "option -C") || strings.Contains(err.Error(), EnvDir) {
			t.Errorf("the message must quote neither -C nor WATCHOO_DIR: %v", err)
		}
	})
	t.Run("invalid settings", func(t *testing.T) {
		root := newDir(t, base, "invalide")
		ws := mustInit(t, root)
		writeFile(t, ws.ConfigPath(), "sync:\n  parallelism: 9\n")
		_, err := ResolveFrom(root, base)
		assertErrorContains(t, err, "sync.parallelism")
	})
}

func TestResolveUsesWorkingDirectory(t *testing.T) {
	isolateEnv(t)
	root := t.TempDir()
	mustInit(t, root)
	t.Chdir(newDir(t, root, "sous"))

	ws, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertSameDir(t, ws.Root, root)
	if ws.Config.Sync.Interval != DefaultSyncInterval {
		t.Errorf("settings not loaded: %+v", ws.Config)
	}
}

func TestWorkspacePaths(t *testing.T) {
	ws := Workspace{Root: "/x", Dir: filepath.Join("/x", DirName)}
	if got, want := ws.DBPath(), filepath.Join("/x", ".watchoo", "watchoo.db"); got != want {
		t.Errorf("DBPath = %s, want %s", got, want)
	}
	if got, want := ws.ConfigPath(), filepath.Join("/x", ".watchoo", "config.yaml"); got != want {
		t.Errorf("ConfigPath = %s, want %s", got, want)
	}
	// Clones with a working copy, at the root of the workspace (next to .watchoo/).
	if got, want := ws.ReposDir(), filepath.Join("/x", "repos"); got != want {
		t.Errorf("ReposDir = %s, want %s", got, want)
	}
}

// assertSameDir checks that got and want designate the same directory (on macOS, os.Getwd may
// return /private/var/… for a temporary directory /var/…).
func assertSameDir(t *testing.T, got, want string) {
	t.Helper()
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Errorf("directory %s, want %s", got, want)
	}
}
