package web

import (
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// thresholdsOf describes the per-repository thresholds of repository id, in English ("Slacking 3 d ·
// Dead 10 d", "workspace" = thresholds of the workspace).
func (e *testEnv) thresholdsOf(id int64) string {
	e.t.Helper()
	r := e.mustRepo(id)
	return thresholdsText(en, r.FlemmeAfterDays, r.DeadAfterDays)
}

func TestRepoThresholdsForm(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed() // bob: last commit 9 days ago (Slacking with the thresholds 5 / 15)

	w := e.get(repoPath(ids.Bob))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="repo-thresholds"`, `action="/repos/2/thresholds"`, `name="flemme" value=""`,
		`name="dead" value=""`, `name="csrf_token"`, "Slacking after 5 days (the workspace&#39;s)",
		"Dead after 15 days (the workspace&#39;s)", "Slacking · 9 d")
	assertNotContains(t, w, `id="repo-thresholds" class="collapse collapse-arrow bg-base-100 shadow-sm" open`)

	// Save: redirection, thresholds written, state computed again (bob becomes Active).
	w = e.post(repoPath(ids.Bob)+"/thresholds", url.Values{"flemme": {" 10 "}, "dead": {"20"}}, false)
	assertRedirect(t, w, w.Code, repoPath(ids.Bob)+"?msg=thresholds#repo-thresholds")
	if got := e.thresholdsOf(ids.Bob); got != "Slacking 10 d · Dead 20 d" {
		t.Errorf("saved thresholds: %s", got)
	}
	w = e.get(repoPath(ids.Bob) + "?msg=thresholds")
	assertContains(t, w, html.EscapeString(en.T("web.thresholds.saved")), `name="flemme" value="10"`, `name="dead" value="20"`,
		"Slacking after 10 days (this repository&#39;s)", "Dead after 20 days (this repository&#39;s)", "Active")

	// Empty field (or 0): threshold of the workspace.
	w = e.post(repoPath(ids.Bob)+"/thresholds", url.Values{"flemme": {""}, "dead": {"0"}}, false)
	assertStatus(t, w, http.StatusSeeOther)
	if got := e.thresholdsOf(ids.Bob); got != "workspace" {
		t.Errorf("thresholds after the reset: %s", got)
	}

	// HTMX: HX-Redirect.
	w = e.post(repoPath(ids.Bob)+"/thresholds", url.Values{"dead": {"8"}}, true)
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("HX-Redirect"), repoPath(ids.Bob)) {
		t.Errorf("HTMX: %d, HX-Redirect %q", w.Code, w.Header().Get("HX-Redirect"))
	}
	if got := e.thresholdsOf(ids.Bob); got != "Slacking workspace · Dead 8 d" {
		t.Errorf("thresholds: %s", got)
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
		{url.Values{"flemme": {"-3"}}, "Slacking: &#34;-3&#34; : nombre entier de jours attendu"},
		{url.Values{"dead": {"<b>x</b>"}}, "Dead: &#34;&lt;b&gt;x&lt;/b&gt;&#34;"},
		{url.Values{"dead": {"4000"}}, "de 1 à 3650"},
	} {
		w := e.post(repoPath(ids.Bob)+"/thresholds", tc.form, false)
		assertStatus(t, w, http.StatusBadRequest)
		assertContains(t, w, "Thresholds refused", tc.want, `class="collapse collapse-arrow bg-base-100 shadow-sm" open`)
		assertNotContains(t, w, "<b>x</b>")
		if got := e.thresholdsOf(ids.Bob); got != "workspace" {
			t.Errorf("%v: thresholds written despite the error: %s", tc.form, got)
		}
	}
	// Input shown again.
	assertContains(t, e.post(repoPath(ids.Bob)+"/thresholds", url.Values{"flemme": {"10"}, "dead": {"5"}}, false),
		`name="flemme" value="10"`, `name="dead" value="5"`)

	// Unknown repository: 404.
	assertStatus(t, e.post("/repos/999/thresholds", url.Values{"flemme": {"3"}}, false), http.StatusNotFound)
	assertStatus(t, e.post("/repos/999/thresholds", url.Values{"flemme": {"x"}}, false), http.StatusNotFound)
}

func TestRepoThresholdsRequireCSRF(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	target := repoPath(ids.Bob) + "/thresholds"
	form := url.Values{"flemme": {"10"}}

	// Without token or cookie.
	r := e.request(http.MethodPost, target, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", testOrigin)
	assertStatus(t, e.do(r), http.StatusForbidden)

	// Valid token, foreign origin.
	ck := e.csrfCookie()
	f := url.Values{CSRFField: {ck.Value}, "flemme": {"10"}}
	r = e.request(http.MethodPost, target, strings.NewReader(f.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "http://evil.example.test")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.AddCookie(ck)
	assertStatus(t, e.do(r), http.StatusForbidden)

	if got := e.thresholdsOf(ids.Bob); got != "workspace" {
		t.Errorf("thresholds written without CSRF: %s", got)
	}
}

// Import preview: "Thresholds" column (before → after).
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
	assertContains(t, w, "<th>Thresholds</th>", "Slacking workspace · Dead 20 d</span> → Slacking 3 d · Dead workspace")
}
