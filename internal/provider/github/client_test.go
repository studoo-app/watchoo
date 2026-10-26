package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
)

// countingTransport compte les requêtes qui atteignent le transport de base.
type countingTransport struct {
	n    atomic.Int64
	next http.RoundTripper
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.n.Add(1)
	return c.next.RoundTrip(req)
}

func TestNewRejectsBadToken(t *testing.T) {
	for _, token := range []string{"", " ", "gho_a b", "gho_x\n", "gho_\x00", "\xff"} {
		p, err := New(token, Options{})
		if p != nil || !errors.Is(err, provider.ErrUnauthorized) {
			t.Errorf("New(%q) = %v, %v ; veut ErrUnauthorized", token, p, err)
		}
		if err != nil && strings.HasPrefix(token, "gho_") && strings.Contains(err.Error(), token) {
			t.Errorf("l'erreur cite le token : %v", err)
		}
	}
}

func TestNewOptions(t *testing.T) {
	p, err := New(testToken, Options{})
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	if got := p.client.BaseURL(); got != DefaultBaseURL {
		t.Errorf("BaseURL = %q, veut %q", got, DefaultBaseURL)
	}
	if p.tr.userAgent != DefaultUserAgent || p.client.UserAgent() != DefaultUserAgent {
		t.Errorf("User-Agent = %q / %q", p.tr.userAgent, p.client.UserAgent())
	}
	if p.client.Client().Timeout != DefaultTimeout {
		t.Errorf("Timeout = %s, veut %s", p.client.Client().Timeout, DefaultTimeout)
	}
	if p.cache == nil || p.quota == nil || p.logger == nil || p.tr.base != http.DefaultTransport {
		t.Error("valeurs par défaut manquantes")
	}
	if p.Name() != model.ProviderGitHub {
		t.Errorf("Name = %q", p.Name())
	}

	// Cache et quota partagés : les mêmes pointeurs.
	cache, quota := NewCache(10, 100), NewQuota()
	p2, err := New(testToken, Options{
		BaseURL: "http://127.0.0.1:8080/api/v3", UserAgent: "watchoo/v0.2.0", Timeout: time.Second,
		Cache: cache, Quota: quota,
	})
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	if p2.cache != cache || p2.quota != quota || p2.tr.cache != cache || p2.tr.quota != quota {
		t.Error("le cache et le quota passés en options doivent être partagés")
	}
	if got := p2.client.BaseURL(); got != "http://127.0.0.1:8080/api/v3/" {
		t.Errorf("BaseURL = %q (le « / » final doit être ajouté)", got)
	}
	if p2.tr.basePath != "/api/v3/" || p2.tr.origin.String() != "http://127.0.0.1:8080" {
		t.Errorf("basePath = %q, origin = %q", p2.tr.basePath, p2.tr.origin)
	}
	if p2.client.Client().Timeout != time.Second {
		t.Errorf("Timeout = %s", p2.client.Client().Timeout)
	}
	if p.fp != p2.fp {
		t.Error("même token, même empreinte attendue")
	}
}

func TestNewRejectsBadOptions(t *testing.T) {
	bad := []Options{
		{BaseURL: "http://api.github.com/"}, // http hors bouclage
		{BaseURL: "ftp://127.0.0.1/"},
		{BaseURL: "api.github.com"},
		{BaseURL: "https://user:pass@api.github.com/"},
		{BaseURL: "https://api.github.com/?x=1"},
		{BaseURL: "https://api.github.com/#f"},
		{BaseURL: "https://api.github.com/%zz"},
		{UserAgent: "watchoo\r\nX-Injecte: 1"},
		{Timeout: -time.Second},
	}
	for _, opts := range bad {
		if p, err := New(testToken, opts); err == nil {
			t.Errorf("New(%+v) = %v, veut une erreur", opts, p)
		} else if strings.Contains(err.Error(), testToken) {
			t.Errorf("l'erreur cite le token : %v", err)
		}
	}
	for _, base := range []string{"http://localhost:1234", "http://[::1]:1234/", "https://ghe.example.com/api/v3/"} {
		if _, err := New(testToken, Options{BaseURL: base}); err != nil {
			t.Errorf("New(BaseURL %q) : %v", base, err)
		}
	}
}

func TestProviderNeverPrintsToken(t *testing.T) {
	p, err := New(testToken, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		fmt.Sprintf("%v", p), fmt.Sprintf("%+v", p), fmt.Sprintf("%#v", p), fmt.Sprintf("%s", p),
		fmt.Sprintf("%v", p.tr), fmt.Sprintf("%+v", p.tr), fmt.Sprintf("%#v", p.tr),
	} {
		if strings.Contains(s, testToken) {
			t.Errorf("le token apparaît dans %q", s)
		}
	}
}

