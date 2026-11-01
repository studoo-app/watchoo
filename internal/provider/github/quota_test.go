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

// testToken is the fake token of the tests; no test reads GH_TOKEN or GITHUB_TOKEN.
const testToken = "gho_fake_for_tests"

func TestFingerprint(t *testing.T) {
	a, b := fingerprintOf(testToken), fingerprintOf(testToken+"_autre")
	if a == b {
		t.Fatal("two different tokens have the same fingerprint")
	}
	if a != fingerprintOf(testToken) {
		t.Fatal("the fingerprint is not deterministic")
	}
}

func TestQuotaObserve(t *testing.T) {
	fp := fingerprintOf(testToken)
	reset := time.Unix(1_800_000_000, 0).UTC()
	q := NewQuota()
	if _, ok := q.last(fp); ok {
		t.Fatal("a new quota must know nothing")
	}

	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 4000, Used: 1000, Reset: reset})
	if rl, ok := q.last(fp); !ok || rl.Remaining != 4000 {
		t.Fatalf("last = %+v, %v", rl, ok)
	}
	// Same window: a late response (higher Remaining) does not raise the state.
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 4500, Reset: reset})
	if rl, _ := q.last(fp); rl.Remaining != 4000 {
		t.Errorf("Remaining = %d after a late response, want 4000", rl.Remaining)
	}
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 3999, Reset: reset})
	if rl, _ := q.last(fp); rl.Remaining != 3999 {
		t.Errorf("Remaining = %d, want 3999", rl.Remaining)
	}
	// Older window: ignored.
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 1, Reset: reset.Add(-time.Hour)})
	if rl, _ := q.last(fp); rl.Remaining != 3999 {
		t.Errorf("Remaining = %d after an old window, want 3999", rl.Remaining)
	}
	// New window: replaces, even if Remaining goes up.
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 4999, Reset: reset.Add(time.Hour)})
	if rl, _ := q.last(fp); rl.Remaining != 4999 || !rl.Reset.Equal(reset.Add(time.Hour)) {
		t.Errorf("last = %+v after a new window", rl)
	}
}

// TestQuotaString checks that a %v, %+v or %#v on a *Quota does not reveal the token's fingerprint.
func TestQuotaString(t *testing.T) {
	q := NewQuota()
	if got := fmt.Sprint(q); got != "github.Quota(inconnu)" {
		t.Errorf("String() = %q", got)
	}
	fp := fingerprintOf(testToken)
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 4321, Reset: time.Unix(1_800_000_000, 0)})
	out := fmt.Sprintf("%v %+v %#v %s", q, q, q, q)
	if !strings.Contains(out, "github.Quota(4321/5000 restantes)") {
		t.Errorf("unexpected output: %q", out)
	}
	for _, leak := range []string{hex.EncodeToString(fp[:]), fmt.Sprint(fp[:4]), testToken} {
		if strings.Contains(out, strings.Trim(leak, "[]")) {
			t.Errorf("the output %q reveals the fingerprint or the token", out)
		}
	}
}

func TestQuotaTokenChange(t *testing.T) {
	fp, other := fingerprintOf(testToken), fingerprintOf("gho_fake_for_tests_autre")
	now := time.Unix(1_800_000_000, 0)
	q := NewQuota()
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 0, Reset: now.Add(time.Hour)})
	q.hold(fp, now.Add(time.Minute), true)

	// Another token sees neither the state nor the block, without erasing them either.
	if _, ok := q.last(other); ok {
		t.Error("last(other token) must return false")
	}
	if err := q.blocked(other, now); err != nil {
		t.Errorf("blocked(other token) = %v, want nil", err)
	}
	if _, ok := q.last(fp); !ok {
		t.Error("a mere read by another token must not erase the state")
	}

	// A write by another token starts again from an empty state.
	q.observe(other, provider.RateLimit{Limit: 5000, Remaining: 10, Reset: now.Add(30 * time.Minute)})
	if _, ok := q.last(fp); ok {
		t.Error("the old token's state must be forgotten")
	}
	if err := q.blocked(fp, now); err != nil {
		t.Errorf("blocked(old token) = %v", err)
	}
	if rl, ok := q.last(other); !ok || rl.Remaining != 10 {
		t.Errorf("last(new token) = %+v, %v", rl, ok)
	}
	if err := q.blocked(other, now); err != nil {
		t.Errorf("the old token's secondary block must not carry over to the new one: %v", err)
	}
}

