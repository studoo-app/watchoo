package store

// Tests du journal des synchronisations et du verrou par dépôt.

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
		t.Errorf("sync_run ouvert : %+v", run)
	}

	// 29 min plus tard : toujours verrouillé.
	_, err = s.BeginSyncRun(ctx, repoID, now.Add(29*time.Minute), 30*time.Minute)
	if !errors.Is(err, ErrLocked) {
		t.Errorf("erreur %v, attendu ErrLocked", err)
	}
	if got := count(t, s, "SELECT count(*) FROM sync_runs WHERE repo_id = ?", repoID); got != 1 {
		t.Errorf("%d sync_runs, attendu 1 (rien inséré quand verrouillé)", got)
	}
	// Le verrou est par dépôt.
	if _, err := s.BeginSyncRun(ctx, other, now, 30*time.Minute); err != nil {
		t.Errorf("autre dépôt : %v", err)
	}

	// Une fois clos, le dépôt se synchronise de nouveau.
	if err := s.FinishSyncRun(ctx, run.ID, now.Add(time.Minute), model.SyncError, 0, "échec factice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginSyncRun(ctx, repoID, now.Add(2*time.Minute), 30*time.Minute); err != nil {
		t.Errorf("après clôture : %v", err)
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
		t.Fatalf("running périmé : %v, attendu un nouveau sync_run", err)
	}
	var status, msg, finished string
	if err := s.DB().QueryRowContext(ctx, "SELECT status, error, finished_at FROM sync_runs WHERE id = ?", old.ID).
		Scan(&status, &msg, &finished); err != nil {
		t.Fatal(err)
	}
	if status != "error" || msg != InterruptedMessage || finished != FormatTime(now) {
		t.Errorf("ancien sync_run : %s, %q, %s", status, msg, finished)
	}
	last, err := s.LastSyncRun(ctx, repoID)
	if err != nil || last.ID != run.ID {
		t.Errorf("LastSyncRun = %+v, %v ; attendu le nouveau", last, err)
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
		t.Fatalf("erreur %v, attendu ErrLocked", err)
	}
	if got := count(t, s, "SELECT count(*) FROM sync_runs WHERE id = ? AND status = 'error'", stale); got != 1 {
		t.Error("le running périmé n'a pas été clos")
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
			t.Errorf("erreur inattendue : %v", err)
		}
	}
	if won != 1 {
		t.Errorf("%d synchros ont pris le verrou, attendu 1", won)
	}
}

func TestBeginSyncRunUnknownRepository(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.BeginSyncRun(t.Context(), 42, t0, 30*time.Minute); !errors.Is(err, ErrNotFound) {
		t.Errorf("erreur %v, attendu ErrNotFound", err)
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
	if err := s.FinishSyncRun(ctx, run.ID, t0.Add(time.Minute), model.SyncError, 3, "dépôt inaccessible"); err != nil {
		t.Fatal(err)
	}
	last, err := s.LastSyncRun(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if last.Status != model.SyncError || last.Error != "dépôt inaccessible" || last.NewCommits != 3 ||
		!last.FinishedAt.Equal(t0.Add(time.Minute)) {
		t.Errorf("sync_run clos : %+v", last)
	}
	repo, _ := s.GetRepository(ctx, repoID)
	if !repo.LastSyncedAt.IsZero() {
		t.Error("FinishSyncRun ne doit pas toucher last_synced_at")
	}

	// Déjà clos, inexistant : ErrNotFound. Statut invalide : refusé.
	if err := s.FinishSyncRun(ctx, run.ID, t0, model.SyncSuccess, 0, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("déjà clos : %v, attendu ErrNotFound", err)
	}
	if err := s.FinishSyncRun(ctx, 999, t0, model.SyncSuccess, 0, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("inexistant : %v, attendu ErrNotFound", err)
	}
	run2, _ := s.BeginSyncRun(ctx, repoID, t0.Add(time.Hour), 30*time.Minute)
	if err := s.FinishSyncRun(ctx, run2.ID, t0, model.SyncRunning, 0, ""); err == nil {
		t.Error("statut running accepté pour une clôture")
	}
}

func TestLastSyncRun(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	if _, err := s.LastSyncRun(ctx, repoID); !errors.Is(err, ErrNotFound) {
		t.Errorf("aucun sync_run : %v, attendu ErrNotFound", err)
	}
	mustExec(t, s, insertRun, repoID, FormatTime(t0.Add(time.Hour)), "success")
	mustExec(t, s, insertRun, repoID, FormatTime(t0), "error")
	tie := mustExec(t, s, insertRun, repoID, FormatTime(t0.Add(time.Hour)), "error")
	last, err := s.LastSyncRun(ctx, repoID)
	if err != nil || last.ID != tie {
		t.Errorf("LastSyncRun = %+v, %v ; attendu le plus récent, puis id le plus grand (%d)", last, err, tie)
	}
}
