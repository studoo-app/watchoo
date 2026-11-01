package sync_test

// Tests of the sync coordinator ("watchoo serve") with a fake Syncer that can be suspended and a
// controlled clock: sync at startup depending on the last sync, periodic sync, refusal of a
// concurrent sync, progress, interval change, clean shutdown.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	gosync "sync"
	"testing"
	"time"

	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// blockingSyncer is a fake Syncer: every sync sends its events, then waits for release (or the
// cancellation of its context).
type blockingSyncer struct {
	mu      gosync.Mutex
	calls   []int64 // 0 = SyncAll, otherwise the identifier of the repository
	started chan int64
	release chan struct{}
	err     error
}

func newBlockingSyncer() *blockingSyncer {
	return &blockingSyncer{started: make(chan int64, 16), release: make(chan struct{})}
}

func (s *blockingSyncer) run(ctx context.Context, repoID int64, opts wsync.SyncOptions) (wsync.Report, error) {
	s.mu.Lock()
	s.calls = append(s.calls, repoID)
	s.mu.Unlock()
	opts.Progress(wsync.Event{Kind: wsync.EventStarted, Total: 2})
	opts.Progress(wsync.Event{Kind: wsync.EventRepoStarted, Total: 2, Repo: wsync.RepoReport{DisplayName: "Alice Martin"}})
	s.started <- repoID
	select {
	case <-s.release:
	case <-ctx.Done():
		return wsync.Report{}, ctx.Err()
	}
	opts.Progress(wsync.Event{Kind: wsync.EventRepoFinished, Done: 1, Total: 2})
	return wsync.Report{Repos: []wsync.RepoReport{{RepoID: 1, Status: wsync.StatusSuccess, NewCommits: 3}}}, s.err
}

func (s *blockingSyncer) SyncAll(ctx context.Context, opts wsync.SyncOptions) (wsync.Report, error) {
	return s.run(ctx, 0, opts)
}

func (s *blockingSyncer) SyncRepo(ctx context.Context, repoID int64, opts wsync.SyncOptions) (wsync.Report, error) {
	return s.run(ctx, repoID, opts)
}

func (s *blockingSyncer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// fakeTimer replaces time.After: every call records the requested duration and returns a channel
// that the test triggers with fire.
type fakeTimer struct {
	mu    gosync.Mutex
	waits []time.Duration
	chans []chan time.Time
	asked chan struct{}
}

func newFakeTimer() *fakeTimer { return &fakeTimer{asked: make(chan struct{}, 64)} }

func (f *fakeTimer) after(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan time.Time, 1)
	f.waits = append(f.waits, d)
	f.chans = append(f.chans, ch)
	f.asked <- struct{}{}
	return ch
}

