package cli

// Contract of the --json outputs: every command that produces data writes a single JSON object whose
// top-level keys are the ones documented in 05-cli.md ("JSON output" section); empty lists are empty
// arrays, never null; unknown dates are null.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/selfupdate"
	"github.com/studoo-app/watchoo/internal/selfupdate/selfupdatetest"
)

// jsonTopLevelKeys are the top-level keys of each command, by command name (without "watchoo"). The
// documentation must list exactly the same ones (TestJSONSchemaDocumented).
var jsonTopLevelKeys = map[string][]string{
	"init":         {"name", "root", "dir", "config_file", "database", "parent_workspace", "github", "source", "warnings"},
	"version":      {"version", "commit", "date", "go", "os", "arch"},
	"repo add":     {"outcome", "repository"},
	"repo import":  {"file", "format", "dry_run", "summary", "entries"},
	"repo list":    {"repositories"},
	"repo remove":  {"action", "repository", "deleted_commits"},
	"sync":         {"started_at", "finished_at", "summary", "repositories", "rate_limit", "source"},
	"log":          {"commits"},
	"contributors": {"contributors"},
	"prs":          {"pull_requests"},
	"releases":     {"releases", "tags"},
	"languages":    {"languages"},
	"status": {"workspace", "github", "source", "rate_limit", "rate_limit_error", "quota_low", "repositories", "errors",
		"last_synced_at", "tracked_branches"},
	"config export": {"format", "version", "exported_at", "watchoo_version", "workspace", "settings", "repositories"},
	"config import": {"file", "dry_run", "applied", "source", "settings", "repositories", "summary", "warnings"},
	"update":        {"current", "latest", "update_available", "release_url"},
}

// pendingJSONCommands are documented in 05-cli.md but not implemented yet (none today): a command
// added to the documentation before its code is listed here while it is being written.
var pendingJSONCommands = map[string]bool{}

// assertSingleJSONObject checks that stdout is exactly one JSON object followed by a line ending.
func assertSingleJSONObject(t *testing.T, what, stdout string) {
	t.Helper()
	if !strings.HasPrefix(stdout, "{") || !strings.HasSuffix(stdout, "}\n") {
		t.Errorf("%s: the output is not a single JSON object:\n%s", what, stdout)
		return
	}
	dec := json.NewDecoder(strings.NewReader(stdout))
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		t.Errorf("%s: invalid JSON: %v\n%s", what, err, stdout)
		return
	}
	if dec.More() {
		t.Errorf("%s: text after the JSON object:\n%s", what, stdout)
	}
}

