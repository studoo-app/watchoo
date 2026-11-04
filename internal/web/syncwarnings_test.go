package web

// Warnings of the last sync of each repository (1.3.0): badge on the dashboard card, alert on the
// repository's page, read from the database (they survive a restart), in the language of the page.

import (
	"html"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// cloneWarning is the warning recorded during a French sync (process language).
const cloneWarning = "clone Git local indisponible, statistiques de lignes lues par l'API (limitées) : accès refusé"

// seedWarnings records two warnings on the last sync of promo/alice and a failed sync on promo/bob.
func (e *testEnv) seedWarnings(ids seededRepos) {
	e.t.Helper()
	ctx := e.t.Context()
	last, err := e.store.LastSyncRun(ctx, ids.Alice)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.store.SetSyncRunWarnings(ctx, last.ID, []model.SyncWarning{
		{Kind: model.WarnCloneUnavailable, Message: cloneWarning},
		{Kind: model.WarnListsNotSynced, Message: "tags non synchronisés : HTTP 502"},
	}); err != nil {
		e.t.Fatal(err)
	}
	run, err := e.store.BeginSyncRun(ctx, ids.Bob, e.now.Add(-10*time.Minute), wsync.LockStaleAfter)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.store.FinishSyncRun(ctx, run.ID, e.now.Add(-9*time.Minute), model.SyncError, 0,
		"mise à jour du dépôt promo/bob (git, code 128) : accès refusé"); err != nil {
		e.t.Fatal(err)
	}
}

func TestDashboardWarningBadges(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	e.seedWarnings(ids)

	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		`href="/repos/`+strconv.FormatInt(ids.Alice, 10)+`#repo-sync-alert"`, "⚠ 2</a>",
		"title=\"Local Git clone unavailable\nPull requests, releases or tags not read\"",
		"Line statistics read through the API (limited)",
		// Failed sync: first line of the error, in red.
		`<p class="text-xs text-error truncate"`, "(git, code 128) : accès refusé")
	assertNotContains(t, w, `href="/repos/`+strconv.FormatInt(ids.Carol, 10)+`#repo-sync-alert"`)

	// Same fragment after a sync.
	assertContains(t, e.htmx("/fragments/dashboard/repos"), "⚠ 2</a>")
}

func TestRepoSyncAlert(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	e.seedWarnings(ids)

	w := e.get(repoPath(ids.Alice))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="repo-sync-alert"`, "alert-warning", "Last sync finished with 2 warnings",
		"Local Git clone unavailable", html.EscapeString(cloneWarning), "Pull requests, releases or tags not read",
		"tags non synchronisés : HTTP 502", "Fix the cause shown above (SSH access or Git credentials of this machine)")
	// Refreshed with the header fragment.
	h := e.request(http.MethodGet, repoPath(ids.Alice), nil)
	h.Header.Set("HX-Request", "true")
	h.Header.Set("HX-Target", "repo-overview")
	assertContains(t, e.do(h), `id="repo-sync-alert"`)

	w = e.get(repoPath(ids.Bob))
	assertContains(t, w, `id="repo-sync-alert"`, "alert-error", "Last sync failed", "(git, code 128)")
	assertNotContains(t, w, "SSH key")

	assertNotContains(t, e.get(repoPath(ids.Carol)), `id="repo-sync-alert"`)
}

func TestSyncWarningsFrench(t *testing.T) {
	e := newTestEnv(t, withLang("fr"))
	ids := e.seed()
	e.seedWarnings(ids)

	assertContains(t, e.get("/"), "Statistiques de lignes lues par l&#39;API (limitées)")
	assertContains(t, e.get(repoPath(ids.Alice)), "La dernière synchro s&#39;est terminée avec 2 avertissements",
		"Clone Git local indisponible", "Corrigez la cause indiquée ci-dessus (accès SSH ou identifiants Git de cette machine)")
}

func TestWarnHelpers(t *testing.T) {
	tr := i18n.For(i18n.English)
	if got := warnLabel(tr, "future_kind"); got != "Sync warning" {
		t.Errorf("unknown kind: %q", got)
	}
	if got := warnEffect(tr, model.WarnAINotScanned); got != "AI signatures analyzed at the next sync" {
		t.Errorf("warnEffect = %q", got)
	}
	one := []model.SyncWarning{{Kind: model.WarnLanguagesNotComputed}}
	if got := warnSummary(tr, one); got != "Languages not computed" {
		t.Errorf("warnSummary(1) = %q", got)
	}
	two := append(one, model.SyncWarning{Kind: model.WarnAINotScanned})
	if got := warnSummary(tr, two); got != "2 warnings" {
		t.Errorf("warnSummary(2) = %q", got)
	}
	if !warnCloneHint(two) || warnCloneHint(two[1:]) {
		t.Error("warnCloneHint")
	}
}
