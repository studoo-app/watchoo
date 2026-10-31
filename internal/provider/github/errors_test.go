package github

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v93/github"

	"github.com/studoo-app/watchoo/internal/provider"
)

// trSentinels est la liste des erreurs sentinelles du package provider, pour vérifier qu'une erreur
// convertie n'en satisfait pas d'autres que celles attendues.
var trSentinels = map[string]error{
	"ErrNotFound": provider.ErrNotFound, "ErrUnauthorized": provider.ErrUnauthorized,
	"ErrForbidden": provider.ErrForbidden, "ErrRateLimited": provider.ErrRateLimited,
	"ErrEmptyRepository": provider.ErrEmptyRepository, "ErrUnknownBase": provider.ErrUnknownBase,
	"ErrWriteForbidden": provider.ErrWriteForbidden, "ErrInvalidRef": provider.ErrInvalidRef,
	"ErrUnavailable": provider.ErrUnavailable, "ErrUnexpected": provider.ErrUnexpected,
}

// trCheckSentinels vérifie que err satisfait exactement les sentinelles want.
func trCheckSentinels(t *testing.T, err error, want ...error) {
	t.Helper()
	for name, s := range trSentinels {
		expected := false
		for _, w := range want {
			expected = expected || w == s
		}
		if got := errors.Is(err, s); got != expected {
			t.Errorf("errors.Is(%q, %s) = %v, veut %v", err, name, got, expected)
		}
	}
}

// trErrorBody est le corps d'erreur des réponses simulées : il contient des données qui ne doivent
// jamais se retrouver dans une erreur de Watchoo.
var trErrorBody = fmt.Sprintf(`{"message":"%s %s","documentation_url":"https://docs.github.com/rest"}`,
	trSecretMessage, trSecretEmail)

