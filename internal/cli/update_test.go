package cli

// Update tests (0.6.0): new-version notice and "watchoo update". The releases are those of the fake
// selfupdatetest server: no network access.

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/selfupdate"
	"github.com/studoo-app/watchoo/internal/selfupdate/selfupdatetest"
	"github.com/studoo-app/watchoo/internal/tui"
)

const wantNotice = `New Watchoo version available: v0.5.0 → v0.6.0. Update with "watchoo update".`

// newUpdateHarness returns a harness at version current (as injected by -ldflags) whose releases are
// those of a fake server, with latest as the latest version, for the current platform.
func newUpdateHarness(t *testing.T, current, latest string) (*harness, *selfupdatetest.Server) {
	t.Helper()
	h := newHarness(t)
	setBuildVars(t, current, "", "")
	srv := selfupdatetest.New(t, latest, runtime.GOOS, runtime.GOARCH)
	h.Updater = selfupdate.New(selfupdate.Options{BaseURL: srv.URL, UserAgent: "watchoo/" + current})
	return h, srv
}

// fakeExecutable creates a fake installed binary and declares it to the harness.
func fakeExecutable(t *testing.T, h *harness) string {
	t.Helper()
	name := "watchoo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(exe, []byte("ancien"), 0o755); err != nil {
		t.Fatal(err)
	}
	h.Executable = exe
	return exe
}

func TestUpdateNoticePrintedOnStderr(t *testing.T) {
	h, srv := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	h.StderrTerminal = true
	r := h.run("version").mustSucceed(t)
	if r.stderr != wantNotice+"\n" {
		t.Errorf("stderr = %q, want the notice", r.stderr)
	}
	assertNotContains(t, "stdout", r.stdout, "New Watchoo version")
	// Also after a failing command (outside a workspace), after the error message.
	r = h.run("status").mustCode(t, 1)
	if !strings.HasPrefix(r.stderr, i18n.T("cli.core.err.line", "")) || !strings.HasSuffix(r.stderr, wantNotice+"\n") {
		t.Errorf("stderr = %q", r.stderr)
	}
	// One check per command (no cache).
	if n := srv.Requests(); n != 2 {
		t.Errorf("%d requests, want 2", n)
	}
	// In the Docker image: change the tag.
	h.InDocker = true
	r = h.run("version").mustSucceed(t)
	assertContains(t, "Docker notice", r.stderr, "v0.5.0 → v0.6.0", "Change the Docker image tag")
}

func TestUpdateNoticeSkipped(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		latest   string
		terminal bool
		env      string
		args     []string
	}{
		{name: "json", current: "v0.5.0", latest: "v0.6.0", terminal: true, args: []string{"version", "--json"}},
		{name: "stderr off a terminal", current: "v0.5.0", latest: "v0.6.0", args: []string{"version"}},
		{name: "variable", current: "v0.5.0", latest: "v0.6.0", terminal: true, env: "1", args: []string{"version"}},
		{name: "dev version", current: "dev", latest: "v0.6.0", terminal: true, args: []string{"version"}},
		{name: "pseudo-version", current: "v0.0.0-20261027100000-9412b2f170d8", latest: "v0.6.0", terminal: true, args: []string{"version"}},
		{name: "up to date", current: "v0.6.0", latest: "v0.6.0", terminal: true, args: []string{"version"}},
		{name: "newer", current: "v0.7.0", latest: "v0.6.0", terminal: true, args: []string{"version"}},
		{name: "help", current: "v0.5.0", latest: "v0.6.0", terminal: true, args: []string{"help", "sync"}},
		{name: "--help", current: "v0.5.0", latest: "v0.6.0", terminal: true, args: []string{"--help"}},
		{name: "update --check", current: "v0.5.0", latest: "v0.6.0", terminal: true, args: []string{"update", "--check"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, srv := newUpdateHarness(t, tt.current, tt.latest)
			h.StderrTerminal = tt.terminal
			if tt.env != "" {
				t.Setenv(selfupdate.EnvNoCheck, tt.env)
			}
			r := h.run(tt.args...).mustSucceed(t)
			assertNotContains(t, "stderr", r.stderr, "New Watchoo version")
			wantRequests := int64(0)
			switch tt.name {
			case "up to date", "newer":
				wantRequests = 1 // checked, but nothing to announce
			case "update --check":
				wantRequests = 1 // the command itself, not the notice
			}
			if n := srv.Requests(); n != wantRequests {
				t.Errorf("%d requests, want %d", n, wantRequests)
			}
		})
	}
	// "0" and "false" do not turn the check off.
	for _, v := range []string{"0", "false"} {
		h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
		h.StderrTerminal = true
		t.Setenv(selfupdate.EnvNoCheck, v)
		assertContains(t, "WATCHOO_NO_UPDATE_CHECK="+v, h.run("version").mustSucceed(t).stderr, wantNotice)
	}
}

