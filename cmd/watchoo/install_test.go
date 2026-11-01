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

// Tests of the install.sh installation script, run with sh against a fake releases server
// (WATCHOO_RELEASES_URL): no network access.

const fakeBinary = "#!/bin/sh\necho faux-watchoo\n"

// fakeArchive builds a release archive holding a fake watchoo binary.
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

// releaseServer simulates the GitHub releases of the version version; sums replaces the checksum
// file if it is not empty.
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

// runInstall runs install.sh with the variables env and returns its output and its error.
func runInstall(t *testing.T, env ...string) (string, error) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("install.sh only targets Linux and macOS")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("architecture without release archive")
	}
	for _, tool := range []string{"sh", "curl", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s missing", tool)
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
		t.Fatalf("install.sh: %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "watchoo"))
	if err != nil || string(got) != fakeBinary {
		t.Fatalf("installed binary = %q, %v", got, err)
	}
	info, _ := os.Stat(filepath.Join(dir, "watchoo"))
	if info.Mode().Perm() != 0o755 {
		t.Errorf("permissions = %v", info.Mode().Perm())
	}
	for _, want := range []string{"Watchoo v1.2.3 installed", "SHA-256 checksum verified", "is not in your PATH"} {
		if !strings.Contains(out, want) {
			t.Errorf("output without %q:\n%s", want, out)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("files in %s: %v (temporary file left behind?)", dir, entries)
	}
}

func TestInstallDefaultDirAndPinnedVersion(t *testing.T) {
	srv := releaseServer(t, "0.4.0", "")
	home := t.TempDir()
	// Version without "v": the script adds it.
	cmdOut, err := runInstall(t, "WATCHOO_RELEASES_URL="+srv.URL, "WATCHOO_VERSION=0.4.0", "HOME="+home)
	if err != nil {
		t.Fatalf("install.sh: %v\n%s", err, cmdOut)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "watchoo")); err != nil {
		t.Errorf("binary missing from ~/.local/bin: %v\n%s", err, cmdOut)
	}
}

func TestInstallRefusesBadChecksum(t *testing.T) {
	archive := fmt.Sprintf("watchoo_1.2.3_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	for name, sums := range map[string]string{
		"wrong checksum":  strings.Repeat("a", 64) + "  " + archive + "\n",
		"missing archive": strings.Repeat("a", 64) + "  autre.tar.gz\n",
	} {
		t.Run(name, func(t *testing.T) {
			srv := releaseServer(t, "1.2.3", sums)
			dir := filepath.Join(t.TempDir(), "bin")
			out, err := runInstall(t, "WATCHOO_RELEASES_URL="+srv.URL, "WATCHOO_INSTALL_DIR="+dir)
			if err == nil {
				t.Fatalf("install.sh succeeded despite %s:\n%s", name, out)
			}
			if _, err := os.Stat(filepath.Join(dir, "watchoo")); err == nil {
				t.Error("binary installed despite an invalid checksum")
			}
		})
	}
}

// TestInstallFrench checks the French messages, selected by WATCHOO_LANG or a French locale.
func TestInstallFrench(t *testing.T) {
	srv := releaseServer(t, "1.2.3", "")
	for name, langEnv := range map[string]string{"WATCHOO_LANG": "WATCHOO_LANG=fr", "locale": "LANG=fr_FR.UTF-8"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "bin")
			out, err := runInstall(t, "WATCHOO_RELEASES_URL="+srv.URL, "WATCHOO_INSTALL_DIR="+dir, langEnv)
			if err != nil {
				t.Fatalf("install.sh: %v\n%s", err, out)
			}
			for _, want := range []string{"Watchoo v1.2.3 installé", "Somme SHA-256 vérifiée", "n'est pas dans votre PATH"} {
				if !strings.Contains(out, want) {
					t.Errorf("output without %q:\n%s", want, out)
				}
			}
			out, err = runInstall(t, "WATCHOO_RELEASES_URL="+srv.URL, "WATCHOO_VERSION=abc", langEnv)
			if err == nil || !strings.Contains(out, "erreur : version invalide : abc") {
				t.Errorf("err = %v, output without the French error:\n%s", err, out)
			}
		})
	}
	// WATCHOO_LANG takes precedence over the locale.
	dir := filepath.Join(t.TempDir(), "bin")
	out, err := runInstall(t, "WATCHOO_RELEASES_URL="+srv.URL, "WATCHOO_INSTALL_DIR="+dir, "LANG=fr_FR.UTF-8", "WATCHOO_LANG=en")
	if err != nil || !strings.Contains(out, "SHA-256 checksum verified") {
		t.Errorf("WATCHOO_LANG=en with a French locale: err = %v, output:\n%s", err, out)
	}
}

func TestInstallErrors(t *testing.T) {
	srv := releaseServer(t, "1.2.3", "")
	for name, tc := range map[string]struct {
		env  []string
		want string
	}{
		"nonexistent version": {[]string{"WATCHOO_VERSION=v9.9.9"}, "does version v9.9.9 exist?"},
		"invalid version":     {[]string{"WATCHOO_VERSION=abc"}, "invalid version: abc"},
		"without HOME":        {[]string{"HOME="}, "WATCHOO_INSTALL_DIR"},
	} {
		t.Run(name, func(t *testing.T) {
			env := append([]string{"WATCHOO_RELEASES_URL=" + srv.URL}, tc.env...)
			out, err := runInstall(t, env...)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Errorf("err = %v, output without %q:\n%s", err, tc.want, out)
			}
		})
	}
}
