package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

func TestStatus(t *testing.T) {
	h := newSyncedWorkspace(t)
	// Un dépôt en erreur, un jamais synchronisé, un arrêté.
	st := openTestStore(t, h)
	bob := storedRepo(t, h, "bob", "projet")
	run, err := st.BeginSyncRun(context.Background(), bob.ID, testNow, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishSyncRun(context.Background(), run.ID, testNow, model.SyncError, 0, "accès perdu"); err != nil {
		t.Fatal(err)
	}
	h.Provider.AddRepository(provider.Repository{ID: 105, Owner: "eve", Name: "projet"})
	h.run("repo", "add", "eve/projet").mustSucceed(t)
	h.run("repo", "remove", "dave/projet", "--keep-history", "--yes").mustSucceed(t)
	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 4321, Reset: testNow.Add(20 * time.Minute)})
	rateCalls := h.Provider.Calls(providertest.MethodRateLimit)

	r := h.run("status").mustSucceed(t)
	want := fmt.Sprintf(`Espace : licence-projet (%s)
Compte gh : watchoo-test sur github.com (droits : gist, read:org, repo)
Source : API GitHub (gh connecté)
Quota GitHub : 4321 / 5000 requêtes restantes (remise à zéro à 12:20).

Dépôts : 4
  à jour (Actif)         1
  Flemme                 0
  Dead                   0
  en erreur              1   bob/projet
  jamais synchronisés    1
  suivi arrêté           1

Dernière synchro : à l'instant (27/10/2026 12:00)
Branches suivies : 3
`, displayPath(h.cwd))
	if r.stdout != want || r.stderr != "" {
		t.Errorf("status =\n%s\nattendu :\n%s\nstderr : %s", r.stdout, want, r.stderr)
	}
	if n := h.Provider.Calls(providertest.MethodRateLimit) - rateCalls; n != 1 {
		t.Errorf("RateLimit appelé %d fois, attendu 1", n)
	}

	r = h.run("status", "--json").mustSucceed(t)
	var rep statusReport
	decodeStrict(t, r.stdout, &rep)
	if !rep.GitHub.OK || rep.GitHub.Login != authtest.FakeLogin || rep.GitHub.TokenSource != "keyring" ||
		rep.RateLimit == nil || rep.RateLimit.Remaining != 4321 || rep.RateLimitError != "" ||
		rep.Repositories != (statusRepoCounts{Total: 4, Active: 1, Error: 1, NeverSynced: 1, Stopped: 1}) ||
		len(rep.Errors) != 1 || rep.Errors[0] != (statusRepoError{Repo: "bob/projet", Message: "accès perdu"}) ||
		rep.LastSyncedAt == nil || rep.TrackedBranches != 3 || rep.Workspace.Name != "licence-projet" {
		t.Errorf("status --json = %+v", rep)
	}
	if rep.QuotaLow {
		t.Error("status --json : quota_low vrai avec 4321 / 5000")
	}
	assertJSONKeys(t, r.stdout, "", "workspace", "github", "source", "rate_limit", "rate_limit_error", "quota_low",
		"repositories", "errors", "last_synced_at", "tracked_branches")
	assertJSONKeys(t, r.stdout, "source", "kind", "reason")
	if rep.Source != (statusSource{Kind: wsync.SourceAPI}) {
		t.Errorf("status --json : source %+v, api attendue", rep.Source)
	}
	assertJSONKeys(t, r.stdout, "github", "ok", "host", "login", "token_source", "scopes", "error")
	assertJSONKeys(t, r.stdout, "repositories", "total", "active", "flemme", "dead", "error", "never_synced",
		"archived", "empty", "stopped")
	assertJSONKeys(t, r.stdout, "workspace", "name", "root")
}

func TestStatusEmptyWorkspace(t *testing.T) {
	h := newWorkspace(t)
	r := h.run("status").mustSucceed(t)
	assertContains(t, "status vide", r.stdout, "Compte gh : watchoo-test", "Quota GitHub : 5000 / 5000",
		"Aucun dépôt suivi.")
	assertNotContains(t, "status vide", r.stdout, "Dernière synchro")
}

