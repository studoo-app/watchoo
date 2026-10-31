package github

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

// apiNow est l'horloge fixe des tests du quota.
var apiNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60))

func TestRateLimit(t *testing.T) {
	s := apiNewServer(t)
	s.handle("/rate_limit", apiStatic(http.StatusOK, apiFixture(t, "rate_limit.json")))
	p := apiProvider(t, s, Options{})
	p.tr.now = func() time.Time { return apiNow }

	if rl, ok := p.LastRateLimit(); ok {
		t.Fatalf("LastRateLimit avant toute requête = %+v, true", rl)
	}
	rl, err := p.RateLimit(context.Background())
	if err != nil {
		t.Fatalf("RateLimit : %v", err)
	}
	want := provider.RateLimit{
		Limit:      5000,
		Remaining:  4880,
		Used:       120,
		Reset:      time.Unix(1792000000, 0).UTC(),
		ObservedAt: apiNow.UTC(),
	}
	if rl != want {
		t.Errorf("RateLimit =\n%+v\nveut\n%+v", rl, want)
	}
	if rl.Reset.Location() != time.UTC || rl.ObservedAt.Location() != time.UTC {
		t.Error("Reset et ObservedAt doivent être en UTC")
	}
	last, ok := p.LastRateLimit()
	if !ok || last != want {
		t.Errorf("LastRateLimit = %+v, %v ; veut %+v", last, ok, want)
	}
	reqs := s.requests()
	if len(reqs) != 1 || reqs[0].Path != "/rate_limit" || reqs[0].Method != http.MethodGet {
		t.Errorf("requêtes = %+v", reqs)
	}
}

// TestRateLimitShared : l'état du quota est partagé entre les Provider construits avec le même Quota et
// le même token (une synchro après l'autre) ; un autre token ne le voit pas.
func TestRateLimitShared(t *testing.T) {
	s := apiNewServer(t)
	s.handle("/rate_limit", apiStatic(http.StatusOK, apiFixture(t, "rate_limit.json")))
	quota := NewQuota()
	first := apiProvider(t, s, Options{Quota: quota})
	if _, err := first.RateLimit(context.Background()); err != nil {
		t.Fatalf("RateLimit : %v", err)
	}

	next := apiProvider(t, s, Options{Quota: quota})
	if rl, ok := next.LastRateLimit(); !ok || rl.Remaining != 4880 {
		t.Errorf("LastRateLimit du Provider suivant = %+v, %v ; veut l'état partagé", rl, ok)
	}

	other, err := New(testToken+"_autre", Options{BaseURL: s.srv.URL, Quota: quota})
	if err != nil {
		t.Fatal(err)
	}
	if rl, ok := other.LastRateLimit(); ok {
		t.Errorf("un autre token voit l'état du quota : %+v", rl)
	}
}

func TestRateLimitUnexpected(t *testing.T) {
	for label, body := range map[string]string{
		"objet vide":      `{}`,
		"sans core":       `{"resources":{"search":{"limit":30,"used":0,"remaining":30,"reset":1791996460}}}`,
		"core null":       `{"resources":{"core":null}}`,
		"sans remise à 0": `{"resources":{"core":{"limit":5000,"used":0,"remaining":5000}}}`,
		"null":            `null`,
	} {
		t.Run(label, func(t *testing.T) {
			s := apiNewServer(t)
			s.handle("/rate_limit", apiStatic(http.StatusOK, []byte(body)))
			p := apiProvider(t, s, Options{})
			_, err := p.RateLimit(context.Background())
			if !errors.Is(err, provider.ErrUnexpected) {
				t.Fatalf("RateLimit : %v, veut ErrUnexpected", err)
			}
			apiAPIError(t, err, "reading the GitHub API rate limit", http.StatusOK)
			if rl, ok := p.LastRateLimit(); ok {
				t.Errorf("LastRateLimit = %+v après une réponse inexploitable", rl)
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
		t.Fatalf("RateLimit : %v, veut ErrUnauthorized", err)
	}
	apiAPIError(t, err, "reading the GitHub API rate limit", http.StatusUnauthorized)
	apiNoLeak(t, err, "Bad credentials")
}
