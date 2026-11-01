package store

// Tests of the read views: latest commits, ranking of contributors, summary per repository.

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// viewFixture fills two repositories:
//   - alice (label "Alice Martin"): commits 1 and 2 on main (Alice, account 10), 3 on main and dev
//     (Bob, without account), 4 on dev only (Bob), 5 without author on main;
//   - bob: commit 6 on main (Alice), known statistics.
func viewFixture(t *testing.T, s *Store) (aliceRepo, bobRepo int64) {
	t.Helper()
	ctx := t.Context()
	aliceRepo = mustCreateRepo(t, s, 1, "promo", "alice")
	if _, err := s.UpdateRepositorySettings(ctx, aliceRepo, "Alice Martin", nil); err != nil {
		t.Fatal(err)
	}
	bobRepo = mustCreateRepo(t, s, 2, "promo", "bob")

	alice := person("Alice", "Alice@Example.test", "alice-gh", 10)
	bob := person("Bob", "bob@example.test", "", 0)
	anon := person("", "", "", 0)
	applySync(t, s, aliceRepo,
		[]BranchState{{Name: "main", HeadSHA: sha(5)}, {Name: "dev", HeadSHA: sha(4)}},
		cw(1, alice, "main"), cw(2, alice, "main"), cw(3, bob, "main", "dev"), cw(4, bob, "dev"), cw(5, anon, "main"))

	now := t0.Add(1000 * time.Hour)
	c6 := cw(6, alice, "main")
	c6.Additions, c6.Deletions = 7, 3
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{
		RepoID: bobRepo, RunID: beginRun(t, s, bobRepo, now), Metadata: meta("promo", "bob"),
		Branches: []BranchState{{Name: "main", HeadSHA: sha(6)}}, Commits: []CommitWrite{c6}, SyncedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return aliceRepo, bobRepo
}

// shasOf returns the SHAs of a list of commits, in order.
func shasOf(views []CommitView) []string {
	out := make([]string, 0, len(views))
	for _, v := range views {
		out = append(out, v.SHA)
	}
	return out
}

func TestListRecentCommitsFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, bobRepo := viewFixture(t, s)

	tests := []struct {
		name   string
		filter CommitFilter
		want   []string
	}{
		{"all", CommitFilter{}, []string{sha(6), sha(5), sha(4), sha(3), sha(2), sha(1)}},
		{"limit", CommitFilter{Limit: 2}, []string{sha(6), sha(5)}},
		{"repository", CommitFilter{RepoID: bobRepo}, []string{sha(6)}},
		{"branch", CommitFilter{RepoID: aliceRepo, Branch: "dev"}, []string{sha(4), sha(3)}},
		{"main branch", CommitFilter{Branch: "main"}, []string{sha(6), sha(5), sha(3), sha(2), sha(1)}},
		{"unknown branch", CommitFilter{Branch: "nope"}, []string{}},
		{"period", CommitFilter{Since: t0.Add(4 * time.Hour)}, []string{sha(6), sha(5), sha(4)}},
		{"author by login, case ignored", CommitFilter{Author: "ALICE-GH"}, []string{sha(6), sha(2), sha(1)}},
		{"author by name", CommitFilter{Author: "bob"}, []string{sha(4), sha(3)}},
		{"author by e-mail", CommitFilter{Author: "alice@EXAMPLE.test", RepoID: aliceRepo}, []string{sha(2), sha(1)}},
		{"unknown author", CommitFilter{Author: "zoé"}, []string{}},
		{"combined", CommitFilter{Author: "Alice", Branch: "main", Since: t0.Add(2 * time.Hour), Limit: 1}, []string{sha(6)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ListRecentCommits(ctx, tt.filter)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("nil list, want a non-nil list")
			}
			if !slices.Equal(shasOf(got), tt.want) {
				t.Errorf("SHA %v, want %v", shasOf(got), tt.want)
			}
		})
	}
}

func TestListRecentCommitsView(t *testing.T) {
	s := newTestStore(t)
	aliceRepo, _ := viewFixture(t, s)
	got, err := s.ListRecentCommits(t.Context(), CommitFilter{RepoID: aliceRepo})
	if err != nil {
		t.Fatal(err)
	}
	byS := map[string]CommitView{}
	for _, v := range got {
		byS[v.SHA] = v
	}
	c1 := byS[sha(1)]
	if c1.RepoOwner != "promo" || c1.RepoName != "alice" || c1.RepoDisplayName() != "Alice Martin" ||
		c1.AuthorName != "Alice" || c1.AuthorLogin != "alice-gh" || c1.Message != "message 1" ||
		!c1.CommittedAt.Equal(t0.Add(time.Hour)) || c1.AuthorID == 0 || c1.AuthorID != c1.CommitterID {
		t.Errorf("view of commit 1: %+v", c1)
	}
	c5 := byS[sha(5)]
	if c5.AuthorID != 0 || c5.AuthorDisplayName() != "unknown" {
		t.Errorf("commit without author: %+v", c5)
	}
}

