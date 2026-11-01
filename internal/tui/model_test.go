package tui

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// lineWith returns the first line of the screen that contains sub, or "".
func lineWith(screen, sub string) string {
	for line := range strings.Lines(screen) {
		if strings.Contains(line, sub) {
			return strings.TrimRight(line, "\n")
		}
	}
	return ""
}

// mustContain checks that the screen contains each of the parts.
func mustContain(t *testing.T, screen string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(screen, p) {
			t.Errorf("the screen does not contain %q:\n%s", p, screen)
		}
	}
}

// mustNotContain checks that the screen contains none of the parts.
func mustNotContain(t *testing.T, screen string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(screen, p) {
			t.Errorf("the screen contains %q:\n%s", p, screen)
		}
	}
}

func TestKeyMsgStrings(t *testing.T) {
	for _, k := range []string{"enter", "esc", "tab", "shift+tab", "backspace", "up", "down", "pgdown",
		"ctrl+c", "ctrl+u", "q", "S", "/", "?", "1"} {
		if got := keyMsg(k).String(); got != k {
			t.Errorf("keyMsg(%q).String() = %q", k, got)
		}
	}
}

func TestReposViewAndHeader(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	screen := l.screen()

	mustContain(t, screen, "█▄    ▄█", "█ ●██● █", "Watchoo v0.1.0", "licence-projet",
		"[1 Repositories]", "2 Commits", "3 Contributors", "Manual sync (s)",
		"REPOSITORY", "LAST COMMIT", "AUTHOR", "STATE")
	if strings.Contains(screen, "\x1b") {
		t.Errorf("colorless screen with an escape sequence:\n%q", screen)
	}

	tests := []struct{ repo, when, state string }{
		{"Alice Martin", "2 h ago", "-"},
		{"promo/bob", "9 d ago", "~ 9 d"},
		{"promo/chloe", "42 d ago", "⚠ 42 d"},
		{"promo/dan", "never", "never"},
	}
	for _, tt := range tests {
		line := lineWith(screen, tt.repo)
		if line == "" || !strings.Contains(line, tt.when) || !strings.HasSuffix(line, tt.state) {
			t.Errorf("line of %s = %q; want %q and the state %q at the end", tt.repo, line, tt.when, tt.state)
		}
	}
	// The first line is selected.
	if line := lineWith(screen, "Alice Martin"); !strings.HasPrefix(line, "› ") {
		t.Errorf("selected line = %q; want the \"›\" marker", line)
	}
	// The screen has exactly the height of the terminal.
	if n := strings.Count(screen, "\n") + 1; n != defaultHeight {
		t.Errorf("screen of %d lines; want %d", n, defaultHeight)
	}
	if v := l.m.View(); !v.AltScreen {
		t.Error("View().AltScreen = false; want the alternate screen")
	}
}

func TestASCIIAndColor(t *testing.T) {
	deps := testDeps(newFakeData(), &fakeSyncer{})
	deps.Unicode = false
	l := newLoop(t, deps)
	screen := l.screen()
	mustContain(t, screen, `|\    /|`, "| o##o |", "! 42 d", "> Alice Martin")
	mustNotContain(t, screen, "⚠", "●", "›")

	deps = testDeps(newFakeData(), &fakeSyncer{})
	deps.Color = true
	l = newLoop(t, deps)
	if screen := l.screen(); !strings.Contains(screen, "\x1b[") {
		t.Errorf("colored screen without an escape sequence:\n%s", screen)
	}
}

func TestNavigationBetweenViews(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))

	l.press("tab")
	screen := l.screen()
	mustContain(t, screen, "[2 Commits]", "MESSAGE", "Ajoute la page d'accueil", "1111111", "rouge corrige")
	mustNotContain(t, screen, "Détails", "\x1b", "[31m")

	l.press("tab")
	screen = l.screen()
	mustContain(t, screen, "[3 Contributors]", "CONTRIBUTOR", "alice-gh", "+10/-2")
	if line := lineWith(screen, "Bob"); !strings.Contains(line, " - ") {
		t.Errorf("line of Bob = %q; want \"-\" for a missing login", line)
	}

	l.press("tab")
	mustContain(t, l.screen(), "[1 Repositories]")
	l.press("shift+tab")
	mustContain(t, l.screen(), "[3 Contributors]")
	l.press("1")
	mustContain(t, l.screen(), "[1 Repositories]")
	l.press("2")
	mustContain(t, l.screen(), "[2 Commits]")
	l.press("3")
	mustContain(t, l.screen(), "[3 Contributors]")
}

