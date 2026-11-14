package web

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// gateSyncer is a fake engine whose every sync waits for release (or the cancellation).
type gateSyncer struct {
	started chan int64
	release chan struct{}
}

func newGateSyncer() *gateSyncer {
	return &gateSyncer{started: make(chan int64, 8), release: make(chan struct{})}
}

func (g *gateSyncer) run(ctx context.Context, id int64, opts wsync.SyncOptions) (wsync.Report, error) {
	opts.Progress(wsync.Event{Kind: wsync.EventStarted, Total: 2})
	opts.Progress(wsync.Event{Kind: wsync.EventRepoStarted, Total: 2, Repo: wsync.RepoReport{DisplayName: "<b>Alice</b>"}})
	g.started <- id
	select {
	case <-g.release:
	case <-ctx.Done():
		return wsync.Report{}, ctx.Err()
	}
	return wsync.Report{Repos: []wsync.RepoReport{{Status: wsync.StatusSuccess}, {Status: wsync.StatusError}}}, nil
}

func (g *gateSyncer) SyncAll(ctx context.Context, opts wsync.SyncOptions) (wsync.Report, error) {
	return g.run(ctx, 0, opts)
}

func (g *gateSyncer) SyncRepo(ctx context.Context, id int64, opts wsync.SyncOptions) (wsync.Report, error) {
	return g.run(ctx, id, opts)
}

