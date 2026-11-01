package cli

// Tests of "watchoo serve": options, refusal to start without authentication outside the loopback,
// real start on a free local port, sync at startup, clean shutdown.

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

// servedRun is a "watchoo serve" started in the background by startServe.
type servedRun struct {
	addr      string // address really listened
	requested string // address requested from Listen
	cancel    context.CancelFunc
	done      chan result
}

// startServe starts "watchoo serve args…"; Listen always listens on a free port of 127.0.0.1,
// whatever the requested address (kept in requested).
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
		t.Fatalf("serve stopped before listening: %d\n%s", r.code, r.stderr)
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not open a listener")
	}
	return run
}

// stop stops the server and returns the result of the command.
func (s *servedRun) stop(t *testing.T) result {
	t.Helper()
	s.cancel()
	select {
	case r := <-s.done:
		return r
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not stop")
		return result{}
	}
}

// get makes a GET on the server, with the Host header of the listened address.
func (s *servedRun) get(t *testing.T, path string) (int, string) {
	t.Helper()
	resp, err := http.Get("http://" + s.addr + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
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
	h.run("serve", "--json").mustFail(t, "has no JSON output")
	h.run("serve", "extra").mustFail(t, "takes no argument")
	h.run("serve", "--interval", "5m").mustFail(t, "--interval", "minimum of 15 min")
	h.run("serve", "--addr", "not-an-address").mustFail(t, "--addr", "web.addr")
}

func TestServeRefusesNetworkWithoutAuth(t *testing.T) {
	h := newWorkspace(t)
	called := false
	h.Listen = func(string, string) (net.Listener, error) {
		called = true
		return nil, fmt.Errorf("must not be called")
	}
	h.run("serve", "--addr", "0.0.0.0:8080").mustFail(t, "refused without authentication", "WATCHOO_WEB_PASSWORD",
		"web.trusted_proxy")
	h.run("serve", "--addr", ":8080").mustFail(t, "refused without authentication")
	// Reverse proxy declared without its public names: refused (Host header check impossible).
	t.Setenv("WATCHOO_WEB_TRUSTED_PROXY", "true")
	h.run("serve", "--addr", "0.0.0.0:8080").mustFail(t, "web.allowed_hosts")
	t.Setenv("WATCHOO_WEB_TRUSTED_PROXY", "false")

	t.Setenv("WATCHOO_WEB_AUTH_USERNAME", "prof")
	h.run("serve").mustFail(t, "WATCHOO_WEB_PASSWORD variable is empty")
	t.Setenv("WATCHOO_WEB_PASSWORD", "short")
	h.run("serve").mustFail(t, "too short")
	if called {
		t.Error("listener opened despite the refusal")
	}
}

func TestServeAddressInUse(t *testing.T) {
	h := newWorkspace(t)
	h.Listen = func(string, string) (net.Listener, error) {
		return nil, &net.OpError{Op: "listen", Net: "tcp", Err: syscall.EADDRINUSE}
	}
	h.run("serve", "--interval", "0").mustFail(t, "127.0.0.1:8080 is already in use", "--addr")
}

func TestServeRunsAndStops(t *testing.T) {
	h := newSyncedWorkspace(t)
	run := startServe(t, h, "--interval", "0", "--addr", "127.0.0.1:9123")
	if run.requested != "127.0.0.1:9123" {
		t.Errorf("requested address %q, want the one of --addr", run.requested)
	}
	code, body := run.get(t, "/fragments/sync")
	if code != http.StatusOK || !strings.Contains(body, `id="sync-status"`) {
		t.Errorf("GET /fragments/sync: %d\n%s", code, body)
	}
	code, _ = run.get(t, "/static/app.css")
	if code != http.StatusOK {
		t.Errorf("GET /static/app.css: %d", code)
	}
	r := run.stop(t).mustSucceed(t)
	for _, want := range []string{"Web interface: http://" + run.addr + "/", "Automatic sync: disabled",
		"Web interface stopped."} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output without %q:\n%s", want, r.stdout)
		}
	}
	if r.stderr != "" {
		t.Errorf("non-empty error output:\n%s", r.stderr)
	}
}

// TestServeSyncsAtStartWhenNeverSynced: without a known sync, the first automatic sync happens at
// startup; the shutdown waits for its end.
func TestServeSyncsAtStartWhenNeverSynced(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	before := h.Provider.Calls("RepositoryByID")
	run := startServe(t, h) // sync.auto and sync.interval by default: 30 min
	deadline := time.Now().Add(10 * time.Second)
	for h.Provider.Calls("RepositoryByID") == before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.Provider.Calls("RepositoryByID") == before {
		t.Error("no sync at startup")
	}
	r := run.stop(t).mustSucceed(t)
	if !strings.Contains(r.stdout, "Automatic sync: every 30 min") {
		t.Errorf("output:\n%s", r.stdout)
	}
	// Started again right after: the last sync is recent, no new sync at startup.
	after := h.Provider.Calls("RepositoryByID")
	run = startServe(t, h)
	time.Sleep(200 * time.Millisecond)
	run.stop(t).mustSucceed(t)
	if got := h.Provider.Calls("RepositoryByID"); got != after {
		t.Errorf("sync started again at startup despite a recent sync (%d calls, want %d)", got, after)
	}
}

func TestServeTrustedProxyWarns(t *testing.T) {
	h := newWorkspace(t)
	t.Setenv("WATCHOO_WEB_TRUSTED_PROXY", "true")
	t.Setenv("WATCHOO_WEB_ALLOWED_HOSTS", "watchoo.example.test")
	run := startServe(t, h, "--interval", "0", "--addr", "0.0.0.0:8080")
	r := run.stop(t).mustSucceed(t)
	if !strings.Contains(r.stderr, "warning: listening on 0.0.0.0:8080 without basic authentication") {
		t.Errorf("warning missing:\n%s", r.stderr)
	}
}

func TestServeWithBasicAuth(t *testing.T) {
	h := newWorkspace(t)
	t.Setenv("WATCHOO_WEB_AUTH_USERNAME", "prof")
	t.Setenv("WATCHOO_WEB_PASSWORD", "a-long-password")
	run := startServe(t, h, "--interval", "0", "--addr", "0.0.0.0:8080")
	if code, _ := run.get(t, "/fragments/sync"); code != http.StatusUnauthorized {
		t.Errorf("without credentials: %d, want 401", code)
	}
	req, err := http.NewRequest(http.MethodGet, "http://"+run.addr+"/fragments/sync", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("prof", "a-long-password")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("with credentials: %d, want 200", resp.StatusCode)
	}
	r := run.stop(t).mustSucceed(t)
	if strings.Contains(r.stdout+r.stderr+testLogs.String(), "a-long-password") {
		t.Error("password shown or logged")
	}
}

func TestServeCardOnTerminal(t *testing.T) {
	h := newWorkspace(t)
	h.Terminal = func() TermInfo { return TermInfo{IsTerminal: true, Width: 100, Unicode: true} }
	run := startServe(t, h, "--interval", "1h")
	r := run.stop(t).mustSucceed(t)
	for _, want := range []string{"╭", `Web interface of workspace "licence-projet"`, "Ctrl+C stops the server",
		"● Web interface: http://" + run.addr + "/", "● Automatic sync: every 1 h"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output without %q:\n%s", want, r.stdout)
		}
	}
}
