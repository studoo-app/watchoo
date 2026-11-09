package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/configshare"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// configPath returns the path of .watchoo/config.yaml of the harness workspace.
func configPath(h *harness) string {
	return filepath.Join(h.cwd, workspace.DirName, workspace.ConfigFileName)
}

// listRepos returns the repositories of the database of the harness workspace.
func listRepos(t *testing.T, h *harness) []model.Repository {
	t.Helper()
	repos, err := openTestStore(t, h).ListRepositories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return repos
}

// sharedDoc is a dummy sharing document: alice (new label), bob (branches), carol (inaccessible),
// dave (without label or branches).
const sharedDoc = `{
  "format": "watchoo-config",
  "version": 1,
  "exported_at": "2026-10-20T08:00:00Z",
  "watchoo_version": "v0.1.0",
  "workspace": {"name": "licence-projet"},
  "repositories": [
    {"provider": "github", "repo": "alice/projet", "label": "Alice M.", "branches": []},
    {"provider": "github", "repo": "bob/projet", "label": "Bob Durand", "branches": ["main", "dev"]},
    {"provider": "github", "repo": "carol/projet", "label": "Carol Leroy", "branches": []},
    {"repo": "dave/projet", "label": ""}
  ]
}
`

func TestConfigExport(t *testing.T) {
	h := newSyncedWorkspace(t)
	h.run("repo", "add", "bob/projet", "--branch", "main,dev").mustSucceed(t)
	h.run("repo", "remove", "dave/projet", "--keep-history", "--yes").mustSucceed(t)

	r := h.run("config", "export").mustSucceed(t)
	assertSingleJSONObject(t, "config export", r.stdout)
	var doc configshare.Document
	decodeStrict(t, r.stdout, &doc)
	if doc.Format != configshare.Format || doc.Version != 1 || !doc.ExportedAt.Equal(testNow) ||
		doc.Workspace.Name != "licence-projet" || doc.WatchooVersion != appVersion() {
		t.Errorf("document header = %+v", doc)
	}
	// dave is stopped: not exported; sorted by owner/name.
	want := []configshare.Repository{
		{Provider: "github", Repo: "alice/projet", Label: "Alice Martin", Branches: []string{}},
		{Provider: "github", Repo: "bob/projet", Label: "", Branches: []string{"main", "dev"}},
	}
	if !reflect.DeepEqual(doc.Repositories, want) {
		t.Errorf("exported repositories = %+v, want %+v", doc.Repositories, want)
	}
	if doc.Settings == nil || doc.Settings.Timezone == nil || *doc.Settings.Timezone != "UTC" ||
		doc.Settings.Sync == nil || *doc.Settings.Sync.Interval != "30m" || doc.Settings.Lang == nil || *doc.Settings.Lang != "en" {
		t.Errorf("exported settings = %+v", doc.Settings)
	}
	assertJSONKeys(t, r.stdout, "", jsonTopLevelKeys["config export"]...)
	assertJSONKeys(t, r.stdout, "settings", "sync", "activity", "branches", "timezone", "lang", "conventions")
	assertJSONKeys(t, r.stdout, "settings.conventions", "since", "commit", "branch", "pr", "tags", "changelog", "readme", "docs")
	assertJSONKeys(t, r.stdout, "settings.conventions.commit", "types", "scopes", "max_subject_soft",
		"max_subject_hard", "forbidden", "language")
	assertJSONKeys(t, r.stdout, "settings.conventions.branch", "pattern", "exclude")
	assertJSONKeys(t, r.stdout, "settings.conventions.pr", "body_sections", "require_closes")
	assertJSONKeys(t, r.stdout, "settings.conventions.tags", "milestones", "prerelease_forms")
	assertJSONKeys(t, r.stdout, "settings.conventions.changelog", "categories_fr")
	assertJSONKeys(t, r.stdout, "settings.conventions.readme", "sections", "min_words")
	assertJSONKeys(t, r.stdout, "settings.conventions.docs", "dir", "media_dir", "archives_scope")
	// No e-mail, no token, no machine setting, no local path, no header card; neither
	// conventions.enabled nor conventions.excluded_accounts.
	assertNotContains(t, "config export", r.stdout, "@", authtest.FakeToken, "gh_path", `"github":`, `"web"`,
		h.cwd, "\x1b", `"enabled"`, "excluded_accounts")

	// --json: same document.
	if rj := h.run("config", "export", "--json").mustSucceed(t); rj.stdout != r.stdout {
		t.Errorf("different export --json:\n%s\nwant:\n%s", rj.stdout, r.stdout)
	}

	// On a terminal: no card either.
	h.Terminal = func() TermInfo { return wideTerminal }
	if rt := h.run("config", "export").mustSucceed(t); rt.stdout != r.stdout {
		t.Errorf("export on a terminal:\n%s", rt.stdout)
	}
	h.Terminal = nil

	// -o: file in 0600, confirmation line.
	r = h.run("config", "export", "-o", "licence.json").mustSucceed(t)
	if r.stdout != "Configuration exported to licence.json: 2 repositories, 27 settings.\n" {
		t.Errorf("export -o = %q", r.stdout)
	}
	out := filepath.Join(h.cwd, "licence.json")
	assertMode(t, out, 0o600)
	var fromFile configshare.Document
	decodeStrict(t, readFile(t, out), &fromFile)
	if !reflect.DeepEqual(fromFile.Repositories, want) {
		t.Errorf("exported file = %+v", fromFile.Repositories)
	}

	// Existing file: refused without --force, nothing is touched.
	writeFile(t, out, "old content\n")
	h.run("config", "export", "-o", "licence.json").mustFail(t, "licence.json already exists", "--force")
	if got := readFile(t, out); got != "old content\n" {
		t.Errorf("file overwritten without --force: %q", got)
	}
	if err := os.Chmod(out, 0o644); err != nil {
		t.Fatal(err)
	}
	// --force --json: file replaced (0600), nothing on the standard output.
	r = h.run("config", "export", "-o", "licence.json", "--force", "--json").mustSucceed(t)
	if r.stdout != "" {
		t.Errorf("export -o --json wrote on the standard output: %q", r.stdout)
	}
	assertMode(t, out, 0o600)
	decodeStrict(t, readFile(t, out), &fromFile)

	// A directory is never replaced.
	newDir(t, h.cwd, "rep")
	h.run("config", "export", "-o", "rep", "--force").mustFail(t, "is a directory")

	// Arguments.
	h.run("config", "export", "extra").mustFail(t, "takes no argument")
}

