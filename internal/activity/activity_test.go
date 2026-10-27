package activity

import (
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/workspace"
)

func intPtr(n int) *int { return &n }

var defaultCfg = workspace.ActivityConfig{FlemmeAfterDays: 5, DeadAfterDays: 15}

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("fuseau %s indisponible : %v", name, err)
	}
	return loc
}

// syncedRepo renvoie un dépôt suivi, synchronisé, dont le dernier commit date de lastCommit.
func syncedRepo(lastCommit time.Time) model.Repository {
	return model.Repository{
		ID: 1, Owner: "alice", Name: "projet", Enabled: true,
		LastSyncedAt: lastCommit.Add(time.Hour), LastCommitAt: lastCommit,
	}
}

func TestThresholdsFor(t *testing.T) {
	tests := []struct {
		name string
		repo model.Repository
		cfg  workspace.ActivityConfig
		want Thresholds
	}{
		{"globaux", model.Repository{}, defaultCfg, Thresholds{5, 15}},
		{"propres au dépôt", model.Repository{FlemmeAfterDays: intPtr(3), DeadAfterDays: intPtr(10)}, defaultCfg, Thresholds{3, 10}},
		{"flemme du dépôt seulement", model.Repository{FlemmeAfterDays: intPtr(7)}, defaultCfg, Thresholds{7, 15}},
		{"dead du dépôt seulement", model.Repository{DeadAfterDays: intPtr(30)}, defaultCfg, Thresholds{5, 30}},
		{"mélange incohérent corrigé", model.Repository{FlemmeAfterDays: intPtr(20)}, defaultCfg, Thresholds{20, 21}},
		{"dead du dépôt trop petit", model.Repository{DeadAfterDays: intPtr(2)}, defaultCfg, Thresholds{5, 6}},
		{"valeurs nulles ignorées", model.Repository{FlemmeAfterDays: intPtr(0), DeadAfterDays: intPtr(-1)}, defaultCfg, Thresholds{5, 15}},
		{"configuration vide : défauts", model.Repository{}, workspace.ActivityConfig{}, Thresholds{workspace.DefaultFlemmeAfterDays, workspace.DefaultDeadAfterDays}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ThresholdsFor(tt.repo, tt.cfg); got != tt.want {
				t.Errorf("ThresholdsFor = %+v, attendu %+v", got, tt.want)
			}
		})
	}
}

func TestEvaluateBoundaries(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		days int
		want State
	}{
		{0, StateActive},
		{1, StateActive},
		{5, StateActive},
		{6, StateFlemme},
		{15, StateFlemme},
		{16, StateDead},
		{42, StateDead},
	}
	for _, tt := range tests {
		repo := syncedRepo(now.AddDate(0, 0, -tt.days))
		got := Evaluate(repo, model.SyncRun{}, defaultCfg, now, loc)
		if got.State != tt.want || got.Days != tt.days {
			t.Errorf("%d jours : état %s (%d j), attendu %s (%d j)", tt.days, got.State, got.Days, tt.want, tt.days)
		}
		if got.Thresholds != (Thresholds{5, 15}) {
			t.Errorf("%d jours : seuils %+v", tt.days, got.Thresholds)
		}
	}
}

func TestEvaluateRepoThresholds(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	repo := syncedRepo(now.AddDate(0, 0, -4))
	repo.FlemmeAfterDays = intPtr(3)
	repo.DeadAfterDays = intPtr(4)
	if got := Evaluate(repo, model.SyncRun{}, defaultCfg, now, time.UTC); got.State != StateFlemme {
		t.Errorf("4 j avec seuils 3/4 : %s, attendu flemme", got.State)
	}
	repo.LastCommitAt = now.AddDate(0, 0, -5)
	if got := Evaluate(repo, model.SyncRun{}, defaultCfg, now, time.UTC); got.State != StateDead {
		t.Errorf("5 j avec seuils 3/4 : %s, attendu dead", got.State)
	}
}

func TestEvaluateTimezoneMidnight(t *testing.T) {
	paris := mustLoc(t, "Europe/Paris")
	// Commit le 14 mars à 23 h 30 à Paris ; maintenant le 20 mars à 0 h 10 à Paris : 40 minutes de plus
	// que 5 jours, mais 6 jours calendaires → Flemme.
	commit := time.Date(2026, 3, 14, 23, 30, 0, 0, paris)
	now := time.Date(2026, 3, 20, 0, 10, 0, 0, paris)
	got := Evaluate(syncedRepo(commit), model.SyncRun{}, defaultCfg, now, paris)
	if got.Days != 6 || got.State != StateFlemme {
		t.Errorf("Paris : %d j %s, attendu 6 j flemme", got.Days, got.State)
	}
	// Mêmes instants vus en UTC (22 h 30 le 14, 23 h 10 le 19) : 5 jours → Actif.
	got = Evaluate(syncedRepo(commit), model.SyncRun{}, defaultCfg, now, time.UTC)
	if got.Days != 5 || got.State != StateActive {
		t.Errorf("UTC : %d j %s, attendu 5 j active", got.Days, got.State)
	}
}

func TestEvaluateFutureCommit(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	got := Evaluate(syncedRepo(now.Add(48*time.Hour)), model.SyncRun{}, defaultCfg, now, time.UTC)
	if got.Days != 0 || got.State != StateActive {
		t.Errorf("commit futur : %d j %s, attendu 0 j active", got.Days, got.State)
	}
}

