package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCSRFCookie(t *testing.T) {
	e := newTestEnv(t)
	ck := e.csrfCookie()
	if ck.Name != "watchoo_csrf_8080" {
		t.Errorf("nom du cookie %q, attendu watchoo_csrf_8080 (port compris)", ck.Name)
	}
	if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.Path != "/" || !validCSRFToken(ck.Value) {
		t.Errorf("cookie %+v : attendu HttpOnly, SameSite=Strict, Path=/, jeton bien formé", ck)
	}
	// Un cookie valide est gardé : pas de nouveau Set-Cookie.
	r := e.request(http.MethodGet, "/fragments/sync", nil)
	r.AddCookie(ck)
	if w := e.do(r); len(w.Result().Cookies()) != 0 {
		t.Errorf("nouveau cookie posé alors que le jeton était valide : %v", w.Result().Cookies())
	}
	// Un cookie mal formé est remplacé.
	r = e.request(http.MethodGet, "/fragments/sync", nil)
	r.AddCookie(&http.Cookie{Name: ck.Name, Value: "court"})
	if w := e.do(r); len(w.Result().Cookies()) != 1 {
		t.Errorf("cookie invalide non remplacé")
	}
	// Le jeton est dans la page (hx-headers du <body>, via la page d'erreur ici).
	r = e.request(http.MethodGet, "/page-inconnue", nil)
	r.AddCookie(ck)
	assertContains(t, e.do(r), `X-CSRF-Token`, ck.Value)
}

// postRequest construit un POST /sync avec les en-têtes et le cookie donnés.
func (e *testEnv) postRequest(form url.Values, headers map[string]string, ck *http.Cookie) *http.Request {
	r := e.request(http.MethodPost, "/sync", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	if ck != nil {
		r.AddCookie(ck)
	}
	return r
}

func TestCSRFRejectsForgedPosts(t *testing.T) {
	e := newTestEnv(t)
	ck := e.csrfCookie()
	other := newCSRFToken()
	sameOrigin := map[string]string{"Origin": testOrigin, "Sec-Fetch-Site": "same-origin"}
	cases := []struct {
		name    string
		form    url.Values
		headers map[string]string
		cookie  *http.Cookie
		message string
	}{
		{"sans jeton", url.Values{}, sameOrigin, ck, "jeton de sécurité"},
		{"sans cookie", url.Values{CSRFField: {ck.Value}}, sameOrigin, nil, "jeton de sécurité"},
		{"mauvais jeton", url.Values{CSRFField: {other}}, sameOrigin, ck, "jeton de sécurité"},
		{"mauvais jeton en en-tête", url.Values{}, map[string]string{"Origin": testOrigin, CSRFHeader: other}, ck, "jeton de sécurité"},
		{"autre site (Sec-Fetch-Site)", url.Values{CSRFField: {ck.Value}}, map[string]string{"Sec-Fetch-Site": "cross-site"}, ck, "depuis l&#39;interface"},
		{"autre origine", url.Values{CSRFField: {ck.Value}}, map[string]string{"Origin": "http://evil.example.test"}, ck, "depuis l&#39;interface"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := e.do(e.postRequest(tc.form, tc.headers, tc.cookie))
			assertStatus(t, w, http.StatusForbidden)
			assertContains(t, w, "Requête refusée", tc.message)
		})
	}
	time.Sleep(10 * time.Millisecond)
	if st := e.coord.State(); st.Generation != 0 {
		t.Errorf("une synchro a été lancée par une requête refusée : %+v", st)
	}
}

func TestCSRFAcceptsTokenInHeader(t *testing.T) {
	e := newTestEnv(t)
	ck := e.csrfCookie()
	w := e.do(e.postRequest(url.Values{}, map[string]string{
		"Origin": testOrigin, "Sec-Fetch-Site": "same-origin", CSRFHeader: ck.Value, "HX-Request": "true",
	}, ck))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, msgSyncStarted)
	e.waitSyncIdle()
}

func TestNoActionInGet(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	for _, target := range []string{"/sync", repoPath(ids.Alice) + "/sync", "/settings/repos",
		"/settings/interval", repoPathSettings(ids.Alice), repoPath(ids.Alice) + "/thresholds"} {
		w := e.get(target)
		assertStatus(t, w, http.StatusMethodNotAllowed)
		assertContains(t, w, "Action impossible")
	}
	time.Sleep(10 * time.Millisecond)
	if e.coord.State().Generation != 0 {
		t.Error("une synchro a été lancée par un GET")
	}
	if _, err := e.store.GetRepository(t.Context(), ids.Alice); err != nil {
		t.Errorf("dépôt modifié par un GET : %v", err)
	}
}

func repoPathSettings(id int64) string {
	return "/settings/repos/" + strings.TrimPrefix(repoPath(id), "/repos/")
}

// TestCSRFCookieOnlyOnHTML vérifie que le cookie CSRF n'est posé qu'avec une page ou un fragment
// HTML, jamais avec un fichier statique.
func TestCSRFCookieOnlyOnHTML(t *testing.T) {
	e := newTestEnv(t)
	for _, target := range []string{"/static/app.js", "/static/app.css", "/favicon.ico", "/apple-touch-icon.png",
		"/manifest.webmanifest", "/static/inconnu.js"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			if w := e.do(e.request(method, target, nil)); len(w.Result().Cookies()) != 0 {
				t.Errorf("%s %s : cookie posé %v", method, target, w.Result().Cookies())
			}
		}
	}
	for _, target := range []string{"/", "/contributors", "/fragments/sync", "/page-inconnue"} {
		if w := e.get(target); len(w.Result().Cookies()) != 1 {
			t.Errorf("GET %s : cookies %v, attendu le cookie CSRF", target, w.Result().Cookies())
		}
	}
	// Un POST sur un chemin statique reste refusé (pas de jeton).
	r := e.postRequest(nil, map[string]string{"Origin": testOrigin, "Sec-Fetch-Site": "same-origin"}, nil)
	r.URL.Path = "/static/app.js"
	if w := e.do(r); w.Code != http.StatusForbidden {
		t.Errorf("POST /static/app.js : %d, attendu 403", w.Code)
	}
}
