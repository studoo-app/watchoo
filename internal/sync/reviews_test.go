package sync_test

// Conventions, phase 8.6 (PR4): reviews of the pull requests read after the sync write, only with the
// conventions on and in API mode, once per merged PR (and again for an open PR updated since), at most
// wsync.MaxReviewsPerRepo requests per repository, under the quota reserve; three counters written,
// never a reviewer's login nor a review body; a failure never fails the repository.

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// secretReviewer is a confidential reviewer's login: never in the logs, the reports nor the database.
const secretReviewer = "relecteur-confidentiel"

func init() { forbidden = append(forbidden, secretReviewer) }

// reviewAt returns a review by login (account id) in state, submitted h hours after epoch.
func reviewAt(id int64, login string, account int64, state string, h int) provider.PullRequestReview {
	r := provider.PullRequestReview{ID: id, Reviewer: provider.Account{Login: login, ID: account}, State: state}
	if state != provider.ReviewPending {
		r.SubmittedAt = epoch.Add(time.Duration(h) * time.Hour)
	}
	return r
}

// mergedPR returns a merged PR n (author secretLogin), merged h hours after epoch.
func mergedPR(n, h int) provider.PullRequest {
	pr := fakePR(n, provider.PullRequestClosed, h)
	pr.Merged, pr.MergedAt = true, pr.ClosedAt
	return pr
}

// reviewCounts returns the review counters of the PRs of the repository repoID, by number.
func (e *env) reviewCounts(repoID int64) map[int]model.PullRequest {
	e.t.Helper()
	out := map[int]model.PullRequest{}
	for n, pr := range e.convPulls(repoID) {
		out[n] = pr.PullRequest
	}
	return out
}

// assertNotStored checks that s is in no file of the database.
func (e *env) assertNotStored(s string) {
	e.t.Helper()
	for _, suffix := range []string{"", "-wal"} {
		if b, err := os.ReadFile(e.dbPath + suffix); err == nil && strings.Contains(string(b), s) {
			e.t.Errorf("%q found in %s", s, e.dbPath+suffix)
		}
	}
}

func wantCounts(t *testing.T, n int, pr model.PullRequest, reviews, approvals, byContributors int64) {
	t.Helper()
	if pr.ReviewsByOthers == nil || pr.ApprovalsByOthers == nil || pr.ApprovalsByContributors == nil ||
		*pr.ReviewsByOthers != reviews || *pr.ApprovalsByOthers != approvals ||
		*pr.ApprovalsByContributors != byContributors || pr.ReviewsCheckedAt.IsZero() {
		t.Errorf("PR %d: counters %v %v %v (checked %v), want %d %d %d", n, deref(pr.ReviewsByOthers),
			deref(pr.ApprovalsByOthers), deref(pr.ApprovalsByContributors), pr.ReviewsCheckedAt, reviews, approvals, byContributors)
	}
}

