package web

// Conventions (1.7.0, phase 8.1): section of the repository page and line of the contributor page, only
// when the conventions are enabled; never a commit message.

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
)

func withConventions() envOption { return func(e *testEnv) { e.cfg.Conventions.Enabled = true } }

// conventionsEnv returns an environment with the conventions enabled and the seeded repositories, plus,
// in alice's repository, a conforming commit and a merge (excluded) by Alice.
func conventionsEnv(t *testing.T, opts ...envOption) (*testEnv, seededRepos) {
	t.Helper()
	e := newTestEnv(t, append([]envOption{withConventions()}, opts...)...)
	ids := e.seed()
	alice := store.Identity{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh", ProviderUserID: 10}
	two := 2
	merge := e.commitWrite(7, alice, 6*time.Hour, "Merge branch 'dev'", "main")
	merge.ParentCount = &two
	e.applySync(ids.Alice, "alice", e.commitWrite(6, alice, 12*time.Hour, "feat: ajoute la connexion", "main"), merge)
	return e, ids
}

// Messages of the seeded commits: never shown by the conventions.
var conventionMessages = []string{"premier commit", "ajoute le README", "corrige le bug", "ajoute la connexion",
	"Merge branch", "travail de Bob"}

func TestRepoConventions(t *testing.T) {
	e, ids := conventionsEnv(t)
	path := "/repos/" + strconv.FormatInt(ids.Alice, 10)
	w := e.get(path)
	assertStatus(t, w, http.StatusOK)
	// premier commit (forbidden), README and the XSS message (no type); feat: conforming; the merge is excluded.
	assertContains(t, w, `id="repo-conventions"`, "Conforming commits", "1/4", "25%", "Default branch (main)",
		"Excluded commits, counted apart: 1 (merges 1)", "Issues by code", "<code>no_type</code>",
		"no &#34;type:&#34; header", "blocking", "Latest non-conforming commits (3 of 3)",
		shortSHA(providertest.SHA("commit-"+string(rune('a'+1)))), "may include rewritten commits", "not a grade")
	section := conventionsSection(t, w.Body.String())
	for _, m := range conventionMessages {
		if strings.Contains(section, m) {
			t.Errorf("commit message %q in the conventions section", m)
		}
	}

	// HTMX fragment after a sync.
	w = e.htmxWith(path, map[string]string{"HX-Target": repoConventionsID})
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="repo-conventions"`, "1/4")
	assertNotContains(t, w, `id="repo-commits"`, `id="repo-overview"`)

	// A repository never synced: unknown, not 0%.
	assertContains(t, e.get("/repos/"+strconv.FormatInt(ids.Dave, 10)), `id="repo-conventions"`,
		"Unknown: the repository has not been synced yet.")
	// Everything before the start date: a dedicated text, not 0/0.
	f, ids2 := conventionsEnv(t, func(e *testEnv) { e.cfg.Conventions.Since = "2099-01-01" })
	assertContains(t, f.get("/repos/"+strconv.FormatInt(ids2.Bob, 10)), "No commit evaluated",
		"Commits written before the start date, not evaluated: 1", "Start date: ", "(workspace)")

	// French.
	fr, ids3 := conventionsEnv(t, withLang("fr"))
	assertContains(t, fr.get("/repos/"+strconv.FormatInt(ids3.Alice, 10)), "Commits conformes", "25 %",
		"Branche par défaut (main)", "Commits exclus, comptés à part : 1 (fusions 1)", "bloquant",
		"peut inclure des commits réécrits")
}

// conventionsSection returns the HTML of the conventions section of a repository page.
func conventionsSection(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, `<section id="repo-conventions"`)
	if start < 0 {
		t.Fatal("no conventions section")
	}
	end := strings.Index(body[start:], "</section>")
	return body[start : start+end]
}

func TestRepoConventionsDisabled(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	path := "/repos/" + strconv.FormatInt(ids.Alice, 10)
	assertNotContains(t, e.get(path), `id="repo-conventions"`, "Conforming commits")
	// The fragment targeted while disabled: nothing is written.
	w := e.htmxWith(path, map[string]string{"HX-Target": repoConventionsID})
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, `id="repo-conventions"`)
	// The contributor page has no conventions line either.
	assertNotContains(t, e.get("/contributors/"+strconv.FormatInt(e.contributorIDs()["Alice Martin"], 10)),
		`id="contributor-conventions"`)
}

func TestContributorConventions(t *testing.T) {
	e, _ := conventionsEnv(t)
	alice := e.contributorIDs()["Alice Martin"]
	w := e.get("/contributors/" + strconv.FormatInt(alice, 10))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="contributor-conventions"`, "Conforming commits 1/3 (33%)", "<code>forbidden</code> 1, <code>no_type</code> 1",
		"1 excluded", "a rate, not a grade")
	// The history fragment does not compute it.
	assertNotContains(t, e.htmx("/contributors/"+strconv.FormatInt(alice, 10)+"/commits"), `id="contributor-conventions"`)

	// A contributor without evaluated commit.
	fut, _ := conventionsEnv(t, func(e *testEnv) { e.cfg.Conventions.Since = "2099-01-01" })
	assertContains(t, fut.get("/contributors/"+strconv.FormatInt(fut.contributorIDs()["Bob Durand"], 10)),
		"Conforming commits: no commit evaluated")

	fr, _ := conventionsEnv(t, withLang("fr"))
	assertContains(t, fr.get("/contributors/"+strconv.FormatInt(fr.contributorIDs()["Alice Martin"], 10)),
		"Commits conformes 1/3 (33 %)", "un taux, pas une note")
}

// Every issue code, exclusion reason, severity and origin of the start date has its label, in both
// languages (keys built at run time, which TestTemplateKeysInCatalog does not see).
func TestConventionsLabels(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.English, i18n.French} {
		tr := i18n.For(lang)
		var keys []string
		for _, c := range conventions.AllCodes {
			keys = append(keys, "web.conv.code."+c, "cli.conv.code."+c)
		}
		for _, x := range conventions.AllExclusions {
			keys = append(keys, "web.conv.exclusion."+x, "cli.conv.exclusion."+x)
		}
		for _, s := range []conventions.Severity{conventions.SeverityBlocking, conventions.SeverityWarning} {
			keys = append(keys, "web.conv.severity."+s.String(), "cli.conv.severity."+s.String())
		}
		for _, o := range []string{conventions.SinceOriginWorkspace, conventions.SinceOriginRepository} {
			keys = append(keys, "web.conv.since_origin."+o, "cli.conv.since_"+o)
		}
		for _, s := range []string{conventions.SourceClone, conventions.SourceDatabase} {
			keys = append(keys, "cli.conv.source."+s)
		}
		for _, key := range keys {
			if tr.T(key) == key {
				t.Errorf("%s: no label for %s", lang, key)
			}
		}
	}
	// The two languages differ for the codes (no untranslated copy).
	en, fr := i18n.For(i18n.English), i18n.For(i18n.French)
	for _, c := range conventions.AllCodes {
		if key := "web.conv.code." + c; en.T(key) == fr.T(key) {
			t.Errorf("%s: same label in English and French", key)
		}
	}
}