// "watchoo" alone (card and general help) has its notice, after the help; an unknown word does not.
func TestUpdateNoticeAfterRootHelp(t *testing.T) {
	h, srv := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	h.StderrTerminal = true
	r := h.run().mustSucceed(t)
	assertContains(t, "stdout", r.stdout, "Commands:", `Run "watchoo [command] --help"`)
	if r.stderr != wantNotice+"\n" {
		t.Errorf("stderr = %q, want the notice", r.stderr)
	}
	r = h.run("inconnue").mustCode(t, 1)
	assertNotContains(t, "stderr", r.stderr, "New Watchoo version")
	if n := srv.Requests(); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

// An answer that has already arrived wins over the passed deadline: after a command longer than the
// timeout, the notice is always written (before the fix, select chose at random).
func TestUpdateNoticeAfterLongCommand(t *testing.T) {
	for i := range 200 {
		var stderr strings.Builder
		n := &updateNotice{
			done:     make(chan struct{}),
			status:   selfupdate.Status{Current: "v0.5.0", Latest: "v0.6.0", UpdateAvailable: true},
			deadline: time.Now().Add(-time.Second), // the command lasted longer than the timeout
			cancel:   func() {},
			logger:   slog.New(slog.DiscardHandler),
		}
		close(n.done)
		n.finish(context.Background(), Deps{Stderr: &stderr})
		if stderr.String() != wantNotice+"\n" {
			t.Fatalf("attempt %d: stderr = %q, want the notice", i, stderr.String())
		}
	}
}

// End to end: "watchoo log", made longer than the timeout by a slow clock, keeps its notice every time
// (before the fix, about one time out of two).
func TestUpdateNoticeLongCommandEndToEnd(t *testing.T) {
	original := updateNoticeTimeout
	t.Cleanup(func() { updateNoticeTimeout = original })
	updateNoticeTimeout = 300 * time.Millisecond

	h := newWorkspace(t)
	setBuildVars(t, "v0.5.0", "", "")
	srv := selfupdatetest.New(t, "v0.6.0", runtime.GOOS, runtime.GOARCH)
	h.Updater = selfupdate.New(selfupdate.Options{BaseURL: srv.URL, UserAgent: "watchoo/v0.5.0"})
	h.StderrTerminal = true
	h.Now = func() time.Time {
		time.Sleep(400 * time.Millisecond) // the command lasts longer than the timeout
		return testNow
	}
	for i := range 6 {
		r := h.run("log").mustSucceed(t)
		if r.stderr != wantNotice+"\n" {
			t.Fatalf("attempt %d: stderr = %q, want the notice", i, r.stderr)
		}
	}
}

// A server that is too slow does not delay the command beyond the timeout, and the notice is dropped.
func TestUpdateNoticeDoesNotDelayCommand(t *testing.T) {
	original := updateNoticeTimeout
	t.Cleanup(func() { updateNoticeTimeout = original })
	updateNoticeTimeout = 100 * time.Millisecond

	h, srv := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	h.StderrTerminal = true
	srv.Delay = 5 * time.Second
	start := time.Now()
	r := h.run("version").mustSucceed(t)
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("command delayed by %v", d)
	}
	if r.stderr != "" {
		t.Errorf("stderr = %q, want empty", r.stderr)
	}
}

func TestUpdateCheckText(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	r := h.run("update", "--check").mustSucceed(t)
	assertContains(t, "update --check", r.stdout, "Installed version: v0.5.0", "Latest version: v0.6.0",
		"New version available: http://127.0.0.1", "/tag/v0.6.0", `"watchoo update"`)

	h, _ = newUpdateHarness(t, "v0.6.0", "v0.6.0")
	assertContains(t, "up to date", h.run("update", "--check").mustSucceed(t).stdout, "Watchoo is up to date.")

	h, _ = newUpdateHarness(t, "dev", "v0.6.0")
	assertContains(t, "dev", h.run("update", "--check").mustSucceed(t).stdout, "Latest version: v0.6.0",
		"cannot compare versions")

	// Outside a workspace and without gh: no workspace needed; failing network: non-zero code.
	h = newHarness(t)
	h.run("update", "--check").mustFail(t, "cannot read the latest Watchoo version")
}

