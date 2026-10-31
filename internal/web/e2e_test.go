package web

// Test de bout en bout de la phase 4 : l'interface web complète (routeur, middlewares, CSRF), le
// vrai moteur de synchro et le vrai coordinateur, le vrai provider GitHub (internal/provider/github)
// branché sur le faux serveur githubtest, un faux gh (authtest) et une base SQLite temporaire. Aucun
// appel réseau réel (127.0.0.1) ; données factices uniquement. Le harness vérifie qu'aucune réponse
// ni le journal ne contiennent le token ou un e-mail factice.

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/github"
	"github.com/studoo-app/watchoo/internal/provider/github/githubtest"
)

func TestWebEndToEnd(t *testing.T) {
	srv := githubtest.NewServer(t, githubtest.Options{Now: func() time.Time { return testNow },
		Token: authtest.FakeToken})
	t.Cleanup(srv.Close)
	cache, quota := github.NewCache(0, 0), github.NewQuota()
	e := newTestEnv(t, withProvider(func(token string) (provider.Provider, error) {
		return github.New(token, github.Options{BaseURL: srv.URL(), Cache: cache, Quota: quota,
			UserAgent: "watchoo/test", Logger: slog.Default()})
	}))

	alice := githubtest.Person{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh"}
	bob := githubtest.Person{Name: "Bob Durand", Email: "bob@example.test"} // sans compte GitHub
	carol := githubtest.Person{Name: "Carol", Email: "carol@example.test", Login: "carol-gh"}
	commit := func(repo int64, msg string, p githubtest.Person, ago time.Duration, parents ...string) string {
		return srv.AddCommit(repo, githubtest.Commit{Message: msg, Author: p, AuthoredAt: testNow.Add(-ago),
			Parents: parents, Additions: 3, Deletions: 1})
	}
	// alice/projet : 60 commits anciens (pagination), puis deux commits récents, et une branche dev.
	srv.AddRepo(githubtest.Repo{ID: 1, Owner: "alice", Name: "projet", Private: true})
	old := srv.AppendCommits(1, "main", 60, testNow.Add(-80*24*time.Hour), time.Hour)
	a1 := commit(1, "e2e-alice-tests", alice, 50*time.Hour, old[len(old)-1])
	a2 := commit(1, xssMessage, alice, 2*time.Hour, a1)
	srv.SetBranch(1, "main", a2)
	srv.SetBranch(1, "dev", commit(1, "e2e-bob-essai-dev", bob, time.Hour, a2))
	// Une pull request ouverte et un tag (V2).
	srv.SetPullRequests(1, githubtest.PullRequest{Number: 7, Title: "e2e-pr-ouverte", Author: "alice-gh",
		CreatedAt: testNow.Add(-3 * time.Hour)})
	srv.SetTags(1, githubtest.Tag{Name: "v1.0", SHA: a2})
	// bob/projet : Flemme (9 jours) ; carol/projet : Dead (42 jours) ; cache : inaccessible.
	srv.AddRepo(githubtest.Repo{ID: 2, Owner: "bob", Name: "projet", Private: true})
	srv.SetBranch(2, "main", commit(2, "e2e-bob-init", bob, 9*24*time.Hour))
	srv.AddRepo(githubtest.Repo{ID: 3, Owner: "carol", Name: "projet", Private: true})
	srv.SetBranch(3, "main", commit(3, "e2e-carol-rendu", carol, 42*24*time.Hour))
	srv.AddRepo(githubtest.Repo{ID: 4, Owner: "carol", Name: "cache", Private: true})
	srv.SetRepoAccess(4, githubtest.AccessHidden)

	// 1. Espace vide : le tableau de bord renvoie vers les réglages.
	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, testWorkspace, `href="/settings"`)

	// 2. Ajout des dépôts depuis les réglages (POST avec CSRF) ; le dépôt caché est refusé.
	addRepo := func(ref, label, branches string) int64 {
		t.Helper()
		w := e.post("/settings/repos", url.Values{"repo": {ref}, "label": {label}, "branches": {branches}}, false)
		assertStatus(t, w, http.StatusSeeOther)
		loc, err := url.Parse(w.Header().Get("Location"))
		if err != nil || loc.Path != "/settings" || loc.Query().Get("msg") != msgAdded {
			t.Fatalf("ajout de %s : Location %q", ref, w.Header().Get("Location"))
		}
		id, err := strconv.ParseInt(loc.Query().Get("repo"), 10, 64)
		if err != nil || id <= 0 {
			t.Fatalf("ajout de %s : identifiant %q", ref, loc.Query().Get("repo"))
		}
		return id
	}
	aliceID := addRepo("alice/projet", "Alice Martin", "main, dev")
	bobID := addRepo("https://github.com/bob/projet", "", "")
	carolID := addRepo("carol/projet", "", "")
	w = e.post("/settings/repos", url.Values{"repo": {"carol/cache"}}, false)
	assertStatus(t, w, http.StatusUnprocessableEntity)
	if repos, err := e.store.ListRepositories(t.Context()); err != nil || len(repos) != 3 {
		t.Fatalf("dépôts en base = %d (%v), attendu 3", len(repos), err)
	}
	// Une requête sans jeton CSRF ne crée rien.
	r := e.request(http.MethodPost, "/settings/repos", strings.NewReader("repo=carol%2Fautre"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", testOrigin)
	assertStatus(t, e.do(r), http.StatusForbidden)

	w = e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Never synced", "Alice Martin", "bob/projet")

	// 3. Synchro manuelle depuis le bouton (HTMX), progression, puis fin signalée par HX-Trigger.
	w = e.post("/sync", nil, true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="sync-status"`, msgSyncStarted)
	st := e.waitSyncIdle()
	if !st.HasReport || st.LastReport.NewCommits() != 65 {
		t.Fatalf("rapport de synchro = %+v", st.LastReport)
	}
	w = e.htmx("/fragments/sync?gen=0&run=1")
	assertStatus(t, w, http.StatusOK)
	if got := w.Header().Get("HX-Trigger"); got != SyncFinishedEvent {
		t.Errorf("HX-Trigger = %q, attendu %q", got, SyncFinishedEvent)
	}
	assertContains(t, w, "Synchro")
	assertNotContains(t, w, "every 2s")

	// 4. Tableau de bord : les trois états, le fil, le message échappé.
	w = e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "badge-success", "badge-warning", "badge-error", "Slacking · 9 j", "Dead · 42 j",
		"&lt;script&gt;", "e2e-bob-essai-dev")
	assertNotContains(t, w, xssMessage, "Never synced")
	// Contenu de la page seulement : l'indicateur de synchro (navbar) peut citer des dépôts dans
	// l'infobulle de ses avertissements (clones Git locaux indisponibles dans ce test).
	body := w.Body.String()
	body = body[strings.Index(body, `id="main"`):]
	if c, b, a := strings.Index(body, "carol/projet"), strings.Index(body, "bob/projet"),
		strings.Index(body, "Alice Martin"); c < 0 || b < 0 || a < 0 || c > b || b > a {
		t.Errorf("ordre des cartes : carol %d, bob %d, alice %d (Dead, Flemme, Actif attendu)", c, b, a)
	}

	// 5. Détail d'alice : 63 commits (60 + 2 sur main, 1 sur dev), deux pages de 50, filtres branche et auteur, graphique.
	alicePath := repoPath(aliceID)
	w = e.get(alicePath)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Alice Martin", `id="activity-data"`, "&lt;script&gt;", "Synchroniser ce dépôt",
		"#7 e2e-pr-ouverte", "1 ouverte", ">v1.0<")
	assertNotContains(t, w, xssMessage)
	// Page 1 : les 50 plus récents (3 commits récents, puis « commit 60 » à « commit 14 »).
	assertContains(t, w, "commit 14", "?page=2")
	assertNotContains(t, w, "commit 13<")
	w = e.htmx(alicePath + "/commits?page=2")
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, "<html", "e2e-alice-tests", "commit 14<")
	assertContains(t, w, "commit 13", "commit 1<", "?page=1")
	w = e.htmx(alicePath + "/commits?branch=dev")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "e2e-bob-essai-dev")
	w = e.htmx(alicePath + "/commits?author=alice-gh")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "e2e-alice-tests", "&lt;script&gt;")
	assertNotContains(t, w, "e2e-bob-essai-dev", "commit 12")

	// 6. Synchro d'un seul dépôt : rien de nouveau, aucun commit re-téléchargé.
	w = e.post(repoPath(bobID)+"/sync", nil, true)
	assertStatus(t, w, http.StatusOK)
	if st := e.waitSyncIdle(); st.LastReport.NewCommits() != 0 {
		t.Errorf("deuxième synchro : %d nouveaux commits, attendu 0", st.LastReport.NewCommits())
	}

	// 7. Contributeurs : classement global avec les quatre auteurs.
	w = e.get("/contributors")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Alice Martin", "Bob Durand", "Carol", "Étudiant Test", repoPath(carolID))

	// 7 bis. Page d'un contributeur (0.5.0) : liée depuis le classement, avec le profil donné par
	// l'API GitHub (html_url du compte), ses dépôts et ses commits.
	aliceContrib := e.contributorIDs()["Alice Martin"]
	assertContains(t, w, `href="`+contributorPath(aliceContrib)+`"`)
	w = e.get(contributorPath(aliceContrib))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `href="https://github.com/alice-gh"`, `aria-label="Profil GitHub de alice-gh"`,
		repoPath(aliceID), "e2e-alice-tests")
	assertNoLeak(t, "page d'un contributeur", w.Body.String())

	// 8. Retrait depuis les réglages : arrêt du suivi (historique gardé), puis suppression.
	w = e.get("/settings/repos/" + strconv.FormatInt(carolID, 10) + "/remove")
	assertStatus(t, w, http.StatusOK)
	w = e.post("/settings/repos/"+strconv.FormatInt(carolID, 10)+"/remove", url.Values{"keep_history": {"1"}}, false)
	assertStatus(t, w, http.StatusSeeOther)
	if repo, err := e.store.GetRepository(t.Context(), carolID); err != nil || repo.Enabled {
		t.Errorf("carol après arrêt du suivi : %+v (%v)", repo, err)
	}
	assertStatus(t, e.get(repoPath(carolID)), http.StatusOK) // historique gardé
	w = e.post("/settings/repos/"+strconv.FormatInt(bobID, 10)+"/remove", nil, false)
	assertStatus(t, w, http.StatusSeeOther)
	assertStatus(t, e.get(repoPath(bobID)), http.StatusNotFound)
	w = e.get("/settings")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Alice Martin", "carol/projet")
	if main := w.Body.String()[strings.Index(w.Body.String(), `id="main"`):]; strings.Contains(main, "bob/projet") {
		t.Error("bob/projet encore dans les réglages après sa suppression")
	}

	// 9. Le faux GitHub n'a reçu que des lectures, toutes avec le token (jamais envoyé au navigateur).
	for _, req := range srv.Requests() {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			t.Errorf("requête GitHub %s %s : l'accès doit rester en lecture seule", req.Method, req.Path)
		}
		if !req.ValidToken {
			t.Errorf("requête GitHub %s sans le token attendu", req.Path)
		}
	}
}
