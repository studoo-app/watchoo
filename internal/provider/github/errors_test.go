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

// trSentinels is the list of the sentinel errors of the provider package, to check that a converted
// error satisfies no other than the expected ones.
var trSentinels = map[string]error{
	"ErrNotFound": provider.ErrNotFound, "ErrUnauthorized": provider.ErrUnauthorized,
	"ErrForbidden": provider.ErrForbidden, "ErrRateLimited": provider.ErrRateLimited,
	"ErrEmptyRepository": provider.ErrEmptyRepository, "ErrUnknownBase": provider.ErrUnknownBase,
	"ErrWriteForbidden": provider.ErrWriteForbidden, "ErrInvalidRef": provider.ErrInvalidRef,
	"ErrUnavailable": provider.ErrUnavailable, "ErrUnexpected": provider.ErrUnexpected,
}

// trCheckSentinels checks that err satisfies exactly the sentinels want.
func trCheckSentinels(t *testing.T, err error, want ...error) {
	t.Helper()
	for name, s := range trSentinels {
		expected := false
		for _, w := range want {
			expected = expected || w == s
		}
		if got := errors.Is(err, s); got != expected {
			t.Errorf("errors.Is(%q, %s) = %v, want %v", err, name, got, expected)
		}
	}
}

// trErrorBody is the error body of the simulated responses: it holds data that must never end up in
// a Watchoo error.
var trErrorBody = fmt.Sprintf(`{"message":"%s %s","documentation_url":"https://docs.github.com/rest"}`,
	trSecretMessage, trSecretEmail)

// trReply returns a handler that answers status with the headers h and the body body.
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

// trConvert sends GET /repos/alice/projet?per_page=100 to a server served by h and returns the
// converted error, after checking the common fields of the APIError and the absence of leaks.
func trConvert(t *testing.T, h http.Handler, opts Options, wantStatus int) (*Provider, error) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	clock := newTrClock(trEpoch)
	p := trNew(t, srv.URL, testToken, opts, clock)
	const op = "reading repository alice/projet"
	_, resp, err := trGet(context.Background(), p, "repos/alice/projet?per_page=100")
	if err == nil {
		t.Fatal("want an error")
	}
	e := p.convertError(context.Background(), op, resp, err)
	trCheckAPIError(t, e, op, http.MethodGet, "/repos/alice/projet", wantStatus)
	trNoLeak(t, e, strings.TrimPrefix(srv.URL, "http://"))
	return p, e
}

// trCheckAPIError checks the fields of the *APIError e.
func trCheckAPIError(t *testing.T, e error, op, method, path string, status int) *provider.APIError {
	t.Helper()
	var apiErr *provider.APIError
	if !errors.As(e, &apiErr) {
		t.Fatalf("%T %v: want *provider.APIError", e, e)
	}
	if apiErr.Op != op || apiErr.Method != method || apiErr.Path != path || apiErr.Status != status {
		t.Errorf("APIError = {Op %q, Method %q, Path %q, Status %d}, want {%q, %q, %q, %d}",
			apiErr.Op, apiErr.Method, apiErr.Path, apiErr.Status, op, method, path, status)
	}
	return apiErr
}

// trNoLeak checks that the message of e contains neither token, nor response body, nor request
// parameter, nor any of the extra fragments.
func trNoLeak(t *testing.T, e error, extra ...string) {
	t.Helper()
	msg := e.Error()
	for _, f := range append([]string{testToken, "Bearer", trSecretMessage, trSecretEmail, "per_page", "docs.github.com"}, extra...) {
		if f != "" && strings.Contains(msg, f) {
			t.Errorf("the message %q contains %q", msg, f)
		}
	}
}