// trReply renvoie un handler qui répond status avec les en-têtes h et le corps body.
func trReply(status int, h map[string]string, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for k, v := range h {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// trConvert envoie GET /repos/alice/projet?per_page=100 à un serveur servi par h et renvoie l'erreur
// convertie, après avoir vérifié les champs communs de l'APIError et l'absence de fuite.
func trConvert(t *testing.T, h http.Handler, opts Options, wantStatus int) (*Provider, error) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	clock := newTrClock(trEpoch)
	p := trNew(t, srv.URL, testToken, opts, clock)
	const op = "lecture du dépôt alice/projet"
	_, resp, err := trGet(context.Background(), p, "repos/alice/projet?per_page=100")
	if err == nil {
		t.Fatal("erreur attendue")
	}
	e := p.convertError(context.Background(), op, resp, err)
	trCheckAPIError(t, e, op, http.MethodGet, "/repos/alice/projet", wantStatus)
	trNoLeak(t, e, strings.TrimPrefix(srv.URL, "http://"))
	return p, e
}

// trCheckAPIError vérifie les champs de l'*APIError e.
func trCheckAPIError(t *testing.T, e error, op, method, path string, status int) *provider.APIError {
	t.Helper()
	var apiErr *provider.APIError
	if !errors.As(e, &apiErr) {
		t.Fatalf("%T %v : *provider.APIError attendue", e, e)
	}
	if apiErr.Op != op || apiErr.Method != method || apiErr.Path != path || apiErr.Status != status {
		t.Errorf("APIError = {Op %q, Method %q, Path %q, Status %d}, veut {%q, %q, %q, %d}",
			apiErr.Op, apiErr.Method, apiErr.Path, apiErr.Status, op, method, path, status)
	}
	return apiErr
}

// trNoLeak vérifie que le message de e ne contient ni token, ni corps de réponse, ni paramètre de
// requête, ni aucun des fragments extra.
func trNoLeak(t *testing.T, e error, extra ...string) {
	t.Helper()
	msg := e.Error()
	for _, f := range append([]string{testToken, "Bearer", trSecretMessage, trSecretEmail, "per_page", "docs.github.com"}, extra...) {
		if f != "" && strings.Contains(msg, f) {
			t.Errorf("le message %q contient %q", msg, f)
		}
	}
}

// TestConvertErrorStatus vérifie la conversion de chaque code HTTP d'erreur.
func TestConvertErrorStatus(t *testing.T) {
	reset := trEpoch.Add(42 * time.Minute)
	resetHeader := strconv.FormatInt(reset.Unix(), 10)
	tests := []struct {
		name    string
		status  int
		headers map[string]string
		body    string
		want    []error
		check   func(t *testing.T, p *Provider, e error)
	}{
		{name: "401", status: 401, want: []error{provider.ErrUnauthorized}},
		{name: "401 double authentification", status: 401, headers: map[string]string{"X-GitHub-OTP": "required; app"},
			want: []error{provider.ErrUnauthorized}},
		{name: "403 quota épuisé", status: 403,
			headers: map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Limit": "5000", "X-RateLimit-Reset": resetHeader},
			body:    `{"message":"API rate limit exceeded for user ID 1."}`,
			want:    []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				rle := trRateLimitError(t, e)
				if rle.Secondary || !rle.Reset.Equal(reset) || rle.RetryAfter != 0 {
					t.Errorf("RateLimitError = %+v", rle)
				}
			}},
		{name: "429 quota épuisé avec Retry-After", status: 429,
			headers: map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": resetHeader, "Retry-After": "120"},
			want:    []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				rle := trRateLimitError(t, e)
				if rle.Secondary || !rle.Reset.Equal(reset) || rle.RetryAfter != 2*time.Minute {
					t.Errorf("RateLimitError = %+v", rle)
				}
			}},
		{name: "403 quota épuisé sans date de remise à zéro", status: 403,
			headers: map[string]string{"X-RateLimit-Remaining": "0", "Retry-After": "30"},
			want:    []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				rle := trRateLimitError(t, e)
				if rle.Secondary || !rle.Reset.Equal(trEpoch.Add(30*time.Second)) {
					t.Errorf("RateLimitError = %+v", rle)
				}
			}},
		{name: "403 SSO", status: 403,
			headers: map[string]string{"X-GitHub-SSO": "required; url=https://github.com/orgs/ecole-exemple/sso?authorization_request=A1B2C3"},
			want:    []error{provider.ErrForbidden},
			check: func(t *testing.T, p *Provider, e error) {
				sso := trSSOError(t, e)
				if sso.Organization != "ecole-exemple" ||
					sso.URL != "https://github.com/orgs/ecole-exemple/sso?authorization_request=A1B2C3" {
					t.Errorf("SSORequiredError = %+v", sso)
				}
				trNoLeak(t, e, "A1B2C3", "authorization_request")
				if !strings.Contains(e.Error(), "ecole-exemple") {
					t.Errorf("le message doit nommer l'organisation : %v", e)
				}
			}},
		{name: "403 SSO sans lien", status: 403, headers: map[string]string{"X-GitHub-SSO": "Required"},
			want: []error{provider.ErrForbidden},
			check: func(t *testing.T, p *Provider, e error) {
				if sso := trSSOError(t, e); sso.Organization != "alice" || sso.URL != "" {
					t.Errorf("SSORequiredError = %+v (organisation tirée du chemin attendue)", sso)
				}
			}},
		{name: "403 SSO lien douteux", status: 403,
			headers: map[string]string{"X-GitHub-SSO": "required; url=javascript:alert(1)"},
			want:    []error{provider.ErrForbidden},
			check: func(t *testing.T, p *Provider, e error) {
				if sso := trSSOError(t, e); sso.Organization != "alice" || sso.URL != "" {
					t.Errorf("SSORequiredError = %+v", sso)
				}
			}},
		{name: "403 SSO résultats partiels", status: 403,
			headers: map[string]string{"X-GitHub-SSO": "partial-results; organizations=21955855,20582480"},
			want:    []error{provider.ErrForbidden},
			check: func(t *testing.T, p *Provider, e error) {
				if _, ok := errors.AsType[*provider.SSORequiredError](e); ok {
					t.Error("partial-results n'est pas une demande de SSO")
				}
			}},
		{name: "403 limite secondaire Retry-After", status: 403, headers: map[string]string{"Retry-After": "60"},
			want: []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				rle := trRateLimitError(t, e)
				if !rle.Secondary || rle.RetryAfter != time.Minute || !rle.Reset.Equal(trEpoch.Add(time.Minute)) {
					t.Errorf("RateLimitError = %+v", rle)
				}
				if b := p.quota.blocked(p.fp, trEpoch.Add(30*time.Second)); b == nil || !b.Secondary {
					t.Error("les requêtes suivantes doivent être retenues")
				}
			}},
		{name: "403 limite secondaire documentation", status: 403,
			headers: map[string]string{"X-RateLimit-Remaining": "12", "X-RateLimit-Reset": resetHeader},
			body:    `{"message":"Blocked","documentation_url":"https://docs.github.com/rest/overview/rate-limits-for-the-rest-api#about-secondary-rate-limits"}`,
			want:    []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				// Sans Retry-After, une minute d'attente (recommandation de GitHub), et non
				// X-RateLimit-Reset, qui est la fin de la fenêtre du quota horaire.
				rle := trRateLimitError(t, e)
				wantReset := trEpoch.Add(time.Minute)
				if !rle.Secondary || rle.RetryAfter != 0 || !rle.Reset.Equal(wantReset) {
					t.Errorf("RateLimitError = %+v", rle)
				}
				if b := p.quota.blocked(p.fp, trEpoch); b == nil || !b.Reset.Equal(wantReset) {
					t.Errorf("les requêtes suivantes doivent être retenues jusqu'à %v : %v", wantReset, b)
				}
				if b := p.quota.blocked(p.fp, wantReset); b != nil {
					t.Errorf("les requêtes ne doivent plus être retenues après une minute : %v", b)
				}
			}},
		{name: "403 limite secondaire message", status: 403,
			body: `{"message":"You have exceeded a Secondary Rate Limit. Please wait a few minutes."}`,
			want: []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				rle := trRateLimitError(t, e)
				if !rle.Secondary || !rle.Reset.Equal(trEpoch.Add(time.Minute)) || rle.RetryAfter != 0 {
					t.Errorf("RateLimitError = %+v", rle)
				}
				trNoLeak(t, e, "Please wait")
			}},
		{name: "429 sans indication", status: 429, want: []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				if rle := trRateLimitError(t, e); !rle.Secondary {
					t.Errorf("RateLimitError = %+v", rle)
				}
			}},
		{name: "403", status: 403, body: `{"message":"Resource not accessible by integration"}`,
			want: []error{provider.ErrForbidden}},
		{name: "451", status: 451, want: []error{provider.ErrForbidden}},
		{name: "404", status: 404, want: []error{provider.ErrNotFound}},
		{name: "409 dépôt vide", status: 409, body: `{"message":"Git Repository is empty."}`,
			want: []error{provider.ErrEmptyRepository}},
		{name: "500", status: 500, want: []error{provider.ErrUnavailable}},
		{name: "502", status: 502, want: []error{provider.ErrUnavailable}},
		{name: "503", status: 503, want: []error{provider.ErrUnavailable}},
		{name: "400", status: 400, want: []error{provider.ErrUnexpected}},
		{name: "422", status: 422, want: []error{provider.ErrUnexpected}},
		{name: "304 sans entrée de cache", status: 304, body: "", want: []error{provider.ErrUnexpected}},
		{name: "202 calcul en cours", status: 202, body: "{}", want: []error{provider.ErrUnexpected}},
		{name: "301 sans Location", status: 301, body: "", want: []error{provider.ErrUnexpected}},
		{name: "200 JSON invalide", status: 200, body: `{"id": 42, "name": "pro`, want: []error{provider.ErrUnexpected},
			check: func(t *testing.T, p *Provider, e error) {
				if !strings.Contains(e.Error(), "JSON") {
					t.Errorf("message : %v", e)
				}
			}},
		{name: "200 JSON d'un autre type", status: 200, body: `["` + trSecretMessage + `"]`,
			want: []error{provider.ErrUnexpected}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.body
			if body == "" && tt.status != 304 && tt.status != 301 {
				body = trErrorBody
			}
			p, e := trConvert(t, trReply(tt.status, tt.headers, body), Options{}, tt.status)
			trCheckSentinels(t, e, tt.want...)
			if tt.check != nil {
				tt.check(t, p, e)
			}
		})
	}
}