func TestQuotaBlocked(t *testing.T) {
	fp := fingerprintOf(testToken)
	now := time.Unix(1_800_000_000, 0)
	reset := now.Add(10 * time.Minute)

	q := NewQuota()
	if err := q.blocked(fp, now); err != nil {
		t.Fatalf("unknown quota: blocked = %v", err)
	}
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 1, Reset: reset})
	if err := q.blocked(fp, now); err != nil {
		t.Fatalf("quota not exhausted: blocked = %v", err)
	}
	q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 0, Reset: reset})
	err := q.blocked(fp, now)
	if err == nil || err.Secondary || !err.Reset.Equal(reset) || !errors.Is(err, provider.ErrRateLimited) {
		t.Fatalf("exhausted quota: blocked = %#v", err)
	}
	if err := q.blocked(fp, reset); err != nil {
		t.Errorf("after the reset: blocked = %v", err)
	}

	// Secondary limit: the farthest deadline wins, with its nature.
	q2 := &Quota{} // usable zero value
	q2.hold(fp, now.Add(2*time.Minute), true)
	q2.hold(fp, now.Add(time.Minute), false)
	err = q2.blocked(fp, now)
	if err == nil || !err.Secondary || !err.Reset.Equal(now.Add(2*time.Minute).UTC()) || err.RetryAfter != 2*time.Minute {
		t.Fatalf("secondary limit: blocked = %#v", err)
	}
	if err := q2.blocked(fp, now.Add(2*time.Minute)); err != nil {
		t.Errorf("after the deadline: blocked = %v", err)
	}

	// Waiting for an exhausted quota (reset known through Retry-After only): Secondary = false.
	q3 := NewQuota()
	q3.hold(fp, now.Add(30*time.Second), false)
	if err := q3.blocked(fp, now); err == nil || err.Secondary || !err.Reset.Equal(now.Add(30*time.Second).UTC()) {
		t.Errorf("quota exhausted by Retry-After: blocked = %#v", err)
	}
}

// TestQuotaBlockedFarthest: when a secondary limit and an exhausted quota both hold the requests
// back, the preventive refusal gives the farthest of the two deadlines, with its nature.
// Non-regression: the secondary limit, closer, was returned first, with a Reset too early.
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
		{"secondary limit closer than the reset", now.Add(time.Minute), false, reset},
		{"secondary limit farther than the reset", now.Add(time.Hour), true, now.Add(time.Hour)},
		{"equal deadlines", reset, false, reset},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQuota()
			q.observe(fp, provider.RateLimit{Limit: 5000, Remaining: 0, Reset: reset})
			q.hold(fp, tt.hold, true)
			err := q.blocked(fp, now.Add(10*time.Second))
			if err == nil || err.Secondary != tt.secondary || !err.Reset.Equal(tt.reset) {
				t.Fatalf("blocked = %#v; want Secondary %v, Reset %s", err, tt.secondary, tt.reset)
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
		t.Fatalf("rateFromHeader = %+v, %v; want %+v", rl, ok, want)
	}
	if rl.Reset.Location() != time.UTC || rl.ObservedAt.Location() != time.UTC {
		t.Error("the dates must be in UTC")
	}

	// Used missing: deduced from Limit and Remaining.
	h.Del("X-Ratelimit-Used")
	if rl, ok := rateFromHeader(h, now); !ok || rl.Used != 20 {
		t.Errorf("deduced Used = %d, %v", rl.Used, ok)
	}
	// Resource without header: accepted; other resource: ignored.
	h.Del("X-Ratelimit-Resource")
	if _, ok := rateFromHeader(h, now); !ok {
		t.Error("missing resource: must be accepted")
	}
	h.Set("X-Ratelimit-Resource", "search")
	if _, ok := rateFromHeader(h, now); ok {
		t.Error("search resource: must be ignored")
	}
	h.Set("X-Ratelimit-Resource", "core")

	for _, bad := range []struct{ key, value string }{
		{"X-Ratelimit-Remaining", ""}, {"X-Ratelimit-Remaining", "-1"}, {"X-Ratelimit-Remaining", "abc"},
		{"X-Ratelimit-Reset", ""}, {"X-Ratelimit-Reset", "0"}, {"X-Ratelimit-Reset", "tomorrow"},
	} {
		h2 := h.Clone()
		h2.Set(bad.key, bad.value)
		if _, ok := rateFromHeader(h2, now); ok {
			t.Errorf("%s = %q: must be refused", bad.key, bad.value)
		}
	}
	if _, ok := rateFromHeader(http.Header{}, now); ok {
		t.Error("without headers: must return false")
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
		{"soon", 0, false},
	}
	for _, tt := range tests {
		h := http.Header{}
		if tt.value != "" {
			h.Set("Retry-After", tt.value)
		}
		got, ok := retryAfter(h, now)
		if got != tt.want || ok != tt.ok {
			t.Errorf("retryAfter(%q) = %s, %v; want %s, %v", tt.value, got, ok, tt.want, tt.ok)
		}
	}
	h := http.Header{}
	h.Set("Retry-After", "99999999999999")
	if got, ok := retryAfter(h, now); !ok || got != 365*24*time.Hour {
		t.Errorf("oversized Retry-After = %s, %v; want one year at most", got, ok)
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
		t.Errorf("final Remaining = %d, want the observed minimum %d", rl.Remaining, 5000-7*200-199)
	}
}
