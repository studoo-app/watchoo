package web

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestIsLoopbackHost(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1": true, "127.1.2.3": true, "::1": true, "localhost": true, "LocalHost": true,
		"::ffff:127.0.0.1": true,
		"":                 false, "0.0.0.0": false, "::": false, "192.168.1.10": false, "10.0.0.1": false,
		"example.com": false, "localhost.example.com": false, "[::1]": false,
	} {
		if got := IsLoopbackHost(host); got != want {
			t.Errorf("IsLoopbackHost(%q) = %t, want %t", host, got, want)
		}
	}
}

func TestCheckExposure(t *testing.T) {
	good := Auth{Username: "prof", Password: "a-long-password"}
	tests := []struct {
		name    string
		addr    string
		auth    Auth
		trusted bool
		hosts   []string // web.allowed_hosts
		wantErr string   // empty = accepted
		warn    bool
	}{
		{"loopback without auth", "127.0.0.1:8080", Auth{}, false, nil, "", false},
		{"localhost", "localhost:8080", Auth{}, false, nil, "", false},
		{"local ipv6", "[::1]:8080", Auth{}, false, nil, "", false},
		{"all interfaces without auth", ":8080", Auth{}, false, nil, "refused without authentication", false},
		{"0.0.0.0 without auth", "0.0.0.0:8080", Auth{}, false, nil, "WATCHOO_WEB_PASSWORD", false},
		{"network with auth", "0.0.0.0:8080", good, false, nil, "", false},
		{"network behind a declared proxy", "192.168.1.5:8080", Auth{}, true, []string{"watchoo.example.org"}, "", true},
		{"declared proxy without public names", "0.0.0.0:8080", Auth{}, true, nil, "web.allowed_hosts", false},
		{"declared proxy on loopback", "127.0.0.1:8080", Auth{}, true, nil, "", false},
		{"login without password", "127.0.0.1:8080", Auth{Username: "prof"}, false, nil, "WATCHOO_WEB_PASSWORD variable is empty", false},
		{"password without login", "127.0.0.1:8080", Auth{Password: "a-long-password"}, false, nil, "web.auth.username is empty", false},
		{"password too short", "0.0.0.0:8080", Auth{Username: "prof", Password: "x1y2z3"}, false, nil, "too short", false},
		{"invalid address", "8080", Auth{}, false, nil, "invalid", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warn, err := CheckExposure(tt.addr, tt.auth, tt.trusted, tt.hosts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %v, want %q", err, tt.wantErr)
			}
			if (warn != "") != tt.warn {
				t.Errorf("warning %q, want: %t", warn, tt.warn)
			}
			for _, s := range []string{warn, errString(err)} {
				if tt.auth.Password != "" && strings.Contains(s, tt.auth.Password) {
					t.Errorf("the password appears in %q", s)
				}
			}
		})
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestAllowedHosts(t *testing.T) {
	got, err := allowedHosts("127.0.0.1:8080", nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"localhost:8080", "127.0.0.1:8080", "[::1]:8080"}
	if !slices.Equal(got.exact, want) || len(got.names) != 0 {
		t.Errorf("allowedHosts = %+v, want %v", got, want)
	}
	got, _ = allowedHosts("127.0.0.1:80", nil, false, false)
	for _, h := range []string{"localhost", "127.0.0.1", "[::1]", "localhost:80"} {
		if !got.allows(h) {
			t.Errorf("port 80: %q refused (%+v)", h, got)
		}
	}
	got, _ = allowedHosts("127.0.0.2:9000", nil, false, false)
	if !got.allows("127.0.0.2:9000") {
		t.Errorf("listened host refused: %+v", got)
	}
	// Outside the loopback with basic authentication and without web.allowed_hosts: no check.
	for _, addr := range []string{"0.0.0.0:8080", ":8080"} {
		if got, err := allowedHosts(addr, nil, false, true); err != nil || got != nil {
			t.Errorf("allowedHosts(%q, auth) = %+v, %v; want nil (no check)", addr, got, err)
		}
	}
	// Reverse proxy declared without basic authentication: the check remains active, even on the
	// loopback (DNS rebinding to 127.0.0.1 or to the address on the local network).
	for _, addr := range []string{"127.0.0.1:8080", "0.0.0.0:8080", "192.168.1.5:8080"} {
		got, err := allowedHosts(addr, []string{"Watchoo.Example.org", "[fe80::1]"}, true, false)
		if err != nil || got == nil {
			t.Fatalf("allowedHosts(%q, proxy) = %+v, %v; want a check", addr, got, err)
		}
		for _, h := range []string{"watchoo.example.org", "WATCHOO.example.org:443", "[fe80::1]:8443", "127.0.0.1:8080"} {
			if !got.allows(h) {
				t.Errorf("%s: Host %q refused", addr, h)
			}
		}
		for _, h := range []string{"evil.example.test", "evil.example.test:8080", "watchoo.example.org.evil.test", "", "0.0.0.0:8080"} {
			if got.allows(h) {
				t.Errorf("%s: Host %q accepted", addr, h)
			}
		}
	}
	if got, _ := allowedHosts("192.168.1.5:8080", nil, true, false); !got.allows("192.168.1.5:8080") {
		t.Error("listened address refused behind the proxy")
	}
	// web.allowed_hosts set with basic authentication outside the loopback: check active.
	if got, _ := allowedHosts("0.0.0.0:8080", []string{"watchoo.example.org"}, false, true); got.allows("evil.example.test") {
		t.Error("foreign Host accepted despite web.allowed_hosts")
	}
	if _, err := allowedHosts("not-an-address", nil, false, false); err == nil {
		t.Error("invalid address accepted")
	}
}

func TestHostCheck(t *testing.T) {
	e := newTestEnv(t)
	for _, host := range []string{"127.0.0.1:8080", "localhost:8080", "LOCALHOST:8080", "[::1]:8080"} {
		r := e.request(http.MethodGet, "/fragments/sync", nil)
		r.Host = host
		if w := e.do(r); w.Code != http.StatusOK {
			t.Errorf("Host %q: code %d, want 200", host, w.Code)
		}
	}
	for _, host := range []string{"evil.example.test", "evil.example.test:8080", "127.0.0.1:9999", "localhost", ""} {
		r := e.request(http.MethodGet, "/fragments/sync", nil)
		r.Host = host
		w := e.do(r)
		assertStatus(t, w, http.StatusMisdirectedRequest)
		// Nothing of the workspace on the public page (DNS rebinding: the page is read by another site).
		assertNotContains(t, w, testWorkspace, "sync-status", "/settings")
		assertContains(t, w, "Unexpected address")
	}
	// Static files included.
	r := e.request(http.MethodGet, "/static/app.css", nil)
	r.Host = "evil.example.test:8080"
	assertStatus(t, e.do(r), http.StatusMisdirectedRequest)
}

func TestHostCheckDisabledOffLoopbackWithAuth(t *testing.T) {
	e := newTestEnv(t, withListenAddr("0.0.0.0:8080"), withAuth("prof", "a-long-password"))
	r := e.request(http.MethodGet, "/fragments/sync", nil)
	r.Host = "watchoo.example.test"
	r.SetBasicAuth("prof", "a-long-password")
	assertStatus(t, e.do(r), http.StatusOK)
}

// Non-regression: web.trusted_proxy does not lift the check of the Host header. A proxy on the same
// machine (Watchoo on 127.0.0.1) does not prevent a page rebound to 127.0.0.1 from reaching Watchoo
// directly, without authentication.
func TestHostCheckBehindTrustedProxy(t *testing.T) {
	for _, addr := range []string{testHost, "0.0.0.0:8080"} {
		e := newTestEnv(t, withListenAddr(addr), withTrustedProxy(), withAllowedHosts("watchoo.example.test"))
		e.seed()
		for _, host := range []string{"evil.example.test:8080", "evil.example.test"} {
			r := e.request(http.MethodGet, "/", nil)
			r.Host = host
			w := e.do(r)
			assertStatus(t, w, http.StatusMisdirectedRequest)
			assertNotContains(t, w, testWorkspace)
		}
		r := e.request(http.MethodPost, "/sync", nil)
		r.Host = "evil.example.test:8080"
		assertStatus(t, e.do(r), http.StatusMisdirectedRequest)
		for _, host := range []string{"watchoo.example.test", "watchoo.example.test:443", "127.0.0.1:8080"} {
			r := e.request(http.MethodGet, "/fragments/sync", nil)
			r.Host = host
			assertStatus(t, e.do(r), http.StatusOK)
		}
	}
	// Without web.allowed_hosts, on the loopback: only the local names pass.
	e := newTestEnv(t, withTrustedProxy())
	r := e.request(http.MethodGet, "/fragments/sync", nil)
	r.Host = "evil.example.test:8080"
	assertStatus(t, e.do(r), http.StatusMisdirectedRequest)
}

func TestBasicAuth(t *testing.T) {
	e := newTestEnv(t, withAuth("prof", "a-long-password"))
	e.seed()
	cases := []struct {
		name       string
		user, pass string
		set        bool
		want       int
	}{
		{"without credentials", "", "", false, http.StatusUnauthorized},
		{"wrong password", "prof", "wrong-password", true, http.StatusUnauthorized},
		{"wrong login", "student", "a-long-password", true, http.StatusUnauthorized},
		{"right credentials", "prof", "a-long-password", true, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, target := range []string{"/fragments/sync", "/static/app.css"} {
				r := e.request(http.MethodGet, target, nil)
				if tc.set {
					r.SetBasicAuth(tc.user, tc.pass)
				}
				w := e.do(r)
				assertStatus(t, w, tc.want)
				if tc.want == http.StatusUnauthorized {
					if got := w.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Basic ") {
						t.Errorf("WWW-Authenticate = %q", got)
					}
					assertNotContains(t, w, testWorkspace, "Alice Martin", "a-long-password")
					assertContains(t, w, "Authentication required")
				}
			}
		})
	}
	// A POST without authentication is refused even before the CSRF, and starts nothing.
	r := e.request(http.MethodPost, "/sync", strings.NewReader(url.Values{}.Encode()))
	assertStatus(t, e.do(r), http.StatusUnauthorized)
	time.Sleep(10 * time.Millisecond)
	if e.coord.State().Generation != 0 {
		t.Error("sync started without authentication")
	}
	if strings.Contains(e.logs.String(), "a-long-password") {
		t.Error("password in the log")
	}
}

func TestNewRefusesUsernameWithoutPassword(t *testing.T) {
	e := newTestEnv(t)
	_, err := newServer(Options{Store: e.store, Sync: e.coord, Repos: e.engine, Auth: Auth{Username: "prof"}})
	if err == nil || !strings.Contains(err.Error(), "WATCHOO_WEB_PASSWORD") {
		t.Errorf("error %v, want a refusal that quotes WATCHOO_WEB_PASSWORD", err)
	}
	for _, opts := range []Options{{Sync: e.coord, Repos: e.engine}, {Store: e.store, Repos: e.engine}, {Store: e.store, Sync: e.coord}} {
		if _, err := New(opts); err == nil {
			t.Errorf("incomplete options accepted: %+v", opts)
		}
	}
}
