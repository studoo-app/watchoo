package conventions

import (
	"testing"
	"time"
)

func TestReviewTally(t *testing.T) {
	merged := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	at := func(h int) time.Time { return merged.Add(time.Duration(h) * time.Hour) }
	const author = 1
	rv := func(id int64, login, state string, h int) Review {
		return Review{ReviewerID: id, ReviewerLogin: login, State: state, SubmittedAt: at(h)}
	}
	base := ReviewContext{AuthorID: author, MergedAt: merged}
	contrib := base
	contrib.ContributorIDs = map[int64]bool{2: true}
	contrib.ContributorLogins = map[string]bool{"carol": true}
	excluded := base
	excluded.Excluded = []string{" Teacher ", ""}
	open := base
	open.MergedAt = time.Time{}

	tests := []struct {
		name    string
		reviews []Review
		pr      ReviewContext
		want    ReviewCounts
	}{
		{"no review", nil, base, ReviewCounts{}},
		{"one approval", []Review{rv(2, "bob", ReviewApproved, -2)}, base, ReviewCounts{1, 1, 0}},
		{"approval at merge time", []Review{rv(2, "bob", ReviewApproved, 0)}, base, ReviewCounts{1, 1, 0}},
		{"last state before merge wins: changes requested", []Review{
			rv(2, "bob", ReviewApproved, -5), rv(2, "bob", ReviewChangesRequested, -1),
		}, base, ReviewCounts{2, 0, 0}},
		{"last state before merge wins: approved", []Review{
			rv(2, "bob", ReviewChangesRequested, -5), rv(2, "bob", ReviewApproved, -1),
		}, base, ReviewCounts{2, 1, 0}},
		{"dismissed after approved", []Review{
			rv(2, "bob", ReviewApproved, -5), rv(2, "bob", ReviewDismissed, -1),
		}, base, ReviewCounts{2, 0, 0}},
		// GitHub: dismissing a review does not add one, the APPROVED review becomes DISMISSED and keeps
		// its submitted date. The date of the dismissal is unknown: dismissed after the merge or before,
		// the approval no longer counts once the review reads DISMISSED (documented approximation).
		{"approval dismissed in place, dismissal date unknown", []Review{
			rv(2, "bob", ReviewDismissed, -5),
		}, base, ReviewCounts{1, 0, 0}},
		{"approval dismissed in place, another reviewer approving", []Review{
			rv(2, "bob", ReviewDismissed, -5), rv(3, "carol", ReviewApproved, -1),
		}, base, ReviewCounts{2, 1, 0}},
		{"commented does not change the state", []Review{
			rv(2, "bob", ReviewApproved, -5), rv(2, "bob", ReviewCommented, -1),
		}, base, ReviewCounts{2, 1, 0}},
		{"commented only", []Review{rv(2, "bob", ReviewCommented, -1)}, base, ReviewCounts{1, 0, 0}},
		{"pending ignored", []Review{
			rv(2, "bob", ReviewApproved, -5), {ReviewerID: 2, ReviewerLogin: "bob", State: ReviewPending},
		}, base, ReviewCounts{1, 1, 0}},
		{"pending only", []Review{{ReviewerID: 2, ReviewerLogin: "bob", State: ReviewPending}}, base, ReviewCounts{}},
		{"unknown state ignored", []Review{rv(2, "bob", "WHATEVER", -1)}, base, ReviewCounts{}},
		{"review after merge ignored for approvals", []Review{rv(2, "bob", ReviewApproved, 1)}, base, ReviewCounts{1, 0, 0}},
		{"changes requested after merge does not undo the approval", []Review{
			rv(2, "bob", ReviewApproved, -1), rv(2, "bob", ReviewChangesRequested, 2),
		}, base, ReviewCounts{2, 1, 0}},
		{"undated review ignored for approvals of a merged PR", []Review{
			{ReviewerID: 2, ReviewerLogin: "bob", State: ReviewApproved},
		}, base, ReviewCounts{1, 0, 0}},
		{"author approving its own PR", []Review{rv(author, "alice", ReviewApproved, -1)}, base, ReviewCounts{}},
		{"bot", []Review{rv(9, "dependabot[bot]", ReviewApproved, -1), rv(10, "copilot-swe-agent", ReviewApproved, -1)}, base, ReviewCounts{}},
		{"excluded account, case ignored", []Review{
			rv(7, "teacher", ReviewApproved, -1), rv(2, "bob", ReviewApproved, -1),
		}, excluded, ReviewCounts{1, 1, 0}},
		{"reviewer without account", []Review{rv(0, "", ReviewApproved, -1)}, base, ReviewCounts{}},
		{"author without account, reviewer with one", []Review{rv(2, "bob", ReviewApproved, -1)},
			ReviewContext{MergedAt: merged}, ReviewCounts{1, 1, 0}},
		{"same reviewer approving twice", []Review{
			rv(2, "bob", ReviewApproved, -5), rv(2, "bob", ReviewApproved, -1),
		}, base, ReviewCounts{2, 1, 0}},
		{"two distinct reviewers", []Review{
			rv(2, "bob", ReviewApproved, -5), rv(3, "carol", ReviewApproved, -1), rv(4, "dan", ReviewChangesRequested, -1),
		}, base, ReviewCounts{3, 2, 0}},
		{"contributors by id and by login", []Review{
			rv(2, "bob", ReviewApproved, -5), rv(3, "Carol", ReviewApproved, -1), rv(4, "dan", ReviewApproved, -1),
		}, contrib, ReviewCounts{3, 3, 2}},
		{"contributor not approving", []Review{rv(2, "bob", ReviewChangesRequested, -1)}, contrib, ReviewCounts{1, 0, 0}},
		{"same date: last of the list wins", []Review{
			rv(2, "bob", ReviewChangesRequested, -1), rv(2, "bob", ReviewApproved, -1),
		}, base, ReviewCounts{2, 1, 0}},
		{"out of order list: date wins", []Review{
			rv(2, "bob", ReviewApproved, -1), rv(2, "bob", ReviewChangesRequested, -3),
		}, base, ReviewCounts{2, 1, 0}},
		{"open PR: every dated review counts", []Review{rv(2, "bob", ReviewApproved, 5)}, open, ReviewCounts{1, 1, 0}},
		{"open PR: undated decisive review counts", []Review{{ReviewerID: 2, ReviewerLogin: "bob", State: ReviewApproved}}, open, ReviewCounts{1, 1, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReviewTally(tt.reviews, tt.pr); got != tt.want {
				t.Errorf("ReviewTally = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEvalPR4(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	tests := []struct {
		name           string
		by, ap, contrb *int64
		rules          ReviewRules
		want           ReviewResult
	}{
		{"never reviewed", nil, nil, nil, DefaultReviewRules(), ReviewResult{}},
		{"one approval", n(1), n(1), n(0), DefaultReviewRules(), ReviewResult{true, 1, 1, true}},
		{"no approval", n(2), n(0), n(0), DefaultReviewRules(), ReviewResult{true, 2, 0, false}},
		{"min reviews 2 not reached", n(3), n(1), n(1), ReviewRules{MinReviews: 2}, ReviewResult{true, 3, 1, false}},
		{"min reviews 2 reached", n(3), n(2), n(0), ReviewRules{MinReviews: 2}, ReviewResult{true, 3, 2, true}},
		{"min reviews 0 counts as 1", n(0), n(0), n(0), ReviewRules{}, ReviewResult{true, 0, 0, false}},
		{"must contribute, none", n(1), n(1), n(0), ReviewRules{MinReviews: 1, ReviewerMustContribute: true}, ReviewResult{true, 1, 0, false}},
		{"must contribute, one", n(2), n(2), n(1), ReviewRules{MinReviews: 1, ReviewerMustContribute: true}, ReviewResult{true, 2, 1, true}},
		{"must contribute, contributors unknown", n(2), n(2), nil, ReviewRules{MinReviews: 1, ReviewerMustContribute: true}, ReviewResult{}},
		{"reviews count unknown", nil, n(1), n(1), DefaultReviewRules(), ReviewResult{true, 0, 1, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EvalPR4(tt.by, tt.ap, tt.contrb, tt.rules); got != tt.want {
				t.Errorf("EvalPR4 = %+v, want %+v", got, tt.want)
			}
		})
	}
	if r := DefaultReviewRules(); r.MinReviews != 1 || r.ReviewerMustContribute {
		t.Errorf("DefaultReviewRules = %+v", r)
	}
}

// On GitHub, dismissing an approval replaces the state of that same review (APPROVED becomes DISMISSED,
// submitted date unchanged): a read before the dismissal counts the approval, a read after does not,
// whether the dismissal came before or after the merge (its date is not known).
func TestReviewTallyDismissedInPlace(t *testing.T) {
	merged := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	pr := ReviewContext{AuthorID: 1, MergedAt: merged}
	review := Review{ReviewerID: 2, ReviewerLogin: "bob", State: ReviewApproved, SubmittedAt: merged.Add(-time.Hour)}
	if got := ReviewTally([]Review{review}, pr); got != (ReviewCounts{1, 1, 0}) {
		t.Fatalf("before the dismissal: %+v", got)
	}
	review.State = ReviewDismissed
	if got := ReviewTally([]Review{review}, pr); got != (ReviewCounts{1, 0, 0}) {
		t.Errorf("after the dismissal: %+v, want the approval no longer counted", got)
	}
}
