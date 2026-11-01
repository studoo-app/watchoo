package github

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

// apiNow is the fixed clock of the quota tests.
var apiNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60))

func TestRateLimit(t *testing.T) {
	s := apiNewServer(t)
	s.handle("/rate_limit", apiStatic(http.StatusOK, apiFixture(t, "rate_limit.json")))
	p := apiProvider(t, s, Options{})
	p.tr.now = func() time.Time { return apiNow }

	if rl, ok := p.LastRateLimit(); ok {
		t.Fatalf("LastRateLimit before any request = %+v, true", rl)
	}
	rl, err := p.RateLimit(context.Background())
	if err != nil {
		t.Fatalf("RateLimit: %v", err)
	}
	want := provider.RateLimit{
		Limit:      5000,
		Remaining:  4880,
		Used:       120,
		Reset:      time.Unix(1792000000, 0).UTC(),
		ObservedAt: apiNow.UTC(),
	}
	if rl != want {
		t.Errorf("RateLimit =\n%+v\nwant\n%+v", rl, want)
	}
	if rl.Reset.Location() != time.UTC || rl.ObservedAt.Location() != time.UTC {
		t.Error("Reset and ObservedAt must be in UTC")
	}
	last, ok := p.LastRateLimit()
	if !ok || last != want {
		t.Errorf("LastRateLimit = %+v, %v; want %+v", last, ok, want)
	}
	reqs := s.requests()
	if len(reqs) != 1 || reqs[0].Path != "/rate_limit" || reqs[0].Method != http.MethodGet {
		t.Errorf("requests = %+v", reqs)
	}
}

// TestRateLimitShared: the quota state is shared between the Providers built with the same Quota and
// the same token (one sync after the other); another token does not see it.
func TestRateLimitShared(t *testing.T) {
	s := apiNewServer(t)
	s.handle("/rate_limit", apiStatic(http.StatusOK, apiFixture(t, "rate_limit.json")))
	quota := NewQuota()
	first := apiProvider(t, s, Options{Quota: quota})
	if _, err := first.RateLimit(context.Background()); err != nil {
		t.Fatalf("RateLimit: %v", err)
	}

	next := apiProvider(t, s, Options{Quota: quota})
	if rl, ok := next.LastRateLimit(); !ok || rl.Remaining != 4880 {
		t.Errorf("LastRateLimit of the next Provider = %+v, %v; want the shared state", rl, ok)
	}

	other, err := New(testToken+"_autre", Options{BaseURL: s.srv.URL, Quota: quota})
	if err != nil {
		t.Fatal(err)
	}
	if rl, ok := other.LastRateLimit(); ok {
		t.Errorf("another token sees the quota state: %+v", rl)
	}
}

func TestRateLimitUnexpected(t *testing.T) {
	for label, body := range map[string]string{
		"empty object":  `{}`,
		"without core":  `{"resources":{"search":{"limit":30,"used":0,"remaining":30,"reset":1791996460}}}`,
		"core null":     `{"resources":{"core":null}}`,
		"without reset": `{"resources":{"core":{"limit":5000,"used":0,"remaining":5000}}}`,
		"null":          `null`,
	} {
		t.Run(label, func(t *testing.T) {
			s := apiNewServer(t)
			s.handle("/rate_limit", apiStatic(http.StatusOK, []byte(body)))
			p := apiProvider(t, s, Options{})
			_, err := p.RateLimit(context.Background())
			if !errors.Is(err, provider.ErrUnexpected) {
				t.Fatalf("RateLimit: %v, want ErrUnexpected", err)
			}
			apiAPIError(t, err, "reading the GitHub API rate limit", http.StatusOK)
			if rl, ok := p.LastRateLimit(); ok {
				t.Errorf("LastRateLimit = %+v after an unusable response", rl)
			}
		})
	}
}

func TestRateLimitErrors(t *testing.T) {
	s := apiNewServer(t)
	s.handle("/rate_limit", apiStatic(http.StatusUnauthorized, []byte(`{"message":"Bad credentials","documentation_url":"https://docs.github.com/rest","status":"401"}`)))
	p := apiProvider(t, s, Options{})
	_, err := p.RateLimit(context.Background())
	if !errors.Is(err, provider.ErrUnauthorized) {
		t.Fatalf("RateLimit: %v, want ErrUnauthorized", err)
	}
	apiAPIError(t, err, "reading the GitHub API rate limit", http.StatusUnauthorized)
	apiNoLeak(t, err, "Bad credentials")
}