// Export of a workspace, import into another workspace (a colleague's): preview, application, new
// run without change. WATCHOO_TIMEZONE=UTC (fixtures) is exported as the effective value; in the
// importing workspace, the variable overrides timezone: the value is written to config.yaml
// (compared to the file's, ""), reported, then the new run rewrites nothing any more.
func TestConfigImportRoundTrip(t *testing.T) {
	a := newSyncedWorkspace(t)
	a.run("repo", "add", "bob/projet", "--branch", "main,dev").mustSucceed(t)
	exported := a.run("config", "export").mustSucceed(t).stdout

	b := newWorkspace(t)
	seedProvider(b.Provider)
	writeFile(t, filepath.Join(b.cwd, "licence.json"), exported)
	configBefore := readFile(t, configPath(b))

	// --dry-run: preview with access check; nothing is written.
	r := b.run("config", "import", "licence.json", "--dry-run").mustSucceed(t)
	assertContains(t, "preview", r.stdout,
		"IMPORT  licence.json  (workspace \"licence-projet\", exported on 10/27/2026 with Watchoo "+appVersion()+")\n",
		"Settings\n  timezone                   \"\" → \"UTC\"   (overridden by WATCHOO_TIMEZONE)\n",
		"Repositories\n  to add             3\n",
		"  inaccessible       0\n",
		"! the WATCHOO_TIMEZONE variable is set: it overrides timezone (the imported value will be written",
		"personal comments",
		"Preview only (--dry-run): nothing was modified.\n")
	if n := len(listRepos(t, b)); n != 0 {
		t.Errorf("--dry-run saved %d repositories", n)
	}
	if got := readFile(t, configPath(b)); got != configBefore {
		t.Error("--dry-run changed config.yaml")
	}
	if n := b.Provider.Calls(providertest.MethodRepository); n != 3 {
		t.Errorf("access checked %d times, want 3", n)
	}

	// Application with --yes: preview then summary.
	r = b.run("config", "import", "licence.json", "--yes").mustSucceed(t)
	assertContains(t, "import", r.stdout, "  to add             3\n",
		"(repository access will be checked against GitHub when applying)\n",
		"SUMMARY\n  settings written in ",
		"  added              3\n", "Run \"watchoo sync\"")
	written := readFile(t, configPath(b))
	assertContains(t, "rewritten config.yaml", written, `timezone: "UTC"`)
	bak := filepath.Join(b.cwd, workspace.DirName, workspace.BackupFileName)
	if readFile(t, bak) != configBefore {
		t.Error("config.yaml.bak is not the old file")
	}
	alice := storedRepo(t, b, "alice", "projet")
	bob := storedRepo(t, b, "bob", "projet")
	if alice.Label != "Alice Martin" || len(alice.BranchPatterns) != 0 ||
		!reflect.DeepEqual(bob.BranchPatterns, []string{"main", "dev"}) || !bob.Enabled {
		t.Errorf("imported repositories: alice %+v, bob %+v", alice, bob)
	}

	// New run: nothing to change, no question (even outside a terminal, without --yes).
	r = b.run("config", "import", "licence.json").mustSucceed(t)
	assertContains(t, "new run", r.stdout, "  unchanged          3\n", "Nothing to change: the workspace is already up to date.\n",
		"  already tracked    3\n", "unchanged (27 settings, identical to those of the workspace)\n",
		"! the WATCHOO_TIMEZONE variable is set: it overrides timezone (config.yaml already contains the imported value")
	assertNotContains(t, "new run", r.stderr, "Apply")
	if readFile(t, configPath(b)) != written || readFile(t, bak) != configBefore {
		t.Error("new run: config.yaml or config.yaml.bak rewritten although no setting changes")
	}

	// The second workspace exports the same repositories again.
	var docA, docB configshare.Document
	decodeStrict(t, exported, &docA)
	decodeStrict(t, b.run("config", "export").mustSucceed(t).stdout, &docB)
	if !reflect.DeepEqual(docA.Repositories, docB.Repositories) || !reflect.DeepEqual(docA.Settings, docB.Settings) {
		t.Errorf("round trip: %+v / %+v", docA, docB)
	}
}