func TestCursorMovement(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l.press("j", "down")
	if line := lineWith(l.screen(), "promo/chloe"); !strings.HasPrefix(line, "› ") {
		t.Errorf("after two moves down, line = %q; want chloe selected", line)
	}
	l.press("j", "j", "j") // bounded to the last line
	if c := l.m.lists[viewRepos].cursor; c != 3 {
		t.Errorf("cursor = %d; want 3 (last line)", c)
	}
	l.press("k", "up", "up", "up", "up")
	if c := l.m.lists[viewRepos].cursor; c != 0 {
		t.Errorf("cursor = %d; want 0", c)
	}
	l.press("G")
	if c := l.m.lists[viewRepos].cursor; c != 3 {
		t.Errorf("after G, cursor = %d; want 3", c)
	}
	l.press("g")
	if c := l.m.lists[viewRepos].cursor; c != 0 {
		t.Errorf("after g, cursor = %d; want 0", c)
	}
}

func TestScrollingSmallTerminal(t *testing.T) {
	data := newFakeData()
	for i := range 30 {
		data.repos = append(data.repos, summary(int64(10+i), "promo", "eleve-"+string(rune('a'+i%26))+string(rune('a'+i/26)),
			"", testNow.Add(-time.Hour), "Élève"))
	}
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	l.send(tea.WindowSizeMsg{Width: 60, Height: 12})
	for range 12 {
		l.press("pgdown")
	}
	screen := l.screen()
	if n := strings.Count(screen, "\n") + 1; n != 12 {
		t.Errorf("screen of %d lines; want 12", n)
	}
	if c := l.m.lists[viewRepos].cursor; c != len(data.repos)-1 {
		t.Errorf("cursor = %d; want the last line (%d)", c, len(data.repos)-1)
	}
	if !strings.Contains(screen, "› ") {
		t.Errorf("the selected line is not visible:\n%s", screen)
	}
	for line := range strings.Lines(screen) {
		if w := width(strings.TrimRight(line, "\n")); w > 60 && !strings.Contains(line, "Watchoo") {
			t.Errorf("line of %d columns for a 60-column terminal: %q", w, line)
		}
	}
	// A tiny terminal does not make the rendering panic.
	l.send(tea.WindowSizeMsg{Width: 10, Height: 3})
	_ = l.screen()
}

func TestFilter(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l.press("/", "B", "o")
	screen := l.screen()
	mustContain(t, screen, "promo/bob", "Filter: Bo")
	mustNotContain(t, screen, "Alice Martin", "promo/chloe")

	l.press("backspace", "backspace", "c", "h", "l", "enter")
	screen = l.screen()
	mustContain(t, screen, "promo/chloe", "Filter: chl (esc to clear it)")
	mustNotContain(t, screen, "promo/bob")
	if l.m.filtering {
		t.Error("filter input still active after enter")
	}
	// "S" syncs the selected filtered line: chloe.
	if id, ok := l.m.selectedRepoID(); !ok || id != 3 {
		t.Errorf("selected repository = %d, %v; want 3", id, ok)
	}

	l.press("esc")
	mustContain(t, l.screen(), "promo/bob", "Alice Martin")

	// No result, then esc during the input clears the filter.
	l.press("/", "z", "z")
	mustContain(t, l.screen(), `No row matches the filter "zz"`)
	l.press("esc")
	mustContain(t, l.screen(), "Alice Martin")

	// During the input, "q" is typed into the filter, ctrl+c quits.
	l.press("/", "q")
	if l.quit || l.m.lists[viewRepos].filter != "q" {
		t.Errorf("\"q\" during the input: quit=%v, filter=%q", l.quit, l.m.lists[viewRepos].filter)
	}
	l.press("ctrl+u")
	if f := l.m.lists[viewRepos].filter; f != "" {
		t.Errorf("after ctrl+u, filter = %q", f)
	}
	l.press("ctrl+c")
	l.settle()
	if !l.quit {
		t.Error("ctrl+c during the input did not quit")
	}
}

