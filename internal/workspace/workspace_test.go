package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInitCreatesWorkspace(t *testing.T) {
	isolateEnv(t)
	root := newDir(t, t.TempDir(), "licence-projet")
	requireNoWorkspaceAbove(t, root) // sinon ParentWorkspace serait renseigné

	res, err := Init(root)
	if err != nil {
		t.Fatalf("Init : %v", err)
	}
	ws := res.Workspace
	if ws.Root != root {
		t.Errorf("Root = %q, attendu %q", ws.Root, root)
	}
	if want := filepath.Join(root, DirName); ws.Dir != want {
		t.Errorf("Dir = %q, attendu %q", ws.Dir, want)
	}
	if !reflect.DeepEqual(ws.Config, DefaultConfig("licence-projet")) {
		t.Errorf("Config = %+v, attendu DefaultConfig(\"licence-projet\")", ws.Config)
	}
	if res.ParentWorkspace != "" {
		t.Errorf("ParentWorkspace = %q, attendu vide", res.ParentWorkspace)
	}

	assertMode(t, ws.Dir, DirMode)
	assertMode(t, ws.ConfigPath(), FileMode)
	gitignore := filepath.Join(ws.Dir, GitignoreFileName)
	assertMode(t, gitignore, FileMode)

	if got := readFile(t, gitignore); got != "*\n" {
		t.Errorf(".gitignore = %q, attendu %q", got, "*\n")
	}
	if got := readFile(t, ws.ConfigPath()); got != ConfigTemplate("licence-projet") {
		t.Errorf("config.yaml différent de ConfigTemplate(\"licence-projet\") :\n%s", got)
	}

	// Init ne crée pas la base : c'est le rôle de store.
	if _, err := os.Stat(ws.DBPath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("la base ne doit pas être créée par Init (Stat : %v)", err)
	}
	entries, err := os.ReadDir(ws.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("contenu de .watchoo = %v, attendu config.yaml et .gitignore", entries)
	}

	// L'espace créé se recharge et redonne les réglages par défaut.
	loaded := &Workspace{Root: ws.Root, Dir: ws.Dir}
	if err := Load(loaded); err != nil {
		t.Fatalf("Load après Init : %v", err)
	}
	if !reflect.DeepEqual(loaded.Config, ws.Config) {
		t.Errorf("Load après Init = %+v, attendu %+v", loaded.Config, ws.Config)
	}
}

func TestInitRelativeRoot(t *testing.T) {
	isolateEnv(t)
	root := newDir(t, t.TempDir(), "promo")
	t.Chdir(root)

	res, err := Init(".")
	if err != nil {
		t.Fatalf("Init(\".\") : %v", err)
	}
	if !filepath.IsAbs(res.Workspace.Root) {
		t.Errorf("Root = %q, attendu un chemin absolu", res.Workspace.Root)
	}
	assertSameDir(t, res.Workspace.Root, root)
	if res.Workspace.Config.Name != "promo" {
		t.Errorf("Name = %q, attendu %q", res.Workspace.Config.Name, "promo")
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
		t.Fatalf("second Init : erreur %v, attendu ErrAlreadyInitialized", err)
	}
	assertErrorContains(t, err, "déjà un espace Watchoo", root)
	if got := readFile(t, ws.ConfigPath()); got != custom {
		t.Errorf("config.yaml modifié par le second Init : %q", got)
	}
}

func TestInitRefusesWatchooFile(t *testing.T) {
	isolateEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, DirName)
	writeFile(t, path, "pas un répertoire")

	_, err := Init(root)
	if err == nil || errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("Init avec un fichier .watchoo : erreur %v, attendu une erreur claire", err)
	}
	assertErrorContains(t, err, path, "n'est pas un répertoire")
	if got := readFile(t, path); got != "pas un répertoire" {
		t.Errorf("le fichier .watchoo a été modifié : %q", got)
	}
}