// TestRequestHeaders vérifie, sur une vraie requête de go-github, que le transport pose le token
// (Bearer) et le User-Agent.
func TestRequestHeaders(t *testing.T) {
	var gotAuth, gotUA atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		gotUA.Store(r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"resources":{"core":{"limit":5000,"remaining":4999,"used":1,"reset":1800000000}}}`)
	}))
	defer srv.Close()

	p, err := New(testToken, Options{BaseURL: srv.URL, UserAgent: "watchoo/test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.client.RateLimit.Get(context.Background()); err != nil {
		t.Fatalf("GET /rate_limit : %v", err)
	}
	if gotAuth.Load() != "Bearer "+testToken {
		t.Errorf("Authorization absent ou incorrect")
	}
	if gotUA.Load() != "watchoo/test" {
		t.Errorf("User-Agent = %v", gotUA.Load())
	}
}

// TestTokenOnlyToBaseOrigin vérifie qu'une requête vers un autre hôte part sans le token, et qu'une
// redirection vers un autre hôte est refusée.
func TestTokenOnlyToBaseOrigin(t *testing.T) {
	var otherAuth atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherAuth.Store(r.Header.Get("Authorization"))
	}))
	defer other.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/ailleurs", http.StatusMovedPermanently)
	}))
	defer api.Close()

	p, err := New(testToken, Options{BaseURL: api.URL})
	if err != nil {
		t.Fatal(err)
	}

	// Requête directe vers un autre hôte (même un en-tête posé par l'appelant est retiré).
	req, _ := http.NewRequest(http.MethodGet, other.URL+"/x", nil)
	req.Header.Set("Authorization", "Bearer autre")
	resp, err := p.tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if v, _ := otherAuth.Load().(string); v != "" {
		t.Errorf("un autre hôte a reçu Authorization")
	}
	if req.Header.Get("Authorization") != "Bearer autre" {
		t.Error("la requête de l'appelant ne doit pas être modifiée")
	}

	// Redirection vers un autre hôte : refusée par le http.Client.
	otherAuth.Store("")
	_, err = p.client.Client().Get(api.URL + "/repos/alice/projet")
	if !errors.Is(err, provider.ErrUnexpected) {
		t.Errorf("redirection vers un autre hôte : erreur = %v, veut ErrUnexpected", err)
	}
	if err != nil && strings.Contains(err.Error(), testToken) {
		t.Errorf("l'erreur cite le token : %v", err)
	}
}

// TestWriteForbidden vérifie qu'une méthode d'écriture est refusée avant d'atteindre le réseau.
func TestWriteForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("requête %s reçue : elle aurait dû être bloquée", r.Method)
	}))
	defer srv.Close()
	base := &countingTransport{next: http.DefaultTransport}
	p, err := New(testToken, Options{BaseURL: srv.URL, Transport: base})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete, "OPTIONS"} {
		req, err := http.NewRequest(method, srv.URL+"/repos/alice/projet", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.client.Client().Do(req)
		if !errors.Is(err, provider.ErrWriteForbidden) {
			t.Errorf("%s : erreur = %v, veut ErrWriteForbidden", method, err)
		}
	}
	if n := base.n.Load(); n != 0 {
		t.Errorf("%d requête(s) ont atteint le transport de base", n)
	}
}

func TestSameOrigin(t *testing.T) {
	origin := &url.URL{Scheme: "https", Host: "api.github.com"}
	tests := map[string]bool{
		"https://api.github.com/repos/a/b":     true,
		"https://API.GitHub.com/x":             true,
		"https://api.github.com:443/x":         true,
		"http://api.github.com/x":              false,
		"https://api.github.com:8443/x":        false,
		"https://uploads.github.com/x":         false,
		"https://api.github.com.evil.test/x":   false,
		"https://evil.test/https://api.github": false,
	}
	for raw, want := range tests {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := sameOrigin(u, origin); got != want {
			t.Errorf("sameOrigin(%q) = %v, veut %v", raw, got, want)
		}
	}
	if sameOrigin(nil, origin) || sameOrigin(origin, nil) {
		t.Error("sameOrigin avec nil doit renvoyer false")
	}
}

func TestCheckRedirect(t *testing.T) {
	origin := &url.URL{Scheme: "https", Host: "api.github.com"}
	check := checkRedirect(origin)
	same, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repositories/42", nil)
	if err := check(same, make([]*http.Request, 1)); err != nil {
		t.Errorf("redirection sur le même hôte : %v", err)
	}
	if err := check(same, make([]*http.Request, maxRedirects)); !errors.Is(err, provider.ErrUnexpected) {
		t.Errorf("trop de redirections : %v", err)
	}
	foreign, _ := http.NewRequest(http.MethodGet, "https://evil.test/x?token=1", nil)
	err := check(foreign, make([]*http.Request, 1))
	if !errors.Is(err, provider.ErrUnexpected) || strings.Contains(err.Error(), "token=1") {
		t.Errorf("redirection vers un autre hôte : %v", err)
	}
}

func TestAPIPath(t *testing.T) {
	tr := &transport{basePath: "/api/v3/"}
	u, _ := url.Parse("https://ghe.example.com/api/v3/repos/alice/projet/commits?sha=main&page=2")
	if got := tr.apiPath(u); got != "/repos/alice/projet/commits" {
		t.Errorf("apiPath = %q", got)
	}
	tr = &transport{basePath: "/"}
	u, _ = url.Parse("https://api.github.com/repositories/42")
	if got := tr.apiPath(u); got != "/repositories/42" {
		t.Errorf("apiPath = %q", got)
	}
	if got := tr.apiPath(nil); got != "" {
		t.Errorf("apiPath(nil) = %q", got)
	}
}