func TestPullRequestReviewsPasses(t *testing.T) {
	e := newEnv(t, withConventions(), func(e *env) { e.cfg.Conventions.ExcludedAccounts = []string{"Formateur"} })
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour) // commit author: etudiant-test (1001)
	fork := mergedPR(3, 3)
	fork.FromFork = provider.True
	bot := mergedPR(5, 5)
	bot.Author = provider.Account{Login: "dependabot[bot]", ID: 49699333}
	draft := fakePR(6, provider.PullRequestOpen, 6)
	draft.Draft = true
	e.fake.SetPullRequests(1, mergedPR(1, 10), fakePR(2, provider.PullRequestOpen, 2), fork,
		fakePR(4, provider.PullRequestClosed, 4), bot, draft)
	e.fake.SetPullRequestReviews(1, 1,
		reviewAt(1, secretReviewer, 9001, provider.ReviewApproved, 1),
		reviewAt(2, "etudiant-test", 1001, provider.ReviewChangesRequested, 2),
		reviewAt(3, "etudiant-test", 1001, provider.ReviewCommented, 3),
		reviewAt(4, "etudiant-test", 1001, provider.ReviewApproved, 4),
		reviewAt(5, secretLogin, 4242, provider.ReviewApproved, 5),       // the author
		reviewAt(6, "renovate[bot]", 2740, provider.ReviewApproved, 5),   // a bot
		reviewAt(7, "formateur", 77, provider.ReviewApproved, 5),         // excluded
		reviewAt(8, "", 0, provider.ReviewApproved, 5),                   // deleted account
		reviewAt(9, "autre", 31, provider.ReviewPending, 0),              // pending
		reviewAt(10, "tardif", 32, provider.ReviewApproved, 20),          // after the merge
		reviewAt(11, secretReviewer, 9001, provider.ReviewCommented, 21), // after the merge
	)
	e.fake.SetPullRequestReviews(1, 2, reviewAt(20, secretReviewer, 9001, provider.ReviewCommented, 3))

	e.syncOK(repo.ID)
	if n := e.fake.Calls(providertest.MethodPullReviews); n != 2 {
		t.Errorf("first sync: %d review requests, want 2 (merged PR 1, open PR 2)", n)
	}
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodPullReviews && c.Number != 1 && c.Number != 2 {
			t.Errorf("reviews of PR %d requested", c.Number)
		}
	}
	got := e.reviewCounts(repo.ID)
	// PR 1: reviews by others = secretReviewer (2) + etudiant-test (3) + tardif (1); approvals at the
	// merge: secretReviewer and etudiant-test; contributors among them: etudiant-test.
	wantCounts(t, 1, got[1], 6, 2, 1)
	wantCounts(t, 2, got[2], 1, 0, 0)
	for _, n := range []int{3, 4, 5, 6} {
		if got[n].ReviewsByOthers != nil || !got[n].ReviewsCheckedAt.IsZero() {
			t.Errorf("PR %d: reviews read: %+v", n, got[n])
		}
	}

	// Second sync: nothing new, no request.
	e.syncOK(repo.ID)
	if n := e.fake.Calls(providertest.MethodPullReviews); n != 2 {
		t.Errorf("second sync: %d review requests in all, want 2", n)
	}

	// The open PR 2 is approved then merged after its first read: read again once.
	now := e.clock.Now()
	pr2 := fakePR(2, provider.PullRequestClosed, 0)
	pr2.Merged, pr2.MergedAt, pr2.ClosedAt, pr2.UpdatedAt = true, now.Add(time.Hour), now.Add(time.Hour), now.Add(time.Hour)
	e.fake.SetPullRequests(1, pr2)
	approve := reviewAt(21, secretReviewer, 9001, provider.ReviewApproved, 0)
	approve.SubmittedAt = now.Add(30 * time.Minute)
	e.fake.SetPullRequestReviews(1, 2, reviewAt(20, secretReviewer, 9001, provider.ReviewCommented, 3), approve)
	e.clock.Set(now.Add(2 * time.Hour))
	e.syncOK(repo.ID)
	e.syncOK(repo.ID)
	if n := e.fake.Calls(providertest.MethodPullReviews); n != 3 {
		t.Errorf("after the merge of PR 2: %d review requests in all, want 3", n)
	}
	wantCounts(t, 2, e.reviewCounts(repo.ID)[2], 2, 1, 0)
	e.assertNotStored(secretReviewer)
	e.assertNotStored("formateur")
}

func TestPullRequestReviewsDisabled(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.SetPullRequests(1, mergedPR(1, 2))
	e.syncOK(repo.ID)
	if n := e.fake.Calls(providertest.MethodPullReviews); n != 0 {
		t.Errorf("conventions off: %d review requests", n)
	}
	if pr := e.reviewCounts(repo.ID)[1]; pr.ReviewsByOthers != nil {
		t.Errorf("conventions off: counters written %+v", pr)
	}
}

func TestPullRequestReviewsCap(t *testing.T) {
	e := newEnv(t, withConventions())
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	total := wsync.MaxReviewsPerRepo + 5
	pulls := make([]provider.PullRequest, 0, total)
	for n := 1; n <= total; n++ {
		pulls = append(pulls, mergedPR(n, n))
	}
	e.fake.SetPullRequests(1, pulls...)
	e.syncOK(repo.ID)
	if n := e.fake.Calls(providertest.MethodPullReviews); n != wsync.MaxReviewsPerRepo {
		t.Errorf("first sync: %d review requests, want %d", n, wsync.MaxReviewsPerRepo)
	}
	got := e.reviewCounts(repo.ID)
	for n := 1; n <= 5; n++ { // the oldest merges come last
		if got[n].ReviewsByOthers != nil {
			t.Errorf("PR %d read before the most recent ones", n)
		}
	}
	e.syncOK(repo.ID)
	if n := e.fake.Calls(providertest.MethodPullReviews); n != total {
		t.Errorf("second sync: %d review requests in all, want %d", n, total)
	}
	for n, pr := range e.reviewCounts(repo.ID) {
		if pr.ReviewsByOthers == nil {
			t.Errorf("PR %d never read", n)
		}
	}
}

