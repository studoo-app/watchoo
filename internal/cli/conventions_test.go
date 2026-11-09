package cli

// « watchoo conventions » (phase 8.1): rates of conforming commits for the trainer, text and --json,
// --commits / --failing / --contributor, disabled conventions, French. Commit messages never appear.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
)

// conventionCommitMsg is the message of the only conforming commit of the fixture.
const conventionCommitMsg = "feat: ajoute la connexion"

// conventionsWorkspace returns a synced workspace with the conventions enabled. alice/projet: on main,
// a1 and a2 (no type), then c1 (conforming) and m1 (merge of dev, excluded); on dev, a3 (no type),
// also on main through the merge.
// bob/projet: b1 (no type). The CLI tests have no local clone: database fallback.
func conventionsWorkspace(t *testing.T) *harness {
	t.Helper()
	h := newWorkspace(t)
	t.Setenv("WATCHOO_CONVENTIONS_ENABLED", "true")
	f := h.Provider
	seedProvider(f)
	c1 := addCommits(f, aliceRepoID, "main", providertest.SHA("a2"),
		fakeCommit{"c1", alicePerson, 90 * time.Minute, conventionCommitMsg})
	at := testNow.Add(-30 * time.Minute)
	m1 := provider.Commit{SHA: providertest.SHA("m1"), Message: "Merge branch 'dev'", Author: alicePerson, Committer: alicePerson,
		AuthoredAt: at, CommittedAt: at, Parents: []string{c1, providertest.SHA("a3")}}
	f.AddCommits(aliceRepoID, m1)
	f.SetBranch(aliceRepoID, "main", m1.SHA)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)
	return h
}

// conventionMessages are the commit messages of the fixture: never written by the command.
var conventionMessages = []string{"Premier jet", "Ajoute les tests", "Détail du commit", "Essai sur dev",
	"Initialisation", "ajoute la connexion", "Merge branch"}

func TestConventionsText(t *testing.T) {
	h := conventionsWorkspace(t)
	out := h.run("conventions").mustSucceed(t).stdout
	assertContains(t, "conventions", out,
		"Rates n/m of the evaluated commits, not a grade",
		"alice/projet", "1/4", "25%", "1/3", "33%", "bob/projet", "0/1", "0%", "database*",
		"may include rewritten commits",
		"excluded: merges 1",
		"no_type (3): no \"type:\" header [blocking]",
		"Contributors:", "Alice Martin", "1/3", "Bob Durand", "0/2", "no_type 2")
	assertNotContains(t, "conventions", out, conventionMessages...)
	assertNotContains(t, "conventions", out, "Evaluated commits:")

	// --commits: the evaluated commits, never the merge; --failing: only the non conforming ones.
	all := h.run("conventions", "alice/projet", "--commits").mustSucceed(t).stdout
	assertContains(t, "--commits", all, "Evaluated commits:", provider.ShortSHA(providertest.SHA("c1")), "conform",
		provider.ShortSHA(providertest.SHA("a1")), "not conform (no_type)")
	assertNotContains(t, "--commits", all, provider.ShortSHA(providertest.SHA("m1")), "bob/projet")
	assertNotContains(t, "--commits", all, conventionMessages...)
	failing := h.run("conventions", "--failing").mustSucceed(t).stdout
	assertContains(t, "--failing", failing, provider.ShortSHA(providertest.SHA("a1")), provider.ShortSHA(providertest.SHA("b1")))
	assertNotContains(t, "--failing", failing, provider.ShortSHA(providertest.SHA("c1")))
	assertNotContains(t, "--failing", failing, conventionMessages...)

	// --contributor: one author, matched by login, name or e-mail.
	alice := h.run("conventions", "--contributor", "alice-gh").mustSucceed(t).stdout
	assertContains(t, "--contributor", alice, "Alice Martin", "1/3")
	assertNotContains(t, "--contributor", alice, "Bob Durand")

	// Unknown repository: error that points to repo list.
	if r := h.run("conventions", "carol/projet"); r.code == 0 || !strings.Contains(r.stderr, "watchoo repo list") {
		t.Errorf("unknown repository: %+v", r)
	}
	if r := h.run("conventions", "a/b", "c/d"); r.code == 0 {
		t.Errorf("two arguments accepted: %+v", r)
	}

	// French.
	defer i18n.SetProcessForTest(i18n.French)()
	fr := h.run("conventions", "--failing").mustSucceed(t).stdout
	assertContains(t, "conventions (fr)", fr, "Taux n/m des commits évalués, pas une note", "25 %", "base*",
		"peut inclure des commits réécrits", "exclus : fusions 1", "no_type (1) : pas d'en-tête « type: » [bloquant]",
		"Contributeurs :", "Commits évalués :", "non conforme (no_type)")
	assertNotContains(t, "conventions (fr)", fr, conventionMessages...)
}

