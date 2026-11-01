package activity

// Weekly series (contributors chart) and summary of a daily series.

import (
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

func TestWeeklyCounts(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")
	// now: Tuesday October 27, 2026, 10 am in Paris; weeks of Monday 12, 19 and 26 October.
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	times := []time.Time{
		time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC), // Monday 26 at 0:30 in Paris (winter time): week of the 26th
		time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC),  // Monday 26 at midnight: week of the 26th
		time.Date(2026, 10, 25, 22, 59, 0, 0, time.UTC), // Sunday 25 at 11:59 pm: week of the 19th
		time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),   // Monday 12 at 2 am: week of the 12th
		time.Date(2026, 10, 11, 21, 59, 0, 0, time.UTC), // Sunday 11 at 11:59 pm: outside the window
		time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),    // next week: ignored
	}
	got := WeeklyCounts(times, now, 3, paris)
	want := []WeekCount{{"2026-10-12", 1}, {"2026-10-19", 1}, {"2026-10-26", 2}}
	if !slices.Equal(got, want) {
		t.Errorf("WeeklyCounts = %v, want %v", got, want)
	}
	if start := WeeksStart(now, 3, paris); !start.Equal(time.Date(2026, 10, 11, 22, 0, 0, 0, time.UTC)) {
		t.Errorf("WeeksStart = %v, want 2026-10-11T22:00:00Z (Monday 12 at midnight in Paris)", start)
	}
}

func TestWeeksBounds(t *testing.T) {
	cases := []struct {
		name  string
		now   time.Time
		weeks int
		first string
		last  string
	}{
		{"Sunday evening", time.Date(2026, 11, 1, 23, 59, 0, 0, time.UTC), 2, "2026-10-19", "2026-10-26"},
		{"Monday midnight", time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), 1, "2026-10-26", "2026-10-26"},
		{"end of year", time.Date(2027, 1, 2, 12, 0, 0, 0, time.UTC), 2, "2026-12-21", "2026-12-28"},
		{"weeks < 1", time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC), 0, "2026-10-26", "2026-10-26"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			keys := WeekKeys(c.now, c.weeks, time.UTC)
			if keys[0] != c.first || keys[len(keys)-1] != c.last {
				t.Errorf("weeks %v, want %s … %s", keys, c.first, c.last)
			}
			for i := 1; i < len(keys); i++ {
				a, _ := time.Parse(time.DateOnly, keys[i-1])
				b, _ := time.Parse(time.DateOnly, keys[i])
				if b.Sub(a) != 7*24*time.Hour || b.Weekday() != time.Monday {
					t.Errorf("non-contiguous weeks: %s then %s", keys[i-1], keys[i])
				}
			}
			start := WeeksStart(c.now, c.weeks, time.UTC)
			if start.Format(time.DateOnly) != c.first || start.Hour() != 0 {
				t.Errorf("WeeksStart = %v, want %s at midnight", start, c.first)
			}
		})
	}
	// Time zone very far from UTC: the week follows the display time zone, not UTC.
	kiri := mustLoad(t, "Pacific/Kiritimati")                // UTC+14
	sunday := time.Date(2026, 10, 25, 11, 0, 0, 0, time.UTC) // Monday 26 at 1 am in Kiritimati
	if got := WeekOf(sunday, kiri); got != "2026-10-26" {
		t.Errorf("WeekOf = %s, want 2026-10-26", got)
	}
	if got := WeekOf(sunday, time.UTC); got != "2026-10-19" {
		t.Errorf("WeekOf (UTC) = %s, want 2026-10-19", got)
	}
	if got := WeekOf(sunday, nil); got != WeekOf(sunday, time.Local) {
		t.Errorf("WeekOf(nil) = %s, want the local time zone", got)
	}
}

func TestWeeklyCountsDST(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")
	// Week of the switch to summer time (Sunday March 29, 2026): 7 days of 167 h.
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	times := []time.Time{
		time.Date(2026, 3, 29, 21, 59, 0, 0, time.UTC), // Sunday 29 at 11:59 pm (summer): week of the 23rd
		time.Date(2026, 3, 29, 22, 0, 0, 0, time.UTC),  // Monday 30 at midnight: week of the 30th
	}
	got := WeeklyCounts(times, now, 2, paris)
	want := []WeekCount{{"2026-03-23", 1}, {"2026-03-30", 1}}
	if !slices.Equal(got, want) {
		t.Errorf("WeeklyCounts = %v, want %v", got, want)
	}
}

