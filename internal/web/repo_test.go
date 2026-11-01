package web

// Tests of the repository detail page "/repos/{id}" and of its fragments (repo agent).

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/store"
)

// htmxWith sends GET target as HTMX, with additional headers (HX-Target, HX-Trigger…).
func (e *testEnv) htmxWith(target string, headers map[string]string) *httptest.ResponseRecorder {
	e.t.Helper()
	r := e.request(http.MethodGet, target, nil)
	r.Header.Set("HX-Request", "true")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return e.do(r)
}

var chartDataRE = regexp.MustCompile(`(?s)<script type="application/json" id="activity-data">(.*?)</script>`)

// chartData extracts and decodes the JSON series of the chart of a detail page.
func chartData(t *testing.T, w *httptest.ResponseRecorder) []activity.DayCount {
	t.Helper()
	m := chartDataRE.FindStringSubmatch(w.Body.String())
	if m == nil {
		t.Fatalf("page without data #activity-data:\n%s", w.Body.String())
	}
	var days []activity.DayCount
	if err := json.Unmarshal([]byte(m[1]), &days); err != nil {
		t.Fatalf("unreadable chart data %q: %v", m[1], err)
	}
	return days
}

// countOn returns the number of commits of day date in the series (-1 if the day is missing).
func countOn(days []activity.DayCount, date string) int {
	for _, d := range days {
		if d.Date == date {
			return d.Count
		}
	}
	return -1
}

// commitItems counts the commits listed in the history (one SHA link per commit).
func commitItems(body string) int {
	return strings.Count(body, `title="View the commit on GitHub"`)
}

func TestRepoPage(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	path := repoPath(ids.Alice)

	w := e.get(path)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		"<title>Alice Martin · licence-projet · Watchoo</title>",
		`<h1 id="repo-title"`, "Alice Martin", `href="https://github.com/promo/alice"`, "promo/alice",
		"badge-success", ">Active<",
		// escaping of the commit message (header and history)
		html.EscapeString(xssMessage),
		// sync button wired to POST /repos/{id}/sync
		`action="`+path+`/sync"`, `hx-post="`+path+`/sync"`, `name="csrf_token"`,
		// chart: canvas, JSON data, Chart.js loaded on this page
		`<canvas id="activity-chart"`, `data-refresh-url="`+path+`"`,
		`<script src="/static/vendor/chart-4.6.1.umd.min.js" defer></script>`,
		"Activity over 90 days", "3 commits",
		// contributors: GitHub avatar without Referer, initial otherwise
		"Contributors", "Bob Durand", `src="https://avatars.githubusercontent.com/u/10?v=4"`,
		`referrerpolicy="no-referrer"`, `href="https://github.com/alice-gh"`,
		// history and filters
		`id="repo-commits"`, `id="repo-filters"`, `<option value="dev">dev</option>`,
		`<option value="alice-gh">Alice Martin (alice-gh)</option>`, `<option value="Bob Durand">Bob Durand</option>`,
		"ajoute le README", "premier commit",
		// refresh of the fragments after a sync
		`hx-trigger="sync-finished from:body"`,
	)
	assertNotContains(t, w, xssMessage, "<script>alert", `class="join"`)
	if n := commitItems(w.Body.String()); n != 3 {
		t.Errorf("%d commits listed, want 3", n)
	}
	// No inline script, no event attribute.
	assertNotContains(t, w, " onclick=", " onchange=", "hx-on")
	for _, m := range regexp.MustCompile(`<script[^>]*>`).FindAllString(w.Body.String(), -1) {
		if !strings.Contains(m, " src=") && !strings.Contains(m, `type="application/json"`) {
			t.Errorf("inline script: %s", m)
		}
	}

	days := chartData(t, w)
	if len(days) != activity.ChartDays {
		t.Fatalf("%d days in the chart, want %d", len(days), activity.ChartDays)
	}
	if days[len(days)-1].Date != "2026-10-27" || days[0].Date != "2026-07-30" {
		t.Errorf("chart bounds %s … %s", days[0].Date, days[len(days)-1].Date)
	}
	for date, want := range map[string]int{"2026-10-26": 1, "2026-10-24": 1, "2026-09-27": 1, "2026-10-27": 0} {
		if got := countOn(days, date); got != want {
			t.Errorf("day %s: %d commits, want %d", date, got, want)
		}
	}
}

