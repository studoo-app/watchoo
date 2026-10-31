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

// dashRepo fabrique une carte factice pour les tests du tri.
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
	// Dead puis Flemme (les plus anciens d'abord), Actif (idem), en erreur, puis les autres gris par nom.
	want := []string{"victor", "will", "ugo", "xavier", "sam", "zoe", "theo", "Alba", "bruno", "yann"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ordre %v, attendu %v", got, want)
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
		t.Errorf("égalités : ordre %d,%d,%d, attendu 2,1,3", repos[0].ID, repos[1].ID, repos[2].ID)
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
		t.Errorf("compteurs %+v, attendu %+v", got, want)
	}
	if got := countStates(nil); got != (DashboardCounts{}) {
		t.Errorf("aucun dépôt : %+v", got)
	}
}

func TestDashboardAccent(t *testing.T) {
	for st, want := range map[activity.State]string{
		activity.StateActive: "border-success", activity.StateFlemme: "border-warning",
		activity.StateDead: "border-error", activity.StateNeverSynced: "border-base-300",
	} {
		if got := dashRepo(1, "a", st, testNow).Accent(); got != want {
			t.Errorf("%s : %q, attendu %q", st, got, want)
		}
	}
}

// section renvoie le morceau de body qui commence à l'élément d'identifiant id (jusqu'à la fin).
func section(t *testing.T, body, id string) string {
	t.Helper()
	i := strings.Index(body, `id="`+id+`"`)
	if i < 0 {
		t.Fatalf("élément %s absent :\n%s", id, body)
	}
	return body[i:]
}

func TestDashboardPage(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()

	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		"<title>Tableau de bord · "+testWorkspace+" · Watchoo</title>",
		`aria-current="page">Tableau de bord`,
		"Alice Martin", "promo/bob", "promo/carol", "promo/dave",
		"badge-success", "badge-warning", "badge-error", "badge-neutral",
		"Slacking · 9 j", "Dead · 40 j", "Never synced",
		"Derniers commits", "ajoute le README", "travail de Bob", "dernier signe de vie",
		"Bob Durand", "ago",
		// Pastille gris : dépôt jamais synchronisé, sans commit.
		"Aucun commit connu : pas encore synchronisé",
		// Fragments rafraîchis après une synchro.
		`hx-get="/fragments/dashboard/repos" hx-trigger="sync-finished from:body"`,
		`hx-get="/fragments/dashboard/feed" hx-trigger="sync-finished from:body"`,
		// Bouton de synchro, protégé par le jeton CSRF.
		`action="/sync"`, `name="csrf_token"`,
		// Heure locale dans title, ISO dans datetime.
		`datetime="2026-10-26T10:00:00Z"`, `title="26/10/2026 à 10:00"`,
	)
	// Message de commit : toujours échappé.
	assertContains(t, w, "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt; corrige le bug")
	assertNotContains(t, w, xssMessage, "<script>alert", "Page en construction", "evil.example.test")

	// Compteurs : 1 Actif, 1 Flemme, 1 Dead, 1 gris sur 4 dépôts.
	repos := section(t, w.Body.String(), "dashboard-repos")
	for _, stat := range []string{
		`text-success">1<`, `text-warning">1<`, `text-error">1<`, `text-base-content/60">1<`, "sur 4 dépôts",
	} {
		if !strings.Contains(repos, stat) {
			t.Errorf("compteur %q absent", stat)
		}
	}

	// Ordre des cartes : Dead (Carol), Flemme (Bob), Actif (Alice), puis gris (Dave).
	feedAt := strings.Index(repos, `id="dashboard-feed"`)
	cards := repos[:feedAt]
	last := -1
	for _, id := range []int64{ids.Carol, ids.Bob, ids.Alice, ids.Dave} {
		i := strings.Index(cards, fmt.Sprintf(`href="%s"`, repoPath(id)))
		if i < 0 || i < last {
			t.Fatalf("carte du dépôt %d absente ou mal placée (%d après %d)", id, i, last)
		}
		last = i
	}

	// Fil : du plus récent au plus ancien, SHA court lié au commit, lien vers le dépôt.
	feed := section(t, w.Body.String(), "dashboard-feed")
	xss := strings.Index(feed, "corrige le bug")
	readme := strings.Index(feed, "ajoute le README")
	carol := strings.Index(feed, "dernier signe de vie")
	if xss < 0 || readme < xss || carol < readme {
		t.Errorf("ordre du fil incorrect (%d, %d, %d)", xss, readme, carol)
	}
	if !strings.Contains(feed, `rel="noopener noreferrer"`) || !strings.Contains(feed, "https://github.com/promo/x/commit/") {
		t.Error("lien vers le commit absent du fil")
	}
	if !strings.Contains(feed, fmt.Sprintf(`href="%s"`, repoPath(ids.Carol))) {
		t.Error("lien vers le dépôt absent du fil")
	}
}

func TestDashboardEmpty(t *testing.T) {
	e := newTestEnv(t)
	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Aucun dépôt suivi", `href="/settings"`, "Aucun commit pour l'instant", "sur 0 dépôt<")
}

func TestDashboardFragments(t *testing.T) {
	e := newTestEnv(t)
	e.seed()

	w := e.htmx("/fragments/dashboard/repos")
	assertStatus(t, w, http.StatusOK)
	body := strings.TrimSpace(w.Body.String())
	if !strings.HasPrefix(body, `<section id="dashboard-repos"`) {
		t.Errorf("fragment des dépôts : début inattendu :\n%s", body)
	}
	assertContains(t, w, "Alice Martin", "Dead · 40 j", `hx-trigger="sync-finished from:body"`)
	assertNotContains(t, w, "<html", "<nav", `id="dashboard-feed"`, xssMessage)
	if !strings.Contains(w.Header().Get("Vary"), "HX-Request") {
		t.Errorf("Vary sans HX-Request : %q", w.Header().Get("Vary"))
	}

	w = e.htmx("/fragments/dashboard/feed")
	assertStatus(t, w, http.StatusOK)
	body = strings.TrimSpace(w.Body.String())
	if !strings.HasPrefix(body, `<section id="dashboard-feed"`) {
		t.Errorf("fragment du fil : début inattendu :\n%s", body)
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
		t.Errorf("%d commits dans le fil, attendu %d", got, dashboardFeedSize)
	}
	// Les plus récents d'abord : le commit 6 (il y a 6 min) est là, le 26 (il y a 26 min) non.
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
