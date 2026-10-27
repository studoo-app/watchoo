package web

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

func TestContributorsPage(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()

	w := e.get("/contributors")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		"<title>Contributeurs · "+testWorkspace+" · Watchoo</title>",
		`aria-current="page">Contributeurs`,
		"Alice Martin", "@alice-gh", "Bob Durand", "Carol",
		// Avatar GitHub : seulement depuis avatars.githubusercontent.com, sans Referer, en différé.
		`<img src="https://avatars.githubusercontent.com/u/10?v=4" alt="" loading="lazy" referrerpolicy="no-referrer"`,
		// Commits par dépôt : liens vers le détail.
		`href="`+repoPath(ids.Alice)+`"`, `href="`+repoPath(ids.Bob)+`"`, `href="`+repoPath(ids.Carol)+`"`,
		// Rafraîchi après une synchro.
		`hx-get="/fragments/contributors" hx-trigger="sync-finished from:body"`,
		// Dernière activité : date relative, heure locale dans title.
		`title="26/10/2026 à 10:00">il y a 1 j</time>`,
		// Compteurs : 3 contributeurs, 5 commits, 3 dépôts.
		`<div class="stat-value">3</div>`, `<div class="stat-value">5</div>`,
	)
	// L'avatar de Carol est sur un autre hôte : jamais affiché, l'initiale le remplace.
	assertNotContains(t, w, "evil.example.test", "@Bob")
	assertContains(t, w, "<span>C</span>")

	// Classement : Alice (2 commits, le plus récent), Bob (2 commits), Carol (1).
	body := w.Body.String()
	a, b, c := strings.Index(body, "Alice Martin</a></div>"), strings.Index(body, "Bob Durand</a></div>"), strings.Index(body, "Carol</a></div>")
	if a < 0 || b < 0 || c < 0 || a >= b || b >= c {
		t.Errorf("ordre du classement inattendu : Alice %d, Bob %d, Carol %d", a, b, c)
	}
}

func TestContributorsFragment(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	w := e.htmx("/fragments/contributors")
	assertStatus(t, w, http.StatusOK)
	if !strings.HasPrefix(strings.TrimSpace(w.Body.String()), `<section id="contributors-table"`) {
		t.Errorf("fragment inattendu :\n%s", w.Body.String())
	}
	assertNotContains(t, w, "<html", "<nav")
	assertContains(t, w, "Alice Martin")
}

func TestContributorsEmpty(t *testing.T) {
	e := newTestEnv(t)
	w := e.get("/contributors")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Aucun contributeur pour l'instant", `href="/settings"`)
}

func TestContributorsEscaping(t *testing.T) {
	e := newTestEnv(t)
	id := e.addRepo(9, "mallory", "")
	if _, err := e.store.UpdateRepositorySettings(t.Context(), id, `<b>Dépôt</b>`, nil); err != nil {
		t.Fatal(err)
	}
	who := store.Identity{Name: `<img src=x onerror=alert(1)>`, Email: "etudiant@example.test",
		Login: `"><script>`, ProviderUserID: 99, AvatarURL: `https://avatars.githubusercontent.com/u/1"onload="x`}
	e.applySync(id, "mallory", e.commitWrite(9, who, time.Hour, xssMessage, "main"))

	w := e.get("/contributors")
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, `<img src=x`, `"><script>`, `<b>Dépôt</b>`, `onload="x`, "<script>alert")
	assertContains(t, w, "&lt;img src=x onerror=alert(1)&gt;", "&lt;b&gt;Dépôt&lt;/b&gt;", "@&#34;&gt;&lt;script&gt;")
}

func TestBuildContributorRows(t *testing.T) {
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ranking := []store.ContributorStats{
		{Contributor: model.Contributor{ID: 7, Name: "Zoé"}, Commits: 5},
		{Contributor: model.Contributor{ID: 3, Name: "Yann"}, Commits: 2},
		{Contributor: model.Contributor{ID: 4, Name: "Xavier"}, Commits: 1},
	}
	breakdown := []store.ContributorRepoStats{
		{ContributorID: 3, RepoID: 1, RepoOwner: "o", RepoName: "b", Commits: 1, LastCommitAt: day},
		{ContributorID: 3, RepoID: 2, RepoOwner: "o", RepoName: "a", Commits: 1, LastCommitAt: day},
		{ContributorID: 7, RepoID: 1, RepoOwner: "o", RepoName: "b", Commits: 1, LastCommitAt: day.Add(time.Hour)},
		{ContributorID: 7, RepoID: 2, RepoOwner: "o", RepoName: "a", Commits: 4, LastCommitAt: day},
		{ContributorID: 99, RepoID: 2, RepoOwner: "o", RepoName: "a", Commits: 9}, // absent du classement
	}
	rows := buildContributorRows(ranking, breakdown)
	if len(rows) != 3 {
		t.Fatalf("%d lignes, attendu 3", len(rows))
	}
	for i, want := range []int64{7, 3, 4} {
		if rows[i].ID != want || rows[i].Rank != i+1 {
			t.Errorf("ligne %d : contributeur %d rang %d, attendu %d rang %d", i, rows[i].ID, rows[i].Rank, want, i+1)
		}
	}
	// Zoé : o/a (4 commits) avant o/b (1).
	if got := rows[0].Repos; len(got) != 2 || got[0].RepoID != 2 || got[1].RepoID != 1 {
		t.Errorf("dépôts de Zoé : %+v", got)
	}
	// Yann : égalité de commits et de date, départagée par le nom affiché (o/a avant o/b).
	if got := rows[1].Repos; len(got) != 2 || got[0].RepoDisplayName() != "o/a" {
		t.Errorf("dépôts de Yann : %+v", got)
	}
	if len(rows[2].Repos) != 0 {
		t.Errorf("Xavier sans détail : %+v", rows[2].Repos)
	}
	if rows := buildContributorRows(nil, breakdown); len(rows) != 0 {
		t.Errorf("classement vide : %+v", rows)
	}
}

func TestContributorRowShowLogin(t *testing.T) {
	cases := []struct {
		c    model.Contributor
		want bool
	}{
		{model.Contributor{Name: "Alice", ProviderLogin: "alice-gh"}, true},
		{model.Contributor{ProviderLogin: "alice-gh"}, false}, // le login est déjà le nom affiché
		{model.Contributor{Name: "Bob"}, false},
	}
	for _, tc := range cases {
		row := contributorRow{ContributorStats: store.ContributorStats{Contributor: tc.c}}
		if got := row.ShowLogin(); got != tc.want {
			t.Errorf("ShowLogin(%+v) = %v, attendu %v", tc.c, got, tc.want)
		}
	}
}
