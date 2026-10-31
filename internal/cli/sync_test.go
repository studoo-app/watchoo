package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

func TestSyncAll(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet", "--label", "Alice Martin").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)
	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 4000, Reset: testNow.Add(30 * time.Minute)})

	r := h.run("sync").mustSucceed(t)
	if row := tableRow(t, r.stdout, "Alice Martin"); len(row) != 5 || row[1] != "ok" || row[2] != "3" || row[3] != "2" {
		t.Errorf("ligne d'alice/projet = %q", row)
	}
	if row := tableRow(t, r.stdout, "bob/projet"); row[1] != "ok" || row[2] != "1" || row[3] != "1" {
		t.Errorf("ligne de bob/projet = %q", row)
	}
	assertContains(t, "sync", r.stdout, "DÉPÔT", "STATUT", "NOUVEAUX COMMITS",
		": 2 dépôts, 4 nouveaux commits.\n", "Quota GitHub : ", "requêtes restantes (remise à zéro à 12:30).")
	if r.stderr != "" {
		t.Errorf("sortie d'erreur inattendue :\n%s", r.stderr)
	}

	// Seconde synchro : incrémentale, rien de nouveau ; en JSON.
	h.Provider.ResetCalls()
	r = h.run("sync", "--json").mustSucceed(t)
	var rep syncReport
	decodeStrict(t, r.stdout, &rep)
	if rep.Summary != (syncSummary{Repositories: 2, Success: 2}) || len(rep.Repositories) != 2 ||
		rep.Repositories[0].DisplayName != "Alice Martin" || rep.Repositories[0].Status != wsync.StatusSuccess ||
		rep.Repositories[0].TrackedBranches != 2 || rep.RateLimit == nil || rep.RateLimit.Limit != 5000 ||
		rep.StartedAt == nil || rep.FinishedAt == nil {
		t.Errorf("sync --json = %+v", rep)
	}
	if n := h.Provider.Calls(providertest.MethodListCommits) + h.Provider.Calls(providertest.MethodCompareCommits); n != 0 {
		t.Errorf("seconde synchro : %d requêtes de commits, attendu 0 (têtes inchangées)", n)
	}
	assertJSONKeys(t, r.stdout, "", "started_at", "finished_at", "summary", "repositories", "rate_limit", "source")
	if rep.Source != wsync.SourceAPI {
		t.Errorf("sync --json : source %q, api attendue", rep.Source)
	}
	assertJSONKeys(t, r.stdout, "summary", "repositories", "success", "error", "locked", "deferred", "new_commits")
	assertJSONKeys(t, r.stdout, "repositories.0", "id", "full_name", "display_name", "status", "new_commits",
		"tracked_branches", "stats_fetched", "duration_ms", "error", "resume_at", "warnings")
	assertJSONKeys(t, r.stdout, "rate_limit", "limit", "remaining", "used", "reset")

	// Le token est redemandé à gh à chaque synchro.
	if n := h.fake().TokenCalls(); n != 4 {
		t.Errorf("Token appelé %d fois, attendu 4 (2 repo add, 2 sync)", n)
	}
}

func TestSyncOneRepo(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)

	r := h.run("sync", "https://github.com/bob/projet", "--json").mustSucceed(t)
	var rep syncReport
	decodeStrict(t, r.stdout, &rep)
	if len(rep.Repositories) != 1 || rep.Repositories[0].FullName != "bob/projet" || rep.Repositories[0].NewCommits != 1 {
		t.Errorf("sync bob/projet = %+v", rep)
	}
	h.run("sync", "carol/projet").mustFail(t, "le dépôt carol/projet n'est pas suivi dans cet espace")
	h.run("sync", "pas un dépôt").mustFail(t, "pas un dépôt")
}

