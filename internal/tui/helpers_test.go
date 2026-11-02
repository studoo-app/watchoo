package tui

// TUI test helpers: fake data, fake Syncer, keys and a small event loop that runs the Bubble Tea
// commands like tea.Program would (without a terminal).

import (
	"context"
	"errors"
	"slices"
	gosync "sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// testNow is the fixed time of the tests.
var testNow = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

// fakeData is an in-memory DataSource that counts the reads and keeps the filters it receives.
type fakeData struct {
	mu            gosync.Mutex
	repos         []store.RepositorySummary
	commits       []store.CommitView
	contribs      []store.ContributorStats
	err           error
	loads         int
	commitFilters []store.CommitFilter
	contribFilter []store.ContributorFilter
	languages     []store.LanguageView
	// authorTimes: author and date of the commits, per repository (AuthorCommitTimes).
	authorTimes map[int64][]model.AuthorCommitTime
	authorReads []int64 // repositories asked of AuthorCommitTimes
	// Contributor detail (0.5.0), per contributor ID.
	profiles     map[int64]store.ContributorProfile
	contribRepos map[int64][]store.ContributorRepoStats
	repoTimes    map[int64][]model.RepoCommitTime
}

func (f *fakeData) ContributorProfile(_ context.Context, id int64) (store.ContributorProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.profiles[id]
	if !ok && f.err == nil {
		return p, store.ErrNotFound
	}
	return p, f.err
}

func (f *fakeData) ContributorRepos(_ context.Context, id int64) ([]store.ContributorRepoStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.contribRepos[id]), f.err
}

func (f *fakeData) ContributorCommitTimes(_ context.Context, id int64, since time.Time) ([]model.RepoCommitTime, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.RepoCommitTime
	for _, rt := range f.repoTimes[id] {
		if !rt.CommittedAt.Before(since) {
			out = append(out, rt)
		}
	}
	return out, f.err
}

func (f *fakeData) RepositorySummaries(context.Context) ([]store.RepositorySummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	return slices.Clone(f.repos), f.err
}

func (f *fakeData) ListRecentCommits(_ context.Context, flt store.CommitFilter) ([]store.CommitView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitFilters = append(f.commitFilters, flt)
	var out []store.CommitView
	for _, c := range f.commits {
		if (flt.RepoID == 0 || c.RepoID == flt.RepoID) && (flt.AuthorID == 0 || c.AuthorID == flt.AuthorID) {
			out = append(out, c)
		}
	}
	return out, f.err
}

func (f *fakeData) ContributorRanking(_ context.Context, flt store.ContributorFilter) ([]store.ContributorStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contribFilter = append(f.contribFilter, flt)
	return slices.Clone(f.contribs), f.err
}

func (f *fakeData) ListLanguages(_ context.Context, repoID int64) ([]store.LanguageView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.LanguageView
	for _, l := range f.languages {
		if l.RepoID == repoID {
			out = append(out, l)
		}
	}
	return out, f.err
}

func (f *fakeData) AuthorCommitTimes(_ context.Context, repoID int64, since time.Time) ([]model.AuthorCommitTime, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authorReads = append(f.authorReads, repoID)
	var out []model.AuthorCommitTime
	for id, list := range f.authorTimes {
		if repoID != 0 && id != repoID {
			continue
		}
		for _, at := range list {
			if !at.CommittedAt.Before(since) {
				out = append(out, at)
			}
		}
	}
	return out, f.err
}

// loadCount returns the number of full reads (RepositorySummaries).
func (f *fakeData) loadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loads
}

// summary builds the summary of a synced repository (LastSyncedAt one hour before testNow).
func summary(id int64, owner, name, label string, lastCommit time.Time, author string) store.RepositorySummary {
	r := store.RepositorySummary{
		Repository: model.Repository{
			ID: id, Provider: model.ProviderGitHub, ProviderID: 100 + id, Owner: owner, Name: name, Label: label,
			DefaultBranch: "main", URL: "https://github.com/" + owner + "/" + name,
			LastSyncedAt: testNow.Add(-time.Hour), LastCommitAt: lastCommit, Enabled: true,
		},
		Commits:         3,
		TrackedBranches: 2,
		LastRun:         model.SyncRun{ID: id, RepoID: id, StartedAt: testNow.Add(-time.Hour), Status: model.SyncSuccess, NewCommits: 1},
	}
	if !lastCommit.IsZero() {
		r.LastCommit = store.CommitView{
			Commit:     model.Commit{RepoID: id, SHA: "abcdef0123456789", CommittedAt: lastCommit},
			RepoOwner:  owner,
			RepoName:   name,
			AuthorName: author,
		}
	}
	return r
}

