package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Tests du script d'installation install.sh, lancé avec sh contre un faux serveur de releases
// (WATCHOO_RELEASES_URL) : aucun accès au réseau.

const fakeBinary = "#!/bin/sh\necho faux-watchoo\n"

// fakeArchive construit une archive de release qui contient un faux binaire watchoo.
func fakeArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct{ name, body string }{{"LICENSE", "licence"}, {"watchoo", fakeBinary}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// releaseServer simule les releases GitHub de la version version ; sums remplace le fichier de
// sommes s'il n'est pas vide.
func releaseServer(t *testing.T, version, sums string) *httptest.Server {
	t.Helper()
	archive := fmt.Sprintf("watchoo_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	data := fakeArchive(t)
	sum := sha256.Sum256(data)
	if sums == "" {
		sums = hex.EncodeToString(sum[:]) + "  " + archive + "\n" + strings.Repeat("0", 64) + "  autre.zip\n"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tag/v"+version, http.StatusFound)
	})
	mux.HandleFunc("GET /tag/", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("GET /download/v"+version+"/"+archive, func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	})
	mux.HandleFunc("GET /download/v"+version+"/watchoo_"+version+"_checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, sums)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// runInstall lance install.sh avec les variables env et renvoie sa sortie et son erreur.
func runInstall(t *testing.T, env ...string) (string, error) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("install.sh ne vise que Linux et macOS")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("architecture sans archive de release")
	}
	for _, tool := range []string{"sh", "curl", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s absent", tool)
		}
	}
	cmd := exec.Command("sh", filepath.Join("..", "..", "install.sh"))
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestInstallLatest(t *testing.T) {
	srv := releaseServer(t, "1.2.3", "")
	dir := filepath.Join(t.TempDir(), "bin")
	out, err := runInstall(t, "WATCHOO_RELEASES_URL="+srv.URL, "WATCHOO_INSTALL_DIR="+dir)
	if err != nil {
		t.Fatalf("install.sh : %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "watchoo"))
	if err != nil || string(got) != fakeBinary {
		t.Fatalf("binaire installé = %q, %v", got, err)
	}
	info, _ := os.Stat(filepath.Join(dir, "watchoo"))
	if info.Mode().Perm() != 0o755 {
		t.Errorf("droits = %v", info.Mode().Perm())
	}
	for _, want := range []string{"Watchoo v1.2.3 installé", "Somme SHA-256 vérifiée", "n'est pas dans votre PATH"} {
		if !strings.Contains(out, want) {
			t.Errorf("sortie sans %q :\n%s", want, out)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("fichiers dans %s : %v (fichier temporaire oublié ?)", dir, entries)
	}
}

func TestInstallDefaultDirAndPinnedVersion(t *testing.T) {
	srv := releaseServer(t, "0.4.0", "")
	home := t.TempDir()
	// Version sans « v » : le script l'ajoute.
	cmdOut, err := runInstall(t, "WATCHOO_RELEASES_URL="+srv.URL, "WATCHOO_VERSION=0.4.0", "HOME="+home)
	if err != nil {
		t.Fatalf("install.sh : %v\n%s", err, cmdOut)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "watchoo")); err != nil {
		t.Errorf("binaire absent de ~/.local/bin : %v\n%s", err, cmdOut)
	}
}

func TestInstallRefusesBadChecksum(t *testing.T) {
	archive := fmt.Sprintf("watchoo_1.2.3_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	for name, sums := range map[string]string{
		"somme fausse":    strings.Repeat("a", 64) + "  " + archive + "\n",
		"archive absente": strings.Repeat("a", 64) + "  autre.tar.gz\n",
	} {
		t.Run(name, func(t *testing.T) {
			srv := releaseServer(t, "1.2.3", sums)
			dir := filepath.Join(t.TempDir(), "bin")
			out, err := runInstall(t, "WATCHOO_RELEASES_URL="+srv.URL, "WATCHOO_INSTALL_DIR="+dir)
			if err == nil {
				t.Fatalf("install.sh a réussi malgré %s :\n%s", name, out)
			}
			if _, err := os.Stat(filepath.Join(dir, "watchoo")); err == nil {
				t.Error("binaire installé malgré une somme invalide")
			}
		})
	}
}

func TestInstallErrors(t *testing.T) {
	srv := releaseServer(t, "1.2.3", "")
	for name, tc := range map[string]struct {
		env  []string
		want string
	}{
		"version inexistante": {[]string{"WATCHOO_VERSION=v9.9.9"}, "version v9.9.9 inexistante"},
		"version invalide":    {[]string{"WATCHOO_VERSION=abc"}, "version invalide : abc"},
		"sans HOME":           {[]string{"HOME="}, "WATCHOO_INSTALL_DIR"},
	} {
		t.Run(name, func(t *testing.T) {
			env := append([]string{"WATCHOO_RELEASES_URL=" + srv.URL}, tc.env...)
			out, err := runInstall(t, env...)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Errorf("err = %v, sortie sans %q :\n%s", err, tc.want, out)
			}
		})
	}
}
