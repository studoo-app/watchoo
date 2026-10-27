package web

// Page d'un contributeur « /contributors/{id} » (0.5.0) et liens vers elle et vers les profils.

import (
	"html"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// contributorIDs renvoie l'identifiant interne des contributeurs auteurs, par nom affiché.
func (e *testEnv) contributorIDs() map[string]int64 {
	e.t.Helper()
	ranking, err := e.store.ContributorRanking(e.t.Context(), store.ContributorFilter{})
	if err != nil {
		e.t.Fatalf("ContributorRanking : %v", err)
	}
	out := make(map[string]int64, len(ranking))
	for _, c := range ranking {
		out[c.DisplayName()] = c.ID
	}
	return out
}

// hostileName est un nom de contributeur piégé : il doit toujours apparaître échappé.
const hostileName = `<img src=x onerror="alert(2)">Eve`

// seedHostile ajoute au dépôt repoID (promo/name) deux commits d'un contributeur au nom piégé, avec des
// statistiques de lignes, et un commit dont il n'est que le committer (l'auteur est « Zoé »).
func (e *testEnv) seedHostile(repoID int64, name string) (eve, onlyCommitter store.Identity) {
	e.t.Helper()
	eve = store.Identity{Name: hostileName, Email: "eve@example.test"}
	zoe := store.Identity{Name: "Zoé", Email: "zoe@example.test"}
	onlyCommitter = store.Identity{Name: "Fred Committer", Email: "fred@example.test"}
	c1 := e.commitWrite(10, eve, 2*24*time.Hour, "travail d'Eve", "main")
	c1.Additions, c1.Deletions = 10, 2
	c2 := e.commitWrite(11, eve, 24*time.Hour, "<b>suite</b>", "main")
	c2.Additions = 5
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
		`<h1 id="contributor-title"`, "@alice-gh", "1 identité Git connue",
		// Lien de profil : l'adresse enregistrée, en lien secondaire, nouvel onglet.
		`href="https://github.com/alice-gh"`, `aria-label="Profil GitHub de alice-gh"`, `rel="noopener noreferrer"`,
		// Totaux : 2 commits dans 1 dépôt, lignes inconnues.
		`<div class="stat-value">2</div>`, "dans 1 dépôt", `title="Statistiques de lignes inconnues`,
		// Ses dépôts : lien, état du dépôt.
		`href="`+repoPath(ids.Alice)+`"`, "badge-success",
		// Ses commits : messages échappés, liens vers le dépôt.
		html.EscapeString(xssMessage), "premier commit", "2 commits",
		`hx-get="`+path+`/commits?page=1"`,
		// Graphiques : par jour et par dépôt (table des données, lien vers le dépôt).
		`id="contributor-activity-data"`, `data-refresh-url="`+path+`"`, `id="contributor-repos-chart-data"`,
		"Commits par semaine et par dépôt", "dans 1 dépôt depuis le",
		// Pull requests rapprochées par le compte (identifiant 10), titre échappé.
		`id="contributor-pulls"`, html.EscapeString(xssTitle), "mergée", "brouillon",
		// Fragments rafraîchis après une synchro.
		`hx-trigger="sync-finished from:body"`,
	)
	assertNotContains(t, w, xssMessage, xssTitle, "Bob Durand", "travail de Bob")
	assertNoLeak(t, "page d'un contributeur", w.Body.String())
}

func TestContributorPageWithoutAccount(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	e.seedHostile(ids.Dave, "dave")
	contribs := e.contributorIDs()

	// Bob : identité Git seule, ni profil ni pull requests.
	w := e.get(contributorPath(contribs["Bob Durand"]))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Bob Durand", "Non rapprochées", "2 commits", "dans 2 dépôts", "travail de Bob")
	assertNotContains(t, w, "Profil GitHub", "@")

	// Eve : nom piégé échappé partout, lignes connues.
	w = e.get(contributorPath(contribs[hostileName]))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, html.EscapeString(hostileName), `<span class="text-success">+15</span>`,
		`<span class="text-error">−2</span>`, html.EscapeString("<b>suite</b>"), "+10")
	assertNotContains(t, w, hostileName, "<b>suite</b>")

	// Carol : adresse de profil refusée (pas https://) : pas de lien.
	w = e.get(contributorPath(contribs["Carol"]))
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, "javascript:", "Profil GitHub", "evil.example.test")
	assertContains(t, w, "Aucune pull request de ce compte en base.")
	assertNoLeak(t, "pages des contributeurs", w.Body.String())
}

