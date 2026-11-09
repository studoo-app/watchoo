package diagnostic

// Phase 8.1: adjustments of the diagnostic when the conventions are enabled (A1 and A2 without the squash
// commits, F4 without the « Conventional Commits » novelty from the start date).

import (
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

var conventionsOn = RhythmOptions{ConventionsEnabled: true}

func oneGoProject(squash bool) []store.CommitSignalView {
	g := []store.CommitSignalView{
		measured("init", day0, 1, 10, model.CommitForm{}),
		measured("dump", day0.Add(time.Hour), 12, 900, model.CommitForm{DecorativeLines: 6, FrWords: 20, FrWordsUnaccented: 19}),
	}
	g[1].Squash = squash
	return g
}

func TestAbsoluteSignalsSquashWithConventions(t *testing.T) {
	// The big commit is a squash merge: no A1 nor A2, the other signals (A5, A6, A7) stay.
	got := absoluteSignals(oneGoProject(true), conventionsOn)
	if ids := hitsOf(t, got, "dump"); !slices.Equal(ids, []string{"A5", "A6", "A7"}) {
		t.Errorf("squash with conventions: %v", ids)
	}
	commits, _, _ := rhythm(oneGoProject(true), conventionsOn)
	if len(commits) != 1 || slices.Contains(hitIDs(commits[0].hits), "A1") {
		t.Errorf("rhythm with conventions: %+v", commits)
	}

	// Conventions disabled, or not a squash: same result as before phase 8.
	for _, tc := range []struct {
		squash bool
		opts   RhythmOptions
	}{{true, RhythmOptions{}}, {false, conventionsOn}} {
		got := absoluteSignals(oneGoProject(tc.squash), tc.opts)
		if ids := hitsOf(t, got, "dump"); !slices.Equal(ids, []string{"A1", "A2", "A5", "A6", "A7"}) {
			t.Errorf("squash %v, options %+v: %v", tc.squash, tc.opts, ids)
		}
	}
}

func TestAbsoluteSignalsA2WithoutSquash(t *testing.T) {
	// Two squash merges and one ordinary commit: A2 counts the ordinary commit alone.
	g := []store.CommitSignalView{
		measured("s1", day0, 4, 400, model.CommitForm{}),
		measured("own", day0.Add(time.Hour), 4, 450, model.CommitForm{}),
		measured("s2", day0.Add(2*time.Hour), 4, 400, model.CommitForm{}),
	}
	g[0].Squash, g[2].Squash = true, true
	got := absoluteSignals(g, conventionsOn)
	if ids := hitsOf(t, got, "own"); !slices.Equal(ids, []string{"A2", "A7"}) {
		t.Errorf("ordinary commit: %v", ids)
	}
	if h := got["own"][0]; h.Signal != "A2" || h.Raw != 1 {
		t.Errorf("A2 share = %+v, want 1 (ordinary commits only)", h)
	}
	// A7 keeps its anchor and its total: the biggest commit of all is own (450), total 1250.
	if h := got["own"][1]; h.Signal != "A7" || h.Value != "1250" {
		t.Errorf("A7 = %+v, want the total of all commits", h)
	}
	// Without the conventions, the three commits share the lines: no A2.
	if ids := hitsOf(t, absoluteSignals(g, RhythmOptions{}), "own"); !slices.Equal(ids, []string{"A7"}) {
		t.Errorf("without conventions: %v", ids)
	}

	// Four commits, one of them a squash: three ordinary commits, A2 can be evaluated.
	g = []store.CommitSignalView{
		measured("a", day0, 1, 10, model.CommitForm{}),
		measured("b", day0.Add(time.Hour), 1, 10, model.CommitForm{}),
		measured("big", day0.Add(2*time.Hour), 4, 300, model.CommitForm{}),
		measured("sq", day0.Add(3*time.Hour), 4, 20, model.CommitForm{}),
	}
	g[3].Squash = true
	if ids := hitsOf(t, absoluteSignals(g, conventionsOn), "big"); !slices.Equal(ids, []string{"A2"}) {
		t.Errorf("four commits with one squash: %v", ids)
	}
	if ids := hitsOf(t, absoluteSignals(g, RhythmOptions{}), "big"); len(ids) != 0 {
		t.Errorf("four commits without conventions: %v", ids)
	}
}

func TestAbsoluteSignalsA2UnmeasuredSquash(t *testing.T) {
	// A squash merge whose form is not measured does not block A2 on the ordinary commits, but still
	// blocks A7 (computed on all the commits).
	g := []store.CommitSignalView{
		measured("a", day0, 1, 10, model.CommitForm{}),
		measured("big", day0.Add(time.Hour), 4, 300, model.CommitForm{}),
		view(1, "sq", day0.Add(2*time.Hour), 500, 0, 4),
	}
	g[2].Squash = true
	if ids := hitsOf(t, absoluteSignals(g, conventionsOn), "big"); !slices.Equal(ids, []string{"A2"}) {
		t.Errorf("unmeasured squash with conventions: %v", ids)
	}
	// Without the conventions, the unmeasured commit blocks A2 as before phase 8.
	if ids := hitsOf(t, absoluteSignals(g, RhythmOptions{}), "big"); len(ids) != 0 {
		t.Errorf("unmeasured commit without conventions: %v", ids)
	}
	// An unmeasured ordinary commit still blocks A2 with the conventions.
	g[2].Squash = false
	if ids := hitsOf(t, absoluteSignals(g, conventionsOn), "big"); len(ids) != 0 {
		t.Errorf("unmeasured ordinary commit with conventions: %v", ids)
	}
}

// conventionalOnly returns a 25-commit history and a commit whose only novelty is a Conventional Commits
// subject, with a long message (written at day0 + 25 days, 10:00 UTC).
func conventionalOnly(bullets int) []store.CommitSignalView {
	views := formHistory(25)
	c := view(1, "conv", day0.AddDate(0, 0, 25), 10, 2, 1)
	c.Form = model.CommitForm{Known: true, MessageLength: 400, MessageConventional: true, MessageBullets: bullets}
	return append(views, c)
}

func TestMessageStyleWithConventions(t *testing.T) {
	plus14 := time.FixedZone("UTC+14", 14*3600) // day0 + 25 days, 10:00 UTC is 2026-06-27 00:00 here
	for _, tc := range []struct {
		name     string
		opts     RhythmOptions
		repo     string // repository's own start date
		bullets  int
		wantF4   bool
		describe string
	}{
		{"disabled", RhythmOptions{}, "", 0, true, "conventions disabled: F4 as before"},
		{"no date", conventionsOn, "", 0, false, "empty since: always neutralised"},
		{"after since", RhythmOptions{ConventionsEnabled: true, ConventionsSince: "2026-06-01"}, "", 0, false, "commit after the date"},
		{"same day", RhythmOptions{ConventionsEnabled: true, ConventionsSince: "2026-06-26"}, "", 0, false, "commit on the start day"},
		{"before since", RhythmOptions{ConventionsEnabled: true, ConventionsSince: "2026-07-01"}, "", 0, true, "commit before the date"},
		{"repository date", RhythmOptions{ConventionsEnabled: true, ConventionsSince: "2026-01-01"}, "2026-07-01", 0, true, "repository date wins"},
		{"repository date only", conventionsOn, "2026-06-01", 0, false, "repository date, empty workspace date"},
		{"time zone", RhythmOptions{ConventionsEnabled: true, ConventionsSince: "2026-06-27", Location: plus14}, "", 0, false, "midnight in the workspace time zone"},
		{"time zone UTC", RhythmOptions{ConventionsEnabled: true, ConventionsSince: "2026-06-27"}, "", 0, true, "midnight UTC is later"},
		{"unreadable date", RhythmOptions{ConventionsEnabled: true, ConventionsSince: "bad"}, "", 0, false, "unreadable date: from the start"},
		{"bullets", conventionsOn, "", 4, true, "the bullet novelty stays"},
	} {
		views := conventionalOnly(tc.bullets)
		for i := range views {
			views[i].ConventionsSince = tc.repo
		}
		got := formSignals(views, tc.opts)
		if has := slices.Contains(hitIDs(got["conv"]), "F4"); has != tc.wantF4 {
			t.Errorf("%s (%s): F4 = %v, want %v", tc.name, tc.describe, has, tc.wantF4)
		}
	}
}
