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
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
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
	"config export":       {"format", "version", "exported_at", "watchoo_version", "workspace", "settings", "repositories"},
	"config import":       {"file", "dry_run", "applied", "source", "settings", "repositories", "summary", "warnings"},
	"update":              {"current", "latest", "update_available", "release_url"},
	"diagnostic":          {"enabled", "leads", "contexts", "histories", "notice", "filters"},
	"diagnostic backfill": {"repositories", "measured"},
	"conventions":         {"enabled", "rules_version", "repositories", "contributors", "commits", "notice", "filters", "milestone"},
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
	outputs["diagnostic"] = h.run("diagnostic", "--json").mustSucceed(t).stdout
	// Conventions enabled (phase 8.1): same top-level keys with --commits, here tested on items.
	t.Setenv("WATCHOO_CONVENTIONS_ENABLED", "true")
	// A sync with the conventions enabled observes the tags (phase 8.4, tag_history).
	h.run("sync").mustSucceed(t)
	// Milestone v0.1.0, present as v0.1 without a release: a SV6 note.
	t.Setenv("WATCHOO_CONVENTIONS_TAGS_MILESTONES", "v0.1.0,v1.0.0")
	outputs["conventions"] = h.run("conventions", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, outputs["conventions"], "repositories.0", conventionsRepoKeys...)
	assertJSONKeys(t, outputs["conventions"], "repositories.0.commits", conventionsTallyKeys...)
	assertJSONKeys(t, outputs["conventions"], "repositories.0.commits.issues.0", "code", "severity", "count", "shas")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.commits.excluded", "merge", "bot", "initial_commit", "excluded_account")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.default_branch", "name", "evaluated", "conform", "rate")
	assertJSONKeys(t, outputs["conventions"], "contributors.0", "author", "commits", "traits", "trend", "pull_requests", "branches",
		"arrivals")
	assertJSONKeys(t, outputs["conventions"], "contributors.0.commits", conventionsTallyKeys...)
	assertJSONKeys(t, outputs["conventions"], "filters", "repository", "contributor", "commits", "failing", "milestone")
	// Phase 8.2: branches, pull requests, tags, milestones, releases, traits, language, intervals, trend.
	assertJSONKeys(t, outputs["conventions"], "repositories.0.traits", "evaluated", "wording", "footer", "issue_reference")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.traits.wording", "past_tense", "progressive", "first_person")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.language", "expected", "fr", "en", "unknown", "other", "mixed")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.branches", "source", "checked", "conform", "review", "items",
		"reused")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.branches.items.0", "name", "codes", "conform", "review",
		"in_source", "pull_requests", "authors")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.pull_requests", "known", "reason", "open", "merged", "skipped", "items",
		"methods", "merged_without_review")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.pull_requests.open", "evaluated", "conform", "default",
		"body_known", "body_conform", "merge_known", "squash", "merge_conform", "review_known", "reviewed", "reviews_received")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.pull_requests.skipped", "closed", "draft", "bot", "fork", "before_since")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.pull_requests.methods", "squash", "merge", "rebase", "unknown")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.pull_requests.items.0", "number", "category", "author",
		"head_branch", "codes", "default_codes", "conform", "body", "merge", "review")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.pull_requests.items.0.review", "known", "reviews", "approvals",
		"reviewed")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.pull_requests.items.0.body", "known", "found", "missing",
		"closes", "conform")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.tags", "checked", "conform", "items", "milestones", "releases")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.tags.items.0", "name", "codes", "conform", "final")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.tags.milestones.0", "name", "due", "status", "tag", "others",
		"deadline")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.tags.releases", "known", "reason", "notes")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.tags.releases.notes.0", "code", "tag")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.intervals", "known", "reason", "last_tag", "items", "since_last_tag")
	// Phase 8.3: file criteria (cache of the sync; none here: every criterion unknown, "not computed").
	for _, fam := range []string{"changelog", "readme", "tooling"} {
		assertJSONKeys(t, outputs["conventions"], "repositories.0."+fam, conventionsFilesKeys...)
		assertJSONKeys(t, outputs["conventions"], "repositories.0."+fam+".criteria.0", "id", "status", "reason", "codes", "counts")
	}
	assertJSONKeys(t, outputs["conventions"], "repositories.0.docs", append(conventionsFilesKeys, "lists")...)
	assertJSONKeys(t, outputs["conventions"], "repositories.0.docs.lists", "computed", "reason", "unreferenced",
		"duplicate_rows", "dead_links", "bad_names", "archives", "office_documents")
	// Phase 8.4: history of the tags and of the default branch, versions, freshness of the CHANGELOG.
	assertJSONKeys(t, outputs["conventions"], "repositories.0.rewrites", "known", "reason", "count", "last", "events")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.tag_history", "known", "reason", "since", "counts", "items")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.tag_history.counts", "stable", "restored", "rewritten", "deleted")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.tag_history.items.0", "name", "status", "sha",
		"first_seen_at", "rewrites", "deletions")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.prereleases", "known", "reason", "checked", "conform", "notes")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.bumps", "known", "reason", "counts", "items")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.bumps.counts", "consistent", "under", "undetermined", "unknown")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.precedence", "known", "reason", "pairs", "unknown",
		"conform", "notes")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.changelog_freshness", "known", "reason", "since", "feat",
		"fix", "missing", "grayed")
	// Phase 8.5: commits outside a pull request, reused branches, allowed merge methods.
	assertJSONKeys(t, outputs["conventions"], "repositories.0.arrivals", "known", "reason", "mode", "complete", "by_pr",
		"outside_pr", "undetermined", "reasons", "outside")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.arrivals.reasons", "merge_commit_sha", "rebase_run",
		"github_merge", "squash_suffix", "human_committer", "local_merge", "web_edit", "not_in_db", "committer_unknown",
		"unmatched", "clone_only")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.branches.reused", "known", "reason", "items")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.merge_methods", "known", "reason", "squash", "merge",
		"rebase", "conform")
	// Phase 8.7: leads, outside every grid and every rate.
	assertJSONKeys(t, outputs["conventions"], "repositories.0.leads", "to_review", "items", "broken_images")
	assertJSONKeys(t, outputs["conventions"], "repositories.0.leads.items.0", "id", "status", "reason", "codes", "counts",
		"dates")
	assertJSONKeys(t, outputs["conventions"], "contributors.0.arrivals", "by_pr", "outside_pr", "undetermined")
	assertJSONKeys(t, outputs["conventions"], "contributors.0.traits", "evaluated", "wording", "footer", "issue_reference")
	assertJSONKeys(t, outputs["conventions"], "contributors.0.trend", "known", "repositories", "unmeasured", "before", "since")
	assertJSONKeys(t, outputs["conventions"], "contributors.0.trend.before", conventionsTallyKeys...)
	assertJSONKeys(t, outputs["conventions"], "contributors.0.trend.since", conventionsTallyKeys...)
	assertJSONKeys(t, outputs["conventions"], "contributors.0.pull_requests", "open", "merged")
	for _, state := range []string{"open", "merged"} {
		assertJSONKeys(t, outputs["conventions"], "contributors.0.pull_requests."+state, "evaluated", "conform", "default",
			"body_known", "body_conform", "merge_known", "squash", "merge_conform", "review_known", "reviewed",
			"reviews_received")
	}
	assertJSONKeys(t, outputs["conventions"], "contributors.0.branches", "checked", "conform", "review")
	t.Setenv("WATCHOO_CONVENTIONS_SINCE", "2020-01-01")
	review := h.run("conventions", "--milestone", "v1.0.0", "--json").mustSucceed(t).stdout
	assertSingleJSONObject(t, "conventions --milestone", review)
	assertJSONKeys(t, review, "", jsonTopLevelKeys["conventions"]...)
	assertJSONKeys(t, review, "milestone", "name", "due", "criteria", "repositories")
	assertJSONKeys(t, review, "milestone.repositories.0", "repository", "evaluable", "reason", "since", "status", "tag",
		"tag_codes", "interval", "branches", "files", "deadline", "tag_history", "arrivals", "reviews", "criteria", "conform",
		"unknown", "total")
	assertJSONKeys(t, review, "milestone.repositories.0.criteria.0", "id", "status", "reason")
	assertJSONKeys(t, review, "milestone.repositories.0.files", "state", "reason", "sha", "computed_at",
		"readme_expected", "readme_found")
	t.Setenv("WATCHOO_CONVENTIONS_SINCE", "")
	commits := h.run("conventions", "--commits", "--json").mustSucceed(t).stdout
	assertSingleJSONObject(t, "conventions --commits", commits)
	assertJSONKeys(t, commits, "", jsonTopLevelKeys["conventions"]...)
	assertJSONKeys(t, commits, "commits.0", "repository", "sha", "short_sha", "author", "date", "conform", "codes")
	t.Setenv("WATCHOO_CONVENTIONS_ENABLED", "")
	// Diagnosis enabled, on local clones of the tracked repositories (phase 7.3).
	clones := gittest.NewServer(t)
	for _, owner := range []string{"alice", "bob", "dave"} {
		clones.Create(owner, "projet", "main").Commit("main", "a.txt", "a\n", "init", gittest.Author{Name: "X", Email: "x@example.test"}, testNow)
	}
	h.GitURL = clones.URL
	t.Setenv("WATCHOO_DIAGNOSTIC_ENABLED", "true")
	outputs["diagnostic backfill"] = h.run("diagnostic", "backfill", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, outputs["diagnostic backfill"], "repositories.0", "repository", "measured", "merges", "missing", "error")
	t.Setenv("WATCHOO_DIAGNOSTIC_ENABLED", "")
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