func TestInitRootMustBeADirectory(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()

	missing := filepath.Join(base, "absent")
	_, err := Init(missing)
	assertErrorContains(t, err, missing, "n'existe pas")
	if _, statErr := os.Stat(missing); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("Init ne doit pas créer le répertoire racine")
	}

	file := filepath.Join(base, "fichier")
	writeFile(t, file, "x")
	_, err = Init(file)
	assertErrorContains(t, err, file, "n'est pas un répertoire")
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
		t.Fatalf("Init imbriqué : %v", err)
	}
	if res.ParentWorkspace != parent {
		t.Errorf("ParentWorkspace = %q, attendu %q", res.ParentWorkspace, parent)
	}
	if res.Workspace.Root != child || res.Workspace.Config.Name != "groupe-a" {
		t.Errorf("espace imbriqué = %+v", res.Workspace)
	}

	// Chaque espace est trouvé depuis ses propres sous-répertoires.
	deep := newDir(t, child, "src", "x")
	if ws, err := Find(deep); err != nil || ws.Root != child {
		t.Errorf("Find(%s) = %v, %v ; attendu l'espace imbriqué %s", deep, ws, err, child)
	}
	sibling := newDir(t, parent, "groupes", "groupe-b")
	if ws, err := Find(sibling); err != nil || ws.Root != parent {
		t.Errorf("Find(%s) = %v, %v ; attendu l'espace parent %s", sibling, ws, err, parent)
	}

	// Un espace sans parent ne signale rien.
	other := newDir(t, base, "master-projet")
	res, err = Init(other)
	if err != nil {
		t.Fatal(err)
	}
	if res.ParentWorkspace != "" {
		t.Errorf("ParentWorkspace = %q, attendu vide", res.ParentWorkspace)
	}
}

func TestInitCleansUpOnFailure(t *testing.T) {
	isolateEnv(t)
	root := t.TempDir()

	failure := errors.New("disque plein (simulé)")
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
		t.Fatalf("Init : erreur %v, attendu l'échec simulé", err)
	}
	if _, statErr := os.Lstat(filepath.Join(root, DirName)); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf(".watchoo doit être supprimé après un échec (Lstat : %v)", statErr)
	}

	// Une fois la cause disparue, Init réussit.
	createFile = original
	if _, err := Init(root); err != nil {
		t.Fatalf("Init après nettoyage : %v", err)
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
			t.Fatalf("Find(%s) : %v", start, err)
		}
		if ws.Root != root || ws.Dir != filepath.Join(root, DirName) {
			t.Errorf("Find(%s) = {Root: %s, Dir: %s}, attendu la racine %s", start, ws.Root, ws.Dir, root)
		}
		if !reflect.DeepEqual(ws.Config, Config{}) {
			t.Errorf("Find ne doit pas charger les réglages : %+v", ws.Config)
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
		t.Fatalf("Find(\"sous\") : %v", err)
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
		t.Fatalf("Find : %v, %v ; attendu ErrNotFound", ws, err)
	}
	assertErrorContains(t, err, "aucun espace Watchoo ici, lancez « watchoo init »", start)
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
		t.Fatalf("Find : %v", err)
	}
	if ws.Root != base {
		t.Errorf("Find = %s, attendu l'espace au-dessus (%s) : un fichier .watchoo ne compte pas", ws.Root, base)
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
		t.Errorf("Find = %s, attendu l'espace le plus proche %s", ws.Root, inner)
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
		t.Fatalf("ResolveFrom avec -C : %v", err)
	}
	if ws.Root != fromFlag || ws.Config.Name != "flag" {
		t.Errorf("-C doit l'emporter sur WATCHOO_DIR et le répertoire courant : %s (%q)", ws.Root, ws.Config.Name)
	}

	ws, err = ResolveFrom("", cwd)
	if err != nil {
		t.Fatalf("ResolveFrom avec WATCHOO_DIR : %v", err)
	}
	if ws.Root != fromEnv || ws.Config.Name != "env" {
		t.Errorf("WATCHOO_DIR doit l'emporter sur le répertoire courant : %s", ws.Root)
	}

	t.Setenv(EnvDir, "")
	ws, err = ResolveFrom("", cwd)
	if err != nil {
		t.Fatalf("ResolveFrom depuis le répertoire courant : %v", err)
	}
	if ws.Root != fromCwd || ws.Config.Name != "cwd" {
		t.Errorf("le répertoire courant (et ses parents) doit être utilisé : %s", ws.Root)
	}
}