// Quota: under the reserve, no review is requested; exhausted during the pass, the repository succeeds
// and the following ones are deferred; another error stops the pass without failing the repository.
func TestPullRequestReviewsQuotaAndErrors(t *testing.T) {
	e := newEnv(t, withConventions(), withParallelism(1))
	a, b := e.addRepo(1, "a"), e.addRepo(2, "b")
	for _, id := range []int64{1, 2} {
		e.fake.AppendCommits(id, "main", 1, epoch, time.Hour)
		e.fake.SetPullRequests(id, mergedPR(1, 2), mergedPR(2, 3))
	}
	reset := time.Date(2026, 10, 20, 11, 5, 0, 0, time.UTC)
	e.fake.FailNext(providertest.MethodPullReviews, &provider.APIError{Status: 403, Err: &provider.RateLimitError{Reset: reset}})
	rep := e.syncAll()
	if first := rep.Repos[0]; first.RepoID != a.ID || first.Status != wsync.StatusSuccess || len(first.Warnings) != 0 {
		t.Errorf("first repository: %+v", first)
	}
	if second := rep.Repos[1]; second.RepoID != b.ID || second.Status != wsync.StatusDeferred || !second.ResumeAt.Equal(reset) {
		t.Errorf("next repository: %+v", second)
	}
	if n := e.fake.Calls(providertest.MethodPullReviews); n != 1 {
		t.Errorf("%d review requests after the exhausted quota, want 1", n)
	}

	// Another error: the repository succeeds without warning, the PR is read at the next sync.
	e2 := newEnv(t, withConventions())
	repo := e2.addRepo(1, "a")
	e2.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e2.fake.SetPullRequests(1, mergedPR(1, 2), mergedPR(2, 3))
	e2.fake.FailNext(providertest.MethodPullReviews, provider.ErrUnavailable)
	if r := e2.syncOK(repo.ID); len(r.Warnings) != 0 {
		t.Errorf("warnings: %q", r.Warnings)
	}
	if n := e2.fake.Calls(providertest.MethodPullReviews); n != 1 {
		t.Errorf("%d review requests after the failure, want 1 (pass stopped)", n)
	}
	e2.syncOK(repo.ID)
	for n, pr := range e2.reviewCounts(repo.ID) {
		if pr.ReviewsByOthers == nil {
			t.Errorf("PR %d not read at the next sync", n)
		}
	}
	// A PR that vanished (404) is skipped, the others are read.
	e3 := newEnv(t, withConventions())
	repo = e3.addRepo(1, "a")
	e3.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e3.fake.SetPullRequests(1, mergedPR(1, 2), mergedPR(2, 3))
	e3.fake.FailNext(providertest.MethodPullReviews, &provider.APIError{Status: 404, Err: provider.ErrNotFound})
	e3.syncOK(repo.ID)
	got := e3.reviewCounts(repo.ID)
	if got[2].ReviewsByOthers != nil || got[1].ReviewsByOthers == nil {
		t.Errorf("after a 404 on PR 2: %v / %v", got[2].ReviewsByOthers, got[1].ReviewsByOthers)
	}
}

// Low quota before the reviews: none is requested; reaching the reserve during the pass stops it (the
// remaining PRs wait for the next sync). With the fake quota, the repository, its branches, its commits
// and the three lists take 6 requests.
func TestPullRequestReviewsLowQuota(t *testing.T) {
	for _, tt := range []struct {
		remaining, reviews int
	}{
		{13, 0}, // 7 left after the lists: under the reserve (10 %)
		{17, 2}, // 11 left: 11 and 10 are not under the reserve, 9 is
	} {
		e := newEnv(t, withConventions())
		repo := e.addRepo(1, "a")
		e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
		e.fake.SetPullRequests(1, mergedPR(1, 2), mergedPR(2, 3), mergedPR(3, 4))
		e.fake.SetRateLimit(provider.RateLimit{Limit: 100, Remaining: tt.remaining})
		if r := e.syncOK(repo.ID); len(r.Warnings) != 0 {
			t.Errorf("remaining %d: warnings %q", tt.remaining, r.Warnings)
		}
		if n := e.fake.Calls(providertest.MethodPullRequests); n != 1 {
			t.Fatalf("remaining %d: %d PR lists, want 1", tt.remaining, n)
		}
		if n := e.fake.Calls(providertest.MethodPullReviews); n != tt.reviews {
			t.Errorf("remaining %d: %d review requests, want %d", tt.remaining, n, tt.reviews)
		}
	}
}

