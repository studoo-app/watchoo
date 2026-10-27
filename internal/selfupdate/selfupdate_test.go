package selfupdate_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/selfupdate"
	"github.com/studoo-app/watchoo/internal/selfupdate/selfupdatetest"
)

func newClient(srv *selfupdatetest.Server) *selfupdate.Client {
	return selfupdate.New(selfupdate.Options{BaseURL: srv.URL, UserAgent: "watchoo/v0.5.0", GOOS: srv.GOOS, GOARCH: srv.GOARCH})
}

func TestParseAndCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v0.5.0", "v0.6.0", -1},
		{"0.6.0", "v0.6.0", 0},
		{"v1.0.0", "v0.99.99", 1},
		{"v0.10.0", "v0.9.0", 1},
		{"v1.0.0-rc.1", "v1.0.0", -1},
		{"v1.0.0-alpha", "v1.0.0-alpha.1", -1},
		{"v1.0.0-alpha.1", "v1.0.0-alpha.beta", -1},
		{"v1.0.0-beta.2", "v1.0.0-beta.11", -1},
		{"v1.0.0-rc.1", "v1.0.0-beta", 1},
		{"v1.0.0+build.1", "v1.0.0", 0},
	}
	for _, tt := range tests {
		a, okA := selfupdate.ParseVersion(tt.a)
		b, okB := selfupdate.ParseVersion(tt.b)
		if !okA || !okB {
			t.Fatalf("ParseVersion(%q / %q) refusée", tt.a, tt.b)
		}
		if got := a.Compare(b); got != tt.want {
			t.Errorf("Compare(%s, %s) = %d, attendu %d", tt.a, tt.b, got, tt.want)
		}
		if got := b.Compare(a); got != -tt.want {
			t.Errorf("Compare(%s, %s) = %d, attendu %d", tt.b, tt.a, got, -tt.want)
		}
	}
	for _, bad := range []string{"", "dev", "v1.2", "v01.2.3", "v1.2.3-", "v1.2.3-01", "v1.2.3+", "1.2.3.4", "v1.x.3", "(devel)"} {
		if _, ok := selfupdate.ParseVersion(bad); ok {
			t.Errorf("ParseVersion(%q) acceptée", bad)
		}
	}
	if v, _ := selfupdate.ParseVersion("1.2.3-rc.1+b"); v.String() != "v1.2.3-rc.1+b" {
		t.Errorf("String = %s", v)
	}
}

func TestIsRelease(t *testing.T) {
	for s, want := range map[string]bool{
		"v0.5.0":                             true,
		"0.5.0":                              true,
		"dev":                                false,
		"v0.0.0-20261027100000-9412b2f170d8": false, // pseudo-version de Go
		"v0.5.1-SNAPSHOT-abc1234":            false, // instantané GoReleaser
		"v0.6.0-rc.1":                        false,
		"v0.5.0+dirty":                       false,
	} {
		if got := selfupdate.IsRelease(s); got != want {
			t.Errorf("IsRelease(%q) = %v, attendu %v", s, got, want)
		}
	}
}

func TestCheckLatestFromRedirect(t *testing.T) {
	srv := selfupdatetest.New(t, "v0.6.0", "linux", "amd64")
	c := newClient(srv)
	tests := []struct {
		current string
		want    bool
	}{
		{"v0.5.0", true},
		{"v0.6.0", false},
		{"v0.7.0", false},      // plus récente que la dernière publiée : aucun avis
		{"dev", false},         // comparaison impossible
		{"v0.6.0-rc.1", false}, // pré-version installée : pas une version de release
	}
	for _, tt := range tests {
		st, err := c.Check(context.Background(), tt.current)
		if err != nil {
			t.Fatalf("Check(%s) : %v", tt.current, err)
		}
		if st.Latest != "v0.6.0" || st.Current != tt.current || st.UpdateAvailable != tt.want {
			t.Errorf("Check(%s) = %+v, attendu UpdateAvailable %v", tt.current, st, tt.want)
		}
		if st.ReleaseURL != srv.URL+"/tag/v0.6.0" {
			t.Errorf("ReleaseURL = %q", st.ReleaseURL)
		}
	}
	if got := c.ChangelogURL("v0.6.0"); got != srv.URL+"/blob/v0.6.0/CHANGELOG.md" {
		t.Errorf("ChangelogURL = %q", got)
	}
	for _, ua := range srv.UserAgents() {
		if ua != "watchoo/v0.5.0" {
			t.Errorf("User-Agent = %q", ua)
		}
	}
	// La page de la release n'est jamais demandée : une requête par vérification.
	if n := srv.Requests(); n != int64(len(tests)) {
		t.Errorf("%d requêtes, attendu %d", n, len(tests))
	}
	prod := selfupdate.New(selfupdate.Options{})
	if got := prod.ChangelogURL("v0.6.0"); got != "https://github.com/studoo-app/watchoo/blob/v0.6.0/CHANGELOG.md" {
		t.Errorf("ChangelogURL de production = %q", got)
	}
}