func TestUpdateCheckJSON(t *testing.T) {
	h, srv := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	r := h.run("update", "--check", "--json").mustSucceed(t)
	assertSingleJSONObject(t, "update --check --json", r.stdout)
	assertJSONKeys(t, r.stdout, "", jsonTopLevelKeys["update"]...)
	var got updateReport
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatal(err)
	}
	want := updateReport{Current: "v0.5.0", Latest: "v0.6.0", UpdateAvailable: true, ReleaseURL: srv.URL + "/tag/v0.6.0"}
	if got != want {
		t.Errorf("update --check --json = %+v, want %+v", got, want)
	}
	// Without --check, --json is refused.
	h.run("update", "--json").mustFail(t, "--check")
}

func TestUpdateConfirmed(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	exe := fakeExecutable(t, h)
	h.StdinTerminal, h.Stdin = true, "o\n"
	r := h.run("update").mustSucceed(t)
	assertContains(t, "update", r.stdout, "Updating Watchoo: v0.5.0 → v0.6.0", "/tag/v0.6.0",
		"/blob/v0.6.0/CHANGELOG.md", "Watchoo v0.6.0 installed in "+exe, "watchoo serve", "migrations")
	assertContains(t, "question", r.stderr, "Install Watchoo v0.6.0? [y/N]")
	if got, _ := os.ReadFile(exe); string(got) != selfupdatetest.DefaultBinary {
		t.Errorf("binary = %q, want the new one", got)
	}
}

func TestUpdateRefusedByUser(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	exe := fakeExecutable(t, h)
	h.StdinTerminal, h.Stdin = true, "n\n"
	r := h.run("update").mustCode(t, 1)
	assertContains(t, "cancellation", r.stderr, "update cancelled")
	if got, _ := os.ReadFile(exe); string(got) != "ancien" {
		t.Errorf("binary changed: %q", got)
	}
}

func TestUpdateYesAndNonTerminal(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	exe := fakeExecutable(t, h)
	// Input off a terminal without --yes: refusal, nothing changed.
	h.run("update").mustCode(t, 1)
	if got, _ := os.ReadFile(exe); string(got) != "ancien" {
		t.Errorf("binary changed without --yes: %q", got)
	}
	r := h.run("update", "-y").mustSucceed(t)
	assertNotContains(t, "question", r.stderr, "[y/N]")
	if got, _ := os.ReadFile(exe); string(got) != selfupdatetest.DefaultBinary {
		t.Errorf("binary = %q, want the new one", got)
	}
}

func TestUpdateNonTerminalMessage(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	fakeExecutable(t, h)
	r := h.run("update").mustCode(t, 1)
	assertContains(t, "without a terminal", r.stderr, "--yes")
}

func TestUpdateAlreadyUpToDate(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.6.0", "v0.6.0")
	exe := fakeExecutable(t, h)
	r := h.run("update").mustSucceed(t)
	assertContains(t, "up to date", r.stdout, "Watchoo v0.6.0 is up to date")
	if got, _ := os.ReadFile(exe); string(got) != "ancien" {
		t.Errorf("binary changed: %q", got)
	}
}