func TestContributorRanking(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, _ := viewFixture(t, s)

	got, err := s.ContributorRanking(ctx, ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d contributors ranked, want 2 (commit without author ignored): %+v", len(got), got)
	}
	alice, bob := got[0], got[1]
	if alice.Name != "Alice" || alice.ProviderLogin != "alice-gh" || alice.Provider != model.ProviderGitHub ||
		alice.ProviderUserID != 10 || alice.Commits != 3 || alice.Repos != 2 || alice.Additions != 7 ||
		alice.Deletions != 3 || !alice.LastCommitAt.Equal(t0.Add(6*time.Hour)) || alice.Emails != nil {
		t.Errorf("Alice: %+v", alice)
	}
	if bob.Name != "Bob" || bob.Commits != 2 || bob.Repos != 1 || bob.Provider != "" ||
		!bob.LastCommitAt.Equal(t0.Add(4*time.Hour)) {
		t.Errorf("Bob: %+v", bob)
	}

	// Equal commits: the most recent last commit comes first.
	got, err = s.ContributorRanking(ctx, ContributorFilter{RepoID: aliceRepo})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Bob" || got[0].Commits != 2 || got[1].Name != "Alice" || got[1].Commits != 2 {
		t.Errorf("ranking of the alice repository: %+v", got)
	}

	got, err = s.ContributorRanking(ctx, ContributorFilter{Since: t0.Add(5 * time.Hour)})
	if err != nil || len(got) != 1 || got[0].Name != "Alice" || got[0].Commits != 1 {
		t.Errorf("since t0+5h: %+v, %v", got, err)
	}
	got, err = s.ContributorRanking(ctx, ContributorFilter{Limit: 1})
	if err != nil || len(got) != 1 {
		t.Errorf("limit 1: %+v, %v", got, err)
	}
	got, err = s.ContributorRanking(ctx, ContributorFilter{Since: t0.Add(9999 * time.Hour)})
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("no commit: %+v, %v; want a non-nil empty list", got, err)
	}
}

func TestRepositorySummaries(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, bobRepo := viewFixture(t, s)
	never := mustCreateRepo(t, s, 3, "aaa", "jamais")

	// A failed sync after the successful sync of bob.
	run := beginRun(t, s, bobRepo, t0.Add(2000*time.Hour))
	if err := s.FinishSyncRun(ctx, run, t0.Add(2000*time.Hour), model.SyncError, 0, "fake failure"); err != nil {
		t.Fatal(err)
	}

	got, err := s.RepositorySummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != never || got[1].ID != aliceRepo || got[2].ID != bobRepo {
		t.Fatalf("summaries %+v, want sorted by owner then name", got)
	}

	n := got[0]
	if n.Commits != 0 || n.TrackedBranches != 0 || n.LastCommit.SHA != "" || n.LastRun.ID != 0 {
		t.Errorf("never synced repository: %+v", n)
	}

	a := got[1]
	if a.Label != "Alice Martin" || a.Commits != 5 || a.TrackedBranches != 2 {
		t.Errorf("alice summary: %+v", a)
	}
	// Most recent commit on the tracked branches: 5 (without author).
	if a.LastCommit.SHA != sha(5) || a.LastCommit.RepoDisplayName() != "Alice Martin" || a.LastCommit.AuthorDisplayName() != "unknown" {
		t.Errorf("last commit of alice: %+v", a.LastCommit)
	}
	if a.LastRun.Status != model.SyncSuccess || a.LastRun.NewCommits != 5 {
		t.Errorf("last sync of alice: %+v", a.LastRun)
	}

	b := got[2]
	if b.LastCommit.SHA != sha(6) || b.LastCommit.AuthorName != "Alice" || b.LastCommit.AuthorLogin != "alice-gh" {
		t.Errorf("last commit of bob: %+v", b.LastCommit)
	}
	if b.LastRun.ID != run || b.LastRun.Status != model.SyncError || b.LastRun.Error != "fake failure" {
		t.Errorf("last sync of bob: %+v", b.LastRun)
	}

	// The last commit only counts the tracked branches: without dev, commit 4 is no longer a candidate
	// (here 5 stays the most recent); without main, only dev is left.
	applySync(t, s, aliceRepo, []BranchState{{Name: "dev", HeadSHA: sha(4)}})
	one, err := s.RepositorySummary(ctx, aliceRepo)
	if err != nil {
		t.Fatal(err)
	}
	if one.TrackedBranches != 1 || one.LastCommit.SHA != sha(4) || !one.LastCommitAt.Equal(t0.Add(4*time.Hour)) {
		t.Errorf("summary of alice tracking dev only: %+v", one)
	}
	if _, err := s.RepositorySummary(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("nonexistent repository: %v, want ErrNotFound", err)
	}
}

func TestRepositorySummariesEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.RepositorySummaries(t.Context())
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("RepositorySummaries = %v, %v; want a non-nil empty list", got, err)
	}
}

func TestRepositorySummaryTieBreakBySHA(t *testing.T) {
	s := newTestStore(t)
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	who := person("Nina", "nina@example.test", "", 0)
	a, b := cw(1, who, "main"), cw(2, who, "main")
	b.CommittedAt = a.CommittedAt
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(2)}}, b, a)
	got, err := s.RepositorySummary(t.Context(), repoID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastCommit.SHA != sha(1) {
		t.Errorf("last commit %s, want the smallest SHA on a tie", got.LastCommit.SHA)
	}
}