// All the commands that produce data have --json, with the top-level keys of the contract, and nothing
// else on the standard output.
func TestJSONSchemaAllCommands(t *testing.T) {
	h := newHarness(t)
	r := h.run("init", "--json").mustSucceed(t)
	outputs := map[string]string{"init": r.stdout}
	outputs["version"] = h.run("version", "--json").mustSucceed(t).stdout
	releases := selfupdatetest.New(t, "v0.6.0", runtime.GOOS, runtime.GOARCH)
	h.Updater = selfupdate.New(selfupdate.Options{BaseURL: releases.URL})
	outputs["update"] = h.run("update", "--check", "--json").mustSucceed(t).stdout

	h = newWorkspace(t)
	seedProvider(h.Provider)
	outputs["repo add"] = h.run("repo", "add", "alice/projet", "--json").mustSucceed(t).stdout
	writeFile(t, filepath.Join(h.cwd, "promo.txt"), "bob/projet\ndave/projet\n")
	outputs["repo import"] = h.run("repo", "import", "promo.txt", "--json").mustSucceed(t).stdout
	outputs["sync"] = h.run("sync", "--json").mustSucceed(t).stdout
	outputs["repo list"] = h.run("repo", "list", "--json").mustSucceed(t).stdout
	outputs["log"] = h.run("log", "--json").mustSucceed(t).stdout
	outputs["contributors"] = h.run("contributors", "--json").mustSucceed(t).stdout
	outputs["prs"] = h.run("prs", "--json").mustSucceed(t).stdout
	outputs["releases"] = h.run("releases", "--json").mustSucceed(t).stdout
	outputs["languages"] = h.run("languages", "--json").mustSucceed(t).stdout
	outputs["status"] = h.run("status", "--json").mustSucceed(t).stdout
	outputs["repo remove"] = h.run("repo", "remove", "dave/projet", "--yes", "--json").mustSucceed(t).stdout
	outputs["config export"] = h.run("config", "export", "--json").mustSucceed(t).stdout
	writeFile(t, filepath.Join(h.cwd, "partage.json"), outputs["config export"])
	outputs["config import"] = h.run("config", "import", "partage.json", "--yes", "--json").mustSucceed(t).stdout

	for cmd, keys := range jsonTopLevelKeys {
		out, ok := outputs[cmd]
		if !ok {
			t.Errorf("command %q not tested", cmd)
			continue
		}
		assertSingleJSONObject(t, cmd, out)
		assertJSONKeys(t, out, "", keys...)
	}
	// Items of "prs" and "releases" (V2), and repositories of "sync" (warnings).
	assertJSONKeys(t, outputs["sync"], "repositories.0", "id", "full_name", "display_name", "status", "new_commits",
		"tracked_branches", "stats_fetched", "duration_ms", "error", "resume_at", "warnings")
	assertJSONKeys(t, outputs["prs"], "pull_requests.0", "repository", "number", "title", "state", "draft", "author",
		"url", "base_branch", "head_branch", "created_at", "updated_at", "closed_at", "merged_at")
	assertJSONKeys(t, outputs["prs"], "pull_requests.0.repository", "id", "full_name", "display_name")
	assertJSONKeys(t, outputs["prs"], "pull_requests.0.author", "login")
	assertJSONKeys(t, outputs["releases"], "releases.0", "repository", "id", "tag_name", "name", "draft", "prerelease",
		"author", "url", "created_at", "published_at")
	assertJSONKeys(t, outputs["releases"], "tags.0", "repository", "name", "sha", "short_sha", "commit_url",
		"committed_at")
	assertContains(t, "sync --json", outputs["sync"], `"warnings": []`)
	// Items of "log" (stats_known: 0.5.0).
	assertJSONKeys(t, outputs["log"], "commits.0", "repository", "sha", "short_sha", "message", "url", "author",
		"authored_at", "committed_at", "additions", "deletions", "stats_known", "ai_signatures", "ai_scanned")
	// Items of "contributors": weekly ([] without --weeks, week / count objects with it).
	assertJSONKeys(t, outputs["contributors"], "contributors.0", contributorKeys...)
	weekly := h.run("contributors", "--weeks", "2", "--json").mustSucceed(t).stdout
	assertSingleJSONObject(t, "contributors --weeks", weekly)
	assertJSONKeys(t, weekly, "", jsonTopLevelKeys["contributors"]...)
	assertJSONKeys(t, weekly, "contributors.0", contributorKeys...)
	assertJSONKeys(t, weekly, "contributors.0.weekly.0", "week", "count")
}

// contributorKeys are the keys of an item of "contributors" (05-cli.md, JSON output).
var contributorKeys = []string{"id", "name", "login", "display_name", "avatar_url", "commits", "additions",
	"deletions", "repositories", "share", "last_commit_at", "ai_signed_commits", "ai_scanned_commits", "ai_tools", "weekly"}

// The contract is documented: the "JSON output" table of documentation/en/05-cli.md and the "Sortie
// JSON" table of its French mirror list, for each command, the same top-level keys as jsonTopLevelKeys.
func TestJSONSchemaDocumented(t *testing.T) {
	for _, doc := range []struct{ lang, section string }{
		{"en", "\n## JSON output\n"},
		{"fr", "\n## Sortie JSON\n"},
	} {
		t.Run(doc.lang, func(t *testing.T) {
			checkJSONSchemaDoc(t, doc.lang, doc.section)
		})
	}
}