func trRateLimitError(t *testing.T, e error) *provider.RateLimitError {
	t.Helper()
	rle, ok := errors.AsType[*provider.RateLimitError](e)
	if !ok {
		t.Fatalf("%v : *provider.RateLimitError attendue", e)
	}
	return rle
}

func trSSOError(t *testing.T, e error) *provider.SSORequiredError {
	t.Helper()
	sso, ok := errors.AsType[*provider.SSORequiredError](e)
	if !ok {
		t.Fatalf("%v : *provider.SSORequiredError attendue", e)
	}
	return sso
}

// TestConvertErrorRedirects vérifie qu'une redirection sur le même hôte est suivie (le chemin de
// l'erreur est celui de la dernière requête) et qu'une redirection vers un autre hôte est refusée.
func TestConvertErrorRedirects(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("l'autre hôte ne doit recevoir aucune requête (Authorization présent : %v)", r.Header.Get("Authorization") != "")
	}))
	defer other.Close()
	var lastAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/ancien/projet":
			http.Redirect(w, r, "/repositories/42", http.StatusMovedPermanently)
		case "/repositories/42":
			lastAuth = r.Header.Get("Authorization")
			trReply(http.StatusNotFound, nil, trErrorBody)(w, r)
		default:
			http.Redirect(w, r, other.URL+"/vol?per_page=100", http.StatusMovedPermanently)
		}
	}))
	defer srv.Close()
	p := trNew(t, srv.URL, testToken, Options{}, nil)

	_, resp, err := trGet(context.Background(), p, "repos/ancien/projet")
	e := p.convertError(context.Background(), "lecture du dépôt ancien/projet", resp, err)
	trCheckAPIError(t, e, "lecture du dépôt ancien/projet", http.MethodGet, "/repositories/42", http.StatusNotFound)
	trCheckSentinels(t, e, provider.ErrNotFound)
	if lastAuth != "Bearer "+testToken {
		t.Error("le token doit suivre une redirection sur le même hôte")
	}

	_, resp, err = trGet(context.Background(), p, "repos/alice/projet")
	e = p.convertError(context.Background(), "lecture du dépôt alice/projet", resp, err)
	trCheckAPIError(t, e, "lecture du dépôt alice/projet", http.MethodGet, "/repos/alice/projet", http.StatusMovedPermanently)
	trCheckSentinels(t, e, provider.ErrUnexpected)
	trNoLeak(t, e, "/vol")
}

