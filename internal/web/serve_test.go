package web

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestServeAndShutdown sert sur un vrai port local, puis arrête proprement en annulant le contexte :
// Serve rend la main sans erreur, le coordinateur est fermé (plus de synchro possible).
func TestServeAndShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	e := newTestEnv(t, withListenAddr(ln.Addr().String()))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ServeOptions{Listener: ln, Handler: e.handler, Syncs: e.coord, ShutdownTimeout: 2 * time.Second})
	}()

	resp, err := http.Get("http://" + ln.Addr().String() + "/fragments/sync")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(body) == 0 {
		t.Fatalf("GET : %d, %d octets", resp.StatusCode, len(body))
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve : %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve ne s'est pas arrêté")
	}
	if err := e.coord.SyncAll(); err == nil {
		t.Error("synchro encore possible après l'arrêt")
	}
	if _, err := http.Get("http://" + ln.Addr().String() + "/"); err == nil {
		t.Error("le serveur répond encore après l'arrêt")
	}
}

// TestServeCancelsRunningSync : l'arrêt annule la synchro en cours et l'attend.
func TestServeCancelsRunningSync(t *testing.T) {
	gate := newGateSyncer()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	e := newTestEnv(t, withSyncer(gate), withListenAddr(ln.Addr().String()))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ServeOptions{Listener: ln, Handler: e.handler, Syncs: e.coord}) }()
	if err := e.coord.SyncAll(); err != nil {
		t.Fatal(err)
	}
	<-gate.started
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Serve : %v", err)
	}
	if st := e.coord.State(); st.Running || st.LastError != "synchro annulée" {
		t.Errorf("état après l'arrêt : %+v", st)
	}
}

func TestServeRequiresListener(t *testing.T) {
	if err := Serve(t.Context(), ServeOptions{}); err == nil {
		t.Error("Serve sans écoute accepté")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	err = Serve(t.Context(), ServeOptions{Listener: ln, Handler: http.NotFoundHandler()})
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		t.Errorf("écoute fermée : %v", err)
	}
}
