package web

// Added and deleted lines (0.5.0): per commit ("—" if unknown, "+0 −0" if really zero), and totals
// per repository, per contributor and for the workspace ("including N commits not counted").

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
	unknownLines = `<span class="text-base-content/40" title="Lines unknown: not counted yet`
	zeroLines    = `<span class="text-success">+0</span> <span class="text-error">−0</span>`
)

// seedLines creates a repository of three commits by Alice: known (+12 −3), known empty (+0 −0),
// unknown.
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

	// History of the repository: each commit with its lines, "—" for the unknown one.
	w := e.get(repoPath(id))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<span class="text-success">+12</span> <span class="text-error">−3</span>`, zeroLines,
		unknownLines,
		// Totals of the repository: 1 commit not counted.
		"Lines: ", "(including 1 commit not counted)")

	// Dashboard: totals of the workspace, card of the repository, commit feed.
	w = e.get("/")
	assertStatus(t, w, http.StatusOK)
	body := w.Body.String()
	if n := strings.Count(body, "(including 1 commit not counted)"); n != 2 {
		t.Errorf("partial totals on the dashboard: %d, want 2 (counter and card)", n)
	}
	assertContains(t, w, `<div class="stat-title">Lines</div>`, "added / deleted, 3 commits", zeroLines,
		unknownLines)

	// Contributors: ranking and page.
	assertContains(t, e.get("/contributors"), `+12</span>`, "(including 1 commit not counted)")
	contribs := e.contributorIDs()
	assertContains(t, e.get(contributorPath(contribs["Alice Martin"])), "(including 1 commit not counted)", zeroLines,
		unknownLines)
}

// No commit counted: totals "—", without "including".
func TestLinesAllUnknown(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	w := e.get(repoPath(ids.Bob))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, unknownLines)
	assertNotContains(t, w, "(including", zeroLines)
}

// Warnings of the last report (local Git clone unavailable…): badge and tooltip.
func TestSyncStatusWarnings(t *testing.T) {
	e := newTestEnv(t, withSyncer(&reportSyncer{report: wsync.Report{Repos: []wsync.RepoReport{{
		DisplayName: "promo/alice", Status: wsync.StatusSuccess,
		Warnings: []string{"clone Git local indisponible, statistiques de lignes lues par l'API (limitées) : accès refusé"},
	}}}}))
	assertStatus(t, e.post("/sync", nil, true), http.StatusOK)
	e.waitSyncIdle()
	w := e.htmx("/fragments/sync?gen=0&run=0")
	assertContains(t, w, "1 warning", `title="promo/alice: clone Git local indisponible`)
}

// reportSyncer is a fake engine that returns report right away.
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