func TestUpdateRefusals(t *testing.T) {
	h, srv := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	fakeExecutable(t, h)
	h.InDocker = true
	h.run("update", "--yes").mustFail(t, "Docker", "docker compose pull")
	h.InDocker = false

	setBuildVars(t, "dev", "", "")
	h.run("update", "--yes").mustFail(t, "built from the sources", "go install", "make build")
	if n := srv.Requests(); n != 0 {
		t.Errorf("%d requests before the refusals, want 0", n)
	}

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("POSIX permissions not applicable (Windows or root)")
	}
	setBuildVars(t, "v0.5.0", "", "")
	ro := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(ro, 0o755); err != nil {
		t.Fatal(err)
	}
	h.Executable = filepath.Join(ro, "watchoo")
	if err := os.WriteFile(h.Executable, []byte("ancien"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	h.run("update", "--yes").mustFail(t, "is not writable", "sudo", "WATCHOO_INSTALL_DIR")
}

// "watchoo serve" checks at startup: notice on stderr and banner in the web interface.
func TestServeAnnouncesNewVersion(t *testing.T) {
	h := newSyncedWorkspace(t)
	setBuildVars(t, "v0.5.0", "", "")
	srv := selfupdatetest.New(t, "v0.6.0", runtime.GOOS, runtime.GOARCH)
	h.Updater = selfupdate.New(selfupdate.Options{BaseURL: srv.URL})
	run := startServe(t, h, "--interval", "0")
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, body := run.get(t, "/")
		if strings.Contains(body, `id="update-banner"`) {
			assertContains(t, "banner", body, "v0.5.0 → v0.6.0", "watchoo update", srv.URL+"/tag/v0.6.0")
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("banner missing:\n%s", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	r := run.stop(t).mustSucceed(t)
	if strings.Count(r.stderr, wantNotice) != 1 {
		t.Errorf("stderr = %q, want the notice once", r.stderr)
	}
}

func TestServeUpdateCheckDisabled(t *testing.T) {
	h := newSyncedWorkspace(t)
	setBuildVars(t, "v0.5.0", "", "")
	t.Setenv(selfupdate.EnvNoCheck, "1")
	srv := selfupdatetest.New(t, "v0.6.0", runtime.GOOS, runtime.GOARCH)
	h.Updater = selfupdate.New(selfupdate.Options{BaseURL: srv.URL})
	run := startServe(t, h, "--interval", "0")
	_, body := run.get(t, "/")
	assertNotContains(t, "page", body, `id="update-banner"`)
	r := run.stop(t).mustSucceed(t)
	assertNotContains(t, "stderr", r.stderr, "New Watchoo version")
	if n := srv.Requests(); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

// "watchoo tui" has no notice on the error output: it receives its check (tui.Deps.UpdateCheck) and
// announces it in its header.
func TestTUIGetsUpdateCheck(t *testing.T) {
	h := newWorkspace(t)
	setBuildVars(t, "v0.5.0", "", "")
	srv := selfupdatetest.New(t, "v0.6.0", runtime.GOOS, runtime.GOARCH)
	h.Updater = selfupdate.New(selfupdate.Options{BaseURL: srv.URL, UserAgent: "watchoo/v0.5.0"})
	h.StderrTerminal = true
	call := onTUITerminal(h, nil)

	r := h.run("tui").mustSucceed(t)
	if r.stderr != "" {
		t.Errorf("stderr = %q, want empty (notice in the TUI only)", r.stderr)
	}
	if n := srv.Requests(); n != 0 {
		t.Errorf("%d requests before the TUI check, want 0", n)
	}
	check := call.deps.UpdateCheck
	if check == nil {
		t.Fatal("UpdateCheck nil")
	}
	info, err := check(t.Context())
	if err != nil || info != (tui.UpdateInfo{Current: "v0.5.0", Latest: "v0.6.0"}) {
		t.Errorf("UpdateCheck = %+v, %v", info, err)
	}
	// In the Docker image: reported to the TUI, which adapts its sentence.
	h.InDocker = true
	h.run("tui").mustSucceed(t)
	if info, _ := call.deps.UpdateCheck(t.Context()); !info.Docker {
		t.Errorf("UpdateCheck = %+v, want Docker", info)
	}
	// Up to date: nothing to announce.
	setBuildVars(t, "v0.6.0", "", "")
	h.run("tui").mustSucceed(t)
	if info, err := call.deps.UpdateCheck(t.Context()); err != nil || info.Latest != "" {
		t.Errorf("UpdateCheck = %+v, %v, want empty", info, err)
	}
	// Check turned off: no function.
	for _, tt := range []struct {
		name string
		set  func()
	}{
		{"variable", func() { t.Setenv(selfupdate.EnvNoCheck, "1") }},
		{"dev", func() { t.Setenv(selfupdate.EnvNoCheck, ""); setBuildVars(t, "dev", "", "") }},
	} {
		name := tt.name
		tt.set()
		h.run("tui").mustSucceed(t)
		if call.deps.UpdateCheck != nil {
			t.Errorf("%s: UpdateCheck set, want nil", name)
		}
	}
}