// TestConvertErrorNetwork vérifie la conversion des échecs de transport : serveur injoignable, délai
// dépassé avant ou pendant la lecture du corps, connexion coupée.
func TestConvertErrorNetwork(t *testing.T) {
	t.Run("serveur fermé", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()
		p, e := trConvertNoServer(t, srv.URL, Options{})
		trCheckSentinels(t, e, provider.ErrUnavailable)
		if !strings.Contains(e.Error(), "connection failed") {
			t.Errorf("message : %v", e)
		}
		_ = p
	})

	block := func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }
	partial := func(etag string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if etag != "" {
				w.Header().Set("ETag", etag)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":`)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}
	}
	for _, tt := range []struct {
		name    string
		handler http.HandlerFunc
		status  int
	}{
		{"délai avant la réponse", block, 0},
		{"délai pendant la lecture du corps", partial(""), http.StatusOK},
		{"délai pendant la mise en cache", partial(`"e"`), 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, e := trConvert(t, tt.handler, Options{Timeout: 100 * time.Millisecond}, tt.status)
			trCheckSentinels(t, e, provider.ErrUnavailable)
			if !strings.Contains(e.Error(), "timed out") {
				t.Errorf("message : %v", e)
			}
			if errors.Is(e, context.DeadlineExceeded) {
				t.Error("un délai du client ne doit pas passer pour le délai du contexte de l'appelant")
			}
		})
	}

	t.Run("corps incomplet", func(t *testing.T) {
		_, e := trConvert(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = io.WriteString(w, `{"id":`)
		}), Options{}, http.StatusOK)
		trCheckSentinels(t, e, provider.ErrUnavailable)
		if !strings.Contains(e.Error(), "connection interrupted") {
			t.Errorf("message : %v", e)
		}
	})

	t.Run("erreur du transport de base", func(t *testing.T) {
		for _, tt := range []struct {
			err    error
			nature string
		}{
			{&net.DNSError{Err: "no such host", Name: "api.github.com", IsNotFound: true}, "host name not found"},
			{&tls.CertificateVerificationError{Err: errors.New("x509: certificate signed by unknown authority")}, "TLS"},
			{&net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, "connection interrupted"},
			{io.EOF, "connection interrupted"},
			{errors.New("proxyconnect tcp: dial tcp 10.0.0.1:3128: refus"), "network error"},
		} {
			base := trRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, tt.err })
			_, e := trConvertNoServer(t, "https://api.github.com/", Options{Transport: base})
			trCheckSentinels(t, e, provider.ErrUnavailable)
			if !strings.Contains(e.Error(), tt.nature) {
				t.Errorf("%T : message %q, veut %q", tt.err, e, tt.nature)
			}
			trNoLeak(t, e, "api.github.com", "10.0.0.1", "x509", "no such host")
		}
	})
}