// checkJSONSchemaDoc compares the table of the JSON section of documentation/<lang>/05-cli.md with
// jsonTopLevelKeys.
func checkJSONSchemaDoc(t *testing.T, lang, heading string) {
	t.Helper()
	name := lang + "/05-cli.md"
	doc, err := os.ReadFile(filepath.Join("..", "..", "documentation", lang, "05-cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(doc), heading)
	if !ok {
		t.Fatalf("section %q missing from documentation/%s", strings.TrimSpace(heading), name)
	}
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	row := regexp.MustCompile("^\\| `watchoo ([a-z ]+)` \\| ([^|]*) \\|")
	key := regexp.MustCompile("`([a-z_]+)`")
	documented := map[string][]string{}
	for _, line := range strings.Split(section, "\n") {
		m := row.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var keys []string
		for _, k := range key.FindAllStringSubmatch(m[2], -1) {
			keys = append(keys, k[1])
		}
		documented[m[1]] = keys
	}
	for cmd, want := range jsonTopLevelKeys {
		got, ok := documented[cmd]
		if !ok {
			t.Errorf("%s: no line for \"watchoo %s\" in the JSON section", name, cmd)
			continue
		}
		got, want = slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))
		if !slices.Equal(got, want) {
			t.Errorf("%s: keys of \"watchoo %s\" = %v, want %v", name, cmd, got, want)
		}
	}
	for cmd := range documented {
		if _, ok := jsonTopLevelKeys[cmd]; !ok && !pendingJSONCommands[cmd] {
			t.Errorf("%s: \"watchoo %s\" documented but missing from the tested contract", name, cmd)
		}
	}
}

// Sub-objects of "config import" (preview and summary), with a changed setting, a repository to update
// and a warning.
func TestJSONSchemaConfigImport(t *testing.T) {
	h := newSyncedWorkspace(t)
	writeFile(t, filepath.Join(h.cwd, "partage.json"), `{"format":"watchoo-config","version":1,
"exported_at":"2026-10-20T08:00:00Z","settings":{"sync":{"interval":"1h"}},
"repositories":[{"repo":"bob/projet","label":"Bob D."}]}`)
	for _, args := range [][]string{{"--dry-run"}, {"--yes"}} {
		r := h.run(append([]string{"config", "import", "partage.json", "--json"}, args...)...).mustSucceed(t)
		assertSingleJSONObject(t, "config import", r.stdout)
		assertJSONKeys(t, r.stdout, "", jsonTopLevelKeys["config import"]...)
		assertJSONKeys(t, r.stdout, "source", "workspace_name", "watchoo_version", "exported_at")
		assertJSONKeys(t, r.stdout, "settings", "changes", "rewrite", "written", "backup_file")
		assertJSONKeys(t, r.stdout, "settings.changes.0", "key", "before", "after", "env_var")
		assertJSONKeys(t, r.stdout, "repositories.0", "index", "repo", "action", "reenable", "before", "after",
			"outcome", "error")
		assertJSONKeys(t, r.stdout, "repositories.0.before", "label", "branches", "flemme_after_days",
			"dead_after_days", "enabled")
		assertJSONKeys(t, r.stdout, "repositories.0.after", "label", "branches", "flemme_after_days",
			"dead_after_days", "enabled")
		assertJSONKeys(t, r.stdout, "summary", "added", "already_tracked", "updated", "reenabled", "inaccessible",
			"errors")
		assertJSONKeys(t, r.stdout, "warnings.0", "code", "key", "message")
		assertContains(t, "config import "+args[0], r.stdout, `"workspace_name": ""`, `"branches": []`,
			`"code": "comments_lost"`)
	}
	// An addition: before is null.
	writeFile(t, filepath.Join(h.cwd, "ajout.json"), `{"format":"watchoo-config","version":1,
"repositories":[{"repo":"carol/projet"}]}`)
	r := h.run("config", "import", "ajout.json", "--json", "--dry-run").mustCode(t, 1)
	assertContains(t, "addition", r.stdout, `"before": null`, `"exported_at": null`, `"warnings": []`, `"changes": []`)
}

// Sub-objects of init and version.
func TestJSONSchemaInitVersion(t *testing.T) {
	h := newHarness(t)
	r := h.run("init", "--json").mustSucceed(t)
	assertJSONKeys(t, r.stdout, "github", "host", "login", "token_source", "scopes")
	assertContains(t, "init --json", r.stdout, `"warnings": []`)

	r = h.run("version", "--json").mustSucceed(t)
	// Test binary: neither commit nor date known.
	assertContains(t, "version --json", r.stdout, `"commit": ""`, `"date": null`)
}

