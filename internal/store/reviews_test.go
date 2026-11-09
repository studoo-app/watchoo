package store

// Migration 00021 (phase 8.6): review counters of the pull requests (PR4); selection of the pull
// requests whose reviews are to be read, accounts of the commit authors and writing of the counters.

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestMigrationPullRequestReviews(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 20); err != nil {
		t.Fatalf("UpTo(20): %v", err)
	}
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	mustExec(t, s, insertPR, repo, 1, "closed", testDate, testDate, testDate)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if got := count(t, s, `SELECT count(*) FROM pull_requests WHERE reviews_by_others IS NULL
		AND approvals_by_others IS NULL AND approvals_by_contributors IS NULL AND reviews_checked_at IS NULL`); got != 1 {
		t.Errorf("%d pull requests with unknown review counters, want 1", got)
	}
	views, err := s.ListPullRequests(ctx, PullRequestFilter{RepoID: repo})
	if err != nil || len(views) != 1 || views[0].ReviewsByOthers != nil || views[0].ApprovalsByOthers != nil ||
		views[0].ApprovalsByContributors != nil || !views[0].ReviewsCheckedAt.IsZero() {
		t.Errorf("pull request after the migration: %+v, %v", views, err)
	}
	for stmt, bads := range map[string][]any{
		"UPDATE pull_requests SET reviews_by_others = ?":   {-1},
		"UPDATE pull_requests SET approvals_by_others = ?": {-2},
		"UPDATE pull_requests SET reviews_checked_at = ?":  {"", "2026-10-20 09:00:00", "2026-10-20T09:00:00+02:00"},
	} {
		for _, bad := range bads {
			if err := execErr(t, s, stmt, bad); err == nil {
				t.Errorf("%s with %v accepted", stmt, bad)
			}
		}
	}
	if err := execErr(t, s, "UPDATE pull_requests SET approvals_by_others = 1, approvals_by_contributors = 2"); err == nil {
		t.Error("more approvals by contributors than by others accepted")
	}
	if err := execErr(t, s, "UPDATE pull_requests SET approvals_by_others = 1, approvals_by_contributors = -1"); err == nil {
		t.Error("negative approvals by contributors accepted")
	}
	mustExec(t, s, `UPDATE pull_requests SET reviews_by_others = 3, approvals_by_others = 2,
		approvals_by_contributors = 1, reviews_checked_at = ?`, testDate)

	// Rollback: columns removed, rows kept.
	if _, err := p.DownTo(ctx, 20); err != nil {
		t.Fatalf("DownTo(20): %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM pull_requests"); got != 1 {
		t.Errorf("%d pull requests after the rollback, want 1", got)
	}
	for _, col := range []string{"reviews_by_others", "approvals_by_others", "approvals_by_contributors", "reviews_checked_at"} {
		if err := execErr(t, s, "SELECT "+col+" FROM pull_requests"); err == nil {
			t.Errorf("pull_requests.%s still present", col)
		}
	}
	if _, err := p.UpTo(ctx, 21); err != nil {
		t.Fatalf("UpTo(21) again: %v", err)
	}
}

