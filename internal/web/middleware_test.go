package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	e := newTestEnv(t)
	for _, target := range []string{"/fragments/sync", "/static/app.css", "/page-inconnue", "/favicon.ico"} {
		w := e.get(target)
		h := w.Header()
		csp := h.Get("Content-Security-Policy")
		for _, part := range []string{"default-src 'self'", "script-src 'self'", "style-src 'self'",
			"img-src 'self' https://avatars.githubusercontent.com data:", "frame-ancestors 'none'"} {
			if !strings.Contains(csp, part) {
				t.Errorf("%s : CSP sans %q : %q", target, part, csp)
			}
		}
		for _, bad := range []string{"unsafe-inline", "unsafe-eval"} {
			if strings.Contains(csp, bad) {
				t.Errorf("%s : CSP avec %s", target, bad)
			}
		}
		for name, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"Referrer-Policy":        "no-referrer",
			"X-Frame-Options":        "DENY",
		} {
			if got := h.Get(name); got != want {
				t.Errorf("%s : %s = %q, attendu %q", target, name, got, want)
			}
		}
	}
	if got := e.get("/fragments/sync").Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("fragment : Cache-Control = %q, attendu no-store", got)
	}
}

func TestNotFoundAndMethodNotAllowed(t *testing.T) {
	e := newTestEnv(t)
	for _, target := range []string{"/nexiste-pas", "/repos/abc", "/repos/0x1", "/repos/12345678901234567890",
		"/static/inconnu.js", "/static/../web.go", "/static/"} {
		w := e.get(target)
		assertStatus(t, w, http.StatusNotFound)
		assertContains(t, w, "Page introuvable", `lang="fr"`, testWorkspace)
		assertNotContains(t, w, "goroutine", ".go:")
	}
	w := e.htmx("/nexiste-pas")
	assertStatus(t, w, http.StatusNotFound)
	assertContains(t, w, `role="alert"`, "Page introuvable")
	assertNotContains(t, w, "<html")
	// Une méthode qui modifierait quelque chose passe d'abord par le CSRF : refusée sans jeton.
	assertStatus(t, e.do(e.request(http.MethodDelete, "/", nil)), http.StatusForbidden)
}

func TestRecoverer(t *testing.T) {
	e := newTestEnv(t)
	h := e.server.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("détail interne secret")
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, e.request(http.MethodGet, "/", nil))
	assertStatus(t, w, http.StatusInternalServerError)
	assertContains(t, w, "Erreur interne")
	assertNotContains(t, w, "détail interne secret")
	if strings.Contains(e.logs.String(), "détail interne secret") {
		t.Error("valeur de la panique dans le journal")
	}

	defer func() {
		if rv := recover(); rv != http.ErrAbortHandler {
			t.Errorf("ErrAbortHandler non relancée : %v", rv)
		}
	}()
	abort := e.server.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	abort.ServeHTTP(httptest.NewRecorder(), e.request(http.MethodGet, "/", nil))
}

func TestAccessLogWithoutQueryOrPersonalData(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	e.get(repoPath(ids.Alice) + "?author=alice-gh&branch=secret-branch")
	logs := e.logs.String()
	if !strings.Contains(logs, "route=/repos/{id}") {
		t.Errorf("journal sans le motif de route :\n%s", logs)
	}
	for _, bad := range []string{"alice-gh", "secret-branch", "author=", testHost} {
		if strings.Contains(logs, bad) {
			t.Errorf("journal avec %q :\n%s", bad, logs)
		}
	}
}

func TestBodyLimit(t *testing.T) {
	e := newTestEnv(t)
	ck := e.csrfCookie()
	big := strings.Repeat("a", MaxBodyBytes+1)
	form := url.Values{"repo": {big}}
	r := e.request(http.MethodPost, "/sync", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", testOrigin)
	r.AddCookie(ck)
	// Le jeton ne peut pas être lu dans un corps trop gros : refus, rien n'est lancé.
	assertStatus(t, e.do(r), http.StatusForbidden)
	if e.coord.State().Generation != 0 {
		t.Error("synchro lancée malgré un corps trop gros")
	}
}
