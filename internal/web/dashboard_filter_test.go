package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/model"
)

func names(repos []DashboardRepo) string {
	var out []string
	for _, r := range repos {
		out = append(out, r.Name)
	}
	return strings.Join(out, ",")
}

func TestNaturalCompare(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"gr2", "gr10", -1},
		{"gr10", "gr9", 1},
		{"gr02", "gr2", 0},
		{"gr2-java", "gr2-web", -1},
		{"a", "ab", -1},
		{"2026-x-gr9", "2026-x-gr10", -1},
		{"", "", 0},
		{"é1", "é2", -1},
	} {
		if got := naturalCompare(tc.a, tc.b); got != tc.want {
			t.Errorf("naturalCompare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestParseDashboardFilter(t *testing.T) {
	get := func(v url.Values) func(string) string { return v.Get }
	f := parseDashboardFilter(get(url.Values{"sort": {"name"}, "state": {"flemme"}, "q": {"java"},
		"warnings": {"1"}, "prs": {"1"}}))
	want := DashboardFilter{Sort: sortName, State: filterStateFlemme, Query: "java", Warnings: true, PRs: true}
	if f != want {
		t.Errorf("filter %+v, want %+v", f, want)
	}
	// Unknown values and the default sort are ignored.
	f = parseDashboardFilter(get(url.Values{"sort": {"score"}, "state": {"error"}, "warnings": {"yes"}}))
	if f != (DashboardFilter{}) || f.Changed() || f.SortValue() != sortAttention {
		t.Errorf("unknown values: %+v", f)
	}
	if f := parseDashboardFilter(get(url.Values{"sort": {"attention"}})); f.Sort != "" {
		t.Errorf("default sort kept in the filter: %+v", f)
	}
}

func TestDashboardFilterPaths(t *testing.T) {
	f := DashboardFilter{Sort: sortName, State: filterStateDead, Query: "gr 6"}
	if got, want := f.PagePath(), "/?q=gr+6&sort=name&state=dead"; got != want {
		t.Errorf("PagePath %q, want %q", got, want)
	}
	if got, want := f.FragmentPath(), "/fragments/dashboard/repos?q=gr+6&sort=name&state=dead"; got != want {
		t.Errorf("FragmentPath %q, want %q", got, want)
	}
	// Clicking the active counter removes the state filter; another one replaces it.
	if got, want := f.StatePath(filterStateDead), "/?q=gr+6&sort=name"; got != want {
		t.Errorf("StatePath(active) %q, want %q", got, want)
	}
	if got, want := f.StatePath(filterStateActive), "/?q=gr+6&sort=name&state=active"; got != want {
		t.Errorf("StatePath(other) %q, want %q", got, want)
	}
	if got, want := f.ClearPath(), "/?sort=name"; got != want {
		t.Errorf("ClearPath %q, want %q", got, want)
	}
	if got := (DashboardFilter{Warnings: true, PRs: true}).PagePath(); got != "/?prs=1&warnings=1" {
		t.Errorf("flags: %q", got)
	}
	if got := (DashboardFilter{}).PagePath(); got != "/" {
		t.Errorf("default: %q", got)
	}
}

func TestFilterDashboardRepos(t *testing.T) {
	warned := dashRepo(3, "gr3-java", activity.StateActive, testNow)
	warned.LastRun.Warnings = []model.SyncWarning{{Kind: model.SyncWarningKind("lines")}}
	withPR := dashRepo(4, "gr4-web", activity.StateFlemme, testNow)
	withPR.OpenPullRequests = 2
	labeled := dashRepo(5, "x", activity.StateDead, testNow)
	labeled.Label = "Groupe JAVA 5"
	repos := []DashboardRepo{
		dashRepo(1, "gr1-java", activity.StateActive, testNow),
		dashRepo(2, "gr2-web", activity.StateNeverSynced, time.Time{}),
		warned, withPR, labeled,
		dashRepo(6, "gr6-web", activity.StateError, testNow),
	}
	for _, tc := range []struct {
		name string
		f    DashboardFilter
		want string
	}{
		{"none", DashboardFilter{}, "gr1-java,gr2-web,gr3-java,gr4-web,x,gr6-web"},
		{"active", DashboardFilter{State: filterStateActive}, "gr1-java,gr3-java"},
		{"flemme", DashboardFilter{State: filterStateFlemme}, "gr4-web"},
		{"dead", DashboardFilter{State: filterStateDead}, "x"},
		{"other", DashboardFilter{State: filterStateOther}, "gr2-web,gr6-web"},
		{"query on name, any case", DashboardFilter{Query: "JAVA"}, "gr1-java,gr3-java,x"},
		{"query on owner/repo", DashboardFilter{Query: "promo/gr2"}, "gr2-web"},
		{"warnings or error", DashboardFilter{Warnings: true}, "gr3-java,gr6-web"},
		{"open PRs", DashboardFilter{PRs: true}, "gr4-web"},
		{"combined", DashboardFilter{State: filterStateActive, Query: "java", Warnings: true}, "gr3-java"},
		{"nothing left", DashboardFilter{Query: "absent"}, ""},
	} {
		if got := names(filterDashboardRepos(repos, tc.f)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSortDashboardReposBy(t *testing.T) {
	day := func(n int) time.Time { return testNow.Add(-time.Duration(n) * 24 * time.Hour) }
	mk := func(id int64, name string, st activity.State, last time.Time, commits, added int) DashboardRepo {
		r := dashRepo(id, name, st, last)
		r.Commits = commits
		if added >= 0 {
			r.Lines = model.LineTotals{Commits: commits, KnownCommits: commits, Additions: added}
		}
		return r
	}
	repos := []DashboardRepo{
		mk(1, "GR10", activity.StateActive, day(1), 5, 100),
		mk(2, "gr9", activity.StateDead, day(40), 20, 50),
		mk(3, "gr2", activity.StateNeverSynced, time.Time{}, 0, -1),
		mk(4, "gr1", activity.StateFlemme, day(8), 20, 900),
	}
	for _, tc := range []struct{ sort, want string }{
		{sortAttention, "gr9,gr1,GR10,gr2"},
		{sortName, "gr1,gr2,gr9,GR10"},
		{sortRecent, "GR10,gr1,gr9,gr2"},
		{sortCommits, "gr1,gr9,GR10,gr2"},
		{sortLines, "gr1,GR10,gr9,gr2"},
	} {
		got := append([]DashboardRepo(nil), repos...)
		sortDashboardReposBy(got, tc.sort)
		if names(got) != tc.want {
			t.Errorf("sort %s: %s, want %s", tc.sort, names(got), tc.want)
		}
	}
}

func TestDashboardFilterPage(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()

	// Default view: the form, the counters as state links, no "shown" line.
	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="dashboard-filters"`, `hx-target="#dashboard-cards"`, `value="attention" selected`,
		`href="/?state=flemme"`, `href="/?state=other"`, "Needs attention", "Slacking</option>", "Open PRs")
	assertNotContains(t, w, "Clear filters")

	// Filter on a state: counters still cover the whole workspace.
	w = e.get("/?state=flemme&sort=name")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "1 of 4 repositories shown", `href="/?sort=name"`, `value="flemme" selected`,
		`value="name" selected`, "out of 4 repositories", fmt.Sprintf(`href="%s"`, repoPath(ids.Bob)))
	cards := section(t, w.Body.String(), "dashboard-cards")
	cards = cards[:strings.Index(cards, `id="dashboard-feed"`)]
	for _, id := range []int64{ids.Alice, ids.Carol, ids.Dave} {
		if strings.Contains(cards, fmt.Sprintf(`href="%s"`, repoPath(id))) {
			t.Errorf("repository %d shown despite the filter", id)
		}
	}

	// Search on the label, any case; nothing left; unknown values ignored.
	w = e.get("/?q=ALICE")
	assertContains(t, w, "1 of 4 repositories shown", "Alice Martin", `value="ALICE"`)
	w = e.get("/?q=zzz")
	assertContains(t, w, "0 of 4 repositories shown", "No repository matches these filters.")
	w = e.get("/?sort=score&state=nope")
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, "Clear filters")

}

func TestDashboardFilterFrench(t *testing.T) {
	e := newTestEnv(t, withLang("fr"))
	e.seed()
	w := e.get("/?state=dead")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "1 sur 4 dépôts affichés", "Effacer les filtres", "À surveiller", "Flemme</option>",
		"Autres</option>", "PR ouvertes", "Avec avertissements")
}

func TestDashboardFilterFragments(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()

	// Form change: only the cards, address pushed to the full page.
	r := e.request(http.MethodGet, "/fragments/dashboard/repos?state=dead&q=&sort=attention", nil)
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-Trigger", dashboardFiltersID)
	r.Header.Set("HX-Target", dashboardCardsID)
	w := e.do(r)
	assertStatus(t, w, http.StatusOK)
	body := strings.TrimSpace(w.Body.String())
	if !strings.HasPrefix(body, `<div id="dashboard-cards">`) {
		t.Errorf("cards fragment: unexpected start:\n%s", body)
	}
	if got := w.Header().Get("HX-Push-Url"); got != "/?state=dead" {
		t.Errorf("HX-Push-Url %q, want /?state=dead", got)
	}
	assertContains(t, w, fmt.Sprintf(`href="%s"`, repoPath(ids.Carol)), "1 of 4 repositories shown")
	assertNotContains(t, w, `id="dashboard-filters"`, "stat-value", fmt.Sprintf(`href="%s"`, repoPath(ids.Alice)))

	// Refresh at the end of a sync (form values included): the whole section, filters kept, no push.
	w = e.htmx("/fragments/dashboard/repos?state=dead")
	assertStatus(t, w, http.StatusOK)
	if !strings.HasPrefix(strings.TrimSpace(w.Body.String()), `<section id="dashboard-repos"`) {
		t.Errorf("section fragment: unexpected start:\n%s", w.Body.String())
	}
	if got := w.Header().Get("HX-Push-Url"); got != "" {
		t.Errorf("HX-Push-Url %q on a refresh", got)
	}
	assertContains(t, w, `value="dead" selected`, "1 of 4 repositories shown")
}
