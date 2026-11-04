package store

// Migration 00007 (phase 7.1): commit_signals, and its use (write once, purge, cascades).

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestCommitSignals(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "alice", 501)
	main := []BranchState{{Name: "main", HeadSHA: sha(3)}}
	applySync(t, s, repoID, main, cw(1, alice, "main"), cw(2, alice, "main"), cw(3, alice, "main"))
	computed := t0.Add(2000 * time.Hour)

	n, err := s.WriteCommitSignals(ctx, repoID, []CommitSignalWrite{
		{SHA: sha(1), FilesKept: 3, FilesExcluded: 1, AdditionsKept: 120, DeletionsKept: 4,
			Tools: []string{"claude", "codex"}, AgentConfig: true},
		{SHA: sha(2), FilesKept: 1, AdditionsKept: 2, BotAuthor: true},
		{SHA: sha(9), FilesKept: 1}, // commit missing from the database: ignored
	}, computed)
	if err != nil || n != 2 {
		t.Fatalf("WriteCommitSignals = %d, %v; want 2", n, err)
	}
	// Written once: a second write changes nothing.
	n, err = s.WriteCommitSignals(ctx, repoID, []CommitSignalWrite{{SHA: sha(1), FilesKept: 99}}, computed)
	if err != nil || n != 0 {
		t.Errorf("second write = %d, %v; want 0", n, err)
	}

	got, err := s.CommitSignals(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SHA != sha(2) || got[1].SHA != sha(1) {
		t.Fatalf("CommitSignals = %+v", got)
	}
	first := got[1]
	if first.FilesKept != 3 || first.FilesExcluded != 1 || first.AdditionsKept != 120 || first.DeletionsKept != 4 ||
		!slices.Equal(first.Tools, []string{"claude", "codex"}) || !first.AgentConfig || first.BotAuthor ||
		first.AuthorID == 0 || !first.AuthoredAt.Equal(t0.Add(time.Hour)) || !first.ComputedAt.Equal(computed) {
		t.Errorf("signals of commit 1 = %+v", first)
	}
	if second := got[0]; second.Tools == nil || len(second.Tools) != 0 || !second.BotAuthor {
		t.Errorf("signals of commit 2 = %+v (want an empty non-nil tool list)", second)
	}
	if c, err := s.CountCommitSignals(ctx, repoID); err != nil || c != 2 {
		t.Errorf("CountCommitSignals = %d, %v", c, err)
	}

	// Purge by authored date: commit 1 (t0+1h) goes, commit 2 (t0+2h) stays.
	purged, err := s.PurgeCommitSignals(ctx, t0.Add(90*time.Minute))
	if err != nil || purged != 1 {
		t.Errorf("PurgeCommitSignals = %d, %v; want 1", purged, err)
	}
	if c, _ := s.CountCommitSignals(ctx, repoID); c != 1 {
		t.Errorf("%d rows after the purge, want 1", c)
	}

	// CHECK constraints and cascades.
	if err := execErr(t, s, `UPDATE commit_signals SET agent_config = 2`); err == nil {
		t.Error("agent_config = 2 accepted")
	}
	if err := execErr(t, s, `UPDATE commit_signals SET files_kept = -1`); err == nil {
		t.Error("files_kept = -1 accepted")
	}
	mustExec(t, s, `DELETE FROM commits WHERE repo_id = ? AND sha = ?`, repoID, sha(2))
	if c := count(t, s, "SELECT count(*) FROM commit_signals"); c != 0 {
		t.Errorf("%d rows after the commit was deleted, want 0", c)
	}
}

func TestMigrationCommitSignals(t *testing.T) {
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
	// Existing database at 00006 with a commit: the migration adds an empty table.
	if _, err := p.UpTo(ctx, 6); err != nil {
		t.Fatalf("UpTo(6): %v", err)
	}
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	mustExec(t, s, `INSERT INTO commits (repo_id, sha, authored_at, committed_at) VALUES (?, ?, ?, ?)`,
		repo, sha(1), testDate, testDate)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if c := count(t, s, "SELECT count(*) FROM commit_signals"); c != 0 {
		t.Errorf("%d rows after the migration, want 0", c)
	}
	for _, idx := range []string{"commit_signals_author_date", "commit_signals_date"} {
		if c := count(t, s, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, idx); c != 1 {
			t.Errorf("index %s missing", idx)
		}
	}
	// Rollback: table and indexes removed, commits kept; up again.
	if _, err := p.DownTo(ctx, 6); err != nil {
		t.Fatalf("DownTo(6): %v", err)
	}
	if c := count(t, s, `SELECT count(*) FROM sqlite_master WHERE name LIKE 'commit_signals%'`); c != 0 {
		t.Errorf("%d schema objects left after the rollback", c)
	}
	if c := count(t, s, "SELECT count(*) FROM commits"); c != 1 {
		t.Errorf("%d commits after the rollback, want 1", c)
	}
	if _, err := p.UpTo(ctx, 7); err != nil {
		t.Fatalf("UpTo(7) again: %v", err)
	}
}