// A source without reviews (ErrUnsupported): nothing written, no warning.
func TestPullRequestReviewsUnsupported(t *testing.T) {
	e := newEnv(t, withConventions())
	repo := e.addRepo(1, "a")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.SetPullRequests(1, mergedPR(1, 2), mergedPR(2, 3))
	e.fake.SetError(providertest.MethodPullReviews, fmt.Errorf("no reviews: %w", provider.ErrUnsupported))
	if r := e.syncOK(repo.ID); len(r.Warnings) != 0 {
		t.Errorf("warnings: %q", r.Warnings)
	}
	if n := e.fake.Calls(providertest.MethodPullReviews); n != 1 {
		t.Errorf("%d review requests, want 1 (pass stopped)", n)
	}
	if pr := e.reviewCounts(repo.ID)[1]; pr.ReviewsByOthers != nil {
		t.Errorf("counters written: %+v", pr)
	}
}

// deref returns *p, or -1 for nil.
func deref(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}

// A PR opened before the start date of the conventions is out of PR4: its reviews are never read.
func TestPullRequestReviewsBeforeSince(t *testing.T) {
	e := newEnv(t, withConventions(), func(e *env) { e.cfg.Conventions.Since = "2026-09-02" })
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	old := mergedPR(1, 40) // opened at epoch, before the start date
	recent := mergedPR(2, 60)
	recent.CreatedAt = epoch.Add(48 * time.Hour)
	openOld := fakePR(3, provider.PullRequestOpen, 50)
	e.fake.SetPullRequests(1, old, recent, openOld)
	e.syncOK(repo.ID)
	if n := e.fake.Calls(providertest.MethodPullReviews); n != 1 {
		t.Errorf("%d review requests, want 1 (PR 2 only)", n)
	}
	got := e.reviewCounts(repo.ID)
	if got[1].ReviewsByOthers != nil || got[3].ReviewsByOthers != nil {
		t.Errorf("PRs opened before the start date read: %+v %+v", got[1], got[3])
	}
	wantCounts(t, 2, got[2], 0, 0, 0)
}

// An open PR is marked read at its forge update date, never at the local clock: merged later (here
// before the local "now", as with a local clock ahead of the forge), it is read again once.
func TestPullRequestReviewsOpenCheckedAtUpdate(t *testing.T) {
	e := newEnv(t, withConventions())
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	now := e.clock.Now()
	open := fakePR(1, provider.PullRequestOpen, 0)
	open.UpdatedAt = now.Add(-2 * time.Hour)
	e.fake.SetPullRequests(1, open)
	e.syncOK(repo.ID)
	if pr := e.reviewCounts(repo.ID)[1]; !pr.ReviewsCheckedAt.Equal(open.UpdatedAt) {
		t.Errorf("open PR checked at %v, want its update date %v", pr.ReviewsCheckedAt, open.UpdatedAt)
	}
	merged := open
	merged.State, merged.Merged = provider.PullRequestClosed, true
	merged.MergedAt, merged.ClosedAt, merged.UpdatedAt = now.Add(-time.Hour), now.Add(-time.Hour), now.Add(-time.Hour)
	e.fake.SetPullRequests(1, merged)
	e.fake.SetPullRequestReviews(1, 1, reviewAt(1, secretReviewer, 9001, provider.ReviewApproved, 0))
	e.syncOK(repo.ID)
	e.syncOK(repo.ID)
	if n := e.fake.Calls(providertest.MethodPullReviews); n != 2 {
		t.Errorf("%d review requests in all, want 2 (open, then merged once)", n)
	}
	wantCounts(t, 1, e.reviewCounts(repo.ID)[1], 1, 1, 0)
}
