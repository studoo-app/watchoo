package web

// Tests of the activity charts: computations (pure functions of charts.go), dashboard and
// contributors pages (JSON data, summary, loading of Chart.js), absence of the charts in the
// fragments.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

const chartScript = `<script src="/static/vendor/chart-4.6.1.umd.min.js" defer></script>`

// jsonData extracts the content of the <script type="application/json" id="id"> of the page.
func jsonData(t *testing.T, w *httptest.ResponseRecorder, id string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)<script type="application/json" id="` + regexp.QuoteMeta(id) + `">(.*?)</script>`)
	m := re.FindStringSubmatch(w.Body.String())
	if m == nil {
		t.Fatalf("page without data #%s:\n%s", id, w.Body.String())
	}
	return m[1]
}

func TestNewDailyChart(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	times := []time.Time{now.Add(-time.Hour), now.Add(-2 * time.Hour), now.Add(-24 * time.Hour)}
	c := newDailyChart(en, times, now, 3, time.UTC)
	if c.Days != 3 || len(c.Series) != 3 || c.Total != 3 {
		t.Fatalf("chart %+v: want 3 days, 3 commits", c)
	}
	if c.Peak != (activity.DayCount{Date: "2026-10-27", Count: 2}) || c.PeakDate != "Oct 27, 2026" {
		t.Errorf("peak %v on %q, want 2 on Oct 27, 2026", c.Peak, c.PeakDate)
	}
	if fr := newDailyChart(i18n.For(i18n.French), times, now, 3, time.UTC); fr.PeakDate != "27/10/2026" {
		t.Errorf("French peak date %q, want 27/10/2026", fr.PeakDate)
	}
	empty := newDailyChart(en, nil, now, 0, time.UTC)
	if empty.Days != 1 || len(empty.Series) != 1 || empty.Total != 0 || empty.PeakDate != "" {
		t.Errorf("empty chart %+v", empty)
	}
}

// en is the English translator of the pure chart functions.
var en = i18n.For(i18n.English)

// row builds a ranking row (contributor id, name name).
func row(id int64, name string) store.ContributorStats {
	return store.ContributorStats{Contributor: model.Contributor{ID: id, Name: name}}
}

func TestBuildContributorChart(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC) // Tuesday
	week := 7 * 24 * time.Hour
	at := func(id int64, ago time.Duration) model.AuthorCommitTime {
		return model.AuthorCommitTime{ContributorID: id, CommittedAt: now.Add(-ago)}
	}
	rows := []store.ContributorStats{
		row(1, "Alice"), row(2, "Bob"), row(3, "Carol"), row(4, "Dave"), row(5, "Ève \x1b[31mrouge"),
		row(6, strings.Repeat("n", 60)),
	}
	times := []model.AuthorCommitTime{
		at(1, time.Hour), at(1, week), // Alice: 2 (this week, the previous one)
		at(2, time.Hour), at(2, 2*time.Hour), // Bob: 2 this week (tie with Alice)
		at(3, 2*week), at(3, 2*week), at(3, 2*week), // Carol: 3, two weeks ago
		at(4, 10*week),    // Dave: outside the window (3 weeks)
		at(5, time.Hour),  // Ève: 1
		at(6, time.Hour),  // long name: 1
		at(99, time.Hour), // author outside the ranking: "Other"
		model.AuthorCommitTime{ContributorID: 1, CommittedAt: now.Add(week)}, // future: ignored
	}
	c := buildContributorChart(en, rows, times, now, 3, 3, time.UTC)

	if want := []string{"2026-10-12", "2026-10-19", "2026-10-26"}; !slices.Equal(c.Data.Weeks, want) {
		t.Errorf("weeks %v, want %v", c.Data.Weeks, want)
	}
	if want := []string{"Oct 12", "Oct 19", "Oct 26"}; !slices.Equal(c.WeekLabels, want) {
		t.Errorf("labels %v, want %v", c.WeekLabels, want)
	}
	if c.Weeks != 3 || c.Top != 3 || c.Start != "Oct 12, 2026" || c.TableColumns() != 5 {
		t.Errorf("summary %+v", c)
	}
	fr := buildContributorChart(i18n.For(i18n.French), rows, times, now, 3, 3, time.UTC)
	if want := []string{"12/10", "19/10", "26/10"}; fr.Start != "12/10/2026" || !slices.Equal(fr.WeekLabels, want) ||
		fr.Data.Series[len(fr.Data.Series)-1].Name != "Autres contributeurs" {
		t.Errorf("French chart: start %q, labels %v, series %+v", fr.Start, fr.WeekLabels, fr.Data.Series)
	}
	// 2 + 2 + 3 + 1 + 1 + 1 = 10 commits in the window; 6 authors (Dave outside the window).
	if c.Total != 10 || c.Active != 6 {
		t.Errorf("Total %d, Active %d; want 10 and 6", c.Total, c.Active)
	}
	type serie struct {
		name   string
		counts []int
		other  bool
	}
	want := []serie{
		{"Carol", []int{3, 0, 0}, false},
		{"Alice", []int{0, 1, 1}, false}, // tie with Bob: ranking order
		{"Bob", []int{0, 0, 2}, false},
		{"Other contributors", []int{0, 0, 3}, true}, // Ève, long name, unknown author
	}
	if len(c.Data.Series) != len(want) {
		t.Fatalf("series %+v, want %+v", c.Data.Series, want)
	}
	for i, w := range want {
		got := c.Data.Series[i]
		if got.Name != w.name || !slices.Equal(got.Counts, w.counts) || got.Other != w.other {
			t.Errorf("series %d: %+v, want %+v", i, got, w)
		}
		if sum := w.counts[0] + w.counts[1] + w.counts[2]; got.Total != sum {
			t.Errorf("series %d: total %d, want %d", i, got.Total, sum)
		}
	}

	// With more room: names cleaned (escape sequences) and truncated; no "Other" without commits to
	// group.
	big := buildContributorChart(en, rows, times[:10], now, 3, 10, time.UTC)
	names := make([]string, 0, len(big.Data.Series))
	for _, s := range big.Data.Series {
		names = append(names, s.Name)
	}
	if want := []string{"Carol", "Alice", "Bob", "Ève rouge", strings.Repeat("n", contributorNameMax) + "…"}; !slices.Equal(names, want) {
		t.Errorf("series %q, want %q", names, want)
	}

	// No commit: no series (JSON "[]", not "null").
	none := buildContributorChart(en, rows, nil, now, 2, 8, time.UTC)
	data, err := json.Marshal(none.Data)
	if err != nil || string(data) != `{"weeks":["2026-10-19","2026-10-26"],"series":[]}` {
		t.Errorf("JSON without commit = %s, %v", data, err)
	}
	if neg := buildContributorChart(en, rows, times, now, 3, -1, time.UTC); len(neg.Data.Series) != 1 || !neg.Data.Series[0].Other {
		t.Errorf("top < 0: %+v, want the single \"Other\" series", neg.Data.Series)
	}
}

func TestDashboardActivityChart(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		`<canvas id="workspace-activity-chart" role="img" data-chart="daily" data-source="workspace-activity-data"`,
		`data-summary="workspace-activity-summary" data-refresh-url="/"`,
		"Workspace activity over 90 days", "5 commits, at most 1 on", chartScript)
	var days []activity.DayCount
	if err := json.Unmarshal([]byte(jsonData(t, w, "workspace-activity-data")), &days); err != nil {
		t.Fatal(err)
	}
	if len(days) != activity.ChartDays || days[len(days)-1].Date != "2026-10-27" {
		t.Fatalf("series of %d days ending on %v", len(days), days[len(days)-1])
	}
	total := 0
	for _, d := range days {
		total += d.Count
	}
	if total != 5 { // the five dummy commits of seed, all repositories (40 days at most)
		t.Errorf("total %d, want 5", total)
	}
	// The fragments do not carry the chart (HTMX would remove the <script>).
	for _, target := range []string{"/fragments/dashboard/repos", "/fragments/dashboard/feed"} {
		assertNotContains(t, e.htmx(target), "workspace-activity", "chart-4.6.1")
	}
}

func TestDashboardActivityChartEmpty(t *testing.T) {
	e := newTestEnv(t)
	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, "workspace-activity-chart", "chart-4.6.1")
}

func TestContributorsActivityChart(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	w := e.get("/contributors")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		`<canvas id="contributors-chart" role="img" data-chart="weekly-stacked" data-source="contributors-chart-data"`,
		`data-table="contributors-chart-table" data-refresh-url="/contributors"`,
		"Activity over 12 weeks", "5 commits by 3 contributors since Aug 10, 2026",
		`<table class="table table-xs" id="contributors-chart-table">`, chartScript)

	var data contributorChartData
	if err := json.Unmarshal([]byte(jsonData(t, w, "contributors-chart-data")), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Weeks) != activity.ContributorWeeks || data.Weeks[0] != "2026-08-10" || data.Weeks[11] != "2026-10-26" {
		t.Errorf("weeks %v", data.Weeks)
	}
	var names []string
	for _, s := range data.Series {
		names = append(names, s.Name)
	}
	// Alice and Bob: 2 commits each (Alice first, as in the ranking), Carol: 1.
	if want := []string{"Alice Martin", "Bob Durand", "Carol"}; !slices.Equal(names, want) {
		t.Errorf("series %q, want %q", names, want)
	}
	// The fragment reloaded after a sync does not carry the chart.
	assertNotContains(t, e.htmx("/fragments/contributors"), "contributors-chart", "chart-4.6.1")
}

func TestContributorsActivityChartEmpty(t *testing.T) {
	e := newTestEnv(t)
	w := e.get("/contributors")
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, "contributors-chart", "chart-4.6.1")
}

func TestChartJSONEscaping(t *testing.T) {
	e := newTestEnv(t)
	id := e.addRepo(1, "alice", "")
	evil := store.Identity{Name: "</script><script>alert(1)</script>", Email: "alice@example.test"}
	e.applySync(id, "alice", e.commitWrite(1, evil, time.Hour, "message", "main"))
	w := e.get("/contributors")
	assertStatus(t, w, http.StatusOK)
	raw := jsonData(t, w, "contributors-chart-data")
	if strings.Contains(raw, "<") || strings.Contains(raw, ">") {
		t.Errorf("JSON not escaped in the <script>: %s", raw)
	}
	var data contributorChartData
	if err := json.Unmarshal([]byte(raw), &data); err != nil || len(data.Series) != 1 ||
		data.Series[0].Name != evil.Name {
		t.Errorf("data %+v, %v", data, err)
	}
}

// Repository page: contributors chart restricted to the repository (commits of the other
// repositories excluded, commit present on two branches counted once), outside the HTMX fragments.
func TestRepoContributorsChart(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	path := repoPath(ids.Alice)
	w := e.get(path)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		`<canvas id="repo-contributors-chart" role="img" data-chart="weekly-stacked" data-source="repo-contributors-chart-data"`,
		`data-summary="repo-contributors-chart-summary" data-table="repo-contributors-chart-table" data-refresh-url="`+path+`"`,
		"Activity per contributor over 12 weeks", "3 commits by 2 contributors since Aug 10, 2026",
		"most active contributors in this repository over the last 12 weeks: 3 commits in total",
		"is counted once",
		`<table class="table table-xs" id="repo-contributors-chart-table">`, chartScript)

	var data contributorChartData
	if err := json.Unmarshal([]byte(jsonData(t, w, "repo-contributors-chart-data")), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Weeks) != activity.ContributorWeeks || data.Weeks[11] != "2026-10-26" {
		t.Errorf("weeks %v", data.Weeks)
	}
	got := map[string][]int{}
	for _, s := range data.Series {
		got[s.Name] = s.Counts
	}
	// Alice: Sep 27 (week of Sep 21, index 6) and Oct 26 (index 11); Bob: Oct 24 (index 10), his
	// commit of the bob repository excluded.
	want := map[string][]int{
		"Alice Martin": {0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		"Bob Durand":   {0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0},
	}
	if len(got) != len(want) || len(data.Series) != 2 || data.Series[0].Name != "Alice Martin" {
		t.Fatalf("series %+v, want %v", data.Series, want)
	}
	for name, counts := range want {
		if !slices.Equal(got[name], counts) {
			t.Errorf("%s: %v, want %v", name, got[name], counts)
		}
	}

	// The fragments reloaded after a sync do not carry the chart.
	for _, target := range []string{repoOverviewID, repoContributorsID} {
		assertNotContains(t, e.htmxWith(path, map[string]string{"HX-Target": target}), "repo-contributors-chart")
	}
	assertNotContains(t, e.htmx(path+"/commits"), "repo-contributors-chart")
}

// Repository without commit over the period: the chart stays in place (filled at the next sync) with
// a summary and a table that say so.
func TestRepoContributorsChartEmpty(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	w := e.get(repoPath(ids.Dave))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<canvas id="repo-contributors-chart"`,
		"No commit by a known author since Aug 10, 2026", "No commit over the period.", chartScript)
	if raw := jsonData(t, w, "repo-contributors-chart-data"); !strings.Contains(raw, `"series":[]`) {
		t.Errorf("data without commit: %s", raw)
	}
	// Carol: a single commit, 40 days ago, in the 12-week window.
	w = e.get(repoPath(ids.Carol))
	assertContains(t, w, "1 commit by 1 contributor since Aug 10, 2026")
}

