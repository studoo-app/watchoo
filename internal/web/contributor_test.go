package web

// A contributor's page "/contributors/{id}" (0.5.0) and links to it and to the profiles.

import (
	"html"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// contributorIDs returns the internal identifier of the author contributors, by display name.
func (e *testEnv) contributorIDs() map[string]int64 {
	e.t.Helper()
	ranking, err := e.store.ContributorRanking(e.t.Context(), store.ContributorFilter{})
	if err != nil {
		e.t.Fatalf("ContributorRanking: %v", err)
	}
	out := make(map[string]int64, len(ranking))
	for _, c := range ranking {
		out[c.DisplayName()] = c.ID
	}
	return out
}

// hostileName is a booby-trapped contributor name: it must always appear escaped.
const hostileName = `<img src=x onerror="alert(2)">Eve`

// seedHostile adds to repository repoID (promo/name) two commits of a contributor with a
// booby-trapped name, with line statistics, and a commit of which he is only the committer (the
// author is "Zoé").
func (e *testEnv) seedHostile(repoID int64, name string) (eve, onlyCommitter store.Identity) {
	e.t.Helper()
	eve = store.Identity{Name: hostileName, Email: "eve@example.test"}
	zoe := store.Identity{Name: "Zoé", Email: "zoe@example.test"}
	onlyCommitter = store.Identity{Name: "Fred Committer", Email: "fred@example.test"}
	c1 := e.commitWrite(10, eve, 2*24*time.Hour, "travail d'Eve", "main")
	c1.Additions, c1.Deletions, c1.StatsKnown = 10, 2, true
	c2 := e.commitWrite(11, eve, 24*time.Hour, "<b>suite</b>", "main")
	c2.Additions, c2.StatsKnown = 5, true
	c3 := e.commitWrite(12, zoe, 12*time.Hour, "commit de Zoé", "main")
	c3.Committer = onlyCommitter
	e.applySync(repoID, name, c1, c2, c3)
	return eve, onlyCommitter
}

func TestContributorPage(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	aliceHead := e.commitWrite(3, store.Identity{}, 0, "", "main").SHA
	e.seedPulls(ids.Alice, "alice", aliceHead)
	alice := e.contributorIDs()["Alice Martin"]
	path := contributorPath(alice)

	w := e.get(path)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		"<title>Alice Martin · licence-projet · Watchoo</title>",
		`<h1 id="contributor-title"`, "@alice-gh", "1 known Git identity",
		// Profile link: the saved address, as a secondary link, new tab.
		`href="https://github.com/alice-gh"`, `aria-label="GitHub profile of alice-gh"`, `rel="noopener noreferrer"`,
		// Totals: 2 commits in 1 repository, lines unknown.
		`<div class="stat-value">2</div>`, "in 1 repository", `title="Lines unknown: not counted yet`,
		// Their repositories: link, state of the repository.
		`href="`+repoPath(ids.Alice)+`"`, "badge-success",
		// Their commits: escaped messages, links to the repository.
		html.EscapeString(xssMessage), "premier commit", "2 commits",
		`hx-get="`+path+`/commits?page=1"`,
		// Charts: per day and per repository (data table, link to the repository).
		`id="contributor-activity-data"`, `data-refresh-url="`+path+`"`, `id="contributor-repos-chart-data"`,
		"Commits per week and per repository", "in 1 repository since",
		// Pull requests matched by the account (identifier 10), escaped title.
		`id="contributor-pulls"`, html.EscapeString(xssTitle), "merged", "draft",
		// Fragments refreshed after a sync.
		`hx-trigger="sync-finished from:body"`,
	)
	assertNotContains(t, w, xssMessage, xssTitle, "Bob Durand", "travail de Bob")
	assertNoLeak(t, "contributor page", w.Body.String())
}

func TestContributorPageWithoutAccount(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	e.seedHostile(ids.Dave, "dave")
	contribs := e.contributorIDs()

	// Bob: Git identity only, neither profile nor pull requests.
	w := e.get(contributorPath(contribs["Bob Durand"]))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Bob Durand", "Not matched", "2 commits", "in 2 repositories", "travail de Bob")
	assertNotContains(t, w, "GitHub profile", "@")

	// Eve: booby-trapped name escaped everywhere, lines known.
	w = e.get(contributorPath(contribs[hostileName]))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, html.EscapeString(hostileName), `<span class="text-success">+15</span>`,
		`<span class="text-error">−2</span>`, html.EscapeString("<b>suite</b>"), "+10")
	assertNotContains(t, w, hostileName, "<b>suite</b>")

	// Carol: profile address refused (not https://): no link.
	w = e.get(contributorPath(contribs["Carol"]))
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, "javascript:", "GitHub profile", "evil.example.test")
	assertContains(t, w, "No pull request from this account in the database.")
	assertNoLeak(t, "contributor pages", w.Body.String())
}

// TestContributorPageCommitterOnly checks the page of a contributor who authored no commit (committer
// only).
func TestContributorPageCommitterOnly(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	e.seedHostile(ids.Dave, "dave")
	page, err := e.store.CommitPage(t.Context(), store.CommitFilter{RepoID: ids.Dave}, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	var fred int64
	for _, c := range page.Commits {
		if c.Message == "commit de Zoé" {
			fred = c.CommitterID
		}
	}
	if fred == 0 {
		t.Fatal("committer not found")
	}
	w := e.get(contributorPath(fred))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Fred Committer", "0 commits", "No commit they authored in the database.",
		"No commit since", "never")
}

