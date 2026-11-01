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
		t.Skipf("time zone %s unavailable: %v", name, err)
	}
	return loc
}

// syncedRepo returns a tracked, synced repository whose last commit dates from lastCommit.
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
		{"global", model.Repository{}, defaultCfg, Thresholds{5, 15}},
		{"specific to the repository", model.Repository{FlemmeAfterDays: intPtr(3), DeadAfterDays: intPtr(10)}, defaultCfg, Thresholds{3, 10}},
		{"repository's flemme only", model.Repository{FlemmeAfterDays: intPtr(7)}, defaultCfg, Thresholds{7, 15}},
		{"repository's dead only", model.Repository{DeadAfterDays: intPtr(30)}, defaultCfg, Thresholds{5, 30}},
		{"inconsistent mix corrected", model.Repository{FlemmeAfterDays: intPtr(20)}, defaultCfg, Thresholds{20, 21}},
		{"repository's dead too small", model.Repository{DeadAfterDays: intPtr(2)}, defaultCfg, Thresholds{5, 6}},
		{"zero values ignored", model.Repository{FlemmeAfterDays: intPtr(0), DeadAfterDays: intPtr(-1)}, defaultCfg, Thresholds{5, 15}},
		{"empty configuration: defaults", model.Repository{}, workspace.ActivityConfig{}, Thresholds{workspace.DefaultFlemmeAfterDays, workspace.DefaultDeadAfterDays}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ThresholdsFor(tt.repo, tt.cfg); got != tt.want {
				t.Errorf("ThresholdsFor = %+v, want %+v", got, tt.want)
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
			t.Errorf("%d days: state %s (%d d), want %s (%d d)", tt.days, got.State, got.Days, tt.want, tt.days)
		}
		if got.Thresholds != (Thresholds{5, 15}) {
			t.Errorf("%d days: thresholds %+v", tt.days, got.Thresholds)
		}
	}
}

func TestEvaluateRepoThresholds(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	repo := syncedRepo(now.AddDate(0, 0, -4))
	repo.FlemmeAfterDays = intPtr(3)
	repo.DeadAfterDays = intPtr(4)
	if got := Evaluate(repo, model.SyncRun{}, defaultCfg, now, time.UTC); got.State != StateFlemme {
		t.Errorf("4 d with thresholds 3/4: %s, want flemme", got.State)
	}
	repo.LastCommitAt = now.AddDate(0, 0, -5)
	if got := Evaluate(repo, model.SyncRun{}, defaultCfg, now, time.UTC); got.State != StateDead {
		t.Errorf("5 d with thresholds 3/4: %s, want dead", got.State)
	}
}

func TestEvaluateTimezoneMidnight(t *testing.T) {
	paris := mustLoc(t, "Europe/Paris")
	// Commit on March 14 at 11:30 pm in Paris; now March 20 at 0:10 am in Paris: 40 minutes more than
	// 5 days, but 6 calendar days → Flemme.
	commit := time.Date(2026, 3, 14, 23, 30, 0, 0, paris)
	now := time.Date(2026, 3, 20, 0, 10, 0, 0, paris)
	got := Evaluate(syncedRepo(commit), model.SyncRun{}, defaultCfg, now, paris)
	if got.Days != 6 || got.State != StateFlemme {
		t.Errorf("Paris: %d d %s, want 6 d flemme", got.Days, got.State)
	}
	// Same instants seen in UTC (10:30 pm on the 14th, 11:10 pm on the 19th): 5 days → Active.
	got = Evaluate(syncedRepo(commit), model.SyncRun{}, defaultCfg, now, time.UTC)
	if got.Days != 5 || got.State != StateActive {
		t.Errorf("UTC: %d d %s, want 5 d active", got.Days, got.State)
	}
}

