package activity

// Bornes de la série d'activité (graphique de la page de détail d'un dépôt) : minuit, changements
// d'heure, fin d'année, fuseaux éloignés d'UTC. Écrit par l'agent repo de la phase 4.

import (
	"testing"
	"time"
)

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("fuseau %s indisponible : %v", name, err)
	}
	return loc
}

// checkSeries vérifie les bornes, la continuité (un jour par entrée, sans trou ni doublon) et la
// cohérence avec SeriesStart.
func checkSeries(t *testing.T, got []DayCount, days int, first, last string, now time.Time, loc *time.Location) {
	t.Helper()
	if len(got) != days {
		t.Fatalf("%d jours, attendu %d", len(got), days)
	}
	if got[0].Date != first || got[days-1].Date != last {
		t.Errorf("bornes %s … %s, attendu %s … %s", got[0].Date, got[days-1].Date, first, last)
	}
	for i := 1; i < days; i++ {
		prev, _ := time.Parse(time.DateOnly, got[i-1].Date)
		cur, _ := time.Parse(time.DateOnly, got[i].Date)
		if cur.Sub(prev) != 24*time.Hour {
			t.Errorf("jours %s puis %s : pas consécutifs", got[i-1].Date, got[i].Date)
		}
	}
	if start := SeriesStart(now, days, loc).In(loc); start.Format(time.DateOnly) != first ||
		start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Errorf("SeriesStart = %v dans %v, attendu minuit du %s", start, loc, first)
	}
}

func TestDailyCountsMidnightBounds(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")
	// now : 27/10/2026 à 23 h 59 59 à Paris (22 h 59 59 UTC).
	now := time.Date(2026, 10, 27, 22, 59, 59, 0, time.UTC)
	start := SeriesStart(now, 2, paris)
	times := []time.Time{
		start.Add(-time.Nanosecond), // 25/10 à 23 h 59 59,999… : hors fenêtre
		start,                       // 26/10 à 0 h : premier jour, compté
		now,                         // 27/10 à 23 h 59 59 : dernier jour
		now.Add(time.Second),        // 28/10 à 0 h à Paris : demain, ignoré
	}
	got := DailyCounts(times, now, 2, paris)
	checkSeries(t, got, 2, "2026-10-26", "2026-10-27", now, paris)
	if got[0].Count != 1 || got[1].Count != 1 {
		t.Errorf("DailyCounts = %v, attendu 1 commit chaque jour", got)
	}

	// Même instants vus d'UTC : la fenêtre change de jours.
	utc := DailyCounts(times, now, 2, time.UTC)
	checkSeries(t, utc, 2, "2026-10-26", "2026-10-27", now, time.UTC)
	if utc[0].Count != 0 || utc[1].Count != 2 {
		t.Errorf("en UTC : %v, attendu 0 puis 2", utc)
	}
}

func TestDailyCountsDSTSpring(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")
	// Passage à l'heure d'été le 29/03/2026 (journée de 23 h). now : 30/03 à 12 h à Paris.
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	times := []time.Time{
		time.Date(2026, 3, 28, 23, 0, 0, 0, time.UTC),  // 29/03 à 0 h (heure d'hiver)
		time.Date(2026, 3, 29, 21, 59, 0, 0, time.UTC), // 29/03 à 23 h 59 (heure d'été)
		time.Date(2026, 3, 29, 22, 0, 0, 0, time.UTC),  // 30/03 à 0 h
	}
	got := DailyCounts(times, now, 3, paris)
	checkSeries(t, got, 3, "2026-03-28", "2026-03-30", now, paris)
	want := []int{0, 2, 1}
	for i, n := range want {
		if got[i].Count != n {
			t.Errorf("%s : %d, attendu %d", got[i].Date, got[i].Count, n)
		}
	}
	if s := SeriesStart(now, 3, paris); !s.Equal(time.Date(2026, 3, 27, 23, 0, 0, 0, time.UTC)) {
		t.Errorf("SeriesStart = %v, attendu 2026-03-27T23:00:00Z", s)
	}
}

func TestDailyCountsYearBoundaryAndFarZones(t *testing.T) {
	kiritimati := mustLoad(t, "Pacific/Kiritimati") // UTC+14
	pagoPago := mustLoad(t, "Pacific/Pago_Pago")    // UTC-11
	// now : 1er janvier 2027 à 9 h UTC = 1er janvier 23 h à Kiritimati, 31 décembre 22 h à Pago Pago.
	now := time.Date(2027, 1, 1, 9, 0, 0, 0, time.UTC)
	commit := time.Date(2026, 12, 31, 10, 30, 0, 0, time.UTC) // 1er janvier 0 h 30 à Kiritimati

	k := DailyCounts([]time.Time{commit}, now, ChartDays, kiritimati)
	checkSeries(t, k, ChartDays, "2026-10-04", "2027-01-01", now, kiritimati)
	if countIn(k, "2027-01-01") != 1 || countIn(k, "2026-12-31") != 0 {
		t.Errorf("Kiritimati : commit mal placé")
	}

	p := DailyCounts([]time.Time{commit}, now, ChartDays, pagoPago)
	checkSeries(t, p, ChartDays, "2026-10-03", "2026-12-31", now, pagoPago)
	if countIn(p, "2026-12-30") != 1 {
		t.Errorf("Pago Pago : commit attendu le 30/12 (23 h 30 locale)")
	}
}

func TestDailyCountsNilLocation(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	got := DailyCounts([]time.Time{now}, now, 1, nil)
	if len(got) != 1 || got[0].Date != now.In(time.Local).Format(time.DateOnly) || got[0].Count != 1 {
		t.Errorf("fuseau nil (time.Local) : %v", got)
	}
	if !SeriesStart(now, 1, nil).Equal(SeriesStart(now, 1, time.Local)) {
		t.Error("SeriesStart avec nil ≠ time.Local")
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
