package diagnostic

import (
	"slices"
	"testing"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// formHistory returns n regular commits of author 1, one per day from day0, with a measured form: some
// comments, few documented functions, stable lexical diversity, no tests, short messages.
func formHistory(n int) []store.CommitSignalView {
	views := regular(1, n)
	for i := range views {
		views[i].Form = model.CommitForm{
			Known: true, SourceLines: 40 + i%5, CommentLines: 2 + i%3, Functions: 2,
			DocumentedFunctions: 0, Identifiers: 100, LexicalDiversity: 0.60 + float64(i%5)/100,
			HasDiversity: true, MessageLength: 20 + i%7,
		}
	}
	return views
}

func TestFormSignals(t *testing.T) {
	views := formHistory(25)
	odd := view(1, "odd", day0.AddDate(0, 0, 25), 30, 2, 2)
	odd.Form = model.CommitForm{
		Known: true, SourceLines: 60, CommentLines: 30, Functions: 5, DocumentedFunctions: 5,
		Identifiers: 300, LexicalDiversity: 0.30, HasDiversity: true, TestLines: 20,
		MessageLength: 400, MessageBullets: 4, MessageConventional: true,
	}
	views = append(views, odd)
	got := formSignals(views, RhythmOptions{})
	if len(got) != 1 {
		t.Fatalf("form signals: %+v", got)
	}
	hits := got["odd"]
	if ids := hitIDs(hits); !slices.Equal(ids, []string{"F1", "F2", "F3", "F4", "F5"}) {
		t.Fatalf("signals of the odd commit: %v", ids)
	}
	if f1 := hits[0]; f1.Raw != 0.5 || !f1.HasZ || f1.Z <= ZThreshold || !f1.HasMedian {
		t.Errorf("F1: %+v", f1)
	}
	if f2 := hits[1]; f2.Raw != 1 || f2.Median != 0 || f2.HasZ {
		t.Errorf("F2: %+v", f2)
	}
	if f3 := hits[2]; f3.Raw != 0.30 || f3.Z >= -ZThreshold {
		t.Errorf("F3: %+v", f3)
	}
	if f4 := hits[3]; f4.Raw != 400 || f4.Z <= ZThreshold {
		t.Errorf("F4: %+v", f4)
	}
	if f5 := hits[4]; f5.Raw != 20 || f5.Median != 0 {
		t.Errorf("F5: %+v", f5)
	}

	// Two form signals and no rhythm signal: « to review » ; a form signal alone: no lead.
	commits, _, _ := rhythm(views, RhythmOptions{})
	if len(commits) != 1 || Level(commits[0].hits) != LevelToReview {
		t.Errorf("rhythm with form: %+v", commits)
	}
}

func TestFormSignalsNeedHistoryAndKnownForm(t *testing.T) {
	views := formHistory(10) // fewer than MinHistory measured commits
	odd := view(1, "odd", day0.AddDate(0, 0, 10), 30, 2, 2)
	odd.Form = model.CommitForm{Known: true, SourceLines: 60, CommentLines: 30, Functions: 5,
		DocumentedFunctions: 5, TestLines: 20, MessageLength: 400, MessageConventional: true}
	if got := formSignals(append(views, odd), RhythmOptions{}); len(got) != 0 {
		t.Errorf("short history: %+v", got)
	}

	// Unmeasured forms are not history: the commits written before phase 7.5 do not count.
	views = formHistory(25)
	for i := range views {
		views[i].Form.Known = false
	}
	if got := formSignals(append(views, odd), RhythmOptions{}); len(got) != 0 {
		t.Errorf("unknown forms counted: %+v", got)
	}

	// F4 needs a novelty: a long message alone is not a style break.
	views = formHistory(25)
	long := view(1, "long", day0.AddDate(0, 0, 25), 10, 2, 1)
	long.Form = model.CommitForm{Known: true, MessageLength: 400}
	if got := formSignals(append(views, long), RhythmOptions{}); len(got) != 0 {
		t.Errorf("long message without novelty: %+v", got)
	}
}

func TestFormSignalsProbable(t *testing.T) {
	views := formHistory(25)
	big := view(1, "big", day0.AddDate(0, 0, 25), 400, 5, 10)
	big.Form = model.CommitForm{Known: true, SourceLines: 60, CommentLines: 30, Identifiers: 300,
		LexicalDiversity: 0.62, HasDiversity: true, MessageLength: 22}
	commits, _, _ := rhythm(append(views, big), RhythmOptions{})
	if len(commits) != 1 {
		t.Fatalf("commits: %+v", commits)
	}
	hits := commits[0].hits
	sortHits(hits)
	if ids := hitIDs(hits); !slices.Equal(ids, []string{"A7", "F1", "R2", "R3", "R5"}) || Level(hits) != LevelProbable {
		t.Errorf("signals %v, level %q", ids, Level(hits))
	}
}
