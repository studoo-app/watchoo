package github

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

// testToken est le faux token des tests ; aucun test ne lit GH_TOKEN ni GITHUB_TOKEN.
const testToken = "gho_fake_for_tests"

func TestFingerprint(t *testing.T) {
	a, b := fingerprintOf(testToken), fingerprintOf(testToken+"_autre")
	if a == b {
		t.Fatal("deux tokens différents ont la même empreinte")
	}
	if a != fingerprintOf(testToken) {
		t.Fatal("l'empreinte n'est pas déterministe")
	}
}

func TestQuotaObserve(t *testing.T) {
	fp := fingerprintOf(testToken)
	reset := time.Unix(1_800_000_000, 0).UTC()
	q := NewQuota()
	if _, ok := q.last(fp); ok {
		t.Fatal("un quota neuf ne doit rien connaître")
	}

	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 4000, Used: 1000, Reset: reset})
	if rl, ok := q.last(fp); !ok || rl.Remaining != 4000 {
		t.Fatalf("last = %+v, %v", rl, ok)
	}
	// Même fenêtre : une réponse en retard (Remaining plus haut) ne fait pas remonter l'état.
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 4500, Reset: reset})
	if rl, _ := q.last(fp); rl.Remaining != 4000 {
		t.Errorf("Remaining = %d après une réponse en retard, veut 4000", rl.Remaining)
	}
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 3999, Reset: reset})
	if rl, _ := q.last(fp); rl.Remaining != 3999 {
		t.Errorf("Remaining = %d, veut 3999", rl.Remaining)
	}
	// Fenêtre plus ancienne : ignorée.
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 1, Reset: reset.Add(-time.Hour)})
	if rl, _ := q.last(fp); rl.Remaining != 3999 {
		t.Errorf("Remaining = %d après une fenêtre ancienne, veut 3999", rl.Remaining)
	}
	// Nouvelle fenêtre : remplace, même si Remaining remonte.
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 4999, Reset: reset.Add(time.Hour)})
	if rl, _ := q.last(fp); rl.Remaining != 4999 || !rl.Reset.Equal(reset.Add(time.Hour)) {
		t.Errorf("last = %+v après une nouvelle fenêtre", rl)
	}
}

// TestQuotaString vérifie qu'un %v, %+v ou %#v sur un *Quota ne révèle pas l'empreinte du token.
func TestQuotaString(t *testing.T) {
	q := NewQuota()
	if got := fmt.Sprint(q); got != "github.Quota(inconnu)" {
		t.Errorf("String() = %q", got)
	}
	fp := fingerprintOf(testToken)
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 4321, Reset: time.Unix(1_800_000_000, 0)})
	out := fmt.Sprintf("%v %+v %#v %s", q, q, q, q)
	if !strings.Contains(out, "github.Quota(4321/5000 restantes)") {
		t.Errorf("sortie inattendue : %q", out)
	}
	for _, leak := range []string{hex.EncodeToString(fp[:]), fmt.Sprint(fp[:4]), testToken} {
		if strings.Contains(out, strings.Trim(leak, "[]")) {
			t.Errorf("la sortie %q révèle l'empreinte ou le token", out)
		}
	}
}

func TestQuotaTokenChange(t *testing.T) {
	fp, other := fingerprintOf(testToken), fingerprintOf("gho_fake_for_tests_autre")
	now := time.Unix(1_800_000_000, 0)
	q := NewQuota()
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 0, Reset: now.Add(time.Hour)})
	q.hold(fp, now.Add(time.Minute), true)

	// Un autre token ne voit ni l'état ni le blocage, sans les effacer pour autant.
	if _, ok := q.last(other); ok {
		t.Error("last(autre token) doit renvoyer false")
	}
	if err := q.blocked(other, now); err != nil {
		t.Errorf("blocked(autre token) = %v, veut nil", err)
	}
	if _, ok := q.last(fp); !ok {
		t.Error("une simple lecture par un autre token ne doit pas effacer l'état")
	}

	// Une écriture par un autre token repart d'un état vide.
	q.observe(other, provider.RateLimit{Limit: 5000, Remaining: 10, Reset: now.Add(30 * time.Minute)})
	if _, ok := q.last(fp); ok {
		t.Error("l'état de l'ancien token doit être oublié")
	}
	if err := q.blocked(fp, now); err != nil {
		t.Errorf("blocked(ancien token) = %v", err)
	}
	if rl, ok := q.last(other); !ok || rl.Remaining != 10 {
		t.Errorf("last(nouveau token) = %+v, %v", rl, ok)
	}
	if err := q.blocked(other, now); err != nil {
		t.Errorf("le blocage secondaire de l'ancien token ne doit pas passer au nouveau : %v", err)
	}
}

