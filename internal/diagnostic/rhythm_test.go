package diagnostic

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

var day0 = time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)

// view returns a measured commit of the author authorID in repository 1.
func view(authorID int64, sha string, at time.Time, adds, dels, files int) store.CommitSignalView {
	return store.CommitSignalView{CommitSignal: model.CommitSignal{RepoID: 1, SHA: sha, AuthorID: authorID,
		AuthoredAt: at, AdditionsKept: adds, DeletionsKept: dels, FilesKept: files}}
}

// regular returns n commits of author authorID, one per day from day0, with small varying sizes.
func regular(authorID int64, n int) []store.CommitSignalView {
	var out []store.CommitSignalView
	for i := range n {
		out = append(out, view(authorID, "r"+string(rune('a'+i%26))+string(rune('a'+i/26)), day0.AddDate(0, 0, i),
			8+i%5, 4, 1+i%3))
	}
	return out
}

func hitIDs(hits []Hit) []string {
	var ids []string
	for _, h := range hits {
		ids = append(ids, h.Signal)
	}
	return ids
}

func TestRobustZ(t *testing.T) {
	r := robustOf([]float64{1, 2, 3, 4, 100})
	if r.N != 5 || r.Median != 3 || r.MAD != 1 {
		t.Fatalf("robustOf = %+v", r)
	}
	if _, ok := r.z(100); ok {
		t.Error("z defined with 5 observations")
	}
	xs := make([]float64, 20)
	for i := range xs {
		xs[i] = float64(i % 4) // 0 1 2 3 …: median 1.5, MAD 1
	}
	r = robustOf(xs)
	z, ok := r.z(11.5)
	if !ok || math.Abs(z-0.6745*10) > 1e-9 {
		t.Errorf("z = %v, %v; want %v", z, ok, 0.6745*10)
	}
	if _, ok := robustOf(make([]float64, 30)).z(5); ok {
		t.Error("z defined with a zero MAD")
	}
	if m := median([]float64{4, 1, 3, 2}); m != 2.5 {
		t.Errorf("median = %v", m)
	}
}

func TestRhythmCommitSignals(t *testing.T) {
	views := regular(1, 25)
	big := view(1, "big", day0.AddDate(0, 0, 25), 400, 5, 10)
	views = append(views, big)
	commits, days, histories := rhythm(views, RhythmOptions{})
	if len(commits) != 1 || commits[0].view.SHA != "big" {
		t.Fatalf("commit leads: %+v", commits)
	}
	if got := hitIDs(commits[0].hits); !slices.Equal(got, []string{"R2", "R3", "R5"}) {
		t.Errorf("signals of the big commit: %v", got)
	}
	r2 := commits[0].hits[0]
	if r2.Raw != 400 || r2.Median != 10 || !r2.HasZ || r2.Z <= ZThreshold || r2.Value != "400" {
		t.Errorf("R2: %+v", r2)
	}
	if r3 := commits[0].hits[1]; r3.HasZ || !r3.HasMedian || r3.Raw < 0.95 || r3.Median >= 0.80 {
		t.Errorf("R3: %+v", r3)
	}
	if len(days) != 1 || !days[0].day.Equal(time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)) ||
		!slices.Equal(hitIDs(days[0].hits), []string{"R1"}) {
		t.Errorf("day leads: %+v", days)
	}
	if len(histories) != 1 || !histories[0].Sufficient || histories[0].Commits != 26 || histories[0].WorkedDays != 26 {
		t.Errorf("history: %+v", histories[0])
	}
}

func TestRhythmInsufficientHistory(t *testing.T) {
	// 5 commits only: nothing triggers, the history is marked insufficient.
	views := append(regular(2, 5), view(2, "big", day0.AddDate(0, 0, 5), 900, 0, 30))
	commits, days, histories := rhythm(views, RhythmOptions{})
	if len(commits) != 0 || len(days) != 0 || len(histories) != 1 || histories[0].Sufficient {
		t.Errorf("short history: %+v %+v %+v", commits, days, histories)
	}
	// Zero MAD (all commits identical): R2 and R5 do not trigger.
	var same []store.CommitSignalView
	for i := range 25 {
		same = append(same, view(3, "s"+string(rune('a'+i)), day0.AddDate(0, 0, i), 10, 10, 1))
	}
	same = append(same, view(3, "big", day0.AddDate(0, 0, 25), 900, 900, 30))
	commits, _, _ = rhythm(same, RhythmOptions{})
	for _, c := range commits {
		if slices.ContainsFunc(c.hits, func(h Hit) bool { return h.Signal == "R2" || h.Signal == "R5" }) {
			t.Errorf("R2/R5 with a zero MAD: %+v", c.hits)
		}
	}
	// Commits without author are ignored; commits older than 90 days leave the window.
	old := append(regular(4, 25), view(4, "late", day0.AddDate(0, 0, 25+HistoryWindow+5), 400, 5, 10))
	commits, _, histories = rhythm(append(old, view(0, "anon", day0, 999, 0, 50)), RhythmOptions{})
	if len(commits) != 0 || len(histories) != 1 || histories[0].Sufficient {
		t.Errorf("window of 90 days: %+v %+v", commits, histories)
	}
}

