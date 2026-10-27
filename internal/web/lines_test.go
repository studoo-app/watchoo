package web

// Lignes ajoutées et supprimées (0.5.0) : par commit (« — » si inconnues, « +0 −0 » si réellement
// nulles), et totaux par dépôt, par contributeur et pour l'espace (« dont N commits non comptés »).

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

const (
	unknownLines = `<span class="text-base-content/40" title="Lignes inconnues : pas encore comptées`
	zeroLines    = `<span class="text-success">+0</span> <span class="text-error">−0</span>`
)

// seedLines crée un dépôt de trois commits d'Alice : connu (+12 −3), vide connu (+0 −0), inconnu.
func (e *testEnv) seedLines() int64 {
	e.t.Helper()
	alice := store.Identity{Name: "Alice Martin", Email: "alice@example.test"}
	id := e.addRepo(1, "alice", "")
	c1 := e.commitWrite(1, alice, 3*time.Hour, "commit compté", "main")
	c1.Additions, c1.Deletions, c1.StatsKnown = 12, 3, true
	c2 := e.commitWrite(2, alice, 2*time.Hour, "commit vide", "main")
	c2.StatsKnown = true
	c3 := e.commitWrite(3, alice, time.Hour, "commit inconnu", "main")
	e.applySync(id, "alice", c1, c2, c3)
	return id
}

func TestLinesPerCommitAndTotals(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedLines()

	// Historique du dépôt : chaque commit avec ses lignes, « — » pour l'inconnu.
	w := e.get(repoPath(id))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<span class="text-success">+12</span> <span class="text-error">−3</span>`, zeroLines,
		unknownLines,
		// Totaux du dépôt : 1 commit non compté.
		"Lignes : ", "(dont 1 commit non compté)")

	// Tableau de bord : totaux de l'espace, carte du dépôt, fil des commits.
	w = e.get("/")
	assertStatus(t, w, http.StatusOK)
	body := w.Body.String()
	if n := strings.Count(body, "(dont 1 commit non compté)"); n != 2 {
		t.Errorf("totaux partiels sur le tableau de bord : %d, attendu 2 (compteur et carte)", n)
	}
	assertContains(t, w, `<div class="stat-title">Lignes</div>`, "ajoutées / supprimées, 3 commits", zeroLines,
		unknownLines)

	// Contributeurs : classement et page.
	assertContains(t, e.get("/contributors"), `+12</span>`, "(dont 1 commit non compté)")
	contribs := e.contributorIDs()
	assertContains(t, e.get(contributorPath(contribs["Alice Martin"])), "(dont 1 commit non compté)", zeroLines,
		unknownLines)
}

// Aucun commit compté : totaux « — », sans « dont ».
func TestLinesAllUnknown(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	w := e.get(repoPath(ids.Bob))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, unknownLines)
	assertNotContains(t, w, "non compté", zeroLines)
}

// Avertissements du dernier rapport (clone Git local indisponible…) : badge et infobulle.
func TestSyncStatusWarnings(t *testing.T) {
	e := newTestEnv(t, withSyncer(&reportSyncer{report: wsync.Report{Repos: []wsync.RepoReport{{
		DisplayName: "promo/alice", Status: wsync.StatusSuccess,
		Warnings: []string{"clone Git local indisponible, statistiques de lignes lues par l'API (limitées) : accès refusé"},
	}}}}))
	assertStatus(t, e.post("/sync", nil, true), http.StatusOK)
	e.waitSyncIdle()
	w := e.htmx("/fragments/sync?gen=0&run=0")
	assertContains(t, w, "1 avertissement", `title="promo/alice : clone Git local indisponible`)
}

// reportSyncer est un faux moteur qui renvoie aussitôt report.
type reportSyncer struct{ report wsync.Report }

func (r *reportSyncer) SyncAll(_ context.Context, opts wsync.SyncOptions) (wsync.Report, error) {
	if opts.Progress != nil {
		opts.Progress(wsync.Event{Kind: wsync.EventStarted})
		opts.Progress(wsync.Event{Kind: wsync.EventFinished})
	}
	return r.report, nil
}

func (r *reportSyncer) SyncRepo(ctx context.Context, _ int64, opts wsync.SyncOptions) (wsync.Report, error) {
	return r.SyncAll(ctx, opts)
}