func TestRepoPageStates(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()

	w := e.get(repoPath(ids.Carol))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "badge-error", "Dead · 40 d", "dernier signe de vie")
	// Avatar outside avatars.githubusercontent.com: never loaded, initial instead.
	assertNotContains(t, w, "evil.example.test")
	assertContains(t, w, "avatar-placeholder")

	w = e.get(repoPath(ids.Bob))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "badge-warning", "Slacking · 9 d")

	w = e.get(repoPath(ids.Dave))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "badge-neutral", "Never synced", "No commit in the database for this repository",
		"0 commits", ">never<")
	if days := chartData(t, w); len(days) != activity.ChartDays {
		t.Errorf("%d days for an empty repository, want %d", len(days), activity.ChartDays)
	}

	// Tracking stopped: gray badge, no sync button.
	if err := e.store.SetRepositoryEnabled(t.Context(), ids.Bob, false); err != nil {
		t.Fatalf("SetRepositoryEnabled: %v", err)
	}
	w = e.get(repoPath(ids.Bob))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Tracking stopped", "badge-neutral")
	assertNotContains(t, w, "Sync this repository")
}

func TestRepoNotFound(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	for _, target := range []string{"/repos/9999", "/repos/9999/commits", "/repos/0", "/repos/abc",
		"/repos/12345678901234567890", "/repos/1/unknown"} {
		w := e.get(target)
		assertStatus(t, w, http.StatusNotFound)
		assertContains(t, w, "Page not found")
	}
	w := e.htmx("/repos/9999/commits")
	assertStatus(t, w, http.StatusNotFound)
	assertContains(t, w, `role="alert"`)
	assertNotContains(t, w, "<html")
}

