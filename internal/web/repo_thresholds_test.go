package web

import (
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// thresholdsOf décrit les seuils propres du dépôt id (« 3/10 », « -/- » = seuils de l'espace).
func (e *testEnv) thresholdsOf(id int64) string {
	e.t.Helper()
	r := e.mustRepo(id)
	return thresholdsText(r.FlemmeAfterDays, r.DeadAfterDays)
}

func TestRepoThresholdsForm(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed() // bob : dernier commit il y a 9 jours (Flemme avec les seuils 5 / 15)

	w := e.get(repoPath(ids.Bob))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="repo-thresholds"`, `action="/repos/2/thresholds"`, `name="flemme" value=""`,
		`name="dead" value=""`, `name="csrf_token"`, "Flemme après 5 jours (de l'espace)",
		"Dead après 15 jours (de l'espace)", "Flemme · 9 j")
	assertNotContains(t, w, `id="repo-thresholds" class="collapse collapse-arrow bg-base-100 shadow-sm" open`)

	// Enregistrement : redirection, seuils écrits, état recalculé (bob devient Actif).
	w = e.post(repoPath(ids.Bob)+"/thresholds", url.Values{"flemme": {" 10 "}, "dead": {"20"}}, false)
	assertRedirect(t, w, w.Code, repoPath(ids.Bob)+"?msg=thresholds#repo-thresholds")
	if got := e.thresholdsOf(ids.Bob); got != "Flemme 10 j · Dead 20 j" {
		t.Errorf("seuils enregistrés : %s", got)
	}
	w = e.get(repoPath(ids.Bob) + "?msg=thresholds")
	assertContains(t, w, html.EscapeString(thresholdsSaved), `name="flemme" value="10"`, `name="dead" value="20"`,
		"Flemme après 10 jours (propre à ce dépôt)", "Dead après 20 jours (propre à ce dépôt)", "Actif")

	// Champ vide (ou 0) : seuil de l'espace.
	w = e.post(repoPath(ids.Bob)+"/thresholds", url.Values{"flemme": {""}, "dead": {"0"}}, false)
	assertStatus(t, w, http.StatusSeeOther)
	if got := e.thresholdsOf(ids.Bob); got != "espace" {
		t.Errorf("seuils après remise à zéro : %s", got)
	}

	// HTMX : HX-Redirect.
	w = e.post(repoPath(ids.Bob)+"/thresholds", url.Values{"dead": {"8"}}, true)
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("HX-Redirect"), repoPath(ids.Bob)) {
		t.Errorf("HTMX : %d, HX-Redirect %q", w.Code, w.Header().Get("HX-Redirect"))
	}
	if got := e.thresholdsOf(ids.Bob); got != "Flemme espace · Dead 8 j" {
		t.Errorf("seuils : %s", got)
	}
}

func TestRepoThresholdsInvalid(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	for _, tc := range []struct {
		form url.Values
		want string
	}{
		{url.Values{"flemme": {"10"}, "dead": {"5"}}, "doit être strictement supérieur au seuil flemme"},
		{url.Values{"flemme": {"-3"}}, "Flemme : &#34;-3&#34; : nombre entier de jours attendu"},
		{url.Values{"dead": {"<b>x</b>"}}, "Dead : &#34;&lt;b&gt;x&lt;/b&gt;&#34;"},
		{url.Values{"dead": {"4000"}}, "de 1 à 3650"},
	} {
		w := e.post(repoPath(ids.Bob)+"/thresholds", tc.form, false)
		assertStatus(t, w, http.StatusBadRequest)
		assertContains(t, w, "Seuils refusés", tc.want, `class="collapse collapse-arrow bg-base-100 shadow-sm" open`)
		assertNotContains(t, w, "<b>x</b>")
		if got := e.thresholdsOf(ids.Bob); got != "espace" {
			t.Errorf("%v : seuils écrits malgré l'erreur : %s", tc.form, got)
		}
	}
	// Saisie réaffichée.
	assertContains(t, e.post(repoPath(ids.Bob)+"/thresholds", url.Values{"flemme": {"10"}, "dead": {"5"}}, false),
		`name="flemme" value="10"`, `name="dead" value="5"`)

	// Dépôt inconnu : 404.
	assertStatus(t, e.post("/repos/999/thresholds", url.Values{"flemme": {"3"}}, false), http.StatusNotFound)
	assertStatus(t, e.post("/repos/999/thresholds", url.Values{"flemme": {"x"}}, false), http.StatusNotFound)
}

func TestRepoThresholdsRequireCSRF(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	target := repoPath(ids.Bob) + "/thresholds"
	form := url.Values{"flemme": {"10"}}

	// Sans jeton ni cookie.
	r := e.request(http.MethodPost, target, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", testOrigin)
	assertStatus(t, e.do(r), http.StatusForbidden)

	// Jeton valide, origine étrangère.
	ck := e.csrfCookie()
	f := url.Values{CSRFField: {ck.Value}, "flemme": {"10"}}
	r = e.request(http.MethodPost, target, strings.NewReader(f.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "http://evil.example.test")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.AddCookie(ck)
	assertStatus(t, e.do(r), http.StatusForbidden)

	if got := e.thresholdsOf(ids.Bob); got != "espace" {
		t.Errorf("seuils écrits sans CSRF : %s", got)
	}
}

// Aperçu d'un import : colonne « Seuils » (avant → après).
func TestSettingsImportPreviewThresholds(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	ids := e.seed()
	e.withShareWorkspace()
	dead := 20
	if _, err := e.store.SetRepositoryThresholds(t.Context(), ids.Bob, nil, &dead); err != nil {
		t.Fatal(err)
	}
	doc := `{"format":"watchoo-config","version":1,
	  "repositories":[{"repo":"promo/bob","flemme_after_days":3,"dead_after_days":0}]}`
	w := e.postMultipart("/settings/import", nil, []byte(doc), true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "<th>Seuils</th>", "Flemme espace · Dead 20 j</span> → Flemme 3 j · Dead espace")
}