func TestQuotaBlocked(t *testing.T) {
	fp := fingerprintOf(testToken)
	now := time.Unix(1_800_000_000, 0)
	reset := now.Add(10 * time.Minute)

	q := NewQuota()
	if err := q.blocked(fp, now); err != nil {
		t.Fatalf("quota inconnu : blocked = %v", err)
	}
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 1, Reset: reset})
	if err := q.blocked(fp, now); err != nil {
		t.Fatalf("quota non épuisé : blocked = %v", err)
	}
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 0, Reset: reset})
	err := q.blocked(fp, now)
	if err == nil || err.Secondary || !err.Reset.Equal(reset) || !errors.Is(err, provider.ErrRateLimited) {
		t.Fatalf("quota épuisé : blocked = %#v", err)
	}
	if err := q.blocked(fp, reset); err != nil {
		t.Errorf("après la remise à zéro : blocked = %v", err)
	}

	// Limite secondaire : la plus lointaine échéance l'emporte, avec sa nature.
	q2 := &Quota{} // valeur zéro utilisable
	q2.hold(fp, now.Add(2*time.Minute), true)
	q2.hold(fp, now.Add(time.Minute), false)
	err = q2.blocked(fp, now)
	if err == nil || !err.Secondary || !err.Reset.Equal(now.Add(2*time.Minute).UTC()) || err.RetryAfter != 2*time.Minute {
		t.Fatalf("limite secondaire : blocked = %#v", err)
	}
	if err := q2.blocked(fp, now.Add(2*time.Minute)); err != nil {
		t.Errorf("après l'échéance : blocked = %v", err)
	}

	// Attente de quota épuisé (remise à zéro connue par Retry-After seulement) : Secondary = false.
	q3 := NewQuota()
	q3.hold(fp, now.Add(30*time.Second), false)
	if err := q3.blocked(fp, now); err == nil || err.Secondary || !err.Reset.Equal(now.Add(30*time.Second).UTC()) {
		t.Errorf("quota épuisé par Retry-After : blocked = %#v", err)
	}
}

// TestQuotaBlockedFarthest : quand une limite secondaire et un quota épuisé retiennent tous deux les
// requêtes, le refus préventif donne la plus lointaine des deux échéances, avec sa nature.
// Non-régression : la limite secondaire, plus proche, était renvoyée d'abord, avec un Reset trop tôt.
func TestQuotaBlockedFarthest(t *testing.T) {
	fp := fingerprintOf(testToken)
	now := time.Unix(1_800_000_000, 0).UTC()
	reset := now.Add(30 * time.Minute)
	for _, tt := range []struct {
		name      string
		hold      time.Time
		secondary bool
		reset     time.Time
	}{
		{"limite secondaire plus proche que la remise à zéro", now.Add(time.Minute), false, reset},
		{"limite secondaire plus lointaine que la remise à zéro", now.Add(time.Hour), true, now.Add(time.Hour)},
		{"échéances égales", reset, false, reset},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQuota()
			q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 0, Reset: reset})
			q.hold(fp, tt.hold, true)
			err := q.blocked(fp, now.Add(10*time.Second))
			if err == nil || err.Secondary != tt.secondary || !err.Reset.Equal(tt.reset) {
				t.Fatalf("blocked = %#v ; veut Secondary %v, Reset %s", err, tt.secondary, tt.reset)
			}
			if tt.secondary && err.RetryAfter != tt.hold.Sub(now.Add(10*time.Second)) {
				t.Errorf("RetryAfter = %s", err.RetryAfter)
			}
		})
	}
}