// seedMany creates a repository "promo/gros" of n commits by Alice, the i-th dated testNow - (i+1) h.
func (e *testEnv) seedMany(n int) int64 {
	e.t.Helper()
	id := e.addRepo(50, "gros", "")
	alice := store.Identity{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh", ProviderUserID: 10}
	commits := make([]store.CommitWrite, 0, n)
	for i := n - 1; i >= 0; i-- {
		c := e.commitWrite(100+i, alice, time.Duration(i+1)*time.Hour, fmt.Sprintf("commit n°%03d", i), "main")
		commits = append(commits, c)
	}
	e.applySync(id, "gros", commits...)
	return id
}

func TestRepoPagination(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedMany(120)
	path := repoPath(id)

	w := e.get(path)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "120 commits", "page 1 of 3", "commit n°000", "commit n°049",
		`href="`+path+`?page=2"`, `hx-get="`+path+`/commits?page=2"`, `hx-push-url="`+path+`?page=2"`)
	assertNotContains(t, w, "commit n°050", `rel="prev"`)
	if n := commitItems(w.Body.String()); n != repoCommitsPerPage {
		t.Errorf("%d commits on page 1, want %d", n, repoCommitsPerPage)
	}

	w = e.get(path + "?page=2")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "page 2 of 3", "commit n°050", "commit n°099",
		`href="`+path+`?page=1"`, `href="`+path+`?page=3"`, `rel="prev"`, `rel="next"`)
	assertNotContains(t, w, "commit n°049<", "commit n°100")

	// HTMX fragment: the last page alone, without the document.
	w = e.htmx(path + "/commits?page=3")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<section id="repo-commits"`, "page 3 of 3", "commit n°119",
		`hx-get="`+path+`/commits?page=3"`)
	assertNotContains(t, w, "<html", "<title>", `id="activity-data"`, `rel="next"`)
	if n := commitItems(w.Body.String()); n != 20 {
		t.Errorf("%d commits on page 3, want 20", n)
	}
	if w.Header().Get("HX-Push-Url") != "" {
		t.Errorf("HX-Push-Url %q for a pagination (the URL comes from hx-push-url)", w.Header().Get("HX-Push-Url"))
	}

	// Page out of bounds or invalid: brought back to the last or to the first one.
	assertContains(t, e.htmx(path+"/commits?page=999"), "page 3 of 3")
	assertContains(t, e.htmx(path+"/commits?page=-4"), "page 1 of 3")
	assertContains(t, e.htmx(path+"/commits?page=abc"), "page 1 of 3")

	// Without HTMX (reloaded address), the fragment gives the full page.
	w = e.get(path + "/commits?page=2")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "<html", "page 2 of 3", `id="activity-data"`)
}

func TestRepoFilters(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	path := repoPath(ids.Alice)

	w := e.get(path + "?branch=dev")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "ajoute le README", "1 commit matching the filters",
		`<option value="dev" selected>dev</option>`, "Clear the filters")
	assertNotContains(t, w, "premier commit")

	w = e.htmx(path + "/commits?author=alice-gh")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "2 commits matching the filters", "premier commit",
		`<option value="alice-gh" selected>`, `hx-get="`+path+`/commits?author=alice-gh&amp;page=1"`)
	assertNotContains(t, w, "ajoute le README", "<html")

	// Author without account: filter by name.
	w = e.htmx(path + "/commits?author=" + "Bob+Durand&branch=main")
	assertContains(t, w, "ajoute le README", `<option value="Bob Durand" selected>`, `<option value="main" selected>`)
	assertNotContains(t, w, "premier commit")

	// Unknown filters: kept shown, no result; values encoded in the URLs.
	w = e.htmx(path + "/commits?branch=" + "feat%2Fa%26b&author=x%22y")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "No commit matches these filters", "feat/a&amp;b (unknown)", "x&#34;y (unknown)",
		`hx-get="`+path+`/commits?author=x%22y&amp;branch=feat%2Fa%26b&amp;page=1"`)

	// Filter by e-mail: ignored (no address in the page or in the URLs).
	w = e.get(path + "?author=alice@example.test")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Filtering by e-mail address is not offered here", "3 commits")

	// Filter change through the form: the browser address follows (full page).
	w = e.htmxWith(path+"/commits?branch=dev&author=", map[string]string{"HX-Trigger": "repo-filters"})
	assertStatus(t, w, http.StatusOK)
	if got, want := w.Header().Get("HX-Push-Url"), path+"?branch=dev"; got != want {
		t.Errorf("HX-Push-Url = %q, want %q", got, want)
	}
	// Refresh after a sync: no history.
	w = e.htmxWith(path+"/commits?branch=dev", map[string]string{"HX-Trigger": "repo-commits"})
	if got := w.Header().Get("HX-Push-Url"); got != "" {
		t.Errorf("HX-Push-Url = %q for a refresh", got)
	}
}

func TestRepoFiltersKeepPagination(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedMany(60)
	path := repoPath(id)
	w := e.get(path + "?branch=main&author=alice-gh")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "page 1 of 2",
		`href="`+path+`?author=alice-gh&amp;branch=main&amp;page=2"`,
		`hx-get="`+path+`/commits?author=alice-gh&amp;branch=main&amp;page=2"`)
}

func TestRepoFragments(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	path := repoPath(ids.Alice)

	w := e.htmxWith(path, map[string]string{"HX-Target": "repo-overview"})
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<section id="repo-overview"`, "Alice Martin", "badge-success", "Sync this repository",
		html.EscapeString(xssMessage))
	assertNotContains(t, w, "<html", `id="repo-commits"`, `id="activity-data"`, xssMessage)

	w = e.htmxWith(path, map[string]string{"HX-Target": "repo-contributors"})
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<section id="repo-contributors"`, "Bob Durand", "2 commits")
	assertNotContains(t, w, "<html", `id="repo-overview"`)

	// Other target, or history restoration: full page.
	for _, h := range []map[string]string{{"HX-Target": "main"}, {"HX-Target": "repo-overview", "HX-History-Restore-Request": "true"}} {
		w = e.htmxWith(path, h)
		assertStatus(t, w, http.StatusOK)
		assertContains(t, w, "<html", `id="activity-data"`)
	}
	w = e.htmxWith(path+"/commits", map[string]string{"HX-History-Restore-Request": "true"})
	assertContains(t, w, "<html")
}

func TestRepoSyncButton(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	w := e.post(repoPath(ids.Bob)+"/sync", nil, true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Sync started.")
	st := e.waitSyncIdle()
	if st.RepoID != ids.Bob {
		t.Errorf("sync of repository %d, want %d", st.RepoID, ids.Bob)
	}
	// The page reads normally after the sync.
	assertStatus(t, e.get(repoPath(ids.Bob)), http.StatusOK)
}

func TestRepoGetDoesNotModify(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	before, err := e.store.RepositorySummary(t.Context(), ids.Alice)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{repoPath(ids.Alice), repoPath(ids.Alice) + "/commits?page=2&branch=dev"} {
		assertStatus(t, e.get(target), http.StatusOK)
	}
	after, err := e.store.RepositorySummary(t.Context(), ids.Alice)
	if err != nil {
		t.Fatal(err)
	}
	if before.LastRun.ID != after.LastRun.ID || before.Commits != after.Commits || e.coord.State().Generation != 0 {
		t.Errorf("a GET changed the state: %+v → %+v", before.LastRun, after.LastRun)
	}
}

// TestRepoChartTimezone checks the chart series at the day and window bounds, in a display time zone
// that is not UTC (Europe/Paris: summer time until October 25, 2026).
func TestRepoChartTimezone(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skipf("Europe/Paris time zone unavailable: %v", err)
	}
	e := newTestEnv(t)
	e.server.loc = paris
	id := e.addRepo(60, "fuseau", "")
	who := store.Identity{Name: "Eve", Email: "etudiant@example.test"}
	at := func(n int, ts time.Time) store.CommitWrite {
		return e.commitWrite(n, who, e.now.Sub(ts), "commit "+strconv.Itoa(n), "main")
	}
	// testNow = Oct 27 at 10:00 UTC = 11:00 in Paris. First day of the window: Jul 30 (summer
	// time), midnight in Paris = Jul 29 at 22:00 UTC.
	e.applySync(id, "fuseau",
		at(1, time.Date(2026, 7, 29, 21, 59, 59, 0, time.UTC)), // Jul 29 at 23:59 in Paris: outside the window
		at(2, time.Date(2026, 7, 29, 22, 0, 0, 0, time.UTC)),   // Jul 30 at 0:00 in Paris: first day
		at(3, time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)),  // Oct 25 at 2:30 (summer time): 25
		at(4, time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC)),  // Oct 25 at 2:30 (winter time): 25
		at(5, time.Date(2026, 10, 26, 22, 59, 0, 0, time.UTC)), // Oct 26 at 23:59: 26
		at(6, time.Date(2026, 10, 26, 23, 0, 0, 0, time.UTC)),  // Oct 27 at 0:00: 27 (the 26th in UTC)
		at(7, time.Date(2026, 10, 27, 9, 59, 0, 0, time.UTC)),  // Oct 27, just before now
	)
	w := e.get(repoPath(id))
	assertStatus(t, w, http.StatusOK)
	days := chartData(t, w)
	if len(days) != activity.ChartDays || days[0].Date != "2026-07-30" || days[len(days)-1].Date != "2026-10-27" {
		t.Fatalf("window of %d days, %s … %s", len(days), days[0].Date, days[len(days)-1].Date)
	}
	want := map[string]int{"2026-07-29": -1, "2026-07-30": 1, "2026-10-25": 2, "2026-10-26": 1, "2026-10-27": 2}
	for date, n := range want {
		if got := countOn(days, date); got != n {
			t.Errorf("day %s: %d, want %d", date, got, n)
		}
	}
	total := 0
	for _, d := range days {
		total += d.Count
	}
	if total != 6 {
		t.Errorf("total %d, want 6 (commit outside the window excluded)", total)
	}
	assertContains(t, w, "6 commits", "at most 2 on Oct 25, 2026")
}

func TestAuthorOptions(t *testing.T) {
	stats := func(name, login string) store.ContributorStats {
		var s store.ContributorStats
		s.Name, s.ProviderLogin = name, login
		return s
	}
	got := authorOptions([]store.ContributorStats{
		stats("Zoé", "zoe-gh"), stats("bob", "BOB"), stats("Alice", ""), stats("alice", ""), stats("", ""),
	}, "unknown-gh")
	want := []filterOption{
		{Value: "Alice", Label: "Alice"}, {Value: "BOB", Label: "bob"},
		{Value: "zoe-gh", Label: "Zoé (zoe-gh)"}, {Value: "unknown-gh", Label: "unknown-gh", Unknown: true},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("authorOptions = %v, want %v", got, want)
	}
	if got := authorOptions(nil, "zoe-gh"); len(got) != 1 || !got[0].Unknown {
		t.Errorf("selected author missing from the list: %v", got)
	}
}
