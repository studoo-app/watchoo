package web

import (
	"net/http"
	"testing"
)

// TestHeadAcceptedWhereGetIs checks that every GET route accepts HEAD (same code, same
// Content-Type) and that a POST-only route still refuses it.
func TestHeadAcceptedWhereGetIs(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	targets := []string{
		"/", "/fragments/dashboard/repos", "/fragments/dashboard/feed",
		repoPath(ids.Alice), repoPath(ids.Alice) + "/commits",
		"/contributors", "/fragments/contributors", "/contributors/1", "/contributors/1/commits",
		"/settings/repos", "/settings/sync", "/settings/share", "/settings" + repoPath(ids.Alice) + "/remove", "/fragments/quota", "/fragments/sync",
		"/conventions",
		"/static/app.js", "/favicon.ico", "/apple-touch-icon.png", "/manifest.webmanifest",
	}
	for _, target := range targets {
		get := e.get(target)
		head := e.do(e.request(http.MethodHead, target, nil))
		if get.Code != http.StatusOK || head.Code != get.Code {
			t.Errorf("%s: GET %d, HEAD %d; want 200 for both", target, get.Code, head.Code)
			continue
		}
		if g, h := get.Header().Get("Content-Type"), head.Header().Get("Content-Type"); g != h {
			t.Errorf("%s: Content-Type GET %q, HEAD %q", target, g, h)
		}
	}
	// /settings redirects to the page of the topic (1.8.0), for HEAD too.
	if get, head := e.get("/settings"), e.do(e.request(http.MethodHead, "/settings", nil)); get.Code != http.StatusFound || head.Code != http.StatusFound {
		t.Errorf("/settings: GET %d, HEAD %d; want 302 for both", get.Code, head.Code)
	}
	for _, target := range []string{"/sync", "/settings/interval"} {
		if w := e.do(e.request(http.MethodHead, target, nil)); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("HEAD %s: %d, want 405", target, w.Code)
		}
	}
	if w := e.do(e.request(http.MethodHead, "/page-inconnue", nil)); w.Code != http.StatusNotFound {
		t.Errorf("HEAD /page-inconnue: %d, want 404", w.Code)
	}
}

func TestIsAssetPath(t *testing.T) {
	for path, want := range map[string]bool{
		"/static/app.js": true, "/static/vendor/htmx-2.0.11.min.js": true, "/favicon.ico": true,
		"/apple-touch-icon.png": true, "/manifest.webmanifest": true,
		"/": false, "/static": false, "/contributors": false, "/fragments/sync": false, "/favicon.ico/x": false,
	} {
		if got := isAssetPath(path); got != want {
			t.Errorf("isAssetPath(%q) = %v, want %v", path, got, want)
		}
	}
}
