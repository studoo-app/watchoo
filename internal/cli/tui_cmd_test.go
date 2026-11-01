package cli

// Tests of "watchoo tui": refusal off a terminal, interval, dependencies passed to the interactive mode.
// The real TUI is never started: Deps.RunTUI is replaced.

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/tui"
)

// tuiCall keeps the arguments received by the fake RunTUI.
type tuiCall struct {
	calls int
	deps  tui.Deps
	term  tui.IO
}

// onTUITerminal describes a Unicode terminal without color, and installs a fake RunTUI that records its
// arguments and returns ret.
func onTUITerminal(h *harness, ret error) *tuiCall {
	call := &tuiCall{}
	h.Terminal = func() TermInfo { return TermInfo{IsTerminal: true, Width: 100, Unicode: true} }
	h.StdinTerminal = true
	h.RunTUI = func(_ context.Context, d tui.Deps, term tui.IO) error {
		call.calls++
		call.deps, call.term = d, term
		return ret
	}
	return call
}

func TestTUICmdRefusesNonTerminal(t *testing.T) {
	h := newWorkspace(t)
	call := onTUITerminal(h, nil)

	h.Terminal = func() TermInfo { return TermInfo{} } // redirected output
	h.run("tui").mustFail(t, "must be run in an interactive terminal", "watchoo repo list")
	h.Terminal = func() TermInfo { return TermInfo{IsTerminal: true, Width: 80} }
	h.StdinTerminal = false // redirected input
	h.run("tui").mustFail(t, "must be run in an interactive terminal")

	if call.calls != 0 {
		t.Errorf("RunTUI called %d times off a terminal", call.calls)
	}
}

func TestTUICmdRefusesJSONAndArgs(t *testing.T) {
	h := newWorkspace(t)
	call := onTUITerminal(h, nil)
	h.run("tui", "--json").mustFail(t, "has no JSON output")
	h.run("tui", "extra").mustFail(t, "takes no argument")
	if call.calls != 0 {
		t.Errorf("RunTUI called %d times", call.calls)
	}
}

func TestTUICmdPassesDeps(t *testing.T) {
	h := newSyncedWorkspace(t)
	call := onTUITerminal(h, nil)
	h.Terminal = func() TermInfo { return TermInfo{IsTerminal: true, Width: 100, Unicode: true, Color: true} }
	h.Stdin = "clavier"

	r := h.run("tui").mustSucceed(t)
	if r.stdout != "" || r.stderr != "" {
		t.Errorf("non-empty outputs:\nstdout: %q\nstderr: %q", r.stdout, r.stderr)
	}
	if call.calls != 1 {
		t.Fatalf("RunTUI called %d times; want 1", call.calls)
	}
	d := call.deps
	if d.Interval != 30*time.Minute {
		t.Errorf("Interval = %s; want 30m (sync.auto, default sync.interval)", d.Interval)
	}
	if d.Config.Name != "licence-projet" || d.Location == nil || d.Location.String() != "UTC" {
		t.Errorf("Config.Name = %q, Location = %v", d.Config.Name, d.Location)
	}
	if d.Version != Version || !d.Unicode || !d.Color || d.Now == nil || !d.Now().Equal(testNow) {
		t.Errorf("Version=%q Unicode=%v Color=%v Now nil=%v", d.Version, d.Unicode, d.Color, d.Now == nil)
	}
	if call.term.Out == nil || call.term.In == nil {
		t.Fatalf("incomplete IO: %+v", call.term)
	}
	if in, err := io.ReadAll(call.term.In); err != nil || string(in) != "clavier" {
		t.Errorf("IO.In = %q, %v; want the standard input", in, err)
	}
	if d.Data == nil || d.Syncer == nil {
		t.Fatal("Data or Syncer missing")
	}
}

// The dependencies received by the TUI read the database and sync during the TUI, without any log.
func TestTUICmdDepsWork(t *testing.T) {
	h := newSyncedWorkspace(t)
	var summaries, synced int
	var syncErr, readErr error
	onTUITerminal(h, nil)
	h.RunTUI = func(ctx context.Context, d tui.Deps, _ tui.IO) error {
		rows, err := d.Data.RepositorySummaries(ctx)
		summaries, readErr = len(rows), err
		rep, err := d.Syncer.SyncAll(ctx, wsync.SyncOptions{})
		synced, syncErr = rep.Count(wsync.StatusSuccess), err
		return nil
	}
	r := h.run("tui").mustSucceed(t)
	if readErr != nil || summaries != 3 {
		t.Errorf("RepositorySummaries: %d rows, %v; want 3", summaries, readErr)
	}
	if syncErr != nil || synced != 3 {
		t.Errorf("SyncAll: %d successes, %v; want 3", synced, syncErr)
	}
	if r.stderr != "" {
		t.Errorf("logs or messages on stderr during the TUI:\n%s", r.stderr)
	}
}

func TestTUICmdInterval(t *testing.T) {
	h := newWorkspace(t)
	call := onTUITerminal(h, nil)
	for _, tt := range []struct {
		args []string
		want time.Duration
	}{
		{[]string{"tui", "--interval", "1h"}, time.Hour},
		{[]string{"tui", "--interval", "15m"}, 15 * time.Minute},
		{[]string{"tui", "--interval", "0"}, 0},
	} {
		h.run(tt.args...).mustSucceed(t)
		if call.deps.Interval != tt.want {
			t.Errorf("%v: Interval = %s; want %s", tt.args, call.deps.Interval, tt.want)
		}
	}
	// Complete messages of internal/sync (AutoInterval).
	h.run("tui", "--interval", "10m").mustFail(t, "--interval: 10m0s is below the minimum of 15 min")
	h.run("tui", "--interval", "-1h").mustFail(t, "--interval: -1h0m0s is negative")

	// sync.auto false: manual sync, except with --interval.
	t.Setenv("WATCHOO_SYNC_AUTO", "false")
	h.run("tui").mustSucceed(t)
	if call.deps.Interval != 0 {
		t.Errorf("sync.auto false: Interval = %s; want 0", call.deps.Interval)
	}
	h.run("tui", "--interval", "45m").mustSucceed(t)
	if call.deps.Interval != 45*time.Minute {
		t.Errorf("sync.auto false and --interval 45m: Interval = %s", call.deps.Interval)
	}
}

func TestTUICmdReturnsTUIError(t *testing.T) {
	h := newWorkspace(t)
	onTUITerminal(h, errors.New("interactive mode: terminal lost"))
	r := h.run("tui").mustFail(t, "interactive mode: terminal lost")
	if strings.Count(r.stderr, i18n.T("cli.core.err.line", "")) != 1 {
		t.Errorf("error printed several times:\n%s", r.stderr)
	}
}