// trConvertNoServer envoie GET /repos/alice/projet?per_page=100 vers baseURL et renvoie l'erreur
// convertie (sans réponse : Status 0).
func trConvertNoServer(t *testing.T, baseURL string, opts Options) (*Provider, error) {
	t.Helper()
	p := trNew(t, baseURL, testToken, opts, nil)
	_, resp, err := trGet(context.Background(), p, "repos/alice/projet?per_page=100")
	if err == nil {
		t.Fatal("erreur attendue")
	}
	e := p.convertError(context.Background(), "lecture du dépôt alice/projet", resp, err)
	trCheckAPIError(t, e, "lecture du dépôt alice/projet", http.MethodGet, "/repos/alice/projet", 0)
	trNoLeak(t, e, strings.TrimPrefix(baseURL, "http://"))
	return p, e
}

// TestConvertErrorCallerContext vérifie qu'un contexte de l'appelant annulé ou expiré donne une erreur
// qui enveloppe ctx.Err(), et non ErrUnavailable.
func TestConvertErrorCallerContext(t *testing.T) {
	arrived := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		if r.URL.Path == "/repos/alice/corps" {
			if r.URL.Query().Get("etag") != "" {
				w.Header().Set("ETag", `"e"`)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":`)
			w.(http.Flusher).Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()
	p := trNew(t, srv.URL, testToken, Options{}, nil)

	check := func(t *testing.T, ctx context.Context, path string, want error, msg string) {
		t.Helper()
		_, resp, err := trGet(ctx, p, path)
		e := p.convertError(ctx, "lecture", resp, err)
		if !errors.Is(e, want) || errors.Is(e, provider.ErrUnavailable) || errors.Is(e, provider.ErrUnexpected) {
			t.Errorf("%s : %v, veut %v seulement", path, e, want)
		}
		if !strings.Contains(e.Error(), msg) {
			t.Errorf("%s : message %q, veut %q", path, e, msg)
		}
		if _, ok := errors.AsType[*provider.APIError](e); !ok {
			t.Errorf("%s : *provider.APIError attendue", path)
		}
	}

	t.Run("annulé avant", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		check(t, ctx, "repos/alice/projet", context.Canceled, "request canceled")
	})
	t.Run("annulé pendant", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		go func() { <-arrived; cancel() }()
		check(t, ctx, "repos/alice/projet", context.Canceled, "request canceled")
	})
	for _, path := range []string{"repos/alice/projet", "repos/alice/corps", "repos/alice/corps?etag=1"} {
		t.Run("expiré "+path, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			check(t, ctx, path, context.DeadlineExceeded, "caller's deadline exceeded")
		})
	}
}