func TestEvaluateGrayStates(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -40)
	errRun := model.SyncRun{ID: 9, Status: model.SyncError, Error: "boum"}
	okRun := model.SyncRun{ID: 9, Status: model.SyncSuccess}

	tests := []struct {
		name     string
		mod      func(r *model.Repository)
		run      model.SyncRun
		want     State
		wantDays int
	}{
		{"arrêté prime sur tout", func(r *model.Repository) { r.Enabled = false; r.Archived = true }, errRun, StateStopped, 40},
		{"erreur prime sur archivé", func(r *model.Repository) { r.Archived = true }, errRun, StateError, 40},
		{"erreur sans synchro réussie", func(r *model.Repository) { r.LastSyncedAt = time.Time{}; r.LastCommitAt = time.Time{} }, errRun, StateError, -1},
		{"jamais synchronisé", func(r *model.Repository) { r.LastSyncedAt = time.Time{}; r.LastCommitAt = time.Time{} }, model.SyncRun{}, StateNeverSynced, -1},
		{"synchro en cours, jamais réussie", func(r *model.Repository) { r.LastSyncedAt = time.Time{}; r.LastCommitAt = time.Time{} },
			model.SyncRun{ID: 3, Status: model.SyncRunning}, StateNeverSynced, -1},
		{"archivé, jamais Dead", func(r *model.Repository) { r.Archived = true }, okRun, StateArchived, 40},
		{"vide", func(r *model.Repository) { r.LastCommitAt = time.Time{} }, okRun, StateEmpty, -1},
		{"dead normal", func(r *model.Repository) {}, okRun, StateDead, 40},
		{"run d'ID 0 ignoré", func(r *model.Repository) {}, model.SyncRun{Status: model.SyncError}, StateDead, 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := syncedRepo(old)
			tt.mod(&repo)
			got := Evaluate(repo, tt.run, defaultCfg, now, time.UTC)
			if got.State != tt.want || got.Days != tt.wantDays {
				t.Errorf("état %s (%d j), attendu %s (%d j)", got.State, got.Days, tt.want, tt.wantDays)
			}
			if got.State.Gray() != (tt.want != StateActive && tt.want != StateFlemme && tt.want != StateDead) {
				t.Errorf("Gray() = %v pour %s", got.State.Gray(), got.State)
			}
		})
	}
}

func TestStateLabelAndGray(t *testing.T) {
	want := map[State]struct {
		label string
		gray  bool
	}{
		StateActive:      {"Actif", false},
		StateFlemme:      {"Flemme", false},
		StateDead:        {"Dead", false},
		StateStopped:     {"Suivi arrêté", true},
		StateError:       {"En erreur", true},
		StateNeverSynced: {"Jamais synchronisé", true},
		StateArchived:    {"Archivé", true},
		StateEmpty:       {"Vide", true},
	}
	for s, w := range want {
		if s.Label() != w.label || s.Gray() != w.gray {
			t.Errorf("%s : Label %q Gray %v, attendu %q %v", s, s.Label(), s.Gray(), w.label, w.gray)
		}
	}
	// Valeurs JSON stables.
	stable := map[State]string{StateActive: "active", StateFlemme: "flemme", StateDead: "dead", StateStopped: "stopped",
		StateError: "error", StateNeverSynced: "never_synced", StateArchived: "archived", StateEmpty: "empty"}
	for s, v := range stable {
		if string(s) != v {
			t.Errorf("valeur JSON %q, attendu %q", s, v)
		}
	}
}

func TestCalendarDays(t *testing.T) {
	paris := mustLoc(t, "Europe/Paris")
	tests := []struct {
		name     string
		from, to time.Time
		loc      *time.Location
		want     int
	}{
		{"même jour", time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC), time.Date(2026, 1, 1, 23, 59, 0, 0, time.UTC), time.UTC, 0},
		{"passage de minuit", time.Date(2026, 1, 1, 23, 59, 0, 0, time.UTC), time.Date(2026, 1, 2, 0, 1, 0, 0, time.UTC), time.UTC, 1},
		{"négatif", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC, -2},
		// Passage à l'heure d'été (29 mars 2026) : le jour ne fait que 23 h.
		{"heure d'été", time.Date(2026, 3, 28, 12, 0, 0, 0, paris), time.Date(2026, 3, 30, 12, 0, 0, 0, paris), paris, 2},
		// Retour à l'heure d'hiver (25 octobre 2026) : le jour fait 25 h.
		{"heure d'hiver", time.Date(2026, 10, 24, 0, 30, 0, 0, paris), time.Date(2026, 10, 26, 0, 30, 0, 0, paris), paris, 2},
		{"fin d'année", time.Date(2025, 12, 31, 22, 0, 0, 0, paris), time.Date(2026, 1, 1, 1, 0, 0, 0, paris), paris, 1},
		{"fuseau du point de vue", time.Date(2026, 5, 1, 21, 30, 0, 0, time.UTC), time.Date(2026, 5, 1, 22, 30, 0, 0, time.UTC), paris, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalendarDays(tt.from, tt.to, tt.loc); got != tt.want {
				t.Errorf("CalendarDays = %d, attendu %d", got, tt.want)
			}
		})
	}
}

func TestCalendarDaysNilLocation(t *testing.T) {
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	if got := CalendarDays(from, from.AddDate(0, 0, 3), nil); got != 3 {
		t.Errorf("CalendarDays(nil) = %d, attendu 3", got)
	}
}
