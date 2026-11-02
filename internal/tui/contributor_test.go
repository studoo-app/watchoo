package tui

// Contributor detail (0.5.0).

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// withContributorDetail adds to data the profile of Alice (contributor 1): two repositories, commits
// with and without known lines, activity this week and two weeks ago.
func withContributorDetail(data *fakeData) {
	data.profiles = map[int64]store.ContributorProfile{
		1: {Contributor: model.Contributor{ID: 1, Name: "Alice", ProviderLogin: "alice-gh", ProviderUserID: 10,
			ProfileURL: "https://github.com/alice-gh"}, Identities: 2, Commits: 5, Additions: 40, Deletions: 7, KnownCommits: 3, Repos: 2,
			FirstCommitAt: testNow.Add(-30 * 24 * time.Hour), LastCommitAt: testNow.Add(-2 * time.Hour)},
		2: {Contributor: model.Contributor{ID: 2, Name: "Bob"}, Identities: 1},
	}
	data.contribRepos = map[int64][]store.ContributorRepoStats{
		1: {{ContributorID: 1, RepoID: 1, RepoOwner: "promo", RepoName: "alice", RepoLabel: "Alice Martin", Commits: 4,
			Additions: 40, Deletions: 7, KnownCommits: 4, LastCommitAt: testNow.Add(-2 * time.Hour)},
			{ContributorID: 1, RepoID: 3, RepoOwner: "promo", RepoName: "chloe", Commits: 1,
				LastCommitAt: testNow.Add(-42 * 24 * time.Hour)}},
	}
	data.repoTimes = map[int64][]model.RepoCommitTime{
		1: {{RepoID: 1, CommittedAt: testNow.Add(-2 * time.Hour)}, {RepoID: 1, CommittedAt: testNow.Add(-3 * time.Hour)},
			{RepoID: 1, CommittedAt: testNow.Add(-14 * 24 * time.Hour)}},
	}
	data.commits[0].AuthorID = 1
	data.commits[0].Additions, data.commits[0].Deletions, data.commits[0].StatsKnown = 12, 3, true
}

func TestContributorDetail(t *testing.T) {
	data := newFakeData()
	withContributorDetail(data)
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	l.press("3", "enter")
	l.until(func(m Model) bool { return m.cdetail.open && !m.cdetail.loading })
	screen := l.screen()
	mustContain(t, screen, "Alice (@alice-gh)", "esc to go back", "@alice-gh",
		"2 linked addresses (not shown)", "5 commits in 2 repositories, +40/-7 (2 uncounted)", "30 d ago", "2 h ago",
		"12 weeks", "3 commits", "Repositories", "Alice Martin", "+40/-7", "promo/chloe",
		"Latest commits", "Ajoute la page d'accueil", "+12/-3", "+/- lines: counted in the local Git clone")
	mustNotContain(t, screen, "rouge", "https://github.com/alice-gh", "@example")
	// Activity curve of all their repositories: 2 commits this week, 1 two weeks ago.
	if line := lineWith(screen, "▁▁▁▁▁▁▁▁▁▅▁█"); !strings.Contains(line, "12 weeks") {
		t.Errorf("overall curve = %q", line)
	}
	// Line of the Dead repository: state of the repository, no activity over the period.
	if line := lineWith(screen, "promo/chloe"); !strings.Contains(line, "⚠ 42 d") || !strings.Contains(line, " - ") {
		t.Errorf("line of promo/chloe = %q", line)
	}
	data.mu.Lock()
	last := data.commitFilters[len(data.commitFilters)-1]
	data.mu.Unlock()
	if last.AuthorID != 1 || last.Limit != contribDetailCommits {
		t.Errorf("commit filter = %+v", last)
	}

	// Scrolling, filter refused, reload, then back to the list.
	l.press("j", "/")
	if l.m.filtering {
		t.Error("the filter opened in the detail")
	}
	l.press("r")
	l.until(func(m Model) bool { return !m.cdetail.loading })
	l.press("esc")
	if l.m.cdetail.open {
		t.Error("esc did not close the contributor detail")
	}
	mustContain(t, l.screen(), "CONTRIBUTOR", "LOGIN")

	// Contributor without an account or authored commit.
	l.press("j", "enter")
	l.until(func(m Model) bool { return m.cdetail.open && !m.cdetail.loading })
	mustContain(t, l.screen(), "Bob", "no known GitHub account", "No commit authored by them.")
	// Switching views closes the detail.
	l.press("1")
	if l.m.cdetail.open {
		t.Error("switching views did not close the detail")
	}
}

