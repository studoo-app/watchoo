package activity

import (
	"testing"
	"time"
)

func TestDailyCounts(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skipf("fuseau Europe/Paris indisponible : %v", err)
	}
	// now : 27 octobre 2026, 10 h à Paris (passage à l'heure d'hiver le 25 octobre).
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	times := []time.Time{
		time.Date(2026, 10, 26, 22, 30, 0, 0, time.UTC), // 26 à 23 h 30 à Paris
		time.Date(2026, 10, 26, 23, 30, 0, 0, time.UTC), // 27 à 0 h 30 à Paris
		time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC),   // 27
		time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC),  // 25, jour du changement d'heure
		time.Date(2026, 10, 24, 21, 59, 0, 0, time.UTC), // 24 à 23 h 59 (heure d'été) : hors fenêtre
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC),   // futur : ignoré
	}
	got := DailyCounts(times, now, 3, paris)
	want := []DayCount{{"2026-10-25", 1}, {"2026-10-26", 1}, {"2026-10-27", 2}}
	if len(got) != len(want) {
		t.Fatalf("DailyCounts = %v, attendu %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("jour %d : %v, attendu %v", i, got[i], want[i])
		}
	}

	if start := SeriesStart(now, 3, paris); !start.Equal(time.Date(2026, 10, 24, 22, 0, 0, 0, time.UTC)) {
		t.Errorf("SeriesStart = %v, attendu 2026-10-24T22:00:00Z (minuit du 25 à Paris)", start)
	}
}

func TestDailyCountsDefaults(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	got := DailyCounts(nil, now, ChartDays, time.UTC)
	if len(got) != ChartDays {
		t.Fatalf("%d jours, attendu %d", len(got), ChartDays)
	}
	if got[0].Date != "2025-12-02" || got[ChartDays-1].Date != "2026-03-01" {
		t.Errorf("bornes %s … %s", got[0].Date, got[ChartDays-1].Date)
	}
	for _, d := range got {
		if d.Count != 0 {
			t.Errorf("jour %s : %d commits, attendu 0", d.Date, d.Count)
		}
	}
	if one := DailyCounts(nil, now, 0, nil); len(one) != 1 {
		t.Errorf("days 0 : %d jours, attendu 1", len(one))
	}
	if !SeriesStart(now, ChartDays, time.UTC).Equal(time.Date(2025, 12, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("SeriesStart = %v", SeriesStart(now, ChartDays, time.UTC))
	}
}