// Empty workspace: the lists are empty arrays, never null.
func TestJSONSchemaEmptyWorkspace(t *testing.T) {
	h := newWorkspace(t)
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"repo", "list"}, `"repositories": []`},
		{[]string{"log"}, `"commits": []`},
		{[]string{"contributors"}, `"contributors": []`},
		{[]string{"prs"}, `"pull_requests": []`},
		{[]string{"releases"}, `"tags": []`},
		{[]string{"languages"}, `"languages": []`},
		{[]string{"sync"}, `"repositories": []`},
		{[]string{"status"}, `"errors": []`},
	} {
		r := h.run(append(tt.args, "--json")...).mustSucceed(t)
		assertSingleJSONObject(t, strings.Join(tt.args, " "), r.stdout)
		assertContains(t, strings.Join(tt.args, " ")+" --json", r.stdout, tt.want)
	}
	r := h.run("status", "--json").mustSucceed(t)
	assertContains(t, "empty status --json", r.stdout, `"last_synced_at": null`, `"scopes": [`)
}

// The three actions of repo remove have the same schema, with the complete repository.
func TestJSONSchemaRepoRemove(t *testing.T) {
	h := newSyncedWorkspace(t)
	for _, tt := range []struct {
		args   []string
		action string
	}{
		{[]string{"repo", "remove", "bob/projet", "--keep-history", "--yes"}, "stopped"},
		{[]string{"repo", "remove", "bob/projet", "--keep-history", "--yes"}, "already_stopped"},
		{[]string{"repo", "remove", "bob/projet", "--yes"}, "deleted"},
	} {
		r := h.run(append(tt.args, "--json")...).mustSucceed(t)
		assertJSONKeys(t, r.stdout, "", jsonTopLevelKeys["repo remove"]...)
		assertJSONKeys(t, r.stdout, "repository", repositoryKeys...)
		var rep repoRemoveReport
		decodeStrict(t, r.stdout, &rep)
		if rep.Action != tt.action {
			t.Errorf("%v: action = %q, want %q", tt.args, rep.Action, tt.action)
		}
	}
}

// The items of status.errors and of an inaccessible import.
func TestJSONSchemaErrorItems(t *testing.T) {
	h := newSyncedWorkspace(t)
	st := openTestStore(t, h)
	bob := storedRepo(t, h, "bob", "projet")
	run, err := st.BeginSyncRun(context.Background(), bob.ID, testNow, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishSyncRun(context.Background(), run.ID, testNow, model.SyncError, 0, "access lost"); err != nil {
		t.Fatal(err)
	}
	r := h.run("status", "--json").mustSucceed(t)
	assertJSONKeys(t, r.stdout, "errors.0", "repo", "message")
	assertJSONKeys(t, r.stdout, "workspace", "name", "root")

	writeFile(t, filepath.Join(h.cwd, "carol.txt"), "carol/projet\n")
	r = h.run("repo", "import", "carol.txt", "--json").mustCode(t, 1)
	assertJSONKeys(t, r.stdout, "entries.0", "line", "repo", "outcome", "repository", "error")
	assertContains(t, "inaccessible import", r.stdout, `"repository": null`, `"outcome": "inaccessible"`)
}

// fakeBuildInfo replaces Go's build information for the duration of the test.
func fakeBuildInfo(t *testing.T, info *debug.BuildInfo) {
	t.Helper()
	original := readBuildInfo
	t.Cleanup(func() { readBuildInfo = original })
	readBuildInfo = func() (*debug.BuildInfo, bool) { return info, info != nil }
}

// setBuildVars sets Version, Commit and Date (-ldflags values) for the duration of the test.
func setBuildVars(t *testing.T, version, commit, date string) {
	t.Helper()
	v, c, d := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = v, c, d })
	Version, Commit, Date = version, commit, date
}

