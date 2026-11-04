package diagnostic

import (
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
)

func TestLeadKeyAndFind(t *testing.T) {
	day := time.Date(2026, 6, 1, 22, 0, 0, 0, time.UTC)
	leads := []Lead{
		{Kind: KindCommit, RepoID: 1, SHA: "abc"},
		{Kind: KindPullRequest, RepoID: 1, Number: 7},
		{Kind: KindDay, RepoID: 2, AuthorID: 42, Date: day},
	}
	want := []string{"commit/abc", "pull_request/7", "day/42/20260601T220000Z"}
	for i, l := range leads {
		if got := l.Key(); got != want[i] {
			t.Errorf("Key = %q, want %q", got, want[i])
		}
	}
	rep := Report{Leads: leads}
	if l, ok := FindLead(rep, 2, "day/42/20260601T220000Z"); !ok || l.AuthorID != 42 {
		t.Errorf("FindLead day: %+v, %v", l, ok)
	}
	if _, ok := FindLead(rep, 2, "commit/abc"); ok {
		t.Error("lead found in the wrong repository")
	}
}

func TestNewReview(t *testing.T) {
	tr := i18n.For(i18n.English)
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	lead := Lead{Kind: KindCommit, RepoID: 3, SHA: "abc", Date: now.Add(-time.Hour),
		Hits: []Hit{{Signal: "R2"}, {Signal: "D1"}, {Signal: "R2"}}}
	r, err := NewReview(tr, lead, ReviewInput{Decision: DecisionNotAssisted, Cause: CauseRefactoring,
		Confidence: ConfidenceMedium, Note: "  vu en séance  "}, "prof", now)
	if err != nil || r.LeadKey != "commit/abc" || r.Cause != CauseRefactoring || r.Note != "vu en séance" ||
		r.Reviewer != "prof" || strings.Join(r.Signals, ",") != "D1,R2" || !r.ReviewedAt.Equal(now) {
		t.Errorf("NewReview = %+v, %v", r, err)
	}
	// The cause is only kept for "not assisted".
	if r, _ := NewReview(tr, lead, ReviewInput{Decision: DecisionAssisted, Cause: CauseOther, Confidence: ConfidenceLow}, "p", now); r.Cause != "" {
		t.Errorf("cause kept for assisted: %+v", r)
	}
	for name, in := range map[string]ReviewInput{
		"unknown decision": {Decision: "guilty", Confidence: ConfidenceLow},
		"missing cause":    {Decision: DecisionNotAssisted, Confidence: ConfidenceLow},
		"unknown cause":    {Decision: DecisionNotAssisted, Cause: "lazy", Confidence: ConfidenceLow},
		"no confidence":    {Decision: DecisionUndetermined},
		"note too long":    {Decision: DecisionUndetermined, Confidence: ConfidenceLow, Note: strings.Repeat("é", MaxNote+1)},
	} {
		if _, err := NewReview(tr, lead, in, "p", now); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := NewReview(tr, lead, ReviewInput{Decision: DecisionUndetermined, Confidence: ConfidenceLow,
		Note: strings.Repeat("é", MaxNote)}, "p", now); err != nil {
		t.Errorf("note at the limit refused: %v", err)
	}
}

func TestThroughputAndNeighbors(t *testing.T) {
	views := regular(1, 10) // one commit per day from day0 (2026-06-01 10:00 UTC), 8 to 12 lines
	views = append(views, view(2, "other", day0.AddDate(0, 0, 9), 500, 0, 1))
	series := Throughput(views, 1, 1, day0.AddDate(0, 0, 9), 5, time.UTC)
	if len(series) != 5 || series[0].Date != "2026-06-06" || series[4].Date != "2026-06-10" || !series[4].Highlight ||
		series[3].Highlight || series[4].Count != 8+9%5 {
		t.Errorf("Throughput = %+v", series)
	}
	n := Neighbors(views, 1, 1, views[5].SHA, 3)
	if len(n) != 7 || n[0].SHA != views[2].SHA || n[6].SHA != views[8].SHA {
		t.Errorf("Neighbors = %d items", len(n))
	}
	if n := Neighbors(views, 1, 1, views[0].SHA, 3); len(n) != 4 {
		t.Errorf("Neighbors at the start = %d", len(n))
	}
	if Neighbors(views, 1, 1, "absent", 3) != nil {
		t.Error("Neighbors of an unknown commit")
	}
}