func TestResolveFromRelativePaths(t *testing.T) {
	isolateEnv(t)
	base := t.TempDir()
	root := newDir(t, base, "licence-projet")
	mustInit(t, root)
	newDir(t, root, "groupe-a")

	// -C relatif, interprété par rapport à cwd ; un sous-répertoire d'espace convient (comme git -C).
	ws, err := ResolveFrom(filepath.Join("licence-projet", "groupe-a"), base)
	if err != nil {
		t.Fatalf("ResolveFrom(-C relatif) : %v", err)
	}
	if ws.Root != root {
		t.Errorf("Root = %s, attendu %s", ws.Root, root)
	}

	// WATCHOO_DIR relatif.
	t.Setenv(EnvDir, "licence-projet")
	ws, err = ResolveFrom("", base)
	if err != nil {
		t.Fatalf("ResolveFrom(WATCHOO_DIR relatif) : %v", err)
	}
	if ws.Root != root {
		t.Errorf("Root = %s, attendu %s", ws.Root, root)
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

	t.Run("-C sans espace", func(t *testing.T) {
		_, err := ResolveFrom(empty, base)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("erreur %v, attendu ErrNotFound", err)
		}
		assertErrorContains(t, err, "option -C", "aucun espace Watchoo", empty)
	})
	t.Run("-C inexistant", func(t *testing.T) {
		_, err := ResolveFrom(missing, base)
		assertErrorContains(t, err, "option -C", missing, "n'existe pas")
	})
	t.Run("-C fichier", func(t *testing.T) {
		_, err := ResolveFrom(file, base)
		assertErrorContains(t, err, "option -C", file, "n'est pas un répertoire")
	})
	t.Run("WATCHOO_DIR inexistant", func(t *testing.T) {
		t.Setenv(EnvDir, missing)
		_, err := ResolveFrom("", base)
		assertErrorContains(t, err, "variable WATCHOO_DIR", missing, "n'existe pas")
	})
	t.Run("WATCHOO_DIR sans espace", func(t *testing.T) {
		t.Setenv(EnvDir, empty)
		_, err := ResolveFrom("", base)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("erreur %v, attendu ErrNotFound", err)
		}
		assertErrorContains(t, err, "variable WATCHOO_DIR")
	})
	t.Run("répertoire courant sans espace", func(t *testing.T) {
		_, err := ResolveFrom("", empty)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("erreur %v, attendu ErrNotFound", err)
		}
		if strings.Contains(err.Error(), "option -C") || strings.Contains(err.Error(), EnvDir) {
			t.Errorf("le message ne doit citer ni -C ni WATCHOO_DIR : %v", err)
		}
	})
	t.Run("réglages invalides", func(t *testing.T) {
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
		t.Fatalf("Resolve : %v", err)
	}
	assertSameDir(t, ws.Root, root)
	if ws.Config.Sync.Interval != DefaultSyncInterval {
		t.Errorf("réglages non chargés : %+v", ws.Config)
	}
}

func TestWorkspacePaths(t *testing.T) {
	ws := Workspace{Root: "/x", Dir: filepath.Join("/x", DirName)}
	if got, want := ws.DBPath(), filepath.Join("/x", ".watchoo", "watchoo.db"); got != want {
		t.Errorf("DBPath = %s, attendu %s", got, want)
	}
	if got, want := ws.ConfigPath(), filepath.Join("/x", ".watchoo", "config.yaml"); got != want {
		t.Errorf("ConfigPath = %s, attendu %s", got, want)
	}
	if got, want := ws.ReposDir(), filepath.Join("/x", ".watchoo", "repos"); got != want {
		t.Errorf("ReposDir = %s, attendu %s", got, want)
	}
	// Les clones sont dans .watchoo/, dont le .gitignore ignore tout.
	if GitignoreContent != "*\n" {
		t.Errorf(".gitignore de .watchoo/ = %q : les clones (repos/) doivent rester ignorés", GitignoreContent)
	}
}

// assertSameDir vérifie que got et want désignent le même répertoire (sous macOS, os.Getwd peut
// renvoyer /private/var/… pour un répertoire temporaire /var/…).
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
		t.Errorf("répertoire %s, attendu %s", got, want)
	}
}