func TestFilterIsPerView(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l.press("/", "b", "o", "b", "enter", "tab")
	mustContain(t, l.screen(), "Ajoute la page d'accueil")
	l.press("/", "a", "l", "i", "enter")
	screen := l.screen()
	mustContain(t, screen, "Ajoute la page d'accueil")
	mustNotContain(t, screen, "corrige")
	l.press("shift+tab")
	screen = l.screen()
	mustContain(t, screen, "promo/bob")
	mustNotContain(t, screen, "Alice Martin")
}

func TestDetail(t *testing.T) {
	data := newFakeData()
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	l.press("enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	screen := l.screen()
	mustContain(t, screen, "Alice Martin (promo/alice)", "https://github.com/promo/alice", "Active",
		"thresholds: Slacking after 5 d, Dead after 15 d", "2 tracked branches, default: main, all branches",
		"2 h ago by Alice (abcdef0)", "successful (1 new commit)",
		"Latest commits", "Ajoute la page d'accueil", "Top contributors")
	mustNotContain(t, screen, "corrige") // commit of another repository

	// Weekly activity of the repository's contributors (12 weeks, most recent on the right): Alice
	// 3 commits (2 this week, 1 two weeks ago); Bob, his only commit outside the window.
	mustContain(t, screen, "activity over 12 weeks, most recent on the right", "12 WK", "PER WEEK")
	alice := lineWith(screen, "▁▁▁▁▁▁▁▁▁▅▁█")
	if !strings.Contains(alice, "Alice") || !strings.Contains(alice, " 3 ") {
		t.Errorf("line of Alice = %q; want 3 commits over 12 weeks and the curve ▁▁▁▁▁▁▁▁▁▅▁█", alice)
	}
	bob := lineWith(screen, "▁▁▁▁▁▁▁▁▁▁▁▁")
	if !strings.Contains(bob, "Bob") || !strings.Contains(bob, " 0 ") {
		t.Errorf("line of Bob = %q; want 0 commits over 12 weeks", bob)
	}

	data.mu.Lock()
	if !slices.Equal(data.authorReads, []int64{1}) {
		t.Errorf("AuthorCommitTimes read for %v, want only repository 1", data.authorReads)
	}
	last := data.commitFilters[len(data.commitFilters)-1]
	data.mu.Unlock()
	if last.RepoID != 1 || last.Limit != detailCommits {
		t.Errorf("detail filter = %+v; want RepoID 1, Limit %d", last, detailCommits)
	}

	l.press("esc")
	if l.m.detail.open {
		t.Error("esc did not close the detail")
	}
	mustContain(t, l.screen(), "LAST COMMIT")

	// Enter on a commit opens the detail of its repository.
	l.press("tab", "j", "enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	mustContain(t, l.screen(), "promo/bob", "Slacking (~ 9 d)")
	// Enter on a contributor opens their detail (0.5.0), not a repository's.
	l.press("3", "enter")
	if l.m.detail.open || !l.m.cdetail.open {
		t.Error("enter in the contributors view did not open the contributor detail")
	}
}

func TestDetailGrayAndError(t *testing.T) {
	data := newFakeData()
	data.repos[2].LastRun = model.SyncRun{ID: 9, StartedAt: testNow.Add(-5 * time.Minute), Status: model.SyncError,
		Error: "dépôt introuvable ou inaccessible"}
	data.repos[2].Enabled = false
	data.repos[2].BranchPatterns = []string{"main", "release/*"}
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	if line := lineWith(l.screen(), "promo/chloe"); !strings.HasSuffix(line, "stopped") {
		t.Errorf("line of chloe = %q; want the gray state \"stopped\"", line)
	}
	l.press("j", "j", "enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	mustContain(t, l.screen(), "Tracking stopped", "patterns: main, release/*",
		"5 min ago, error: dépôt introuvable ou inaccessible", "tracking stopped")
}

func TestSyncAllWithProgress(t *testing.T) {
	data := newFakeData()
	gate := make(chan struct{})
	syncer := &fakeSyncer{
		repos: []wsync.RepoReport{
			{RepoID: 1, DisplayName: "Alice Martin", Status: wsync.StatusSuccess, NewCommits: 2},
			{RepoID: 2, DisplayName: "promo/bob", Status: wsync.StatusSuccess, NewCommits: 1},
		},
		gate: gate,
	}
	syncer.report = wsync.Report{Repos: syncer.repos}
	l := newLoop(t, testDeps(data, syncer))
	loadsBefore := data.loadCount()

	l.press("s")
	l.until(func(m Model) bool { return m.sync.done == 1 && m.sync.current == "promo/bob" })
	mustContain(t, l.screen(), "Sync in progress: 1/2 repositories (promo/bob)…")

	// A second request during the sync is ignored and reported.
	l.press("S")
	mustContain(t, l.screen(), "a sync is already running (all repositories)")
	if all, repos := syncer.counts(); all != 1 || len(repos) != 0 {
		t.Errorf("SyncAll %d times, SyncRepo %v; want a single sync", all, repos)
	}

	close(gate)
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
	screen := l.screen()
	mustContain(t, screen, "sync finished: 2 repositories, 3 new commits", "Last sync just now")
	if data.loadCount() <= loadsBefore {
		t.Error("the database was not reread after the sync")
	}
	if l.m.statusErr {
		t.Error("summary without error shown as an error")
	}
}

func TestSyncSelectedRepo(t *testing.T) {
	syncer := &fakeSyncer{
		repos: []wsync.RepoReport{{RepoID: 2, DisplayName: "promo/bob", Status: wsync.StatusError,
			Error: "dépôt introuvable ou inaccessible"}},
	}
	syncer.report = wsync.Report{Repos: syncer.repos}
	l := newLoop(t, testDeps(newFakeData(), syncer))
	l.press("j", "S")
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
	if _, repos := syncer.counts(); len(repos) != 1 || repos[0] != 2 {
		t.Errorf("SyncRepo called with %v; want [2]", repos)
	}
	mustContain(t, l.screen(), "promo/bob: dépôt introuvable ou inaccessible")
	if !l.m.statusErr {
		t.Error("failure of a repository not shown as an error")
	}

	// In the contributors view, no repository is selected.
	l.press("3", "S")
	mustContain(t, l.screen(), "no repository selected")
}

func TestSyncSummaryDeferredLocalTime(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("Europe/Paris time zone unavailable")
	}
	resume := time.Date(2026, 10, 27, 13, 30, 0, 0, time.UTC) // 14:30 in Paris (winter time)
	syncer := &fakeSyncer{}
	syncer.report = wsync.Report{Repos: []wsync.RepoReport{
		{RepoID: 1, Status: wsync.StatusSuccess, NewCommits: 1},
		{RepoID: 2, Status: wsync.StatusError, Error: "erreur"},
		{RepoID: 3, Status: wsync.StatusLocked, Error: "déjà en cours de synchronisation"},
		{RepoID: 4, Status: wsync.StatusDeferred, ResumeAt: resume},
	}}
	deps := testDeps(newFakeData(), syncer)
	deps.Location = paris
	l := newLoop(t, deps)
	l.send(tea.WindowSizeMsg{Width: 200, Height: 30})
	l.press("s")
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
	mustContain(t, l.screen(), "sync finished: 4 repositories, 1 new commit, 1 in error, "+
		"1 already running elsewhere, 1 deferred (low GitHub quota, resuming around 14:30)")
	if !l.m.statusErr {
		t.Error("summary with errors not reported as an error")
	}

	// A single deferred repository: its local resume time.
	m := l.m
	text, bad := m.summarize(wsync.Report{Repos: []wsync.RepoReport{
		{DisplayName: "promo/bob", Status: wsync.StatusDeferred, ResumeAt: resume}}})
	if text != "promo/bob: deferred: low GitHub quota until 14:30" || !bad {
		t.Errorf("summarize = %q, %v", text, bad)
	}
	if text, _ := m.summarize(wsync.Report{}); text != "sync finished: no tracked repository" {
		t.Errorf("summarize(empty) = %q", text)
	}
}

func TestSyncGlobalError(t *testing.T) {
	syncer := &fakeSyncer{err: errors.New(`gh is not signed in: run "gh auth login"`)}
	l := newLoop(t, testDeps(newFakeData(), syncer))
	l.press("s")
	l.until(func(m Model) bool { return !m.sync.running })
	mustContain(t, l.screen(), "sync failed: gh is not signed in")
}

func TestAutoSync(t *testing.T) {
	syncer := &fakeSyncer{repos: []wsync.RepoReport{{RepoID: 1, Status: wsync.StatusSuccess}}}
	syncer.report = wsync.Report{Repos: syncer.repos}
	deps := testDeps(newFakeData(), syncer)
	deps.Interval = 30 * time.Minute
	l := newLoop(t, deps)
	l.settle()

	ticks := l.ticksOf(30 * time.Minute)
	if len(ticks) != 1 {
		t.Fatalf("%d automatic syncs scheduled; want 1", len(ticks))
	}
	mustContain(t, l.screen(), "Auto sync every 30 min, next at 12:30")

	l.send(ticks[0].msg)
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
	if all, _ := syncer.counts(); all != 1 {
		t.Errorf("SyncAll called %d times; want 1 (automatic sync)", all)
	}
	if n := len(l.ticksOf(30 * time.Minute)); n != 2 {
		t.Errorf("%d automatic syncs scheduled; want 2 (the next one)", n)
	}

	// An automatic sync during a running sync is skipped, the next one stays scheduled.
	gate := make(chan struct{})
	syncer.gate = gate
	syncer.repos = append(syncer.repos, wsync.RepoReport{RepoID: 2})
	l.press("s")
	l.until(func(m Model) bool { return m.sync.done == 1 })
	l.send(autoSyncMsg{})
	l.until(func(Model) bool { return len(l.ticksOf(30*time.Minute)) == 3 })
	close(gate)
	l.until(func(m Model) bool { return !m.sync.running })
	if all, _ := syncer.counts(); all != 2 {
		t.Errorf("SyncAll called %d times; want 2 (the automatic sync skipped)", all)
	}
}

func TestManualOnlyHasNoAutoTick(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l.settle()
	for _, tk := range l.ticks {
		if tk.d != clockInterval {
			t.Errorf("unexpected tick of %s with manual sync", tk.d)
		}
	}
	if len(l.ticksOf(clockInterval)) != 1 {
		t.Error("date refresh not scheduled")
	}
	l.send(clockMsg{})
	l.until(func(Model) bool { return len(l.ticksOf(clockInterval)) == 2 })
}

func TestQuitCancelsContext(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	ctx := l.m.ctx
	l.press("q")
	l.settle()
	if !l.quit {
		t.Error("\"q\" did not quit")
	}
	if ctx.Err() == nil {
		t.Error("the model context is not cancelled when quitting")
	}
}

func TestQuitStopsRunningSync(t *testing.T) {
	gate := make(chan struct{}) // never closed: only the cancellation stops the sync
	syncer := &fakeSyncer{repos: []wsync.RepoReport{{RepoID: 1}, {RepoID: 2}}, gate: gate}
	l := newLoop(t, testDeps(newFakeData(), syncer))
	l.press("s")
	l.until(func(m Model) bool { return m.sync.done == 1 })
	l.press("q")
	l.settle()
	if !l.quit {
		t.Error("\"q\" did not quit during the sync")
	}
	if strings.Contains(l.m.status, "failed") {
		t.Errorf("error shown when quitting: %q", l.m.status)
	}
}

func TestHelpScreen(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l.press("?")
	mustContain(t, l.screen(), `Keys  ("?" or esc to close)`, "sync everything", "sync this repository",
		"reload the database", "quit", "repository or contributor detail", "shift+tab", "pgup")
	l.press("j") // ignored
	l.press("esc")
	mustContain(t, l.screen(), "LAST COMMIT")
	l.press("?", "?")
	mustNotContain(t, l.screen(), "or esc to close")
}

func TestReloadAndLoadError(t *testing.T) {
	data := newFakeData()
	data.err = errBoom
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	mustContain(t, l.screen(), "Cannot read the database", "cannot read the database: database unavailable")

	data.mu.Lock()
	data.err = nil
	data.mu.Unlock()
	l.press("r")
	l.until(func(m Model) bool { return !m.loading })
	mustContain(t, l.screen(), "Alice Martin")
}

func TestEmptyWorkspace(t *testing.T) {
	l := newLoop(t, testDeps(&fakeData{}, &fakeSyncer{}))
	mustContain(t, l.screen(), "No tracked repository", "watchoo repo add owner/repo")
	l.press("tab")
	mustContain(t, l.screen(), "No commit in the database")
	l.press("tab")
	mustContain(t, l.screen(), "No contributor in the database")
	l.press("enter", "S")
	if l.m.detail.open {
		t.Error("detail opened without a repository")
	}
}

func TestNewModelDefaults(t *testing.T) {
	m := NewModel(t.Context(), Deps{})
	if m.deps.Location != time.Local || m.deps.CommitLimit != defaultCommitLimit {
		t.Errorf("defaults: Location=%v, CommitLimit=%d", m.deps.Location, m.deps.CommitLimit)
	}
	if m.Init() == nil {
		t.Error("Init returns no command")
	}
	// Without a Syncer, the sync is refused cleanly.
	next, _ := m.startSync(0, false)
	if !strings.Contains(next.status, "unavailable") {
		t.Errorf("status = %q", next.status)
	}
}

func TestHelpers(t *testing.T) {
	if got := truncate("abcdef", 4, "…"); got != "abc…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("abc", 1, "…"); got != "a" {
		t.Errorf("narrow truncate = %q", got)
	}
	if got := pad("12", 4, true, ""); got != "  12" {
		t.Errorf("right pad = %q", got)
	}
	if got := clean("a\x1b[31mb\u202ec\nd\x1b]8;;http://x\x07e\x1b[0m"); got != "ab c de" {
		t.Errorf("clean = %q", got)
	}
	if got := firstLine("\n  titre  \ncorps"); got != "titre" {
		t.Errorf("firstLine = %q", got)
	}
	for d, want := range map[time.Duration]string{15 * time.Minute: "15 min", time.Hour: "1 h",
		90 * time.Minute: "1 h 30"} {
		if got := formatInterval(d); got != want {
			t.Errorf("formatInterval(%s) = %q; want %q", d, got, want)
		}
	}
	for v, want := range map[string]string{"0.1.0": "v0.1.0", "v0.1.0": "v0.1.0", "dev": "dev", "": ""} {
		if got := displayVersion(v); got != want {
			t.Errorf("displayVersion(%q) = %q", v, got)
		}
	}
	if newStyles(false).Title.Render("x") != "x" {
		t.Error("colorless style with an escape sequence")
	}
}

// TestWithRealStore reads a real temporary SQLite database, filled through the store API.
func TestWithRealStore(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	repo, err := st.CreateRepository(ctx, model.Repository{Provider: model.ProviderGitHub, ProviderID: 42,
		Owner: "promo", Name: "zoe", Label: "Zoé Leroy", URL: "https://github.com/promo/zoe", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.BeginSyncRun(ctx, repo.ID, testNow.Add(-time.Minute), wsync.LockStaleAfter)
	if err != nil {
		t.Fatal(err)
	}
	zoe := store.Identity{Name: "Zoé", Email: "zoe@example.test", Login: "zoe-gh", ProviderUserID: 7}
	if _, err := st.ApplyRepoSync(ctx, store.RepoSyncWrite{
		RepoID: repo.ID, RunID: run.ID,
		Metadata: store.RepositoryMetadata{Owner: "promo", Name: "zoe", DefaultBranch: "main",
			URL: "https://github.com/promo/zoe"},
		Branches: []store.BranchState{{Name: "main", HeadSHA: "cafe000000000000000000000000000000000001"}},
		Commits: []store.CommitWrite{{SHA: "cafe000000000000000000000000000000000001", Message: "Premier rendu",
			Author: zoe, Committer: zoe, AuthoredAt: testNow.Add(-3 * time.Hour),
			CommittedAt: testNow.Add(-3 * time.Hour), Branches: []string{"main"}}},
		SyncedAt: testNow,
	}); err != nil {
		t.Fatal(err)
	}

	l := newLoop(t, testDeps(st, &fakeSyncer{}))
	if line := lineWith(l.screen(), "Zoé Leroy"); !strings.Contains(line, "3 h ago") ||
		!strings.Contains(line, "Zoé") || !strings.HasSuffix(line, "-") {
		t.Errorf("repository line = %q", line)
	}
	l.press("tab")
	mustContain(t, l.screen(), "Premier rendu", "cafe000")
	l.press("tab")
	mustContain(t, l.screen(), "zoe-gh")
	l.press("1", "enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	mustContain(t, l.screen(), "1 tracked branch, default: main", "Premier rendu", "Top contributors")
	mustNotContain(t, l.screen(), "zoe@example.test")
	// Weekly activity read from the real database: Zoé's commit, this week.
	if line := lineWith(l.screen(), "▁▁▁▁▁▁▁▁▁▁▁█"); !strings.Contains(line, "Zoé") {
		t.Errorf("line of Zoé = %q; want the curve ▁▁▁▁▁▁▁▁▁▁▁█", line)
	}
}

// Repository-specific thresholds: applied and reported in the detail.
func TestDetailOwnThresholds(t *testing.T) {
	data := newFakeData()
	flemme := 1
	data.repos[0].FlemmeAfterDays = &flemme // Alice: last commit 2 h ago, still Active
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	l.press("enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	mustContain(t, l.screen(), "thresholds: Slacking after 1 d, Dead after 15 d (specific to the repository)")
}

// Repository detail: open pull requests and latest release (V2); warnings in the summary.
func TestDetailPullsAndRelease(t *testing.T) {
	data := newFakeData()
	data.repos[0].OpenPullRequests = 2
	data.repos[0].LatestRelease = model.Release{ProviderID: 1, TagName: "v1.0\x1b[2J",
		PublishedAt: data.repos[0].LastCommitAt}
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	l.press("enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	screen := l.screen()
	mustContain(t, screen, "Pull requests", "2 open, latest release v1.0 (2 h ago)")
	mustNotContain(t, screen, "\x1b[2J")

	if got := l.m.pullsText(0, model.Release{}); got != "none open, no release" {
		t.Errorf("pullsText without anything = %q", got)
	}
	text, bad := l.m.summarize(wsync.Report{Repos: []wsync.RepoReport{
		{DisplayName: "a", Status: wsync.StatusSuccess, Warnings: []string{"tags non synchronisés : x"}},
		{DisplayName: "b", Status: wsync.StatusSuccess}}})
	if text != "sync finished: 2 repositories, 0 new commits, 1 warning (pull requests, releases or tags)" || bad {
		t.Errorf("summarize with a warning = %q, %v", text, bad)
	}
}

// Repository tracked only by local Git clone (unknown forge ID, V2): stars, issues and pull requests
// are reported as not available.
func TestDetailWithoutForgeDetails(t *testing.T) {
	data := newFakeData()
	data.repos[0].ProviderID = 0
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	l.press("enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	mustContain(t, l.screen(), "stars and issues not available (local Git clone)",
		"not available (local Git clone, without gh)")
	mustNotContain(t, l.screen(), "no release")
}