// Quota sous 10 % : status signale le garde-fou (même règle que le moteur), en texte et en JSON ;
// après la remise à zéro, plus rien.
func TestStatusQuotaLow(t *testing.T) {
	h := newWorkspace(t)
	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 300, Reset: testNow.Add(20 * time.Minute)})
	r := h.run("status").mustSucceed(t)
	assertContains(t, "status quota bas", r.stdout,
		"Quota GitHub : 300 / 5000 requêtes restantes (remise à zéro à 12:20).\n"+
			"Quota bas : synchros reportées jusqu'à 12:20 (moins de 10 % du quota restant).\n")
	r = h.run("status", "--json").mustSucceed(t)
	var rep statusReport
	decodeStrict(t, r.stdout, &rep)
	if !rep.QuotaLow || rep.RateLimit == nil || rep.RateLimit.Remaining != 300 {
		t.Errorf("status --json quota bas = %+v", rep)
	}

	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 500, Reset: testNow.Add(20 * time.Minute)})
	r = h.run("status").mustSucceed(t)
	assertNotContains(t, "status quota à 10 %", r.stdout, "Quota bas")
}

// gh déconnecté : status le signale sans échouer, sans demander le quota, et annonce les clones Git
// locaux comme source des synchros.
func TestStatusLoggedOut(t *testing.T) {
	h := newWorkspace(t)
	h.Client = authtest.NewLoggedOut()
	r := h.run("status").mustSucceed(t)
	assertContains(t, "status déconnecté", r.stdout, "Compte gh : vérification de gh : ", "gh auth login",
		"Source : clones Git locaux", "Quota GitHub : inconnu (sans objet : clones Git locaux, sans quota)")
	r = h.run("status", "--json").mustSucceed(t)
	var rep statusReport
	decodeStrict(t, r.stdout, &rep)
	if rep.GitHub.OK || rep.GitHub.Error == "" || rep.RateLimit != nil || rep.RateLimitError == "" ||
		rep.GitHub.Scopes == nil || rep.Source.Kind != wsync.SourceGit || rep.Source.Reason == "" {
		t.Errorf("status --json déconnecté = %+v", rep)
	}
	if len(h.tokens) != 0 {
		t.Error("le provider a été construit malgré gh déconnecté")
	}

	// Quota illisible : signalé, sans échec.
	h.Client = authtest.NewLoggedIn("")
	h.Provider.SetError(providertest.MethodRateLimit, provider.ErrUnavailable)
	r = h.run("status").mustSucceed(t)
	assertContains(t, "status quota en erreur", r.stdout, "Quota GitHub : inconnu (lecture du quota : GitHub is unreachable")
}

// Sur un terminal, status commence par la carte d'en-tête ; jamais en JSON ni hors terminal.
func TestStatusCard(t *testing.T) {
	h := newWorkspace(t)
	h.Terminal = func() TermInfo { return wideTerminal }
	r := h.run("status").mustSucceed(t)
	card := cardFor(wideTerminal, "État de l'espace « licence-projet »", tipStatus)
	if !strings.HasPrefix(r.stdout, card+"\n") {
		t.Errorf("status sans carte en tête :\n%s", r.stdout)
	}
	assertContains(t, "status sur terminal", ansiSeq.ReplaceAllString(r.stdout, ""),
		"Astuce : « watchoo sync » met à jour les dépôts suivis.", "Compte gh : watchoo-test")
	r = h.run("status", "--json").mustSucceed(t)
	if !strings.HasPrefix(r.stdout, "{") {
		t.Errorf("status --json avec carte :\n%s", r.stdout)
	}
	h.Terminal = nil
	if r := h.run("status").mustSucceed(t); strings.Contains(r.stdout, "╭") || strings.Contains(r.stdout, "Astuce") {
		t.Errorf("status redirigé avec carte :\n%s", r.stdout)
	}
}