// TestContributorPageCommitterOnly vérifie la page d'un contributeur qui n'est l'auteur d'aucun commit
// (seulement committer).
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
		t.Fatal("committer introuvable")
	}
	w := e.get(contributorPath(fred))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Fred Committer", "0 commit", "Aucun commit dont il ou elle est l'auteur en base.",
		"Aucun commit depuis le", "jamais")
}

func TestContributorNotFoundAndFragments(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	for _, target := range []string{"/contributors/999999", "/contributors/0", "/contributors/abc",
		"/contributors/999999/commits", "/contributors/1234567890123456789"} {
		if w := e.get(target); w.Code != http.StatusNotFound {
			t.Errorf("GET %s : %d, attendu 404", target, w.Code)
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
			t.Errorf("fragment %s inattendu :\n%s", target, body)
		}
	}
	w := e.htmx(path + "/commits?page=7")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="contributor-commits"`, "premier commit")
	assertNotContains(t, w, "<html", "<script")
	// Sans HTMX : la page complète.
	if w := e.get(path + "/commits"); !strings.Contains(w.Body.String(), "<html") {
		t.Error("GET …/commits sans HTMX : page complète attendue")
	}
}

// TestContributorPagination vérifie l'historique paginé d'un contributeur (50 par page).
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
	assertContains(t, w, "55 commits · page 2 sur 2", `rel="prev"`, `href="`+path+`?page=1"`)
	if n := commitItems(w.Body.String()); n != 5 {
		t.Errorf("page 2 : %d commits, attendu 5", n)
	}
}

// TestContributorLinks vérifie les liens vers la page d'un contributeur (lien principal) et vers son
// profil (lien secondaire) depuis les autres pages.
func TestContributorLinks(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	contribs := e.contributorIDs()
	alice, bob := contributorPath(contribs["Alice Martin"]), contributorPath(contribs["Bob Durand"])
	profile := `href="https://github.com/alice-gh"`

	for _, target := range []string{"/contributors", "/", repoPath(ids.Alice)} {
		w := e.get(target)
		assertStatus(t, w, http.StatusOK)
		assertContains(t, w, `href="`+alice+`"`, `href="`+bob+`"`, profile, `aria-label="Profil GitHub de alice-gh"`)
		assertNotContains(t, w, "javascript:", `Profil GitHub de Bob`)
	}
	// Historique d'un dépôt (fragment) : auteur lié.
	w := e.htmx(repoPath(ids.Alice) + "/commits")
	assertContains(t, w, `href="`+alice+`"`, `href="`+bob+`"`, profile)
	// Table des données du graphique par contributeur : nom lié.
	w = e.get("/contributors")
	body := w.Body.String()
	table := body[strings.Index(body, `id="contributors-chart-table"`):]
	if !strings.Contains(table, `<a href="`+alice+`" class="link link-hover">Alice Martin</a>`) {
		t.Error("table du graphique : nom sans lien vers la page du contributeur")
	}
	// Aucune adresse construite par l'interface : un profil inconnu ne donne pas de lien.
	assertNotContains(t, w, "https://github.com/carol-gh")
}

// TestContributorPageLogs vérifie que le journal n'écrit ni nom, ni login, ni message, ni identifiant
// de contributeur : seulement le motif de route.
func TestContributorPageLogs(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	alice := e.contributorIDs()["Alice Martin"]
	e.get(contributorPath(alice))
	e.htmx(contributorPath(alice) + "/commits")
	logs := e.logs.String()
	if !strings.Contains(logs, "/contributors/{id") {
		t.Errorf("journal sans le motif de route :\n%s", logs)
	}
	for _, s := range []string{"Alice Martin", "alice-gh", "premier commit", "xss", contributorPath(alice)} {
		if strings.Contains(logs, s) {
			t.Errorf("journal : contient %q\n%s", s, logs)
		}
	}
	assertNoLeak(t, "journal", logs)
}

// TestContributorPageAfterSync vérifie qu'une synchro par le moteur (faux provider) remplit l'adresse
// de profil donnée par le provider et que la page la montre.
func TestContributorPageAfterSync(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	e.fake.AppendCommits(1, "main", 1, e.now.Add(-time.Hour), time.Minute)
	if _, err := e.engine.SyncRepo(t.Context(), ids.Alice, wsync.SyncOptions{}); err != nil {
		t.Fatalf("SyncRepo : %v", err)
	}
	id := e.contributorIDs()["Étudiant Test"]
	if id == 0 {
		t.Fatal("contributeur du faux provider absent")
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
			t.Errorf("profileURL(%q) = %q, attendu %q", in, got, want)
		}
	}
	if got := contributorPath(42); got != "/contributors/42" {
		t.Errorf("contributorPath(42) = %q", got)
	}
}