func TestContributorDetailError(t *testing.T) {
	data := newFakeData()
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	data.mu.Lock()
	data.err = errors.New("unreadable database")
	data.mu.Unlock()
	l.press("3", "enter")
	l.until(func(m Model) bool { return m.cdetail.open && !m.cdetail.loading })
	mustContain(t, l.screen(), "cannot read: unreadable database")
}

func TestContributorDetailWithRealStore(t *testing.T) {
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
	zoe := store.Identity{Name: "Zoé", Email: "zoe@example.test", Login: "zoe-gh", ProviderUserID: 7,
		ProfileURL: "https://github.com/zoe-gh"}
	sha := "cafe000000000000000000000000000000000001"
	if _, err := st.ApplyRepoSync(ctx, store.RepoSyncWrite{
		RepoID: repo.ID, RunID: run.ID,
		Metadata: store.RepositoryMetadata{Owner: "promo", Name: "zoe", DefaultBranch: "main",
			URL: "https://github.com/promo/zoe"},
		Branches: []store.BranchState{{Name: "main", HeadSHA: sha}},
		Commits: []store.CommitWrite{{SHA: sha, Message: "Premier rendu", Author: zoe, Committer: zoe,
			AuthoredAt: testNow.Add(-3 * time.Hour), CommittedAt: testNow.Add(-3 * time.Hour),
			Additions: 9, Deletions: 1, StatsKnown: true, Branches: []string{"main"}}},
		SyncedAt: testNow,
	}); err != nil {
		t.Fatal(err)
	}
	l := newLoop(t, testDeps(st, &fakeSyncer{}))
	l.press("3", "enter")
	l.until(func(m Model) bool { return m.cdetail.open && !m.cdetail.loading })
	screen := l.screen()
	mustContain(t, screen, "Zoé (@zoe-gh)", "1 linked address", "1 commit in 1 repository, +9/-1",
		"Zoé Leroy", "Premier rendu", "cafe000")
	mustNotContain(t, screen, "zoe@example.test", "https://github.com/zoe-gh")
	if line := lineWith(screen, "Zoé Leroy"); !strings.Contains(line, "▁▁▁▁▁▁▁▁▁▁▁█") {
		t.Errorf("repository line = %q", line)
	}
}

// Lines of a commit ("-" if unknown, "+0/-0" if zero) and totals ("(N uncounted)").
func TestLineFormats(t *testing.T) {
	for _, tc := range []struct {
		got, want string
	}{
		{commitLines(model.Commit{}), "-"},
		{commitLines(model.Commit{StatsKnown: true}), "+0/-0"},
		{commitLines(model.Commit{StatsKnown: true, Additions: 4, Deletions: 1}), "+4/-1"},
		{lineTotals(model.LineTotals{Commits: 3}), "-"},
		{lineTotals(model.LineTotals{Commits: 3, KnownCommits: 3, Additions: 5}), "+5/-0"},
		{lineTotals(model.LineTotals{Commits: 3, KnownCommits: 1, Additions: 5, Deletions: 2}), "+5/-2 (2 uncounted)"},
	} {
		if tc.got != tc.want {
			t.Errorf("%q, want %q", tc.got, tc.want)
		}
	}
}

func TestAITotalsFormat(t *testing.T) {
	got := aiTotals(model.AITotals{Scanned: 120, Signed: 3, Tools: []model.AIToolCount{{Tool: "claude", Commits: 2}, {Tool: "cursor", Commits: 1}}})
	if want := "3/120 (Claude 2, Cursor 1)"; got != want {
		t.Errorf("aiTotals = %q, want %q", got, want)
	}
}