// TestConvertErrorStatus checks the conversion of every HTTP error code.
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
		{name: "401 two-factor authentication", status: 401, headers: map[string]string{"X-GitHub-OTP": "required; app"},
			want: []error{provider.ErrUnauthorized}},
		{name: "403 rate limit exhausted", status: 403,
			headers: map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Limit": "5000", "X-RateLimit-Reset": resetHeader},
			body:    `{"message":"API rate limit exceeded for user ID 1."}`,
			want:    []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				rle := trRateLimitError(t, e)
				if rle.Secondary || !rle.Reset.Equal(reset) || rle.RetryAfter != 0 {
					t.Errorf("RateLimitError = %+v", rle)
				}
			}},
		{name: "429 rate limit exhausted with Retry-After", status: 429,
			headers: map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": resetHeader, "Retry-After": "120"},
			want:    []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				rle := trRateLimitError(t, e)
				if rle.Secondary || !rle.Reset.Equal(reset) || rle.RetryAfter != 2*time.Minute {
					t.Errorf("RateLimitError = %+v", rle)
				}
			}},
		{name: "403 rate limit exhausted without reset date", status: 403,
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
					t.Errorf("the message must name the organization: %v", e)
				}
			}},
		{name: "403 SSO without link", status: 403, headers: map[string]string{"X-GitHub-SSO": "Required"},
			want: []error{provider.ErrForbidden},
			check: func(t *testing.T, p *Provider, e error) {
				if sso := trSSOError(t, e); sso.Organization != "alice" || sso.URL != "" {
					t.Errorf("SSORequiredError = %+v (want the organization taken from the path)", sso)
				}
			}},
		{name: "403 SSO dubious link", status: 403,
			headers: map[string]string{"X-GitHub-SSO": "required; url=javascript:alert(1)"},
			want:    []error{provider.ErrForbidden},
			check: func(t *testing.T, p *Provider, e error) {
				if sso := trSSOError(t, e); sso.Organization != "alice" || sso.URL != "" {
					t.Errorf("SSORequiredError = %+v", sso)
				}
			}},
		{name: "403 SSO partial results", status: 403,
			headers: map[string]string{"X-GitHub-SSO": "partial-results; organizations=21955855,20582480"},
			want:    []error{provider.ErrForbidden},
			check: func(t *testing.T, p *Provider, e error) {
				if _, ok := errors.AsType[*provider.SSORequiredError](e); ok {
					t.Error("partial-results is not an SSO request")
				}
			}},
		{name: "403 secondary limit Retry-After", status: 403, headers: map[string]string{"Retry-After": "60"},
			want: []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				rle := trRateLimitError(t, e)
				if !rle.Secondary || rle.RetryAfter != time.Minute || !rle.Reset.Equal(trEpoch.Add(time.Minute)) {
					t.Errorf("RateLimitError = %+v", rle)
				}
				if b := p.quota.blocked(p.fp, trEpoch.Add(30*time.Second)); b == nil || !b.Secondary {
					t.Error("the following requests must be held back")
				}
			}},
		{name: "403 secondary limit documentation", status: 403,
			headers: map[string]string{"X-RateLimit-Remaining": "12", "X-RateLimit-Reset": resetHeader},
			body:    `{"message":"Blocked","documentation_url":"https://docs.github.com/rest/overview/rate-limits-for-the-rest-api#about-secondary-rate-limits"}`,
			want:    []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				// Without Retry-After, one minute of waiting (GitHub's recommendation), and not
				// X-RateLimit-Reset, which is the end of the hourly quota window.
				rle := trRateLimitError(t, e)
				wantReset := trEpoch.Add(time.Minute)
				if !rle.Secondary || rle.RetryAfter != 0 || !rle.Reset.Equal(wantReset) {
					t.Errorf("RateLimitError = %+v", rle)
				}
				if b := p.quota.blocked(p.fp, trEpoch); b == nil || !b.Reset.Equal(wantReset) {
					t.Errorf("the following requests must be held back until %v: %v", wantReset, b)
				}
				if b := p.quota.blocked(p.fp, wantReset); b != nil {
					t.Errorf("the requests must no longer be held back after one minute: %v", b)
				}
			}},
		{name: "403 secondary limit message", status: 403,
			body: `{"message":"You have exceeded a Secondary Rate Limit. Please wait a few minutes."}`,
			want: []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				rle := trRateLimitError(t, e)
				if !rle.Secondary || !rle.Reset.Equal(trEpoch.Add(time.Minute)) || rle.RetryAfter != 0 {
					t.Errorf("RateLimitError = %+v", rle)
				}
				trNoLeak(t, e, "Please wait")
			}},
		{name: "429 without hint", status: 429, want: []error{provider.ErrRateLimited},
			check: func(t *testing.T, p *Provider, e error) {
				if rle := trRateLimitError(t, e); !rle.Secondary {
					t.Errorf("RateLimitError = %+v", rle)
				}
			}},
		{name: "403", status: 403, body: `{"message":"Resource not accessible by integration"}`,
			want: []error{provider.ErrForbidden}},
		{name: "451", status: 451, want: []error{provider.ErrForbidden}},
		{name: "404", status: 404, want: []error{provider.ErrNotFound}},
		{name: "409 empty repository", status: 409, body: `{"message":"Git Repository is empty."}`,
			want: []error{provider.ErrEmptyRepository}},
		{name: "500", status: 500, want: []error{provider.ErrUnavailable}},
		{name: "502", status: 502, want: []error{provider.ErrUnavailable}},
		{name: "503", status: 503, want: []error{provider.ErrUnavailable}},
		{name: "400", status: 400, want: []error{provider.ErrUnexpected}},
		{name: "422", status: 422, want: []error{provider.ErrUnexpected}},
		{name: "304 without cache entry", status: 304, body: "", want: []error{provider.ErrUnexpected}},
		{name: "202 computation in progress", status: 202, body: "{}", want: []error{provider.ErrUnexpected}},
		{name: "301 without Location", status: 301, body: "", want: []error{provider.ErrUnexpected}},
		{name: "200 JSON invalide", status: 200, body: `{"id": 42, "name": "pro`, want: []error{provider.ErrUnexpected},
			check: func(t *testing.T, p *Provider, e error) {
				if !strings.Contains(e.Error(), "JSON") {
					t.Errorf("message: %v", e)
				}
			}},
		{name: "200 JSON of another type", status: 200, body: `["` + trSecretMessage + `"]`,
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
		t.Fatalf("%v: want *provider.RateLimitError", e)
	}
	return rle
}