func TestLatestErrors(t *testing.T) {
	srv := selfupdatetest.New(t, "", "linux", "amd64")
	c := newClient(srv)
	if _, _, err := c.Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "aucune release") {
		t.Errorf("sans release : %v", err)
	}
	for _, tag := range []string{"v0.7.0-rc.1", "nightly", "0.7.0"} {
		srv.Set(tag)
		if _, _, err := c.Latest(context.Background()); err == nil {
			t.Errorf("tag %q accepté", tag)
		}
	}
}

func TestHTTPRefusedOutsideLoopback(t *testing.T) {
	c := selfupdate.New(selfupdate.Options{BaseURL: "http://example.com/releases"})
	if _, err := c.Check(context.Background(), "v0.5.0"); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("HTTP hors boucle locale : %v", err)
	}
	if _, err := c.Download(context.Background(), "v0.6.0"); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("téléchargement HTTP hors boucle locale : %v", err)
	}
	c = selfupdate.New(selfupdate.Options{BaseURL: "ftp://127.0.0.1/releases"})
	if _, err := c.Check(context.Background(), "v0.5.0"); err == nil {
		t.Error("schéma ftp accepté")
	}
}

func TestRedirectToHTTPRefused(t *testing.T) {
	// Racine en HTTPS (serveur TLS de test) qui redirige vers une adresse HTTP : refusée.
	plain := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(plain.Close)
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/releases/latest", http.StatusFound)
	}))
	t.Cleanup(tlsSrv.Close)
	c := selfupdate.New(selfupdate.Options{BaseURL: tlsSrv.URL + "/releases", Transport: tlsSrv.Client().Transport})
	if _, _, err := c.Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("redirection vers HTTP : %v", err)
	}
}

