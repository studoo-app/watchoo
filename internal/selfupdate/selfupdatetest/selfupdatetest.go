// Package selfupdatetest simule les releases GitHub de Watchoo pour les tests de la mise à jour
// (internal/selfupdate, internal/cli) : aucun accès au réseau.
//
// Le faux serveur (httptest, en HTTP sur 127.0.0.1) a la disposition de
// https://github.com/studoo-app/watchoo/releases : /latest redirige vers /tag/vX.Y.Z ; /tag/vX.Y.Z
// répond une page vide ; /download/vX.Y.Z/<archive> et /download/vX.Y.Z/watchoo_X.Y.Z_checksums.txt
// servent une archive (tar.gz, ou zip pour windows) construite en mémoire et son fichier de sommes.
package selfupdatetest

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// DefaultBinary est le contenu du faux binaire watchoo des archives.
const DefaultBinary = "#!/bin/sh\necho faux-watchoo\n"

// File est un fichier d'une archive.
type File struct {
	Name string
	Body string
}

// Server est un faux serveur de releases. Ses champs peuvent être changés entre deux requêtes.
type Server struct {
	*httptest.Server

	mu sync.Mutex
	// Latest est le tag vers lequel /latest redirige (« v0.6.0 ») ; vide = /latest répond 200 sans
	// redirection (aucune release).
	Latest string
	// GOOS et GOARCH sont la plateforme de l'archive servie.
	GOOS, GOARCH string
	// Files est le contenu de l'archive ; nil = LICENSE et watchoo (ou watchoo.exe) avec DefaultBinary.
	Files []File
	// Checksums remplace le fichier de sommes s'il n'est pas vide.
	Checksums string
	// Delay retarde chaque réponse (test du délai de l'avis).
	Delay time.Duration

	requests atomic.Int64
	agents   []string
}

// New démarre un faux serveur dont la dernière release est latest, pour la plateforme goos / goarch ;
// il est arrêté à la fin du test.
func New(t testing.TB, latest, goos, goarch string) *Server {
	t.Helper()
	s := &Server{Latest: latest, GOOS: goos, GOARCH: goarch}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /latest", s.handleLatest)
	mux.HandleFunc("GET /tag/{tag}", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("GET /download/{tag}/{file}", s.handleDownload)
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		s.mu.Lock()
		s.agents = append(s.agents, r.UserAgent())
		delay := s.Delay
		s.mu.Unlock()
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

// Requests renvoie le nombre de requêtes reçues.
func (s *Server) Requests() int64 { return s.requests.Load() }

// UserAgents renvoie les en-têtes User-Agent reçus.
func (s *Server) UserAgents() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.agents...)
}

// Set change la dernière release.
func (s *Server) Set(latest string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Latest = latest
}

func (s *Server) handleLatest(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	latest := s.Latest
	s.mu.Unlock()
	if latest == "" {
		return // 200 sans redirection
	}
	http.Redirect(w, r, "/tag/"+latest, http.StatusFound)
}

// ArchiveName renvoie le nom de l'archive de tag servie (même règle que .goreleaser.yaml).
func (s *Server) ArchiveName(tag string) string {
	ext := ".tar.gz"
	if s.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("watchoo_%s_%s_%s%s", strings.TrimPrefix(tag, "v"), s.GOOS, s.GOARCH, ext)
}

// Archive renvoie l'archive servie.
func (s *Server) Archive() ([]byte, error) {
	s.mu.Lock()
	files := s.Files
	goos := s.GOOS
	s.mu.Unlock()
	if files == nil {
		bin := "watchoo"
		if goos == "windows" {
			bin = "watchoo.exe"
		}
		files = []File{{"LICENSE", "licence"}, {bin, DefaultBinary}}
	}
	if goos == "windows" {
		return ZipArchive(files)
	}
	return TarGzArchive(files)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	tag, file := r.PathValue("tag"), r.PathValue("file")
	archive, err := s.Archive()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch file {
	case s.ArchiveName(tag):
		_, _ = w.Write(archive)
	case fmt.Sprintf("watchoo_%s_checksums.txt", strings.TrimPrefix(tag, "v")):
		s.mu.Lock()
		sums := s.Checksums
		s.mu.Unlock()
		if sums == "" {
			sum := sha256.Sum256(archive)
			sums = strings.Repeat("0", 64) + "  autre.zip\n" + hex.EncodeToString(sum[:]) + "  " + s.ArchiveName(tag) + "\n"
		}
		_, _ = fmt.Fprint(w, sums)
	default:
		http.NotFound(w, r)
	}
}

// TarGzArchive construit une archive tar.gz qui contient files (droits 0755).
func TarGzArchive(files []File) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f.Name, Mode: 0o755, Size: int64(len(f.Body)), Typeflag: tar.TypeReg}); err != nil {
			return nil, err
		}
		if _, err := tw.Write([]byte(f.Body)); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ZipArchive construit une archive zip qui contient files.
func ZipArchive(files []File) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.Name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.Body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
