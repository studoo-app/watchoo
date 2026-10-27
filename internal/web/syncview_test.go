package web

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// gateSyncer est un faux moteur dont chaque synchro attend release (ou l'annulation).
type gateSyncer struct {
	started chan int64
	release chan struct{}
}

func newGateSyncer() *gateSyncer {
	return &gateSyncer{started: make(chan int64, 8), release: make(chan struct{})}
}

func (g *gateSyncer) run(ctx context.Context, id int64, opts wsync.SyncOptions) (wsync.Report, error) {
	opts.Progress(wsync.Event{Kind: wsync.EventStarted, Total: 2})
	opts.Progress(wsync.Event{Kind: wsync.EventRepoStarted, Total: 2, Repo: wsync.RepoReport{DisplayName: "<b>Alice</b>"}})
	g.started <- id
	select {
	case <-g.release:
	case <-ctx.Done():
		return wsync.Report{}, ctx.Err()
	}
	return wsync.Report{Repos: []wsync.RepoReport{{Status: wsync.StatusSuccess}, {Status: wsync.StatusError}}}, nil
}

func (g *gateSyncer) SyncAll(ctx context.Context, opts wsync.SyncOptions) (wsync.Report, error) {
	return g.run(ctx, 0, opts)
}

func (g *gateSyncer) SyncRepo(ctx context.Context, id int64, opts wsync.SyncOptions) (wsync.Report, error) {
	return g.run(ctx, id, opts)
}