func TestContributorNotFoundAndFragments(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	for _, target := range []string{"/contributors/999999", "/contributors/0", "/contributors/abc",
		"/contributors/999999/commits", "/contributors/1234567890123456789"} {
		if w := e.get(target); w.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", target, w.Code)
		}
	}

	alice := e.contributorIDs()["Alice Martin"]
	path := contributorPath(alice)
	for _, target := range []string{contributorOverviewID, contributorReposID, contributorPullsID} {
		r := e.request(http.MethodGet, path, nil)
		r.Header.Set("HX-Request", "true")
		r.Header.Set("HX-Target", target)
		w := e.do(r)
		assertStatus(t, w, http.StatusOK)
		body := w.Body.String()
		if !strings.Contains(body, `id="`+target+`"`) || strings.Contains(body, "<html") ||
			strings.Contains(body, "<script") {
			t.Errorf("unexpected fragment %s:\n%s", target, body)
		}
	}
	w := e.htmx(path + "/commits?page=7")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="contributor-commits"`, "premier commit")
	assertNotContains(t, w, "<html", "<script")
	// Without HTMX: the full page.
	if w := e.get(path + "/commits"); !strings.Contains(w.Body.String(), "<html") {
		t.Error("GET …/commits without HTMX: full page expected")
	}
}

// TestContributorPagination checks the paginated history of a contributor (50 per page).
func TestContributorPagination(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	who := store.Identity{Name: "Prolifique", Email: "prolifique@example.test"}
	var commits []store.CommitWrite
	for i := range 55 {
		c := e.commitWrite(100+i, who, time.Duration(i+1)*time.Hour, "commit prolifique", "main")
		commits = append(commits, c)
	}
	e.applySync(ids.Dave, "dave", commits...)
	path := contributorPath(e.contributorIDs()["Prolifique"])
	w := e.get(path + "?page=2")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "55 commits · page 2 of 2", `rel="prev"`, `href="`+path+`?page=1"`)
	if n := commitItems(w.Body.String()); n != 5 {
		t.Errorf("page 2: %d commits, want 5", n)
	}
}

// TestContributorLinks checks the links to a contributor's page (main link) and to their profile
// (secondary link) from the other pages.
func TestContributorLinks(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	contribs := e.contributorIDs()
	alice, bob := contributorPath(contribs["Alice Martin"]), contributorPath(contribs["Bob Durand"])
	profile := `href="https://github.com/alice-gh"`

	for _, target := range []string{"/contributors", "/", repoPath(ids.Alice)} {
		w := e.get(target)
		assertStatus(t, w, http.StatusOK)
		assertContains(t, w, `href="`+alice+`"`, `href="`+bob+`"`, profile, `aria-label="GitHub profile of alice-gh"`)
		assertNotContains(t, w, "javascript:", `GitHub profile of Bob`)
	}
	// History of a repository (fragment): linked author.
	w := e.htmx(repoPath(ids.Alice) + "/commits")
	assertContains(t, w, `href="`+alice+`"`, `href="`+bob+`"`, profile)
	// Data table of the per-contributor chart: linked name.
	w = e.get("/contributors")
	body := w.Body.String()
	table := body[strings.Index(body, `id="contributors-chart-table"`):]
	if !strings.Contains(table, `<a href="`+alice+`" class="link link-hover">Alice Martin</a>`) {
		t.Error("chart table: name without a link to the contributor's page")
	}
	// No address built by the interface: an unknown profile gives no link.
	assertNotContains(t, w, "https://github.com/carol-gh")
}

// TestContributorPageLogs checks that the log writes neither name, nor login, nor message, nor
// contributor identifier: only the route pattern.
func TestContributorPageLogs(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	alice := e.contributorIDs()["Alice Martin"]
	e.get(contributorPath(alice))
	e.htmx(contributorPath(alice) + "/commits")
	logs := e.logs.String()
	if !strings.Contains(logs, "/contributors/{id") {
		t.Errorf("log without the route pattern:\n%s", logs)
	}
	for _, s := range []string{"Alice Martin", "alice-gh", "premier commit", "xss", contributorPath(alice)} {
		if strings.Contains(logs, s) {
			t.Errorf("log: contains %q\n%s", s, logs)
		}
	}
	assertNoLeak(t, "log", logs)
}

// TestContributorPageAfterSync checks that a sync by the engine (fake provider) fills the profile
// address given by the provider and that the page shows it.
func TestContributorPageAfterSync(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	e.fake.AppendCommits(1, "main", 1, e.now.Add(-time.Hour), time.Minute)
	if _, err := e.engine.SyncRepo(t.Context(), ids.Alice, wsync.SyncOptions{}); err != nil {
		t.Fatalf("SyncRepo: %v", err)
	}
	id := e.contributorIDs()["Étudiant Test"]
	if id == 0 {
		t.Fatal("contributor of the fake provider missing")
	}
	w := e.get(contributorPath(id))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `href="https://github.example.test/etudiant-test"`)
}

func TestProfileURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/alice-gh":        "https://github.com/alice-gh",
		"https://forge.example.test/u/bob":   "https://forge.example.test/u/bob",
		"http://github.com/alice":            "",
		"javascript:alert(1)":                "",
		"https://user:pw@github.com/x":       "",
		`https://github.com/a"onmouseover=x`: "",
		"https:///chemin-sans-hote":          "",
		"":                                   "",
	} {
		if got := profileURL(in); got != want {
			t.Errorf("profileURL(%q) = %q, want %q", in, got, want)
		}
	}
	if got := contributorPath(42); got != "/contributors/42" {
		t.Errorf("contributorPath(42) = %q", got)
	}
}