// Merge: addition, update, resumption of a stopped repository, unchanged, inaccessible; nothing is
// removed; non-zero code.
func TestConfigImportMerge(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet", "--label", "Alice Martin").mustSucceed(t)
	h.run("repo", "add", "bob/projet", "--label", "Bob Durand").mustSucceed(t)
	h.run("repo", "remove", "bob/projet", "--keep-history", "--yes").mustSucceed(t)
	h.run("repo", "add", "dave/projet").mustSucceed(t)
	writeFile(t, filepath.Join(h.cwd, "partage.json"), sharedDoc)

	// Outside a terminal, without --yes: refused before any output, nothing is changed.
	r := h.run("config", "import", "partage.json").mustFail(t, "--yes")
	if got := storedRepo(t, h, "alice", "projet").Label; got != "Alice Martin" {
		t.Errorf("label changed without confirmation: %q", got)
	}

	// Preview without access check (not --dry-run, on a terminal, answer "no").
	h.StdinTerminal, h.Stdin = true, "n\n"
	r = h.run("config", "import", "partage.json")
	h.StdinTerminal, h.Stdin = false, ""
	if r.code != 1 {
		t.Fatalf("import refused on the terminal: code %d\n%s", r.code, r.stderr)
	}
	assertContains(t, "preview", r.stdout,
		"  to add             1\n",
		"  to update          1   alice/projet (label: \"Alice Martin\" → \"Alice M.\")\n",
		"  to resume          1   bob/projet (branches: all → main, dev)\n",
		"  unchanged          1\n",
		"Settings\n  none in the file\n")
	assertContains(t, "refusal", r.stderr, "nothing was modified")
	if got := storedRepo(t, h, "alice", "projet").Label; got != "Alice Martin" {
		t.Errorf("label changed without confirmation: %q", got)
	}

	// --dry-run --json: access check, carol inaccessible, non-zero code, nothing written.
	r = h.run("config", "import", "partage.json", "--dry-run", "--json").mustCode(t, 1)
	var rep configImportReport
	decodeStrict(t, r.stdout, &rep)
	wantSum := configSummaryJSON{Added: 0, AlreadyTracked: 1, Updated: 2, Reenabled: 1, Inaccessible: 1}
	if !rep.DryRun || rep.Applied || rep.Summary != wantSum || len(rep.Repositories) != 4 ||
		rep.Repositories[2].Action != "inaccessible" || rep.Repositories[2].Error == "" ||
		rep.Repositories[1].Action != "update" || !rep.Repositories[1].Reenable ||
		rep.Repositories[1].Before == nil || rep.Repositories[1].Before.Enabled ||
		rep.Repositories[0].Outcome != "" {
		t.Errorf("preview --json = %+v", rep)
	}
	assertContains(t, "--dry-run --json", r.stderr, "import incomplete: 1 inaccessible repository (carol/projet)")
	if storedRepo(t, h, "bob", "projet").Enabled {
		t.Error("--dry-run resumed the tracking of bob")
	}

	// Application in JSON: summary on the standard output, non-zero code (carol).
	r = h.run("config", "import", "partage.json", "--yes", "--json").mustCode(t, 1)
	decodeStrict(t, r.stdout, &rep)
	if rep.DryRun || !rep.Applied || rep.Summary != wantSum || rep.Repositories[0].Outcome != "updated" ||
		rep.Repositories[2].Outcome != "inaccessible" || rep.Settings.Written {
		t.Errorf("summary --json = %+v", rep)
	}
	alice, bob, dave := storedRepo(t, h, "alice", "projet"), storedRepo(t, h, "bob", "projet"),
		storedRepo(t, h, "dave", "projet")
	if alice.Label != "Alice M." || !bob.Enabled || !reflect.DeepEqual(bob.BranchPatterns, []string{"main", "dev"}) ||
		dave.Label != "" || !dave.Enabled {
		t.Errorf("after the import: alice %+v, bob %+v, dave %+v", alice, bob, dave)
	}
	if n := len(listRepos(t, h)); n != 3 {
		t.Errorf("%d repositories, want 3 (nothing is removed, carol not added)", n)
	}

	// As text: summary with the detail of the failure.
	r = h.run("config", "import", "partage.json", "--yes").mustCode(t, 1)
	assertContains(t, "text summary", r.stdout, "SUMMARY\n", "  inaccessible       1   carol/projet\n",
		"  repository #3  carol/projet: ")
	assertContains(t, "error", r.stderr, "error: import incomplete: 1 inaccessible repository (carol/projet)")
}

