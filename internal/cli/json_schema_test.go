package cli

// Contrat des sorties --json : chaque commande qui produit des données écrit un seul objet JSON dont
// les clés de premier niveau sont celles documentées dans 05-cli.md (section « Sortie JSON ») ; les
// listes vides sont des tableaux vides, jamais null ; les dates inconnues sont null.

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
)

// jsonTopLevelKeys sont les clés de premier niveau de chaque commande, par nom de commande (sans
// « watchoo »). La documentation doit lister exactement les mêmes (TestJSONSchemaDocumented).
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
	"status": {"workspace", "github", "source", "rate_limit", "rate_limit_error", "quota_low", "repositories", "errors",
		"last_synced_at", "tracked_branches"},
	"config export": {"format", "version", "exported_at", "watchoo_version", "workspace", "settings", "repositories"},
	"config import": {"file", "dry_run", "applied", "source", "settings", "repositories", "summary", "warnings"},
}

// pendingJSONCommands sont documentées dans 05-cli.md mais pas encore implémentées (aucune
// aujourd'hui) : une commande ajoutée à la documentation avant son code y figure le temps de l'écrire.
var pendingJSONCommands = map[string]bool{}

// assertSingleJSONObject vérifie que stdout est exactement un objet JSON suivi d'une fin de ligne.
func assertSingleJSONObject(t *testing.T, what, stdout string) {
	t.Helper()
	if !strings.HasPrefix(stdout, "{") || !strings.HasSuffix(stdout, "}\n") {
		t.Errorf("%s : la sortie n'est pas un objet JSON seul :\n%s", what, stdout)
		return
	}
	dec := json.NewDecoder(strings.NewReader(stdout))
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		t.Errorf("%s : JSON invalide : %v\n%s", what, err, stdout)
		return
	}
	if dec.More() {
		t.Errorf("%s : texte après l'objet JSON :\n%s", what, stdout)
	}
}

// Toutes les commandes qui produisent des données ont --json, avec les clés de premier niveau du
// contrat, et rien d'autre sur la sortie standard.
func TestJSONSchemaAllCommands(t *testing.T) {
	h := newHarness(t)
	r := h.run("init", "--json").mustSucceed(t)
	outputs := map[string]string{"init": r.stdout}
	outputs["version"] = h.run("version", "--json").mustSucceed(t).stdout

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
	outputs["status"] = h.run("status", "--json").mustSucceed(t).stdout
	outputs["repo remove"] = h.run("repo", "remove", "dave/projet", "--yes", "--json").mustSucceed(t).stdout
	outputs["config export"] = h.run("config", "export", "--json").mustSucceed(t).stdout
	writeFile(t, filepath.Join(h.cwd, "partage.json"), outputs["config export"])
	outputs["config import"] = h.run("config", "import", "partage.json", "--yes", "--json").mustSucceed(t).stdout

	for cmd, keys := range jsonTopLevelKeys {
		out, ok := outputs[cmd]
		if !ok {
			t.Errorf("commande %q non testée", cmd)
			continue
		}
		assertSingleJSONObject(t, cmd, out)
		assertJSONKeys(t, out, "", keys...)
	}
	// Éléments de « prs » et « releases » (V2), et dépôts de « sync » (warnings).
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
	// Éléments de « contributors » : weekly ([] sans --weeks, objets week / count avec).
	assertJSONKeys(t, outputs["contributors"], "contributors.0", contributorKeys...)
	weekly := h.run("contributors", "--weeks", "2", "--json").mustSucceed(t).stdout
	assertSingleJSONObject(t, "contributors --weeks", weekly)
	assertJSONKeys(t, weekly, "", jsonTopLevelKeys["contributors"]...)
	assertJSONKeys(t, weekly, "contributors.0", contributorKeys...)
	assertJSONKeys(t, weekly, "contributors.0.weekly.0", "week", "count")
}

// contributorKeys sont les clés d'un élément de « contributors » (05-cli.md, Sortie JSON).
var contributorKeys = []string{"id", "name", "login", "display_name", "avatar_url", "commits", "additions",
	"deletions", "repositories", "last_commit_at", "weekly"}

