package store

// Tests of the sync log and of the per-repository lock.

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

func TestBeginSyncRunLock(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")
	now := t0

	run, err := s.BeginSyncRun(ctx, repoID, now, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == 0 || run.RepoID != repoID || run.Status != model.SyncRunning || !run.StartedAt.Equal(now) ||
		!run.FinishedAt.IsZero() || run.NewCommits != 0 {
		t.Errorf("open sync_run: %+v", run)
	}

	// 29 min later: still locked.
	_, err = s.BeginSyncRun(ctx, repoID, now.Add(29*time.Minute), 30*time.Minute)
	if !errors.Is(err, ErrLocked) {
		t.Errorf("error %v, want ErrLocked", err)
	}
	if got := count(t, s, "SELECT count(*) FROM sync_runs WHERE repo_id = ?", repoID); got != 1 {
		t.Errorf("%d sync_runs, want 1 (nothing inserted when locked)", got)
	}
	// The lock is per repository.
	if _, err := s.BeginSyncRun(ctx, other, now, 30*time.Minute); err != nil {
		t.Errorf("other repository: %v", err)
	}

	// Once closed, the repository syncs again.
	if err := s.FinishSyncRun(ctx, run.ID, now.Add(time.Minute), model.SyncError, 0, "fake failure"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginSyncRun(ctx, repoID, now.Add(2*time.Minute), 30*time.Minute); err != nil {
		t.Errorf("after closing: %v", err)
	}
}

func TestBeginSyncRunClosesStaleRun(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	old, err := s.BeginSyncRun(ctx, repoID, t0, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	now := t0.Add(31 * time.Minute)
	run, err := s.BeginSyncRun(ctx, repoID, now, 30*time.Minute)
	if err != nil {
		t.Fatalf("stale running: %v, want a new sync_run", err)
	}
	var status, msg, finished string
	if err := s.DB().QueryRowContext(ctx, "SELECT status, error, finished_at FROM sync_runs WHERE id = ?", old.ID).
		Scan(&status, &msg, &finished); err != nil {
		t.Fatal(err)
	}
	if status != "error" || msg != InterruptedMessage() || msg != "sync interrupted" || finished != FormatTime(now) {
		t.Errorf("old sync_run: %s, %q, %s", status, msg, finished)
	}
	last, err := s.LastSyncRun(ctx, repoID)
	if err != nil || last.ID != run.ID {
		t.Errorf("LastSyncRun = %+v, %v; want the new one", last, err)
	}
}

func TestBeginSyncRunClosesStaleEvenWhenLocked(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	stale := mustExec(t, s, insertRun, repoID, FormatTime(t0), "running")
	mustExec(t, s, insertRun, repoID, FormatTime(t0.Add(50*time.Minute)), "running")

	_, err := s.BeginSyncRun(ctx, repoID, t0.Add(60*time.Minute), 30*time.Minute)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("error %v, want ErrLocked", err)
	}
	if got := count(t, s, "SELECT count(*) FROM sync_runs WHERE id = ? AND status = 'error'", stale); got != 1 {
		t.Error("the stale running was not closed")
	}
}

func TestBeginSyncRunConcurrent(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")

	const workers = 8
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for range workers {
		wg.Go(func() {
			_, err := s.BeginSyncRun(ctx, repoID, t0, 30*time.Minute)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	won := 0
	for err := range results {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, ErrLocked):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if won != 1 {
		t.Errorf("%d syncs took the lock, want 1", won)
	}
}

func TestBeginSyncRunUnknownRepository(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.BeginSyncRun(t.Context(), 42, t0, 30*time.Minute); !errors.Is(err, ErrNotFound) {
		t.Errorf("error %v, want ErrNotFound", err)
	}
}

func TestFinishSyncRun(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	run, err := s.BeginSyncRun(ctx, repoID, t0, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSyncRun(ctx, run.ID, t0.Add(time.Minute), model.SyncError, 3, "repository inaccessible"); err != nil {
		t.Fatal(err)
	}
	last, err := s.LastSyncRun(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if last.Status != model.SyncError || last.Error != "repository inaccessible" || last.NewCommits != 3 ||
		!last.FinishedAt.Equal(t0.Add(time.Minute)) {
		t.Errorf("closed sync_run: %+v", last)
	}
	repo, _ := s.GetRepository(ctx, repoID)
	if !repo.LastSyncedAt.IsZero() {
		t.Error("FinishSyncRun must not touch last_synced_at")
	}

	// Already closed, nonexistent: ErrNotFound. Invalid status: refused.
	if err := s.FinishSyncRun(ctx, run.ID, t0, model.SyncSuccess, 0, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("already closed: %v, want ErrNotFound", err)
	}
	if err := s.FinishSyncRun(ctx, 999, t0, model.SyncSuccess, 0, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("nonexistent: %v, want ErrNotFound", err)
	}
	run2, _ := s.BeginSyncRun(ctx, repoID, t0.Add(time.Hour), 30*time.Minute)
	if err := s.FinishSyncRun(ctx, run2.ID, t0, model.SyncRunning, 0, ""); err == nil {
		t.Error("running status accepted for a closing")
	}
}

func TestLastSyncRun(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	if _, err := s.LastSyncRun(ctx, repoID); !errors.Is(err, ErrNotFound) {
		t.Errorf("no sync_run: %v, want ErrNotFound", err)
	}
	mustExec(t, s, insertRun, repoID, FormatTime(t0.Add(time.Hour)), "success")
	mustExec(t, s, insertRun, repoID, FormatTime(t0), "error")
	tie := mustExec(t, s, insertRun, repoID, FormatTime(t0.Add(time.Hour)), "error")
	last, err := s.LastSyncRun(ctx, repoID)
	if err != nil || last.ID != tie {
		t.Errorf("LastSyncRun = %+v, %v; want the most recent, then the largest id (%d)", last, err, tie)
	}
}