func TestVersionBuildMetadata(t *testing.T) {
	const sha = "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d"
	platform := runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH
	vcs := &debug.BuildInfo{
		Main: debug.Module{Path: "github.com/studoo-app/watchoo", Version: "v0.1.1"},
		Settings: []debug.BuildSetting{
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: "ffffffffffffffffffffffffffffffffffffffff"},
			{Key: "vcs.time", Value: "2026-10-20T08:00:00Z"},
		},
	}
	devel := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: vcs.Settings}
	dirty := &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20261020080000-ffffffffffff+dirty"},
		Settings: vcs.Settings}
	pseudo := &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20261020080000-ffffffffffff"},
		Settings: vcs.Settings}
	for _, tt := range []struct {
		name                  string
		version, commit, date string
		info                  *debug.BuildInfo
		wantVersion           string
		wantCommit            string
		wantDate              string // empty = null
		wantText              string
	}{
		{"complete -ldflags", "v0.1.0", sha, "2026-10-27T10:00:00Z", nil,
			"v0.1.0", sha, "2026-10-27T10:00:00Z", "watchoo v0.1.0 (commit 1a2b3c4, 2026-10-27, " + platform + ")\n"},
		{"-ldflags take precedence", "v0.1.0", sha, "2026-10-27T10:00:00Z", vcs,
			"v0.1.0", sha, "2026-10-27T10:00:00Z", "watchoo v0.1.0 (commit 1a2b3c4, 2026-10-27, " + platform + ")\n"},
		{"go install", "dev", "", "", vcs,
			"v0.1.1", "ffffffffffffffffffffffffffffffffffffffff", "2026-10-20T08:00:00Z",
			"watchoo v0.1.1 (commit fffffff, 2026-10-20, " + platform + ")\n"},
		{"go build local", "dev", "", "", devel,
			"dev", "ffffffffffffffffffffffffffffffffffffffff", "2026-10-20T08:00:00Z",
			"watchoo dev (commit fffffff, 2026-10-20, " + platform + ")\n"},
		{"modified go build", "dev", "", "", dirty,
			"dev", "ffffffffffffffffffffffffffffffffffffffff", "2026-10-20T08:00:00Z",
			"watchoo dev (commit fffffff, 2026-10-20, " + platform + ")\n"},
		{"go build off a tag", "dev", "", "", pseudo,
			"v0.0.0-20261020080000-ffffffffffff", "ffffffffffffffffffffffffffffffffffffffff", "2026-10-20T08:00:00Z",
			"watchoo v0.0.0-20261020080000-ffffffffffff (commit fffffff, 2026-10-20, " + platform + ")\n"},
		{"unreadable date", "v0.1.0", "abc", "hier", nil,
			"v0.1.0", "abc", "", "watchoo v0.1.0 (commit abc, " + platform + ")\n"},
		{"non-UTC date", "v0.1.0", "", "2026-10-27T23:30:00+02:00", nil,
			"v0.1.0", "", "2026-10-27T21:30:00Z", "watchoo v0.1.0 (2026-10-27, " + platform + ")\n"},
		{"nothing known", "dev", "", "", nil,
			"dev", "", "", "watchoo dev (" + platform + ")\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setBuildVars(t, tt.version, tt.commit, tt.date)
			fakeBuildInfo(t, tt.info)
			h := newHarness(t)
			if r := h.run("version").mustSucceed(t); r.stdout != tt.wantText {
				t.Errorf("version = %q, want %q", r.stdout, tt.wantText)
			}
			r := h.run("version", "--json").mustSucceed(t)
			assertJSONKeys(t, r.stdout, "", jsonTopLevelKeys["version"]...)
			var got versionReport
			decodeStrict(t, r.stdout, &got)
			date := ""
			if got.Date != nil {
				date = got.Date.Format(time.RFC3339)
			}
			if got.Version != tt.wantVersion || got.Commit != tt.wantCommit || date != tt.wantDate ||
				got.Go != runtime.Version() || got.OS != runtime.GOOS || got.Arch != runtime.GOARCH {
				t.Errorf("version --json = %+v (date %q)", got, date)
			}
			if appVersion() != tt.wantVersion {
				t.Errorf("appVersion() = %q, want %q", appVersion(), tt.wantVersion)
			}
			if r2 := h.run("--version", "--json").mustSucceed(t); r2.stdout != r.stdout {
				t.Errorf("--version --json = %s, want %s", r2.stdout, r.stdout)
			}
		})
	}
}

// The header card shows the same version as "watchoo version" (here the go install one).
func TestVersionCardUsesBuildInfo(t *testing.T) {
	setBuildVars(t, "dev", "", "")
	fakeBuildInfo(t, &debug.BuildInfo{Main: debug.Module{Version: "v0.1.1"}})
	h := newHarness(t)
	h.Terminal = func() TermInfo { return wideTerminal }
	r := h.run("version").mustSucceed(t)
	assertContains(t, "version card", ansiSeq.ReplaceAllString(r.stdout, ""), "v0.1.1")
}