func trSSOError(t *testing.T, e error) *provider.SSORequiredError {
	t.Helper()
	sso, ok := errors.AsType[*provider.SSORequiredError](e)
	if !ok {
		t.Fatalf("%v: want *provider.SSORequiredError", e)
	}
	return sso
}

// TestConvertErrorRedirects checks that a redirection on the same host is followed (the path of the
// error is the one of the last request) and that a redirection to another host is refused.
func TestConvertErrorRedirects(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the other host must receive no request (Authorization present: %v)", r.Header.Get("Authorization") != "")
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
	e := p.convertError(context.Background(), "reading repository ancien/projet", resp, err)
	trCheckAPIError(t, e, "reading repository ancien/projet", http.MethodGet, "/repositories/42", http.StatusNotFound)
	trCheckSentinels(t, e, provider.ErrNotFound)
	if lastAuth != "Bearer "+testToken {
		t.Error("the token must follow a redirection on the same host")
	}

	_, resp, err = trGet(context.Background(), p, "repos/alice/projet")
	e = p.convertError(context.Background(), "reading repository alice/projet", resp, err)
	trCheckAPIError(t, e, "reading repository alice/projet", http.MethodGet, "/repos/alice/projet", http.StatusMovedPermanently)
	trCheckSentinels(t, e, provider.ErrUnexpected)
	trNoLeak(t, e, "/vol")
}