func TestManualSyncAndConcurrentRefusal(t *testing.T) {
	gate := newGateSyncer()
	e := newTestEnv(t, withSyncer(gate))
	ids := e.seed()

	w := e.post("/sync", nil, true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="sync-status"`, msgSyncStarted)
	<-gate.started

	// Pendant la synchro : le fragment se rafraîchit toutes les 2 s et montre la progression.
	w = e.htmx("/fragments/sync?gen=0&run=0")
	assertContains(t, w, `hx-trigger="every 2s"`, "0/2", "&lt;b&gt;Alice&lt;/b&gt;", `run=1`)
	if w.Header().Get("HX-Trigger") != "" {
		t.Error("HX-Trigger envoyé pendant la synchro")
	}

	// Une seconde demande (tous les dépôts ou un seul) est refusée proprement.
	w = e.post("/sync", nil, true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Synchro déjà en cours", "badge-error")
	w = e.post(repoPath(ids.Bob)+"/sync", nil, true)
	assertContains(t, w, "Synchro déjà en cours")
	w = e.post("/sync", nil, false)
	assertStatus(t, w, http.StatusConflict)
	assertContains(t, w, "Synchro déjà en cours")

	close(gate.release)
	st := e.waitSyncIdle()
	// La fin est signalée au fragment qui l'a vue tourner.
	w = e.htmx("/fragments/sync?gen=" + strconv.FormatUint(st.Generation, 10) + "&run=1")
	if got := w.Header().Get("HX-Trigger"); got != SyncFinishedEvent {
		t.Errorf("HX-Trigger = %q, attendu %q", got, SyncFinishedEvent)
	}
	assertContains(t, w, `hx-trigger="every 30s"`, "1 dépôt en erreur")
	// Une page chargée après la fin ne reçoit pas l'événement.
	w = e.htmx("/fragments/sync?gen=" + strconv.FormatUint(st.Generation, 10) + "&run=0")
	if got := w.Header().Get("HX-Trigger"); got != "" {
		t.Errorf("HX-Trigger = %q pour une synchro déjà vue", got)
	}
	// Une page plus ancienne (synchro automatique terminée entre deux rafraîchissements) le reçoit.
	w = e.htmx("/fragments/sync?gen=0&run=0")
	if got := w.Header().Get("HX-Trigger"); got != SyncFinishedEvent {
		t.Errorf("HX-Trigger = %q après une synchro inconnue du fragment", got)
	}
}

func TestSyncRepoRoutes(t *testing.T) {
	gate := newGateSyncer()
	close(gate.release)
	e := newTestEnv(t, withSyncer(gate))
	ids := e.seed()

	w := e.post(repoPath(ids.Carol)+"/sync", nil, false)
	assertStatus(t, w, http.StatusSeeOther)
	if got := w.Header().Get("Location"); got != repoPath(ids.Carol) {
		t.Errorf("Location = %q", got)
	}
	if id := <-gate.started; id != ids.Carol {
		t.Errorf("dépôt synchronisé %d, attendu %d", id, ids.Carol)
	}
	e.waitSyncIdle()

	w = e.post("/sync", nil, false)
	assertStatus(t, w, http.StatusSeeOther)
	if got := w.Header().Get("Location"); got != "/settings" {
		t.Errorf("Location = %q", got)
	}
	<-gate.started
	e.waitSyncIdle()

	assertStatus(t, e.post("/repos/999/sync", nil, true), http.StatusNotFound)
}

// TestSyncWithRealEngine lance une synchro manuelle avec le vrai moteur et le faux GitHub.
func TestSyncWithRealEngine(t *testing.T) {
	e := newTestEnv(t)
	id := e.addRepo(1, "alice", "Alice Martin")
	sha := e.fake.AppendCommits(1, "main", 2, e.now.Add(-48*time.Hour), time.Hour)
	if len(sha) != 2 {
		t.Fatalf("commits factices : %v", sha)
	}
	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 4000})
	assertStatus(t, e.post(repoPath(id)+"/sync", url.Values{}, true), http.StatusOK)
	st := e.waitSyncIdle()
	if st.LastError != "" || len(st.LastReport.Repos) != 1 || st.LastReport.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("synchro : %+v", st)
	}
	summary, err := e.store.RepositorySummary(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Commits != 2 {
		t.Errorf("%d commits en base, attendu 2", summary.Commits)
	}
	w := e.htmx("/fragments/sync?gen=0&run=1")
	assertContains(t, w, "Synchro")
	assertNotContains(t, w, "quota bas")
}

// finishedFirst lance la synchro par le coordinateur et attend sa fin avant de rendre la main : la
// synchro est finie quand startSync rend le fragment (moteur qui échoue tout de suite).
type finishedFirst struct {
	SyncController
	wait func()
}

func (f finishedFirst) SyncAll() error {
	if err := f.SyncController.SyncAll(); err != nil {
		return err
	}
	f.wait()
	return nil
}

func (f finishedFirst) SyncRepo(id int64) error {
	if err := f.SyncController.SyncRepo(id); err != nil {
		return err
	}
	f.wait()
	return nil
}

// Non-régression : une synchro terminée avant le rendu de la réponse au POST est signalée tout de
// suite (sinon le sondage suivant, qui porte déjà la nouvelle génération avec run=0, ne la verrait
// jamais finir et les fragments de la page ne se rechargeraient pas).
func TestSyncFinishedBeforeResponse(t *testing.T) {
	gate := newGateSyncer()
	close(gate.release)
	e := newTestEnv(t, withSyncer(gate))
	ids := e.seed()
	e.server.sync = finishedFirst{SyncController: e.coord, wait: func() { e.waitSyncIdle() }}

	for _, target := range []string{"/sync", repoPath(ids.Bob) + "/sync"} {
		w := e.post(target, nil, true)
		assertStatus(t, w, http.StatusOK)
		<-gate.started
		if got := w.Header().Get("HX-Trigger"); got != SyncFinishedEvent {
			t.Errorf("%s : HX-Trigger = %q, attendu %q", target, got, SyncFinishedEvent)
		}
		assertContains(t, w, msgSyncStarted, "run=0")
	}
}

// Non-régression : les boutons de synchro, hors des fragments rechargés à la fin d'une synchro, ne
// sont jamais désactivés au rendu (sinon ils le resteraient après la fin de la synchro).
func TestSyncButtonsNotDisabledDuringSync(t *testing.T) {
	gate := newGateSyncer()
	e := newTestEnv(t, withSyncer(gate))
	ids := e.seed()
	assertStatus(t, e.post("/sync", nil, true), http.StatusOK)
	<-gate.started
	for _, target := range []string{"/", "/settings", repoPath(ids.Bob)} {
		w := e.get(target)
		assertStatus(t, w, http.StatusOK)
		assertNotContains(t, w, " disabled")
	}
	close(gate.release)
	e.waitSyncIdle()
}
