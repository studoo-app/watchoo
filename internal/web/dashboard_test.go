package web

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// dashRepo builds a dummy card for the sorting tests.
func dashRepo(id int64, name string, st activity.State, lastCommit time.Time) DashboardRepo {
	return DashboardRepo{
		RepositorySummary: store.RepositorySummary{Repository: model.Repository{
			ID: id, Owner: "promo", Name: name, LastCommitAt: lastCommit,
		}},
		State: activity.Result{State: st},
	}
}

func TestSortDashboardRepos(t *testing.T) {
	day := func(n int) time.Time { return testNow.Add(-time.Duration(n) * 24 * time.Hour) }
	repos := []DashboardRepo{
		dashRepo(1, "zoe", activity.StateActive, day(1)),
		dashRepo(2, "yann", activity.StateNeverSynced, time.Time{}),
		dashRepo(3, "xavier", activity.StateFlemme, day(8)),
		dashRepo(4, "will", activity.StateDead, day(31)),
		dashRepo(5, "victor", activity.StateDead, day(90)),
		dashRepo(6, "ugo", activity.StateFlemme, day(20)),
		dashRepo(7, "Alba", activity.StateArchived, day(400)),
		dashRepo(8, "theo", activity.StateError, day(2)),
		dashRepo(9, "sam", activity.StateActive, day(5)),
		dashRepo(10, "bruno", activity.StateStopped, time.Time{}),
	}
	sortDashboardRepos(repos)
	var got []string
	for _, r := range repos {
		got = append(got, r.Name)
	}
	// Dead then Flemme (the oldest first), Active (same), in error, then the other gray ones by name.
	want := []string{"victor", "will", "ugo", "xavier", "sam", "zoe", "theo", "Alba", "bruno", "yann"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order %v, want %v", got, want)
	}
}

func TestSortDashboardReposTies(t *testing.T) {
	same := testNow.Add(-10 * 24 * time.Hour)
	repos := []DashboardRepo{
		dashRepo(3, "beta", activity.StateFlemme, same),
		dashRepo(2, "Alpha", activity.StateFlemme, same),
		dashRepo(1, "beta", activity.StateFlemme, same),
	}
	sortDashboardRepos(repos)
	if repos[0].ID != 2 || repos[1].ID != 1 || repos[2].ID != 3 {
		t.Errorf("ties: order %d,%d,%d, want 2,1,3", repos[0].ID, repos[1].ID, repos[2].ID)
	}
}

func TestCountStates(t *testing.T) {
	repos := []DashboardRepo{
		dashRepo(1, "a", activity.StateActive, testNow),
		dashRepo(2, "b", activity.StateActive, testNow),
		dashRepo(3, "c", activity.StateFlemme, testNow),
		dashRepo(4, "d", activity.StateDead, testNow),
		dashRepo(5, "e", activity.StateEmpty, testNow),
		dashRepo(6, "f", activity.StateError, testNow),
	}
	got := countStates(repos)
	want := DashboardCounts{Total: 6, Active: 2, Flemme: 1, Dead: 1, Gray: 2}
	if got != want {
		t.Errorf("counters %+v, want %+v", got, want)
	}
	if got := countStates(nil); got != (DashboardCounts{}) {
		t.Errorf("no repository: %+v", got)
	}
}

func TestDashboardAccent(t *testing.T) {
	for st, want := range map[activity.State]string{
		activity.StateActive: "border-success", activity.StateFlemme: "border-warning",
		activity.StateDead: "border-error", activity.StateNeverSynced: "border-base-300",
	} {
		if got := dashRepo(1, "a", st, testNow).Accent(); got != want {
			t.Errorf("%s: %q, want %q", st, got, want)
		}
	}
}

// section returns the part of body that starts at the element with identifier id (up to the end).
func section(t *testing.T, body, id string) string {
	t.Helper()
	i := strings.Index(body, `id="`+id+`"`)
	if i < 0 {
		t.Fatalf("element %s missing:\n%s", id, body)
	}
	return body[i:]
}

