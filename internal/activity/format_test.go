package activity

import (
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

func TestShort(t *testing.T) {
	tests := []struct {
		res      Result
		unicode  string
		asciiOut string
	}{
		{Result{State: StateActive, Days: 2}, "-", "-"},
		{Result{State: StateFlemme, Days: 9}, "~ 9 j", "~ 9 j"},
		{Result{State: StateDead, Days: 42}, "⚠ 42 j", "! 42 j"},
		{Result{State: StateStopped, Days: 3}, "arrêté", "arrete"},
		{Result{State: StateError, Days: -1}, "erreur", "erreur"},
		{Result{State: StateNeverSynced, Days: -1}, "jamais", "jamais"},
		{Result{State: StateArchived, Days: 80}, "archivé", "archive"},
		{Result{State: StateEmpty, Days: -1}, "vide", "vide"},
	}
	for _, tt := range tests {
		if got := tt.res.Short(true); got != tt.unicode {
			t.Errorf("%s Short(true) = %q, attendu %q", tt.res.State, got, tt.unicode)
		}
		if got := tt.res.Short(false); got != tt.asciiOut {
			t.Errorf("%s Short(false) = %q, attendu %q", tt.res.State, got, tt.asciiOut)
		}
	}
}

func TestRelative(t *testing.T) {
	now := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		t       time.Time
		unicode bool
		want    string
	}{
		{"zéro", time.Time{}, true, "jamais"},
		{"à l'instant", now.Add(-30 * time.Second), true, "à l'instant"},
		{"à l'instant ascii", now.Add(-30 * time.Second), false, "a l'instant"},
		{"futur", now.Add(time.Hour), true, "à l'instant"},
		{"minutes", now.Add(-5 * time.Minute), true, "il y a 5 min"},
		{"59 min", now.Add(-59*time.Minute - 59*time.Second), true, "il y a 59 min"},
		{"heures", now.Add(-2*time.Hour - 10*time.Minute), true, "il y a 2 h"},
		{"23 h, la veille", now.Add(-23 * time.Hour), true, "il y a 23 h"},
		{"jours", now.AddDate(0, 0, -9), true, "il y a 9 j"},
		{"24 h juste", now.Add(-24 * time.Hour), true, "il y a 1 j"},
		{"34 h sur deux minuits", time.Date(2026, 3, 18, 23, 30, 0, 0, time.UTC), true, "il y a 2 j"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Relative(tt.t, now, time.UTC, tt.unicode); got != tt.want {
				t.Errorf("Relative = %q, attendu %q", got, tt.want)
			}
		})
	}
}

func TestRelativeConcordsWithShort(t *testing.T) {
	paris := mustLoc(t, "Europe/Paris")
	commit := time.Date(2026, 3, 10, 23, 50, 0, 0, paris)
	now := time.Date(2026, 3, 19, 0, 5, 0, 0, paris)
	res := Evaluate(syncedRepo(commit), syncedRunOK(), defaultCfg, now, paris)
	if res.Short(true) != "~ 9 j" {
		t.Fatalf("Short = %q, attendu « ~ 9 j »", res.Short(true))
	}
	if got := Relative(commit, now, paris, true); got != "il y a 9 j" {
		t.Errorf("Relative = %q, attendu « il y a 9 j »", got)
	}
}

func TestParseSince(t *testing.T) {
	ok := map[string]time.Duration{
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
		"12h": 12 * time.Hour,
		"30m": 30 * time.Minute,
		"1m":  time.Minute,
	}
	for in, want := range ok {
		got, err := ParseSince(in)
		if err != nil || got != want {
			t.Errorf("ParseSince(%q) = %v, %v ; attendu %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "d", "7", "0d", "-3d", "+3d", "7 d", " 7d", "1.5h", "7w", "7D", "3s", "abc",
		"99999999999999999999d", "200000000d"} {
		_, err := ParseSince(in)
		if err == nil {
			t.Errorf("ParseSince(%q) : erreur attendue", in)
			continue
		}
		if !strings.Contains(err.Error(), "7d") || !strings.Contains(err.Error(), "--since") {
			t.Errorf("ParseSince(%q) : message peu clair : %v", in, err)
		}
	}
}

// syncedRunOK renvoie un sync_run réussi.
func syncedRunOK() model.SyncRun { return model.SyncRun{ID: 1, Status: model.SyncSuccess} }

func TestSparkline(t *testing.T) {
	tests := []struct {
		counts  []int
		unicode bool
		want    string
	}{
		{nil, true, ""},
		{[]int{0, 0, 0}, true, "▁▁▁"},
		{[]int{0, 1, 7, 3, 100}, true, "▁▂▂▂█"},
		{[]int{1, 2, 3, 4, 5, 6, 7}, true, "▂▃▄▅▆▇█"},
		{[]int{0, 1, 2, 3}, false, ".-=#"},
		{[]int{0, 0}, false, ".."},
		{[]int{5}, false, "#"},
	}
	for _, tt := range tests {
		if got := Sparkline(tt.counts, tt.unicode); got != tt.want {
			t.Errorf("Sparkline(%v, %v) = %q, attendu %q", tt.counts, tt.unicode, got, tt.want)
		}
	}
}