func TestRhythmSpeed(t *testing.T) {
	views := []store.CommitSignalView{
		view(5, "a", day0, 10, 0, 1),
		view(5, "b", day0.Add(10*time.Minute), 200, 0, 2),                // 20 lines/min
		view(5, "c", day0.Add(10*time.Minute+30*time.Second), 300, 0, 2), // same minute: not measured
		view(5, "d", day0.Add(24*time.Hour), 500, 0, 2),                  // other day
	}
	commits, _, _ := rhythm(views, RhythmOptions{})
	if len(commits) != 0 {
		t.Errorf("R4 without speed_signal: %+v", commits)
	}
	commits, _, _ = rhythm(views, RhythmOptions{SpeedSignal: true})
	if len(commits) != 1 || commits[0].view.SHA != "b" || commits[0].hits[0].Signal != "R4" || commits[0].hits[0].Raw != 20 {
		t.Errorf("R4: %+v", commits)
	}
}

func TestRhythmRegimeBreak(t *testing.T) {
	var views []store.CommitSignalView
	for i := range 30 {
		views = append(views, view(6, "w"+string(rune('a'+i%26))+string(rune('a'+i/26)), day0.AddDate(0, 0, i), 8+i%5, 2, 1))
	}
	for i, d := range []int{39, 40, 41, 42} {
		views = append(views, view(6, "x"+string(rune('a'+i)), day0.AddDate(0, 0, d), 102, 2, 1))
	}
	_, days, _ := rhythm(views, RhythmOptions{})
	byDay := map[int][]string{}
	for _, d := range days {
		byDay[int(d.day.Sub(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)).Hours()/24)] = hitIDs(d.hits)
	}
	// Day 39: R1 (one big day). The 14-day window still holds days 27 to 29: its median jumps on day 41
	// (R6, first day of the break only), day 42 continues the break without a new R6.
	if !slices.Contains(byDay[39], "R1") || slices.Contains(byDay[39], "R6") || slices.Contains(byDay[40], "R6") {
		t.Errorf("days 39 and 40: %v", byDay)
	}
	if !slices.Contains(byDay[41], "R6") || slices.Contains(byDay[42], "R6") {
		t.Errorf("R6: day 41 %v, day 42 %v (%v)", byDay[41], byDay[42], byDay)
	}
}

func TestLevel(t *testing.T) {
	for _, tc := range []struct {
		hits []Hit
		want string
	}{
		{nil, ""},
		{[]Hit{{Signal: "R2"}}, LevelToReview},
		{[]Hit{{Signal: "R2"}, {Signal: "R5"}}, LevelToReview},
		{[]Hit{{Signal: "R2"}, {Signal: "D1"}}, LevelDeclared},
		{[]Hit{{Signal: "D5"}}, LevelDeclared},
		{[]Hit{{Signal: "D4"}}, ""},
		{[]Hit{{Signal: "F1"}}, ""}, // a form signal never raises a lead alone
		{[]Hit{{Signal: "F1"}, {Signal: "F3"}}, LevelToReview},
		{[]Hit{{Signal: "R2"}, {Signal: "F2"}}, LevelToReview},
		{[]Hit{{Signal: "R2"}, {Signal: "R5"}, {Signal: "F2"}}, LevelProbable},
		{[]Hit{{Signal: "R2"}, {Signal: "R5"}, {Signal: "F2"}, {Signal: "D2"}}, LevelDeclared},
	} {
		if got := Level(tc.hits); got != tc.want {
			t.Errorf("Level(%v) = %q, want %q", hitIDs(tc.hits), got, tc.want)
		}
	}
}