func TestManualSyncAndConcurrentRefusal(t *testing.T) {
	gate := newGateSyncer()
	e := newTestEnv(t, withSyncer(gate))
	ids := e.seed()

	w := e.post("/sync", nil, true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="sync-status"`, "Sync started.")
	<-gate.started

	// During the sync: the fragment refreshes every 2 s and shows the progress.
	w = e.htmx("/fragments/sync?gen=0&run=0")
	assertContains(t, w, `hx-trigger="every 2s"`, "0/2", "&lt;b&gt;Alice&lt;/b&gt;", `run=1`)
	if w.Header().Get("HX-Trigger") != "" {
		t.Error("HX-Trigger sent during the sync")
	}

	// A second request (all repositories or a single one) is refused cleanly.
	w = e.post("/sync", nil, true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Sync already running", "badge-error")
	w = e.post(repoPath(ids.Bob)+"/sync", nil, true)
	assertContains(t, w, "Sync already running")
	w = e.post("/sync", nil, false)
	assertStatus(t, w, http.StatusConflict)
	assertContains(t, w, "Sync already running", "Sync not possible")

	close(gate.release)
	st := e.waitSyncIdle()
	// The end is reported to the fragment that saw it run.
	w = e.htmx("/fragments/sync?gen=" + strconv.FormatUint(st.Generation, 10) + "&run=1")
	if got := w.Header().Get("HX-Trigger"); got != SyncFinishedEvent {
		t.Errorf("HX-Trigger = %q, want %q", got, SyncFinishedEvent)
	}
	assertContains(t, w, `hx-trigger="every 30s"`, "1 repository in error")
	// A page loaded after the end does not receive the event.
	w = e.htmx("/fragments/sync?gen=" + strconv.FormatUint(st.Generation, 10) + "&run=0")
	if got := w.Header().Get("HX-Trigger"); got != "" {
		t.Errorf("HX-Trigger = %q for a sync already seen", got)
	}
	// An older page (automatic sync finished between two refreshes) receives it.
	w = e.htmx("/fragments/sync?gen=0&run=0")
	if got := w.Header().Get("HX-Trigger"); got != SyncFinishedEvent {
		t.Errorf("HX-Trigger = %q after a sync unknown to the fragment", got)
	}
}

func TestSyncRepoRoutes(t *testing.T) {
	gate := newGateSyncer()
	close(gate.release)
	e := newTestEnv(t, withSyncer(gate))
	ids := e.seed()

	w := e.post(repoPath(ids.Carol)+"/sync", nil, false)
	assertStatus(t, w, http.StatusSeeOther)
	if got := w.Header().Get("Location"); got != repoPath(ids.Carol) {
		t.Errorf("Location = %q", got)
	}
	if id := <-gate.started; id != ids.Carol {
		t.Errorf("synced repository %d, want %d", id, ids.Carol)
	}
	e.waitSyncIdle()

	w = e.post("/sync", nil, false)
	assertStatus(t, w, http.StatusSeeOther)
	if got := w.Header().Get("Location"); got != "/settings/sync" {
		t.Errorf("Location = %q", got)
	}
	<-gate.started
	e.waitSyncIdle()

	assertStatus(t, e.post("/repos/999/sync", nil, true), http.StatusNotFound)
}

// TestSyncWithRealEngine starts a manual sync with the real engine and the fake GitHub.
func TestSyncWithRealEngine(t *testing.T) {
	e := newTestEnv(t)
	id := e.addRepo(1, "alice", "Alice Martin")
	sha := e.fake.AppendCommits(1, "main", 2, e.now.Add(-48*time.Hour), time.Hour)
	if len(sha) != 2 {
		t.Fatalf("dummy commits: %v", sha)
	}
	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 4000})
	assertStatus(t, e.post(repoPath(id)+"/sync", url.Values{}, true), http.StatusOK)
	st := e.waitSyncIdle()
	if st.LastError != "" || len(st.LastReport.Repos) != 1 || st.LastReport.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("sync: %+v", st)
	}
	summary, err := e.store.RepositorySummary(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Commits != 2 {
		t.Errorf("%d commits in the database, want 2", summary.Commits)
	}
	w := e.htmx("/fragments/sync?gen=0&run=1")
	assertContains(t, w, "Last sync")
	assertNotContains(t, w, "quota low")
}

// finishedFirst starts the sync through the coordinator and waits for its end before returning: the
// sync is finished when startSync renders the fragment (engine that fails right away).
type finishedFirst struct {
	SyncController
	wait func()
}

func (f finishedFirst) SyncAll() error {
	if err := f.SyncController.SyncAll(); err != nil {
		return err
	}
	f.wait()
	return nil
}

func (f finishedFirst) SyncRepo(id int64) error {
	if err := f.SyncController.SyncRepo(id); err != nil {
		return err
	}
	f.wait()
	return nil
}

// Non-regression: a sync finished before the rendering of the response to the POST is reported right
// away (otherwise the next poll, which already carries the new generation with run=0, would never
// see it finish and the fragments of the page would not reload).
func TestSyncFinishedBeforeResponse(t *testing.T) {
	gate := newGateSyncer()
	close(gate.release)
	e := newTestEnv(t, withSyncer(gate))
	ids := e.seed()
	e.server.sync = finishedFirst{SyncController: e.coord, wait: func() { e.waitSyncIdle() }}

	for _, target := range []string{"/sync", repoPath(ids.Bob) + "/sync"} {
		w := e.post(target, nil, true)
		assertStatus(t, w, http.StatusOK)
		<-gate.started
		if got := w.Header().Get("HX-Trigger"); got != SyncFinishedEvent {
			t.Errorf("%s: HX-Trigger = %q, want %q", target, got, SyncFinishedEvent)
		}
		assertContains(t, w, "Sync started.", "run=0")
	}
}

// Non-regression: the sync buttons, outside the fragments reloaded at the end of a sync, are never
// disabled at rendering (otherwise they would stay so after the end of the sync).
func TestSyncButtonsNotDisabledDuringSync(t *testing.T) {
	gate := newGateSyncer()
	e := newTestEnv(t, withSyncer(gate))
	ids := e.seed()
	assertStatus(t, e.post("/sync", nil, true), http.StatusOK)
	<-gate.started
	for _, target := range []string{"/", "/settings/sync", repoPath(ids.Bob)} {
		w := e.get(target)
		assertStatus(t, w, http.StatusOK)
		assertNotContains(t, w, " disabled")
	}
	close(gate.release)
	e.waitSyncIdle()
}