func TestSyncEmptyAndErrors(t *testing.T) {
	h := newWorkspace(t)
	r := h.run("sync").mustSucceed(t)
	assertContains(t, "sync sans dépôt", r.stdout, "Aucun dépôt à synchroniser", "watchoo repo add owner/repo")
	r = h.run("sync", "--json").mustSucceed(t)
	var rep syncReport
	decodeStrict(t, r.stdout, &rep)
	if len(rep.Repositories) != 0 || rep.Repositories == nil {
		t.Errorf("sync --json sans dépôt = %+v", rep)
	}

	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)

	// Un dépôt en erreur n'arrête pas les autres ; code de retour non nul.
	h.Provider.SetRepoError("alice", "projet", &provider.APIError{Op: "lecture du dépôt alice/projet",
		Status: 404, Err: provider.ErrNotFound})
	r = h.run("sync").mustCode(t, 1)
	if row := tableRow(t, r.stdout, "alice/projet"); row[1] != "erreur" || !strings.Contains(r.stdout, "not found") {
		t.Errorf("ligne d'alice/projet = %q\n%s", row, r.stdout)
	}
	if row := tableRow(t, r.stdout, "bob/projet"); row[1] != "ok" {
		t.Errorf("ligne de bob/projet = %q", row)
	}
	assertContains(t, "sync en erreur", r.stderr, "erreur : synchro terminée avec 1 dépôt en erreur")
	r = h.run("repo", "list").mustSucceed(t)
	if row := tableRow(t, r.stdout, "alice/projet"); row[len(row)-1] != "erreur" {
		t.Errorf("état d'alice/projet = %q, attendu « erreur »", row)
	}

	// gh déconnecté : synchro par clones Git locaux ; dépôts absents du serveur Git : accès refusé.
	h.Client = authtest.NewLoggedOut()
	h.GitURL = gittest.NewServer(t).URL
	r = h.run("sync").mustCode(t, 1)
	assertContains(t, "sync sans gh", r.stdout, "access denied", "clones Git locaux")
}

func TestSyncDeferredAndLocked(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)

	// Quota sous 10 % : dépôts reportés, heure de reprise en heure locale (UTC ici).
	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 100, Reset: testNow.Add(45 * time.Minute)})
	r := h.run("sync").mustSucceed(t)
	row := tableRow(t, r.stdout, "alice/projet")
	if row[1] != "reporté" || !strings.Contains(r.stdout, "reporté : quota bas jusqu'à 12:45") {
		t.Errorf("sync quota bas = %q\n%s", row, r.stdout)
	}
	assertContains(t, "sync quota bas", r.stdout, "2 reportés")
	r = h.run("sync", "--json").mustSucceed(t)
	var rep syncReport
	decodeStrict(t, r.stdout, &rep)
	if rep.Summary.Deferred != 2 || rep.Repositories[0].ResumeAt == nil ||
		!rep.Repositories[0].ResumeAt.Equal(testNow.Add(45*time.Minute)) {
		t.Errorf("sync --json quota bas = %+v", rep)
	}

	// Verrou : un sync_run « running » récent fait ignorer le dépôt.
	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 5000})
	st := openTestStore(t, h)
	alice := storedRepo(t, h, "alice", "projet")
	if _, err := st.BeginSyncRun(context.Background(), alice.ID, testNow.Add(-time.Minute), wsync.LockStaleAfter); err != nil {
		t.Fatal(err)
	}
	r = h.run("sync").mustSucceed(t)
	if row := tableRow(t, r.stdout, "alice/projet"); row[1] != "ignoré" ||
		!strings.Contains(r.stdout, "déjà en cours de synchronisation") {
		t.Errorf("sync verrouillé = %q\n%s", row, r.stdout)
	}
	assertContains(t, "sync verrouillé", r.stdout, "1 déjà en cours")
}

func TestSyncCanceled(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := h.runContext(ctx, "sync")
	r.mustFail(t, "erreur : ")
	assertContains(t, "sync annulé", r.stderr, "commande interrompue")
}

func TestDurationText(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second:                         "0 s",
		0:                                    "0,0 s",
		1234 * time.Millisecond:              "1,2 s",
		12 * time.Second:                     "12 s",
		3*time.Minute + 5*time.Second:        "3 min 05 s",
		75*time.Minute + 59*time.Second:      "75 min 59 s",
		9*time.Second + 990*time.Millisecond: "9,9 s",
	} {
		if got := durationText(d); got != want {
			t.Errorf("durationText(%s) = %q, attendu %q", d, got, want)
		}
	}
}
