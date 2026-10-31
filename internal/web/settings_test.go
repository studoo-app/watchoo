package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
)

// repoCount renvoie le nombre de dépôts en base.
func (e *testEnv) repoCount() int {
	e.t.Helper()
	repos, err := e.store.ListRepositories(e.t.Context())
	if err != nil {
		e.t.Fatal(err)
	}
	return len(repos)
}

// mustRepo lit le dépôt id.
func (e *testEnv) mustRepo(id int64) model.Repository {
	e.t.Helper()
	r, err := e.store.GetRepository(e.t.Context(), id)
	if err != nil {
		e.t.Fatalf("GetRepository(%d) : %v", id, err)
	}
	return r
}

// assertRedirect vérifie une redirection 303 vers want.
func assertRedirect(t *testing.T, w interface {
	Header() http.Header
}, code int, want string) {
	t.Helper()
	if code != http.StatusSeeOther {
		t.Fatalf("code %d, attendu 303", code)
	}
	if got := w.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, attendu %q", got, want)
	}
}

func TestSettingsPage(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	if _, err := e.store.UpdateRepositorySettings(t.Context(), ids.Bob, "<script>bob</script>", []string{"main", "release/*"}); err != nil {
		t.Fatal(err)
	}

	w := e.get("/settings")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		"<title>Réglages · "+testWorkspace+" · Watchoo</title>",
		`aria-current="page">Réglages`,
		"Alice Martin", "promo/alice", "promo/dave", "toutes les branches", "main, release/*",
		"Actif", "Dead · 40 j", "Jamais synchronisé",
		`action="/settings/repos/`+strconv.FormatInt(ids.Alice, 10)+`"`,
		`href="/settings/repos/`+strconv.FormatInt(ids.Carol, 10)+`/remove"`,
		`action="/settings/repos"`, `action="/settings/interval"`, `action="/sync"`,
		`name="csrf_token"`, "Synchroniser maintenant",
		// Quota chargé après la page.
		`hx-get="/fragments/quota" hx-trigger="load"`,
		// Intervalle : manuelle seule dans l'environnement de test, fichier à 30 min.
		"Intervalle en vigueur : <strong>manuelle seule</strong>", "ce formulaire ne réécrit jamais config.yaml",
		"&lt;script&gt;bob&lt;/script&gt;",
	)
	assertNotContains(t, w, "<script>bob", "Reprendre le suivi")
}

func TestSettingsFlashMessages(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	w := e.get("/settings?msg=saved&repo=" + strconv.FormatInt(ids.Alice, 10))
	assertContains(t, w, "Le dépôt « Alice Martin » est modifié")
	// Code inconnu : aucun message, rien de la requête n'est recopié.
	w = e.get("/settings?msg=%3Cscript%3E&repo=abc")
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, "alert-info", "&lt;script&gt;")
	// Dépôt inconnu : message sans nom.
	w = e.get("/settings?msg=added&repo=999")
	assertContains(t, w, "Le dépôt est suivi")
}

