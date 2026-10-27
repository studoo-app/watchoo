package cli

// Tests de « watchoo serve » : options, refus de démarrer sans authentification hors boucle locale,
// démarrage réel sur un port local libre, synchro au démarrage, arrêt propre.

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"
)

// servedRun est un « watchoo serve » lancé en arrière-plan par startServe.
type servedRun struct {
	addr      string // adresse réellement écoutée
	requested string // adresse demandée à Listen
	cancel    context.CancelFunc
	done      chan result
}

// startServe lance « watchoo serve args… » ; Listen écoute toujours sur un port libre de 127.0.0.1,
// quelle que soit l'adresse demandée (retenue dans requested).
func startServe(t *testing.T, h *harness, args ...string) *servedRun {
	t.Helper()
	run := &servedRun{done: make(chan result, 1)}
	ready := make(chan struct{})
	h.Listen = func(network, address string) (net.Listener, error) {
		run.requested = address
		ln, err := net.Listen(network, "127.0.0.1:0")
		if err == nil {
			run.addr = ln.Addr().String()
		}
		close(ready)
		return ln, err
	}
	ctx, cancel := context.WithCancel(t.Context())
	run.cancel = cancel
	go func() { run.done <- h.runContext(ctx, append([]string{"serve"}, args...)...) }()
	select {
	case <-ready:
	case r := <-run.done:
		t.Fatalf("serve s'est arrêté avant d'écouter : %d\n%s", r.code, r.stderr)
	case <-time.After(10 * time.Second):
		t.Fatal("serve n'a pas ouvert d'écoute")
	}
	return run
}

// stop arrête le serveur et renvoie le résultat de la commande.
func (s *servedRun) stop(t *testing.T) result {
	t.Helper()
	s.cancel()
	select {
	case r := <-s.done:
		return r
	case <-time.After(15 * time.Second):
		t.Fatal("serve ne s'est pas arrêté")
		return result{}
	}
}

// get fait un GET sur le serveur, avec l'en-tête Host de l'adresse écoutée.
func (s *servedRun) get(t *testing.T, path string) (int, string) {
	t.Helper()
	resp, err := http.Get("http://" + s.addr + path)
	if err != nil {
		t.Fatalf("GET %s : %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body)
}

func TestServeRefusesJSONArgsAndBadOptions(t *testing.T) {
	h := newWorkspace(t)
	h.run("serve", "--json").mustFail(t, "n'a pas de sortie JSON")
	h.run("serve", "extra").mustFail(t, "« watchoo serve » ne prend pas d'argument")
	h.run("serve", "--interval", "5m").mustFail(t, "--interval", "minimum de 15 min")
	h.run("serve", "--addr", "pas-une-adresse").mustFail(t, "--addr", "web.addr")
}

func TestServeRefusesNetworkWithoutAuth(t *testing.T) {
	h := newWorkspace(t)
	called := false
	h.Listen = func(string, string) (net.Listener, error) {
		called = true
		return nil, fmt.Errorf("ne doit pas être appelé")
	}
	h.run("serve", "--addr", "0.0.0.0:8080").mustFail(t, "refusée sans authentification", "WATCHOO_WEB_PASSWORD",
		"web.trusted_proxy")
	h.run("serve", "--addr", ":8080").mustFail(t, "refusée sans authentification")
	// Reverse proxy déclaré sans ses noms publics : refus (contrôle de l'en-tête Host impossible).
	t.Setenv("WATCHOO_WEB_TRUSTED_PROXY", "true")
	h.run("serve", "--addr", "0.0.0.0:8080").mustFail(t, "web.allowed_hosts")
	t.Setenv("WATCHOO_WEB_TRUSTED_PROXY", "false")

	t.Setenv("WATCHOO_WEB_AUTH_USERNAME", "prof")
	h.run("serve").mustFail(t, "WATCHOO_WEB_PASSWORD est vide")
	t.Setenv("WATCHOO_WEB_PASSWORD", "court")
	h.run("serve").mustFail(t, "trop court")
	if called {
		t.Error("écoute ouverte malgré le refus")
	}
}

func TestServeAddressInUse(t *testing.T) {
	h := newWorkspace(t)
	h.Listen = func(string, string) (net.Listener, error) {
		return nil, &net.OpError{Op: "listen", Net: "tcp", Err: syscall.EADDRINUSE}
	}
	h.run("serve", "--interval", "0").mustFail(t, "127.0.0.1:8080 est déjà utilisée", "--addr")
}

func TestServeRunsAndStops(t *testing.T) {
	h := newSyncedWorkspace(t)
	run := startServe(t, h, "--interval", "0", "--addr", "127.0.0.1:9123")
	if run.requested != "127.0.0.1:9123" {
		t.Errorf("adresse demandée %q, attendu celle de --addr", run.requested)
	}
	code, body := run.get(t, "/fragments/sync")
	if code != http.StatusOK || !strings.Contains(body, `id="sync-status"`) {
		t.Errorf("GET /fragments/sync : %d\n%s", code, body)
	}
	code, _ = run.get(t, "/static/app.css")
	if code != http.StatusOK {
		t.Errorf("GET /static/app.css : %d", code)
	}
	r := run.stop(t).mustSucceed(t)
	for _, want := range []string{"Interface web : http://" + run.addr + "/", "Synchro automatique : désactivée",
		"Interface web arrêtée."} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("sortie sans %q :\n%s", want, r.stdout)
		}
	}
	if r.stderr != "" {
		t.Errorf("sortie d'erreur non vide :\n%s", r.stderr)
	}
}

