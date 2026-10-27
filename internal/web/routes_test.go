package web

import (
	"net/http"
	"testing"
)

// TestHeadAcceptedWhereGetIs vérifie que toute route GET accepte HEAD (même code, même
// Content-Type) et qu'une route POST seule la refuse toujours.
func TestHeadAcceptedWhereGetIs(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	targets := []string{
		"/", "/fragments/dashboard/repos", "/fragments/dashboard/feed",
		repoPath(ids.Alice), repoPath(ids.Alice) + "/commits",
		"/contributors", "/fragments/contributors", "/contributors/1", "/contributors/1/commits",
		"/settings", "/settings" + repoPath(ids.Alice) + "/remove", "/fragments/quota", "/fragments/sync",
		"/static/app.js", "/favicon.ico", "/apple-touch-icon.png", "/manifest.webmanifest",
	}
	for _, target := range targets {
		get := e.get(target)
		head := e.do(e.request(http.MethodHead, target, nil))
		if get.Code != http.StatusOK || head.Code != get.Code {
			t.Errorf("%s : GET %d, HEAD %d ; attendu 200 tous les deux", target, get.Code, head.Code)
			continue
		}
		if g, h := get.Header().Get("Content-Type"), head.Header().Get("Content-Type"); g != h {
			t.Errorf("%s : Content-Type GET %q, HEAD %q", target, g, h)
		}
	}
	for _, target := range []string{"/sync", "/settings/interval"} {
		if w := e.do(e.request(http.MethodHead, target, nil)); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("HEAD %s : %d, attendu 405", target, w.Code)
		}
	}
	if w := e.do(e.request(http.MethodHead, "/page-inconnue", nil)); w.Code != http.StatusNotFound {
		t.Errorf("HEAD /page-inconnue : %d, attendu 404", w.Code)
	}
}

func TestIsAssetPath(t *testing.T) {
	for path, want := range map[string]bool{
		"/static/app.js": true, "/static/vendor/htmx-2.0.11.min.js": true, "/favicon.ico": true,
		"/apple-touch-icon.png": true, "/manifest.webmanifest": true,
		"/": false, "/static": false, "/contributors": false, "/fragments/sync": false, "/favicon.ico/x": false,
	} {
		if got := isAssetPath(path); got != want {
			t.Errorf("isAssetPath(%q) = %v, attendu %v", path, got, want)
		}
	}
}
