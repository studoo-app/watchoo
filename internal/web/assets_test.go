package web

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticAssets(t *testing.T) {
	e := newTestEnv(t)
	cases := map[string]struct{ ctype, cache string }{
		"/static/app.css":                    {"text/css; charset=utf-8", cacheRevalidate},
		"/static/app.js":                     {"text/javascript; charset=utf-8", cacheRevalidate},
		"/static/theme.js":                   {"text/javascript; charset=utf-8", cacheRevalidate},
		"/static/" + htmxFile:                {"text/javascript; charset=utf-8", cacheImmutable},
		"/static/" + chartFile:               {"text/javascript; charset=utf-8", cacheImmutable},
		"/static/watchoo-icone.svg":          {"image/svg+xml", cacheRevalidate},
		"/static/icon-192.png":               {"image/png", cacheRevalidate},
		"/favicon.ico":                       {"image/x-icon", cacheRevalidate},
		"/apple-touch-icon.png":              {"image/png", cacheRevalidate},
		"/manifest.webmanifest":              {"application/manifest+json", cacheRevalidate},
		"/static/vendor/htmx-2.0.11.LICENSE": {"text/plain; charset=utf-8", cacheImmutable},
	}
	for target, want := range cases {
		w := e.get(target)
		assertStatus(t, w, http.StatusOK)
		if got := w.Header().Get("Content-Type"); got != want.ctype {
			t.Errorf("%s: Content-Type %q, want %q", target, got, want.ctype)
		}
		if got := w.Header().Get("Cache-Control"); got != want.cache {
			t.Errorf("%s: Cache-Control %q, want %q", target, got, want.cache)
		}
		if w.Body.Len() == 0 {
			t.Errorf("%s: empty body", target)
		}
		etag := w.Header().Get("ETag")
		if etag == "" {
			t.Errorf("%s: no ETag", target)
			continue
		}
		r := e.request(http.MethodGet, target, nil)
		r.Header.Set("If-None-Match", etag)
		if w := e.do(r); w.Code != http.StatusNotModified || w.Body.Len() != 0 {
			t.Errorf("%s: If-None-Match → %d (%d bytes), want an empty 304", target, w.Code, w.Body.Len())
		}
	}
}

func TestAppCSSGenerated(t *testing.T) {
	css, err := os.ReadFile("static/app.css")
	if err != nil {
		t.Fatalf("static/app.css missing (make css): %v", err)
	}
	if len(css) < 1000 {
		t.Fatalf("static/app.css too small (%d bytes): make css", len(css))
	}
	for _, class := range []string{".badge-success", ".badge-warning", ".badge-error", ".badge-neutral", ".navbar", ".btn"} {
		if !bytes.Contains(css, []byte(class)) {
			t.Errorf("static/app.css without %s: run make css again", class)
		}
	}
	if !bytes.Contains(css, []byte("prefers-color-scheme:dark")) {
		t.Error("static/app.css without a dark theme (prefersdark)")
	}
}

func TestManifest(t *testing.T) {
	data, err := os.ReadFile("static/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Name  string `json:"name"`
		Icons []struct {
			Src string `json:"src"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid manifest: %v", err)
	}
	if m.Name != "Watchoo" || len(m.Icons) == 0 {
		t.Errorf("manifest: %+v", m)
	}
	for _, icon := range m.Icons {
		if _, err := os.Stat(filepath.Join("static", strings.TrimPrefix(icon.Src, "/static/"))); err != nil {
			t.Errorf("icon %s missing: %v", icon.Src, err)
		}
	}
}

// TestLogoCopies checks that the embedded logos are identical to assets/logo (otherwise: make logo).
func TestLogoCopies(t *testing.T) {
	for _, name := range []string{"watchoo-icone.svg", "watchoo-logo-final-nom.svg", "watchoo-logo-final-nom-dark.svg",
		"favicon.ico", "apple-touch-icon.png", "icon-192.png", "icon-512.png"} {
		want, err := os.ReadFile(filepath.Join("..", "..", "assets", "logo", name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join("static", name))
		if err != nil {
			t.Fatalf("static/%s missing (make logo): %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("static/%s differs from assets/logo/%s: run make logo again", name, name)
		}
	}
}

// TestIconCopies checks that the embedded icons of the AI score are identical to assets/icon (make logo).
func TestIconCopies(t *testing.T) {
	for _, name := range []string{"score-ia-max.svg", "score-ia.svg", "score-not-ia.svg"} {
		want, err := os.ReadFile(filepath.Join("..", "..", "assets", "icon", name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join("static", name))
		if err != nil {
			t.Fatalf("static/%s missing (make logo): %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("static/%s differs from assets/icon/%s: run make logo again", name, name)
		}
	}
}

// TestVendoredChecksums checks the checksums recorded in tailwind/VERSIONS.
func TestVendoredChecksums(t *testing.T) {
	f, err := os.Open("tailwind/VERSIONS")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	checked := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			t.Errorf("malformed line: %q", line)
			continue
		}
		path, sum := fields[0], fields[2]
		if path == "tailwindcss" {
			continue // CLI of the CI, not vendored
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != sum {
			t.Errorf("%s: checksum %x, VERSIONS says %s", path, got, sum)
		}
		checked++
	}
	if checked < 6 {
		t.Errorf("only %d files checked", checked)
	}
	for _, file := range []string{htmxFile, chartFile} {
		if !strings.Contains(readString(t, "tailwind/VERSIONS"), "static/"+file) {
			t.Errorf("%s missing from VERSIONS", file)
		}
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
