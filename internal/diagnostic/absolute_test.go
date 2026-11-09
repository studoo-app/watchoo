package diagnostic

import (
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// measured returns a commit of author 1 with a measured form.
func measured(sha string, at time.Time, files, adds int, f model.CommitForm) store.CommitSignalView {
	v := view(1, sha, at, adds, 0, files)
	f.Known = true
	if f.SourceLines == 0 {
		f.SourceLines = adds
	}
	v.Form = f
	return v
}

func hitsOf(t *testing.T, got map[string][]Hit, sha string) []string {
	t.Helper()
	ids := hitIDs(got[sha])
	slices.Sort(ids)
	return ids
}

// A project delivered in one go: a big commit with decorative comments and French without accents, no
// work-in-progress marker nor short variable anywhere.
func TestAbsoluteSignalsOneGo(t *testing.T) {
	g := []store.CommitSignalView{
		measured("init", day0, 1, 10, model.CommitForm{ShortIdentifiers: 0}),
		measured("dump", day0.Add(time.Hour), 12, 900, model.CommitForm{DecorativeLines: 6, FrWords: 20, FrWordsUnaccented: 19}),
	}
	got := absoluteSignals(g, RhythmOptions{})
	if ids := hitsOf(t, got, "dump"); !slices.Equal(ids, []string{"A1", "A2", "A5", "A6", "A7"}) {
		t.Fatalf("signals of the dump commit: %v", ids)
	}
	if len(got["init"]) != 0 {
		t.Errorf("signals of the first commit: %v", hitIDs(got["init"]))
	}
	commits, _, _ := rhythm(g, RhythmOptions{})
	if len(commits) != 1 || Level(commits[0].hits) != LevelProbable {
		t.Errorf("one-go project: %+v", commits)
	}
}

func TestAbsoluteSignalsSilenceRevertAndCounters(t *testing.T) {
	g := []store.CommitSignalView{
		measured("a", day0, 1, 20, model.CommitForm{MessageFix: true}),
		measured("b", day0.Add(time.Hour), 1, 20, model.CommitForm{MessageFix: true}),
		measured("c", day0.Add(2*time.Hour), 1, 20, model.CommitForm{MessageFix: true, MessageRevert: true}),
		measured("d", day0.Add(150*time.Minute), 2, 150, model.CommitForm{WIPMarkers: 2, ShortIdentifiers: 4}),
		measured("e", day0.AddDate(0, 0, 20), 5, 350, model.CommitForm{}),
	}
	got := absoluteSignals(g, RhythmOptions{})
	if ids := hitsOf(t, got, "d"); !slices.Equal(ids, []string{"A4", "H1", "H2", "H4"}) {
		t.Errorf("revert then replacement: %v", ids)
	}
	if ids := hitsOf(t, got, "e"); !slices.Equal(ids, []string{"A3"}) {
		t.Errorf("after a silence: %v", ids)
	}
	if h := got["e"][0]; h.Value != "19" {
		t.Errorf("A3 value = %q, want 19 full days", h.Value)
	}
	// More than 3 commits: not « in one go »; work-in-progress markers: not « final state ».
	for _, ids := range got {
		if slices.Contains(hitIDs(ids), "A2") || slices.Contains(hitIDs(ids), "A7") {
			t.Errorf("A2 or A7 raised: %v", hitIDs(ids))
		}
	}
}

func TestAbsoluteSignalsSkeleton(t *testing.T) {
	g := []store.CommitSignalView{
		measured("skel", day0, 8, 8, model.CommitForm{}),
		measured("big", day0.Add(time.Hour), 3, 600, model.CommitForm{}),
	}
	got := absoluteSignals(g, RhythmOptions{})
	if ids := hitsOf(t, got, "big"); !slices.Equal(ids, []string{"A1", "A2", "H3"}) {
		t.Errorf("signals with an initial skeleton: %v", ids)
	}
}

func TestAbsoluteSignalsNeedMeasuredForm(t *testing.T) {
	v := view(1, "x", day0, 900, 0, 10) // form not measured (row from before phase 7.7)
	if got := absoluteSignals([]store.CommitSignalView{v}, RhythmOptions{}); len(got) != 0 {
		t.Errorf("unmeasured form: %v", got)
	}
}

func TestLevelWithAbsoluteAndCounters(t *testing.T) {
	for _, tc := range []struct {
		hits []string
		want string
	}{
		{[]string{"A1"}, LevelToReview},
		{[]string{"A5"}, ""},
		{[]string{"A5", "A6"}, LevelToReview},
		{[]string{"A1", "A2", "A5"}, LevelProbable},
		{[]string{"A1", "A2", "A5", "H1"}, LevelToReview}, // lowered, never removed
		{[]string{"A1", "H2"}, LevelToReview},
		{[]string{"H1"}, ""},
		{[]string{"D1", "H1"}, LevelDeclared},
		{[]string{"R2", "R5", "F1", "H4"}, LevelToReview},
	} {
		var hits []Hit
		for _, id := range tc.hits {
			hits = append(hits, Hit{Signal: id})
		}
		if got := Level(hits); got != tc.want {
			t.Errorf("Level(%v) = %q, want %q", tc.hits, got, tc.want)
		}
	}
}