// fakeExe crée un faux exécutable dans un répertoire temporaire et renvoie son chemin.
func fakeExe(t *testing.T, name string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(exe, []byte("ancien"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestInstallReplacesExecutable(t *testing.T) {
	srv := selfupdatetest.New(t, "v0.6.0", "linux", "amd64")
	exe := fakeExe(t, "watchoo")
	if err := newClient(srv).Install(context.Background(), "v0.6.0", exe); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exe)
	if err != nil || string(got) != selfupdatetest.DefaultBinary {
		t.Fatalf("binaire = %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(exe)
		if info.Mode().Perm() != 0o755 {
			t.Errorf("droits = %v", info.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Errorf("fichiers restants : %v (fichier temporaire oublié ?)", entries)
	}
}

func TestInstallWindowsZipKeepsOld(t *testing.T) {
	srv := selfupdatetest.New(t, "v0.6.0", "windows", "amd64")
	exe := fakeExe(t, "watchoo.exe")
	if err := newClient(srv).Install(context.Background(), "v0.6.0", exe); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != selfupdatetest.DefaultBinary {
		t.Errorf("binaire = %q", got)
	}
	if got, _ := os.ReadFile(exe + ".old"); string(got) != "ancien" {
		t.Errorf("ancien binaire = %q", got)
	}
	// Une deuxième mise à jour remplace le .old précédent.
	if err := newClient(srv).Install(context.Background(), "v0.6.0", exe); err != nil {
		t.Fatal(err)
	}
}

func TestInstallRefusesBadChecksum(t *testing.T) {
	archive := "watchoo_0.6.0_linux_amd64.tar.gz"
	for name, sums := range map[string]string{
		"somme fausse":    strings.Repeat("a", 64) + "  " + archive + "\n",
		"archive absente": strings.Repeat("a", 64) + "  autre.tar.gz\n",
		"somme illisible": "zz  " + archive + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			srv := selfupdatetest.New(t, "v0.6.0", "linux", "amd64")
			srv.Checksums = sums
			exe := fakeExe(t, "watchoo")
			err := newClient(srv).Install(context.Background(), "v0.6.0", exe)
			if err == nil {
				t.Fatal("archive acceptée")
			}
			if name == "somme fausse" && !errors.Is(err, selfupdate.ErrChecksum) {
				t.Errorf("erreur = %v, attendu ErrChecksum", err)
			}
			if got, _ := os.ReadFile(exe); string(got) != "ancien" {
				t.Errorf("binaire modifié : %q", got)
			}
		})
	}
}

func TestInstallRefusesArchiveWithoutBinary(t *testing.T) {
	for name, files := range map[string][]selfupdatetest.File{
		"sans binaire":            {{Name: "LICENSE", Body: "licence"}},
		"binaire dans un dossier": {{Name: "bin/watchoo", Body: "x"}},
		"binaire vide":            {{Name: "watchoo", Body: ""}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := selfupdatetest.New(t, "v0.6.0", "linux", "amd64")
			srv.Files = files
			exe := fakeExe(t, "watchoo")
			if err := newClient(srv).Install(context.Background(), "v0.6.0", exe); err == nil {
				t.Fatal("archive acceptée")
			}
			if got, _ := os.ReadFile(exe); string(got) != "ancien" {
				t.Errorf("binaire modifié : %q", got)
			}
		})
	}
}

func TestDownloadMissingVersion(t *testing.T) {
	srv := selfupdatetest.New(t, "v0.6.0", "linux", "amd64")
	c := selfupdate.New(selfupdate.Options{BaseURL: srv.URL, GOOS: "linux", GOARCH: "riscv64"})
	if _, err := c.Download(context.Background(), "v0.6.0"); err == nil {
		t.Error("plateforme sans archive acceptée")
	}
	if _, err := newClient(srv).Download(context.Background(), "dev"); err == nil {
		t.Error("version invalide acceptée")
	}
}

func TestCheckWritable(t *testing.T) {
	dir := t.TempDir()
	if err := selfupdate.CheckWritable(dir); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("fichier de test oublié : %v", entries)
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("droits POSIX non applicables (Windows ou root)")
	}
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if err := selfupdate.CheckWritable(ro); !errors.Is(err, selfupdate.ErrNotWritable) {
		t.Errorf("répertoire en lecture seule : %v", err)
	}
	if err := selfupdate.Replace(filepath.Join(ro, "watchoo"), []byte("x"), "linux"); !errors.Is(err, selfupdate.ErrNotWritable) {
		t.Errorf("Replace en lecture seule : %v", err)
	}
}

func TestCheckInstallable(t *testing.T) {
	if err := selfupdate.CheckInstallable("v0.5.0", false); err != nil {
		t.Errorf("version de release refusée : %v", err)
	}
	if err := selfupdate.CheckInstallable("v0.5.0", true); !errors.Is(err, selfupdate.ErrDocker) {
		t.Errorf("Docker : %v", err)
	}
	for _, v := range []string{"dev", "v0.0.0-20261027100000-9412b2f170d8", "v0.6.0-rc.1"} {
		if err := selfupdate.CheckInstallable(v, false); !errors.Is(err, selfupdate.ErrDevBuild) {
			t.Errorf("%s : %v", v, err)
		}
	}
}

func TestEnvironment(t *testing.T) {
	for v, want := range map[string]bool{"": false, "0": false, "false": false, "FALSE": false, "1": true, "oui": true, "true": true} {
		getenv := func(name string) string {
			if name == selfupdate.EnvNoCheck {
				return v
			}
			return ""
		}
		if got := selfupdate.CheckDisabled(getenv); got != want {
			t.Errorf("CheckDisabled(%q) = %v, attendu %v", v, got, want)
		}
	}
	t.Setenv(selfupdate.EnvDocker, "1")
	if !selfupdate.InDocker() {
		t.Error("WATCHOO_DOCKER=1 : InDocker = false")
	}
}

// fakeChecker renvoie des statuts programmés.
type fakeChecker struct {
	mu    sync.Mutex
	calls int
	st    selfupdate.Status
	err   error
}

func (f *fakeChecker) Check(_ context.Context, current string) (selfupdate.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	st := f.st
	st.Current = current
	return st, f.err
}

func TestMonitor(t *testing.T) {
	checker := &fakeChecker{st: selfupdate.Status{Latest: "v0.6.0", UpdateAvailable: true}}
	var announced []string
	m := selfupdate.NewMonitor(selfupdate.MonitorOptions{Checker: checker, Current: "v0.5.0",
		OnUpdate: func(st selfupdate.Status) { announced = append(announced, st.Latest) }})
	if _, ok := m.Available(); ok {
		t.Fatal("version disponible avant toute vérification")
	}
	m.CheckNow(context.Background())
	m.CheckNow(context.Background()) // même version : pas de deuxième annonce
	st, ok := m.Available()
	if !ok || st.Latest != "v0.6.0" || st.Current != "v0.5.0" {
		t.Errorf("Available = %+v, %v", st, ok)
	}
	// Un échec garde la version connue.
	checker.err = errors.New("hors ligne")
	m.CheckNow(context.Background())
	if _, ok := m.Available(); !ok {
		t.Error("version oubliée après un échec")
	}
	checker.err = nil
	checker.st.Latest = "v0.7.0"
	m.CheckNow(context.Background())
	if strings.Join(announced, ",") != "v0.6.0,v0.7.0" {
		t.Errorf("annonces = %v", announced)
	}
}

func TestMonitorRunChecksPeriodically(t *testing.T) {
	checker := &fakeChecker{st: selfupdate.Status{Latest: "v0.5.0"}}
	m := selfupdate.NewMonitor(selfupdate.MonitorOptions{Checker: checker, Current: "v0.5.0", Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		checker.mu.Lock()
		n := checker.calls
		checker.mu.Unlock()
		if n >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d vérifications seulement", n)
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if _, ok := m.Available(); ok {
		t.Error("version à jour signalée comme nouvelle")
	}
}