func TestConventionsJSON(t *testing.T) {
	h := conventionsWorkspace(t)
	out := h.run("conventions", "--failing", "--json").mustSucceed(t).stdout
	assertSingleJSONObject(t, "conventions --json", out)
	assertJSONKeys(t, out, "", jsonTopLevelKeys["conventions"]...)
	assertNotContains(t, "conventions --json", out, conventionMessages...)
	var rep conventionsReport
	decodeStrict(t, out, &rep)
	if !rep.Enabled || rep.RulesVersion != 1 || len(rep.Repositories) != 2 || !rep.Filters.Commits || !rep.Filters.Failing {
		t.Fatalf("conventions --json: %+v", rep)
	}
	a := rep.Repositories[0]
	if a.Repository.FullName != "alice/projet" || a.Source != "database" || a.Since != nil || a.SinceOrigin != nil ||
		a.Commits.Evaluated != 4 || a.Commits.Conform != 1 || a.Commits.Rate == nil || *a.Commits.Rate != 0.25 ||
		a.Commits.Excluded["merge"] != 1 || a.DefaultBranch.Name != "main" || a.DefaultBranch.Evaluated != 4 {
		t.Errorf("alice/projet: %+v", a)
	}
	if len(a.Commits.Issues) != 1 || a.Commits.Issues[0].Code != "no_type" || a.Commits.Issues[0].Severity != "blocking" ||
		a.Commits.Issues[0].Count != 3 || len(a.Commits.Issues[0].SHAs) != 3 {
		t.Errorf("issues: %+v", a.Commits.Issues)
	}
	if len(rep.Commits) != 4 {
		t.Errorf("failing commits: %+v", rep.Commits)
	}
	for _, c := range rep.Commits {
		if c.Conform || c.ShortSHA != provider.ShortSHA(c.SHA) || len(c.Codes) == 0 || c.Date == nil {
			t.Errorf("failing commit: %+v", c)
		}
	}
	if len(rep.Contributors) != 2 || rep.Contributors[0].Author.Login != "alice-gh" {
		t.Errorf("contributors: %+v", rep.Contributors)
	}

	// Without --commits: the same keys, an empty list. A repository's own start date.
	h.run("repo", "add", "bob/projet", "--conventions-since", "2026-11-01").mustSucceed(t)
	out = h.run("conventions", "bob/projet", "--json").mustSucceed(t).stdout
	assertContains(t, "conventions bob --json", out, `"commits": []`, `"since": "2026-11-01"`, `"since_origin": "repository"`,
		`"before_since": 1`, `"rate": null`)
	var bob conventionsReport
	if err := json.Unmarshal([]byte(out), &bob); err != nil || len(bob.Repositories) != 1 || bob.Filters.Repository != "bob/projet" {
		t.Errorf("bob/projet: %+v, %v", bob, err)
	}
}

func TestConventionsDisabled(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)
	out := h.run("conventions").mustSucceed(t).stdout
	assertContains(t, "conventions (disabled)", out, "Conventions are disabled in this workspace")
	js := h.run("conventions", "--commits", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, js, "", jsonTopLevelKeys["conventions"]...)
	assertContains(t, "conventions --json (disabled)", js, `"enabled": false`, `"repositories": []`,
		`"contributors": []`, `"commits": []`)
	defer i18n.SetProcessForTest(i18n.French)()
	assertContains(t, "conventions (disabled, fr)", h.run("conventions").mustSucceed(t).stdout,
		"Les conventions sont désactivées dans cet espace")
}

// No tracked repository: a dedicated line, not an empty table.
func TestConventionsNoRepository(t *testing.T) {
	h := newWorkspace(t)
	t.Setenv("WATCHOO_CONVENTIONS_ENABLED", "true")
	assertContains(t, "conventions (empty)", h.run("conventions").mustSucceed(t).stdout, "No tracked repository.")
}
