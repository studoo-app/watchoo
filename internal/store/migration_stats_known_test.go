package store

// Migration 00006 (0.5.0): commits.stats_known, and its use (writing, pagination, totals).

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestMigrationStatsKnown(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 5); err != nil {
		t.Fatalf("UpTo(5): %v", err)
	}
	// Database at schema 00005: one commit read through the API (non-zero lines), two at 0/0 (never
	// read, or really empty: indistinguishable).
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	for i, st := range [][2]int{{4, 1}, {0, 0}, {0, 3}, {0, 0}} {
		mustExec(t, s, `INSERT INTO commits (repo_id, sha, authored_at, committed_at, additions, deletions)
			VALUES (?, ?, ?, ?, ?, ?)`, repo, sha(i+1), testDate, testDate, st[0], st[1])
	}

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM commits WHERE stats_known = 1"); got != 2 {
		t.Errorf("%d known commits after the migration, want 2 (non-zero lines)", got)
	}
	unknown, err := s.UnknownStatsSHAs(ctx, repo, "", 10)
	if err != nil || !slices.Equal(unknown, []string{sha(2), sha(4)}) {
		t.Errorf("UnknownStatsSHAs = %v, %v", unknown, err)
	}
	if got := count(t, s, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'commits_stats_unknown'`); got != 1 {
		t.Error("index commits_stats_unknown missing")
	}
	if err := execErr(t, s, `UPDATE commits SET stats_known = 2`); err == nil {
		t.Error("stats_known = 2 accepted")
	}

	// Rollback: column and index removed, rows and values kept.
	if _, err := p.DownTo(ctx, 5); err != nil {
		t.Fatalf("DownTo(5): %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM commits WHERE additions = 4 AND deletions = 1"); got != 1 {
		t.Error("values lost on rollback")
	}
	if got := count(t, s, "SELECT count(*) FROM commits"); got != 4 {
		t.Errorf("%d commits after the rollback, want 4", got)
	}
	if err := execErr(t, s, "SELECT stats_known FROM commits"); err == nil {
		t.Error("stats_known column still present")
	}
	if got := count(t, s, `SELECT count(*) FROM sqlite_master WHERE name = 'commits_stats_unknown'`); got != 0 {
		t.Error("index still present")
	}
	if _, err := p.UpTo(ctx, 6); err != nil {
		t.Fatalf("UpTo(6) again: %v", err)
	}
}

func TestApplyRepoSyncStats(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "alice", 501)
	main := []BranchState{{Name: "main", HeadSHA: sha(3)}}

	// New known commit (even at 0/0), new unknown commit.
	c1, c2, c3 := cw(1, alice, "main"), cw(2, alice, "main"), cw(3, alice, "main")
	c1.Additions, c1.Deletions, c1.StatsKnown = 5, 1, true
	c2.StatsKnown = true // empty commit: 0/0 known
	applySync(t, s, repoID, main, c1, c2, c3)
	unknown, err := s.UnknownStatsSHAs(ctx, repoID, "", 10)
	if err != nil || !slices.Equal(unknown, []string{sha(3)}) {
		t.Fatalf("UnknownStatsSHAs = %v, %v", unknown, err)
	}
	views, err := s.ListRecentCommits(ctx, CommitFilter{RepoID: repoID})
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, v := range views {
		known[v.SHA] = v.StatsKnown
	}
	if !known[sha(1)] || !known[sha(2)] || known[sha(3)] {
		t.Errorf("stats_known read: %v", known)
	}

	// Statistics of a commit already in the database (and of an unknown SHA, ignored), in the same transaction.
	now := t0.Add(2000 * time.Hour)
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{
		RepoID: repoID, RunID: beginRun(t, s, repoID, now), Metadata: meta("promo", "alice"), Branches: main,
		Stats:    []CommitStatsWrite{{SHA: sha(3), Additions: 7, Deletions: 2}, {SHA: sha(99), Additions: 1}},
		SyncedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if unknown, _ := s.UnknownStatsSHAs(ctx, repoID, "", 10); len(unknown) != 0 {
		t.Errorf("still unknown: %v", unknown)
	}
	sum, err := s.RepositorySummary(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if want := (model.LineTotals{Commits: 3, KnownCommits: 3, Additions: 12, Deletions: 3}); sum.Lines != want {
		t.Errorf("totals of the repository = %+v, want %+v", sum.Lines, want)
	}

	// Invalid statistics: refused before any write.
	for _, bad := range []CommitStatsWrite{{SHA: ""}, {SHA: sha(1), Additions: -1}} {
		if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{
			RepoID: repoID, RunID: beginRun(t, s, repoID, now.Add(time.Hour)), Metadata: meta("promo", "alice"),
			Branches: main, Stats: []CommitStatsWrite{bad}, SyncedAt: now.Add(time.Hour),
		}); err == nil {
			t.Errorf("statistics %+v accepted", bad)
		}
		mustExec(t, s, `UPDATE sync_runs SET status = 'error', finished_at = ? WHERE status = 'running'`, testDate)
	}
	if _, err := s.UnknownStatsSHAs(ctx, repoID, "", 0); err == nil {
		t.Error("limit 0 accepted")
	}
}

// Pagination of the unknown commits and partial totals (contributors, repository).
func TestUnknownStatsPagingAndTotals(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "alice", 501)
	var commits []CommitWrite
	for i := 1; i <= 5; i++ {
		c := cw(i, alice, "main")
		if i <= 2 {
			c.Additions, c.StatsKnown = 10, true
		}
		commits = append(commits, c)
	}
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(5)}}, commits...)

	var all []string
	after := ""
	for {
		page, err := s.UnknownStatsSHAs(ctx, repoID, after, 2)
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

	ranking, err := s.ContributorRanking(ctx, ContributorFilter{})
	if err != nil || len(ranking) != 1 {
		t.Fatalf("ranking: %v, %v", ranking, err)
	}
	if l := ranking[0].Lines(); !l.Known() || l.Uncounted() != 3 || l.Additions != 20 {
		t.Errorf("totals of the contributor = %+v", l)
	}
	profile, err := s.ContributorProfile(ctx, ranking[0].ID)
	if err != nil || profile.Lines().Uncounted() != 3 {
		t.Errorf("record: %+v, %v", profile.Lines(), err)
	}
	repos, err := s.ContributorRepos(ctx, ranking[0].ID)
	if err != nil || len(repos) != 1 || repos[0].Lines().KnownCommits != 2 {
		t.Errorf("by repository: %+v, %v", repos, err)
	}
	sums, err := s.RepositorySummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if total := SumLines(sums); total.Commits != 5 || total.KnownCommits != 2 || total.Additions != 20 {
		t.Errorf("totals of the workspace = %+v", total)
	}
	if (model.LineTotals{Commits: 2}).Known() {
		t.Error("totals without a known commit: Known true")
	}
}
