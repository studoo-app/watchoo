package diagnostic

import (
	"reflect"
	"testing"
)

func TestWeight(t *testing.T) {
	cases := map[string]int{
		"D1": 10, "D2": 10, "D3": 10, "D5": 10, "D4": 0,
		"R1": 3, "R6": 3, "A1": 3, "A4": 3,
		"F1": 1, "F5": 1, "A5": 1, "A7": 1,
		"H1": 0, "H4": 0, "X9": 0,
	}
	for sig, want := range cases {
		if got := Weight(sig); got != want {
			t.Errorf("Weight(%s) = %d, want %d", sig, got, want)
		}
	}
}

func TestSummarize(t *testing.T) {
	if got := Summarize(nil); !reflect.DeepEqual(got, Summary{}) {
		t.Fatalf("Summarize(nil) = %+v", got)
	}
	leads := []Lead{
		// Two D1 values on the same lead count once.
		{Level: LevelDeclared, Hits: []Hit{{Signal: "D1", Value: "claude"}, {Signal: "D1", Value: "copilot"}}},
		{Level: LevelProbable, Hits: []Hit{{Signal: "R1"}, {Signal: "R2"}, {Signal: "F1"}}},
		{Level: LevelToReview, Hits: []Hit{{Signal: "R2"}, {Signal: "H1"}}},
	}
	got := Summarize(leads)
	want := Summary{Score: 10 + 3 + 6 + 1, Leads: 3, Declared: 1, Probable: 1, ToReview: 1, Signals: []SignalCount{
		{Signal: "D1", Leads: 1, Weight: 10, Points: 10},
		{Signal: "R2", Leads: 2, Weight: 3, Points: 6},
		{Signal: "R1", Leads: 1, Weight: 3, Points: 3},
		{Signal: "F1", Leads: 1, Weight: 1, Points: 1},
		{Signal: "H1", Leads: 1, Weight: 0, Points: 0},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Summarize = %+v\nwant %+v", got, want)
	}
}

func TestSummarizeByContributor(t *testing.T) {
	leads := []Lead{
		{Kind: KindCommit, Level: LevelDeclared, AuthorID: 1, Hits: []Hit{{Signal: "D1"}}},
		{Kind: KindDay, Level: LevelToReview, AuthorID: 1, Hits: []Hit{{Signal: "R1"}}},
		{Kind: KindPullRequest, Level: LevelDeclared, AuthorLogin: "Bob", Hits: []Hit{{Signal: "D5"}}},
		{Kind: KindPullRequest, Level: LevelDeclared, AuthorLogin: "ghost", Hits: []Hit{{Signal: "D5"}}},
		{Kind: KindPullRequest, Level: LevelDeclared, Hits: []Hit{{Signal: "D2"}}},
	}
	got := SummarizeByContributor(leads, map[string]int64{"bob": 2})
	if len(got) != 2 {
		t.Fatalf("SummarizeByContributor = %+v, want 2 contributors", got)
	}
	if a := got[1]; a.Score != 13 || a.Leads != 2 || a.Declared != 1 || a.ToReview != 1 {
		t.Errorf("contributor 1 = %+v", a)
	}
	if b := got[2]; b.Score != 10 || b.Leads != 1 {
		t.Errorf("contributor 2 = %+v", b)
	}
}

func TestTier(t *testing.T) {
	for score, want := range map[int]string{0: TierNone, -1: TierNone, 1: TierMedium, 29: TierMedium, 30: TierHigh, 120: TierHigh} {
		if got := Tier(score); got != want {
			t.Errorf("Tier(%d) = %s, want %s", score, got, want)
		}
	}
}