// Le contrat est documenté : la table « Sortie JSON » de 05-cli.md liste, pour chaque commande, les
// mêmes clés de premier niveau que jsonTopLevelKeys.
func TestJSONSchemaDocumented(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "documentation", "05-cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(doc), "\n## Sortie JSON\n")
	if !ok {
		t.Fatal("section « ## Sortie JSON » absente de 05-cli.md")
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
			t.Errorf("05-cli.md : pas de ligne pour « watchoo %s » dans la section Sortie JSON", cmd)
			continue
		}
		got, want = slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))
		if !slices.Equal(got, want) {
			t.Errorf("05-cli.md : clés de « watchoo %s » = %v, attendu %v", cmd, got, want)
		}
	}
	for cmd := range documented {
		if _, ok := jsonTopLevelKeys[cmd]; !ok && !pendingJSONCommands[cmd] {
			t.Errorf("05-cli.md : « watchoo %s » documentée mais absente du contrat testé", cmd)
		}
	}
}

// Sous-objets de « config import » (aperçu et bilan), avec un réglage modifié, un dépôt à mettre à
// jour et un avertissement.
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
	// Un ajout : before vaut null.
	writeFile(t, filepath.Join(h.cwd, "ajout.json"), `{"format":"watchoo-config","version":1,
"repositories":[{"repo":"carol/projet"}]}`)
	r := h.run("config", "import", "ajout.json", "--json", "--dry-run").mustCode(t, 1)
	assertContains(t, "ajout", r.stdout, `"before": null`, `"exported_at": null`, `"warnings": []`, `"changes": []`)
}

// Sous-objets de init et version.
func TestJSONSchemaInitVersion(t *testing.T) {
	h := newHarness(t)
	r := h.run("init", "--json").mustSucceed(t)
	assertJSONKeys(t, r.stdout, "github", "host", "login", "token_source", "scopes")
	assertContains(t, "init --json", r.stdout, `"warnings": []`)

	r = h.run("version", "--json").mustSucceed(t)
	// Binaire de test : ni commit ni date connus.
	assertContains(t, "version --json", r.stdout, `"commit": ""`, `"date": null`)
}

// Espace vide : les listes sont des tableaux vides, jamais null.
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
		{[]string{"sync"}, `"repositories": []`},
		{[]string{"status"}, `"errors": []`},
	} {
		r := h.run(append(tt.args, "--json")...).mustSucceed(t)
		assertSingleJSONObject(t, strings.Join(tt.args, " "), r.stdout)
		assertContains(t, strings.Join(tt.args, " ")+" --json", r.stdout, tt.want)
	}
	r := h.run("status", "--json").mustSucceed(t)
	assertContains(t, "status --json vide", r.stdout, `"last_synced_at": null`, `"scopes": [`)
}

// Les trois actions de repo remove ont le même schéma, avec le dépôt complet.
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
			t.Errorf("%v : action = %q, attendu %q", tt.args, rep.Action, tt.action)
		}
	}
}

