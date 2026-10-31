package web

// Tests de la page de détail d'un dépôt « /repos/{id} » et de ses fragments (agent repo).

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

// htmxWith envoie GET target en HTMX, avec des en-têtes supplémentaires (HX-Target, HX-Trigger…).
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

// chartData extrait et décode la série JSON du graphique d'une page de détail.
func chartData(t *testing.T, w *httptest.ResponseRecorder) []activity.DayCount {
	t.Helper()
	m := chartDataRE.FindStringSubmatch(w.Body.String())
	if m == nil {
		t.Fatalf("page sans données #activity-data :\n%s", w.Body.String())
	}
	var days []activity.DayCount
	if err := json.Unmarshal([]byte(m[1]), &days); err != nil {
		t.Fatalf("données du graphique illisibles %q : %v", m[1], err)
	}
	return days
}

// countOn renvoie le nombre de commits du jour date dans la série (-1 si le jour est absent).
func countOn(days []activity.DayCount, date string) int {
	for _, d := range days {
		if d.Date == date {
			return d.Count
		}
	}
	return -1
}

// commitItems compte les commits listés dans l'historique (un lien de SHA par commit).
func commitItems(body string) int {
	return strings.Count(body, `title="Voir le commit sur GitHub"`)
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
		// échappement du message de commit (en-tête et historique)
		html.EscapeString(xssMessage),
		// bouton de synchro branché sur POST /repos/{id}/sync
		`action="`+path+`/sync"`, `hx-post="`+path+`/sync"`, `name="csrf_token"`,
		// graphique : canvas, données JSON, Chart.js chargé sur cette page
		`<canvas id="activity-chart"`, `data-refresh-url="`+path+`"`,
		`<script src="/static/vendor/chart-4.6.1.umd.min.js" defer></script>`,
		"Activité sur 90 jours", "3 commits",
		// contributeurs : avatar GitHub sans Referer, initiale sinon
		"Contributeurs", "Bob Durand", `src="https://avatars.githubusercontent.com/u/10?v=4"`,
		`referrerpolicy="no-referrer"`, `href="https://github.com/alice-gh"`,
		// historique et filtres
		`id="repo-commits"`, `id="repo-filters"`, `<option value="dev">dev</option>`,
		`<option value="alice-gh">Alice Martin (alice-gh)</option>`, `<option value="Bob Durand">Bob Durand</option>`,
		"ajoute le README", "premier commit",
		// rafraîchissement des fragments après une synchro
		`hx-trigger="sync-finished from:body"`,
	)
	assertNotContains(t, w, xssMessage, "<script>alert", `class="join"`)
	if n := commitItems(w.Body.String()); n != 3 {
		t.Errorf("%d commits listés, attendu 3", n)
	}
	// Aucun script en ligne, aucun attribut d'événement.
	assertNotContains(t, w, " onclick=", " onchange=", "hx-on")
	for _, m := range regexp.MustCompile(`<script[^>]*>`).FindAllString(w.Body.String(), -1) {
		if !strings.Contains(m, " src=") && !strings.Contains(m, `type="application/json"`) {
			t.Errorf("script en ligne : %s", m)
		}
	}

	days := chartData(t, w)
	if len(days) != activity.ChartDays {
		t.Fatalf("%d jours dans le graphique, attendu %d", len(days), activity.ChartDays)
	}
	if days[len(days)-1].Date != "2026-10-27" || days[0].Date != "2026-07-30" {
		t.Errorf("bornes du graphique %s … %s", days[0].Date, days[len(days)-1].Date)
	}
	for date, want := range map[string]int{"2026-10-26": 1, "2026-10-24": 1, "2026-09-27": 1, "2026-10-27": 0} {
		if got := countOn(days, date); got != want {
			t.Errorf("jour %s : %d commits, attendu %d", date, got, want)
		}
	}
}