// waitAsk waits for a new timer request and returns its duration.
func (f *fakeTimer) waitAsk(t *testing.T) time.Duration {
	t.Helper()
	select {
	case <-f.asked:
	case <-time.After(5 * time.Second):
		t.Fatal("no timer requested")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.waits[len(f.waits)-1]
}

// fire triggers the last requested timer.
func (f *fakeTimer) fire() {
	f.mu.Lock()
	ch := f.chans[len(f.chans)-1]
	f.mu.Unlock()
	ch <- time.Now()
}

var coordNow = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

func newTestCoordinator(t *testing.T, s wsync.Syncer, interval time.Duration, last time.Time, timer *fakeTimer) *wsync.Coordinator {
	t.Helper()
	c, err := wsync.NewCoordinator(wsync.CoordinatorOptions{
		Syncer:   s,
		Interval: interval,
		LastSync: func(context.Context) (time.Time, error) { return last, nil },
		Now:      func() time.Time { return coordNow },
		After:    timer.after,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(5 * time.Second) })
	return c
}

// waitStarted waits for the start of a sync of the fake Syncer and returns its repository (0 = all).
func waitStarted(t *testing.T, s *blockingSyncer) int64 {
	t.Helper()
	select {
	case id := <-s.started:
		return id
	case <-time.After(5 * time.Second):
		t.Fatal("no sync started")
		return -1
	}
}

// waitIdle waits for the end of the running sync.
func waitIdle(t *testing.T, c *wsync.Coordinator) wsync.SyncState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := c.State(); !st.Running {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the sync did not finish")
	return wsync.SyncState{}
}

func TestNewCoordinatorValidates(t *testing.T) {
	if _, err := wsync.NewCoordinator(wsync.CoordinatorOptions{}); err == nil {
		t.Error("missing engine accepted")
	}
	for _, d := range []time.Duration{-time.Minute, 10 * time.Minute} {
		if _, err := wsync.NewCoordinator(wsync.CoordinatorOptions{Syncer: newBlockingSyncer(), Interval: d}); err == nil {
			t.Errorf("interval %s accepted", d)
		}
	}
}

func TestCoordinatorNotStarted(t *testing.T) {
	c := newTestCoordinator(t, newBlockingSyncer(), 0, time.Time{}, newFakeTimer())
	if err := c.SyncAll(); !errors.Is(err, wsync.ErrCoordinatorNotStarted) {
		t.Errorf("SyncAll before Start: %v", err)
	}
}

func TestCoordinatorFirstSyncAtStartWhenStale(t *testing.T) {
	s := newBlockingSyncer()
	timer := newFakeTimer()
	c := newTestCoordinator(t, s, 30*time.Minute, coordNow.Add(-2*time.Hour), timer)
	c.Start(t.Context())
	if d := timer.waitAsk(t); d != 0 {
		t.Errorf("first wait %s, want 0 (last sync too old)", d)
	}
	timer.fire()
	if id := waitStarted(t, s); id != 0 {
		t.Errorf("sync of repository %d, want all repositories", id)
	}
	st := c.State()
	if !st.Running || !st.Auto || st.Total != 2 || st.Current != "Alice Martin" {
		t.Errorf("state during the sync: %+v", st)
	}
	if want := coordNow.Add(30 * time.Minute); !st.NextAuto.Equal(want) {
		t.Errorf("NextAuto = %v, want %v", st.NextAuto, want)
	}
	// A manual sync during the automatic sync is refused.
	if err := c.SyncAll(); !errors.Is(err, wsync.ErrSyncRunning) {
		t.Errorf("SyncAll during a sync: %v", err)
	}
	if err := c.SyncRepo(1); !errors.Is(err, wsync.ErrSyncRunning) {
		t.Errorf("SyncRepo during a sync: %v", err)
	}
	close(s.release)
	st = waitIdle(t, c)
	if !st.HasReport || st.LastReport.NewCommits() != 3 || st.Done != 1 || st.LastError != "" ||
		!st.LastFinishedAt.Equal(coordNow) {
		t.Errorf("state after the sync: %+v", st)
	}
	if s.callCount() != 1 {
		t.Errorf("%d syncs, want 1", s.callCount())
	}
}

func TestCoordinatorWaitsForIntervalAfterRecentSync(t *testing.T) {
	s := newBlockingSyncer()
	timer := newFakeTimer()
	c := newTestCoordinator(t, s, 30*time.Minute, coordNow.Add(-10*time.Minute), timer)
	c.Start(t.Context())
	if d := timer.waitAsk(t); d != 20*time.Minute {
		t.Errorf("first wait %s, want 20m (one interval after the last sync)", d)
	}
	if s.callCount() != 0 {
		t.Error("sync started at startup despite a recent sync")
	}
}

func TestCoordinatorManualOnly(t *testing.T) {
	s := newBlockingSyncer()
	timer := newFakeTimer()
	c := newTestCoordinator(t, s, 0, time.Time{}, timer)
	c.Start(t.Context())
	if st := c.State(); !st.NextAuto.IsZero() || st.Interval != 0 {
		t.Errorf("manual only: %+v", st)
	}
	if err := c.SyncRepo(7); err != nil {
		t.Fatal(err)
	}
	if id := waitStarted(t, s); id != 7 {
		t.Errorf("repository %d, want 7", id)
	}
	if st := c.State(); st.Auto || st.RepoID != 7 || !st.Running {
		t.Errorf("state: %+v", st)
	}
	close(s.release)
	waitIdle(t, c)
	if err := c.SyncRepo(0); err == nil {
		t.Error("identifier 0 accepted")
	}
}

func TestCoordinatorRecordsGlobalError(t *testing.T) {
	s := newBlockingSyncer()
	s.err = errors.New("reading the GitHub token: gh not signed in")
	close(s.release)
	c := newTestCoordinator(t, s, 0, time.Time{}, newFakeTimer())
	c.Start(t.Context())
	if err := c.SyncAll(); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, s)
	if st := waitIdle(t, c); st.LastError != s.err.Error() {
		t.Errorf("LastError = %q", st.LastError)
	}
}

