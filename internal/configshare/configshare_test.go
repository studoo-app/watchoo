package configshare_test

import (
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
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/configshare"
	"github.com/studoo-app/watchoo/internal/provider"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// doc builds a v1 document with the given JSON parts (settings and repositories may be empty: key
// absent).
func doc(settings, repositories string) string {
	parts := []string{`"format": "watchoo-config"`, `"version": 1`}
	if settings != "" {
		parts = append(parts, `"settings": `+settings)
	}
	if repositories != "" {
		parts = append(parts, `"repositories": `+repositories)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func mustParse(t *testing.T, s string) *configshare.File {
	t.Helper()
	f, err := configshare.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, s)
	}
	return f
}

func TestExport(t *testing.T) {
	isolate(t)
	t.Setenv("WATCHOO_TIMEZONE", "UTC") // the EFFECTIVE value is exported, variable included
	e := newEnv(t, "licence-projet")
	e.ws.Config.Timezone = "UTC"
	e.track(1, "zoe", "projet", "Zoé Leroy")
	e.track(2, "Alice", "projet", "Alice Martin", "main", "release/*")
	stopped := e.track(3, "carol", "projet", "Carol")
	e.stop(stopped.ID)

	d, err := configshare.Export(t.Context(), e.ws, e.st, configshare.ExportOptions{Now: testNow.Add(500 * time.Millisecond), WatchooVersion: "v0.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "format": "watchoo-config",
  "version": 1,
  "exported_at": "2026-10-27T10:00:00Z",
  "watchoo_version": "v0.1.0",
  "workspace": {
    "name": "licence-projet"
  },
  "settings": {
    "sync": {
      "auto": true,
      "interval": "30m",
      "parallelism": 4
    },
    "activity": {
      "flemme_after_days": 5,
      "dead_after_days": 15
    },
    "branches": {
      "exclude": []
    },
    "timezone": "UTC",
    "lang": "en"
  },
  "repositories": [
    {
      "provider": "github",
      "repo": "Alice/projet",
      "label": "Alice Martin",
      "branches": [
        "main",
        "release/*"
      ]
    },
    {
      "provider": "github",
      "repo": "zoe/projet",
      "label": "Zoé Leroy",
      "branches": []
    }
  ]
}
`
	if string(data) != want {
		t.Errorf("export:\n%s\nwant:\n%s", data, want)
	}
	for _, forbidden := range []string{"github.", "gh_path", "web", "addr", authtest.FakeToken, "@", e.ws.Root, "carol"} {
		if forbidden == "github." || forbidden == "web" {
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			s := m["settings"].(map[string]any)
			if _, ok := s[strings.TrimSuffix(forbidden, ".")]; ok {
				t.Errorf("key %s exported", forbidden)
			}
			continue
		}
		if strings.Contains(string(data), forbidden) {
			t.Errorf("the export contains %q", forbidden)
		}
	}
	// The exported document is accepted as is.
	if _, err := configshare.ParseBytes(data); err != nil {
		t.Errorf("export read back: %v", err)
	}
}

func TestExportEmptyWorkspace(t *testing.T) {
	isolate(t)
	e := newEnv(t, "vide")
	d, err := configshare.Export(t.Context(), e.ws, e.st, configshare.ExportOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := d.Marshal()
	if !bytes.Contains(data, []byte(`"repositories": []`)) {
		t.Errorf("empty list expected:\n%s", data)
	}
}

// Round trip: export of a workspace, import into another one (accessible repositories), same
// configuration.
func TestRoundTrip(t *testing.T) {
	isolate(t)
	a := newEnv(t, "licence-a")
	a.ws.Config.Sync.Interval = time.Hour
	a.ws.Config.Activity.DeadAfterDays = 21
	a.ws.Config.Branches.Exclude = []string{"dependabot/*"}
	a.ws.Config.Timezone = "Europe/Paris"
	a.ws.Config.Lang = "fr"
	a.track(1, "alice", "projet", "Alice Martin", "main", "dev")
	a.track(2, "bob", "projet", "")
	d, err := configshare.Export(t.Context(), a.ws, a.st, configshare.ExportOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := d.Marshal()

	b := newEnv(t, "licence-b")
	b.forge(1, "alice", "projet")
	b.forge(2, "bob", "projet")
	f, err := configshare.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.Source.WorkspaceName != "licence-a" || !f.Source.ExportedAt.Equal(testNow) {
		t.Errorf("source = %+v", f.Source)
	}
	res, err := configshare.Apply(t.Context(), b.ws, b.st, b.eng, f, configshare.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Incomplete() || res.Summary.Added != 2 || !res.Settings.Written {
		t.Errorf("result = %+v", res)
	}
	repos := b.repos()
	if r := repos["alice/projet"]; r.Label != "Alice Martin" || !slices.Equal(r.BranchPatterns, []string{"main", "dev"}) || !r.Enabled {
		t.Errorf("alice = %+v", r)
	}
	if r := repos["bob/projet"]; r.Label != "" || len(r.BranchPatterns) != 0 {
		t.Errorf("bob = %+v", r)
	}
	cfg := b.reload()
	if cfg.Name != "licence-b" || cfg.Sync.Interval != time.Hour || cfg.Activity.DeadAfterDays != 21 ||
		cfg.Timezone != "Europe/Paris" || cfg.Lang != "fr" || !slices.Equal(cfg.Branches.Exclude, []string{"dependabot/*"}) {
		t.Errorf("settings of b = %+v", cfg)
	}
	// Re-export of b: same settings and repositories as a.
	b.ws.Config = cfg
	d2, _ := configshare.Export(t.Context(), b.ws, b.st, configshare.ExportOptions{Now: testNow})
	if !reflect.DeepEqual(d2.Settings, d.Settings) || !reflect.DeepEqual(d2.Repositories, d.Repositories) {
		t.Errorf("different re-export:\n%+v\n%+v", d2, d)
	}
}

func TestParseAccepts(t *testing.T) {
	// Repositories only: branches absent (keep), [] (all), duplicate patterns removed, URL.
	f := mustParse(t, doc("", `[
		{"repo": "alice/projet"},
		{"provider": "github", "repo": "https://github.com/bob/projet", "label": "Bob", "branches": []},
		{"repo": "carol/projet", "branches": [" main ", "main", "release/*"]},
		{"repo": "dave/projet", "branches": null}]`))
	if !f.Settings.IsZero() || len(f.Repositories) != 4 {
		t.Fatalf("file = %+v", f)
	}
	r := f.Repositories
	if r[0].Branches != nil || r[0].Index != 1 || r[0].Ref.String() != "alice/projet" {
		t.Errorf("alice = %+v", r[0])
	}
	if r[1].Branches == nil || len(r[1].Branches) != 0 || r[1].Ref.String() != "bob/projet" || r[1].Label != "Bob" {
		t.Errorf("bob = %+v", r[1])
	}
	if !slices.Equal(r[2].Branches, []string{"main", "release/*"}) {
		t.Errorf("carol = %+v", r[2])
	}
	if r[3].Branches != nil {
		t.Errorf("dave = %+v", r[3])
	}

	// Only a time zone; no repository.
	f = mustParse(t, doc(`{"timezone": "Europe/Paris"}`, ""))
	if keys := f.Settings.Keys(); !slices.Equal(keys, []string{workspace.KeyTimezone}) || len(f.Repositories) != 0 {
		t.Errorf("keys = %v, repositories = %d", keys, len(f.Repositories))
	}
	// Null values: not provided.
	f = mustParse(t, doc(`{"sync": {"interval": null, "auto": false}, "activity": null}`, `[]`))
	if keys := f.Settings.Keys(); !slices.Equal(keys, []string{workspace.KeySyncAuto}) {
		t.Errorf("keys = %v", keys)
	}
	// Duration "0": manual sync only.
	f = mustParse(t, doc(`{"sync": {"interval": "0"}}`, ""))
	if *f.Settings.SyncInterval != 0 {
		t.Error("interval 0 expected")
	}
	// The language, optional key (format v1 unchanged), normalized like config.yaml.
	f = mustParse(t, doc(`{"lang": " FR "}`, ""))
	if keys := f.Settings.Keys(); !slices.Equal(keys, []string{workspace.KeyLang}) || *f.Settings.Lang != "fr" {
		t.Errorf("keys = %v, lang = %v", keys, f.Settings.Lang)
	}
}

func TestParseRejects(t *testing.T) {
	big := `{"format": "watchoo-config", "version": 1, "workspace": {"name": "` + strings.Repeat("x", configshare.MaxFileSize) + `"}}`
	var many strings.Builder
	many.WriteString("[")
	for i := range configshare.MaxRepositories + 1 {
		if i > 0 {
			many.WriteString(",")
		}
		fmt.Fprintf(&many, `{"repo": "o/r%d"}`, i)
	}
	many.WriteString("]")
	var errorsList strings.Builder
	errorsList.WriteString("[")
	for i := range 25 {
		if i > 0 {
			errorsList.WriteString(",")
		}
		errorsList.WriteString(`{"repo": "invalid"}`)
	}
	errorsList.WriteString("]")

	for _, tt := range []struct {
		name, input string
		target      error
		parts       []string
	}{
		{"empty", "", configshare.ErrInvalid, []string{"JSON"}},
		{"invalid JSON", "{", configshare.ErrInvalid, []string{"JSON"}},
		{"not an object", "[]", configshare.ErrInvalid, []string{"JSON object expected"}},
		{"text after", doc("", "") + " {}", configshare.ErrInvalid, []string{"text after"}},
		{"too large", big, configshare.ErrTooLarge, nil},
		{"format missing", `{"version": 1}`, configshare.ErrUnsupportedFormat, []string{"watchoo-config"}},
		{"other format", `{"format": "other", "version": 1}`, configshare.ErrUnsupportedFormat, nil},
		{"format not text", `{"format": 1, "version": 1}`, configshare.ErrUnsupportedFormat, nil},
		{"version missing", `{"format": "watchoo-config"}`, configshare.ErrInvalid, []string{"version key missing"}},
		{"version as text", `{"format": "watchoo-config", "version": "1"}`, configshare.ErrInvalid, []string{"integer"}},
		{"future version", `{"format": "watchoo-config", "version": 2}`, configshare.ErrNewerVersion, []string{"newer"}},
		{"version zero", `{"format": "watchoo-config", "version": 0}`, configshare.ErrInvalid, []string{"version 0"}},
		{"unknown key", `{"format": "watchoo-config", "version": 1, "token": "x"}`, configshare.ErrInvalid, []string{`"token"`}},
		{"github in settings", doc(`{"github": {"gh_path": "/tmp/evil"}}`, ""), configshare.ErrInvalid, []string{`"github"`}},
		{"web in settings", doc(`{"web": {"addr": "0.0.0.0:80"}}`, ""), configshare.ErrInvalid, []string{`"web"`}},
		{"name in settings", doc(`{"name": "x"}`, ""), configshare.ErrInvalid, []string{`"name"`}},
		{"unknown sync key", doc(`{"sync": {"intervall": "1h"}}`, ""), configshare.ErrInvalid, []string{`"intervall"`}},
		{"unknown repository key", doc("", `[{"repo": "a/b", "enabled": false}]`), configshare.ErrInvalid, []string{`"enabled"`}},
		{"type", doc(`{"sync": {"parallelism": "4"}}`, ""), configshare.ErrInvalid, []string{"parallelism", "integer"}},
		{"too many repositories", doc("", many.String()), configshare.ErrInvalid, []string{"1001 repositories"}},
		{"provider", doc("", `[{"provider": "gitlab", "repo": "a/b"}]`), configshare.ErrInvalid, []string{"gitlab"}},
		{"empty repo", doc("", `[{"label": "x"}]`), configshare.ErrInvalid, []string{"repository #1", "dépôt vide"}},
		{"invalid repo", doc("", `[{"repo": "a/b/c"}]`), configshare.ErrInvalid, []string{"repository #1"}},
		{"other host", doc("", `[{"repo": "https://gitlab.com/a/b"}]`), configshare.ErrInvalid, []string{"github.com"}},
		{"duplicate", doc("", `[{"repo": "Alice/Projet"}, {"repo": "alice/projet"}]`), configshare.ErrInvalid,
			[]string{"repository #2", "present twice", "#1"}},
		{"label control", doc("", `[{"repo": "a/b", "label": "x\u001b[2J"}]`), configshare.ErrInvalid, []string{"contrôle"}},
		{"long label", doc("", `[{"repo": "a/b", "label": "`+strings.Repeat("é", 201)+`"}]`), configshare.ErrInvalid, []string{"trop long"}},
		{"empty pattern", doc("", `[{"repo": "a/b", "branches": [" "]}]`), configshare.ErrInvalid, []string{"empty"}},
		{"pattern with comma", doc("", `[{"repo": "a/b", "branches": ["main,dev"]}]`), configshare.ErrInvalid, []string{"comma"}},
		{"invalid pattern", doc("", `[{"repo": "a/b", "branches": ["[a"]}]`), configshare.ErrInvalid, []string{`"[a"`}},
		{"pattern control", doc("", `[{"repo": "a/b", "branches": ["a\nb"]}]`), configshare.ErrInvalid, []string{"branches"}},
		{"duration", doc(`{"sync": {"interval": "30 minutes"}}`, ""), configshare.ErrInvalid, []string{"interval", "unreadable"}},
		{"invalid exclude", doc(`{"branches": {"exclude": ["[a"]}}`, ""), configshare.ErrInvalid, []string{"exclude"}},
		{"empty exclude", doc(`{"branches": {"exclude": [""]}}`, ""), configshare.ErrInvalid, []string{"empty"}},
		{"exclude control", doc(`{"branches": {"exclude": ["a\nb: c"]}}`, ""), configshare.ErrInvalid, []string{"control character"}},
		{"time zone injection", doc(`{"timezone": "Europe/Paris\"\ngithub:\n  gh_path: /tmp/evil"}`, ""), configshare.ErrInvalid,
			[]string{"timezone", "control character"}},
		{"unknown language", doc(`{"lang": "de"}`, ""), configshare.ErrInvalid, []string{"settings.lang", `"de"`}},
		{"language not text", doc(`{"lang": 1}`, ""), configshare.ErrInvalid, []string{"lang"}},
		{"workspace name control", `{"format": "watchoo-config", "version": 1, "workspace": {"name": "a\u0007"}}`,
			configshare.ErrInvalid, []string{"workspace.name"}},
		{"date", `{"format": "watchoo-config", "version": 1, "exported_at": "yesterday"}`, configshare.ErrInvalid, []string{"exported_at"}},
		{"several errors", doc("", errorsList.String()), configshare.ErrInvalid, []string{"repository #1", "repository #20", "and 5 other problems"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := configshare.Parse(strings.NewReader(tt.input))
			if !errors.Is(err, tt.target) {
				t.Fatalf("error = %v, want %v", err, tt.target)
			}
			for _, p := range tt.parts {
				if !strings.Contains(err.Error(), p) {
					t.Errorf("message without %q:\n%v", p, err)
				}
			}
		})
	}
}

// failingReader always returns an error.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestParseReadError(t *testing.T) {
	if _, err := configshare.Parse(failingReader{}); err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Errorf("error = %v", err)
	}
}

// Merge: addition, update (label, branches), unchanged, stopped repository resumed; the preview
// changes nothing (database and config.yaml byte for byte).
func TestPlanAndApplyMerge(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	e.track(1, "alice", "projet", "Alice")                 // unchanged
	e.track(2, "bob", "projet", "Bob", "main")             // label and branches updated
	carol := e.track(3, "carol", "projet", "Carol", "dev") // stopped: resumed
	e.stop(carol.ID)
	e.track(4, "dave", "projet", "Dave", "main") // absent from the file: never removed
	e.forge(5, "eva", "projet")                  // to add
	e.forge(6, "fred", "projet")

	f := mustParse(t, doc(`{"sync": {"interval": "30m"}}`, `[
		{"repo": "alice/projet", "label": "", "branches": null},
		{"repo": "Bob/Projet", "label": "Bob Durand", "branches": []},
		{"repo": "carol/projet"},
		{"repo": "eva/projet", "label": "Eva", "branches": ["main"]},
		{"repo": "fred/projet", "label": "Fred"},
		{"repo": "ghost/projet"}]`))

	configBefore := readFile(t, e.ws.ConfigPath())
	dbBefore := readFile(t, e.ws.DBPath())
	reposBefore := e.repos()

	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantActions := []configshare.Action{configshare.ActionUnchanged, configshare.ActionUpdate, configshare.ActionUpdate,
		configshare.ActionAdd, configshare.ActionAdd, configshare.ActionAdd}
	for i, c := range p.Repositories {
		if c.Action != wantActions[i] || c.Index != i+1 || c.Outcome != "" || c.Repository != nil {
			t.Errorf("repository %d = %+v, want action %s", i+1, c, wantActions[i])
		}
	}
	bob := p.Repositories[1]
	if bob.Repo != "Bob/Projet" || bob.Before == nil || bob.Before.Label != "Bob" || !slices.Equal(bob.Before.Branches, []string{"main"}) ||
		bob.After.Label != "Bob Durand" || bob.After.Branches == nil || len(bob.After.Branches) != 0 || bob.Reenable {
		t.Errorf("bob = %+v (before %+v)", bob, bob.Before)
	}
	if c := p.Repositories[2]; !c.Reenable || c.Before.Enabled || !c.After.Enabled || c.After.Label != "Carol" {
		t.Errorf("carol = %+v", c)
	}
	if c := p.Repositories[3]; c.Before != nil || c.After.Label != "Eva" || !slices.Equal(c.After.Branches, []string{"main"}) {
		t.Errorf("eva = %+v", c)
	}
	if want := (configshare.Summary{Added: 3, AlreadyTracked: 1, Updated: 2, Reenabled: 1}); p.Summary != want {
		t.Errorf("summary = %+v, want %+v", p.Summary, want)
	}
	if p.Settings.Rewrite || len(p.Settings.Changes) != 0 || len(p.Warnings) != 0 || p.AccessChecked {
		t.Errorf("identical settings: %+v, warnings %v", p.Settings, p.Warnings)
	}

	// Preview with access check: ghost inaccessible; nothing is written either.
	p, err = configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{CheckAccess: e.eng})
	if err != nil {
		t.Fatal(err)
	}
	if !p.AccessChecked || p.Repositories[5].Action != configshare.ActionInaccessible || p.Repositories[5].Error == "" {
		t.Errorf("ghost = %+v", p.Repositories[5])
	}
	if want := (configshare.Summary{Added: 2, AlreadyTracked: 1, Updated: 2, Reenabled: 1, Inaccessible: 1}); p.Summary != want {
		t.Errorf("checked summary = %+v, want %+v", p.Summary, want)
	}
	if got := readFile(t, e.ws.ConfigPath()); got != configBefore {
		t.Error("the preview changed config.yaml")
	}
	if got := readFile(t, e.ws.DBPath()); got != dbBefore {
		t.Error("the preview changed the database")
	}
	if !reflect.DeepEqual(e.repos(), reposBefore) {
		t.Error("the preview changed the repositories")
	}

	// Application.
	res, err := configshare.Apply(t.Context(), e.ws, e.st, e.eng, f, configshare.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Incomplete() || res.Settings.Written {
		t.Errorf("result = %+v", res)
	}
	if want := (configshare.Summary{Added: 2, AlreadyTracked: 1, Updated: 2, Reenabled: 1, Inaccessible: 1}); res.Summary != want {
		t.Errorf("result = %+v, want %+v", res.Summary, want)
	}
	outcomes := []wsync.AddOutcome{wsync.OutcomeAlreadyTracked, wsync.OutcomeUpdated, wsync.OutcomeUpdated,
		wsync.OutcomeAdded, wsync.OutcomeAdded, wsync.OutcomeInaccessible}
	for i, c := range res.Preview.Repositories {
		if c.Outcome != outcomes[i] {
			t.Errorf("repository %d: outcome %s, want %s", i+1, c.Outcome, outcomes[i])
		}
		if (c.Repository == nil) != (outcomes[i] == wsync.OutcomeInaccessible) {
			t.Errorf("repository %d: Repository = %v", i+1, c.Repository)
		}
	}
	repos := e.repos()
	if r := repos["bob/projet"]; r.Label != "Bob Durand" || len(r.BranchPatterns) != 0 {
		t.Errorf("bob = %+v", r)
	}
	if r := repos["carol/projet"]; !r.Enabled || r.Label != "Carol" || !slices.Equal(r.BranchPatterns, []string{"dev"}) {
		t.Errorf("carol = %+v", r)
	}
	if r := repos["alice/projet"]; r.Label != "Alice" {
		t.Errorf("alice = %+v", r)
	}
	if r, ok := repos["dave/projet"]; !ok || !r.Enabled {
		t.Error("dave was removed or stopped")
	}
	if r := repos["eva/projet"]; r.Label != "Eva" || !slices.Equal(r.BranchPatterns, []string{"main"}) {
		t.Errorf("eva = %+v", r)
	}
	if r := repos["fred/projet"]; r.Label != "Fred" || len(r.BranchPatterns) != 0 {
		t.Errorf("fred = %+v", r)
	}
	if _, ok := repos["ghost/projet"]; ok || len(repos) != 6 {
		t.Errorf("repositories = %v", repos)
	}
	// Identical settings: config.yaml not rewritten, no .bak.
	if got := readFile(t, e.ws.ConfigPath()); got != configBefore {
		t.Error("config.yaml rewritten although no setting changes")
	}
	if _, err := os.Stat(filepath.Join(e.ws.Dir, workspace.BackupFileName)); !os.IsNotExist(err) {
		t.Error(".bak created without a rewrite")
	}
}

// Settings: only different values are changes; WATCHOO_* variable reported; comments lost;
// config.yaml rewritten with .bak, machine keys kept.
func TestPlanAndApplySettings(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	custom := "# personal comment\nname: \"Licence\"\ngithub:\n  gh_path: \"/opt/gh\"\nweb:\n  addr: \"127.0.0.1:9000\"\n"
	if err := os.WriteFile(e.ws.ConfigPath(), []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Load(e.ws); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHOO_SYNC_INTERVAL", "1h")
	if err := workspace.Load(e.ws); err != nil {
		t.Fatal(err)
	}
	f := mustParse(t, doc(`{"sync": {"auto": true, "interval": "45m", "parallelism": 2},
		"activity": {"dead_after_days": 20}, "branches": {"exclude": ["renovate/*"]}, "timezone": "Europe/Paris", "lang": "fr"}`, ""))

	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantChanges := []configshare.SettingChange{
		{Key: "sync.interval", Before: "30m", After: "45m", EnvVar: "WATCHOO_SYNC_INTERVAL"}, // file value, not the variable
		{Key: "sync.parallelism", Before: "4", After: "2"},
		{Key: "activity.dead_after_days", Before: "15", After: "20"},
		{Key: "branches.exclude", Before: "[]", After: `["renovate/*"]`},
		{Key: "timezone", Before: `""`, After: `"Europe/Paris"`},
		{Key: "lang", Before: `"en"`, After: `"fr"`},
	}
	if !reflect.DeepEqual(p.Settings.Changes, wantChanges) {
		t.Errorf("changes = %+v\nwant %+v", p.Settings.Changes, wantChanges)
	}
	if !p.Settings.Rewrite || len(p.Settings.Provided) != 7 {
		t.Errorf("settings = %+v", p.Settings)
	}
	var codes []string
	for _, w := range p.Warnings {
		codes = append(codes, w.Code)
	}
	if !slices.Equal(codes, []string{configshare.WarnEnvOverride, configshare.WarnCommentsLost}) ||
		p.Warnings[0].Key != "sync.interval" || !strings.Contains(p.Warnings[0].Message, "WATCHOO_SYNC_INTERVAL") ||
		!strings.Contains(p.Warnings[1].Message, workspace.BackupFileName) {
		t.Errorf("warnings = %+v", p.Warnings)
	}
	if got := readFile(t, e.ws.ConfigPath()); got != custom {
		t.Error("the preview changed config.yaml")
	}

	res, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Settings.Written || res.Settings.Path != e.ws.ConfigPath() ||
		res.Settings.BackupPath != filepath.Join(e.ws.Dir, workspace.BackupFileName) || res.Incomplete() {
		t.Errorf("result = %+v", res.Settings)
	}
	if got := readFile(t, res.Settings.BackupPath); got != custom {
		t.Errorf(".bak = %q", got)
	}
	t.Setenv("WATCHOO_SYNC_INTERVAL", "")
	cfg := e.reload()
	if cfg.Name != "Licence" || cfg.GitHub.GHPath != "/opt/gh" || cfg.Web.Addr != "127.0.0.1:9000" {
		t.Errorf("machine keys lost: %+v", cfg)
	}
	if cfg.Sync.Interval != 45*time.Minute || cfg.Sync.Parallelism != 2 || cfg.Activity.DeadAfterDays != 20 ||
		cfg.Timezone != "Europe/Paris" || cfg.Lang != "fr" || !slices.Equal(cfg.Branches.Exclude, []string{"renovate/*"}) {
		t.Errorf("imported settings = %+v", cfg)
	}
	if info, err := os.Stat(e.ws.ConfigPath()); err == nil && info.Mode().Perm() != 0o600 && os.PathSeparator == '/' {
		t.Errorf("permissions = %v", info.Mode().Perm())
	}
}

// A WATCHOO_* variable equal to the imported value, while config.yaml has another one: the setting
// is compared to the file value, hence written, and the variable is reported (finding: comparing to
// the effective value saw no change and reported nothing).
func TestPlanEnvOverrideEqualToImported(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	if err := os.WriteFile(e.ws.ConfigPath(), []byte("sync:\n  interval: 1h\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHOO_SYNC_INTERVAL", "2h")
	if err := workspace.Load(e.ws); err != nil {
		t.Fatal(err)
	}
	f := mustParse(t, doc(`{"sync": {"interval": "2h"}}`, ""))
	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []configshare.SettingChange{{Key: "sync.interval", Before: "1h", After: "2h", EnvVar: "WATCHOO_SYNC_INTERVAL"}}
	if !reflect.DeepEqual(p.Settings.Changes, want) || !p.Settings.Rewrite {
		t.Errorf("settings = %+v", p.Settings)
	}
	if len(p.Warnings) == 0 || p.Warnings[0].Code != configshare.WarnEnvOverride || p.Warnings[0].Key != "sync.interval" {
		t.Errorf("warnings = %+v", p.Warnings)
	}
	res, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{})
	if err != nil || !res.Settings.Written {
		t.Fatalf("Apply = %+v, %v", res, err)
	}
	t.Setenv("WATCHOO_SYNC_INTERVAL", "")
	if cfg := e.reload(); cfg.Sync.Interval != 2*time.Hour {
		t.Errorf("sync.interval without the variable = %v, want 2h", cfg.Sync.Interval)
	}
}

// An overridden setting already written in config.yaml: importing the same file again rewrites
// nothing (the .bak keeps the original file), but the variable is still reported.
func TestApplyEnvOverrideIdempotent(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	original := "# personal comment\nsync:\n  interval: 1h\n"
	if err := os.WriteFile(e.ws.ConfigPath(), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHOO_SYNC_INTERVAL", "1h")
	if err := workspace.Load(e.ws); err != nil {
		t.Fatal(err)
	}
	f := mustParse(t, doc(`{"sync": {"interval": "45m"}}`, ""))
	res, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{})
	if err != nil || !res.Settings.Written {
		t.Fatalf("first import = %+v, %v", res, err)
	}
	written := readFile(t, e.ws.ConfigPath())
	bak := filepath.Join(e.ws.Dir, workspace.BackupFileName)

	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Settings.Rewrite || len(p.Settings.Changes) != 0 {
		t.Errorf("second preview: settings = %+v", p.Settings)
	}
	if len(p.Warnings) != 1 || p.Warnings[0].Code != configshare.WarnEnvOverride ||
		!strings.Contains(p.Warnings[0].Message, "config.yaml already contains the imported value") {
		t.Errorf("second preview: warnings = %+v", p.Warnings)
	}
	res, err = configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{})
	if err != nil || res.Settings.Written {
		t.Fatalf("second import = %+v, %v", res, err)
	}
	if readFile(t, e.ws.ConfigPath()) != written {
		t.Error("config.yaml rewritten by the second import")
	}
	if readFile(t, bak) != original {
		t.Error("config.yaml.bak no longer keeps the original file")
	}
}

// A WATCHOO_* variable that makes the imported settings invalid (thresholds): the preview refuses
// like the write, with ErrInvalid; nothing is written.
func TestPlanRejectsSettingsInvalidWithEnv(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	t.Setenv("WATCHOO_ACTIVITY_DEAD_AFTER_DAYS", "10")
	if err := workspace.Load(e.ws); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, e.ws.ConfigPath())
	f := mustParse(t, doc(`{"activity": {"flemme_after_days": 12, "dead_after_days": 20}}`, ""))
	_, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if !errors.Is(err, configshare.ErrInvalid) || !strings.Contains(err.Error(), "WATCHOO_") {
		t.Errorf("Plan = %v, want ErrInvalid", err)
	}
	if _, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{}); !errors.Is(err, configshare.ErrInvalid) {
		t.Errorf("Apply = %v, want ErrInvalid", err)
	}
	if readFile(t, e.ws.ConfigPath()) != before {
		t.Error("config.yaml changed")
	}
	if _, err := os.Stat(filepath.Join(e.ws.Dir, workspace.BackupFileName)); err == nil {
		t.Error("config.yaml.bak created")
	}
}

// The settings are validated once merged with those of the workspace: refused, nothing is written
// (neither settings nor repositories).
func TestPlanRejectsInvalidMergedSettings(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	e.forge(1, "alice", "projet")
	before := readFile(t, e.ws.ConfigPath())
	for name, settings := range map[string]string{
		"thresholds":  `{"activity": {"flemme_after_days": 20}}`,
		"parallelism": `{"sync": {"parallelism": 9}}`,
		"interval":    `{"sync": {"interval": "5m"}}`,
		"time zone":   `{"timezone": "Mars/Olympus"}`,
	} {
		f := mustParse(t, doc(settings, `[{"repo": "alice/projet"}]`))
		if _, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{}); !errors.Is(err, configshare.ErrInvalid) {
			t.Errorf("%s: Plan = %v, want ErrInvalid", name, err)
		}
		if _, err := configshare.Apply(t.Context(), e.ws, e.st, e.eng, f, configshare.ApplyOptions{}); !errors.Is(err, configshare.ErrInvalid) {
			t.Errorf("%s: Apply = %v, want ErrInvalid", name, err)
		}
	}
	if got := readFile(t, e.ws.ConfigPath()); got != before {
		t.Error("config.yaml changed")
	}
	if n := len(e.repos()); n != 0 {
		t.Errorf("%d repositories added", n)
	}
	if n := e.fake.Calls("Repository"); n != 0 {
		t.Errorf("%d calls to the forge", n)
	}
}

// A global error of the addition (gh not signed in) is returned with the result; the settings are
// already written, the unprocessed repositories are in error.
func TestApplyGlobalError(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	e.forge(1, "alice", "projet")
	e.auth.SetTokenErr(errors.New("gh not signed in"))
	f := mustParse(t, doc(`{"timezone": "UTC"}`, `[{"repo": "alice/projet"}]`))
	res, err := configshare.Apply(t.Context(), e.ws, e.st, e.eng, f, configshare.ApplyOptions{})
	if err == nil || res == nil {
		t.Fatalf("Apply = %+v, %v", res, err)
	}
	if !res.Settings.Written || res.Summary.Errors != 1 || !res.Incomplete() ||
		res.Preview.Repositories[0].Outcome != wsync.OutcomeError {
		t.Errorf("result = %+v", res)
	}
	if strings.Contains(err.Error(), authtest.FakeToken) {
		t.Error("token in the error")
	}
}

func TestApplyWithoutAdder(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	f := mustParse(t, doc("", `[{"repo": "alice/projet"}]`))
	if _, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{}); err == nil {
		t.Error("error expected without an Adder")
	}
}

// Quota exhausted during the access check: repositories in error, without blocking the preview.
func TestPlanAccessQuota(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	e.forge(1, "alice", "projet")
	e.fake.SetRepoError("alice", "projet", &provider.RateLimitError{Reset: testNow})
	f := mustParse(t, doc("", `[{"repo": "alice/projet"}]`))
	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{CheckAccess: e.eng})
	if err != nil {
		t.Fatal(err)
	}
	if p.Repositories[0].Action != configshare.ActionError || p.Summary.Errors != 1 {
		t.Errorf("preview = %+v", p.Repositories[0])
	}
}

func TestPlanRequiresArguments(t *testing.T) {
	if _, err := configshare.Plan(context.Background(), nil, nil, nil, configshare.PlanOptions{}); err == nil {
		t.Error("error expected")
	}
	if _, err := configshare.Export(context.Background(), nil, nil, configshare.ExportOptions{}); err == nil {
		t.Error("error expected")
	}
}

func TestFileName(t *testing.T) {
	for in, want := range map[string]string{
		"licence-projet":           "watchoo-config-licence-projet.json",
		"Licence Projet 2026":      "watchoo-config-Licence-Projet-2026.json",
		"Élodie & co / \"x\"\r\n.": "watchoo-config-lodie-co-x.json",
		"../../etc/passwd":         "watchoo-config-etc-passwd.json",
		"":                         "watchoo-config.json",
		"ééé":                      "watchoo-config.json",
		strings.Repeat("a", 100):   "watchoo-config-" + strings.Repeat("a", 60) + ".json",
	} {
		if got := configshare.FileName(in); got != want {
			t.Errorf("FileName(%q) = %q, want %q", in, got, want)
		}
	}
}