func TestRepoPageStates(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()

	w := e.get(repoPath(ids.Carol))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "badge-error", "Dead · 40 j", "dernier signe de vie")
	// Avatar hors de avatars.githubusercontent.com : jamais chargé, initiale à la place.
	assertNotContains(t, w, "evil.example.test")
	assertContains(t, w, "avatar-placeholder")

	w = e.get(repoPath(ids.Bob))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "badge-warning", "Slacking · 9 j")

	w = e.get(repoPath(ids.Dave))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "badge-neutral", "Never synced", "Aucun commit en base pour ce dépôt",
		"0 commit", ">jamais<")
	if days := chartData(t, w); len(days) != activity.ChartDays {
		t.Errorf("%d jours pour un dépôt vide, attendu %d", len(days), activity.ChartDays)
	}

	// Suivi arrêté : pastille grise, pas de bouton de synchro.
	if err := e.store.SetRepositoryEnabled(t.Context(), ids.Bob, false); err != nil {
		t.Fatalf("SetRepositoryEnabled : %v", err)
	}
	w = e.get(repoPath(ids.Bob))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Tracking stopped", "badge-neutral")
	assertNotContains(t, w, "Synchroniser ce dépôt")
}

func TestRepoNotFound(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	for _, target := range []string{"/repos/9999", "/repos/9999/commits", "/repos/0", "/repos/abc",
		"/repos/12345678901234567890", "/repos/1/inconnu"} {
		w := e.get(target)
		assertStatus(t, w, http.StatusNotFound)
		assertContains(t, w, "Page introuvable")
	}
	w := e.htmx("/repos/9999/commits")
	assertStatus(t, w, http.StatusNotFound)
	assertContains(t, w, `role="alert"`)
	assertNotContains(t, w, "<html")
}