func TestSummarize(t *testing.T) {
	total, peak := Summarize([]DayCount{{"2026-10-25", 2}, {"2026-10-26", 5}, {"2026-10-27", 5}})
	if total != 12 || peak != (DayCount{"2026-10-26", 5}) {
		t.Errorf("Summarize = %d, %v; want 12, on the 26th (first peak)", total, peak)
	}
	if total, peak := Summarize(nil); total != 0 || peak != (DayCount{}) {
		t.Errorf("Summarize(nil) = %d, %v", total, peak)
	}
}

func TestCountContributorWeeks(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC) // Tuesday October 27, weeks of the 12th, 19th and 26th
	at := func(id int64, tm time.Time) model.AuthorCommitTime {
		return model.AuthorCommitTime{ContributorID: id, CommittedAt: tm}
	}
	times := []model.AuthorCommitTime{
		at(1, time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC)), // Monday 26 in Paris: week of the 26th
		at(1, time.Date(2026, 10, 25, 22, 59, 0, 0, time.UTC)), // Sunday 25 in Paris: week of the 19th
		at(2, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)),   // week of the 12th
		at(2, time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)),   // week of the 12th
		at(3, time.Date(2026, 10, 11, 21, 59, 0, 0, time.UTC)), // outside the window
		at(1, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)),    // next week: ignored
	}
	c := CountContributorWeeks(times, now, 3, paris)
	if want := []string{"2026-10-12", "2026-10-19", "2026-10-26"}; !slices.Equal(c.Weeks, want) {
		t.Errorf("Weeks = %v, want %v", c.Weeks, want)
	}
	if c.Total != 4 || len(c.Counts) != 2 {
		t.Errorf("Total %d, %d contributors; want 4 and 2 (%v)", c.Total, len(c.Counts), c.Counts)
	}
	if got := c.Of(1); !slices.Equal(got, []int{0, 1, 1}) || c.TotalOf(1) != 2 {
		t.Errorf("contributor 1: %v (total %d), want [0 1 1]", got, c.TotalOf(1))
	}
	if got := c.Of(2); !slices.Equal(got, []int{2, 0, 0}) {
		t.Errorf("contributor 2: %v, want [2 0 0]", got)
	}
	if got := c.Of(3); !slices.Equal(got, []int{0, 0, 0}) || c.TotalOf(3) != 0 {
		t.Errorf("contributor outside the window: %v, want zeros", got)
	}
	// Of returns a copy.
	c.Of(1)[0] = 99
	if c.Counts[1][0] != 0 {
		t.Error("Of returned the internal slice")
	}

	empty := CountContributorWeeks(nil, now, 0, nil)
	if len(empty.Weeks) != 1 || empty.Total != 0 || len(empty.Counts) != 0 || len(empty.Of(1)) != 1 {
		t.Errorf("without commit: %+v", empty)
	}
}

func TestCountRepoWeeks(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	at := func(repo int64, tm time.Time) model.RepoCommitTime {
		return model.RepoCommitTime{RepoID: repo, CommittedAt: tm}
	}
	times := []model.RepoCommitTime{
		at(7, time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC)), // Monday 26 in Paris
		at(7, time.Date(2026, 10, 25, 22, 59, 0, 0, time.UTC)), // Sunday 25 in Paris
		at(9, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)),
		at(9, time.Date(2026, 10, 11, 21, 59, 0, 0, time.UTC)), // outside the window
	}
	c := CountRepoWeeks(times, now, 3, paris)
	if c.Total != 3 || len(c.Counts) != 2 {
		t.Fatalf("Total %d, %d repositories; want 3 and 2 (%v)", c.Total, len(c.Counts), c.Counts)
	}
	if got := c.Of(7); !slices.Equal(got, []int{0, 1, 1}) {
		t.Errorf("repository 7: %v, want [0 1 1]", got)
	}
	if got := c.Of(9); !slices.Equal(got, []int{1, 0, 0}) {
		t.Errorf("repository 9: %v, want [1 0 0]", got)
	}
	if empty := CountRepoWeeks(nil, now, 2, paris); empty.Total != 0 || len(empty.Weeks) != 2 {
		t.Errorf("no commit: %+v", empty)
	}
}
