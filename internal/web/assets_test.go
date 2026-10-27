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
		"/static/watchoo.svg":                {"image/svg+xml", cacheRevalidate},
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
			t.Errorf("%s : Content-Type %q, attendu %q", target, got, want.ctype)
		}
		if got := w.Header().Get("Cache-Control"); got != want.cache {
			t.Errorf("%s : Cache-Control %q, attendu %q", target, got, want.cache)
		}
		if w.Body.Len() == 0 {
			t.Errorf("%s : corps vide", target)
		}
		etag := w.Header().Get("ETag")
		if etag == "" {
			t.Errorf("%s : pas d'ETag", target)
			continue
		}
		r := e.request(http.MethodGet, target, nil)
		r.Header.Set("If-None-Match", etag)
		if w := e.do(r); w.Code != http.StatusNotModified || w.Body.Len() != 0 {
			t.Errorf("%s : If-None-Match → %d (%d octets), attendu 304 vide", target, w.Code, w.Body.Len())
		}
	}
}

func TestAppCSSGenerated(t *testing.T) {
	css, err := os.ReadFile("static/app.css")
	if err != nil {
		t.Fatalf("static/app.css absent (make css) : %v", err)
	}
	if len(css) < 1000 {
		t.Fatalf("static/app.css trop petit (%d octets) : make css", len(css))
	}
	for _, class := range []string{".badge-success", ".badge-warning", ".badge-error", ".badge-neutral", ".navbar", ".btn"} {
		if !bytes.Contains(css, []byte(class)) {
			t.Errorf("static/app.css sans %s : relancez make css", class)
		}
	}
	if !bytes.Contains(css, []byte("prefers-color-scheme:dark")) {
		t.Error("static/app.css sans thème sombre (prefersdark)")
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
		t.Fatalf("manifeste invalide : %v", err)
	}
	if m.Name != "Watchoo" || len(m.Icons) == 0 {
		t.Errorf("manifeste : %+v", m)
	}
	for _, icon := range m.Icons {
		if _, err := os.Stat(filepath.Join("static", strings.TrimPrefix(icon.Src, "/static/"))); err != nil {
			t.Errorf("icône %s absente : %v", icon.Src, err)
		}
	}
}

// TestLogoCopies vérifie que les logos embarqués sont identiques à assets/logo (sinon : make logo).
func TestLogoCopies(t *testing.T) {
	for _, name := range []string{"watchoo.svg", "watchoo-light.svg", "watchoo-nom.svg", "watchoo-nom-light.svg",
		"favicon.ico", "apple-touch-icon.png", "icon-192.png", "icon-512.png"} {
		want, err := os.ReadFile(filepath.Join("..", "..", "assets", "logo", name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join("static", name))
		if err != nil {
			t.Fatalf("static/%s absent (make logo) : %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("static/%s diffère de assets/logo/%s : relancez make logo", name, name)
		}
	}
}

// TestVendoredChecksums vérifie les empreintes consignées dans tailwind/VERSIONS.
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
			t.Errorf("ligne mal formée : %q", line)
			continue
		}
		path, sum := fields[0], fields[2]
		if path == "tailwindcss" {
			continue // CLI de la CI, non vendored
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s : %v", path, err)
			continue
		}
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != sum {
			t.Errorf("%s : empreinte %x, VERSIONS indique %s", path, got, sum)
		}
		checked++
	}
	if checked < 6 {
		t.Errorf("%d fichiers vérifiés seulement", checked)
	}
	for _, file := range []string{htmxFile, chartFile} {
		if !strings.Contains(readString(t, "tailwind/VERSIONS"), "static/"+file) {
			t.Errorf("%s absent de VERSIONS", file)
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
