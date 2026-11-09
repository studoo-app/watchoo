package cli

// « watchoo conventions » (phases 8.1 and 8.2): rates of conforming commits for the trainer, text and
// --json, --commits / --failing / --contributor / --milestone, branches, pull requests, tags, milestones,
// releases, wording, footers, language, intervals between tags, disabled conventions, French. Neither a
// commit message nor a pull request title ever appears.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
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
	"Initialisation", "ajoute la connexion", "Merge branch",
	// Titles of the pull requests of seedProvider (phase 8.2): never written either.
	"Ajoute la page de connexion", "Corrige le build"}

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
	// Phase 8.2: branches (database fallback, PR head branches), pull requests (number, author, head branch
	// and codes, never the title), tags, milestones, releases, wording, footers, language, intervals.
	assertContains(t, "conventions (8.2)", out,
		"branches (database, tracked branches only): 0/3 conform, 0 to review",
		`no_slash (3): no "type/" prefix [blocking]: dev, fix (#2), login (#1)`,
		"pull requests: conventional titles open 0/1, merged 0/1; default titles 0",
		"#2 Alice Martin (fix): no_type", "#1 Alice Martin (login): no_type",
		"tags: 0/2 conform", "missing_component (2): fewer than three numbers [blocking]: v0.1, v1.0",
		"milestones: v1.0.0 nonconforming name (v1.0), v2.0.0 absent, v3.0.0 absent",
		"releases (information): nothing to report",
		`wording (warnings): past tense 0, "-ing" form 0, first person 0`,
		"footers: 0/4 commits, issue references 0/4", "subject language: fr 4, en 0, unknown 0",
		"CC1 between tags: unknown (no local clone)",
		"branches (database, tracked branches only): none to evaluate", "pull requests: none evaluated", "tags: none",
		"UP TO LAST TAG", "SINCE LAST TAG", "PR TITLES", "0/2", "Last tag: rates up to and since")

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
	// A report restricted to Bob shows neither Alice's nonconforming pull requests nor their head branches.
	bob := h.run("conventions", "--contributor", "Bob Durand").mustSucceed(t).stdout
	assertContains(t, "--contributor bob", bob, "Bob Durand", "pull requests: none evaluated")
	assertNotContains(t, "--contributor bob", bob, "Alice Martin", "#1 ", "#2 ", "login (#1)", "fix (#2)")
	var bobJSON struct {
		Repositories []struct {
			PullRequests struct{ Items []any } `json:"pull_requests"`
			Branches     struct{ Items []any } `json:"branches"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal([]byte(h.run("conventions", "--contributor", "Bob Durand", "--json").mustSucceed(t).stdout), &bobJSON); err != nil {
		t.Fatal(err)
	}
	for _, r := range bobJSON.Repositories {
		if len(r.PullRequests.Items) != 0 || len(r.Branches.Items) != 0 {
			t.Errorf("--contributor bob --json: other authors' pull requests or branches: %+v", r)
		}
	}

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
		"Contributeurs :", "Commits évalués :", "non conforme (no_type)",
		"branches (base, branches suivies seulement) : 0/3 conformes, 0 à revoir",
		"pull requests : titres conventionnels ouvertes 0/1, fusionnées 0/1 ; titres par défaut 0",
		"#1 Alice Martin (login) : no_type", "tags : 0/2 conformes",
		"jalons : v1.0.0 nom non conforme (v1.0), v2.0.0 absent", "releases (information) : rien à signaler",
		"formulation (avertissements) : passé 0", "pieds : 0/4 commits", "langue des sujets : fr 4, en 0, inconnue 0",
		"CC1 entre tags : inconnu (pas de clone local)", "JUSQU'AU DERNIER TAG")
	assertNotContains(t, "conventions (fr)", fr, conventionMessages...)
}

// --milestone: the review of a configured milestone, repository by repository; without a start date, "not
// evaluable"; an unknown milestone is a usage error.
func TestConventionsMilestone(t *testing.T) {
	h := conventionsWorkspace(t)
	out := h.run("conventions", "--milestone", "v1.0.0").mustSucceed(t).stdout
	assertContains(t, "--milestone (no since)", out, "Milestone review: repository indicators", "Milestone review v1.0.0",
		"alice/projet: not evaluable (missing start date)", "bob/projet: not evaluable (missing start date)")

	t.Setenv("WATCHOO_CONVENTIONS_SINCE", "2020-01-01")
	out = h.run("conventions", "--milestone", "v1.0.0").mustSucceed(t).stdout
	assertContains(t, "--milestone", out,
		"alice/projet: 0/4 criteria conform, 1 unknown",
		"CC1  unknown (no local clone)", "BR1  not conform — 0/3 branches conform",
		"SV1  not conform — v1.0 (missing_component)", "SV2  not conform — v1.0.0 nonconforming name (v1.0)",
		"bob/projet: 0/4 criteria conform, 3 unknown", "CC1  unknown (milestone tag missing, no due date)",
		"BR1  unknown (no branch to evaluate)", "SV2  not conform — v1.0.0 absent")
	assertNotContains(t, "--milestone", out, conventionMessages...)
	assertNotContains(t, "--milestone", out, "Contributors:", "Rates n/m")

	js := h.run("conventions", "alice/projet", "--milestone", "v1.0.0", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, js, "", jsonTopLevelKeys["conventions"]...)
	assertNotContains(t, "--milestone --json", js, conventionMessages...)
	var rep conventionsReport
	decodeStrict(t, js, &rep)
	m := rep.Milestone
	if m == nil || m.Name != "v1.0.0" || m.Due != nil || len(m.Repositories) != 1 || rep.Filters.Milestone != "v1.0.0" ||
		len(rep.Repositories) != 1 {
		t.Fatalf("--milestone --json: %+v", rep)
	}
	a := m.Repositories[0]
	if !a.Evaluable || a.Reason != nil || a.Status != "nonconforming_name" || a.Tag == nil || *a.Tag != "v1.0" ||
		len(a.TagCodes) != 1 || a.Interval != nil || a.Conform != 0 || a.Unknown != 1 || a.Total != 4 || len(a.Criteria) != 4 ||
		a.Criteria[0].ID != "CC1" || a.Criteria[0].Status != "unknown" || *a.Criteria[0].Reason != "no_clone" ||
		a.Criteria[1].Status != "nonconform" || a.Criteria[1].Reason != nil {
		t.Errorf("milestone of alice/projet: %+v", a)
	}
	// Without --milestone: the same keys, milestone null.
	plain := h.run("conventions", "--json").mustSucceed(t).stdout
	assertContains(t, "conventions --json", plain, `"milestone": null`, `"milestone": ""`)

	// A milestone that is not configured: usage error, translated.
	r := h.run("conventions", "--milestone", "v9.0.0")
	if r.code == 0 || !strings.Contains(r.stderr, `unknown milestone "v9.0.0"`) || !strings.Contains(r.stderr, "v1.0.0, v2.0.0, v3.0.0") {
		t.Errorf("--milestone v9.0.0: %+v", r)
	}
	defer i18n.SetProcessForTest(i18n.French)()
	if r := h.run("conventions", "--milestone", "1.0.0"); r.code == 0 || !strings.Contains(r.stderr, `jalon "1.0.0" inconnu`) {
		t.Errorf("--milestone 1.0.0 (fr): %+v", r)
	}
	fr := h.run("conventions", "--milestone", "v1.0.0").mustSucceed(t).stdout
	assertContains(t, "--milestone (fr)", fr, "Revue du jalon v1.0.0", "alice/projet : 0/4 critères conformes, 1 inconnus",
		"CC1  inconnu (pas de clone local)", "BR1  non conforme — 0/3 branches conformes")
}

// Local clones (no gh): branches read in the clone, pull requests and releases unknown, CC1 per interval
// between final tags, CC3 trend and the milestone review on the interval of the milestone.
func TestConventionsLocalClone(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	who := gittest.Author{Name: "Alice Martin", Email: "alice@example.test"}
	r.Commit("main", "a.txt", "1\n", "feat: ajoute la page d'accueil", who, testNow.Add(-50*time.Hour))
	r.Commit("main", "a.txt", "2\n", "Ajout du menu", who, testNow.Add(-40*time.Hour))
	r.Tag("v0.1.0", "main", "")
	r.Commit("main", "a.txt", "3\n", "fix: corrige le menu", who, testNow.Add(-30*time.Hour))
	r.Tag("v1.0.0", "main", "version 1")
	r.Tag("V2.0", "main", "")
	r.Commit("main", "a.txt", "4\n", "docs: added the readme\n\nCloses #4", who, testNow.Add(-20*time.Hour))
	r.Branch("feat/page-login", "main")
	r.Commit("feat/page-login", "b.txt", "1\n", "feat: ajoute la connexion", who, testNow.Add(-10*time.Hour))
	r.Branch("Feature_X", "main")

	h := newWorkspace(t)
	h.Client = authtest.NewLoggedOut()
	h.GitURL = srv.URL
	t.Setenv("WATCHOO_CONVENTIONS_ENABLED", "true")
	t.Setenv("WATCHOO_CONVENTIONS_SINCE", "2020-01-01")
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)

	out := h.run("conventions").mustSucceed(t).stdout
	assertContains(t, "conventions (clone)", out, "4/5", "clone",
		"branches (clone): 1/2 conform, 0 to review", "Feature_X",
		"pull requests: unknown (no data from the GitHub API)",
		"tags: 2/3 conform", "V2.0", "milestones: v1.0.0 present, v2.0.0 nonconforming name (V2.0), v3.0.0 absent",
		"releases (information): unknown (no data from the GitHub API)",
		"past tense 1", "footers: 1/5 commits, issue references 1/5",
		"CC1 between tags: start→v0.1.0 1/2 50%, v0.1.0→v1.0.0 1/1 100%, since v1.0.0 2/2 100%",
		"2/3 67%")
	assertNotContains(t, "conventions (clone)", out, "page d'accueil", "Ajout du menu", "corrige le menu", "added the readme",
		"ajoute la connexion", "Closes #4")

	js := h.run("conventions", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, js, "repositories.0.intervals.items.0", "from", "to", "known", "commits", "before_since", "missing")
	assertNotContains(t, "conventions --json (clone)", js, "page d'accueil", "Ajout du menu", "added the readme", "Closes #4")
	var rep conventionsReport
	decodeStrict(t, js, &rep)
	a := rep.Repositories[0]
	if a.Source != "clone" || a.Branches.Source != "clone" || a.Branches.Checked != 2 || a.PullRequests.Known ||
		*a.PullRequests.Reason != "no_api" || a.Tags.Releases.Known || !a.Intervals.Known || *a.Intervals.LastTag != "v1.0.0" ||
		len(a.Intervals.Items) != 2 || a.Intervals.Items[0].From != nil || a.Intervals.Items[1].Commits.Conform != 1 ||
		a.Intervals.SinceLastTag.Evaluated != 2 || a.Traits.Wording["past_tense"] != 1 || a.Traits.IssueReference != 1 {
		t.Errorf("conventions --json (clone): %+v", a)
	}
	c := rep.Contributors[0]
	if !c.Trend.Known || c.Trend.Repositories != 1 || c.Trend.Before.Evaluated != 3 || c.Trend.Before.Conform != 2 ||
		c.Trend.Since.Evaluated != 2 || c.Trend.Since.Conform != 2 {
		t.Errorf("trend: %+v", c.Trend)
	}

	review := h.run("conventions", "--milestone", "v1.0.0", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, review, "milestone.repositories.0.interval", "from", "to", "due", "commits", "before_since", "missing")
	ms := h.run("conventions", "--milestone", "v1.0.0").mustSucceed(t).stdout
	assertContains(t, "--milestone (clone)", ms, "alice/projet: 3/4 criteria conform, 0 unknown",
		"CC1  conform — v0.1.0→v1.0.0 1/1 100%", "BR1  not conform — 1/2 branches conform", "SV1  conform — v1.0.0",
		"SV2  conform — v1.0.0 present")
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