// TestConvertErrorMisc couvre nil, une erreur déjà convertie, une erreur sans réponse ni requête et le
// refus préventif.
func TestConvertErrorMisc(t *testing.T) {
	noNetwork := trRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("aucune requête ne doit partir")
		return nil, errors.New("envoi interdit dans ce test")
	})
	p := trNew(t, "https://api.github.com/", testToken, Options{Transport: noNetwork}, nil)
	if p.convertError(context.Background(), "op", nil, nil) != nil {
		t.Error("convertError(nil) doit renvoyer nil")
	}
	already := &provider.APIError{Op: "déjà", Err: provider.ErrNotFound}
	if got := p.convertError(context.Background(), "op", nil, already); got != already {
		t.Errorf("une APIError doit être renvoyée telle quelle : %v", got)
	}
	e := p.convertError(context.Background(), "op", nil, errors.New("détail interne "+testToken))
	trCheckAPIError(t, e, "op", "", "", 0)
	trCheckSentinels(t, e, provider.ErrUnexpected)
	trNoLeak(t, e, "détail interne")

	// Refus préventif (quota épuisé connu) : l'erreur du transport est retrouvée à travers *url.Error.
	p.quota.observe(p.fp, provider.RateLimit{Limit: 5000, Remaining: 0, Reset: time.Now().Add(time.Hour).UTC()})
	_, resp, err := trGet(context.Background(), p, "repos/alice/projet")
	e = p.convertError(context.Background(), "lecture du dépôt alice/projet", resp, err)
	trCheckAPIError(t, e, "lecture du dépôt alice/projet", http.MethodGet, "/repos/alice/projet", 0)
	trCheckSentinels(t, e, provider.ErrRateLimited)
}

// TestUnexpectedHelper vérifie unexpected avec et sans réponse.
func TestUnexpectedHelper(t *testing.T) {
	srv := httptest.NewServer(trReply(http.StatusOK, nil, "null"))
	defer srv.Close()
	p := trNew(t, srv.URL, testToken, Options{}, nil)
	_, resp, err := trGet(context.Background(), p, "repos/alice/projet?per_page=100")
	if err != nil {
		t.Fatal(err)
	}
	e := p.unexpected("lecture du dépôt alice/projet", resp)
	trCheckAPIError(t, e, "lecture du dépôt alice/projet", http.MethodGet, "/repos/alice/projet", http.StatusOK)
	trCheckSentinels(t, e, provider.ErrUnexpected)
	e = p.unexpected("op", nil)
	trCheckAPIError(t, e, "op", "", "", 0)
}