func TestDashboardPage(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()

	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		"<title>Dashboard · "+testWorkspace+" · Watchoo</title>",
		`aria-current="page">Dashboard`,
		"Alice Martin", "promo/bob", "promo/carol", "promo/dave",
		"badge-success", "badge-warning", "badge-error", "badge-neutral",
		"Slacking · 9 d", "Dead · 40 d", "Never synced",
		"Latest commits", "ajoute le README", "travail de Bob", "dernier signe de vie",
		"Bob Durand", "ago",
		// Gray badge: repository never synced, without commit.
		"No known commit: not synced yet.",
		// Fragments refreshed after a sync.
		`hx-get="/fragments/dashboard/repos" hx-include="#dashboard-filters" hx-trigger="sync-finished from:body"`,
		`hx-get="/fragments/dashboard/feed" hx-trigger="sync-finished from:body"`,
		// Sync button, protected by the CSRF token.
		`action="/sync"`, `name="csrf_token"`,
		// Local time in title, ISO in datetime.
		`datetime="2026-10-26T10:00:00Z"`, `title="Oct 26, 2026 10:00"`,
		`<html lang="en">`,
	)
	// Commit message: always escaped.
	assertContains(t, w, "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt; corrige le bug")
	assertNotContains(t, w, xssMessage, "<script>alert", "Page en construction", "evil.example.test")

	// Counters: 1 Active, 1 Flemme, 1 Dead, 1 gray out of 4 repositories.
	repos := section(t, w.Body.String(), "dashboard-repos")
	for _, stat := range []string{
		`text-success">1<`, `text-warning">1<`, `text-error">1<`, `text-base-content/60">1<`, "out of 4 repositories",
	} {
		if !strings.Contains(repos, stat) {
			t.Errorf("counter %q missing", stat)
		}
	}

	// Order of the cards: Dead (Carol), Flemme (Bob), Active (Alice), then gray (Dave).
	feedAt := strings.Index(repos, `id="dashboard-feed"`)
	cards := repos[:feedAt]
	last := -1
	for _, id := range []int64{ids.Carol, ids.Bob, ids.Alice, ids.Dave} {
		i := strings.Index(cards, fmt.Sprintf(`href="%s"`, repoPath(id)))
		if i < 0 || i < last {
			t.Fatalf("card of repository %d missing or misplaced (%d after %d)", id, i, last)
		}
		last = i
	}

	// Feed: from the most recent to the oldest, short SHA linked to the commit, link to the repository.
	feed := section(t, w.Body.String(), "dashboard-feed")
	xss := strings.Index(feed, "corrige le bug")
	readme := strings.Index(feed, "ajoute le README")
	carol := strings.Index(feed, "dernier signe de vie")
	if xss < 0 || readme < xss || carol < readme {
		t.Errorf("wrong feed order (%d, %d, %d)", xss, readme, carol)
	}
	if !strings.Contains(feed, `rel="noopener noreferrer"`) || !strings.Contains(feed, "https://github.com/promo/x/commit/") {
		t.Error("link to the commit missing from the feed")
	}
	if !strings.Contains(feed, fmt.Sprintf(`href="%s"`, repoPath(ids.Carol))) {
		t.Error("link to the repository missing from the feed")
	}
}

func TestDashboardEmpty(t *testing.T) {
	e := newTestEnv(t)
	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "No tracked repository", `href="/settings/repos"`, "No commit yet", "out of 0 repositories<")
}

func TestDashboardFragments(t *testing.T) {
	e := newTestEnv(t)
	e.seed()

	w := e.htmx("/fragments/dashboard/repos")
	assertStatus(t, w, http.StatusOK)
	body := strings.TrimSpace(w.Body.String())
	if !strings.HasPrefix(body, `<section id="dashboard-repos"`) {
		t.Errorf("repositories fragment: unexpected start:\n%s", body)
	}
	assertContains(t, w, "Alice Martin", "Dead · 40 d", `hx-trigger="sync-finished from:body"`)
	assertNotContains(t, w, "<html", "<nav", `id="dashboard-feed"`, xssMessage)
	if !strings.Contains(w.Header().Get("Vary"), "HX-Request") {
		t.Errorf("Vary without HX-Request: %q", w.Header().Get("Vary"))
	}

	w = e.htmx("/fragments/dashboard/feed")
	assertStatus(t, w, http.StatusOK)
	body = strings.TrimSpace(w.Body.String())
	if !strings.HasPrefix(body, `<section id="dashboard-feed"`) {
		t.Errorf("feed fragment: unexpected start:\n%s", body)
	}
	assertContains(t, w, "&lt;script&gt;alert", "travail de Bob")
	assertNotContains(t, w, "<html", `id="dashboard-repos"`, xssMessage)
}

func TestDashboardFeedLimit(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	var commits []store.CommitWrite
	for n := 6; n < 6+dashboardFeedSize+5; n++ {
		commits = append(commits, e.commitWrite(n, store.Identity{Name: "Dave", Email: "etudiant@example.test"},
			time.Duration(n)*time.Minute, fmt.Sprintf("commit numéro %d", n), "main"))
	}
	e.applySync(ids.Dave, "dave", commits...)

	w := e.htmx("/fragments/dashboard/feed")
	assertStatus(t, w, http.StatusOK)
	if got := strings.Count(w.Body.String(), `class="list-row`); got != dashboardFeedSize {
		t.Errorf("%d commits in the feed, want %d", got, dashboardFeedSize)
	}
	// The most recent first: commit 6 (6 min ago) is there, 26 (26 min ago) is not.
	assertContains(t, w, "commit numéro 6<", "commit numéro 25<")
	assertNotContains(t, w, "commit numéro 26<", "dernier signe de vie")
}

func TestDashboardUnsafeCommitURL(t *testing.T) {
	e := newTestEnv(t)
	id := e.addRepo(1, "eve", "")
	c := e.commitWrite(1, store.Identity{Name: "Eve", Email: "etudiant@example.test"}, time.Hour, "piège", "main")
	c.URL = "javascript:alert(1)"
	e.applySync(id, "eve", c)

	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, "javascript:alert")
}

func TestDashboardGetOnlyRoutes(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	for _, target := range []string{"/", "/fragments/dashboard/repos", "/fragments/dashboard/feed"} {
		w := e.post(target, nil, false)
		assertStatus(t, w, http.StatusMethodNotAllowed)
	}
}