// newFakeData returns fake data: four repositories (Active, Slacking, Dead, never synced), two
// commits and two contributors.
func newFakeData() *fakeData {
	never := summary(4, "promo", "dan", "", time.Time{}, "")
	never.LastSyncedAt, never.LastRun, never.Commits, never.TrackedBranches = time.Time{}, model.SyncRun{}, 0, 0
	return &fakeData{
		repos: []store.RepositorySummary{
			summary(1, "promo", "alice", "Alice Martin", testNow.Add(-2*time.Hour), "Alice"),
			summary(2, "promo", "bob", "", testNow.Add(-9*24*time.Hour), "Bob"),
			summary(3, "promo", "chloe", "", testNow.Add(-42*24*time.Hour), "Chloé"),
			never,
		},
		commits: []store.CommitView{
			{Commit: model.Commit{RepoID: 1, SHA: "1111111aaaaaaa", Message: "Ajoute la page d'accueil\n\nDétails",
				CommittedAt: testNow.Add(-2 * time.Hour)}, RepoOwner: "promo", RepoName: "alice",
				RepoLabel: "Alice Martin", AuthorName: "Alice", AuthorLogin: "alice-gh"},
			{Commit: model.Commit{RepoID: 2, SHA: "2222222bbbbbbb", Message: "\x1b[31mrouge\x1b[0m corrige",
				CommittedAt: testNow.Add(-9 * 24 * time.Hour)}, RepoOwner: "promo", RepoName: "bob", AuthorName: "Bob"},
		},
		contribs: []store.ContributorStats{
			{Contributor: model.Contributor{ID: 1, Name: "Alice", ProviderLogin: "alice-gh"}, Commits: 5,
				Additions: 10, Deletions: 2, KnownCommits: 5, Repos: 1, LastCommitAt: testNow.Add(-2 * time.Hour)},
			{Contributor: model.Contributor{ID: 2, Name: "Bob"}, Commits: 1, Repos: 1,
				LastCommitAt: testNow.Add(-9 * 24 * time.Hour)},
		},
		authorTimes: map[int64][]model.AuthorCommitTime{
			1: { // Alice's repository: Alice 2 commits this week and 1 two weeks ago, Bob 1 twenty weeks ago
				{ContributorID: 1, CommittedAt: testNow.Add(-2 * time.Hour)},
				{ContributorID: 1, CommittedAt: testNow.Add(-3 * time.Hour)},
				{ContributorID: 1, CommittedAt: testNow.Add(-14 * 24 * time.Hour)},
				{ContributorID: 2, CommittedAt: testNow.Add(-20 * 7 * 24 * time.Hour)},
			},
			2: {{ContributorID: 2, CommittedAt: testNow.Add(-9 * 24 * time.Hour)}},
		},
	}
}

// fakeSyncer is a fake Syncer: it sends progress events for each repository of repos and returns
// report (or err). If gate is not nil, it waits for gate after the first repository.
type fakeSyncer struct {
	mu      gosync.Mutex
	repos   []wsync.RepoReport
	report  wsync.Report
	err     error
	gate    chan struct{}
	allRuns int
	repoIDs []int64
}

func (f *fakeSyncer) SyncAll(ctx context.Context, opts wsync.SyncOptions) (wsync.Report, error) {
	f.mu.Lock()
	f.allRuns++
	f.mu.Unlock()
	return f.run(ctx, opts)
}

func (f *fakeSyncer) SyncRepo(ctx context.Context, repoID int64, opts wsync.SyncOptions) (wsync.Report, error) {
	f.mu.Lock()
	f.repoIDs = append(f.repoIDs, repoID)
	f.mu.Unlock()
	return f.run(ctx, opts)
}