// conventionsRepoKeys and conventionsTallyKeys are the keys of a repository of "conventions" and of
// its rates (05-cli.md, JSON output).
var (
	conventionsRepoKeys = []string{"repository", "source", "since", "since_origin", "commits", "before_since", "parents_unknown",
		"default_branch", "traits", "language", "branches", "pull_requests", "tags", "intervals",
		"changelog", "readme", "docs", "tooling",
		"rewrites", "tag_history", "prereleases", "bumps", "precedence", "changelog_freshness",
		"arrivals", "merge_methods", "leads"}
	conventionsFilesKeys = []string{"state", "reason", "sha", "computed_at", "conform", "unknown", "total", "criteria"}
	conventionsTallyKeys = []string{"evaluated", "conform", "rate", "excluded", "issues"}
)

// contributorKeys are the keys of an item of "contributors" (05-cli.md, JSON output).
var contributorKeys = []string{"id", "name", "login", "label", "display_name", "avatar_url", "commits", "additions",
	"deletions", "repositories", "share", "last_commit_at", "ai_signed_commits", "ai_scanned_commits", "ai_tools",
	"languages_scanned_commits", "languages", "weekly"}

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
			"dead_after_days", "conventions_since", "enabled")
		assertJSONKeys(t, r.stdout, "repositories.0.after", "label", "branches", "flemme_after_days",
			"dead_after_days", "conventions_since", "enabled")
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
		{[]string{"conventions"}, `"repositories": []`},
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
