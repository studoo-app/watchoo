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
			t.Errorf("IsLoopbackHost(%q) = %t, attendu %t", host, got, want)
		}
	}
}

func TestCheckExposure(t *testing.T) {
	good := Auth{Username: "prof", Password: "un-mot-de-passe-long"}
	tests := []struct {
		name    string
		addr    string
		auth    Auth
		trusted bool
		hosts   []string // web.allowed_hosts
		wantErr string   // vide = accepté
		warn    bool
	}{
		{"boucle locale sans auth", "127.0.0.1:8080", Auth{}, false, nil, "", false},
		{"localhost", "localhost:8080", Auth{}, false, nil, "", false},
		{"ipv6 locale", "[::1]:8080", Auth{}, false, nil, "", false},
		{"toutes les interfaces sans auth", ":8080", Auth{}, false, nil, "refusée sans authentification", false},
		{"0.0.0.0 sans auth", "0.0.0.0:8080", Auth{}, false, nil, "WATCHOO_WEB_PASSWORD", false},
		{"réseau avec auth", "0.0.0.0:8080", good, false, nil, "", false},
		{"réseau derrière un proxy déclaré", "192.168.1.5:8080", Auth{}, true, []string{"watchoo.example.org"}, "", true},
		{"proxy déclaré sans noms publics", "0.0.0.0:8080", Auth{}, true, nil, "web.allowed_hosts", false},
		{"proxy déclaré en boucle locale", "127.0.0.1:8080", Auth{}, true, nil, "", false},
		{"identifiant sans mot de passe", "127.0.0.1:8080", Auth{Username: "prof"}, false, nil, "WATCHOO_WEB_PASSWORD est vide", false},
		{"mot de passe sans identifiant", "127.0.0.1:8080", Auth{Password: "un-mot-de-passe-long"}, false, nil, "web.auth.username est vide", false},
		{"mot de passe trop court", "0.0.0.0:8080", Auth{Username: "prof", Password: "x1y2z3"}, false, nil, "trop court", false},
		{"adresse invalide", "8080", Auth{}, false, nil, "invalide", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warn, err := CheckExposure(tt.addr, tt.auth, tt.trusted, tt.hosts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("erreur inattendue : %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("erreur %v, attendu %q", err, tt.wantErr)
			}
			if (warn != "") != tt.warn {
				t.Errorf("avertissement %q, attendu : %t", warn, tt.warn)
			}
			for _, s := range []string{warn, errString(err)} {
				if tt.auth.Password != "" && strings.Contains(s, tt.auth.Password) {
					t.Errorf("le mot de passe apparaît dans %q", s)
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
		t.Errorf("allowedHosts = %+v, attendu %v", got, want)
	}
	got, _ = allowedHosts("127.0.0.1:80", nil, false, false)
	for _, h := range []string{"localhost", "127.0.0.1", "[::1]", "localhost:80"} {
		if !got.allows(h) {
			t.Errorf("port 80 : %q refusé (%+v)", h, got)
		}
	}
	got, _ = allowedHosts("127.0.0.2:9000", nil, false, false)
	if !got.allows("127.0.0.2:9000") {
		t.Errorf("hôte écouté refusé : %+v", got)
	}
	// Hors boucle locale avec l'authentification basique et sans web.allowed_hosts : pas de contrôle.
	for _, addr := range []string{"0.0.0.0:8080", ":8080"} {
		if got, err := allowedHosts(addr, nil, false, true); err != nil || got != nil {
			t.Errorf("allowedHosts(%q, auth) = %+v, %v ; attendu nil (pas de contrôle)", addr, got, err)
		}
	}
	// Reverse proxy déclaré sans authentification basique : le contrôle reste actif, même en boucle
	// locale (DNS rebinding vers 127.0.0.1 ou vers l'adresse du réseau local).
	for _, addr := range []string{"127.0.0.1:8080", "0.0.0.0:8080", "192.168.1.5:8080"} {
		got, err := allowedHosts(addr, []string{"Watchoo.Example.org", "[fe80::1]"}, true, false)
		if err != nil || got == nil {
			t.Fatalf("allowedHosts(%q, proxy) = %+v, %v ; attendu un contrôle", addr, got, err)
		}
		for _, h := range []string{"watchoo.example.org", "WATCHOO.example.org:443", "[fe80::1]:8443", "127.0.0.1:8080"} {
			if !got.allows(h) {
				t.Errorf("%s : Host %q refusé", addr, h)
			}
		}
		for _, h := range []string{"evil.example.test", "evil.example.test:8080", "watchoo.example.org.evil.test", "", "0.0.0.0:8080"} {
			if got.allows(h) {
				t.Errorf("%s : Host %q accepté", addr, h)
			}
		}
	}
	if got, _ := allowedHosts("192.168.1.5:8080", nil, true, false); !got.allows("192.168.1.5:8080") {
		t.Error("adresse écoutée refusée derrière le proxy")
	}
	// web.allowed_hosts réglé avec l'authentification basique hors boucle locale : contrôle actif.
	if got, _ := allowedHosts("0.0.0.0:8080", []string{"watchoo.example.org"}, false, true); got.allows("evil.example.test") {
		t.Error("Host étranger accepté malgré web.allowed_hosts")
	}
	if _, err := allowedHosts("pas-une-adresse", nil, false, false); err == nil {
		t.Error("adresse invalide acceptée")
	}
}

func TestHostCheck(t *testing.T) {
	e := newTestEnv(t)
	for _, host := range []string{"127.0.0.1:8080", "localhost:8080", "LOCALHOST:8080", "[::1]:8080"} {
		r := e.request(http.MethodGet, "/fragments/sync", nil)
		r.Host = host
		if w := e.do(r); w.Code != http.StatusOK {
			t.Errorf("Host %q : code %d, attendu 200", host, w.Code)
		}
	}
	for _, host := range []string{"evil.example.test", "evil.example.test:8080", "127.0.0.1:9999", "localhost", ""} {
		r := e.request(http.MethodGet, "/fragments/sync", nil)
		r.Host = host
		w := e.do(r)
		assertStatus(t, w, http.StatusMisdirectedRequest)
		// Rien de l'espace sur la page publique (DNS rebinding : la page est lue par un autre site).
		assertNotContains(t, w, testWorkspace, "sync-status", "/settings")
		assertContains(t, w, "Adresse inattendue")
	}
	// Fichiers statiques compris.
	r := e.request(http.MethodGet, "/static/app.css", nil)
	r.Host = "evil.example.test:8080"
	assertStatus(t, e.do(r), http.StatusMisdirectedRequest)
}

func TestHostCheckDisabledOffLoopbackWithAuth(t *testing.T) {
	e := newTestEnv(t, withListenAddr("0.0.0.0:8080"), withAuth("prof", "un-mot-de-passe-long"))
	r := e.request(http.MethodGet, "/fragments/sync", nil)
	r.Host = "watchoo.example.test"
	r.SetBasicAuth("prof", "un-mot-de-passe-long")
	assertStatus(t, e.do(r), http.StatusOK)
}

// Non-régression : web.trusted_proxy ne lève pas le contrôle de l'en-tête Host. Un proxy sur la même
// machine (Watchoo sur 127.0.0.1) n'empêche pas une page rebindée vers 127.0.0.1 de joindre Watchoo
// directement, sans authentification.
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
	// Sans web.allowed_hosts, en boucle locale : seuls les noms locaux passent.
	e := newTestEnv(t, withTrustedProxy())
	r := e.request(http.MethodGet, "/fragments/sync", nil)
	r.Host = "evil.example.test:8080"
	assertStatus(t, e.do(r), http.StatusMisdirectedRequest)
}

func TestBasicAuth(t *testing.T) {
	e := newTestEnv(t, withAuth("prof", "un-mot-de-passe-long"))
	e.seed()
	cases := []struct {
		name       string
		user, pass string
		set        bool
		want       int
	}{
		{"sans identifiants", "", "", false, http.StatusUnauthorized},
		{"mauvais mot de passe", "prof", "faux-mot-de-passe", true, http.StatusUnauthorized},
		{"mauvais identifiant", "eleve", "un-mot-de-passe-long", true, http.StatusUnauthorized},
		{"bons identifiants", "prof", "un-mot-de-passe-long", true, http.StatusOK},
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
					assertNotContains(t, w, testWorkspace, "Alice Martin", "un-mot-de-passe-long")
				}
			}
		})
	}
	// Un POST sans authentification est refusé avant même le CSRF, et ne lance rien.
	r := e.request(http.MethodPost, "/sync", strings.NewReader(url.Values{}.Encode()))
	assertStatus(t, e.do(r), http.StatusUnauthorized)
	time.Sleep(10 * time.Millisecond)
	if e.coord.State().Generation != 0 {
		t.Error("synchro lancée sans authentification")
	}
	if strings.Contains(e.logs.String(), "un-mot-de-passe-long") {
		t.Error("mot de passe dans le journal")
	}
}

func TestNewRefusesUsernameWithoutPassword(t *testing.T) {
	e := newTestEnv(t)
	_, err := newServer(Options{Store: e.store, Sync: e.coord, Repos: e.engine, Auth: Auth{Username: "prof"}})
	if err == nil || !strings.Contains(err.Error(), "WATCHOO_WEB_PASSWORD") {
		t.Errorf("erreur %v, attendu un refus qui cite WATCHOO_WEB_PASSWORD", err)
	}
	for _, opts := range []Options{{Sync: e.coord, Repos: e.engine}, {Store: e.store, Repos: e.engine}, {Store: e.store, Sync: e.coord}} {
		if _, err := New(opts); err == nil {
			t.Errorf("options incomplètes acceptées : %+v", opts)
		}
	}
}
