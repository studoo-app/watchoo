package store

// Migration 00016 (phase 8.1): commits.parent_count and repositories.conventions_since, and their use
// (writing, backfill pagination, per-repository date).

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestMigrationConventions(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 15); err != nil {
		t.Fatalf("UpTo(15): %v", err)
	}
	// Database at schema 00015: a repository and two commits, without parent count.
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	for i := 1; i <= 2; i++ {
		mustExec(t, s, `INSERT INTO commits (repo_id, sha, authored_at, committed_at) VALUES (?, ?, ?, ?)`,
			repo, sha(i), testDate, testDate)
	}

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM commits WHERE parent_count IS NULL"); got != 2 {
		t.Errorf("%d commits with an unknown parent count, want 2", got)
	}
	unknown, err := s.UnknownParentSHAs(ctx, repo, "", 10)
	if err != nil || !slices.Equal(unknown, []string{sha(1), sha(2)}) {
		t.Errorf("UnknownParentSHAs = %v, %v", unknown, err)
	}
	if got := count(t, s, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'commits_parent_count_unknown'`); got != 1 {
		t.Error("index commits_parent_count_unknown missing")
	}
	if err := execErr(t, s, `UPDATE commits SET parent_count = -1`); err == nil {
		t.Error("parent_count = -1 accepted")
	}
	got, err := s.GetRepository(ctx, repo)
	if err != nil || got.ConventionsSince != "" {
		t.Errorf("conventions_since after the migration: %+v, %v", got, err)
	}
	for _, bad := range []string{"2026-09-01", "2026-09-01T10:00:00Z", "demain"} {
		if err := execErr(t, s, `UPDATE repositories SET conventions_since = ?`, bad); err == nil {
			t.Errorf("conventions_since %q accepted", bad)
		}
	}
	mustExec(t, s, `UPDATE commits SET parent_count = 1 WHERE sha = ?`, sha(1))
	mustExec(t, s, `UPDATE repositories SET conventions_since = '2026-09-01T00:00:00Z'`)

	// Rollback: columns and index removed, rows kept.
	if _, err := p.DownTo(ctx, 15); err != nil {
		t.Fatalf("DownTo(15): %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM commits"); got != 2 {
		t.Errorf("%d commits after the rollback, want 2", got)
	}
	if err := execErr(t, s, "SELECT parent_count FROM commits"); err == nil {
		t.Error("parent_count column still present")
	}
	if err := execErr(t, s, "SELECT conventions_since FROM repositories"); err == nil {
		t.Error("conventions_since column still present")
	}
	if got := count(t, s, `SELECT count(*) FROM sqlite_master WHERE name = 'commits_parent_count_unknown'`); got != 0 {
		t.Error("index still present")
	}
	if _, err := p.UpTo(ctx, 16); err != nil {
		t.Fatalf("UpTo(16) again: %v", err)
	}
}

func TestApplyRepoSyncParentCounts(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "alice", 501)
	main := []BranchState{{Name: "main", HeadSHA: sha(3)}}

	// New commits: root (0), merge (2), unknown (nil).
	zero, two := 0, 2
	c1, c2, c3 := cw(1, alice, "main"), cw(2, alice, "main"), cw(3, alice, "main")
	c1.ParentCount, c2.ParentCount = &zero, &two
	applySync(t, s, repoID, main, c1, c2, c3)
	unknown, err := s.UnknownParentSHAs(ctx, repoID, "", 10)
	if err != nil || !slices.Equal(unknown, []string{sha(3)}) {
		t.Fatalf("UnknownParentSHAs = %v, %v", unknown, err)
	}
	views, err := s.ListRecentCommits(ctx, CommitFilter{RepoID: repoID})
	if err != nil {
		t.Fatal(err)
	}
	parents := map[string]*int{}
	for _, v := range views {
		parents[v.SHA] = v.ParentCount
	}
	if p := parents[sha(1)]; p == nil || *p != 0 {
		t.Errorf("root commit: %v", p)
	}
	if p := parents[sha(2)]; p == nil || *p != 2 {
		t.Errorf("merge commit: %v", p)
	}
	if p := parents[sha(3)]; p != nil {
		t.Errorf("unknown commit: %v", *p)
	}

	// Backfill of a commit already in the database (and of an unknown SHA, ignored); a known count is
	// never rewritten.
	now := t0.Add(2000 * time.Hour)
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{
		RepoID: repoID, RunID: beginRun(t, s, repoID, now), Metadata: meta("promo", "alice"), Branches: main,
		ParentCounts: []CommitParentsWrite{{SHA: sha(3), Parents: 1}, {SHA: sha(99), Parents: 1}, {SHA: sha(2), Parents: 1}},
		SyncedAt:     now,
	}); err != nil {
		t.Fatal(err)
	}
	if unknown, _ := s.UnknownParentSHAs(ctx, repoID, "", 10); len(unknown) != 0 {
		t.Errorf("still unknown: %v", unknown)
	}
	if got := count(t, s, "SELECT parent_count FROM commits WHERE sha = ?", sha(2)); got != 2 {
		t.Errorf("known parent count rewritten: %d", got)
	}

	// Invalid counts: refused before any write.
	minus := -1
	bad := cw(4, alice, "main")
	bad.ParentCount = &minus
	for i, w := range []RepoSyncWrite{
		{ParentCounts: []CommitParentsWrite{{SHA: ""}}},
		{ParentCounts: []CommitParentsWrite{{SHA: sha(1), Parents: -1}}},
		{Commits: []CommitWrite{bad}},
	} {
		at := now.Add(time.Duration(i+1) * time.Hour)
		w.RepoID, w.RunID, w.Metadata, w.Branches, w.SyncedAt = repoID, beginRun(t, s, repoID, at), meta("promo", "alice"), main, at
		if _, err := s.ApplyRepoSync(ctx, w); err == nil {
			t.Errorf("write %d accepted", i)
		}
		mustExec(t, s, `UPDATE sync_runs SET status = 'error', finished_at = ? WHERE status = 'running'`, testDate)
	}
	if _, err := s.UnknownParentSHAs(ctx, repoID, "", 0); err == nil {
		t.Error("limit 0 accepted")
	}
}

func TestUnknownParentPaging(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "alice", 501)
	var commits []CommitWrite
	for i := 1; i <= 5; i++ {
		c := cw(i, alice, "main")
		if i <= 2 {
			one := 1
			c.ParentCount = &one
		}
		commits = append(commits, c)
	}
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(5)}}, commits...)
	var all []string
	after := ""
	for {
		page, err := s.UnknownParentSHAs(ctx, repoID, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page...)
		if len(page) < 2 {
			break
		}
		after = page[len(page)-1]
	}
	if !slices.Equal(all, []string{sha(3), sha(4), sha(5)}) {
		t.Errorf("pages = %v", all)
	}
}

func TestRepositoryConventionsSince(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	// Creation with an own date: stored at midnight UTC, read back as a calendar date.
	r := testRepo(1, "promo", "alice")
	r.ConventionsSince = "2026-09-01"
	created, err := s.CreateRepository(ctx, r)
	if err != nil || created.ConventionsSince != "2026-09-01" {
		t.Fatalf("created: %+v, %v", created, err)
	}
	if got := count(t, s, "SELECT count(*) FROM repositories WHERE conventions_since = '2026-09-01T00:00:00Z'"); got != 1 {
		t.Error("conventions_since not stored at midnight UTC")
	}
	bad := testRepo(2, "promo", "bob")
	bad.ConventionsSince = "01/09/2026"
	if _, err := s.CreateRepository(ctx, bad); err == nil {
		t.Error("invalid date accepted at creation")
	}

	// Change, then back to the workspace date (NULL).
	repo, err := s.SetRepositoryConventionsSince(ctx, created.ID, "2027-01-15")
	if err != nil || repo.ConventionsSince != "2027-01-15" {
		t.Errorf("set: %+v, %v", repo, err)
	}
	repo, err = s.SetRepositoryConventionsSince(ctx, created.ID, "")
	if err != nil || repo.ConventionsSince != "" {
		t.Errorf("reset: %+v, %v", repo, err)
	}
	if got := count(t, s, "SELECT count(*) FROM repositories WHERE conventions_since IS NULL"); got != 1 {
		t.Error("conventions_since not NULL after the reset")
	}
	if _, err := s.SetRepositoryConventionsSince(ctx, created.ID, "2026-02-30"); err == nil {
		t.Error("invalid date accepted")
	}
	if _, err := s.SetRepositoryConventionsSince(ctx, 999, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("nonexistent repository: %v, want ErrNotFound", err)
	}

	// A malformed value in the database (bypassing the CHECK is impossible: refused by SQLite).
	if err := execErr(t, s, "UPDATE repositories SET conventions_since = 'x'"); err == nil {
		t.Error("malformed conventions_since accepted by the database")
	}
}
