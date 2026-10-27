package web

// Tests des graphiques d'activité : calculs (fonctions pures de charts.go), pages du tableau de bord
// et des contributeurs (données JSON, résumé, chargement de Chart.js), absence des graphiques dans
// les fragments.

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
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

const chartScript = `<script src="/static/vendor/chart-4.6.1.umd.min.js" defer></script>`

// jsonData extrait le contenu du <script type="application/json" id="id"> de la page.
func jsonData(t *testing.T, w *httptest.ResponseRecorder, id string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)<script type="application/json" id="` + regexp.QuoteMeta(id) + `">(.*?)</script>`)
	m := re.FindStringSubmatch(w.Body.String())
	if m == nil {
		t.Fatalf("page sans données #%s :\n%s", id, w.Body.String())
	}
	return m[1]
}

func TestNewDailyChart(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	times := []time.Time{now.Add(-time.Hour), now.Add(-2 * time.Hour), now.Add(-24 * time.Hour)}
	c := newDailyChart(times, now, 3, time.UTC)
	if c.Days != 3 || len(c.Series) != 3 || c.Total != 3 {
		t.Fatalf("graphique %+v : attendu 3 jours, 3 commits", c)
	}
	if c.Peak != (activity.DayCount{Date: "2026-10-27", Count: 2}) || c.PeakDate != "27/10/2026" {
		t.Errorf("pic %v le %q, attendu 2 le 27/10/2026", c.Peak, c.PeakDate)
	}
	empty := newDailyChart(nil, now, 0, time.UTC)
	if empty.Days != 1 || len(empty.Series) != 1 || empty.Total != 0 || empty.PeakDate != "" {
		t.Errorf("graphique vide %+v", empty)
	}
}

// row fabrique une ligne du classement (contributeur id, nom name).
func row(id int64, name string) contributorRow {
	return contributorRow{ContributorStats: store.ContributorStats{Contributor: model.Contributor{ID: id, Name: name}}}
}

func TestBuildContributorChart(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC) // mardi
	week := 7 * 24 * time.Hour
	at := func(id int64, ago time.Duration) store.AuthorCommitTime {
		return store.AuthorCommitTime{ContributorID: id, CommittedAt: now.Add(-ago)}
	}
	rows := []contributorRow{
		row(1, "Alice"), row(2, "Bob"), row(3, "Carol"), row(4, "Dave"), row(5, "Ève \x1b[31mrouge"),
		row(6, strings.Repeat("n", 60)),
	}
	times := []store.AuthorCommitTime{
		at(1, time.Hour), at(1, week), // Alice : 2 (cette semaine, la précédente)
		at(2, time.Hour), at(2, 2*time.Hour), // Bob : 2 cette semaine (égalité avec Alice)
		at(3, 2*week), at(3, 2*week), at(3, 2*week), // Carol : 3, il y a deux semaines
		at(4, 10*week),    // Dave : hors fenêtre (3 semaines)
		at(5, time.Hour),  // Ève : 1
		at(6, time.Hour),  // nom long : 1
		at(99, time.Hour), // auteur hors du classement : « Autres »
		store.AuthorCommitTime{ContributorID: 1, CommittedAt: now.Add(week)}, // futur : ignoré
	}
	c := buildContributorChart(rows, times, now, 3, 3, time.UTC)

	if want := []string{"2026-10-12", "2026-10-19", "2026-10-26"}; !slices.Equal(c.Data.Weeks, want) {
		t.Errorf("semaines %v, attendu %v", c.Data.Weeks, want)
	}
	if want := []string{"12/10", "19/10", "26/10"}; !slices.Equal(c.WeekLabels, want) {
		t.Errorf("libellés %v, attendu %v", c.WeekLabels, want)
	}
	if c.Weeks != 3 || c.Top != 3 || c.Start != "12/10/2026" || c.TableColumns() != 5 {
		t.Errorf("résumé %+v", c)
	}
	// 2 + 2 + 3 + 1 + 1 + 1 = 10 commits dans la fenêtre ; 6 auteurs (Dave hors fenêtre).
	if c.Total != 10 || c.Active != 6 {
		t.Errorf("Total %d, Active %d ; attendu 10 et 6", c.Total, c.Active)
	}
	type serie struct {
		name   string
		counts []int
		other  bool
	}
	want := []serie{
		{"Carol", []int{3, 0, 0}, false},
		{"Alice", []int{0, 1, 1}, false}, // égalité avec Bob : ordre du classement
		{"Bob", []int{0, 0, 2}, false},
		{otherContributorsLabel, []int{0, 0, 3}, true}, // Ève, nom long, auteur inconnu
	}
	if len(c.Data.Series) != len(want) {
		t.Fatalf("séries %+v, attendu %+v", c.Data.Series, want)
	}
	for i, w := range want {
		got := c.Data.Series[i]
		if got.Name != w.name || !slices.Equal(got.Counts, w.counts) || got.Other != w.other {
			t.Errorf("série %d : %+v, attendu %+v", i, got, w)
		}
		if sum := w.counts[0] + w.counts[1] + w.counts[2]; got.Total != sum {
			t.Errorf("série %d : total %d, attendu %d", i, got.Total, sum)
		}
	}

	// Avec plus de place : noms nettoyés (séquences d'échappement) et tronqués ; pas de « Autres »
	// sans commit à regrouper.
	big := buildContributorChart(rows, times[:10], now, 3, 10, time.UTC)
	names := make([]string, 0, len(big.Data.Series))
	for _, s := range big.Data.Series {
		names = append(names, s.Name)
	}
	if want := []string{"Carol", "Alice", "Bob", "Ève rouge", strings.Repeat("n", contributorNameMax) + "…"}; !slices.Equal(names, want) {
		t.Errorf("séries %q, attendu %q", names, want)
	}

	// Aucun commit : aucune série (JSON « [] », pas « null »).
	none := buildContributorChart(rows, nil, now, 2, 8, time.UTC)
	data, err := json.Marshal(none.Data)
	if err != nil || string(data) != `{"weeks":["2026-10-19","2026-10-26"],"series":[]}` {
		t.Errorf("JSON sans commit = %s, %v", data, err)
	}
	if neg := buildContributorChart(rows, times, now, 3, -1, time.UTC); len(neg.Data.Series) != 1 || !neg.Data.Series[0].Other {
		t.Errorf("top < 0 : %+v, attendu la seule série « Autres »", neg.Data.Series)
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
		"Activité de l'espace sur 90 jours", "5 commits, au plus 1 le", chartScript)
	var days []activity.DayCount
	if err := json.Unmarshal([]byte(jsonData(t, w, "workspace-activity-data")), &days); err != nil {
		t.Fatal(err)
	}
	if len(days) != activity.ChartDays || days[len(days)-1].Date != "2026-10-27" {
		t.Fatalf("série de %d jours finissant le %v", len(days), days[len(days)-1])
	}
	total := 0
	for _, d := range days {
		total += d.Count
	}
	if total != 5 { // les cinq commits factices de seed, tous dépôts confondus (40 jours au plus)
		t.Errorf("total %d, attendu 5", total)
	}
	// Les fragments ne portent pas le graphique (HTMX retirerait le <script>).
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
		"Activité sur 12 semaines", "5 commits de 3 contributeurs depuis le 10/08/2026",
		`<table class="table table-xs" id="contributors-chart-table">`, chartScript)

	var data contributorChartData
	if err := json.Unmarshal([]byte(jsonData(t, w, "contributors-chart-data")), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Weeks) != activity.ContributorWeeks || data.Weeks[0] != "2026-08-10" || data.Weeks[11] != "2026-10-26" {
		t.Errorf("semaines %v", data.Weeks)
	}
	var names []string
	for _, s := range data.Series {
		names = append(names, s.Name)
	}
	// Alice et Bob : 2 commits chacun (Alice d'abord, comme au classement), Carol : 1.
	if want := []string{"Alice Martin", "Bob Durand", "Carol"}; !slices.Equal(names, want) {
		t.Errorf("séries %q, attendu %q", names, want)
	}
	// Le fragment rechargé après une synchro ne porte pas le graphique.
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
		t.Errorf("JSON non échappé dans le <script> : %s", raw)
	}
	var data contributorChartData
	if err := json.Unmarshal([]byte(raw), &data); err != nil || len(data.Series) != 1 ||
		data.Series[0].Name != evil.Name {
		t.Errorf("données %+v, %v", data, err)
	}
}