// Les éléments de status.errors et d'un import inaccessible.
func TestJSONSchemaErrorItems(t *testing.T) {
	h := newSyncedWorkspace(t)
	st := openTestStore(t, h)
	bob := storedRepo(t, h, "bob", "projet")
	run, err := st.BeginSyncRun(context.Background(), bob.ID, testNow, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishSyncRun(context.Background(), run.ID, testNow, model.SyncError, 0, "accès perdu"); err != nil {
		t.Fatal(err)
	}
	r := h.run("status", "--json").mustSucceed(t)
	assertJSONKeys(t, r.stdout, "errors.0", "repo", "message")
	assertJSONKeys(t, r.stdout, "workspace", "name", "root")

	writeFile(t, filepath.Join(h.cwd, "carol.txt"), "carol/projet\n")
	r = h.run("repo", "import", "carol.txt", "--json").mustCode(t, 1)
	assertJSONKeys(t, r.stdout, "entries.0", "line", "repo", "outcome", "repository", "error")
	assertContains(t, "import inaccessible", r.stdout, `"repository": null`, `"outcome": "inaccessible"`)
}

// fakeBuildInfo remplace les informations de compilation de Go le temps du test.
func fakeBuildInfo(t *testing.T, info *debug.BuildInfo) {
	t.Helper()
	original := readBuildInfo
	t.Cleanup(func() { readBuildInfo = original })
	readBuildInfo = func() (*debug.BuildInfo, bool) { return info, info != nil }
}

// setBuildVars fixe Version, Commit et Date (valeurs de -ldflags) le temps du test.
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
		wantDate              string // vide = null
		wantText              string
	}{
		{"-ldflags complet", "v0.1.0", sha, "2026-10-27T10:00:00Z", nil,
			"v0.1.0", sha, "2026-10-27T10:00:00Z", "watchoo v0.1.0 (commit 1a2b3c4, 2026-10-27, " + platform + ")\n"},
		{"-ldflags prioritaire", "v0.1.0", sha, "2026-10-27T10:00:00Z", vcs,
			"v0.1.0", sha, "2026-10-27T10:00:00Z", "watchoo v0.1.0 (commit 1a2b3c4, 2026-10-27, " + platform + ")\n"},
		{"go install", "dev", "", "", vcs,
			"v0.1.1", "ffffffffffffffffffffffffffffffffffffffff", "2026-10-20T08:00:00Z",
			"watchoo v0.1.1 (commit fffffff, 2026-10-20, " + platform + ")\n"},
		{"go build local", "dev", "", "", devel,
			"dev", "ffffffffffffffffffffffffffffffffffffffff", "2026-10-20T08:00:00Z",
			"watchoo dev (commit fffffff, 2026-10-20, " + platform + ")\n"},
		{"go build modifié", "dev", "", "", dirty,
			"dev", "ffffffffffffffffffffffffffffffffffffffff", "2026-10-20T08:00:00Z",
			"watchoo dev (commit fffffff, 2026-10-20, " + platform + ")\n"},
		{"go build hors tag", "dev", "", "", pseudo,
			"v0.0.0-20261020080000-ffffffffffff", "ffffffffffffffffffffffffffffffffffffffff", "2026-10-20T08:00:00Z",
			"watchoo v0.0.0-20261020080000-ffffffffffff (commit fffffff, 2026-10-20, " + platform + ")\n"},
		{"date illisible", "v0.1.0", "abc", "hier", nil,
			"v0.1.0", "abc", "", "watchoo v0.1.0 (commit abc, " + platform + ")\n"},
		{"date hors UTC", "v0.1.0", "", "2026-10-27T23:30:00+02:00", nil,
			"v0.1.0", "", "2026-10-27T21:30:00Z", "watchoo v0.1.0 (2026-10-27, " + platform + ")\n"},
		{"rien de connu", "dev", "", "", nil,
			"dev", "", "", "watchoo dev (" + platform + ")\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setBuildVars(t, tt.version, tt.commit, tt.date)
			fakeBuildInfo(t, tt.info)
			h := newHarness(t)
			if r := h.run("version").mustSucceed(t); r.stdout != tt.wantText {
				t.Errorf("version = %q, attendu %q", r.stdout, tt.wantText)
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
				t.Errorf("appVersion() = %q, attendu %q", appVersion(), tt.wantVersion)
			}
			if r2 := h.run("--version", "--json").mustSucceed(t); r2.stdout != r.stdout {
				t.Errorf("--version --json = %s, attendu %s", r2.stdout, r.stdout)
			}
		})
	}
}

// La carte d'en-tête montre la même version que « watchoo version » (ici celle de go install).
func TestVersionCardUsesBuildInfo(t *testing.T) {
	setBuildVars(t, "dev", "", "")
	fakeBuildInfo(t, &debug.BuildInfo{Main: debug.Module{Version: "v0.1.1"}})
	h := newHarness(t)
	h.Terminal = func() TermInfo { return wideTerminal }
	r := h.run("version").mustSucceed(t)
	assertContains(t, "carte de version", ansiSeq.ReplaceAllString(r.stdout, ""), "v0.1.1")
}