// TestBuildRepoChart checks the per-repository chart of a contributor's page: one series per
// repository (link to its page), the repositories beyond top grouped in "Other repositories".
func TestBuildRepoChart(t *testing.T) {
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	repos := []store.ContributorRepoStats{
		{RepoID: 1, RepoOwner: "promo", RepoName: "a", RepoLabel: "Projet A"},
		{RepoID: 2, RepoOwner: "promo", RepoName: "b"},
		{RepoID: 3, RepoOwner: "promo", RepoName: "c"},
	}
	at := func(repo int64, ago time.Duration) model.RepoCommitTime {
		return model.RepoCommitTime{RepoID: repo, CommittedAt: now.Add(-ago)}
	}
	times := []model.RepoCommitTime{at(2, time.Hour), at(2, 2*time.Hour), at(1, 8*24*time.Hour), at(3, time.Hour),
		at(9, time.Hour)} // repository 9: absent from the list (read between two reads)
	c := buildRepoChart(en, repos, times, now, 2, 2, time.UTC)
	if c.Total != 5 || c.Weeks != 2 {
		t.Fatalf("Total %d, Weeks %d", c.Total, c.Weeks)
	}
	names := make([]string, 0, len(c.Data.Series))
	for _, s := range c.Data.Series {
		names = append(names, s.Name)
	}
	if want := []string{"promo/b", "Projet A", "Other repositories"}; !slices.Equal(names, want) {
		t.Errorf("series = %v, want %v", names, want)
	}
	if s := c.Data.Series[0]; s.Path != repoPath(2) || s.Total != 2 || s.ProfileURL != "" {
		t.Errorf("series of repository 2 = %+v", s)
	}
	if o := c.Data.Series[2]; !o.Other || o.Total != 2 || o.Path != "" {
		t.Errorf("other repositories = %+v", o)
	}
}
