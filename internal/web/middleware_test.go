package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/configshare"
)

func TestSecurityHeaders(t *testing.T) {
	e := newTestEnv(t)
	for _, target := range []string{"/fragments/sync", "/static/app.css", "/unknown-page", "/favicon.ico"} {
		w := e.get(target)
		h := w.Header()
		csp := h.Get("Content-Security-Policy")
		for _, part := range []string{"default-src 'self'", "script-src 'self'", "style-src 'self'",
			"img-src 'self' https://avatars.githubusercontent.com data:", "frame-ancestors 'none'"} {
			if !strings.Contains(csp, part) {
				t.Errorf("%s: CSP without %q: %q", target, part, csp)
			}
		}
		for _, bad := range []string{"unsafe-inline", "unsafe-eval"} {
			if strings.Contains(csp, bad) {
				t.Errorf("%s: CSP with %s", target, bad)
			}
		}
		for name, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"Referrer-Policy":        "no-referrer",
			"X-Frame-Options":        "DENY",
		} {
			if got := h.Get(name); got != want {
				t.Errorf("%s: %s = %q, want %q", target, name, got, want)
			}
		}
	}
	if got := e.get("/fragments/sync").Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("fragment: Cache-Control = %q, want no-store", got)
	}
}

func TestNotFoundAndMethodNotAllowed(t *testing.T) {
	e := newTestEnv(t)
	for _, target := range []string{"/does-not-exist", "/repos/abc", "/repos/0x1", "/repos/12345678901234567890",
		"/static/unknown.js", "/static/../web.go", "/static/"} {
		w := e.get(target)
		assertStatus(t, w, http.StatusNotFound)
		assertContains(t, w, "Page not found", `lang="en"`, testWorkspace, "Back to the dashboard")
		assertNotContains(t, w, "goroutine", ".go:")
	}
	w := e.htmx("/does-not-exist")
	assertStatus(t, w, http.StatusNotFound)
	assertContains(t, w, `role="alert"`, "<strong>Page not found</strong>: This page does not exist")
	assertNotContains(t, w, "<html")
	// A method that would change something goes through the CSRF first: refused without token.
	assertStatus(t, e.do(e.request(http.MethodDelete, "/", nil)), http.StatusForbidden)
}

func TestRecoverer(t *testing.T) {
	e := newTestEnv(t)
	h := e.server.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret internal detail")
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, e.request(http.MethodGet, "/", nil))
	assertStatus(t, w, http.StatusInternalServerError)
	assertContains(t, w, "Internal error")
	assertNotContains(t, w, "secret internal detail")
	if strings.Contains(e.logs.String(), "secret internal detail") {
		t.Error("value of the panic in the log")
	}

	defer func() {
		if rv := recover(); rv != http.ErrAbortHandler {
			t.Errorf("ErrAbortHandler not raised again: %v", rv)
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
		t.Errorf("log without the route pattern:\n%s", logs)
	}
	for _, bad := range []string{"alice-gh", "secret-branch", "author=", testHost} {
		if strings.Contains(logs, bad) {
			t.Errorf("log with %q:\n%s", bad, logs)
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
	// The token cannot be read in a body too large: refused (413), nothing is started.
	assertStatus(t, e.do(r), http.StatusRequestEntityTooLarge)
	if e.coord.State().Generation != 0 {
		t.Error("sync started despite a body too large")
	}
}

// multipartBody builds a multipart form with the CSRF token and a file of size bytes.
func multipartBody(t *testing.T, token string, size int) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField(CSRFField, token); err != nil {
		t.Fatal(err)
	}
	fw, err := mw.CreateFormFile("file", "config.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(bytes.Repeat([]byte("a"), size)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

// The import routes accept a file up to 1 MiB (beyond MaxBodyBytes); beyond MaxImportBodyBytes: 413.
// The other routes keep MaxBodyBytes, even in multipart.
func TestImportBodyLimit(t *testing.T) {
	e := newTestEnv(t)
	ck := e.csrfCookie()
	post := func(path string, size int) int {
		body, ctype := multipartBody(t, ck.Value, size)
		r := e.request(http.MethodPost, path, body)
		r.Header.Set("Content-Type", ctype)
		r.Header.Set("Origin", testOrigin)
		r.AddCookie(ck)
		return e.do(r).Code
	}
	for _, path := range []string{"/settings/import", "/settings/import/apply"} {
		// The request passes the limit and the CSRF; the handler answers.
		if code := post(path, configshare.MaxFileSize); code == http.StatusRequestEntityTooLarge || code == http.StatusForbidden {
			t.Errorf("%s: file of 1 MiB refused (%d)", path, code)
		}
		if code := post(path, MaxImportBodyBytes+1); code != http.StatusRequestEntityTooLarge {
			t.Errorf("%s: body too large, code %d instead of 413", path, code)
		}
	}
	if code := post("/sync", MaxBodyBytes+1); code != http.StatusRequestEntityTooLarge {
		t.Errorf("/sync: multipart body too large, code %d instead of 413", code)
	}
	if e.coord.State().Generation != 0 {
		t.Error("sync started despite a body too large")
	}
}