func TestCoordinatorSetInterval(t *testing.T) {
	s := newBlockingSyncer()
	timer := newFakeTimer()
	c := newTestCoordinator(t, s, 0, time.Time{}, timer)
	c.Start(t.Context())
	for _, d := range []time.Duration{-time.Minute, 14 * time.Minute} {
		if err := c.SetInterval(d); err == nil {
			t.Errorf("SetInterval(%s) accepted", d)
		}
	}
	if err := c.SetInterval(time.Hour); err != nil {
		t.Fatal(err)
	}
	if d := timer.waitAsk(t); d != time.Hour {
		t.Errorf("wait %s, want 1h", d)
	}
	if st := c.State(); st.Interval != time.Hour || !st.NextAuto.Equal(coordNow.Add(time.Hour)) {
		t.Errorf("state: %+v", st)
	}
	if c.Interval() != time.Hour {
		t.Errorf("Interval() = %s", c.Interval())
	}
	if err := c.SetInterval(0); err != nil {
		t.Fatal(err)
	}
	if st := c.State(); !st.NextAuto.IsZero() {
		t.Errorf("NextAuto = %v after SetInterval(0)", st.NextAuto)
	}
}

func TestCoordinatorCloseCancelsRunningSync(t *testing.T) {
	s := newBlockingSyncer()
	c := newTestCoordinator(t, s, 0, time.Time{}, newFakeTimer())
	c.Start(t.Context())
	if err := c.SyncAll(); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, s)
	if err := c.Close(5 * time.Second); err != nil {
		t.Fatalf("Close: %v", err)
	}
	st := c.State()
	if st.Running || st.LastError != "sync canceled" {
		t.Errorf("state after Close: %+v", st)
	}
	if err := c.SyncAll(); !errors.Is(err, wsync.ErrCoordinatorClosed) {
		t.Errorf("SyncAll after Close: %v", err)
	}
	if err := c.Close(time.Second); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestCoordinatorCloseTimeout(t *testing.T) {
	stuck := make(chan struct{})
	defer close(stuck)
	s := syncerFunc(func(ctx context.Context) (wsync.Report, error) {
		<-stuck // ignores the cancellation
		return wsync.Report{}, nil
	})
	c, err := wsync.NewCoordinator(wsync.CoordinatorOptions{Syncer: s, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	c.Start(t.Context())
	if err := c.SyncAll(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(20 * time.Millisecond); err == nil {
		t.Error("Close without error although the sync does not stop")
	}
}

// syncerFunc is a Syncer reduced to a function.
type syncerFunc func(ctx context.Context) (wsync.Report, error)

func (f syncerFunc) SyncAll(ctx context.Context, _ wsync.SyncOptions) (wsync.Report, error) {
	return f(ctx)
}

func (f syncerFunc) SyncRepo(ctx context.Context, _ int64, _ wsync.SyncOptions) (wsync.Report, error) {
	return f(ctx)
}

// TestCoordinatorWithEngine checks the coordinator with the real engine: complete manual sync,
// then clean shutdown.
func TestCoordinatorWithEngine(t *testing.T) {
	env := newEnv(t)
	env.addRepo(1, "alice")
	c, err := wsync.NewCoordinator(wsync.CoordinatorOptions{Syncer: env.eng,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	c.Start(t.Context())
	if err := c.SyncAll(); err != nil {
		t.Fatal(err)
	}
	st := waitIdle(t, c)
	if st.LastError != "" || len(st.LastReport.Repos) != 1 || st.LastReport.Repos[0].Status != wsync.StatusSuccess {
		t.Errorf("report: %+v", st)
	}
	if err := c.Close(5 * time.Second); err != nil {
		t.Fatal(err)
	}
}
