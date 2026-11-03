package sync

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
)

func TestQuotaLow(t *testing.T) {
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	reset := now.Add(time.Hour)
	for _, tc := range []struct {
		rl   provider.RateLimit
		want bool
	}{
		{provider.RateLimit{Limit: 5000, Remaining: 499, Reset: reset}, true},
		{provider.RateLimit{Limit: 5000, Remaining: 500, Reset: reset}, false},
		{provider.RateLimit{Limit: 5000, Remaining: 0, Reset: now}, false}, // window over
		{provider.RateLimit{Limit: 5000, Remaining: 0}, true},              // unknown reset
		{provider.RateLimit{}, false},                                      // unknown limit
	} {
		if got := QuotaLow(tc.rl, now); got != tc.want {
			t.Errorf("QuotaLow(%+v) = %v", tc.rl, got)
		}
	}
}

func TestMessages(t *testing.T) {
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	if got := deferredMessage(DeferLowQuota, time.Time{}); got != "deferred: low quota" {
		t.Errorf("without time: %q", got)
	}
	paris := time.FixedZone("Paris", 2*3600)
	if got := deferredMessage(DeferQuotaExhausted, time.Date(2026, 10, 20, 13, 7, 0, 0, paris)); got !=
		"deferred: quota exhausted until 11:07 UTC" {
		t.Errorf("with time: %q", got)
	}
	if got := rateLimitResume(&provider.RateLimitError{}, now); !got.IsZero() {
		t.Errorf("unknown resume: %v", got)
	}
	if got := errorMessage(fmt.Errorf("x: %w", context.DeadlineExceeded)); got != "sync interrupted: deadline exceeded" {
		t.Errorf("deadline: %q", got)
	}
	if got := errorMessage(errors.New("other")); got != "other" {
		t.Errorf("other: %q", got)
	}
	if got := DeferReason("").Text(); got != "low quota" {
		t.Errorf("empty reason: %q", got)
	}
}

// The messages recorded for the user follow the process language: French typography. Not parallel:
// the process language is shared.
func TestMessagesFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	paris := time.FixedZone("Paris", 2*3600)
	if got := deferredMessage(DeferQuotaExhausted, time.Date(2026, 10, 20, 13, 7, 0, 0, paris)); got !=
		"reporté : quota épuisé jusqu'à 11:07 UTC" {
		t.Errorf("deferred: %q", got)
	}
	if got := deferredMessage(DeferLowQuota, time.Time{}); got != "reporté : quota bas" {
		t.Errorf("deferred without time: %q", got)
	}
	if got := errorMessage(context.Canceled); got != "synchro annulée" {
		t.Errorf("canceled: %q", got)
	}
	var x extras
	x.warn(DeferLowQuota.Text(), listPulls, listReleases, listTags)
	x.warn("HTTP 502", listReleases)
	if len(x.warnings) != 2 || x.warnings[0].Message != "pull requests, releases et tags non synchronisés : quota bas" ||
		x.warnings[1].Message != "releases non synchronisées : HTTP 502" || x.warnings[0].Kind != model.WarnListsNotSynced {
		t.Errorf("warnings = %q", x.warnings)
	}
	if got := (Source{Kind: SourceGit}).Label(); got != "clones Git locaux" {
		t.Errorf("Label = %q", got)
	}
	if got := ErrSyncRunning.Error(); got != "synchro déjà en cours" {
		t.Errorf("ErrSyncRunning = %q", got)
	}
}

func TestHalt(t *testing.T) {
	var h halt
	if _, _, ok := h.get(); ok {
		t.Error("initial halt")
	}
	t1 := time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC)
	h.set(t1, DeferLowQuota)
	h.set(t1.Add(-time.Minute), DeferQuotaExhausted)
	at, reason, ok := h.get()
	if !ok || !at.Equal(t1) || reason != DeferQuotaExhausted {
		t.Errorf("halt = %v %q %v: furthest deadline and exhausted quota expected", at, reason, ok)
	}
	h.set(t1.Add(time.Hour), DeferLowQuota)
	if at, reason, _ := h.get(); !at.Equal(t1.Add(time.Hour)) || reason != DeferQuotaExhausted {
		t.Errorf("halt = %v %q", at, reason)
	}
}

func TestForEachBorne(t *testing.T) {
	seen := make([]bool, 10)
	forEach(10, 99, func(i int) { seen[i] = true })
	for i, ok := range seen {
		if !ok {
			t.Errorf("index %d not processed", i)
		}
	}
}