// TestSSOHeaderParsing teste la lecture de l'en-tête X-GitHub-SSO.
func TestSSOHeaderParsing(t *testing.T) {
	tests := []struct {
		header, url, org string
		required         bool
	}{
		{"required; url=https://github.com/orgs/ecole/sso?authorization_request=X", "https://github.com/orgs/ecole/sso?authorization_request=X", "ecole", true},
		{` REQUIRED ;  url = "https://github.com/orgs/ecole-2/sso" `, "https://github.com/orgs/ecole-2/sso", "ecole-2", true},
		{"required", "", "", true},
		{"required; url=http://github.com/orgs/ecole/sso", "", "", true},
		{"required; url=https://user:pw@github.com/orgs/ecole/sso", "", "", true},
		{"required; url=https://github.com/orgs/-mauvais-/sso", "https://github.com/orgs/-mauvais-/sso", "", true},
		{"required; url=https://github.com/enterprises/x/sso", "https://github.com/enterprises/x/sso", "", true},
		{"required; url=https://github.com/orgs/ecole/sso\x01", "", "", true},
		{"partial-results; organizations=1,2", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		h := http.Header{}
		h[http.CanonicalHeaderKey(headerSSO)] = []string{tt.header}
		if got := ssoRequired(h); got != tt.required {
			t.Errorf("ssoRequired(%q) = %v", tt.header, got)
		}
		link := ssoURL(tt.header)
		if link != tt.url {
			t.Errorf("ssoURL(%q) = %q, veut %q", tt.header, link, tt.url)
		}
		if got := ssoOrganization(link); got != tt.org {
			t.Errorf("ssoOrganization(%q) = %q, veut %q", link, got, tt.org)
		}
	}
	for path, want := range map[string]string{
		"/repos/alice/projet": "alice", "/repos/alice": "alice", "/repos": "", "/repositories/42": "", "/repos/a b/x": "",
	} {
		if got := repoOwner(path); got != want {
			t.Errorf("repoOwner(%q) = %q, veut %q", path, got, want)
		}
	}
}

// TestConvertErrorContextDecides vérifie que le classement « contexte de l'appelant terminé » dépend de
// l'état du contexte de l'appelant, et non de la forme de l'erreur. Non-régression : pendant la lecture
// du corps, le délai du http.Client (Options.Timeout) peut remonter la valeur context.DeadlineExceeded
// brute (sans *url.Error ni timeoutError), selon l'ordre dans lequel l'échéance du contexte de la
// requête et le minuteur du client se déclenchent ; il était alors parfois pris pour l'échéance de
// l'appelant (test « délai pendant la lecture du corps » intermittent).
func TestConvertErrorContextDecides(t *testing.T) {
	noNetwork := trRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("aucune requête ne doit partir")
		return nil, errors.New("envoi interdit dans ce test")
	})
	p := trNew(t, "https://api.github.com/", testToken, Options{Transport: noNetwork}, nil)
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos/alice/projet?per_page=100", nil)
	reply := func(status int) *gogithub.Response {
		return &gogithub.Response{Response: &http.Response{StatusCode: status, Header: http.Header{}, Request: req}}
	}
	const op = "lecture du dépôt alice/projet"

	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	stopped, stop := context.WithCancelCause(context.Background())
	cause := errors.New("arrêt demandé par le moteur")
	stop(cause)

	tests := []struct {
		name    string
		ctx     context.Context
		err     error
		want    error  // sentinelle du package provider ou valeur du contexte attendue
		msg     string // fragment du message
		ctxDone bool   // l'erreur doit envelopper ctx.Err(), et aucune sentinelle du package provider
	}{
		{"délai brut, contexte de l'appelant actif", context.Background(), context.DeadlineExceeded,
			provider.ErrUnavailable, "timed out", false},
		{"délai enveloppé, contexte de l'appelant actif", context.Background(),
			fmt.Errorf("lecture du corps : %w", context.DeadlineExceeded), provider.ErrUnavailable, "timed out", false},
		{"délai brut, échéance de l'appelant dépassée", expired, context.DeadlineExceeded,
			context.DeadlineExceeded, "caller's deadline exceeded", true},
		{"délai du client, échéance de l'appelant dépassée", expired, trLookalike{},
			context.DeadlineExceeded, "caller's deadline exceeded", true},
		{"annulation, contexte annulé", canceled, context.Canceled, context.Canceled, "request canceled", true},
		{"cause personnalisée, contexte annulé", stopped, cause, context.Canceled, "request canceled", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := p.convertError(tt.ctx, op, reply(http.StatusOK), tt.err)
			trCheckAPIError(t, e, op, http.MethodGet, "/repos/alice/projet", http.StatusOK)
			if !errors.Is(e, tt.want) {
				t.Errorf("%v : veut %v", e, tt.want)
			}
			if !strings.Contains(e.Error(), tt.msg) {
				t.Errorf("message %q, veut %q", e, tt.msg)
			}
			if tt.ctxDone {
				trCheckSentinels(t, e)
				return
			}
			trCheckSentinels(t, e, provider.ErrUnavailable)
			if errors.Is(e, context.DeadlineExceeded) || errors.Is(e, context.Canceled) {
				t.Errorf("%v : un délai du client ne doit pas passer pour la fin du contexte de l'appelant", e)
			}
		})
	}

	// Contexte de l'appelant terminé, mais erreur sans rapport (réponse 404 reçue juste avant) : la
	// réponse l'emporte.
	notFound := reply(http.StatusNotFound)
	e := p.convertError(canceled, op, notFound, &gogithub.ErrorResponse{Response: notFound.Response, Message: "Not Found"})
	trCheckSentinels(t, e, provider.ErrNotFound)
	if errors.Is(e, context.Canceled) {
		t.Errorf("%v : une réponse 404 ne doit pas passer pour une annulation", e)
	}
}