func TestEvaluateFutureCommit(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	got := Evaluate(syncedRepo(now.Add(48*time.Hour)), model.SyncRun{}, defaultCfg, now, time.UTC)
	if got.Days != 0 || got.State != StateActive {
		t.Errorf("future commit: %d d %s, want 0 d active", got.Days, got.State)
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
		{"stopped prevails over everything", func(r *model.Repository) { r.Enabled = false; r.Archived = true }, errRun, StateStopped, 40},
		{"error prevails over archived", func(r *model.Repository) { r.Archived = true }, errRun, StateError, 40},
		{"error without successful sync", func(r *model.Repository) { r.LastSyncedAt = time.Time{}; r.LastCommitAt = time.Time{} }, errRun, StateError, -1},
		{"never synced", func(r *model.Repository) { r.LastSyncedAt = time.Time{}; r.LastCommitAt = time.Time{} }, model.SyncRun{}, StateNeverSynced, -1},
		{"sync in progress, never successful", func(r *model.Repository) { r.LastSyncedAt = time.Time{}; r.LastCommitAt = time.Time{} },
			model.SyncRun{ID: 3, Status: model.SyncRunning}, StateNeverSynced, -1},
		{"archived, never Dead", func(r *model.Repository) { r.Archived = true }, okRun, StateArchived, 40},
		{"empty", func(r *model.Repository) { r.LastCommitAt = time.Time{} }, okRun, StateEmpty, -1},
		{"normal dead", func(r *model.Repository) {}, okRun, StateDead, 40},
		{"run with ID 0 ignored", func(r *model.Repository) {}, model.SyncRun{Status: model.SyncError}, StateDead, 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := syncedRepo(old)
			tt.mod(&repo)
			got := Evaluate(repo, tt.run, defaultCfg, now, time.UTC)
			if got.State != tt.want || got.Days != tt.wantDays {
				t.Errorf("state %s (%d d), want %s (%d d)", got.State, got.Days, tt.want, tt.wantDays)
			}
			if got.State.Gray() != (tt.want != StateActive && tt.want != StateFlemme && tt.want != StateDead) {
				t.Errorf("Gray() = %v for %s", got.State.Gray(), got.State)
			}
		})
	}
}

func TestStateLabelAndGray(t *testing.T) {
	want := map[State]struct {
		label string
		gray  bool
	}{
		StateActive:      {"Active", false},
		StateFlemme:      {"Slacking", false},
		StateDead:        {"Dead", false},
		StateStopped:     {"Tracking stopped", true},
		StateError:       {"Error", true},
		StateNeverSynced: {"Never synced", true},
		StateArchived:    {"Archived", true},
		StateEmpty:       {"Empty", true},
	}
	for s, w := range want {
		if s.Label() != w.label || s.Gray() != w.gray {
			t.Errorf("%s: Label %q Gray %v, want %q %v", s, s.Label(), s.Gray(), w.label, w.gray)
		}
	}
	// Stable JSON values.
	stable := map[State]string{StateActive: "active", StateFlemme: "flemme", StateDead: "dead", StateStopped: "stopped",
		StateError: "error", StateNeverSynced: "never_synced", StateArchived: "archived", StateEmpty: "empty"}
	for s, v := range stable {
		if string(s) != v {
			t.Errorf("JSON value %q, want %q", s, v)
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
		{"same day", time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC), time.Date(2026, 1, 1, 23, 59, 0, 0, time.UTC), time.UTC, 0},
		{"past midnight", time.Date(2026, 1, 1, 23, 59, 0, 0, time.UTC), time.Date(2026, 1, 2, 0, 1, 0, 0, time.UTC), time.UTC, 1},
		{"negative", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC, -2},
		// Switch to summer time (March 29, 2026): the day only lasts 23 h.
		{"summer time", time.Date(2026, 3, 28, 12, 0, 0, 0, paris), time.Date(2026, 3, 30, 12, 0, 0, 0, paris), paris, 2},
		// Back to winter time (October 25, 2026): the day lasts 25 h.
		{"winter time", time.Date(2026, 10, 24, 0, 30, 0, 0, paris), time.Date(2026, 10, 26, 0, 30, 0, 0, paris), paris, 2},
		{"end of year", time.Date(2025, 12, 31, 22, 0, 0, 0, paris), time.Date(2026, 1, 1, 1, 0, 0, 0, paris), paris, 1},
		{"time zone of the point of view", time.Date(2026, 5, 1, 21, 30, 0, 0, time.UTC), time.Date(2026, 5, 1, 22, 30, 0, 0, time.UTC), paris, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalendarDays(tt.from, tt.to, tt.loc); got != tt.want {
				t.Errorf("CalendarDays = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCalendarDaysNilLocation(t *testing.T) {
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	if got := CalendarDays(from, from.AddDate(0, 0, 3), nil); got != 3 {
		t.Errorf("CalendarDays(nil) = %d, want 3", got)
	}
}