func TestRateFromHeader(t *testing.T) {
	now := time.Date(2026, 10, 26, 14, 0, 0, 0, time.FixedZone("CET", 3600))
	h := http.Header{}
	h.Set("X-Ratelimit-Limit", "5000")
	h.Set("X-Ratelimit-Remaining", "4980")
	h.Set("X-Ratelimit-Reset", "1793053402")
	h.Set("X-Ratelimit-Resource", "core")
	h.Set("X-Ratelimit-Used", "20")
	rl, ok := rateFromHeader(h, now)
	want := provider.RateLimit{
		Limit: 5000, Remaining: 4980, Used: 20,
		Reset: time.Unix(1793053402, 0).UTC(), ObservedAt: now.UTC(),
	}
	if !ok || rl != want {
		t.Fatalf("rateFromHeader = %+v, %v ; veut %+v", rl, ok, want)
	}
	if rl.Reset.Location() != time.UTC || rl.ObservedAt.Location() != time.UTC {
		t.Error("les dates doivent être en UTC")
	}

	// Used absent : déduit de Limit et Remaining.
	h.Del("X-Ratelimit-Used")
	if rl, ok := rateFromHeader(h, now); !ok || rl.Used != 20 {
		t.Errorf("Used déduit = %d, %v", rl.Used, ok)
	}
	// Ressource sans en-tête : acceptée ; autre ressource : ignorée.
	h.Del("X-Ratelimit-Resource")
	if _, ok := rateFromHeader(h, now); !ok {
		t.Error("ressource absente : doit être acceptée")
	}
	h.Set("X-Ratelimit-Resource", "search")
	if _, ok := rateFromHeader(h, now); ok {
		t.Error("ressource search : doit être ignorée")
	}
	h.Set("X-Ratelimit-Resource", "core")

	for _, bad := range []struct{ key, value string }{
		{"X-Ratelimit-Remaining", ""}, {"X-Ratelimit-Remaining", "-1"}, {"X-Ratelimit-Remaining", "abc"},
		{"X-Ratelimit-Reset", ""}, {"X-Ratelimit-Reset", "0"}, {"X-Ratelimit-Reset", "demain"},
	} {
		h2 := h.Clone()
		h2.Set(bad.key, bad.value)
		if _, ok := rateFromHeader(h2, now); ok {
			t.Errorf("%s = %q : doit être refusé", bad.key, bad.value)
		}
	}
	if _, ok := rateFromHeader(http.Header{}, now); ok {
		t.Error("sans en-têtes : doit renvoyer false")
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 26, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		value string
		want  time.Duration
		ok    bool
	}{
		{"60", time.Minute, true},
		{" 0 ", 0, true},
		{now.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second, true},
		{now.Add(-time.Hour).Format(http.TimeFormat), 0, true},
		{"", 0, false},
		{"-5", 0, false},
		{"bientôt", 0, false},
	}
	for _, tt := range tests {
		h := http.Header{}
		if tt.value != "" {
			h.Set("Retry-After", tt.value)
		}
		got, ok := retryAfter(h, now)
		if got != tt.want || ok != tt.ok {
			t.Errorf("retryAfter(%q) = %s, %v ; veut %s, %v", tt.value, got, ok, tt.want, tt.ok)
		}
	}
	h := http.Header{}
	h.Set("Retry-After", "99999999999999")
	if got, ok := retryAfter(h, now); !ok || got != 365*24*time.Hour {
		t.Errorf("Retry-After démesuré = %s, %v ; veut un an au plus", got, ok)
	}
}

func TestQuotaConcurrent(t *testing.T) {
	fp := fingerprintOf(testToken)
	reset := time.Unix(1_800_000_000, 0)
	q := NewQuota()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 200 {
				q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 5000 - i*200 - j, Reset: reset})
				q.hold(fp, reset.Add(-time.Hour), i%2 == 0)
				_, _ = q.last(fp)
				_ = q.blocked(fp, reset.Add(-2*time.Hour))
			}
		})
	}
	wg.Wait()
	if rl, ok := q.last(fp); !ok || rl.Remaining != 5000-7*200-199 {
		t.Errorf("Remaining final = %d, veut le minimum observé %d", rl.Remaining, 5000-7*200-199)
	}
}
