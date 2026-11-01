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
		"<title>Contributors · "+testWorkspace+" · Watchoo</title>",
		`aria-current="page">Contributors`,
		"Alice Martin", "@alice-gh", "Bob Durand", "Carol",
		// GitHub avatar: only from avatars.githubusercontent.com, without Referer, deferred.
		`<img src="https://avatars.githubusercontent.com/u/10?v=4" alt="" loading="lazy" referrerpolicy="no-referrer"`,
		// Commits per repository: links to the detail.
		`href="`+repoPath(ids.Alice)+`"`, `href="`+repoPath(ids.Bob)+`"`, `href="`+repoPath(ids.Carol)+`"`,
		// Refreshed after a sync.
		`hx-get="/fragments/contributors" hx-trigger="sync-finished from:body"`,
		// Last activity: relative date, local time in title.
		`title="Oct 26, 2026 10:00">1 d ago</time>`,
		// Counters: 3 contributors, 5 commits, 3 repositories.
		`<div class="stat-value">3</div>`, `<div class="stat-value">5</div>`,
	)
	// Carol's avatar is on another host: never shown, the initial replaces it.
	assertNotContains(t, w, "evil.example.test", "@Bob")
	assertContains(t, w, "<span>C</span>")

	// Ranking: Alice (2 commits, the most recent), Bob (2 commits), Carol (1).
	body := w.Body.String()
	a, b, c := strings.Index(body, "Alice Martin</a></div>"), strings.Index(body, "Bob Durand</a></div>"), strings.Index(body, "Carol</a></div>")
	if a < 0 || b < 0 || c < 0 || a >= b || b >= c {
		t.Errorf("unexpected ranking order: Alice %d, Bob %d, Carol %d", a, b, c)
	}
}

func TestContributorsFragment(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	w := e.htmx("/fragments/contributors")
	assertStatus(t, w, http.StatusOK)
	if !strings.HasPrefix(strings.TrimSpace(w.Body.String()), `<section id="contributors-table"`) {
		t.Errorf("unexpected fragment:\n%s", w.Body.String())
	}
	assertNotContains(t, w, "<html", "<nav")
	assertContains(t, w, "Alice Martin")
}

func TestContributorsEmpty(t *testing.T) {
	e := newTestEnv(t)
	w := e.get("/contributors")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "No contributor yet", `href="/settings"`)
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
		{ContributorID: 99, RepoID: 2, RepoOwner: "o", RepoName: "a", Commits: 9}, // absent from the ranking
	}
	rows := buildContributorRows(ranking, breakdown)
	if len(rows) != 3 {
		t.Fatalf("%d rows, want 3", len(rows))
	}
	for i, want := range []int64{7, 3, 4} {
		if rows[i].ID != want || rows[i].Rank != i+1 {
			t.Errorf("row %d: contributor %d rank %d, want %d rank %d", i, rows[i].ID, rows[i].Rank, want, i+1)
		}
	}
	// Zoé: o/a (4 commits) before o/b (1).
	if got := rows[0].Repos; len(got) != 2 || got[0].RepoID != 2 || got[1].RepoID != 1 {
		t.Errorf("repositories of Zoé: %+v", got)
	}
	// Yann: tie of commits and date, broken by the display name (o/a before o/b).
	if got := rows[1].Repos; len(got) != 2 || got[0].RepoDisplayName() != "o/a" {
		t.Errorf("repositories of Yann: %+v", got)
	}
	if len(rows[2].Repos) != 0 {
		t.Errorf("Xavier without detail: %+v", rows[2].Repos)
	}
	if rows := buildContributorRows(nil, breakdown); len(rows) != 0 {
		t.Errorf("empty ranking: %+v", rows)
	}
}

func TestContributorRowShowLogin(t *testing.T) {
	cases := []struct {
		c    model.Contributor
		want bool
	}{
		{model.Contributor{Name: "Alice", ProviderLogin: "alice-gh"}, true},
		{model.Contributor{ProviderLogin: "alice-gh"}, false}, // the login is already the display name
		{model.Contributor{Name: "Bob"}, false},
	}
	for _, tc := range cases {
		row := contributorRow{ContributorStats: store.ContributorStats{Contributor: tc.c}}
		if got := row.ShowLogin(); got != tc.want {
			t.Errorf("ShowLogin(%+v) = %v, want %v", tc.c, got, tc.want)
		}
	}
}
