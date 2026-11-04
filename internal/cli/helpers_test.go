package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/selfupdate"
	"github.com/studoo-app/watchoo/internal/tui"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// isolateEnv isolates a test from the developer's environment: HOME (and USERPROFILE on Windows) and
// XDG_CONFIG_HOME point to empty temporary directories, and the WATCHOO_* variables (WATCHOO_DIR
// included), GH_TOKEN and GITHUB_TOKEN are removed. It returns the path of the user settings file, which
// does not exist yet. The tests that call it cannot be parallel (t.Setenv).
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
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home) // home directory per os.UserHomeDir ("~" in displayed paths)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, workspace.EnvPrefix+"_") || name == "GH_TOKEN" || name == "GITHUB_TOKEN" {
			t.Setenv(name, "") // restored at the end of the test
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
	return filepath.Join(xdg, workspace.UserConfigDirName, workspace.ConfigFileName)
}

// harness runs the CLI with test dependencies: captured outputs, simulated current directory, gh
// access provided by Client (an authtest.Fake by default), standard output described by Terminal.
type harness struct {
	t      *testing.T
	cwd    string      // current directory returned by Getwd
	root   string      // root of the test workspace (initial cwd): Git clones in root/repos
	Client auth.Client // gh access returned by NewAuthClient; nil = real auth.GH (DefaultDeps)
	// Terminal describes the standard output; nil = default value (DetectTerminal of the buffer that
	// captures the output: never a terminal).
	Terminal func() TermInfo
	// configs receives the Config passed to each call of NewAuthClient.
	configs []workspace.Config

	// Phase 3 dependencies.

	// Provider is the fake provider returned by NewProvider; nil = NewProvider returns an error
	// (no test reaches GitHub).
	Provider *providertest.Fake
	// NewProvider, if set, replaces Provider: it builds the provider from the token (the real GitHub
	// provider wired to githubtest, for the end-to-end test).
	NewProvider func(token string) (provider.Provider, error)
	// tokens receives the token passed to each call of NewProvider.
	tokens []string
	// Now is the clock; nil = time.Now.
	Now func() time.Time
	// Stdin is the content of the standard input; StdinTerminal tells whether it is a terminal.
	Stdin         string
	StdinTerminal bool
	// RunTUI replaces tui.Run; nil = a fake that fails (the real TUI is never started).
	RunTUI func(ctx context.Context, deps tui.Deps, term tui.IO) error
	// Listen replaces net.Listen for "watchoo serve"; nil = a fake that fails (no test listens
	// without asking for it).
	Listen func(network, address string) (net.Listener, error)
	// GitURL is the root of the fake Git server (gittest) of the local clones, used without gh; empty =
	// NewGitProvider returns an error (no test reaches GitHub through git).
	GitURL string
	// GitMissing simulates a missing git for "watchoo init" without gh.
	GitMissing bool

	// Update (0.6.0).

	// Updater replaces the releases client; nil = a fake that fails (no test reaches the network).
	// The update tests plug a selfupdate.Client wired to selfupdatetest into it.
	Updater Updater
	// StderrTerminal tells whether the error output is a terminal (new-version notice).
	StderrTerminal bool
	// Executable is the binary path returned to "watchoo update"; empty = error.
	Executable string
	// InDocker simulates the Docker image.
	InDocker bool
}

// offlineUpdater is the tests' default Updater: every request fails.
type offlineUpdater struct{}

func (offlineUpdater) Check(context.Context, string) (selfupdate.Status, error) {
	return selfupdate.Status{}, errors.New("no network access in this test")
}

func (offlineUpdater) Install(context.Context, string, string) error {
	return errors.New("no network access in this test")
}

// newHarness isolates the environment and returns a harness whose current directory is a temporary
// directory named "licence-projet", with a fake gh signed in. The test is skipped if a Watchoo
// workspace exists above the temporary directory (see requireNoWorkspaceAbove).
func newHarness(t *testing.T) *harness {
	t.Helper()
	isolateEnv(t)
	base := t.TempDir()
	requireNoWorkspaceAbove(t, base)
	root := newDir(t, base, "licence-projet")
	return &harness{
		t:      t,
		cwd:    root,
		root:   root,
		Client: authtest.NewLoggedIn(""),
	}
}

// requireNoWorkspaceAbove skips the test if a Watchoo workspace exists in dir or above (TMPDIR placed
// under a directory that contains a .watchoo/, for example): init would see a parent workspace there
// and print a warning that the tests do not expect.
func requireNoWorkspaceAbove(t *testing.T, dir string) {
	t.Helper()
	if ws, err := workspace.Find(dir); err == nil {
		t.Skipf("a Watchoo workspace exists above the temporary directory %s: %s", dir, ws.Root)
	}
}

// result is the result of a CLI run.
type result struct {
	code   int
	stdout string
	stderr string
}

// run runs "watchoo args…" with a background context.
func (h *harness) run(args ...string) result {
	h.t.Helper()
	return h.runContext(context.Background(), args...)
}

