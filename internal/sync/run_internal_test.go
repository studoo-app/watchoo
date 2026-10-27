package sync

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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
		{provider.RateLimit{Limit: 5000, Remaining: 0, Reset: now}, false}, // fenêtre échue
		{provider.RateLimit{Limit: 5000, Remaining: 0}, true},              // reset inconnu
		{provider.RateLimit{}, false},                                      // limite inconnue
	} {
		if got := QuotaLow(tc.rl, now); got != tc.want {
			t.Errorf("QuotaLow(%+v) = %v", tc.rl, got)
		}
	}
}

func TestMessages(t *testing.T) {
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	if got := deferredMessage(reasonLow, time.Time{}); got != "reporté : quota bas" {
		t.Errorf("sans heure : %q", got)
	}
	paris := time.FixedZone("Paris", 2*3600)
	if got := deferredMessage(reasonSpent, time.Date(2026, 10, 20, 13, 7, 0, 0, paris)); got !=
		"reporté : quota épuisé jusqu'à 11:07 UTC" {
		t.Errorf("avec heure : %q", got)
	}
	if got := rateLimitResume(&provider.RateLimitError{}, now); !got.IsZero() {
		t.Errorf("reprise inconnue : %v", got)
	}
	if got := errorMessage(fmt.Errorf("x : %w", context.DeadlineExceeded)); got != msgDeadline {
		t.Errorf("délai : %q", got)
	}
	if got := errorMessage(errors.New("autre")); got != "autre" {
		t.Errorf("autre : %q", got)
	}
}

func TestHalt(t *testing.T) {
	var h halt
	if _, _, ok := h.get(); ok {
		t.Error("arrêt initial")
	}
	t1 := time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC)
	h.set(t1, reasonLow)
	h.set(t1.Add(-time.Minute), reasonSpent)
	at, reason, ok := h.get()
	if !ok || !at.Equal(t1) || reason != reasonSpent {
		t.Errorf("arrêt = %v %q %v : échéance la plus lointaine et quota épuisé attendus", at, reason, ok)
	}
	h.set(t1.Add(time.Hour), reasonLow)
	if at, reason, _ := h.get(); !at.Equal(t1.Add(time.Hour)) || reason != reasonSpent {
		t.Errorf("arrêt = %v %q", at, reason)
	}
}

func TestForEachBorne(t *testing.T) {
	seen := make([]bool, 10)
	forEach(10, 99, func(i int) { seen[i] = true })
	for i, ok := range seen {
		if !ok {
			t.Errorf("indice %d non traité", i)
		}
	}
}