func (f *fakeSyncer) run(ctx context.Context, opts wsync.SyncOptions) (wsync.Report, error) {
	progress := opts.Progress
	if progress == nil {
		progress = func(wsync.Event) {}
	}
	total := len(f.repos)
	progress(wsync.Event{Kind: wsync.EventStarted, Total: total})
	for i, r := range f.repos {
		progress(wsync.Event{Kind: wsync.EventRepoStarted, Repo: r, Done: i, Total: total})
		if i == 1 && f.gate != nil {
			select {
			case <-f.gate:
			case <-ctx.Done():
				return wsync.Report{}, ctx.Err()
			}
		}
		progress(wsync.Event{Kind: wsync.EventRepoFinished, Repo: r, Done: i + 1, Total: total})
	}
	progress(wsync.Event{Kind: wsync.EventFinished, Done: total, Total: total})
	return f.report, f.err
}

// counts returns the number of SyncAll calls and the repositories passed to SyncRepo.
func (f *fakeSyncer) counts() (int, []int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.allRuns, slices.Clone(f.repoIDs)
}

// testDeps returns test dependencies without color, in Unicode, in UTC.
func testDeps(data DataSource, syncer Syncer) Deps {
	return Deps{
		Data:     data,
		Syncer:   syncer,
		Config:   workspace.DefaultConfig("licence-projet"),
		Location: time.UTC,
		Now:      func() time.Time { return testNow },
		Version:  "0.1.0",
		Unicode:  true,
	}
}

// scheduledTick replaces tea.Tick in the tests: the message is kept, not sent.
type scheduledTick struct {
	d   time.Duration
	msg tea.Msg
}

// loop runs the model like tea.Program: each command runs in a goroutine and its message goes back
// through Update.
type loop struct {
	t     *testing.T
	m     Model
	msgs  chan tea.Msg
	ticks []scheduledTick
	quit  bool
}

// newLoop builds the model (ticks intercepted), runs Init and waits for the first load.
func newLoop(t *testing.T, deps Deps) *loop {
	t.Helper()
	m := NewModel(t.Context(), deps)
	m.tick = func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		return func() tea.Msg { return scheduledTick{d: d, msg: fn(testNow.Add(d))} }
	}
	l := &loop{t: t, m: m, msgs: make(chan tea.Msg, 64)}
	l.do(m.Init())
	l.until(func(m Model) bool { return !m.loading })
	return l
}

// do runs cmd in a goroutine.
func (l *loop) do(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() { l.msgs <- cmd() }()
}

// send handles a message: command batches, ticks kept, Quit noted, the rest passed to Update.
func (l *loop) send(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			l.do(c)
		}
	case scheduledTick:
		l.ticks = append(l.ticks, msg)
	case tea.QuitMsg:
		l.quit = true
	default:
		next, cmd := l.m.Update(msg)
		l.m = next.(Model)
		l.do(cmd)
	}
}

// press sends a sequence of keys.
func (l *loop) press(keys ...string) {
	for _, k := range keys {
		l.send(keyMsg(k))
	}
}

// until handles the messages until cond is true (5 s at most).
func (l *loop) until(cond func(Model) bool) {
	l.t.Helper()
	timeout := time.After(5 * time.Second)
	for !cond(l.m) {
		select {
		case msg := <-l.msgs:
			l.send(msg)
		case <-timeout:
			l.t.Fatalf("condition never met; screen:\n%s", l.screen())
		}
	}
}

// settle handles the messages already ready or arriving within 50 ms.
func (l *loop) settle() {
	for {
		select {
		case msg := <-l.msgs:
			l.send(msg)
		case <-time.After(50 * time.Millisecond):
			return
		}
	}
}

// screen returns the content of the screen.
func (l *loop) screen() string {
	return l.m.View().Content
}

// ticksOf returns the scheduled ticks of duration d.
func (l *loop) ticksOf(d time.Duration) []scheduledTick {
	var out []scheduledTick
	for _, tk := range l.ticks {
		if tk.d == d {
			out = append(out, tk)
		}
	}
	return out
}

// keyMsg builds the press of a key named as in keyMap ("enter", "shift+tab", "q"…).
func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+u":
		return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// errBoom is a fake error.
var errBoom = errors.New("database unavailable")
