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

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/selfupdate"
	"github.com/studoo-app/watchoo/internal/selfupdate/selfupdatetest"
)

// TestMain sets the process language for the whole package: the expected texts of the tests are
// written in English, whatever WATCHOO_LANG and the machine locale. No restore: English stays the
// language of the tests.
func TestMain(m *testing.M) {
	i18n.SetProcessForTest(i18n.English)
	os.Exit(m.Run())
}

func newClient(srv *selfupdatetest.Server) *selfupdate.Client {
	return selfupdate.New(selfupdate.Options{BaseURL: srv.URL, UserAgent: "watchoo/v0.5.0", GOOS: srv.GOOS, GOARCH: srv.GOARCH})
}

func TestIsRelease(t *testing.T) {
	for s, want := range map[string]bool{
		"v0.5.0":                             true,
		"0.5.0":                              true,
		"dev":                                false,
		"v0.0.0-20261027100000-9412b2f170d8": false, // Go pseudo-version
		"v0.5.1-SNAPSHOT-abc1234":            false, // GoReleaser snapshot
		"v0.6.0-rc.1":                        false,
		"v0.5.0+dirty":                       false,
	} {
		if got := selfupdate.IsRelease(s); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", s, got, want)
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
		{"v0.7.0", false},      // newer than the latest published one: no notice
		{"dev", false},         // comparison impossible
		{"v0.6.0-rc.1", false}, // installed pre-release: not a release version
	}
	for _, tt := range tests {
		st, err := c.Check(context.Background(), tt.current)
		if err != nil {
			t.Fatalf("Check(%s): %v", tt.current, err)
		}
		if st.Latest != "v0.6.0" || st.Current != tt.current || st.UpdateAvailable != tt.want {
			t.Errorf("Check(%s) = %+v, want UpdateAvailable %v", tt.current, st, tt.want)
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
	// The release page is never requested: one request per check.
	if n := srv.Requests(); n != int64(len(tests)) {
		t.Errorf("%d requests, want %d", n, len(tests))
	}
	prod := selfupdate.New(selfupdate.Options{})
	if got := prod.ChangelogURL("v0.6.0"); got != "https://github.com/studoo-app/watchoo/blob/v0.6.0/CHANGELOG.md" {
		t.Errorf("production ChangelogURL = %q", got)
	}
}

func TestLatestErrors(t *testing.T) {
	srv := selfupdatetest.New(t, "", "linux", "amd64")
	c := newClient(srv)
	if _, _, err := c.Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "no published release") {
		t.Errorf("without release: %v", err)
	}
	for _, tag := range []string{"v0.7.0-rc.1", "nightly", "0.7.0"} {
		srv.Set(tag)
		if _, _, err := c.Latest(context.Background()); err == nil {
			t.Errorf("tag %q accepted", tag)
		}
	}
}

func TestHTTPRefusedOutsideLoopback(t *testing.T) {
	c := selfupdate.New(selfupdate.Options{BaseURL: "http://example.com/releases"})
	if _, err := c.Check(context.Background(), "v0.5.0"); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("HTTP outside loopback: %v", err)
	}
	if _, err := c.Download(context.Background(), "v0.6.0"); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("HTTP download outside loopback: %v", err)
	}
	c = selfupdate.New(selfupdate.Options{BaseURL: "ftp://127.0.0.1/releases"})
	if _, err := c.Check(context.Background(), "v0.5.0"); err == nil {
		t.Error("ftp scheme accepted")
	}
}

func TestRedirectToHTTPRefused(t *testing.T) {
	// HTTPS root (test TLS server) that redirects to an HTTP address: refused.
	plain := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(plain.Close)
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/releases/latest", http.StatusFound)
	}))
	t.Cleanup(tlsSrv.Close)
	c := selfupdate.New(selfupdate.Options{BaseURL: tlsSrv.URL + "/releases", Transport: tlsSrv.Client().Transport})
	if _, _, err := c.Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("redirect to HTTP: %v", err)
	}
}

// fakeExe creates a fake executable in a temporary directory and returns its path.
func fakeExe(t *testing.T, name string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
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
		t.Fatalf("binary = %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(exe)
		if info.Mode().Perm() != 0o755 {
			t.Errorf("permissions = %v", info.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Errorf("remaining files: %v (temporary file left behind?)", entries)
	}
}

func TestInstallWindowsZipKeepsOld(t *testing.T) {
	srv := selfupdatetest.New(t, "v0.6.0", "windows", "amd64")
	exe := fakeExe(t, "watchoo.exe")
	if err := newClient(srv).Install(context.Background(), "v0.6.0", exe); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != selfupdatetest.DefaultBinary {
		t.Errorf("binary = %q", got)
	}
	if got, _ := os.ReadFile(exe + ".old"); string(got) != "old" {
		t.Errorf("old binary = %q", got)
	}
	// A second update replaces the previous .old.
	if err := newClient(srv).Install(context.Background(), "v0.6.0", exe); err != nil {
		t.Fatal(err)
	}
}

func TestInstallRefusesBadChecksum(t *testing.T) {
	archive := "watchoo_0.6.0_linux_amd64.tar.gz"
	for name, sums := range map[string]string{
		"wrong checksum":      strings.Repeat("a", 64) + "  " + archive + "\n",
		"missing archive":     strings.Repeat("a", 64) + "  other.tar.gz\n",
		"unreadable checksum": "zz  " + archive + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			srv := selfupdatetest.New(t, "v0.6.0", "linux", "amd64")
			srv.Checksums = sums
			exe := fakeExe(t, "watchoo")
			err := newClient(srv).Install(context.Background(), "v0.6.0", exe)
			if err == nil {
				t.Fatal("archive accepted")
			}
			if name == "wrong checksum" && !errors.Is(err, selfupdate.ErrChecksum) {
				t.Errorf("error = %v, want ErrChecksum", err)
			}
			if got, _ := os.ReadFile(exe); string(got) != "old" {
				t.Errorf("binary changed: %q", got)
			}
		})
	}
}

