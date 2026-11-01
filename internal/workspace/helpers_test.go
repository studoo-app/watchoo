package workspace

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMain forces the process language for the whole package: the tests' expected texts are
// written in English, regardless of WATCHOO_LANG and the machine's locale (see
// documentation/en/09-testing.md, "Languages in tests"). No restoring: English stays the
// language of the tests; the test dedicated to French forces its own language and restores it.
func TestMain(m *testing.M) {
	i18n.SetProcessForTest(i18n.English)
	os.Exit(m.Run())
}

// isolateEnv isolates a test from the developer's environment: HOME and XDG_CONFIG_HOME point to
// empty temporary directories, and all the WATCHOO_* variables (WATCHOO_DIR included) are removed.
// It returns the path of the user settings file (under XDG_CONFIG_HOME), which does not exist yet.
// The tests that call it cannot be parallel (t.Setenv).
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
			t.Setenv(name, "") // restored at the end of the test
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
	return filepath.Join(xdg, UserConfigDirName, ConfigFileName)
}

// newDir creates the directory parent/elems… (with its parents) and returns its path.
func newDir(t *testing.T, parent string, elems ...string) string {
	t.Helper()
	dir := filepath.Join(append([]string{parent}, elems...)...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeFile writes content to path, creating the parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// readFile returns the content of path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// mustInit creates a workspace in root and fails the test on error.
func mustInit(t *testing.T, root string) *Workspace {
	t.Helper()
	res, err := Init(root)
	if err != nil {
		t.Fatalf("Init(%s): %v", root, err)
	}
	return res.Workspace
}

// requireNoWorkspaceAbove skips the test if a Watchoo workspace already exists above dir (system
// temporary directory): the search in the parents would find it.
func requireNoWorkspaceAbove(t *testing.T, dir string) {
	t.Helper()
	for d := filepath.Dir(dir); ; d = filepath.Dir(d) {
		if info, err := os.Stat(filepath.Join(d, DirName)); err == nil && info.IsDir() {
			t.Skipf("a Watchoo workspace exists above the temporary directory: %s", d)
		}
		if filepath.Dir(d) == d {
			return
		}
	}
}

// assertMode checks the permissions of path (ignored on Windows, which has no Unix permissions).
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
		t.Errorf("permissions of %s = %#o, want %#o", path, got, want)
	}
}

// assertErrorContains checks that err is not nil and that its message contains each of the parts.
func assertErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want an error containing %q, got nil", parts)
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error message without %q:\n%v", part, err)
		}
	}
}