func TestPullRequestReviewsStore(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	yes := true
	merged := func(n, h int) model.PullRequest {
		p := pr(n, model.PullRequestClosed, h)
		p.Merged, p.MergedAt = true, p.UpdatedAt
		return p
	}
	fork := merged(4, 4)
	fork.FromFork = &yes
	draft := pr(6, model.PullRequestOpen, 6)
	draft.Draft = true
	closed := pr(7, model.PullRequestClosed, 7)
	if err := applyExtras(t, s, repoID, RepoSyncWrite{PullRequests: []model.PullRequest{
		merged(1, 1), merged(2, 3), pr(3, model.PullRequestOpen, 2), fork, pr(5, model.PullRequestOpen, 5), draft, closed,
	}}); err != nil {
		t.Fatalf("ApplyRepoSync: %v", err)
	}
	numbers := func() []int {
		t.Helper()
		got, err := s.PullRequestsToReview(ctx, repoID)
		if err != nil {
			t.Fatal(err)
		}
		out := []int{}
		for _, c := range got {
			out = append(out, c.Number)
			if c.Merged == c.MergedAt.IsZero() || c.UpdatedAt.IsZero() || c.AuthorProviderID != 501 {
				t.Errorf("candidate %+v", c)
			}
		}
		return out
	}
	// Merged first (most recent merge first), then open (most recent update first); no fork, draft or
	// closed PR.
	if got, want := numbers(), []int{2, 1, 5, 3}; !equalInts(got, want) {
		t.Fatalf("to review = %v, want %v", got, want)
	}

	checked := t0.Add(100 * time.Hour)
	for _, n := range []int{1, 2, 3, 5} {
		ok, err := s.SetPullRequestReviews(ctx, PullRequestReviewsWrite{RepoID: repoID, Number: n,
			ReviewsByOthers: 3, ApprovalsByOthers: 2, ApprovalsByContributors: 1, CheckedAt: checked})
		if err != nil || !ok {
			t.Fatalf("SetPullRequestReviews(%d): %v, %v", n, ok, err)
		}
	}
	if got := numbers(); len(got) != 0 {
		t.Errorf("to review after reading = %v, want none", got)
	}
	conv, err := s.ConventionPullRequests(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range conv {
		switch c.Number {
		case 1, 2, 3, 5:
			if !equalInt64(c.ReviewsByOthers, ptr64(3)) || !equalInt64(c.ApprovalsByOthers, ptr64(2)) ||
				!equalInt64(c.ApprovalsByContributors, ptr64(1)) || !c.ReviewsCheckedAt.Equal(checked) {
				t.Errorf("PR %d: %+v", c.Number, c.PullRequest)
			}
		default:
			if c.ReviewsByOthers != nil || !c.ReviewsCheckedAt.IsZero() {
				t.Errorf("PR %d never read: %+v", c.Number, c.PullRequest)
			}
		}
	}

	// A new sync keeps the counters (UpsertPullRequest does not list them); an open PR updated after
	// the reading comes back, and so does one merged after it.
	reopened := pr(3, model.PullRequestOpen, 200)
	later := merged(5, 300)
	if err := applyExtras(t, s, repoID, RepoSyncWrite{PullRequests: []model.PullRequest{reopened, later}}); err != nil {
		t.Fatal(err)
	}
	if got, want := numbers(), []int{5, 3}; !equalInts(got, want) {
		t.Errorf("to review after updates = %v, want %v", got, want)
	}
	views, err := s.ListPullRequests(ctx, PullRequestFilter{RepoID: repoID})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.Number == 3 && !equalInt64(v.ReviewsByOthers, ptr64(3)) {
			t.Errorf("counters lost by the upsert: %+v", v.PullRequest)
		}
	}

	// Invalid counters and absent PR.
	for _, bad := range []PullRequestReviewsWrite{
		{RepoID: repoID, Number: 1, ReviewsByOthers: -1, CheckedAt: checked},
		{RepoID: repoID, Number: 1, ApprovalsByOthers: 1, ApprovalsByContributors: 2, CheckedAt: checked},
		{RepoID: repoID, Number: 1},
	} {
		if _, err := s.SetPullRequestReviews(ctx, bad); err == nil {
			t.Errorf("SetPullRequestReviews(%+v) accepted", bad)
		}
	}
	if ok, err := s.SetPullRequestReviews(ctx, PullRequestReviewsWrite{RepoID: repoID, Number: 99, CheckedAt: checked}); ok || err != nil {
		t.Errorf("absent PR: %v, %v", ok, err)
	}
}

func TestRepoCommitAuthorAccounts(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")
	bob := mustExec(t, s, `INSERT INTO contributors (provider, provider_user_id, provider_login) VALUES ('github', 77, 'Bob')`)
	carol := mustExec(t, s, `INSERT INTO contributors (provider_login, name) VALUES ('', 'Carol')`)
	dave := mustExec(t, s, `INSERT INTO contributors (provider, provider_user_id, provider_login) VALUES ('github', 88, 'dave')`)
	insert := `INSERT INTO commits (repo_id, sha, author_id, authored_at, committed_at) VALUES (?, ?, ?, ?, ?)`
	mustExec(t, s, insert, repoID, sha(1), bob, testDate, testDate)
	mustExec(t, s, insert, repoID, sha(2), bob, testDate, testDate)
	mustExec(t, s, insert, repoID, sha(3), carol, testDate, testDate)
	mustExec(t, s, insert, other, sha(4), dave, testDate, testDate)
	ids, logins, err := s.RepoCommitAuthorAccounts(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || !ids[77] || len(logins) != 1 || !logins["bob"] {
		t.Errorf("accounts = %v, %v; want {77}, {bob}", ids, logins)
	}
}

func ptr64(v int64) *int64 { return &v }