// TestConvertErrorNetwork checks the conversion of transport failures: unreachable server, timeout
// before or while reading the body, connection cut.
func TestConvertErrorNetwork(t *testing.T) {
	t.Run("closed server", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()
		p, e := trConvertNoServer(t, srv.URL, Options{})
		trCheckSentinels(t, e, provider.ErrUnavailable)
		if !strings.Contains(e.Error(), "connection failed") {
			t.Errorf("message: %v", e)
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
		{"timeout before the response", block, 0},
		{"timeout while reading the body", partial(""), http.StatusOK},
		{"timeout while caching", partial(`"e"`), 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, e := trConvert(t, tt.handler, Options{Timeout: 100 * time.Millisecond}, tt.status)
			trCheckSentinels(t, e, provider.ErrUnavailable)
			if !strings.Contains(e.Error(), "timed out") {
				t.Errorf("message: %v", e)
			}
			if errors.Is(e, context.DeadlineExceeded) {
				t.Error("a client timeout must not pass for the timeout of the caller's context")
			}
		})
	}

	t.Run("incomplete body", func(t *testing.T) {
		_, e := trConvert(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = io.WriteString(w, `{"id":`)
		}), Options{}, http.StatusOK)
		trCheckSentinels(t, e, provider.ErrUnavailable)
		if !strings.Contains(e.Error(), "connection interrupted") {
			t.Errorf("message: %v", e)
		}
	})

	t.Run("base transport error", func(t *testing.T) {
		for _, tt := range []struct {
			err    error
			nature string
		}{
			{&net.DNSError{Err: "no such host", Name: "api.github.com", IsNotFound: true}, "host name not found"},
			{&tls.CertificateVerificationError{Err: errors.New("x509: certificate signed by unknown authority")}, "TLS"},
			{&net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, "connection interrupted"},
			{io.EOF, "connection interrupted"},
			{errors.New("proxyconnect tcp: dial tcp 10.0.0.1:3128: refused"), "network error"},
		} {
			base := trRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, tt.err })
			_, e := trConvertNoServer(t, "https://api.github.com/", Options{Transport: base})
			trCheckSentinels(t, e, provider.ErrUnavailable)
			if !strings.Contains(e.Error(), tt.nature) {
				t.Errorf("%T: message %q, want %q", tt.err, e, tt.nature)
			}
			trNoLeak(t, e, "api.github.com", "10.0.0.1", "x509", "no such host")
		}
	})
}

// trConvertNoServer sends GET /repos/alice/projet?per_page=100 to baseURL and returns the converted
// error (without a response: Status 0).
func trConvertNoServer(t *testing.T, baseURL string, opts Options) (*Provider, error) {
	t.Helper()
	p := trNew(t, baseURL, testToken, opts, nil)
	_, resp, err := trGet(context.Background(), p, "repos/alice/projet?per_page=100")
	if err == nil {
		t.Fatal("want an error")
	}
	e := p.convertError(context.Background(), "reading repository alice/projet", resp, err)
	trCheckAPIError(t, e, "reading repository alice/projet", http.MethodGet, "/repos/alice/projet", 0)
	trNoLeak(t, e, strings.TrimPrefix(baseURL, "http://"))
	return p, e
}

// TestConvertErrorCallerContext checks that a canceled or expired caller context gives an error that
// wraps ctx.Err(), and not ErrUnavailable.
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
			t.Errorf("%s: %v, want only %v", path, e, want)
		}
		if !strings.Contains(e.Error(), msg) {
			t.Errorf("%s: message %q, want %q", path, e, msg)
		}
		if _, ok := errors.AsType[*provider.APIError](e); !ok {
			t.Errorf("%s: want *provider.APIError", path)
		}
	}

	t.Run("canceled before", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		check(t, ctx, "repos/alice/projet", context.Canceled, "request canceled")
	})
	t.Run("canceled during", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		go func() { <-arrived; cancel() }()
		check(t, ctx, "repos/alice/projet", context.Canceled, "request canceled")
	})
	for _, path := range []string{"repos/alice/projet", "repos/alice/corps", "repos/alice/corps?etag=1"} {
		t.Run("expired "+path, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			check(t, ctx, path, context.DeadlineExceeded, "caller's deadline exceeded")
		})
	}
}

