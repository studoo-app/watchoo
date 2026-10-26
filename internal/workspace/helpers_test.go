package workspace

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isolateEnv isole un test de l'environnement du développeur : HOME et XDG_CONFIG_HOME pointent vers
// des répertoires temporaires vides, et toutes les variables WATCHOO_* (WATCHOO_DIR compris) sont
// retirées. Elle renvoie le chemin du fichier de réglages utilisateur (sous XDG_CONFIG_HOME), qui
// n'existe pas encore. Les tests qui l'appellent ne peuvent pas être parallèles (t.Setenv).
func isolateEnv(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	home := filepath.Join(base, "home")
	xdg := filepath.Join(base, "xdg")
	for _, dir := range []string{home, xdg} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, EnvPrefix+"_") {
			t.Setenv(name, "") // restaurée à la fin du test
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
	return filepath.Join(xdg, UserConfigDirName, ConfigFileName)
}

// newDir crée le répertoire parent/elems… (avec ses parents) et renvoie son chemin.
func newDir(t *testing.T, parent string, elems ...string) string {
	t.Helper()
	dir := filepath.Join(append([]string{parent}, elems...)...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeFile écrit content dans path, en créant les répertoires parents.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// readFile renvoie le contenu de path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// mustInit crée un espace dans root et échoue le test en cas d'erreur.
func mustInit(t *testing.T, root string) *Workspace {
	t.Helper()
	res, err := Init(root)
	if err != nil {
		t.Fatalf("Init(%s) : %v", root, err)
	}
	return res.Workspace
}

// requireNoWorkspaceAbove saute le test si un espace Watchoo existe déjà au-dessus de dir (répertoire
// temporaire du système) : la recherche dans les parents le trouverait.
func requireNoWorkspaceAbove(t *testing.T, dir string) {
	t.Helper()
	for d := filepath.Dir(dir); ; d = filepath.Dir(d) {
		if info, err := os.Stat(filepath.Join(d, DirName)); err == nil && info.IsDir() {
			t.Skipf("un espace Watchoo existe au-dessus du répertoire temporaire : %s", d)
		}
		if filepath.Dir(d) == d {
			return
		}
	}
}

// assertMode vérifie les droits de path (ignoré sous Windows, qui n'a pas de droits Unix).
func assertMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("droits de %s = %#o, attendu %#o", path, got, want)
	}
}

// assertErrorContains vérifie que err n'est pas nil et que son message contient chacun des extraits.
func assertErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("erreur attendue contenant %q, reçu nil", parts)
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("message d'erreur sans %q :\n%v", part, err)
		}
	}
}