type trLookalike struct{}

func (trLookalike) Error() string        { return "délai" }
func (trLookalike) Is(target error) bool { return target == context.DeadlineExceeded }
func (trLookalike) Timeout() bool        { return true }

// TestFailureNature vérifie la description des échecs de transport.
func TestFailureNature(t *testing.T) {
	tests := map[string]error{
		"host name not found":            &net.DNSError{Err: "no such host", Name: "x", IsTimeout: true},
		"timed out":                      trLookalike{},
		"request canceled":               fmt.Errorf("x : %w", context.Canceled),
		"secure connection (TLS) failed": tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"},
		"connection failed":              &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
		"connection interrupted":         io.ErrUnexpectedEOF,
		"network error":                  errors.New("autre"),
	}
	for want, err := range tests {
		if got := failureNature(err); got != want {
			t.Errorf("failureNature(%v) = %q, veut %q", err, got, want)
		}
	}
	if !isNetworkError(&net.OpError{Op: "read", Err: syscall.ECONNRESET}) || isNetworkError(errors.New("x")) {
		t.Error("isNetworkError")
	}
}

// TestConvertErrorGoGitHubTypes vérifie la reconnaissance des types d'erreur de go-github quand la
// réponse n'est connue que par l'erreur (resp nil).
func TestConvertErrorGoGitHubTypes(t *testing.T) {
	noNetwork := trRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("aucune requête ne doit partir")
		return nil, errors.New("envoi interdit dans ce test")
	})
	p := trNew(t, "https://api.github.com/", testToken, Options{Transport: noNetwork}, newTrClock(trEpoch))
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos/alice/projet?per_page=100", nil)
	response := func(status int, h map[string]string) *http.Response {
		r := &http.Response{StatusCode: status, Header: http.Header{}, Request: req}
		for k, v := range h {
			r.Header.Set(k, v)
		}
		return r
	}
	retry := 90 * time.Second
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"ErrorResponse 404", &gogithub.ErrorResponse{Response: response(404, nil), Message: trSecretMessage}, provider.ErrNotFound},
		{"RateLimitError", &gogithub.RateLimitError{Response: response(403, nil), Message: trSecretMessage}, provider.ErrRateLimited},
		{"AbuseRateLimitError", &gogithub.AbuseRateLimitError{Response: response(403, nil), RetryAfter: &retry}, provider.ErrRateLimited},
		{"TwoFactorAuthError", &gogithub.TwoFactorAuthError{Response: response(401, nil)}, provider.ErrUnauthorized},
		{"RedirectionError", &gogithub.RedirectionError{Response: response(302, nil), StatusCode: 302}, provider.ErrUnexpected},
	}
	for _, tt := range tests {
		e := p.convertError(context.Background(), "lecture du dépôt alice/projet", nil, tt.err)
		status := httpResponse(nil, tt.err).StatusCode
		trCheckAPIError(t, e, "lecture du dépôt alice/projet", http.MethodGet, "/repos/alice/projet", status)
		trCheckSentinels(t, e, tt.want)
		trNoLeak(t, e)
	}
	// Abus sans Retry-After : limite secondaire, une minute d'attente (recommandation de GitHub).
	e := p.convertError(context.Background(), "op", nil, &gogithub.AbuseRateLimitError{Response: response(403, nil)})
	if rle := trRateLimitError(t, e); !rle.Secondary || !rle.Reset.Equal(trEpoch.Add(time.Minute)) || rle.RetryAfter != 0 {
		t.Errorf("RateLimitError = %+v", rle)
	}
}