// TestConvertErrorMisc covers nil, an already converted error, an error without response or request,
// and the preventive refusal.
func TestConvertErrorMisc(t *testing.T) {
	noNetwork := trRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("no request must leave")
		return nil, errors.New("sending forbidden in this test")
	})
	p := trNew(t, "https://api.github.com/", testToken, Options{Transport: noNetwork}, nil)
	if p.convertError(context.Background(), "op", nil, nil) != nil {
		t.Error("convertError(nil) must return nil")
	}
	already := &provider.APIError{Op: "already", Err: provider.ErrNotFound}
	if got := p.convertError(context.Background(), "op", nil, already); got != already {
		t.Errorf("an APIError must be returned as is: %v", got)
	}
	e := p.convertError(context.Background(), "op", nil, errors.New("internal detail "+testToken))
	trCheckAPIError(t, e, "op", "", "", 0)
	trCheckSentinels(t, e, provider.ErrUnexpected)
	trNoLeak(t, e, "internal detail")

	// Preventive refusal (known exhausted quota): the transport error is found through *url.Error.
	p.quota.observe(p.fp, provider.RateLimit{Limit: 5000, Remaining: 0, Reset: time.Now().Add(time.Hour).UTC()})
	_, resp, err := trGet(context.Background(), p, "repos/alice/projet")
	e = p.convertError(context.Background(), "reading repository alice/projet", resp, err)
	trCheckAPIError(t, e, "reading repository alice/projet", http.MethodGet, "/repos/alice/projet", 0)
	trCheckSentinels(t, e, provider.ErrRateLimited)
}

// TestUnexpectedHelper checks unexpected with and without a response.
func TestUnexpectedHelper(t *testing.T) {
	srv := httptest.NewServer(trReply(http.StatusOK, nil, "null"))
	defer srv.Close()
	p := trNew(t, srv.URL, testToken, Options{}, nil)
	_, resp, err := trGet(context.Background(), p, "repos/alice/projet?per_page=100")
	if err != nil {
		t.Fatal(err)
	}
	e := p.unexpected("reading repository alice/projet", resp)
	trCheckAPIError(t, e, "reading repository alice/projet", http.MethodGet, "/repos/alice/projet", http.StatusOK)
	trCheckSentinels(t, e, provider.ErrUnexpected)
	e = p.unexpected("op", nil)
	trCheckAPIError(t, e, "op", "", "", 0)
}

// TestSSOHeaderParsing tests the reading of the X-GitHub-SSO header.
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
			t.Errorf("ssoURL(%q) = %q, want %q", tt.header, link, tt.url)
		}
		if got := ssoOrganization(link); got != tt.org {
			t.Errorf("ssoOrganization(%q) = %q, want %q", link, got, tt.org)
		}
	}
	for path, want := range map[string]string{
		"/repos/alice/projet": "alice", "/repos/alice": "alice", "/repos": "", "/repositories/42": "", "/repos/a b/x": "",
	} {
		if got := repoOwner(path); got != want {
			t.Errorf("repoOwner(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestConvertErrorContextDecides checks that the "caller's context done" classification depends on
// the state of the caller's context, and not on the shape of the error. Non-regression: while reading
// the body, the http.Client timeout (Options.Timeout) can bubble up the raw context.DeadlineExceeded
// value (without *url.Error or timeoutError), depending on the order in which the request context's
// deadline and the client's timer fire; it was then sometimes taken for the caller's deadline
// (intermittent "timeout while reading the body" test).
func TestConvertErrorContextDecides(t *testing.T) {
	noNetwork := trRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("no request must leave")
		return nil, errors.New("sending forbidden in this test")
	})
	p := trNew(t, "https://api.github.com/", testToken, Options{Transport: noNetwork}, nil)
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos/alice/projet?per_page=100", nil)
	reply := func(status int) *gogithub.Response {
		return &gogithub.Response{Response: &http.Response{StatusCode: status, Header: http.Header{}, Request: req}}
	}
	const op = "reading repository alice/projet"

	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	stopped, stop := context.WithCancelCause(context.Background())
	cause := errors.New("stop requested by the engine")
	stop(cause)

	tests := []struct {
		name    string
		ctx     context.Context
		err     error
		want    error  // sentinel of the provider package or expected context value
		msg     string // fragment of the message
		ctxDone bool   // the error must wrap ctx.Err(), and no sentinel of the provider package
	}{
		{"raw timeout, caller's context active", context.Background(), context.DeadlineExceeded,
			provider.ErrUnavailable, "timed out", false},
		{"wrapped timeout, caller's context active", context.Background(),
			fmt.Errorf("reading the body: %w", context.DeadlineExceeded), provider.ErrUnavailable, "timed out", false},
		{"raw timeout, caller's deadline exceeded", expired, context.DeadlineExceeded,
			context.DeadlineExceeded, "caller's deadline exceeded", true},
		{"client timeout, caller's deadline exceeded", expired, trLookalike{},
			context.DeadlineExceeded, "caller's deadline exceeded", true},
		{"cancellation, canceled context", canceled, context.Canceled, context.Canceled, "request canceled", true},
		{"custom cause, canceled context", stopped, cause, context.Canceled, "request canceled", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := p.convertError(tt.ctx, op, reply(http.StatusOK), tt.err)
			trCheckAPIError(t, e, op, http.MethodGet, "/repos/alice/projet", http.StatusOK)
			if !errors.Is(e, tt.want) {
				t.Errorf("%v: want %v", e, tt.want)
			}
			if !strings.Contains(e.Error(), tt.msg) {
				t.Errorf("message %q, want %q", e, tt.msg)
			}
			if tt.ctxDone {
				trCheckSentinels(t, e)
				return
			}
			trCheckSentinels(t, e, provider.ErrUnavailable)
			if errors.Is(e, context.DeadlineExceeded) || errors.Is(e, context.Canceled) {
				t.Errorf("%v: a client timeout must not pass for the end of the caller's context", e)
			}
		})
	}

	// Caller's context done, but unrelated error (404 response received just before): the response
	// wins.
	notFound := reply(http.StatusNotFound)
	e := p.convertError(canceled, op, notFound, &gogithub.ErrorResponse{Response: notFound.Response, Message: "Not Found"})
	trCheckSentinels(t, e, provider.ErrNotFound)
	if errors.Is(e, context.Canceled) {
		t.Errorf("%v: a 404 response must not pass for a cancellation", e)
	}
}