// seedMany crée un dépôt « promo/gros » de n commits d'Alice, le i-e daté de testNow - (i+1) h.
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
	assertContains(t, w, "120 commits", "page 1 sur 3", "commit n°000", "commit n°049",
		`href="`+path+`?page=2"`, `hx-get="`+path+`/commits?page=2"`, `hx-push-url="`+path+`?page=2"`)
	assertNotContains(t, w, "commit n°050", `rel="prev"`)
	if n := commitItems(w.Body.String()); n != repoCommitsPerPage {
		t.Errorf("%d commits en page 1, attendu %d", n, repoCommitsPerPage)
	}

	w = e.get(path + "?page=2")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "page 2 sur 3", "commit n°050", "commit n°099",
		`href="`+path+`?page=1"`, `href="`+path+`?page=3"`, `rel="prev"`, `rel="next"`)
	assertNotContains(t, w, "commit n°049<", "commit n°100")

	// Fragment HTMX : la dernière page seule, sans le document.
	w = e.htmx(path + "/commits?page=3")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<section id="repo-commits"`, "page 3 sur 3", "commit n°119",
		`hx-get="`+path+`/commits?page=3"`)
	assertNotContains(t, w, "<html", "<title>", `id="activity-data"`, `rel="next"`)
	if n := commitItems(w.Body.String()); n != 20 {
		t.Errorf("%d commits en page 3, attendu 20", n)
	}
	if w.Header().Get("HX-Push-Url") != "" {
		t.Errorf("HX-Push-Url %q pour une pagination (l'URL vient de hx-push-url)", w.Header().Get("HX-Push-Url"))
	}

	// Page hors bornes ou invalide : ramenée à la dernière ou à la première.
	assertContains(t, e.htmx(path+"/commits?page=999"), "page 3 sur 3")
	assertContains(t, e.htmx(path+"/commits?page=-4"), "page 1 sur 3")
	assertContains(t, e.htmx(path+"/commits?page=abc"), "page 1 sur 3")

	// Sans HTMX (adresse rechargée), le fragment donne la page complète.
	w = e.get(path + "/commits?page=2")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "<html", "page 2 sur 3", `id="activity-data"`)
}

func TestRepoFilters(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	path := repoPath(ids.Alice)

	w := e.get(path + "?branch=dev")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "ajoute le README", "1 commit correspondant aux filtres",
		`<option value="dev" selected>dev</option>`, "Effacer les filtres")
	assertNotContains(t, w, "premier commit")

	w = e.htmx(path + "/commits?author=alice-gh")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "2 commits correspondant aux filtres", "premier commit",
		`<option value="alice-gh" selected>`, `hx-get="`+path+`/commits?author=alice-gh&amp;page=1"`)
	assertNotContains(t, w, "ajoute le README", "<html")

	// Auteur sans compte : filtre par nom.
	w = e.htmx(path + "/commits?author=" + "Bob+Durand&branch=main")
	assertContains(t, w, "ajoute le README", `<option value="Bob Durand" selected>`, `<option value="main" selected>`)
	assertNotContains(t, w, "premier commit")

	// Filtres inconnus : gardés affichés, aucun résultat ; valeurs encodées dans les URL.
	w = e.htmx(path + "/commits?branch=" + "feat%2Fa%26b&author=x%22y")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Aucun commit ne correspond à ces filtres", "feat/a&amp;b (inconnue)", "x&#34;y (inconnu)",
		`hx-get="`+path+`/commits?author=x%22y&amp;branch=feat%2Fa%26b&amp;page=1"`)

	// Filtre par e-mail : ignoré (aucune adresse dans la page ni dans les URL).
	w = e.get(path + "?author=alice@example.test")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Le filtre par adresse e-mail n&#39;est pas proposé ici", "3 commits")

	// Changement de filtre par le formulaire : l'adresse du navigateur suit (page complète).
	w = e.htmxWith(path+"/commits?branch=dev&author=", map[string]string{"HX-Trigger": "repo-filters"})
	assertStatus(t, w, http.StatusOK)
	if got, want := w.Header().Get("HX-Push-Url"), path+"?branch=dev"; got != want {
		t.Errorf("HX-Push-Url = %q, attendu %q", got, want)
	}
	// Rafraîchissement après une synchro : pas d'historique.
	w = e.htmxWith(path+"/commits?branch=dev", map[string]string{"HX-Trigger": "repo-commits"})
	if got := w.Header().Get("HX-Push-Url"); got != "" {
		t.Errorf("HX-Push-Url = %q pour un rafraîchissement", got)
	}
}

func TestRepoFiltersKeepPagination(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedMany(60)
	path := repoPath(id)
	w := e.get(path + "?branch=main&author=alice-gh")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "page 1 sur 2",
		`href="`+path+`?author=alice-gh&amp;branch=main&amp;page=2"`,
		`hx-get="`+path+`/commits?author=alice-gh&amp;branch=main&amp;page=2"`)
}

func TestRepoFragments(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	path := repoPath(ids.Alice)

	w := e.htmxWith(path, map[string]string{"HX-Target": "repo-overview"})
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<section id="repo-overview"`, "Alice Martin", "badge-success", "Synchroniser ce dépôt",
		html.EscapeString(xssMessage))
	assertNotContains(t, w, "<html", `id="repo-commits"`, `id="activity-data"`, xssMessage)

	w = e.htmxWith(path, map[string]string{"HX-Target": "repo-contributors"})
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<section id="repo-contributors"`, "Bob Durand", "2 commits")
	assertNotContains(t, w, "<html", `id="repo-overview"`)

	// Autre cible, ou restauration d'historique : page complète.
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
	assertContains(t, w, msgSyncStarted)
	st := e.waitSyncIdle()
	if st.RepoID != ids.Bob {
		t.Errorf("synchro du dépôt %d, attendu %d", st.RepoID, ids.Bob)
	}
	// La page se lit normalement après la synchro.
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
		t.Errorf("un GET a modifié l'état : %+v → %+v", before.LastRun, after.LastRun)
	}
}

// TestRepoChartTimezone vérifie la série du graphique aux bornes de jour et de fenêtre, dans un
// fuseau d'affichage qui n'est pas UTC (Europe/Paris : heure d'été jusqu'au 25 octobre 2026).
func TestRepoChartTimezone(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skipf("fuseau Europe/Paris indisponible : %v", err)
	}
	e := newTestEnv(t)
	e.server.loc = paris
	id := e.addRepo(60, "fuseau", "")
	who := store.Identity{Name: "Eve", Email: "etudiant@example.test"}
	at := func(n int, ts time.Time) store.CommitWrite {
		return e.commitWrite(n, who, e.now.Sub(ts), "commit "+strconv.Itoa(n), "main")
	}
	// testNow = 27/10 à 10 h UTC = 11 h à Paris. Premier jour de la fenêtre : 30/07 (heure d'été),
	// minuit à Paris = 29/07 à 22 h UTC.
	e.applySync(id, "fuseau",
		at(1, time.Date(2026, 7, 29, 21, 59, 59, 0, time.UTC)), // 29/07 à 23 h 59 à Paris : hors fenêtre
		at(2, time.Date(2026, 7, 29, 22, 0, 0, 0, time.UTC)),   // 30/07 à 0 h à Paris : premier jour
		at(3, time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)),  // 25/10 à 2 h 30 (heure d'été) : 25
		at(4, time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC)),  // 25/10 à 2 h 30 (heure d'hiver) : 25
		at(5, time.Date(2026, 10, 26, 22, 59, 0, 0, time.UTC)), // 26/10 à 23 h 59 : 26
		at(6, time.Date(2026, 10, 26, 23, 0, 0, 0, time.UTC)),  // 27/10 à 0 h : 27 (le 26 en UTC)
		at(7, time.Date(2026, 10, 27, 9, 59, 0, 0, time.UTC)),  // 27/10, juste avant now
	)
	w := e.get(repoPath(id))
	assertStatus(t, w, http.StatusOK)
	days := chartData(t, w)
	if len(days) != activity.ChartDays || days[0].Date != "2026-07-30" || days[len(days)-1].Date != "2026-10-27" {
		t.Fatalf("fenêtre %d jours, %s … %s", len(days), days[0].Date, days[len(days)-1].Date)
	}
	want := map[string]int{"2026-07-29": -1, "2026-07-30": 1, "2026-10-25": 2, "2026-10-26": 1, "2026-10-27": 2}
	for date, n := range want {
		if got := countOn(days, date); got != n {
			t.Errorf("jour %s : %d, attendu %d", date, got, n)
		}
	}
	total := 0
	for _, d := range days {
		total += d.Count
	}
	if total != 6 {
		t.Errorf("total %d, attendu 6 (commit hors fenêtre exclu)", total)
	}
	assertContains(t, w, "6 commits", "au plus 2 le 25/10/2026")
}

func TestAuthorOptions(t *testing.T) {
	stats := func(name, login string) store.ContributorStats {
		var s store.ContributorStats
		s.Name, s.ProviderLogin = name, login
		return s
	}
	got := authorOptions([]store.ContributorStats{
		stats("Zoé", "zoe-gh"), stats("bob", "BOB"), stats("Alice", ""), stats("alice", ""), stats("", ""),
	}, "inconnu-gh")
	want := []filterOption{
		{Value: "Alice", Label: "Alice"}, {Value: "BOB", Label: "bob"},
		{Value: "zoe-gh", Label: "Zoé (zoe-gh)"}, {Value: "inconnu-gh", Label: "inconnu-gh (inconnu)"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("authorOptions = %v, attendu %v", got, want)
	}
	if got := authorOptions(nil, "zoe-gh"); len(got) != 1 {
		t.Errorf("auteur sélectionné absent de la liste : %v", got)
	}
}