func TestInstallRefusesArchiveWithoutBinary(t *testing.T) {
	for name, files := range map[string][]selfupdatetest.File{
		"without binary":     {{Name: "LICENSE", Body: "license"}},
		"binary in a folder": {{Name: "bin/watchoo", Body: "x"}},
		"empty binary":       {{Name: "watchoo", Body: ""}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := selfupdatetest.New(t, "v0.6.0", "linux", "amd64")
			srv.Files = files
			exe := fakeExe(t, "watchoo")
			if err := newClient(srv).Install(context.Background(), "v0.6.0", exe); err == nil {
				t.Fatal("archive accepted")
			}
			if got, _ := os.ReadFile(exe); string(got) != "old" {
				t.Errorf("binary changed: %q", got)
			}
		})
	}
}

func TestDownloadMissingVersion(t *testing.T) {
	srv := selfupdatetest.New(t, "v0.6.0", "linux", "amd64")
	c := selfupdate.New(selfupdate.Options{BaseURL: srv.URL, GOOS: "linux", GOARCH: "riscv64"})
	if _, err := c.Download(context.Background(), "v0.6.0"); err == nil {
		t.Error("platform without an archive accepted")
	}
	if _, err := newClient(srv).Download(context.Background(), "dev"); err == nil {
		t.Error("invalid version accepted")
	}
}

func TestCheckWritable(t *testing.T) {
	dir := t.TempDir()
	if err := selfupdate.CheckWritable(dir); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("test file left behind: %v", entries)
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("POSIX permissions not applicable (Windows or root)")
	}
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if err := selfupdate.CheckWritable(ro); !errors.Is(err, selfupdate.ErrNotWritable) {
		t.Errorf("read-only directory: %v", err)
	}
	if err := selfupdate.Replace(filepath.Join(ro, "watchoo"), []byte("x"), "linux"); !errors.Is(err, selfupdate.ErrNotWritable) {
		t.Errorf("read-only Replace: %v", err)
	}
}

func TestCheckInstallable(t *testing.T) {
	if err := selfupdate.CheckInstallable("v0.5.0", false); err != nil {
		t.Errorf("release version refused: %v", err)
	}
	if err := selfupdate.CheckInstallable("v0.5.0", true); !errors.Is(err, selfupdate.ErrDocker) {
		t.Errorf("Docker: %v", err)
	}
	for _, v := range []string{"dev", "v0.0.0-20261027100000-9412b2f170d8", "v0.6.0-rc.1"} {
		if err := selfupdate.CheckInstallable(v, false); !errors.Is(err, selfupdate.ErrDevBuild) {
			t.Errorf("%s: %v", v, err)
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
			t.Errorf("CheckDisabled(%q) = %v, want %v", v, got, want)
		}
	}
	t.Setenv(selfupdate.EnvDocker, "1")
	if !selfupdate.InDocker() {
		t.Error("WATCHOO_DOCKER=1: InDocker = false")
	}
}

// fakeChecker returns scheduled statuses.
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
		t.Fatal("version available before any check")
	}
	m.CheckNow(context.Background())
	m.CheckNow(context.Background()) // same version: no second announcement
	st, ok := m.Available()
	if !ok || st.Latest != "v0.6.0" || st.Current != "v0.5.0" {
		t.Errorf("Available = %+v, %v", st, ok)
	}
	// A failure keeps the known version.
	checker.err = errors.New("offline")
	m.CheckNow(context.Background())
	if _, ok := m.Available(); !ok {
		t.Error("version forgotten after a failure")
	}
	checker.err = nil
	checker.st.Latest = "v0.7.0"
	m.CheckNow(context.Background())
	if strings.Join(announced, ",") != "v0.6.0,v0.7.0" {
		t.Errorf("announcements = %v", announced)
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
			t.Fatalf("only %d checks", n)
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if _, ok := m.Available(); ok {
		t.Error("up-to-date version reported as new")
	}
}

// The messages follow the process language: French typography (space before ":"). Not parallel:
// the process language is shared.
func TestMessagesFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	err := selfupdate.CheckInstallable("dev", false)
	if !errors.Is(err, selfupdate.ErrDevBuild) || err.Error() != "version de développement : dev" {
		t.Errorf("CheckInstallable = %v", err)
	}
	srv := selfupdatetest.New(t, "", "linux", "amd64")
	if _, _, err := newClient(srv).Latest(context.Background()); err == nil ||
		err.Error() != "aucune release publiée trouvée (pas de redirection vers une version)" {
		t.Errorf("Latest = %v", err)
	}
}
