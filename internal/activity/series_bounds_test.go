package activity

// Bounds of the activity series (chart of a repository's detail page): midnight, clock changes, end
// of year, time zones far from UTC. Written by the repo agent of phase 4.

import (
	"testing"
	"time"
)

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("time zone %s unavailable: %v", name, err)
	}
	return loc
}

// checkSeries checks the bounds, the continuity (one day per entry, without gap or duplicate) and
// the consistency with SeriesStart.
func checkSeries(t *testing.T, got []DayCount, days int, first, last string, now time.Time, loc *time.Location) {
	t.Helper()
	if len(got) != days {
		t.Fatalf("%d days, want %d", len(got), days)
	}
	if got[0].Date != first || got[days-1].Date != last {
		t.Errorf("bounds %s … %s, want %s … %s", got[0].Date, got[days-1].Date, first, last)
	}
	for i := 1; i < days; i++ {
		prev, _ := time.Parse(time.DateOnly, got[i-1].Date)
		cur, _ := time.Parse(time.DateOnly, got[i].Date)
		if cur.Sub(prev) != 24*time.Hour {
			t.Errorf("days %s then %s: not consecutive", got[i-1].Date, got[i].Date)
		}
	}
	if start := SeriesStart(now, days, loc).In(loc); start.Format(time.DateOnly) != first ||
		start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Errorf("SeriesStart = %v in %v, want midnight of %s", start, loc, first)
	}
}

func TestDailyCountsMidnightBounds(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")
	// now: 10/27/2026 at 11:59:59 pm in Paris (10:59:59 pm UTC).
	now := time.Date(2026, 10, 27, 22, 59, 59, 0, time.UTC)
	start := SeriesStart(now, 2, paris)
	times := []time.Time{
		start.Add(-time.Nanosecond), // 10/25 at 11:59:59.999… pm: outside the window
		start,                       // 10/26 at 0:00: first day, counted
		now,                         // 10/27 at 11:59:59 pm: last day
		now.Add(time.Second),        // 10/28 at 0:00 in Paris: tomorrow, ignored
	}
	got := DailyCounts(times, now, 2, paris)
	checkSeries(t, got, 2, "2026-10-26", "2026-10-27", now, paris)
	if got[0].Count != 1 || got[1].Count != 1 {
		t.Errorf("DailyCounts = %v, want 1 commit every day", got)
	}

	// Same instants seen from UTC: the window changes days.
	utc := DailyCounts(times, now, 2, time.UTC)
	checkSeries(t, utc, 2, "2026-10-26", "2026-10-27", now, time.UTC)
	if utc[0].Count != 0 || utc[1].Count != 2 {
		t.Errorf("in UTC: %v, want 0 then 2", utc)
	}
}

func TestDailyCountsDSTSpring(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")
	// Switch to summer time on 03/29/2026 (23-hour day). now: 03/30 at noon in Paris.
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	times := []time.Time{
		time.Date(2026, 3, 28, 23, 0, 0, 0, time.UTC),  // 03/29 at 0:00 (winter time)
		time.Date(2026, 3, 29, 21, 59, 0, 0, time.UTC), // 03/29 at 11:59 pm (summer time)
		time.Date(2026, 3, 29, 22, 0, 0, 0, time.UTC),  // 03/30 at 0:00
	}
	got := DailyCounts(times, now, 3, paris)
	checkSeries(t, got, 3, "2026-03-28", "2026-03-30", now, paris)
	want := []int{0, 2, 1}
	for i, n := range want {
		if got[i].Count != n {
			t.Errorf("%s: %d, want %d", got[i].Date, got[i].Count, n)
		}
	}
	if s := SeriesStart(now, 3, paris); !s.Equal(time.Date(2026, 3, 27, 23, 0, 0, 0, time.UTC)) {
		t.Errorf("SeriesStart = %v, want 2026-03-27T23:00:00Z", s)
	}
}

func TestDailyCountsYearBoundaryAndFarZones(t *testing.T) {
	kiritimati := mustLoad(t, "Pacific/Kiritimati") // UTC+14
	pagoPago := mustLoad(t, "Pacific/Pago_Pago")    // UTC-11
	// now: January 1, 2027 at 9 am UTC = January 1 at 11 pm in Kiritimati, December 31 at 10 pm in Pago Pago.
	now := time.Date(2027, 1, 1, 9, 0, 0, 0, time.UTC)
	commit := time.Date(2026, 12, 31, 10, 30, 0, 0, time.UTC) // January 1 at 0:30 in Kiritimati

	k := DailyCounts([]time.Time{commit}, now, ChartDays, kiritimati)
	checkSeries(t, k, ChartDays, "2026-10-04", "2027-01-01", now, kiritimati)
	if countIn(k, "2027-01-01") != 1 || countIn(k, "2026-12-31") != 0 {
		t.Errorf("Kiritimati: commit misplaced")
	}

	p := DailyCounts([]time.Time{commit}, now, ChartDays, pagoPago)
	checkSeries(t, p, ChartDays, "2026-10-03", "2026-12-31", now, pagoPago)
	if countIn(p, "2026-12-30") != 1 {
		t.Errorf("Pago Pago: want the commit on 12/30 (11:30 pm local)")
	}
}

func TestDailyCountsNilLocation(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	got := DailyCounts([]time.Time{now}, now, 1, nil)
	if len(got) != 1 || got[0].Date != now.In(time.Local).Format(time.DateOnly) || got[0].Count != 1 {
		t.Errorf("nil time zone (time.Local): %v", got)
	}
	if !SeriesStart(now, 1, nil).Equal(SeriesStart(now, 1, time.Local)) {
		t.Error("SeriesStart with nil ≠ time.Local")
	}
}

func countIn(days []DayCount, date string) int {
	for _, d := range days {
		if d.Date == date {
			return d.Count
		}
	}
	return -1
}