// Settings: rewrite of config.yaml (machine keys kept, .bak, 0600), warnings.
func TestConfigImportSettings(t *testing.T) {
	h := newWorkspace(t)
	cfg := configPath(h)
	// A key specific to the machine, to keep.
	writeFile(t, cfg, readFile(t, cfg)+"github:\n  gh_path: /opt/gh/bin/gh\n")
	if err := os.Chmod(cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	original := readFile(t, cfg)
	doc := `{"format":"watchoo-config","version":1,"settings":{"sync":{"interval":"1h"},
"activity":{"dead_after_days":21},"timezone":"Europe/Paris","lang":"fr"}}`
	writeFile(t, filepath.Join(h.cwd, "reglages.json"), doc)

	r := h.run("config", "import", "reglages.json", "--dry-run").mustSucceed(t)
	assertContains(t, "settings preview", r.stdout,
		"IMPORT  reglages.json\n",
		"  sync.interval              30m → 1h\n",
		"  activity.dead_after_days   15 → 21\n",
		`  timezone                   "" → "Europe/Paris"   (overridden by WATCHOO_TIMEZONE)`,
		`  lang                       "en" → "fr"`,
		"Repositories\n  none in the file\n",
		"! the WATCHOO_TIMEZONE variable is set",
		"! config.yaml will be rewritten")
	if readFile(t, cfg) != original {
		t.Fatal("--dry-run changed config.yaml")
	}

	// Confirmation on a terminal: "n" cancels.
	h.StdinTerminal, h.Stdin = true, "n\n"
	h.run("config", "import", "reglages.json").mustCode(t, 1)
	if readFile(t, cfg) != original {
		t.Fatal("config.yaml changed despite the refusal")
	}
	// "o" applies (accepted like "y").
	h.Stdin = "o\n"
	r = h.run("config", "import", "reglages.json").mustSucceed(t)
	assertContains(t, "question", r.stderr, "Apply these changes? [y/N]")
	assertContains(t, "summary", r.stdout, "SUMMARY\n  settings written in ", "config.yaml.bak",
		"next start")
	assertNotContains(t, "summary without repository", r.stdout, "added")
	written := readFile(t, cfg)
	assertContains(t, "rewritten config.yaml", written, "interval: 1h", "dead_after_days: 21",
		`timezone: "Europe/Paris"`, `lang: "fr"`, `gh_path: "/opt/gh/bin/gh"`)
	assertMode(t, cfg, 0o600)
	bak := filepath.Join(h.cwd, workspace.DirName, workspace.BackupFileName)
	if readFile(t, bak) != original {
		t.Error("config.yaml.bak is not the old file")
	}
	assertMode(t, bak, 0o600)

	// The next commands see the settings (except the time zone, overridden by the variable).
	var exp configshare.Document
	decodeStrict(t, h.run("config", "export").mustSucceed(t).stdout, &exp)
	if *exp.Settings.Sync.Interval != "1h" || *exp.Settings.Activity.DeadAfterDays != 21 || *exp.Settings.Timezone != "UTC" ||
		*exp.Settings.Lang != "fr" {
		t.Errorf("settings after the import: %+v", exp.Settings)
	}

	// Settings refused once merged: nothing is written.
	writeFile(t, filepath.Join(h.cwd, "faux.json"),
		`{"format":"watchoo-config","version":1,"settings":{"activity":{"flemme_after_days":30}}}`)
	h.run("config", "import", "faux.json", "--yes").mustFail(t, "faux.json: ", "nothing was imported")
	if readFile(t, cfg) != written {
		t.Error("config.yaml changed by a refused import")
	}
}

// Confirmation: --yes mandatory outside a terminal, with --json and with "-".
func TestConfigImportConfirmation(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	writeFile(t, filepath.Join(h.cwd, "partage.json"), sharedDoc)

	h.run("config", "import", "partage.json", "--json").mustFail(t, "--yes")
	h.Stdin = sharedDoc
	h.run("config", "import", "-").mustFail(t, "--yes")
	if n := h.Provider.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("access checked %d times before the confirmation", n)
	}
	if n := len(listRepos(t, h)); n != 0 {
		t.Errorf("%d repositories saved without confirmation", n)
	}

	// Terminal, empty answer (= no).
	h.StdinTerminal, h.Stdin = true, "\n"
	r := h.run("config", "import", "partage.json").mustCode(t, 1)
	assertContains(t, "cancelled", r.stderr, "import cancelled: nothing was modified")
	if n := len(listRepos(t, h)); n != 0 {
		t.Errorf("%d repositories saved after \"no\"", n)
	}

	// Standard input with --yes.
	h.StdinTerminal, h.Stdin = false, sharedDoc
	r = h.run("config", "import", "-", "--yes").mustCode(t, 1) // carol inaccessible
	assertContains(t, "import -", r.stdout, "IMPORT  -  (workspace \"licence-projet\", exported on 10/20/2026",
		"  added              3\n")
}