type trLookalike struct{}

func (trLookalike) Error() string        { return "timeout" }
func (trLookalike) Is(target error) bool { return target == context.DeadlineExceeded }
func (trLookalike) Timeout() bool        { return true }

// TestFailureNature checks the description of transport failures.
func TestFailureNature(t *testing.T) {
	tests := map[string]error{
		"host name not found":            &net.DNSError{Err: "no such host", Name: "x", IsTimeout: true},
		"timed out":                      trLookalike{},
		"request canceled":               fmt.Errorf("x: %w", context.Canceled),
		"secure connection (TLS) failed": tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"},
		"connection failed":              &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
		"connection interrupted":         io.ErrUnexpectedEOF,
		"network error":                  errors.New("other"),
	}
	for want, err := range tests {
		if got := failureNature(err); got != want {
			t.Errorf("failureNature(%v) = %q, want %q", err, got, want)
		}
	}
	if !isNetworkError(&net.OpError{Op: "read", Err: syscall.ECONNRESET}) || isNetworkError(errors.New("x")) {
		t.Error("isNetworkError")
	}
}

// TestConvertErrorGoGitHubTypes checks the recognition of go-github's error types when the response
// is only known through the error (resp nil).
func TestConvertErrorGoGitHubTypes(t *testing.T) {
	noNetwork := trRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("no request must leave")
		return nil, errors.New("sending forbidden in this test")
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
		e := p.convertError(context.Background(), "reading repository alice/projet", nil, tt.err)
		status := httpResponse(nil, tt.err).StatusCode
		trCheckAPIError(t, e, "reading repository alice/projet", http.MethodGet, "/repos/alice/projet", status)
		trCheckSentinels(t, e, tt.want)
		trNoLeak(t, e)
	}
	// Abuse without Retry-After: secondary limit, one minute of waiting (GitHub's recommendation).
	e := p.convertError(context.Background(), "op", nil, &gogithub.AbuseRateLimitError{Response: response(403, nil)})
	if rle := trRateLimitError(t, e); !rle.Secondary || !rle.Reset.Equal(trEpoch.Add(time.Minute)) || rle.RetryAfter != 0 {
		t.Errorf("RateLimitError = %+v", rle)
	}
}