func TestSettingsAddRepository(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	e.fake.AddRepository(provider.Repository{ID: 50, Owner: "promo", Name: "eve", DefaultBranch: "main",
		URL: "https://github.com/promo/eve", Private: true})
	before := e.fake.Calls(providertest.MethodListCommits)

	w := e.post("/settings/repos", url.Values{"repo": {"https://github.com/promo/eve"}, "label": {"Ève"},
		"branches": {"main, dev,main"}}, false)
	repo, err := e.store.GetRepositoryByName(t.Context(), model.ProviderGitHub, "promo", "eve")
	if err != nil {
		t.Fatalf("dépôt non créé : %v", err)
	}
	assertRedirect(t, w, w.Code, "/settings?msg=added&repo="+strconv.FormatInt(repo.ID, 10))
	if repo.Label != "Ève" || strings.Join(repo.BranchPatterns, ",") != "main,dev" || !repo.Enabled {
		t.Errorf("dépôt enregistré : %+v", repo)
	}
	// Aucune synchro lancée.
	if e.fake.Calls(providertest.MethodListCommits) != before || e.coord.State().Generation != 0 {
		t.Error("l'ajout a lancé une synchro")
	}
	w = e.get(w.Header().Get("Location"))
	assertContains(t, w, "Le dépôt « Ève » est suivi")

	// Déjà suivi, sans changement ; puis avec un nouveau label (HTMX : HX-Redirect).
	w = e.post("/settings/repos", url.Values{"repo": {"promo/eve"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=already&repo="+strconv.FormatInt(repo.ID, 10))
	w = e.post("/settings/repos", url.Values{"repo": {"promo/eve"}, "label": {"Eve B."}}, true)
	assertStatus(t, w, http.StatusOK)
	if got := w.Header().Get("HX-Redirect"); got != "/settings?msg=updated&repo="+strconv.FormatInt(repo.ID, 10) {
		t.Errorf("HX-Redirect = %q", got)
	}
	if got := e.mustRepo(repo.ID); got.Label != "Eve B." || strings.Join(got.BranchPatterns, ",") != "main,dev" {
		t.Errorf("mise à jour : %+v (les branches ne doivent pas changer sans saisie)", got)
	}
}

func TestSettingsAddRepositoryErrors(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	count := e.repoCount()
	cases := []struct {
		name   string
		form   url.Values
		status int
		want   []string
	}{
		{"référence invalide", url.Values{"repo": {"<b>pas un dépôt</b>"}}, http.StatusBadRequest,
			[]string{"is invalid", `value="&lt;b&gt;pas un dépôt&lt;/b&gt;"`}},
		{"URL avec identifiants", url.Values{"repo": {"https://user:secret@github.com/promo/x"}}, http.StatusBadRequest,
			[]string{"identifiants interdits"}},
		{"label avec caractère de contrôle", url.Values{"repo": {"promo/eve"}, "label": {"a\x1b[31mb"}}, http.StatusBadRequest,
			[]string{"caractère de contrôle"}},
		{"motif de branche invalide", url.Values{"repo": {"promo/eve"}, "branches": {"main,[x"}}, http.StatusBadRequest,
			[]string{"motif de branche"}},
		{"dépôt inaccessible", url.Values{"repo": {"promo/inconnu"}, "label": {"Inconnu"}}, http.StatusUnprocessableEntity,
			[]string{"Dépôt promo/inconnu inaccessible", `value="Inconnu"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := e.post("/settings/repos", tc.form, false)
			assertStatus(t, w, tc.status)
			assertContains(t, w, append(tc.want, "Ajouter un dépôt", "alert-error")...)
			assertNotContains(t, w, "secret", "<b>pas")
			if got := e.repoCount(); got != count {
				t.Errorf("%d dépôts après l'échec, attendu %d", got, count)
			}
		})
	}

	// Erreur de GitHub (réseau…) : message générique, détail dans le journal seulement.
	e.fake.SetError(providertest.MethodRepository, errors.New("connexion refusée"))
	w := e.post("/settings/repos", url.Values{"repo": {"promo/eve"}}, false)
	assertStatus(t, w, http.StatusBadGateway)
	assertContains(t, w, "Ajout du dépôt promo/eve impossible pour le moment")
	assertNotContains(t, w, "connexion refusée")
}

func TestSettingsUpdateRepository(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	target := "/settings/repos/" + strconv.FormatInt(ids.Bob, 10)

	w := e.post(target, url.Values{"label": {"Bob D."}, "branches": {"main, feature/*"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=saved&repo="+strconv.FormatInt(ids.Bob, 10))
	if got := e.mustRepo(ids.Bob); got.Label != "Bob D." || strings.Join(got.BranchPatterns, ",") != "main,feature/*" {
		t.Errorf("dépôt après modification : %+v", got)
	}

	// Saisie invalide : rien ne change, le formulaire de ce dépôt est rouvert avec l'erreur.
	w = e.post(target, url.Values{"label": {"ok"}, "branches": {"[x"}}, false)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "Modification refusée", `open>`, `value="[x"`)
	if got := e.mustRepo(ids.Bob); got.Label != "Bob D." {
		t.Errorf("dépôt modifié malgré l'erreur : %+v", got)
	}

	// Vide : plus de label, toutes les branches.
	w = e.post(target, url.Values{"label": {""}, "branches": {" "}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=saved&repo="+strconv.FormatInt(ids.Bob, 10))
	if got := e.mustRepo(ids.Bob); got.Label != "" || len(got.BranchPatterns) != 0 {
		t.Errorf("dépôt après remise à zéro : %+v", got)
	}

	// Dépôt inconnu : 404, saisie valide ou non.
	assertStatus(t, e.post("/settings/repos/999", url.Values{"label": {"x"}}, false), http.StatusNotFound)
	assertStatus(t, e.post("/settings/repos/999", url.Values{"branches": {"[x"}}, false), http.StatusNotFound)
}

func TestSettingsRemoveRepository(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	confirm := "/settings/repos/" + strconv.FormatInt(ids.Alice, 10) + "/remove"

	// La confirmation ne modifie rien.
	w := e.get(confirm)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Retirer « Alice Martin » ?", "3 commits en base", `name="keep_history" value="1"`,
		`action="`+confirm+`"`, "Supprimer le dépôt et son historique", "Arrêter le suivi, garder l'historique")
	if !e.mustRepo(ids.Alice).Enabled {
		t.Fatal("GET de confirmation : suivi arrêté")
	}
	assertContains(t, w, "les commits en base (3 commits)")
	assertStatus(t, e.get("/settings/repos/999/remove"), http.StatusNotFound)

	// Non-régression : accord avec 0 commit (pas de « ses 0 commit »).
	empty := e.addRepo(99, "vide", "Dépôt vide")
	w = e.get("/settings/repos/" + strconv.FormatInt(empty, 10) + "/remove")
	assertContains(t, w, "les commits en base (0 commit)")
	assertNotContains(t, w, "ses 0", "ses 1")

	// Garder l'historique : suivi arrêté.
	w = e.post(confirm, url.Values{"keep_history": {"1"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=stopped&repo="+strconv.FormatInt(ids.Alice, 10))
	if e.mustRepo(ids.Alice).Enabled {
		t.Error("suivi non arrêté")
	}
	w = e.post(confirm, url.Values{"keep_history": {"1"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=already_stopped&repo="+strconv.FormatInt(ids.Alice, 10))
	w = e.get("/settings")
	assertContains(t, w, "Suivi arrêté", "Reprendre le suivi", `name="repo" value="promo/alice"`)
	assertContains(t, e.get(confirm), "suivi déjà arrêté")

	// Reprendre le suivi : comme « repo add ».
	w = e.post("/settings/repos", url.Values{"repo": {"promo/alice"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=updated&repo="+strconv.FormatInt(ids.Alice, 10))
	if !e.mustRepo(ids.Alice).Enabled {
		t.Error("suivi non repris")
	}

	// Supprimer avec l'historique.
	w = e.post(confirm, nil, false)
	assertRedirect(t, w, w.Code, "/settings?msg=deleted")
	if _, err := e.store.GetRepository(t.Context(), ids.Alice); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("dépôt toujours en base : %v", err)
	}
	assertStatus(t, e.post(confirm, nil, false), http.StatusNotFound)
	assertContains(t, e.get("/settings?msg=deleted"), "Le dépôt et son historique sont supprimés")
}

func TestSettingsRemoveRefusedDuringSync(t *testing.T) {
	gate := newGateSyncer()
	e := newTestEnv(t, withSyncer(gate))
	ids := e.seed()
	assertStatus(t, e.post("/sync", nil, true), http.StatusOK)
	<-gate.started

	confirm := "/settings/repos/" + strconv.FormatInt(ids.Bob, 10) + "/remove"
	// Le bouton n'est pas désactivé au rendu (il le resterait après la synchro) : le POST refuse.
	page := e.get(confirm)
	assertContains(t, page, "Une synchro était en cours", "Supprimer le dépôt et son historique")
	assertNotContains(t, page, " disabled")
	w := e.post(confirm, nil, false)
	assertStatus(t, w, http.StatusConflict)
	assertContains(t, w, "Suppression impossible")
	e.mustRepo(ids.Bob)
	// Arrêter le suivi reste possible.
	w = e.post(confirm, url.Values{"keep_history": {"1"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=stopped&repo="+strconv.FormatInt(ids.Bob, 10))

	close(gate.release)
	e.waitSyncIdle()
	w = e.post(confirm, nil, false)
	assertRedirect(t, w, w.Code, "/settings?msg=deleted")
}

func TestSettingsInterval(t *testing.T) {
	e := newTestEnv(t)
	cases := []struct {
		input string
		want  time.Duration
	}{
		{"30m", 30 * time.Minute},
		{"45", 45 * time.Minute},
		{"1h30m", 90 * time.Minute},
		{"0", 0},
	}
	for _, tc := range cases {
		w := e.post("/settings/interval", url.Values{"interval": {tc.input}}, false)
		assertRedirect(t, w, w.Code, "/settings?msg=interval")
		if got := e.coord.Interval(); got != tc.want {
			t.Errorf("%q : intervalle %s, attendu %s", tc.input, got, tc.want)
		}
	}

	assertStatus(t, e.post("/settings/interval", url.Values{"interval": {"2h"}}, false), http.StatusSeeOther)
	w := e.get("/settings?msg=interval")
	assertContains(t, w, "Intervalle en vigueur : <strong>2 h</strong>", "Prochaine synchro",
		"changé pour ce processus", "config.yaml n&#39;est pas modifié")

	for _, input := range []string{"5m", "-1h", "abc", ""} {
		w := e.post("/settings/interval", url.Values{"interval": {input}}, false)
		assertStatus(t, w, http.StatusBadRequest)
		assertContains(t, w, "alert-error")
		if got := e.coord.Interval(); got != 2*time.Hour {
			t.Errorf("%q : intervalle changé en %s", input, got)
		}
	}
	assertContains(t, e.post("/settings/interval", url.Values{"interval": {"5m"}}, false), "minimum de 15 min")
	// GET sur l'action : 405, rien ne change.
	assertStatus(t, e.get("/settings/interval?interval=0"), http.StatusMethodNotAllowed)
	if e.coord.Interval() != 2*time.Hour {
		t.Error("GET a modifié l'intervalle")
	}
}

func TestParseIntervalInput(t *testing.T) {
	ok := map[string]time.Duration{"0": 0, "15": 15 * time.Minute, " 1h ": time.Hour, "90m": 90 * time.Minute}
	for in, want := range ok {
		if got, err := parseIntervalInput(in); err != nil || got != want {
			t.Errorf("parseIntervalInput(%q) = %s, %v ; attendu %s", in, got, err, want)
		}
	}
	for _, in := range []string{"", "  ", "abc", "-5", "1j", "99999999"} {
		if _, err := parseIntervalInput(in); err == nil {
			t.Errorf("parseIntervalInput(%q) sans erreur", in)
		}
	}
}

func TestSettingsQuota(t *testing.T) {
	e := newTestEnv(t)
	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 4000, Reset: testNow.Add(30 * time.Minute)})
	w := e.htmx("/fragments/quota")
	assertStatus(t, w, http.StatusOK)
	if !strings.HasPrefix(strings.TrimSpace(w.Body.String()), `<div id="settings-quota"`) {
		t.Errorf("fragment inattendu :\n%s", w.Body.String())
	}
	assertContains(t, w, ">4000</span>", "/ 5000 requêtes restantes", "80 %", ">10:30</time>",
		`hx-trigger="sync-finished from:body"`)
	assertNotContains(t, w, "quota bas", "<html")

	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 100, Reset: testNow.Add(30 * time.Minute)})
	assertContains(t, e.htmx("/fragments/quota"), "quota bas", "synchros reportées jusque-là")

	// Token illisible : quota inconnu, avec le message, sans token.
	e.auth.SetTokenErr(errors.New("gh n'est pas connecté"))
	w = e.htmx("/fragments/quota")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "quota inconnu", "lecture du quota")

	// gh non connecté : clones Git locaux, sans quota (V2).
	e.auth.SetTokenErr(auth.ErrNotLoggedIn)
	w = e.htmx("/fragments/quota")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "sans objet", "clones Git locaux", "gh auth login")
	assertNotContains(t, w, "quota inconnu")
}

func TestSettingsActionsRequireCSRF(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	e.fake.AddRepository(provider.Repository{ID: 50, Owner: "promo", Name: "eve", DefaultBranch: "main"})
	count := e.repoCount()
	id := strconv.FormatInt(ids.Bob, 10)
	targets := map[string]url.Values{
		"/settings/repos":                   {"repo": {"promo/eve"}},
		"/settings/repos/" + id:             {"label": {"pirate"}},
		"/settings/repos/" + id + "/remove": {},
		"/settings/interval":                {"interval": {"0"}},
	}
	for target, form := range targets {
		// Sans jeton ni cookie.
		r := e.request(http.MethodPost, target, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", testOrigin)
		assertStatus(t, e.do(r), http.StatusForbidden)

		// Jeton valide, mais origine étrangère.
		ck := e.csrfCookie()
		f := url.Values{CSRFField: {ck.Value}}
		for k, v := range form {
			f[k] = v
		}
		r = e.request(http.MethodPost, target, strings.NewReader(f.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "http://evil.example.test")
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		r.AddCookie(ck)
		assertStatus(t, e.do(r), http.StatusForbidden)
	}
	if got := e.repoCount(); got != count {
		t.Errorf("%d dépôts, attendu %d", got, count)
	}
	if got := e.mustRepo(ids.Bob); got.Label != "" || !got.Enabled {
		t.Errorf("dépôt modifié sans CSRF : %+v", got)
	}
	if e.coord.Interval() != 0 {
		t.Error("intervalle modifié sans CSRF")
	}
}

// TestSettingsQuotaCanceled vérifie qu'une lecture du quota interrompue (requête abandonnée, arrêt
// de serve : contexte annulé) n'est journalisée qu'en debug.
func TestSettingsQuotaCanceled(t *testing.T) {
	e := newTestEnv(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := e.request(http.MethodGet, "/fragments/quota", nil).WithContext(ctx)
	r.Header.Set("HX-Request", "true")
	e.do(r)
	logs := e.logs.String()
	if !strings.Contains(logs, `level=DEBUG msg="lecture du quota GitHub interrompue"`) ||
		strings.Contains(logs, "quota GitHub illisible") {
		t.Errorf("journal inattendu :\n%s", logs)
	}

	// Une vraie erreur reste un avertissement.
	e.auth.SetTokenErr(errors.New("gh n'est pas connecté"))
	e.htmx("/fragments/quota")
	if !strings.Contains(e.logs.String(), `level=WARN msg="quota GitHub illisible"`) {
		t.Errorf("avertissement absent :\n%s", e.logs.String())
	}
}
