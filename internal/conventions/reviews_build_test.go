package conventions

import (
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// reviewed sets the review counters of a pull request, read at checked.
func reviewed(pr model.ConventionPullRequest, by, approvals, contributors int64, checked time.Time) model.ConventionPullRequest {
	pr.ReviewsByOthers, pr.ApprovalsByOthers, pr.ApprovalsByContributors = &by, &approvals, &contributors
	pr.ReviewsCheckedAt = checked
	return pr
}

// mergedAt marks a pull request merged at at, by the commit sha.
func mergedAt(pr model.ConventionPullRequest, at time.Time, sha string) model.ConventionPullRequest {
	pr.State, pr.Merged, pr.MergedAt, pr.ClosedAt, pr.MergeCommitSHA = model.PullRequestClosed, true, at, at, sha
	return pr
}

func TestEvalPullReview(t *testing.T) {
	pr := mergedAt(cpr(1, "feat: x", "feat/x", 10, "alice", day(5)), day(7), "")
	if got := EvalPullReview(pr.PullRequest, DefaultReviewRules()); got.Known {
		t.Errorf("never read: %+v", got)
	}
	// Read while open, merged since: stale, unknown.
	if got := EvalPullReview(reviewed(pr, 1, 1, 0, day(6)).PullRequest, DefaultReviewRules()); got.Known {
		t.Errorf("read before the merge: %+v", got)
	}
	// Read at the merge: known.
	if got := EvalPullReview(reviewed(pr, 2, 1, 0, day(7)).PullRequest, DefaultReviewRules()); got != (ReviewResult{true, 2, 1, true}) {
		t.Errorf("read at the merge: %+v", got)
	}
	// Open pull request: known as soon as read.
	open := reviewed(cpr(2, "feat: y", "feat/y", 10, "alice", day(5)), 1, 0, 0, day(6))
	if got := EvalPullReview(open.PullRequest, DefaultReviewRules()); got != (ReviewResult{Known: true, ReviewsByOthers: 1}) {
		t.Errorf("open: %+v", got)
	}
}

func TestBuildPullReviews(t *testing.T) {
	r, cl := intervalFixture()
	r.repos[0].ProviderID = 1
	r.pulls = map[int64][]model.ConventionPullRequest{1: {
		reviewed(mergedAt(cpr(1, "feat: a", "feat/a", 10, "alice", day(5)), day(7), ""), 2, 1, 1, day(10)),
		reviewed(mergedAt(cpr(2, "feat: b", "feat/b", 10, "alice", day(5)), day(7), ""), 1, 0, 0, day(10)),
		reviewed(mergedAt(cpr(3, "feat: c", "feat/c", 11, "bob", day(5)), day(7), ""), 3, 1, 0, day(6)), // stale
		mergedAt(cpr(4, "feat: d", "feat/d", 11, "bob", day(5)), day(7), ""),                            // never read
		reviewed(cpr(5, "feat: e", "feat/e", 11, "bob", day(5)), 1, 1, 0, day(8)),                       // open
		reviewed(mergedAt(cpr(6, "chore: f", "deps", 0, "dependabot[bot]", day(5)), day(7), ""), 0, 0, 0, day(10)),
	}}
	got, err := Build(t.Context(), r, cl, Filter{}, forgeOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := got.Repos[0].Pulls
	m, o := p.Merged, p.Open
	if m.Evaluated != 4 || m.ReviewKnown != 2 || m.Reviewed != 1 || m.ReviewsReceived != 3 ||
		o.Evaluated != 1 || o.ReviewKnown != 1 || o.Reviewed != 1 || o.ReviewsReceived != 1 || p.Skipped[PRSkipBot] != 1 {
		t.Errorf("tally: merged %+v open %+v", m, o)
	}
	without := p.MergedWithoutReview()
	if len(without) != 1 || without[0].Number != 2 || without[0].AuthorLogin != "alice" {
		t.Errorf("merged without review: %+v", without)
	}
	var alice, bob ContributorReport
	for _, c := range got.Contributors {
		switch c.AuthorID {
		case 10:
			alice = c
		case 11:
			bob = c
		}
	}
	if a := alice.Pulls.Merged; a.ReviewKnown != 2 || a.Reviewed != 1 || a.ReviewsReceived != 3 {
		t.Errorf("alice: %+v", alice.Pulls)
	}
	if b := bob.Pulls.Merged; b.Evaluated != 2 || b.ReviewKnown != 0 || bob.Pulls.Open.ReviewsReceived != 1 {
		t.Errorf("bob: %+v", bob.Pulls)
	}

	// The settings are applied at read time: two approvers required, then a contributor approver.
	opts := forgeOptions()
	opts.Review = ReviewRules{MinReviews: 2}
	got, _ = Build(t.Context(), r, cl, Filter{}, opts)
	if m := got.Repos[0].Pulls.Merged; m.ReviewKnown != 2 || m.Reviewed != 0 {
		t.Errorf("min reviews 2: %+v", m)
	}
	opts.Review = ReviewRules{MinReviews: 1, ReviewerMustContribute: true}
	got, _ = Build(t.Context(), r, cl, Filter{}, opts)
	if m := got.Repos[0].Pulls.Merged; m.Reviewed != 1 || got.Repos[0].Pulls.MergedWithoutReview()[0].Number != 2 {
		t.Errorf("must contribute: %+v", m)
	}

	// Clone only: unknown, no line.
	opts = forgeOptions()
	opts.LocalOnly = true
	got, _ = Build(t.Context(), r, cl, Filter{}, opts)
	if p := got.Repos[0].Pulls; p.Known || p.Merged.ReviewKnown != 0 || len(p.MergedWithoutReview()) != 0 {
		t.Errorf("clone only: %+v", p)
	}
}

func TestReviewMilestonePR4(t *testing.T) {
	r, cl := intervalFixture()
	r.repos[0].ProviderID = 1
	// v0.1.0 is c2 (day 6), v1.0.0 is c4 (day 8).
	r.tagDates = map[int64]map[string]time.Time{1: {"v0.1.0": day(6), "v1.0.0": day(8), "v1.1.0-rc.1": day(9)}}
	merged := func(n int, at time.Time, sha string, approvals int64, checked time.Time) model.ConventionPullRequest {
		pr := mergedAt(cpr(n, "feat: x", "feat/x", 10, "alice", day(5)), at, sha)
		if checked.IsZero() {
			return pr
		}
		return reviewed(pr, approvals, approvals, 0, checked)
	}
	all := []model.ConventionPullRequest{
		merged(1, day(7), "", 1, day(10)),                                            // in, reviewed
		merged(2, day(7), "", 0, day(10)),                                            // in, not reviewed
		merged(3, day(8).Add(time.Minute), "c4", 1, day(10)),                         // merge commit of the tag: in
		merged(4, day(9), "", 0, day(10)),                                            // after the tag: out
		merged(5, day(5), "", 0, day(10)),                                            // before the previous tag: out
		merged(6, day(7), "", 0, time.Time{}),                                        // in, never read
		merged(7, day(6).Add(time.Minute), "c2", 0, day(10)),                         // merge commit of the previous tag: out
		mergedAt(cpr(8, "chore: x", "deps", 0, "renovate[bot]", day(5)), day(7), ""), // bot: out
		cpr(9, "feat: open", "feat/o", 10, "alice", day(5)),                          // open: out
	}
	set := func(numbers ...int) {
		var prs []model.ConventionPullRequest
		for _, pr := range all {
			if slices.Contains(numbers, pr.Number) {
				prs = append(prs, pr)
			}
		}
		r.pulls = map[int64][]model.ConventionPullRequest{1: prs}
	}
	opts := reviewOptions(t)
	after := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	pr4 := func() (Criterion, *MilestoneReviews) {
		t.Helper()
		rv, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(rv.Repos[0].Criteria, func(c Criterion) bool { return c.ID == CriterionPR4 })
		return rv.Repos[0].Criteria[i], rv.Repos[0].Reviews
	}

	set(1, 2, 3, 4, 5, 6, 7, 8, 9)
	c, m := pr4()
	if c.Status != CriterionNonconform || m == nil || m.Merged != 4 || m.Known != 3 || m.Reviewed != 2 ||
		!slices.Equal(m.Unreviewed, []int{2}) {
		t.Errorf("nonconform: %+v %+v", c, m)
	}
	set(1, 3, 4, 6)
	if c, m := pr4(); c.Status != CriterionUnknown || c.Reason != UnknownReviewsUnchecked || m.Known != 2 || m.Merged != 3 {
		t.Errorf("unchecked: %+v %+v", c, m)
	}
	set(1, 3, 4)
	if c, m := pr4(); c.Status != CriterionConform || m.Reviewed != 2 || len(m.Unreviewed) != 0 || m.Unreviewed == nil {
		t.Errorf("conform: %+v %+v", c, m)
	}
	// min_reviews applied at read time.
	opts.Review = ReviewRules{MinReviews: 2}
	if c, m := pr4(); c.Status != CriterionNonconform || !slices.Equal(m.Unreviewed, []int{3, 1}) {
		t.Errorf("min reviews 2: %+v %+v", c, m)
	}
	opts.Review = DefaultReviewRules()
	set(4, 5, 7)
	if c, _ := pr4(); c.Status != CriterionUnknown || c.Reason != UnknownNoMergedPR {
		t.Errorf("no merged PR: %+v", c)
	}
	// Tag due date: v2.0.0 absent, due 2026-09-20: merged after v1.0.0 (day 8) and before the end of the due
	// date; #3, whose merge produced the commit of v1.0.0, is counted in v1.0.0 only.
	set(1, 3, 4)
	rv, _ := ReviewMilestone(t.Context(), r, cl, Filter{}, "v2.0.0", opts, after)
	if m := rv.Repos[0].Reviews; m == nil || m.Merged != 1 || m.Reviewed != 0 || !slices.Equal(m.Unreviewed, []int{4}) {
		t.Errorf("v2.0.0: %+v", m)
	}
	// Date of a tag commit missing from the database.
	delete(r.tagDates[1], "v0.1.0")
	if c, m := pr4(); c.Status != CriterionUnknown || c.Reason != UnknownNoTagDate || m != nil {
		t.Errorf("no tag date: %+v", c)
	}
	// Clone only.
	opts.LocalOnly = true
	if c, _ := pr4(); c.Reason != UnknownNoAPI {
		t.Errorf("clone only: %+v", c)
	}
}
