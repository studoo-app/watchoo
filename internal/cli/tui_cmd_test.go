package cli

// Tests de « watchoo tui » : refus hors terminal, intervalle, dépendances passées au mode interactif.
// Le TUI réel n'est jamais lancé : Deps.RunTUI est remplacé.

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/tui"
)

// tuiCall garde les arguments reçus par le faux RunTUI.
type tuiCall struct {
	calls int
	deps  tui.Deps
	term  tui.IO
}

// onTUITerminal décrit un terminal Unicode sans couleur, et installe un faux RunTUI qui enregistre
// ses arguments et renvoie ret.
func onTUITerminal(h *harness, ret error) *tuiCall {
	call := &tuiCall{}
	h.Terminal = func() TermInfo { return TermInfo{IsTerminal: true, Width: 100, Unicode: true} }
	h.StdinTerminal = true
	h.RunTUI = func(_ context.Context, d tui.Deps, term tui.IO) error {
		call.calls++
		call.deps, call.term = d, term
		return ret
	}
	return call
}

func TestTUICmdRefusesNonTerminal(t *testing.T) {
	h := newWorkspace(t)
	call := onTUITerminal(h, nil)

	h.Terminal = func() TermInfo { return TermInfo{} } // sortie redirigée
	h.run("tui").mustFail(t, "doit être lancé dans un terminal interactif", "watchoo repo list")

	h.Terminal = func() TermInfo { return TermInfo{IsTerminal: true, Width: 80} }
	h.StdinTerminal = false // entrée redirigée
	h.run("tui").mustFail(t, "doit être lancé dans un terminal interactif")

	if call.calls != 0 {
		t.Errorf("RunTUI appelé %d fois hors terminal", call.calls)
	}
}

func TestTUICmdRefusesJSONAndArgs(t *testing.T) {
	h := newWorkspace(t)
	call := onTUITerminal(h, nil)
	h.run("tui", "--json").mustFail(t, "n'a pas de sortie JSON")
	h.run("tui", "extra").mustFail(t, "« watchoo tui » ne prend pas d'argument")
	if call.calls != 0 {
		t.Errorf("RunTUI appelé %d fois", call.calls)
	}
}

func TestTUICmdPassesDeps(t *testing.T) {
	h := newSyncedWorkspace(t)
	call := onTUITerminal(h, nil)
	h.Terminal = func() TermInfo { return TermInfo{IsTerminal: true, Width: 100, Unicode: true, Color: true} }
	h.Stdin = "clavier"

	r := h.run("tui").mustSucceed(t)
	if r.stdout != "" || r.stderr != "" {
		t.Errorf("sorties non vides :\nstdout : %q\nstderr : %q", r.stdout, r.stderr)
	}
	if call.calls != 1 {
		t.Fatalf("RunTUI appelé %d fois ; attendu 1", call.calls)
	}
	d := call.deps
	if d.Interval != 30*time.Minute {
		t.Errorf("Interval = %s ; attendu 30m (sync.auto, sync.interval par défaut)", d.Interval)
	}
	if d.Config.Name != "licence-projet" || d.Location == nil || d.Location.String() != "UTC" {
		t.Errorf("Config.Name = %q, Location = %v", d.Config.Name, d.Location)
	}
	if d.Version != Version || !d.Unicode || !d.Color || d.Now == nil || !d.Now().Equal(testNow) {
		t.Errorf("Version=%q Unicode=%v Color=%v Now nil=%v", d.Version, d.Unicode, d.Color, d.Now == nil)
	}
	if call.term.Out == nil || call.term.In == nil {
		t.Fatalf("IO incomplet : %+v", call.term)
	}
	if in, err := io.ReadAll(call.term.In); err != nil || string(in) != "clavier" {
		t.Errorf("IO.In = %q, %v ; attendu l'entrée standard", in, err)
	}
	if d.Data == nil || d.Syncer == nil {
		t.Fatal("Data ou Syncer absent")
	}
}

// Les dépendances reçues par le TUI lisent la base et synchronisent pendant le TUI, sans aucun log.
func TestTUICmdDepsWork(t *testing.T) {
	h := newSyncedWorkspace(t)
	var summaries, synced int
	var syncErr, readErr error
	onTUITerminal(h, nil)
	h.RunTUI = func(ctx context.Context, d tui.Deps, _ tui.IO) error {
		rows, err := d.Data.RepositorySummaries(ctx)
		summaries, readErr = len(rows), err
		rep, err := d.Syncer.SyncAll(ctx, wsync.SyncOptions{})
		synced, syncErr = rep.Count(wsync.StatusSuccess), err
		return nil
	}
	r := h.run("tui").mustSucceed(t)
	if readErr != nil || summaries != 3 {
		t.Errorf("RepositorySummaries : %d lignes, %v ; attendu 3", summaries, readErr)
	}
	if syncErr != nil || synced != 3 {
		t.Errorf("SyncAll : %d succès, %v ; attendu 3", synced, syncErr)
	}
	if r.stderr != "" {
		t.Errorf("logs ou messages sur la sortie d'erreur pendant le TUI :\n%s", r.stderr)
	}
}

func TestTUICmdInterval(t *testing.T) {
	h := newWorkspace(t)
	call := onTUITerminal(h, nil)
	for _, tt := range []struct {
		args []string
		want time.Duration
	}{
		{[]string{"tui", "--interval", "1h"}, time.Hour},
		{[]string{"tui", "--interval", "15m"}, 15 * time.Minute},
		{[]string{"tui", "--interval", "0"}, 0},
	} {
		h.run(tt.args...).mustSucceed(t)
		if call.deps.Interval != tt.want {
			t.Errorf("%v : Interval = %s ; attendu %s", tt.args, call.deps.Interval, tt.want)
		}
	}
	h.run("tui", "--interval", "10m").mustFail(t, "--interval : 10m0s est inférieur au minimum de 15 min")
	h.run("tui", "--interval", "-1h").mustFail(t, "--interval : -1h0m0s est négatif")

	// sync.auto faux : synchro manuelle, sauf --interval.
	t.Setenv("WATCHOO_SYNC_AUTO", "false")
	h.run("tui").mustSucceed(t)
	if call.deps.Interval != 0 {
		t.Errorf("sync.auto faux : Interval = %s ; attendu 0", call.deps.Interval)
	}
	h.run("tui", "--interval", "45m").mustSucceed(t)
	if call.deps.Interval != 45*time.Minute {
		t.Errorf("sync.auto faux et --interval 45m : Interval = %s", call.deps.Interval)
	}
}

func TestTUICmdReturnsTUIError(t *testing.T) {
	h := newWorkspace(t)
	onTUITerminal(h, errors.New("mode interactif : terminal perdu"))
	r := h.run("tui").mustFail(t, "mode interactif : terminal perdu")
	if strings.Count(r.stderr, "erreur : ") != 1 {
		t.Errorf("erreur affichée plusieurs fois :\n%s", r.stderr)
	}
}
