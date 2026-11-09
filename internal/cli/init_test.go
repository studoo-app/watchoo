package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	"github.com/studoo-app/watchoo/internal/workspace"
)

func TestInitCreatesWorkspace(t *testing.T) {
	h := newHarness(t)
	root := h.cwd
	r := h.run("init").mustSucceed(t)

	// Summary in English (the tests' language): name, paths and gh account.
	for _, want := range []string{
		"Workspace \"licence-projet\" created.",
		"directory   " + root + "\n",
		"settings    " + filepath.Join(root, ".watchoo", "config.yaml") + "\n",
		"database    " + filepath.Join(root, ".watchoo", "watchoo.db") + "\n",
		"gh account  watchoo-test on github.com (scopes: gist, read:org, repo)",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("summary without %q:\n%s", want, r.stdout)
		}
	}
	if r.stderr != "" {
		t.Errorf("unexpected stderr:\n%s", r.stderr)
	}

	// gh: checked once with Status, never Token; default settings passed on.
	if got := h.fake().StatusCalls(); got != 1 {
		t.Errorf("Status called %d times, want 1", got)
	}
	if got := h.fake().TokenCalls(); got != 0 {
		t.Errorf("Token called %d times: init must never ask for the token", got)
	}
	if len(h.configs) != 1 || !reflect.DeepEqual(h.configs[0], workspace.DefaultConfig("licence-projet")) {
		t.Errorf("Config passed to NewAuthClient = %+v, want the default values", h.configs)
	}

	// Content and permissions of .watchoo/.
	dir := filepath.Join(root, ".watchoo")
	assertMode(t, dir, 0o700)
	for _, name := range []string{"config.yaml", ".gitignore", "watchoo.db"} {
		assertMode(t, filepath.Join(dir, name), 0o600)
	}
	for _, name := range []string{"watchoo.db-wal", "watchoo.db-shm"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			assertMode(t, filepath.Join(dir, name), 0o600)
		}
	}
	if got := readFile(t, filepath.Join(dir, ".gitignore")); got != "*\n" {
		t.Errorf(".gitignore = %q, want %q", got, "*\n")
	}
	if got := readFile(t, filepath.Join(dir, "config.yaml")); got != workspace.ConfigTemplate(i18n.Process().Lang(), "licence-projet") {
		t.Errorf("config.yaml differs from ConfigTemplate(i18n.Process().Lang(), \"licence-projet\"):\n%s", got)
	}

	// Migrated database: the 15 tables of the model, plus goose's version table.
	want := slices.Sorted(slices.Values(append([]string{"goose_db_version"}, migratedTables...)))
	if got := tables(t, filepath.Join(dir, "watchoo.db")); !reflect.DeepEqual(got, want) {
		t.Errorf("tables = %v, want %v", got, want)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(dir, "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var version int64
	if err := st.DB().QueryRowContext(ctx, `SELECT MAX(version_id) FROM goose_db_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 17 {
		t.Errorf("schema version = %d, want 17 (00017_repository_conventions)", version)
	}
	if repos, err := st.ListRepositories(ctx); err != nil || len(repos) != 0 {
		t.Errorf("ListRepositories = %v, %v; want an empty list", repos, err)
	}

	// The other commands find the workspace, from a subdirectory too.
	ws, err := workspace.ResolveFrom("", newDir(t, root, "src", "module"))
	if err != nil {
		t.Fatalf("ResolveFrom after init: %v", err)
	}
	if ws.Root != root || ws.Config.Name != "licence-projet" {
		t.Errorf("workspace found = %s (name %q), want %s (name licence-projet)", ws.Root, ws.Config.Name, root)
	}
}

// decodeInitReport decodes the JSON output of init, rejecting unknown fields and anything that would
// follow the object.
func decodeInitReport(t *testing.T, stdout string) initReport {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	dec.DisallowUnknownFields()
	var report initReport
	if err := dec.Decode(&report); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	if dec.More() {
		t.Fatalf("text after the JSON object:\n%s", stdout)
	}
	return report
}

func TestInitJSON(t *testing.T) {
	h := newHarness(t)
	root := h.cwd
	r := h.run("init", "--json").mustSucceed(t)
	if r.stderr != "" {
		t.Errorf("unexpected stderr:\n%s", r.stderr)
	}

	got := decodeInitReport(t, r.stdout)
	want := initReport{
		Name:            "licence-projet",
		Root:            root,
		Dir:             filepath.Join(root, ".watchoo"),
		ConfigFile:      filepath.Join(root, ".watchoo", "config.yaml"),
		Database:        filepath.Join(root, ".watchoo", "watchoo.db"),
		ParentWorkspace: "",
		GitHub: githubReport{
			Host:        "github.com",
			Login:       authtest.FakeLogin,
			TokenSource: "keyring",
			Scopes:      []string{"gist", "read:org", "repo"},
		},
		Source:   "api",
		Warnings: []string{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("JSON report = %+v\nwant          %+v", got, want)
	}
	// Lists always present ([] and not null), for scripts.
	for _, key := range []string{`"warnings": []`, `"parent_workspace": ""`} {
		if !strings.Contains(r.stdout, key) {
			t.Errorf("JSON output without %s:\n%s", key, r.stdout)
		}
	}

	// --json placed before the subcommand: same effect (global option).
	h2 := newHarness(t)
	r2 := h2.run("--json", "init").mustSucceed(t)
	if decodeInitReport(t, r2.stdout).Root != h2.cwd {
		t.Errorf("--json before init: unexpected output:\n%s", r2.stdout)
	}
}

func TestInitJSONWithoutScopes(t *testing.T) {
	h := newHarness(t)
	h.fake().SetStatus(auth.Status{Host: "github.com", Login: "ci-bot", TokenSource: "GH_TOKEN",
		Active: true, State: "success"}, nil)

	report := decodeInitReport(t, h.run("init", "--json").mustSucceed(t).stdout)
	if report.GitHub.Scopes == nil || len(report.GitHub.Scopes) != 0 {
		t.Errorf("scopes = %#v, want an empty non-nil list", report.GitHub.Scopes)
	}
	if report.GitHub.TokenSource != "GH_TOKEN" || report.GitHub.Login != "ci-bot" {
		t.Errorf("gh account = %+v", report.GitHub)
	}

	h2 := newHarness(t)
	h2.Client = h.Client
	r := h2.run("init").mustSucceed(t)
	if !strings.Contains(r.stdout, "ci-bot on github.com (scopes not reported by gh)") {
		t.Errorf("summary without scopes: %s", r.stdout)
	}
}

func TestInitWorkspaceFlag(t *testing.T) {
	t.Run("path relative to the current directory", func(t *testing.T) {
		h := newHarness(t)
		target := newDir(t, h.cwd, "promo", "master-projet")
		r := h.run("init", "-C", filepath.Join("promo", "master-projet")).mustSucceed(t)
		if !strings.Contains(r.stdout, "\"master-projet\"") || !strings.Contains(r.stdout, target) {
			t.Errorf("unexpected summary:\n%s", r.stdout)
		}
		if _, err := os.Stat(filepath.Join(target, ".watchoo", "watchoo.db")); err != nil {
			t.Errorf("database missing in the -C directory: %v", err)
		}
		assertNoWorkspace(t, h.cwd)
	})

	t.Run("absolute path, inaccessible current directory", func(t *testing.T) {
		isolateEnv(t)
		target := newDir(t, t.TempDir(), "perso")
		var stdout, stderr bytes.Buffer
		deps := Deps{
			Stdout:        &stdout,
			Stderr:        &stderr,
			NewAuthClient: func(workspace.Config) auth.Client { return authtest.NewLoggedIn("") },
			Getwd:         func() (string, error) { return "", errors.New("directory deleted") },
		}
		if code := Run(context.Background(), deps, []string{"--workspace", target, "init"}); code != 0 {
			t.Fatalf("code %d, stderr:\n%s", code, stderr.String())
		}
		if _, err := os.Stat(filepath.Join(target, ".watchoo")); err != nil {
			t.Errorf(".watchoo missing: %v", err)
		}
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		h := newHarness(t)
		h.run("init", "-C", "absent").mustFail(t, filepath.Join(h.cwd, "absent"), "does not exist")
		if _, err := os.Stat(filepath.Join(h.cwd, "absent")); err == nil {
			t.Error("init must not create the -C directory")
		}
	})

	t.Run("without -C, the current directory", func(t *testing.T) {
		h := newHarness(t)
		h.run("init").mustSucceed(t)
		if _, err := os.Stat(filepath.Join(h.cwd, ".watchoo")); err != nil {
			t.Errorf(".watchoo missing from the current directory: %v", err)
		}
	})
}

func TestInitRefusesExistingWorkspace(t *testing.T) {
	h := newHarness(t)
	h.run("init").mustSucceed(t)
	dir := filepath.Join(h.cwd, ".watchoo")

	// Data of the existing workspace: a change to config.yaml and a repository in the database.
	config := readFile(t, filepath.Join(dir, "config.yaml")) + "# réglage ajouté à la main\n"
	writeFile(t, filepath.Join(dir, "config.yaml"), config)
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(dir, "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRepository(ctx, model.Repository{Provider: model.ProviderGitHub, ProviderID: 42,
		Owner: "alice", Name: "projet-licence", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"init"}, {"init", "--json"}, {"-C", ".", "init"}} {
		h.run(args...).mustFail(t, h.cwd, "already a Watchoo workspace", "nothing was modified")
	}

	if got := readFile(t, filepath.Join(dir, "config.yaml")); got != config {
		t.Errorf("config.yaml changed by the second init:\n%s", got)
	}
	st, err = store.Open(ctx, filepath.Join(dir, "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.GetRepositoryByName(ctx, model.ProviderGitHub, "alice", "projet-licence"); err != nil {
		t.Errorf("repository lost after the second init: %v", err)
	}
	if got := h.fake().TokenCalls(); got != 0 {
		t.Errorf("Token called %d times", got)
	}
}

func TestInitIgnoresConfigOfExistingWorkspace(t *testing.T) {
	h := newHarness(t)
	h.run("init").mustSucceed(t)
	// An invalid config.yaml in the existing workspace: the refusal must stay clear.
	writeFile(t, filepath.Join(h.cwd, ".watchoo", "config.yaml"), "cle_inconnue: 1\n")
	r := h.run("init").mustFail(t, "already a Watchoo workspace")
	if strings.Contains(r.stderr, "cle_inconnue") {
		t.Errorf("init read the config.yaml of the existing workspace:\n%s", r.stderr)
	}
}

func TestInitRefusesWatchooFile(t *testing.T) {
	h := newHarness(t)
	writeFile(t, filepath.Join(h.cwd, ".watchoo"), "pas un répertoire")
	h.run("init").mustFail(t, "is not a directory")
	if got := readFile(t, filepath.Join(h.cwd, ".watchoo")); got != "pas un répertoire" {
		t.Errorf(".watchoo file changed: %q", got)
	}
}

func TestInitNestedWorkspace(t *testing.T) {
	h := newHarness(t)
	parent := h.cwd
	h.run("init").mustSucceed(t)

	// Text: warning on the error output, summary on the standard output.
	// (The "warning: " prefix comes from warn, output.go: outside the scope of this group.)
	h.cwd = newDir(t, parent, "groupe-a")
	r := h.run("init").mustSucceed(t)
	if !strings.Contains(r.stderr, "is inside workspace "+parent) ||
		!strings.Contains(r.stderr, "independent") {
		t.Errorf("parent workspace warning missing or incomplete:\n%s", r.stderr)
	}
	if !strings.Contains(r.stdout, "\"groupe-a\" created") {
		t.Errorf("unexpected summary:\n%s", r.stdout)
	}

	// JSON: same warning, plus parent_workspace.
	h.cwd = newDir(t, parent, "groupe-b", "sous-groupe")
	r = h.run("init", "--json").mustSucceed(t)
	report := decodeInitReport(t, r.stdout)
	if report.ParentWorkspace != parent {
		t.Errorf("parent_workspace = %q, want %q", report.ParentWorkspace, parent)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], parent) {
		t.Errorf("warnings = %q", report.Warnings)
	}
	if !strings.Contains(r.stderr, report.Warnings[0]) {
		t.Errorf("warning missing from stderr in --json mode:\n%s", r.stderr)
	}

	// The workspaces are independent: each one is found from its own directory.
	for _, root := range []string{parent, filepath.Join(parent, "groupe-a"), h.cwd} {
		ws, err := workspace.ResolveFrom("", root)
		if err != nil || ws.Root != root {
			t.Errorf("ResolveFrom(%s) = %v, %v", root, ws, err)
		}
	}
}

// Without gh (missing or not signed in), init creates the workspace and warns that the repositories will
// be tracked by local Git clones (V2); without git either, it stops.
func TestInitWithoutGH(t *testing.T) {
	t.Run("real auth.GH, nonexistent path", func(t *testing.T) {
		h := newHarness(t)
		h.Client = nil // real auth.GH, built from the configuration
		missing := filepath.Join(t.TempDir(), "gh-absent")
		t.Setenv("WATCHOO_GITHUB_GH_PATH", missing)
		r := h.run("init").mustSucceed(t)
		assertContains(t, "init without gh", r.stdout, "source      local Git clones")
		assertContains(t, "init without gh (warning)", r.stderr, "gh not found", missing,
			"install the GitHub CLI gh", "local Git clones", "gh auth login")
		if _, err := os.Stat(filepath.Join(h.cwd, ".watchoo", "watchoo.db")); err != nil {
			t.Errorf("workspace not created: %v", err)
		}
	})

	cases := map[string]func() *authtest.Fake{
		"gh absent": func() *authtest.Fake {
			f := authtest.NewLoggedIn("")
			f.SetStatus(auth.Status{}, fmt.Errorf("launching \"gh\": %w", auth.ErrGHNotFound))
			return f
		},
		"not signed in": authtest.NewLoggedOut,
		"message without advice": func() *authtest.Fake {
			f := authtest.NewLoggedIn("")
			f.SetStatus(auth.Status{}, &wrappedError{msg: "account missing", err: auth.ErrNotLoggedIn})
			return f
		},
	}
	for name, newFake := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.Client = newFake()
			r := h.run("init", "--json").mustSucceed(t)
			got := decodeInitReport(t, r.stdout)
			if got.Source != "git" || got.GitHub.Login != "" || len(got.Warnings) != 1 ||
				!strings.Contains(got.Warnings[0], "local Git clones") || !strings.Contains(got.Warnings[0], "gh auth login") {
				t.Errorf("JSON report without gh = %+v", got)
			}
			if n := strings.Count(r.stderr, "gh auth login"); n != 1 {
				t.Errorf("advice \"gh auth login\" repeated %d times:\n%s", n, r.stderr)
			}
			if got := h.fake().TokenCalls(); got != 0 {
				t.Errorf("Token called %d times", got)
			}

			// Without git either: stop, nothing created.
			h2 := newHarness(t)
			h2.Client = newFake()
			h2.GitMissing = true
			r = h2.run("init").mustFail(t, "local Git clones", "git not found")
			if strings.Count(r.stderr, "\n") != 1 {
				t.Errorf("message on several lines:\n%s", r.stderr)
			}
			assertNoWorkspace(t, h2.cwd)
		})
	}
}

// Token refused by GitHub (gh signed in to an account whose token is revoked): gh keeps this token and
// the syncs would go through the API; init stops and advises signing in again.
func TestInitTokenRefused(t *testing.T) {
	h := newHarness(t)
	f := authtest.NewLoggedIn("alice")
	f.SetStatus(auth.Status{Host: "github.com", Login: "alice", State: "error"},
		fmt.Errorf("the gh token of account \"alice\" is not usable: %w", auth.ErrNotLoggedIn))
	// (The authentication error itself comes from the auth package: the advice is checked here through
	// the command line, which does not depend on the language.)
	h.Client = f
	r := h.run("init").mustFail(t, "gh auth login")
	if n := strings.Count(r.stderr, "gh auth login"); n != 1 {
		t.Errorf("advice \"gh auth login\" repeated %d times:\n%s", n, r.stderr)
	}
	assertNoWorkspace(t, h.cwd)
	if got := h.fake().TokenCalls(); got != 0 {
		t.Errorf("Token called %d times", got)
	}
}

// wrappedError is an error whose message does not repeat the one of the wrapped error.
type wrappedError struct {
	msg string
	err error
}

func (e *wrappedError) Error() string { return e.msg }
func (e *wrappedError) Unwrap() error { return e.err }

func TestInitOtherGHErrors(t *testing.T) {
	for name, err := range map[string]error{
		"timeout": fmt.Errorf("\"gh auth status\": %w (10s)", auth.ErrTimeout),
		"answer":  fmt.Errorf("reading the answer of \"gh auth status\": %w", auth.ErrUnexpectedOutput),
		"GitHub unreachable": fmt.Errorf("the gh token of account \"alice\" could not be checked: %w",
			auth.ErrCheckFailed),
		"unusable github.user": fmt.Errorf("github.user is \"Alice\", but gh knows \"alice\": %w",
			auth.ErrUnusableUser),
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.fake().SetStatus(auth.Status{}, err)
			r := h.run("init").mustFail(t, "checking gh: "+err.Error())
			// Signing in again would not fix anything: no "gh auth login" advice.
			if strings.Contains(r.stderr, "gh auth login") {
				t.Errorf("inappropriate \"gh auth login\" advice:\n%s", r.stderr)
			}
			assertNoWorkspace(t, h.cwd)
		})
	}
}

func TestInitInterrupted(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h.runContext(ctx, "init").mustFail(t, "command interrupted")
	assertNoWorkspace(t, h.cwd)
}

func TestInitUsesUserAndEnvironmentSettings(t *testing.T) {
	h := newHarness(t)
	userConfig := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "watchoo", "config.yaml")
	writeFile(t, userConfig, "github:\n  gh_path: /opt/gh/bin/gh\n  user: compte-utilisateur\n")
	t.Setenv("WATCHOO_GITHUB_USER", "compte-env")

	h.run("init").mustSucceed(t)
	if len(h.configs) != 1 {
		t.Fatalf("NewAuthClient called %d times, want 1", len(h.configs))
	}
	if got := h.configs[0].GitHub; got.GHPath != "/opt/gh/bin/gh" || got.User != "compte-env" {
		t.Errorf("gh settings passed on = %+v, want gh_path from the user file and user from the environment", got)
	}
}

func TestInitInvalidSettings(t *testing.T) {
	h := newHarness(t)
	userConfig := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "watchoo", "config.yaml")
	// Unknown key in the user file: the error quotes the file.
	writeFile(t, userConfig, "sync:\n  intervall: 1h\n")
	h.run("init").mustFail(t, userConfig, "sync.intervall")
	// Value out of bounds: the error quotes the key.
	writeFile(t, userConfig, "sync:\n  interval: 5m\n")
	h.run("init").mustFail(t, "invalid settings", "sync.interval")
	assertNoWorkspace(t, h.cwd)
	if got := h.fake().StatusCalls(); got != 0 {
		t.Errorf("Status called %d times despite invalid settings", got)
	}
}

func TestInitDatabaseFailureRemovesWorkspace(t *testing.T) {
	h := newHarness(t)
	original := setupDatabase
	t.Cleanup(func() { setupDatabase = original })
	var dbPath string
	setupDatabase = func(ctx context.Context, path string) error {
		dbPath = path
		if err := original(ctx, path); err != nil {
			return err
		}
		return errors.New("disk full (simulated)")
	}

	h.run("init").mustFail(t, "creating the workspace database", "disk full (simulated)")
	if dbPath != filepath.Join(h.cwd, ".watchoo", "watchoo.db") {
		t.Errorf("database created at %q", dbPath)
	}
	assertNoWorkspace(t, h.cwd)

	// Once the problem is fixed, init works again.
	setupDatabase = original
	h.run("init").mustSucceed(t)
}

func TestInitWarnsAboutWatchooDir(t *testing.T) {
	t.Run("other workspace", func(t *testing.T) {
		h := newHarness(t)
		other := newDir(t, t.TempDir(), "autre")
		t.Setenv("WATCHOO_DIR", other)
		r := h.run("init").mustSucceed(t)
		if !strings.Contains(r.stderr, "warning: variable WATCHOO_DIR is set to \""+other+"\"") {
			t.Errorf("WATCHOO_DIR warning missing:\n%s", r.stderr)
		}
		// init works on the current directory, not on WATCHOO_DIR.
		assertNoWorkspace(t, other)
	})

	t.Run("other existing workspace", func(t *testing.T) {
		h := newHarness(t)
		other := newDir(t, t.TempDir(), "autre")
		h2 := &harness{t: t, cwd: other, Client: authtest.NewLoggedIn("")}
		h2.run("init").mustSucceed(t)
		t.Setenv("WATCHOO_DIR", filepath.Join(other, "sous-repertoire"))
		newDir(t, other, "sous-repertoire")
		r := h.run("init").mustSucceed(t)
		if !strings.Contains(r.stderr, "the other commands will use workspace "+other+", not this one") {
			t.Errorf("WATCHOO_DIR warning missing or incomplete:\n%s", r.stderr)
		}
	})

	t.Run("same workspace", func(t *testing.T) {
		h := newHarness(t)
		t.Setenv("WATCHOO_DIR", filepath.Join(h.cwd, "sous-repertoire-a-venir"))
		newDir(t, h.cwd, "sous-repertoire-a-venir")
		r := h.run("init").mustSucceed(t)
		if r.stderr != "" {
			t.Errorf("unexpected warning:\n%s", r.stderr)
		}
	})

	// A WATCHOO_DIR that does not exist or that points to a file, in the new workspace: Find would walk up
	// to the created workspace, but the other commands will refuse this path (ResolveFrom).
	for name, setup := range map[string]func(h *harness) string{
		"nonexistent directory":     func(h *harness) string { return filepath.Join(h.cwd, "typo") },
		"nonexistent relative path": func(h *harness) string { return "typo" },
		"file": func(h *harness) string {
			path := filepath.Join(h.cwd, "notes.txt")
			writeFile(t, path, "pas un répertoire")
			return path
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			env := setup(h)
			t.Setenv("WATCHOO_DIR", env)
			r := h.run("init", "--json").mustSucceed(t)
			want := "variable WATCHOO_DIR is set to " + fmt.Sprintf("%q", env) +
				": while it is set, the other commands will fail"
			if !strings.Contains(r.stderr, "warning: "+want) {
				t.Errorf("WATCHOO_DIR warning missing:\n%s", r.stderr)
			}
			if report := decodeInitReport(t, r.stdout); len(report.Warnings) != 1 ||
				!strings.HasPrefix(report.Warnings[0], want) {
				t.Errorf("warnings = %q", report.Warnings)
			}
			// The warning is right: the other commands' lookup does fail.
			if _, err := workspace.ResolveFrom("", h.cwd); err == nil {
				t.Error("ResolveFrom succeeded with this WATCHOO_DIR")
			}
		})
	}

	t.Run("with -C", func(t *testing.T) {
		h := newHarness(t)
		t.Setenv("WATCHOO_DIR", t.TempDir())
		target := newDir(t, h.cwd, "cible")
		r := h.run("init", "-C", target).mustSucceed(t)
		if r.stderr != "" {
			t.Errorf("unexpected warning with -C:\n%s", r.stderr)
		}
	})
}

// fakeGHCall is a call recorded by the fake gh (FAKEGH_RECORD_FILE).
type fakeGHCall struct {
	Args []string `json:"args"`
}

// useFakeGH makes the harness go through the real auth.GH and the fake gh binary, with the given
// scenario; it returns the path of the file where the fake gh records its calls.
func useFakeGH(t *testing.T, h *harness, scenario string) string {
	t.Helper()
	h.Client = nil
	dir := t.TempDir()
	record := filepath.Join(dir, "calls.jsonl")
	t.Setenv("WATCHOO_GITHUB_GH_PATH", fakeGHPath)
	t.Setenv("FAKEGH_SCENARIO", scenario)
	t.Setenv("FAKEGH_RECORD_FILE", record)
	t.Setenv("FAKEGH_STATE_FILE", filepath.Join(dir, "state"))
	return record
}

// readCalls returns the calls recorded by the fake gh.
func readCalls(t *testing.T, record string) []fakeGHCall {
	t.Helper()
	data, err := os.ReadFile(record)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls []fakeGHCall
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		var c fakeGHCall
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatalf("unreadable recorded call: %v", err)
		}
		calls = append(calls, c)
	}
	return calls
}

// assertOnlyStatusCalls checks that the fake gh only received "auth status", never "auth token".
func assertOnlyStatusCalls(t *testing.T, record string) {
	t.Helper()
	calls := readCalls(t, record)
	if len(calls) == 0 {
		t.Fatal("the fake gh was not called")
	}
	for _, c := range calls {
		if len(c.Args) < 2 || c.Args[0] != "auth" || c.Args[1] != "status" {
			t.Errorf("unexpected call of the fake gh: %q (init must only run \"gh auth status\")", c.Args)
		}
	}
}

func TestInitWithFakeGHBinary(t *testing.T) {
	t.Run("signed in", func(t *testing.T) {
		h := newHarness(t)
		record := useFakeGH(t, h, "logged-in")
		report := decodeInitReport(t, h.run("init", "--json").mustSucceed(t).stdout)
		if report.GitHub.Login != authtest.FakeLogin || report.GitHub.TokenSource != "keyring" {
			t.Errorf("gh account = %+v", report.GitHub)
		}
		assertOnlyStatusCalls(t, record)
	})

	t.Run("account chosen by github.user", func(t *testing.T) {
		h := newHarness(t)
		record := useFakeGH(t, h, "multi-account")
		t.Setenv("WATCHOO_GITHUB_USER", "watchoo-other")
		report := decodeInitReport(t, h.run("init", "--json").mustSucceed(t).stdout)
		if report.GitHub.Login != "watchoo-other" || !reflect.DeepEqual(report.GitHub.Scopes, []string{"repo"}) {
			t.Errorf("gh account = %+v, want watchoo-other", report.GitHub)
		}
		assertOnlyStatusCalls(t, record)
	})

	// No account: workspace created, repositories tracked by local Git clones (V2).
	for _, scenario := range []string{"not-logged-in", "exit-not-logged-in"} {
		t.Run(scenario, func(t *testing.T) {
			h := newHarness(t)
			record := useFakeGH(t, h, scenario)
			r := h.run("init", "--json").mustSucceed(t)
			if report := decodeInitReport(t, r.stdout); report.Source != "git" {
				t.Errorf("source = %q, want git", report.Source)
			}
			assertContains(t, scenario, r.stderr, "local Git clones", "run \"gh auth login\"")
			assertOnlyStatusCalls(t, record)
		})
	}

	// Token refused by GitHub: sign in again.
	t.Run("invalid-token", func(t *testing.T) {
		h := newHarness(t)
		record := useFakeGH(t, h, "invalid-token")
		h.run("init").mustFail(t, "gh auth login")
		assertNoWorkspace(t, h.cwd)
		assertOnlyStatusCalls(t, record)
	})

	t.Run("GH_TOKEN only, without github.user", func(t *testing.T) {
		h := newHarness(t)
		record := useFakeGH(t, h, "env-token-only")
		report := decodeInitReport(t, h.run("init", "--json").mustSucceed(t).stdout)
		if report.GitHub.Login != authtest.FakeLogin || report.GitHub.TokenSource != "GH_TOKEN" {
			t.Errorf("gh account = %+v, want the GH_TOKEN one", report.GitHub)
		}
		assertOnlyStatusCalls(t, record)
	})

	// Docker / CI: gh only knows the account through GH_TOKEN, which "gh auth token --user" ignores.
	// init must refuse github.user, otherwise every sync would fail afterwards.
	t.Run("GH_TOKEN only, with github.user", func(t *testing.T) {
		h := newHarness(t)
		record := useFakeGH(t, h, "env-token-only")
		t.Setenv("WATCHOO_GITHUB_USER", authtest.FakeLogin)
		r := h.run("init").mustFail(t, "checking gh", `github.user is "watchoo-test"`, "GH_TOKEN", "empty it")
		if strings.Contains(r.stderr, "gh auth login") {
			t.Errorf("inappropriate \"gh auth login\" advice:\n%s", r.stderr)
		}
		assertNoWorkspace(t, h.cwd)
		assertOnlyStatusCalls(t, record)
	})

	// gh is case-sensitive in "gh auth token --user".
	t.Run("github.user with a different case", func(t *testing.T) {
		h := newHarness(t)
		useFakeGH(t, h, "multi-account")
		t.Setenv("WATCHOO_GITHUB_USER", "Watchoo-Other")
		r := h.run("init").mustFail(t, `github.user is "Watchoo-Other"`, `as "watchoo-other"`, "case-sensitive")
		if strings.Contains(r.stderr, "gh auth login") {
			t.Errorf("inappropriate \"gh auth login\" advice:\n%s", r.stderr)
		}
		assertNoWorkspace(t, h.cwd)
	})

	// GitHub unreachable (proxy, network down): the error says so, without advising "gh auth login".
	t.Run("GitHub unreachable", func(t *testing.T) {
		h := newHarness(t)
		useFakeGH(t, h, "offline")
		r := h.run("init").mustFail(t, "checking gh", "could not be checked", "proxyconnect tcp",
			"check the network connection")
		if strings.Contains(r.stderr, "gh auth login") {
			t.Errorf("inappropriate \"gh auth login\" advice:\n%s", r.stderr)
		}
		if strings.Contains(r.stderr, authtest.FakeToken) {
			t.Errorf("token in stderr:\n%s", r.stderr)
		}
		assertNoWorkspace(t, h.cwd)
	})

	t.Run("github.user account missing", func(t *testing.T) {
		h := newHarness(t)
		useFakeGH(t, h, "logged-in")
		t.Setenv("WATCHOO_GITHUB_USER", "inconnu")
		h.run("init").mustFail(t, "inconnu", "github.user", "gh auth login")
		assertNoWorkspace(t, h.cwd)
	})

	t.Run("gh error", func(t *testing.T) {
		h := newHarness(t)
		useFakeGH(t, h, "error")
		r := h.run("init").mustFail(t, "checking gh")
		if strings.Contains(r.stderr, authtest.FakeToken) {
			t.Errorf("token in stderr:\n%s", r.stderr)
		}
		assertNoWorkspace(t, h.cwd)
	})
}