// Refused files: nothing is changed, no access checked.
func TestConfigImportInvalid(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	cfg := readFile(t, configPath(h))
	big := `{"format":"watchoo-config","version":1,"workspace":{"name":"` +
		strings.Repeat("x", configshare.MaxFileSize) + `"}}`
	for _, tt := range []struct {
		name, content string
		want          []string
	}{
		{"not JSON", "repo,label\nalice/projet,Alice\n", []string{"nothing was imported"}},
		{"format missing", `{"version":1}`, []string{"not a shared Watchoo configuration"}},
		{"newer version", `{"format":"watchoo-config","version":2}`, []string{"newer version"}},
		{"machine key", `{"format":"watchoo-config","version":1,"settings":{"github":{"gh_path":"/tmp/x"}}}`,
			[]string{"github"}},
		{"duplicate", `{"format":"watchoo-config","version":1,"repositories":[{"repo":"alice/projet"},{"repo":"Alice/Projet"}]}`,
			[]string{"present twice"}},
		{"unknown language", `{"format":"watchoo-config","version":1,"settings":{"lang":"de"}}`,
			[]string{"settings.lang", `"de"`}},
		{"too large", big, []string{"too large"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writeFile(t, filepath.Join(h.cwd, "f.json"), tt.content)
			h.run("config", "import", "f.json", "--yes").mustFail(t, append([]string{"f.json: "}, tt.want...)...)
			h.run("config", "import", "f.json", "--dry-run", "--json").mustFail(t, tt.want...)
		})
	}
	h.run("config", "import", "absent.json", "--yes").mustFail(t, "configuration file absent.json not found")
	h.run("config", "import").mustFail(t)
	if n := h.Provider.Calls(providertest.MethodRepository); n != 0 || len(h.tokens) != 0 {
		t.Errorf("access checked despite invalid files (%d calls)", n)
	}
	if n := len(listRepos(t, h)); n != 0 || readFile(t, configPath(h)) != cfg {
		t.Errorf("workspace changed by invalid files (%d repositories)", n)
	}
}

// The texts coming from the file are cleaned before display.
func TestConfigImportCleansText(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	doc, err := json.Marshal(map[string]any{
		"format": "watchoo-config", "version": 1,
		"repositories": []map[string]any{{"repo": "alice/projet", "label": "Alice ‮M"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(h.cwd, "f.json"), string(doc))
	r := h.run("config", "import", "f.json", "--dry-run")
	assertNotContains(t, "preview", r.stdout+r.stderr, "‮", "\x1b")
}

func TestConfigHelp(t *testing.T) {
	h := newHarness(t)
	r := h.run("config").mustSucceed(t)
	assertContains(t, "config help", r.stdout, "export", "import", "Shares the configuration")
	r = h.run("config", "import", "--help").mustSucceed(t)
	assertContains(t, "config import help", r.stdout, "--dry-run", "--yes", "nothing is ever removed")
	h.run("config", "unknown").mustFail(t, `unknown command "unknown"`)
}
