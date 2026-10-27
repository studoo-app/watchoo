package activity

// Séries par semaine (graphique des contributeurs) et résumé d'une série par jour.

import (
	"slices"
	"testing"
	"time"
)

func TestWeeklyCounts(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")
	// now : mardi 27 octobre 2026, 10 h à Paris ; semaines du lundi 12, 19 et 26 octobre.
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	times := []time.Time{
		time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC), // lundi 26 à 0 h 30 à Paris (heure d'hiver) : semaine du 26
		time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC),  // lundi 26 à minuit : semaine du 26
		time.Date(2026, 10, 25, 22, 59, 0, 0, time.UTC), // dimanche 25 à 23 h 59 : semaine du 19
		time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),   // lundi 12 à 2 h : semaine du 12
		time.Date(2026, 10, 11, 21, 59, 0, 0, time.UTC), // dimanche 11 à 23 h 59 : hors fenêtre
		time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),    // semaine suivante : ignoré
	}
	got := WeeklyCounts(times, now, 3, paris)
	want := []WeekCount{{"2026-10-12", 1}, {"2026-10-19", 1}, {"2026-10-26", 2}}
	if !slices.Equal(got, want) {
		t.Errorf("WeeklyCounts = %v, attendu %v", got, want)
	}
	if start := WeeksStart(now, 3, paris); !start.Equal(time.Date(2026, 10, 11, 22, 0, 0, 0, time.UTC)) {
		t.Errorf("WeeksStart = %v, attendu 2026-10-11T22:00:00Z (lundi 12 à minuit à Paris)", start)
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
		{"dimanche soir", time.Date(2026, 11, 1, 23, 59, 0, 0, time.UTC), 2, "2026-10-19", "2026-10-26"},
		{"lundi minuit", time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), 1, "2026-10-26", "2026-10-26"},
		{"fin d'année", time.Date(2027, 1, 2, 12, 0, 0, 0, time.UTC), 2, "2026-12-21", "2026-12-28"},
		{"weeks < 1", time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC), 0, "2026-10-26", "2026-10-26"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			keys := WeekKeys(c.now, c.weeks, time.UTC)
			if keys[0] != c.first || keys[len(keys)-1] != c.last {
				t.Errorf("semaines %v, attendu %s … %s", keys, c.first, c.last)
			}
			for i := 1; i < len(keys); i++ {
				a, _ := time.Parse(time.DateOnly, keys[i-1])
				b, _ := time.Parse(time.DateOnly, keys[i])
				if b.Sub(a) != 7*24*time.Hour || b.Weekday() != time.Monday {
					t.Errorf("semaines non contiguës : %s puis %s", keys[i-1], keys[i])
				}
			}
			start := WeeksStart(c.now, c.weeks, time.UTC)
			if start.Format(time.DateOnly) != c.first || start.Hour() != 0 {
				t.Errorf("WeeksStart = %v, attendu %s à minuit", start, c.first)
			}
		})
	}
	// Fuseau très éloigné d'UTC : la semaine suit le fuseau d'affichage, pas UTC.
	kiri := mustLoad(t, "Pacific/Kiritimati")                // UTC+14
	sunday := time.Date(2026, 10, 25, 11, 0, 0, 0, time.UTC) // lundi 26 à 1 h à Kiritimati
	if got := WeekOf(sunday, kiri); got != "2026-10-26" {
		t.Errorf("WeekOf = %s, attendu 2026-10-26", got)
	}
	if got := WeekOf(sunday, time.UTC); got != "2026-10-19" {
		t.Errorf("WeekOf (UTC) = %s, attendu 2026-10-19", got)
	}
	if got := WeekOf(sunday, nil); got != WeekOf(sunday, time.Local) {
		t.Errorf("WeekOf(nil) = %s, attendu le fuseau local", got)
	}
}

func TestWeeklyCountsDST(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")
	// Semaine du passage à l'heure d'été (dimanche 29 mars 2026) : 7 jours de 167 h.
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	times := []time.Time{
		time.Date(2026, 3, 29, 21, 59, 0, 0, time.UTC), // dimanche 29 à 23 h 59 (été) : semaine du 23
		time.Date(2026, 3, 29, 22, 0, 0, 0, time.UTC),  // lundi 30 à minuit : semaine du 30
	}
	got := WeeklyCounts(times, now, 2, paris)
	want := []WeekCount{{"2026-03-23", 1}, {"2026-03-30", 1}}
	if !slices.Equal(got, want) {
		t.Errorf("WeeklyCounts = %v, attendu %v", got, want)
	}
}

func TestSummarize(t *testing.T) {
	total, peak := Summarize([]DayCount{{"2026-10-25", 2}, {"2026-10-26", 5}, {"2026-10-27", 5}})
	if total != 12 || peak != (DayCount{"2026-10-26", 5}) {
		t.Errorf("Summarize = %d, %v ; attendu 12, le 26 (premier pic)", total, peak)
	}
	if total, peak := Summarize(nil); total != 0 || peak != (DayCount{}) {
		t.Errorf("Summarize(nil) = %d, %v", total, peak)
	}
}
