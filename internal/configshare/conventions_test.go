package configshare_test

import (
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/configshare"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// The conventions charter (settings.conventions) is read, previewed and applied; conventions.enabled
// and conventions.excluded_accounts, never shared, are kept.
func TestConventionsImport(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	custom := "name: \"Licence\"\nconventions:\n  enabled: true\n  excluded_accounts: [\"prof-dupont\"]\n"
	if err := os.WriteFile(e.ws.ConfigPath(), []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Load(e.ws); err != nil {
		t.Fatal(err)
	}
	f := mustParse(t, doc(`{"conventions": {"since": "2026-09-01", "commit": {"types": ["feat", "fix"],
		"scopes": ["api"], "max_subject_soft": 60, "max_subject_hard": 72, "forbidden": [], "language": "fr"}}}`, ""))
	wantKeys := []string{workspace.KeyConventionsSince, workspace.KeyConventionsTypes, workspace.KeyConventionsScopes,
		workspace.KeyConventionsMaxSubjectSoft, workspace.KeyConventionsMaxSubjectHard,
		workspace.KeyConventionsForbidden, workspace.KeyConventionsLanguage}
	if keys := f.Settings.Keys(); !slices.Equal(keys, wantKeys) {
		t.Errorf("keys = %v", keys)
	}

	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var changed []string
	for _, c := range p.Settings.Changes {
		changed = append(changed, c.Key)
	}
	// max_subject_hard (72) is the current value: not a change.
	if want := []string{workspace.KeyConventionsSince, workspace.KeyConventionsTypes, workspace.KeyConventionsScopes,
		workspace.KeyConventionsMaxSubjectSoft, workspace.KeyConventionsForbidden,
		workspace.KeyConventionsLanguage}; !slices.Equal(changed, want) {
		t.Errorf("changes = %v\nwant %v", changed, want)
	}
	if c := p.Settings.Changes[0]; c.Before != `""` || c.After != `"2026-09-01"` {
		t.Errorf("since change = %+v", c)
	}

	if _, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	got := e.reload().Conventions
	want := workspace.ConventionsConfig{Enabled: true, Since: "2026-09-01",
		Commit: workspace.ConventionsCommitConfig{Types: []string{"feat", "fix"}, Scopes: []string{"api"},
			MaxSubjectSoft: 60, MaxSubjectHard: 72, Forbidden: []string{}, Language: "fr"},
		Branch:           workspace.DefaultConfig("x").Conventions.Branch,
		Tags:             workspace.DefaultConfig("x").Conventions.Tags,
		Changelog:        workspace.DefaultConfig("x").Conventions.Changelog,
		Readme:           workspace.DefaultConfig("x").Conventions.Readme,
		Docs:             workspace.DefaultConfig("x").Conventions.Docs,
		ExcludedAccounts: []string{"prof-dupont"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("conventions = %+v\nwant %+v", got, want)
	}

	// Importing the same file again changes nothing (empty lists compared as such).
	e.ws.Config = e.reload()
	p, err = configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Settings.Rewrite || len(p.Settings.Changes) != 0 {
		t.Errorf("second import changes = %+v", p.Settings.Changes)
	}
}

// The branch naming (settings.conventions.branch) and the milestones (settings.conventions.tags), phase
// 8.2, are read, previewed and applied like the commit rules; an empty pattern is the built-in rule.
func TestConventionsBranchTagsImport(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	f := mustParse(t, doc(`{"conventions": {"branch": {"pattern": "^(feat|fix)/[a-z0-9-]{2,}$",
		"exclude": ["gh-pages", "release/*"]}, "tags": {"milestones": ["v1.0.0@2026-12-18", "v2.0.0"]}}}`, ""))
	wantKeys := []string{workspace.KeyConventionsBranchPattern, workspace.KeyConventionsBranchExclude,
		workspace.KeyConventionsMilestones}
	if keys := f.Settings.Keys(); !slices.Equal(keys, wantKeys) {
		t.Errorf("keys = %v", keys)
	}
	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var changed []string
	for _, c := range p.Settings.Changes {
		changed = append(changed, c.Key)
	}
	if !slices.Equal(changed, wantKeys) {
		t.Errorf("changes = %v\nwant %v", changed, wantKeys)
	}
	if c := p.Settings.Changes[2]; c.Before != `["v1.0.0", "v2.0.0", "v3.0.0"]` ||
		c.After != `["v1.0.0@2026-12-18", "v2.0.0"]` {
		t.Errorf("milestones change = %+v", c)
	}
	if _, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	got := e.reload().Conventions
	if got.Branch.Pattern != "^(feat|fix)/[a-z0-9-]{2,}$" || !slices.Equal(got.Branch.Exclude, []string{"gh-pages", "release/*"}) ||
		!slices.Equal(got.Tags.Milestones, []string{"v1.0.0@2026-12-18", "v2.0.0"}) {
		t.Errorf("conventions = %+v", got)
	}

	// Exported again as written; an empty pattern and empty lists stay explicit.
	e.ws.Config = e.reload()
	e.ws.Config.Conventions.Branch.Pattern = ""
	e.ws.Config.Conventions.Tags.Milestones = []string{}
	d, err := configshare.Export(t.Context(), e.ws, e.st, configshare.ExportOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := d.Marshal()
	for _, want := range []string{`"pattern": ""`, `"release/*"`, `"milestones": []`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("export lacks %s:\n%s", want, data)
		}
	}
	back := mustParse(t, string(data))
	if *back.Settings.ConventionsBranchPattern != "" || len(*back.Settings.ConventionsMilestones) != 0 {
		t.Errorf("read back = %q / %v", *back.Settings.ConventionsBranchPattern, *back.Settings.ConventionsMilestones)
	}
}

// The file criteria settings (settings.conventions.changelog, readme and docs), phase 8.3, are read,
// previewed and applied like the other conventions; false and empty lists stay explicit on export.
func TestConventionsFilesImport(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	f := mustParse(t, doc(`{"conventions": {"changelog": {"categories_fr": false},
		"readme": {"sections": ["install", "run"], "min_words": 80},
		"docs": {"dir": "docs/projet", "media_dir": "images", "archives_scope": "documentation"}}}`, ""))
	wantKeys := []string{workspace.KeyConventionsCategoriesFR, workspace.KeyConventionsReadmeSections,
		workspace.KeyConventionsReadmeMinWords, workspace.KeyConventionsDocsDir, workspace.KeyConventionsDocsMediaDir,
		workspace.KeyConventionsArchivesScope}
	if keys := f.Settings.Keys(); !slices.Equal(keys, wantKeys) {
		t.Errorf("keys = %v", keys)
	}
	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var changed []string
	for _, c := range p.Settings.Changes {
		changed = append(changed, c.Key)
	}
	if !slices.Equal(changed, wantKeys) {
		t.Errorf("changes = %v\nwant %v", changed, wantKeys)
	}
	if c := p.Settings.Changes[1]; c.After != `["install", "run"]` {
		t.Errorf("sections change = %+v", c)
	}
	if _, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	got := e.reload().Conventions
	if got.Changelog.CategoriesFR || !slices.Equal(got.Readme.Sections, []string{"install", "run"}) ||
		got.Readme.MinWords != 80 || got.Docs.Dir != "docs/projet" || got.Docs.MediaDir != "images" ||
		got.Docs.ArchivesScope != "documentation" {
		t.Errorf("conventions = %+v", got)
	}

	e.ws.Config = e.reload()
	e.ws.Config.Conventions.Readme.Sections = []string{}
	d, err := configshare.Export(t.Context(), e.ws, e.st, configshare.ExportOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := d.Marshal()
	for _, want := range []string{`"categories_fr": false`, `"sections": []`, `"dir": "docs/projet"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("export lacks %s:\n%s", want, data)
		}
	}
	back := mustParse(t, string(data))
	if *back.Settings.ConventionsCategoriesFR || len(*back.Settings.ConventionsReadmeSections) != 0 {
		t.Errorf("read back = %v / %v", *back.Settings.ConventionsCategoriesFR, *back.Settings.ConventionsReadmeSections)
	}
}

// Optional keys: an empty since (from the beginning), a block without commit, null values.
func TestConventionsParseAccepts(t *testing.T) {
	f := mustParse(t, doc(`{"conventions": {"since": ""}}`, ""))
	if keys := f.Settings.Keys(); !slices.Equal(keys, []string{workspace.KeyConventionsSince}) || *f.Settings.ConventionsSince != "" {
		t.Errorf("keys = %v", keys)
	}
	f = mustParse(t, doc(`{"conventions": {"since": null, "commit": {"types": null, "language": "any"}}}`, ""))
	if keys := f.Settings.Keys(); !slices.Equal(keys, []string{workspace.KeyConventionsLanguage}) {
		t.Errorf("keys = %v", keys)
	}
	f = mustParse(t, doc(`{"conventions": {}}`, ""))
	if !f.Settings.IsZero() {
		t.Errorf("empty block: %v", f.Settings.Keys())
	}
}

func TestConventionsParseRejects(t *testing.T) {
	for in, want := range map[string]string{
		`{"since": "01/09/2026"}`:                       `settings.conventions.since: "01/09/2026" is not a date`,
		`{"since": "2026-09-01T00:00:00Z"}`:             "settings.conventions.since",
		`{"commit": {"types": ["feat", "Fix"]}}`:        `settings.conventions.commit.types: entry #2 "Fix" is invalid`,
		`{"commit": {"types": [""]}}`:                   "settings.conventions.commit.types: entry #1 is empty",
		`{"commit": {"scopes": ["API"]}}`:               `settings.conventions.commit.scopes: entry #1 "API" is invalid`,
		`{"commit": {"forbidden": ["wip", "a\u0007"]}}`: "settings.conventions.commit.forbidden: entry #2 contains a control character",
		`{"commit": {"forbidden": [" "]}}`:              "settings.conventions.commit.forbidden: entry #1 is empty",
		`{"commit": {"language": "de"}}`:                `settings.conventions.commit.language: unknown value "de"`,
		`{"commit": {"max_subject_soft": "50"}}`:        "type",
		`{"enabled": true}`:                             "enabled",
		`{"excluded_accounts": ["prof"]}`:               "excluded_accounts",
		`{"commit": {"type": ["feat"]}}`:                "type",

		`{"branch": {"pattern": "^(feat/"}}`:              `settings.conventions.branch.pattern: "^(feat/" is not a valid RE2`,
		`{"branch": {"pattern": "^(?!main)"}}`:            "settings.conventions.branch.pattern",
		`{"branch": {"pattern": "a\u0007"}}`:              "settings.conventions.branch.pattern",
		`{"branch": {"exclude": ["gh-pages", ""]}}`:       "settings.conventions.branch.exclude: entry #2 is empty",
		`{"branch": {"exclude": ["release/["]}}`:          `settings.conventions.branch.exclude: entry #1 "release/[" is invalid`,
		`{"branch": {"exclude": ["a\u0007"]}}`:            "settings.conventions.branch.exclude: entry #1 contains a control character",
		`{"branch": {"exclude": "gh-pages"}}`:             "type",
		`{"tags": {"milestones": ["1.0.0"]}}`:             `settings.conventions.tags.milestones: entry #1 "1.0.0" is not a final version`,
		`{"tags": {"milestones": ["v1.0.0-rc.1"]}}`:       `entry #1 "v1.0.0-rc.1" is not a final version`,
		`{"tags": {"milestones": ["v1.0.0@2026-02-30"]}}`: `settings.conventions.tags.milestones: entry #1 "v1.0.0@2026-02-30" has an invalid due date`,
		`{"tags": {"milestones": ["v1.0.0", "v1.0.0"]}}`:  "settings.conventions.tags.milestones: entry #2 repeats the milestone v1.0.0",
		`{"tags": {"milestones": ["v1\u0007"]}}`:          "settings.conventions.tags.milestones: entry #1 contains a control character",
		`{"tags": {"prerelease_forms": ["rc"]}}`:          "prerelease_forms",

		`{"readme": {"sections": ["install", "Install"]}}`: `settings.conventions.readme.sections: entry #2 "Install" is invalid`,
		`{"readme": {"sections": [""]}}`:                   "settings.conventions.readme.sections: entry #1 is empty",
		`{"readme": {"min_words": "150"}}`:                 "type",
		`{"readme": {"titles": ["Install"]}}`:              "titles",
		`{"changelog": {"categories_fr": "yes"}}`:          "type",
		`{"docs": {"dir": "../docs"}}`:                     `settings.conventions.docs.dir: "../docs" is invalid`,
		`{"docs": {"dir": "/documentation"}}`:              `settings.conventions.docs.dir: "/documentation" is invalid`,
		`{"docs": {"dir": "a\u0007"}}`:                     "settings.conventions.docs.dir",
		`{"docs": {"media_dir": "a/b"}}`:                   `settings.conventions.docs.media_dir: "a/b" is invalid`,
		`{"docs": {"archives_scope": "docs"}}`:             `settings.conventions.docs.archives_scope: "docs" is invalid`,
	} {
		_, err := configshare.Parse(strings.NewReader(doc(`{"conventions": `+in+`}`, "")))
		if !errors.Is(err, configshare.ErrInvalid) || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%s) = %v, want %q", in, err, want)
		}
	}
}

// The consistency with the workspace (at least one type, hard length above the soft one) is checked
// once merged: refused, nothing is written.
func TestConventionsPlanRejectsInvalidMerged(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	before := readFile(t, e.ws.ConfigPath())
	for name, settings := range map[string]string{
		"no type":          `{"conventions": {"commit": {"types": []}}}`,
		"soft above hard":  `{"conventions": {"commit": {"max_subject_soft": 80}}}`,
		"zero soft length": `{"conventions": {"commit": {"max_subject_soft": 0}}}`,
		"zero min words":   `{"conventions": {"readme": {"min_words": 0}}}`,
		"repeated section": `{"conventions": {"readme": {"sections": ["run", "run"]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := mustParse(t, doc(settings, ""))
			if _, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{}); !errors.Is(err, configshare.ErrInvalid) {
				t.Errorf("Plan = %v, want ErrInvalid", err)
			}
			if readFile(t, e.ws.ConfigPath()) != before {
				t.Error("config.yaml changed")
			}
		})
	}
}

// The per-repository conventions start date is exported (only if there is one), read back,
// previewed and applied; "" goes back to the date of the workspace, absent keeps the existing date.
func TestConventionsSinceExportImport(t *testing.T) {
	isolate(t)
	a := newEnv(t, "licence-a")
	alice := a.track(1, "alice", "projet", "Alice")
	a.track(2, "bob", "projet", "")
	if _, err := a.st.SetRepositoryConventionsSince(t.Context(), alice.ID, "2026-09-01"); err != nil {
		t.Fatal(err)
	}
	d, err := configshare.Export(t.Context(), a.ws, a.st, configshare.ExportOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := d.Marshal()
	if strings.Count(string(data), `"conventions_since": "2026-09-01"`) != 1 || strings.Count(string(data), `"conventions_since"`) != 1 {
		t.Errorf("conventions_since written for a repository without its own date, or missing:\n%s", data)
	}

	// b: alice absent, bob present with its own date that the document does not give (kept).
	b := newEnv(t, "licence-b")
	b.forge(1, "alice", "projet")
	bob := b.track(2, "bob", "projet", "")
	if _, err := b.st.SetRepositoryConventionsSince(t.Context(), bob.ID, "2027-01-04"); err != nil {
		t.Fatal(err)
	}
	f := mustParse(t, string(data))
	if s := f.Repositories[0].ConventionsSince; s == nil || *s != "2026-09-01" || f.Repositories[1].ConventionsSince != nil {
		t.Errorf("dates read back = %v, %v", s, f.Repositories[1].ConventionsSince)
	}
	p, err := configshare.Plan(t.Context(), b.ws, b.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if c := p.Repositories[0]; c.Action != configshare.ActionAdd || c.After.ConventionsSince != "2026-09-01" {
		t.Errorf("preview alice = %+v", c)
	}
	if c := p.Repositories[1]; c.Action != configshare.ActionUnchanged || c.After.ConventionsSince != "2027-01-04" {
		t.Errorf("preview bob = %+v", c)
	}
	if _, err := configshare.Apply(t.Context(), b.ws, b.st, b.eng, f, configshare.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	repos := b.repos()
	if got := repos["alice/projet"].ConventionsSince; got != "2026-09-01" {
		t.Errorf("imported alice = %q", got)
	}
	if got := repos["bob/projet"].ConventionsSince; got != "2027-01-04" {
		t.Errorf("bob = %q", got)
	}

	// "" = date of the workspace: update previewed, then applied.
	f = mustParse(t, doc("", `[{"repo": "bob/projet", "conventions_since": ""}]`))
	p, err = configshare.Plan(t.Context(), b.ws, b.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if c := p.Repositories[0]; c.Action != configshare.ActionUpdate || c.Before.ConventionsSince != "2027-01-04" ||
		c.After.ConventionsSince != "" {
		t.Errorf("preview \"\" = %+v", c)
	}
	if _, err := configshare.Apply(t.Context(), b.ws, b.st, b.eng, f, configshare.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := b.repos()["bob/projet"].ConventionsSince; got != "" {
		t.Errorf("bob after \"\": %q", got)
	}
}

func TestConventionsSinceParseRejects(t *testing.T) {
	for in, want := range map[string]string{
		`[{"repo": "alice/projet", "conventions_since": "01/09/2026"}]`: `repository #1 (alice/projet): "01/09/2026": date expected in the form YYYY-MM-DD`,
		`[{"repo": "alice/projet", "conventions_since": 20260901}]`:     "type",
	} {
		_, err := configshare.Parse(strings.NewReader(doc("", in)))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%s) = %v, want %q", in, err, want)
		}
	}
}
