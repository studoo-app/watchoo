package activity

import (
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
)

func TestShort(t *testing.T) {
	tests := []struct {
		res      Result
		unicode  string
		asciiOut string
	}{
		{Result{State: StateActive, Days: 2}, "-", "-"},
		{Result{State: StateFlemme, Days: 9}, "~ 9 d", "~ 9 d"},
		{Result{State: StateDead, Days: 42}, "⚠ 42 d", "! 42 d"},
		{Result{State: StateStopped, Days: 3}, "stopped", "stopped"},
		{Result{State: StateError, Days: -1}, "error", "error"},
		{Result{State: StateNeverSynced, Days: -1}, "never", "never"},
		{Result{State: StateArchived, Days: 80}, "archived", "archived"},
		{Result{State: StateEmpty, Days: -1}, "empty", "empty"},
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
		{"zéro", time.Time{}, true, "never"},
		{"à l'instant", now.Add(-30 * time.Second), true, "just now"},
		{"à l'instant ascii", now.Add(-30 * time.Second), false, "just now"},
		{"futur", now.Add(time.Hour), true, "just now"},
		{"minutes", now.Add(-5 * time.Minute), true, "5 min ago"},
		{"59 min", now.Add(-59*time.Minute - 59*time.Second), true, "59 min ago"},
		{"heures", now.Add(-2*time.Hour - 10*time.Minute), true, "2 h ago"},
		{"23 h, la veille", now.Add(-23 * time.Hour), true, "23 h ago"},
		{"jours", now.AddDate(0, 0, -9), true, "9 d ago"},
		{"24 h juste", now.Add(-24 * time.Hour), true, "1 d ago"},
		{"34 h sur deux minuits", time.Date(2026, 3, 18, 23, 30, 0, 0, time.UTC), true, "2 d ago"},
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
	if res.Short(true) != "~ 9 d" {
		t.Fatalf("Short = %q, attendu « ~ 9 d »", res.Short(true))
	}
	if got := Relative(commit, now, paris, true); got != "9 d ago" {
		t.Errorf("Relative = %q, attendu « 9 d ago »", got)
	}
}

// TestFrench vérifie les textes français des états et des dates relatives : l'astuce unicode de
// Short (variante sans accent quand le terminal n'affiche pas d'Unicode) et les formats « il y a ».
func TestFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()
	paris := mustLoc(t, "Europe/Paris")
	commit := time.Date(2026, 3, 10, 23, 50, 0, 0, paris)
	now := time.Date(2026, 3, 19, 0, 5, 0, 0, paris)
	res := Evaluate(syncedRepo(commit), syncedRunOK(), defaultCfg, now, paris)
	if got := res.Short(true); got != "~ 9 j" {
		t.Errorf("Short(true) = %q, attendu « ~ 9 j »", got)
	}
	if got := res.Short(false); got != "~ 9 j" {
		t.Errorf("Short(false) = %q, attendu « ~ 9 j »", got)
	}
	stopped := syncedRepo(commit)
	stopped.Enabled = false
	stoppedRes := Evaluate(stopped, syncedRunOK(), defaultCfg, now, paris)
	if got := stoppedRes.Short(true); got != "arrêté" {
		t.Errorf("Short(true) = %q, attendu « arrêté »", got)
	}
	if got := stoppedRes.Short(false); got != "arrete" {
		t.Errorf("Short(false) = %q, attendu « arrete »", got)
	}
	if got := StateNeverSynced.Label(); got != "Jamais synchronisé" {
		t.Errorf("Label = %q, attendu « Jamais synchronisé »", got)
	}
	if got := Relative(commit, now, paris, true); got != "il y a 9 j" {
		t.Errorf("Relative = %q, attendu « il y a 9 j »", got)
	}
	if got := Relative(time.Time{}, now, paris, true); got != "jamais" {
		t.Errorf("Relative(zéro) = %q, attendu « jamais »", got)
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
