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
		t.Errorf("cookie name %q, want watchoo_csrf_8080 (port included)", ck.Name)
	}
	if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.Path != "/" || !validCSRFToken(ck.Value) {
		t.Errorf("cookie %+v: want HttpOnly, SameSite=Strict, Path=/, well-formed token", ck)
	}
	// A valid cookie is kept: no new Set-Cookie.
	r := e.request(http.MethodGet, "/fragments/sync", nil)
	r.AddCookie(ck)
	if w := e.do(r); len(w.Result().Cookies()) != 0 {
		t.Errorf("new cookie set although the token was valid: %v", w.Result().Cookies())
	}
	// A malformed cookie is replaced.
	r = e.request(http.MethodGet, "/fragments/sync", nil)
	r.AddCookie(&http.Cookie{Name: ck.Name, Value: "short"})
	if w := e.do(r); len(w.Result().Cookies()) != 1 {
		t.Errorf("invalid cookie not replaced")
	}
	// The token is in the page (hx-headers of the <body>, through the error page here).
	r = e.request(http.MethodGet, "/unknown-page", nil)
	r.AddCookie(ck)
	assertContains(t, e.do(r), `X-CSRF-Token`, ck.Value)
}

// postRequest builds a POST /sync with the given headers and cookie.
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
		{"without token", url.Values{}, sameOrigin, ck, "security token"},
		{"without cookie", url.Values{CSRFField: {ck.Value}}, sameOrigin, nil, "security token"},
		{"wrong token", url.Values{CSRFField: {other}}, sameOrigin, ck, "security token"},
		{"wrong token in header", url.Values{}, map[string]string{"Origin": testOrigin, CSRFHeader: other}, ck, "security token"},
		{"other site (Sec-Fetch-Site)", url.Values{CSRFField: {ck.Value}}, map[string]string{"Sec-Fetch-Site": "cross-site"}, ck, "from the Watchoo interface"},
		{"other origin", url.Values{CSRFField: {ck.Value}}, map[string]string{"Origin": "http://evil.example.test"}, ck, "from the Watchoo interface"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := e.do(e.postRequest(tc.form, tc.headers, tc.cookie))
			assertStatus(t, w, http.StatusForbidden)
			assertContains(t, w, "Request refused", tc.message)
		})
	}
	time.Sleep(10 * time.Millisecond)
	if st := e.coord.State(); st.Generation != 0 {
		t.Errorf("a sync was started by a refused request: %+v", st)
	}
}

func TestCSRFAcceptsTokenInHeader(t *testing.T) {
	e := newTestEnv(t)
	ck := e.csrfCookie()
	w := e.do(e.postRequest(url.Values{}, map[string]string{
		"Origin": testOrigin, "Sec-Fetch-Site": "same-origin", CSRFHeader: ck.Value, "HX-Request": "true",
	}, ck))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Sync started.")
	e.waitSyncIdle()
}

func TestNoActionInGet(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	for _, target := range []string{"/sync", repoPath(ids.Alice) + "/sync", "/settings/repos",
		"/settings/interval", repoPathSettings(ids.Alice), repoPath(ids.Alice) + "/thresholds"} {
		w := e.get(target)
		assertStatus(t, w, http.StatusMethodNotAllowed)
		assertContains(t, w, "Action not possible")
	}
	time.Sleep(10 * time.Millisecond)
	if e.coord.State().Generation != 0 {
		t.Error("a sync was started by a GET")
	}
	if _, err := e.store.GetRepository(t.Context(), ids.Alice); err != nil {
		t.Errorf("repository changed by a GET: %v", err)
	}
}

func repoPathSettings(id int64) string {
	return "/settings/repos/" + strings.TrimPrefix(repoPath(id), "/repos/")
}

// TestCSRFCookieOnlyOnHTML checks that the CSRF cookie is set only with an HTML page or fragment,
// never with a static file.
func TestCSRFCookieOnlyOnHTML(t *testing.T) {
	e := newTestEnv(t)
	for _, target := range []string{"/static/app.js", "/static/app.css", "/favicon.ico", "/apple-touch-icon.png",
		"/manifest.webmanifest", "/static/unknown.js"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			if w := e.do(e.request(method, target, nil)); len(w.Result().Cookies()) != 0 {
				t.Errorf("%s %s: cookie set %v", method, target, w.Result().Cookies())
			}
		}
	}
	for _, target := range []string{"/", "/contributors", "/fragments/sync", "/unknown-page"} {
		if w := e.get(target); len(w.Result().Cookies()) != 1 {
			t.Errorf("GET %s: cookies %v, want the CSRF cookie", target, w.Result().Cookies())
		}
	}
	// A POST on a static path remains refused (no token).
	r := e.postRequest(nil, map[string]string{"Origin": testOrigin, "Sec-Fetch-Site": "same-origin"}, nil)
	r.URL.Path = "/static/app.js"
	if w := e.do(r); w.Code != http.StatusForbidden {
		t.Errorf("POST /static/app.js: %d, want 403", w.Code)
	}
}
