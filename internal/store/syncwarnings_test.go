package store

// Warnings of a sync (1.3.0, migration 00010): recorded on a successful sync_run, read back by
// LastSyncRun and RepositorySummaries.

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestSetSyncRunWarnings(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	run, err := s.BeginSyncRun(ctx, repoID, t0, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	warnings := []model.SyncWarning{
		{Kind: model.WarnCloneUnavailable, Message: "local Git clone unavailable: access denied"},
		{Kind: model.WarnListsNotSynced, Message: "tags not synced: HTTP 502"},
	}
	// Still running: refused.
	if err := s.SetSyncRunWarnings(ctx, run.ID, warnings); !errors.Is(err, ErrNotFound) {
		t.Errorf("running sync_run: %v, want ErrNotFound", err)
	}
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{RepoID: repoID, RunID: run.ID,
		Metadata: RepositoryMetadata{Owner: "promo", Name: "alice"}, SyncedAt: t0.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	last, err := s.LastSyncRun(ctx, repoID)
	if err != nil || last.Warnings == nil || len(last.Warnings) != 0 {
		t.Errorf("no warning yet: %+v, %v", last.Warnings, err)
	}
	if err := s.SetSyncRunWarnings(ctx, run.ID, warnings); err != nil {
		t.Fatal(err)
	}
	if last, err = s.LastSyncRun(ctx, repoID); err != nil || !slices.Equal(last.Warnings, warnings) {
		t.Errorf("LastSyncRun warnings = %+v, %v", last.Warnings, err)
	}
	sums, err := s.RepositorySummaries(ctx)
	if err != nil || len(sums) != 1 || !slices.Equal(sums[0].LastRun.Warnings, warnings) {
		t.Fatalf("RepositorySummaries = %+v, %v", sums, err)
	}

	// The next sync_run starts without warnings.
	next, err := s.BeginSyncRun(ctx, repoID, t0.Add(time.Hour), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if last, err = s.LastSyncRun(ctx, repoID); err != nil || last.ID != next.ID || len(last.Warnings) != 0 {
		t.Errorf("new sync_run: %+v, %v", last, err)
	}
	if err := s.SetSyncRunWarnings(ctx, 999, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown sync_run: %v, want ErrNotFound", err)
	}
	if err := execErr(t, s, `UPDATE sync_runs SET warnings = '{}' WHERE id = ?`, run.ID); err == nil {
		t.Error("warnings that are not a JSON array accepted")
	}
}

func TestMigrationSyncRunWarnings(t *testing.T) {
	ctx := t.Context()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	p, err := goose.NewProvider(MigrationDialect, s.DB(), migrations.FS, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 9); err != nil {
		t.Fatalf("UpTo(9): %v", err)
	}
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	mustExec(t, s, `INSERT INTO sync_runs (repo_id, started_at, finished_at, status) VALUES (?, ?, ?, 'success')`,
		repo, testDate, testDate)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	last, err := s.LastSyncRun(ctx, repo)
	if err != nil || last.Warnings == nil || len(last.Warnings) != 0 {
		t.Errorf("sync_run from before the migration: %+v, %v", last, err)
	}

	// Rollback: column removed, rows kept.
	if _, err := p.DownTo(ctx, 9); err != nil {
		t.Fatalf("DownTo(9): %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM sync_runs"); got != 1 {
		t.Errorf("%d sync_runs after the rollback, want 1", got)
	}
	if err := execErr(t, s, "SELECT warnings FROM sync_runs"); err == nil {
		t.Error("warnings column still present")
	}
	if _, err := p.UpTo(ctx, 10); err != nil {
		t.Fatalf("UpTo(10) again: %v", err)
	}
}