// runContext runs "watchoo args…" with ctx.
func (h *harness) runContext(ctx context.Context, args ...string) result {
	h.t.Helper()
	var stdout, stderr bytes.Buffer
	deps := Deps{
		Stdout: &stdout,
		Stderr: &stderr,
		NewAuthClient: func(cfg workspace.Config) auth.Client {
			h.configs = append(h.configs, cfg)
			if h.Client == nil {
				return DefaultDeps().NewAuthClient(cfg)
			}
			return h.Client
		},
		Getwd:    func() (string, error) { return h.cwd, nil },
		Terminal: h.Terminal,
		NewProvider: func(token string) (provider.Provider, error) {
			h.tokens = append(h.tokens, token)
			if h.NewProvider != nil {
				return h.NewProvider(token)
			}
			if h.Provider == nil {
				return nil, errors.New("no provider in this test")
			}
			return h.Provider, nil
		},
		NewGitProvider: func(repos []gitprov.Ref) (provider.Provider, error) {
			if h.GitURL == "" {
				return nil, nil // clones disabled: without gh, "clones directory not set"
			}
			return gitprov.New(gitprov.Options{Dir: filepath.Join(h.root, workspace.ReposDirName), Repos: repos,
				BaseURL: h.GitURL, Logger: slog.Default()})
		},
		CheckGit: func(context.Context) error {
			if h.GitMissing {
				return gitprov.ErrGitNotFound
			}
			return nil
		},
		Now:             h.Now,
		Stdin:           strings.NewReader(h.Stdin),
		StdinIsTerminal: func() bool { return h.StdinTerminal },
		RunTUI:          h.RunTUI,
		Listen:          h.Listen,
		Logger:          slog.Default(), // testLogs (TestMain)
		Updater:         h.Updater,
		StderrIsTerminal: func() bool {
			return h.StderrTerminal
		},
		Executable: func() (string, error) {
			if h.Executable == "" {
				return "", errors.New("no binary in this test")
			}
			return h.Executable, nil
		},
		InDocker: func() bool { return h.InDocker },
	}
	if deps.Updater == nil {
		deps.Updater = offlineUpdater{}
	}
	if deps.Listen == nil {
		deps.Listen = func(string, string) (net.Listener, error) {
			return nil, errors.New("no network listening in this test")
		}
	}
	if deps.RunTUI == nil {
		deps.RunTUI = func(context.Context, tui.Deps, tui.IO) error {
			return errors.New("the real TUI must not be started in the tests")
		}
	}
	code := Run(ctx, deps, args)
	r := result{code: code, stdout: stdout.String(), stderr: stderr.String()}
	// The token never appears: neither in the outputs nor in the logs.
	for name, out := range map[string]string{"stdout": r.stdout, "stderr": r.stderr, "logs": testLogs.String()} {
		if strings.Contains(out, authtest.FakeToken) {
			h.t.Errorf("watchoo %q: token in %s:\n%s", args, name, out)
		}
	}
	return r
}

// fake returns the harness's authtest.Fake.
func (h *harness) fake() *authtest.Fake {
	h.t.Helper()
	f, ok := h.Client.(*authtest.Fake)
	if !ok {
		h.t.Fatalf("Client is not an *authtest.Fake: %T", h.Client)
	}
	return f
}

// mustSucceed checks that the run succeeded.
func (r result) mustSucceed(t *testing.T) result {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("exit code %d, want 0\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
	return r
}

// mustFail checks that the run failed (code 1), with nothing on the standard output, with a prefixed
// error message ("error: …" or "erreur : …", in the test's language) that contains each of the
// excerpts.
func (r result) mustFail(t *testing.T, parts ...string) result {
	t.Helper()
	if r.code != 1 {
		t.Fatalf("exit code %d, want 1\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
	if r.stdout != "" {
		t.Errorf("non-empty stdout after an error:\n%s", r.stdout)
	}
	// The prefix comes from the cli.core.err.line key: "error: " (English) or "erreur : " (French),
	// the empty value leaving the prefix alone.
	if prefix := i18n.T("cli.core.err.line", ""); !strings.HasPrefix(r.stderr, prefix) {
		t.Errorf("stderr without %q at the start:\n%s", prefix, r.stderr)
	}
	for _, part := range parts {
		if !strings.Contains(r.stderr, part) {
			t.Errorf("stderr without %q:\n%s", part, r.stderr)
		}
	}
	return r
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

// assertNoWorkspace checks that root contains no .watchoo/.
func assertNoWorkspace(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, workspace.DirName)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s/%s should not exist (Lstat: %v)", root, workspace.DirName, err)
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

// migratedTables is the list of the tables created by the migrations (goose's table excluded).
var migratedTables = []string{
	"branches", "commit_branches", "commit_signals", "commits", "contributor_emails", "contributors", "pull_requests", "releases",
	"diagnostic_reviews", "repositories", "repository_signals", "sync_runs", "tags",
}

// tables returns the tables of the database path, sorted, SQLite's internal tables excluded. The
// database is opened directly, without going through store (no pragmas, no migrations), to check the
// file as init left it. No mode=ro: a WAL database without a -shm file does not always open read-only.
func tables(t *testing.T, path string) []string {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	rows, err := sqlDB.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("reading the tables of %s: %v", path, err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(names)
	return names
}