// TestServeSyncsAtStartWhenNeverSynced : sans synchro connue, la première synchro automatique a lieu
// au démarrage ; l'arrêt attend sa fin.
func TestServeSyncsAtStartWhenNeverSynced(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	before := h.Provider.Calls("RepositoryByID")
	run := startServe(t, h) // sync.auto et sync.interval par défaut : 30 min
	deadline := time.Now().Add(10 * time.Second)
	for h.Provider.Calls("RepositoryByID") == before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.Provider.Calls("RepositoryByID") == before {
		t.Error("aucune synchro au démarrage")
	}
	r := run.stop(t).mustSucceed(t)
	if !strings.Contains(r.stdout, "Synchro automatique : toutes les 30 min") {
		t.Errorf("sortie :\n%s", r.stdout)
	}
	// Relancé juste après : la dernière synchro est récente, pas de nouvelle synchro au démarrage.
	after := h.Provider.Calls("RepositoryByID")
	run = startServe(t, h)
	time.Sleep(200 * time.Millisecond)
	run.stop(t).mustSucceed(t)
	if got := h.Provider.Calls("RepositoryByID"); got != after {
		t.Errorf("synchro relancée au démarrage malgré une synchro récente (%d appels, attendu %d)", got, after)
	}
}

func TestServeTrustedProxyWarns(t *testing.T) {
	h := newWorkspace(t)
	t.Setenv("WATCHOO_WEB_TRUSTED_PROXY", "true")
	t.Setenv("WATCHOO_WEB_ALLOWED_HOSTS", "watchoo.example.test")
	run := startServe(t, h, "--interval", "0", "--addr", "0.0.0.0:8080")
	r := run.stop(t).mustSucceed(t)
	if !strings.Contains(r.stderr, "attention : écoute sur 0.0.0.0:8080 sans authentification basique") {
		t.Errorf("avertissement absent :\n%s", r.stderr)
	}
}

func TestServeWithBasicAuth(t *testing.T) {
	h := newWorkspace(t)
	t.Setenv("WATCHOO_WEB_AUTH_USERNAME", "prof")
	t.Setenv("WATCHOO_WEB_PASSWORD", "un-mot-de-passe-long")
	run := startServe(t, h, "--interval", "0", "--addr", "0.0.0.0:8080")
	if code, _ := run.get(t, "/fragments/sync"); code != http.StatusUnauthorized {
		t.Errorf("sans identifiants : %d, attendu 401", code)
	}
	req, err := http.NewRequest(http.MethodGet, "http://"+run.addr+"/fragments/sync", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("prof", "un-mot-de-passe-long")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("avec identifiants : %d, attendu 200", resp.StatusCode)
	}
	r := run.stop(t).mustSucceed(t)
	if strings.Contains(r.stdout+r.stderr+testLogs.String(), "un-mot-de-passe-long") {
		t.Error("mot de passe affiché ou journalisé")
	}
}

func TestServeCardOnTerminal(t *testing.T) {
	h := newWorkspace(t)
	h.Terminal = func() TermInfo { return TermInfo{IsTerminal: true, Width: 100, Unicode: true} }
	run := startServe(t, h, "--interval", "1h")
	r := run.stop(t).mustSucceed(t)
	for _, want := range []string{"╭", "Interface web de l'espace « licence-projet »", "Ctrl+C arrête le serveur",
		"● Interface web : http://" + run.addr + "/", "● Synchro automatique : toutes les 1 h"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("sortie sans %q :\n%s", want, r.stdout)
		}
	}
}
