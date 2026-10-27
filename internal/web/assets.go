package web

// Fichiers embarqués : templates et fichiers statiques (CSS généré par « make css », HTMX et Chart.js
// vendored, logos copiés de assets/logo par « make logo », JavaScript de l'interface, manifeste).
// Servis sous /static/, avec un ETag (SHA-256 du contenu) et un Content-Type fixé par l'extension.

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Cache des fichiers statiques : les fichiers vendored ont leur version dans le nom (jamais
// modifiés : un an) ; les autres sont revalidés à chaque chargement (ETag). « private » : derrière
// l'authentification basique, un cache partagé ne doit pas les garder.
const (
	cacheImmutable  = "private, max-age=31536000, immutable"
	cacheRevalidate = "private, no-cache"
)

// contentTypes donne le Content-Type de chaque extension servie ; un fichier d'une autre extension
// n'est pas servi.
var contentTypes = map[string]string{
	".css":         "text/css; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".ico":         "image/x-icon",
	".webmanifest": "application/manifest+json",
	".md":          "text/plain; charset=utf-8",
	".LICENSE":     "text/plain; charset=utf-8",
}

// asset est un fichier statique prêt à servir.
type asset struct {
	data        []byte
	contentType string
	etag        string
	cache       string
}

// assetStore associe un chemin relatif à static/ (« vendor/htmx-2.0.11.min.js ») à son fichier.
type assetStore struct {
	files map[string]asset
}

// loadAssets lit tous les fichiers de static/ (une fois, au démarrage).
func loadAssets() (*assetStore, error) {
	st := &assetStore{files: map[string]asset{}}
	err := fs.WalkDir(staticFS, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ctype, ok := contentTypes[path.Ext(p)]
		if !ok {
			return nil // extension non servie
		}
		data, err := staticFS.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		rel := strings.TrimPrefix(p, "static/")
		cache := cacheRevalidate
		if strings.HasPrefix(rel, "vendor/") {
			cache = cacheImmutable
		}
		st.files[rel] = asset{data: data, contentType: ctype, etag: `"` + hex.EncodeToString(sum[:16]) + `"`, cache: cache}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("interface web : fichiers statiques illisibles : %w", err)
	}
	for _, required := range []string{"app.css", "app.js", "theme.js", htmxFile, chartFile} {
		if a, ok := st.files[required]; !ok || len(a.data) == 0 {
			return nil, fmt.Errorf("interface web : fichier statique static/%s absent ou vide (make css ?)", required)
		}
	}
	return st, nil
}

// Fichiers vendored (versions consignées dans tailwind/VERSIONS).
const (
	htmxFile  = "vendor/htmx-2.0.11.min.js"
	chartFile = "vendor/chart-4.6.1.umd.min.js"
)

// serveAsset sert le fichier rel de static/ ; 404 s'il n'existe pas.
func (s *server) serveAsset(w http.ResponseWriter, r *http.Request, rel string) {
	a, ok := s.assets.files[rel]
	if !ok {
		s.notFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", a.contentType)
	h.Set("Cache-Control", a.cache)
	h.Set("ETag", a.etag)
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, a.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write(a.data)
}

// handleStatic sert /static/* .
func (s *server) handleStatic(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/static/")
	if rel == "" || strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") {
		s.notFound(w, r)
		return
	}
	s.serveAsset(w, r, rel)
}
