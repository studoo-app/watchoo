package sync_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	gosync "sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

func TestNewOptionsObligatoires(t *testing.T) {
	if _, err := wsync.New(wsync.Options{Auth: authtest.NewLoggedIn("")}); err == nil {
		t.Error("New sans base : erreur attendue")
	}
	st := openStore(t, t.TempDir()+"/w.db", true)
	if _, err := wsync.New(wsync.Options{Store: st}); err == nil {
		t.Error("New sans accès à gh : erreur attendue")
	}
	if _, err := wsync.New(wsync.Options{Store: st, Auth: authtest.NewLoggedIn("")}); err != nil {
		t.Errorf("New avec le provider GitHub par défaut : %v", err)
	}
}

// Premier import : liste complète de la branche de référence, comparaison pour les autres branches,
// aucune statistique, commit_branches et last_commit_at renseignés.
func TestPremierImport(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 5, epoch, time.Hour)
	e.fake.SetBranch(1, "feature", mainC[2].SHA)
	feat := e.fake.AppendCommits(1, "feature", 2, epoch.Add(24*time.Hour), time.Hour)

	r := e.syncOK(repo.ID)
	if r.NewCommits != 7 || r.TrackedBranches != 2 || r.StatsFetched != 0 {
		t.Errorf("rapport = %+v, 7 commits, 2 branches, 0 statistique attendus", r)
	}
	if r.Duration < 0 || r.FullName != testOwner+"/projet" || r.DisplayName != testOwner+"/projet" {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 1 {
		t.Errorf("ListCommits appelé %d fois, 1 attendu", n)
	}
	if n := e.fake.Calls(providertest.MethodCompareCommits); n != 1 {
		t.Errorf("CompareCommits appelé %d fois, 1 attendu", n)
	}
	if n := e.fake.Calls(providertest.MethodCommitStats); n != 0 {
		t.Errorf("CommitStats appelé %d fois au premier import", n)
	}
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodListCommits && c.StopSet {
			t.Error("ListCommits appelé avec Stop au premier import")
		}
	}
	if got, want := e.commitSHAs(repo.ID, "feature"), sorted(feat[0].SHA, feat[1].SHA); !slices.Equal(got, want) {
		t.Errorf("commits de feature = %v, %v attendus (commits propres à la branche)", got, want)
	}
	if got := e.commitSHAs(repo.ID, "main"); len(got) != 5 {
		t.Errorf("%d commits sur main, 5 attendus", len(got))
	}
	stored, err := e.st.GetRepository(t.Context(), repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.LastCommitAt.Equal(feat[1].CommittedAt) {
		t.Errorf("last_commit_at = %v, %v attendu", stored.LastCommitAt, feat[1].CommittedAt)
	}
	if stored.DefaultBranch != "main" || stored.LastSyncedAt.IsZero() {
		t.Errorf("métadonnées = %+v", stored)
	}
	run := e.lastRun(repo.ID)
	if run.Status != model.SyncSuccess || run.NewCommits != 7 {
		t.Errorf("sync_run = %+v", run)
	}
	b := e.branches(repo.ID)
	if b["main"].HeadSHA != mainC[4].SHA || b["feature"].HeadSHA != feat[1].SHA || !b["main"].Tracked {
		t.Errorf("branches = %+v", b)
	}
	// Contributeur : compte GitHub de l'auteur factice.
	ranking, err := e.st.ContributorRanking(t.Context(), store.ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranking) != 1 || ranking[0].Commits != 7 || ranking[0].ProviderUserID != 1001 {
		t.Errorf("contributeurs = %+v", ranking)
	}
}

// Synchro incrémentale : comparaison ancienne tête → nouvelle, statistiques des nouveaux commits,
// aucune requête de commits quand rien n'a bougé.
func TestIncremental(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	first := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.fake.SetBranch(1, "dev", first[2].SHA)
	e.syncOK(repo.ID)

	next := e.fake.AppendCommits(1, "main", 2, epoch.Add(48*time.Hour), time.Hour)
	e.fake.SetStats(1, next[0].SHA, provider.CommitStats{Additions: 10, Deletions: 2})
	e.fake.SetStats(1, next[1].SHA, provider.CommitStats{Additions: 5, Deletions: 1})
	e.fake.ResetCalls()
	r := e.syncOK(repo.ID)
	if r.NewCommits != 2 || r.StatsFetched != 2 || r.TrackedBranches != 2 {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 0 {
		t.Errorf("ListCommits appelé %d fois sur un dépôt déjà importé", n)
	}
	var compares []providertest.Call
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodCompareCommits {
			compares = append(compares, c)
		}
	}
	if len(compares) != 1 || compares[0].Base != first[2].SHA || compares[0].Head != next[1].SHA {
		t.Errorf("comparaisons = %+v, une seule (ancienne tête de main → nouvelle) attendue", compares)
	}
	if v := e.commitView(repo.ID, next[0].SHA); v.Additions != 10 || v.Deletions != 2 {
		t.Errorf("statistiques = +%d -%d, +10 -2 attendues", v.Additions, v.Deletions)
	}

	// Rien n'a bougé : branches relues, aucune requête de commits.
	e.fake.ResetCalls()
	r = e.syncOK(repo.ID)
	if r.NewCommits != 0 {
		t.Errorf("%d nouveaux commits sans changement", r.NewCommits)
	}
	for _, m := range []providertest.Method{providertest.MethodListCommits, providertest.MethodCompareCommits,
		providertest.MethodCommitStats} {
		if n := e.fake.Calls(m); n != 0 {
			t.Errorf("%s appelé %d fois alors qu'aucune tête n'a changé", m, n)
		}
	}
	if n := e.fake.Calls(providertest.MethodBranches); n != 1 {
		t.Errorf("Branches appelé %d fois, 1 attendu", n)
	}
}

// Nouvelle branche d'un dépôt déjà synchronisé : comparaison depuis la tête de la branche de référence.
func TestNouvelleBranche(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.syncOK(repo.ID)

	e.fake.SetBranch(1, "tp2", mainC[1].SHA)
	tp2 := e.fake.AppendCommits(1, "tp2", 2, epoch.Add(72*time.Hour), time.Hour)
	e.fake.ResetCalls()
	r := e.syncOK(repo.ID)
	if r.NewCommits != 2 || r.TrackedBranches != 2 {
		t.Errorf("rapport = %+v", r)
	}
	log := e.fake.CallLog()
	idx := slices.IndexFunc(log, func(c providertest.Call) bool { return c.Method == providertest.MethodCompareCommits })
	if idx < 0 || log[idx].Base != mainC[2].SHA || log[idx].Head != tp2[1].SHA {
		t.Errorf("comparaison attendue tête de main → tête de tp2, journal %+v", log)
	}
	if got := e.commitSHAs(repo.ID, "tp2"); !slices.Equal(got, sorted(tp2[0].SHA, tp2[1].SHA)) {
		t.Errorf("commits de tp2 = %v", got)
	}
}

// Merge qui apporte un commit plus ancien que l'ancienne tête : la comparaison le trouve (un arrêt au
// premier SHA connu l'aurait perdu).
func TestMergeCommitsPlusAnciens(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	m0 := e.commit(1, "m0", epoch)
	m1 := e.commit(1, "m1", epoch.Add(48*time.Hour), m0)
	e.fake.SetBranch(1, "main", m1)
	e.syncOK(repo.ID)

	f1 := e.commit(1, "f1", epoch.Add(24*time.Hour), m0) // plus ancien que M1
	x := e.commit(1, "x", epoch.Add(72*time.Hour), m1, f1)
	e.fake.SetBranch(1, "main", x)
	r := e.syncOK(repo.ID)
	if r.NewCommits != 2 {
		t.Errorf("%d nouveaux commits, 2 attendus (merge et commit fusionné plus ancien)", r.NewCommits)
	}
	if got := e.commitSHAs(repo.ID, ""); !slices.Equal(got, sorted(m0, m1, f1, x)) {
		t.Errorf("commits = %v", got)
	}
}

// Force-push sur une branche secondaire : base inconnue, repli depuis la tête de la branche de
// référence.
func TestForcePushBranche(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.fake.SetBranch(1, "feature", mainC[2].SHA)
	feat := e.fake.AppendCommits(1, "feature", 2, epoch.Add(24*time.Hour), time.Hour)
	e.syncOK(repo.ID)

	rewritten := e.commit(1, "f1-reecrit", epoch.Add(96*time.Hour), mainC[2].SHA)
	e.fake.SetBranch(1, "feature", rewritten)
	e.fake.RemoveCommit(1, feat[1].SHA)
	e.fake.RemoveCommit(1, feat[0].SHA)
	e.fake.ResetCalls()
	r := e.syncOK(repo.ID)
	if r.NewCommits != 1 {
		t.Errorf("%d nouveaux commits, 1 attendu", r.NewCommits)
	}
	var compares []providertest.Call
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodCompareCommits {
			compares = append(compares, c)
		}
	}
	if len(compares) != 2 || compares[0].Base != feat[1].SHA || compares[1].Base != mainC[2].SHA {
		t.Errorf("comparaisons = %+v : ancienne tête puis tête de main attendues", compares)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 0 {
		t.Errorf("ListCommits appelé %d fois", n)
	}
	if b := e.branches(repo.ID); b["feature"].HeadSHA != rewritten {
		t.Errorf("tête de feature = %s", b["feature"].HeadSHA)
	}
}

// Force-push sur la branche de référence : relecture de la liste sans Stop, SHA connus ignorés (ni
// réenregistrés, ni statistiques).
func TestForcePushReference(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.syncOK(repo.ID)

	b3 := e.commit(1, "b3", epoch.Add(96*time.Hour), mainC[1].SHA)
	e.fake.SetBranch(1, "main", b3)
	e.fake.RemoveCommit(1, mainC[2].SHA)
	e.fake.ResetCalls()
	r := e.syncOK(repo.ID)
	if r.NewCommits != 1 || r.StatsFetched != 1 {
		t.Errorf("rapport = %+v : 1 nouveau commit, 1 statistique attendus", r)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 1 {
		t.Errorf("ListCommits appelé %d fois, 1 attendu (repli)", n)
	}
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodListCommits && c.StopSet {
			t.Error("repli avec Stop")
		}
		if c.Method == providertest.MethodCommitStats && c.SHA != b3 {
			t.Errorf("statistiques demandées pour un commit déjà en base %s", provider.ShortSHA(c.SHA))
		}
	}
	// L'ancien commit réécrit reste en base (historique gardé), hors branche suivie pour last_commit_at.
	if got := e.commitSHAs(repo.ID, ""); len(got) != 4 {
		t.Errorf("%d commits en base, 4 attendus", len(got))
	}
}

// Branche supprimée sur la forge : tracked = false, historique gardé.
func TestBrancheSupprimee(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)
	e.fake.SetBranch(1, "old", mainC[1].SHA)
	old := e.fake.AppendCommits(1, "old", 1, epoch.Add(24*time.Hour), time.Hour)
	e.syncOK(repo.ID)

	e.fake.DeleteBranch(1, "old")
	r := e.syncOK(repo.ID)
	if r.TrackedBranches != 1 {
		t.Errorf("%d branches suivies, 1 attendue", r.TrackedBranches)
	}
	b := e.branches(repo.ID)
	if b["old"].Tracked || !b["main"].Tracked {
		t.Errorf("branches = %+v", b)
	}
	if got := e.commitSHAs(repo.ID, "old"); !slices.Equal(got, []string{old[0].SHA}) {
		t.Errorf("historique de old = %v", got)
	}
	stored, _ := e.st.GetRepository(t.Context(), repo.ID)
	if !stored.LastCommitAt.Equal(mainC[1].CommittedAt) {
		t.Errorf("last_commit_at = %v, dernier commit de main %v attendu (branches suivies)", stored.LastCommitAt,
			mainC[1].CommittedAt)
	}
}

// branch_patterns et branches.exclude ; branche de référence = première branche suivie si la branche
// par défaut ne l'est pas.
func TestMotifsEtExclusions(t *testing.T) {
	e := newEnv(t, withExclude("dependabot/*"))
	repo := e.addRepo(1, "projet", "release/*", "dependabot/*", "dev")
	mainC := e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)
	for _, name := range []string{"dev", "release/1", "dependabot/npm", "wip"} {
		e.fake.SetBranch(1, name, mainC[1].SHA)
	}
	rel := e.fake.AppendCommits(1, "release/1", 1, epoch.Add(24*time.Hour), time.Hour)

	r := e.syncOK(repo.ID)
	if r.TrackedBranches != 2 {
		t.Errorf("%d branches suivies, 2 attendues (dev, release/1)", r.TrackedBranches)
	}
	b := e.branches(repo.ID)
	if _, ok := b["main"]; ok {
		t.Error("main suivie malgré les motifs")
	}
	if _, ok := b["dependabot/npm"]; ok {
		t.Error("branche exclue suivie")
	}
	// dev est la référence (première branche suivie, ordre de la forge) : liste complète ; release/1
	// est comparée depuis dev.
	if n := e.fake.Calls(providertest.MethodListCommits); n != 1 {
		t.Errorf("ListCommits appelé %d fois", n)
	}
	if got := e.commitSHAs(repo.ID, "release/1"); !slices.Equal(got, []string{rel[0].SHA}) {
		t.Errorf("commits de release/1 = %v", got)
	}
	if got := e.commitSHAs(repo.ID, ""); len(got) != 3 {
		t.Errorf("%d commits, 3 attendus", len(got))
	}
}

// Dépôt renommé puis transféré : suivi par identifiant, owner/name mis à jour.
func TestDepotRenomme(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.syncOK(repo.ID)

	e.fake.UpdateRepository(provider.Repository{ID: 1, Owner: "autre-orga", Name: "projet-final",
		DefaultBranch: "main", Stars: 3, OpenIssues: 2, Archived: true})
	r := e.syncOK(repo.ID)
	if r.FullName != "autre-orga/projet-final" || r.DisplayName != "autre-orga/projet-final" {
		t.Errorf("rapport = %+v", r)
	}
	stored, _ := e.st.GetRepository(t.Context(), repo.ID)
	if stored.Owner != "autre-orga" || stored.Name != "projet-final" || stored.Stars != 3 ||
		stored.OpenIssues != 2 || !stored.Archived {
		t.Errorf("dépôt en base = %+v", stored)
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("Repository (par nom) appelé %d fois : RepositoryByID attendu", n)
	}
}

// Dépôt vide : succès sans commit.
func TestDepotVide(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "vide")
	r := e.syncOK(repo.ID)
	if r.NewCommits != 0 || r.TrackedBranches != 0 {
		t.Errorf("rapport = %+v", r)
	}
	stored, _ := e.st.GetRepository(t.Context(), repo.ID)
	if stored.LastSyncedAt.IsZero() || !stored.LastCommitAt.IsZero() {
		t.Errorf("dépôt = %+v", stored)
	}
	// Premier commit poussé ensuite : premier import.
	e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)
	if r := e.syncOK(repo.ID); r.NewCommits != 2 || r.StatsFetched != 0 {
		t.Errorf("rapport = %+v", r)
	}
}

// Dépôt privé devenu inaccessible (404) et dépôt sous SSO : erreur enregistrée, les autres dépôts
// continuent.
func TestDepotInaccessibleEtSSO(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	a := e.addRepo(1, "a-perdu")
	b := e.addRepo(2, "b-sso")
	c := e.addRepo(3, "c-ok")
	e.fake.AppendCommits(3, "main", 1, epoch, time.Hour)
	e.fake.RemoveRepository(1)
	e.fake.SetRepoError(testOwner, "b-sso", &provider.SSORequiredError{Organization: testOwner,
		URL: "https://github.com/orgs/x/sso?authorization_request=secret"})

	rep := e.syncAll()
	if len(rep.Repos) != 3 {
		t.Fatalf("%d rapports", len(rep.Repos))
	}
	if r := rep.Repos[0]; r.Status != wsync.StatusError || !strings.Contains(r.Error, "introuvable ou compte sans accès") {
		t.Errorf("dépôt inaccessible : %+v", r)
	}
	if r := rep.Repos[1]; r.Status != wsync.StatusError || !strings.Contains(r.Error, "SSO") ||
		strings.Contains(r.Error, "authorization_request") {
		t.Errorf("dépôt SSO : %+v", r)
	}
	if r := rep.Repos[2]; r.Status != wsync.StatusSuccess || r.NewCommits != 1 {
		t.Errorf("dépôt accessible : %+v", r)
	}
	if rep.Count(wsync.StatusError) != 2 || rep.NewCommits() != 1 {
		t.Errorf("Count/NewCommits = %d/%d", rep.Count(wsync.StatusError), rep.NewCommits())
	}
	for _, id := range []int64{a.ID, b.ID} {
		run := e.lastRun(id)
		if run.Status != model.SyncError || run.Error == "" || run.FinishedAt.IsZero() {
			t.Errorf("sync_run du dépôt %d = %+v", id, run)
		}
	}
	if run := e.lastRun(c.ID); run.Status != model.SyncSuccess {
		t.Errorf("sync_run du dépôt accessible = %+v", run)
	}
	stored, _ := e.st.GetRepository(t.Context(), a.ID)
	if !stored.LastSyncedAt.IsZero() {
		t.Error("last_synced_at écrit malgré l'échec")
	}
}

// Quota bas avant la synchro : tous les dépôts sont reportés, sans sync_run.
func TestQuotaBasReporte(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	reset := e.clock.Now().Add(40 * time.Minute)
	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 499, Reset: reset})

	rep := e.syncAll()
	r := rep.Repos[0]
	if r.Status != wsync.StatusDeferred || !r.ResumeAt.Equal(reset) || r.Duration != 0 {
		t.Errorf("rapport = %+v", r)
	}
	if r.Error != "reporté : quota bas jusqu'à 10:40 UTC" {
		t.Errorf("message = %q", r.Error)
	}
	if _, err := e.st.LastSyncRun(t.Context(), repo.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("sync_run créé pour un dépôt reporté : %v", err)
	}
	if !rep.RateLimitKnown || rep.RateLimit.Remaining != 499 {
		t.Errorf("quota du rapport = %+v (%v)", rep.RateLimit, rep.RateLimitKnown)
	}
	// Juste au-dessus du seuil (10 % pile) : synchronisé.
	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 500, Reset: reset})
	if r := e.syncAll().Repos[0]; r.Status != wsync.StatusSuccess {
		t.Errorf("à 10 %% : %+v", r)
	}
	// Fenêtre échue : l'état observé est renouvelé, rien n'est reporté.
	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 10, Reset: reset})
	e.clock.Set(reset.Add(time.Minute))
	if r := e.syncAll().Repos[0]; r.Status != wsync.StatusSuccess {
		t.Errorf("après la remise à zéro : %+v", r)
	}
}

// Le quota passe sous 10 % pendant la synchro : les dépôts suivants sont reportés.
func TestQuotaBasEnCours(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	e.addRepo(1, "a")
	e.addRepo(2, "b")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.AppendCommits(2, "main", 1, epoch, time.Hour)
	e.fake.SetRateLimit(provider.RateLimit{Limit: 100, Remaining: 12})

	rep := e.syncAll()
	if rep.Repos[0].Status != wsync.StatusSuccess || rep.Repos[1].Status != wsync.StatusDeferred {
		t.Errorf("rapport = %+v", rep.Repos)
	}
	if rep.RateLimit.Remaining != 9 {
		t.Errorf("quota restant = %d, 9 attendu (3 requêtes pour le premier dépôt)", rep.RateLimit.Remaining)
	}
}

// Quota épuisé (RateLimitError) : le dépôt en cours échoue, les suivants sont reportés jusqu'à la
// reprise.
func TestRateLimitErrorArrete(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	for i, name := range []string{"a", "b", "c"} {
		e.addRepo(int64(i+1), name)
		e.fake.AppendCommits(int64(i+1), "main", 1, epoch, time.Hour)
	}
	reset := time.Date(2026, 10, 20, 11, 5, 0, 0, time.UTC)
	e.fake.FailNext(providertest.MethodBranches, &provider.APIError{Op: "liste des branches", Status: 403,
		Err: &provider.RateLimitError{Reset: reset}})

	rep := e.syncAll()
	if r := rep.Repos[0]; r.Status != wsync.StatusError || !strings.Contains(r.Error, "quota") {
		t.Errorf("premier dépôt : %+v", r)
	}
	for _, r := range rep.Repos[1:] {
		if r.Status != wsync.StatusDeferred || !r.ResumeAt.Equal(reset) ||
			r.Error != "reporté : quota épuisé jusqu'à 11:05 UTC" {
			t.Errorf("dépôt suivant : %+v", r)
		}
	}
	if n := e.fake.Calls(providertest.MethodRepositoryByID); n != 1 {
		t.Errorf("RepositoryByID appelé %d fois après le quota épuisé", n)
	}
	if run := e.lastRun(1); run.Status != model.SyncError {
		t.Errorf("sync_run = %+v", run)
	}
}

// Limite secondaire sans Reset : reprise = maintenant + Retry-After.
func TestRateLimitSecondaire(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	e.addRepo(1, "a")
	e.addRepo(2, "b")
	e.fake.FailNext(providertest.MethodRepositoryByID, &provider.RateLimitError{Secondary: true, RetryAfter: time.Minute})
	rep := e.syncAll()
	if r := rep.Repos[1]; r.Status != wsync.StatusDeferred || !r.ResumeAt.Equal(e.clock.Now().Add(time.Minute)) {
		t.Errorf("rapport = %+v", r)
	}
}

// Verrou : un sync_run récent bloque le dépôt ; un sync_run périmé est clos et la synchro passe.
func TestVerrou(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	now := e.clock.Now()
	if _, err := e.st.BeginSyncRun(t.Context(), repo.ID, now.Add(-10*time.Minute), wsync.LockStaleAfter); err != nil {
		t.Fatal(err)
	}
	rep := e.syncAll()
	if r := rep.Repos[0]; r.Status != wsync.StatusLocked || r.Error != "déjà en cours de synchronisation" {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodRepositoryByID); n != 0 {
		t.Errorf("dépôt verrouillé interrogé %d fois", n)
	}

	e.clock.Set(now.Add(25 * time.Minute)) // le running a 35 min : périmé
	if r := e.syncAll().Repos[0]; r.Status != wsync.StatusSuccess {
		t.Errorf("après péremption : %+v", r)
	}
}

// Deux moteurs sur la même base (deux processus) : le second voit le dépôt verrouillé pendant que le
// premier le synchronise.
func TestVerrouDeuxMoteurs(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once gosync.Once
	e := newEnv(t, withWrap(func(p provider.Provider) provider.Provider {
		return &hookProvider{Provider: p, before: func(ctx context.Context, m string) error {
			if m == "Branches" {
				once.Do(func() {
					close(entered)
					<-release
				})
			}
			return nil
		}}
	}))
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)

	second := e.newEngine(openStore(t, e.dbPath, false))
	done := make(chan wsync.Report)
	go func() {
		rep, err := e.eng.SyncRepo(context.Background(), repo.ID, wsync.SyncOptions{})
		if err != nil {
			t.Errorf("SyncRepo : %v", err)
		}
		done <- rep
	}()
	<-entered
	rep, err := second.SyncAll(t.Context(), wsync.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r := rep.Repos[0]; r.Status != wsync.StatusLocked {
		t.Errorf("second moteur : %+v", r)
	}
	close(release)
	if r := (<-done).Repos[0]; r.Status != wsync.StatusSuccess || r.NewCommits != 2 {
		t.Errorf("premier moteur : %+v", r)
	}
}

// Parallélisme borné par sync.parallelism (et par 4 au plus).
func TestParallelismeBorne(t *testing.T) {
	for _, tc := range []struct{ setting, want int }{{2, 2}, {9, 4}, {0, 1}} {
		var cur, peak atomic.Int32
		e := newEnv(t, withParallelism(tc.setting), withWrap(func(p provider.Provider) provider.Provider {
			return &hookProvider{Provider: p, before: func(ctx context.Context, m string) error {
				if m == "RepositoryByID" {
					n := cur.Add(1)
					for {
						old := peak.Load()
						if n <= old || peak.CompareAndSwap(old, n) {
							break
						}
					}
					time.Sleep(20 * time.Millisecond)
					cur.Add(-1)
				}
				return nil
			}}
		}))
		for i := range 8 {
			e.addRepo(int64(i+1), "depot-"+string(rune('a'+i)))
		}
		rep := e.syncAll()
		if rep.Count(wsync.StatusSuccess) != 8 {
			t.Errorf("parallélisme %d : %d succès", tc.setting, rep.Count(wsync.StatusSuccess))
		}
		if got := int(peak.Load()); got != tc.want {
			t.Errorf("parallélisme %d : au plus %d dépôts en même temps, %d attendus", tc.setting, got, tc.want)
		}
		// Ordre du rapport : celui de ListRepositories.
		for i, r := range rep.Repos {
			if r.FullName != testOwner+"/depot-"+string(rune('a'+i)) {
				t.Errorf("rapport %d = %s", i, r.FullName)
			}
		}
	}
}

// Statistiques : au plus 100 par dépôt et par synchro.
func TestStatistiquesPlafond(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.syncOK(repo.ID)
	e.fake.AppendCommits(1, "main", 150, epoch.Add(time.Hour), time.Minute)
	r := e.syncOK(repo.ID)
	if r.NewCommits != 150 || r.StatsFetched != wsync.MaxStatsPerRepo {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodCommitStats); n != 100 {
		t.Errorf("CommitStats appelé %d fois", n)
	}
	// Les commits les plus récents sont servis d'abord.
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodCommitStats {
			v := e.commitView(repo.ID, c.SHA)
			if v.CommittedAt.Before(epoch.Add(time.Hour + 50*time.Minute)) {
				t.Errorf("statistiques d'un commit ancien %v", v.CommittedAt)
			}
		}
	}
}

// Statistiques : jamais quand le quota restant passe sous 10 %.
func TestStatistiquesQuota(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.syncOK(repo.ID)
	e.fake.AppendCommits(1, "main", 10, epoch.Add(time.Hour), time.Minute)
	// 105 restants : RepositoryByID, Branches, CompareCommits (102), puis statistiques à 102, 101 et
	// 100 (10 % pile) ; à 99, plus rien.
	e.fake.SetRateLimit(provider.RateLimit{Limit: 1000, Remaining: 105})
	r := e.syncOK(repo.ID)
	if r.NewCommits != 10 || r.StatsFetched != 3 {
		t.Errorf("rapport = %+v : 3 statistiques attendues", r)
	}
}

// Statistiques : un quota épuisé pendant les statistiques garde le dépôt en succès et reporte les
// suivants ; une autre erreur arrête seulement les statistiques.
func TestStatistiquesErreurs(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	a := e.addRepo(1, "a")
	e.addRepo(2, "b")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.AppendCommits(2, "main", 1, epoch, time.Hour)
	e.syncAll()
	e.fake.AppendCommits(1, "main", 3, epoch.Add(time.Hour), time.Minute)

	e.fake.FailNext(providertest.MethodCommitStats, &provider.RateLimitError{Reset: e.clock.Now().Add(time.Hour)})
	rep := e.syncAll()
	if r := rep.Repos[0]; r.Status != wsync.StatusSuccess || r.NewCommits != 3 || r.StatsFetched != 1 {
		t.Errorf("dépôt a : %+v", r)
	}
	if r := rep.Repos[1]; r.Status != wsync.StatusDeferred {
		t.Errorf("dépôt b : %+v", r)
	}

	e.fake.AppendCommits(1, "main", 3, epoch.Add(2*time.Hour), time.Minute)
	e.fake.FailNext(providertest.MethodCommitStats, &provider.APIError{Op: "statistiques", Err: provider.ErrUnavailable})
	if r := e.syncOK(a.ID); r.NewCommits != 3 || r.StatsFetched != 1 {
		t.Errorf("erreur passagère : %+v", r)
	}
}

// Fusion d'identités : un auteur d'abord connu par son seul e-mail, puis par son compte GitHub, ne
// fait plus qu'un contributeur ; un commit sans compte ni e-mail reste sans auteur.
func TestFusionIdentites(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	noAccount := provider.Person{Name: secretName, Email: "Eleve.Confidentiel@Example.Test"}
	c1 := providertest.SHA("c1")
	e.fake.AddCommits(1, provider.Commit{SHA: c1, Message: secretMessage, Author: noAccount, Committer: noAccount,
		AuthoredAt: epoch, CommittedAt: epoch})
	anonymous := provider.Person{Name: "Sans Email"}
	c2 := providertest.SHA("c2")
	e.fake.AddCommits(1, provider.Commit{SHA: c2, Message: secretMessage, Author: anonymous, Committer: anonymous,
		AuthoredAt: epoch.Add(time.Hour), CommittedAt: epoch.Add(time.Hour), Parents: []string{c1}})
	e.fake.SetBranch(1, "main", c2)
	e.syncOK(repo.ID)

	c3 := e.commit(1, "c3", epoch.Add(2*time.Hour), c2) // compte 4242, même e-mail
	e.fake.SetBranch(1, "main", c3)
	e.syncOK(repo.ID)

	ranking, err := e.st.ContributorRanking(t.Context(), store.ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranking) != 1 || ranking[0].Commits != 2 || ranking[0].ProviderUserID != 4242 ||
		ranking[0].ProviderLogin != secretLogin {
		t.Errorf("classement = %+v : un seul contributeur (compte 4242, 2 commits) attendu", ranking)
	}
	if v := e.commitView(repo.ID, c2); v.AuthorID != 0 {
		t.Errorf("commit sans compte ni e-mail : auteur %d, aucun attendu", v.AuthorID)
	}
}

// Le token est redemandé à gh à chaque synchro et transmis à la fabrique ; un échec de gh est une
// erreur globale (rapport vide, événement de fin envoyé).
func TestTokenRedemande(t *testing.T) {
	e := newEnv(t)
	e.addRepo(1, "projet")
	e.auth.SetTokens("gho_fake_premier", "gho_fake_second")
	e.syncAll()
	e.syncAll()
	if _, err := e.eng.RateLimit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := e.receivedTokens(); !slices.Equal(got, []string{"gho_fake_premier", "gho_fake_second", "gho_fake_second"}) {
		t.Errorf("tokens reçus = %v", got)
	}
	if n := e.auth.TokenCalls(); n != 3 {
		t.Errorf("Token appelé %d fois, 3 attendus", n)
	}

	e.auth.SetTokenErr(auth.ErrNotLoggedIn)
	var kinds []wsync.EventKind
	rep, err := e.eng.SyncAll(t.Context(), wsync.SyncOptions{Progress: func(ev wsync.Event) { kinds = append(kinds, ev.Kind) }})
	if !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Errorf("erreur = %v, ErrNotLoggedIn attendue", err)
	}
	if rep.Repos == nil || len(rep.Repos) != 0 {
		t.Errorf("rapport = %+v, vide attendu", rep.Repos)
	}
	if !slices.Equal(kinds, []wsync.EventKind{wsync.EventStarted, wsync.EventFinished}) {
		t.Errorf("événements = %v", kinds)
	}
	if _, err := e.eng.RateLimit(t.Context()); !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Errorf("RateLimit : %v", err)
	}
	if _, err := e.eng.AddRepositories(t.Context(), nil, wsync.AddOptions{}); !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Errorf("AddRepositories : %v", err)
	}
}

// Une erreur qui citerait le token est masquée dans le rapport et dans sync_runs.
func TestTokenMasque(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.FailNext(providertest.MethodRepositoryByID, errors.New("panne inattendue "+authtest.FakeToken))
	rep := e.syncAll()
	if r := rep.Repos[0]; r.Status != wsync.StatusError || !strings.Contains(r.Error, "[token masqué]") {
		t.Errorf("rapport = %+v", r)
	}
	if run := e.lastRun(repo.ID); strings.Contains(run.Error, authtest.FakeToken) {
		t.Errorf("token enregistré dans sync_runs : %q", run.Error)
	}
}

// Progression : début, début et fin de chaque dépôt, fin ; Done croissant, Total connu.
func TestProgression(t *testing.T) {
	e := newEnv(t, withParallelism(4))
	for i := range 3 {
		e.addRepo(int64(i+1), "p"+string(rune('a'+i)))
	}
	var (
		mu     gosync.Mutex
		events []wsync.Event
		busy   atomic.Bool
	)
	rep, err := e.eng.SyncAll(t.Context(), wsync.SyncOptions{Progress: func(ev wsync.Event) {
		if !busy.CompareAndSwap(false, true) {
			t.Error("appels de Progress simultanés")
		}
		time.Sleep(time.Millisecond)
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
		busy.Store(false)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 8 || events[0].Kind != wsync.EventStarted || events[7].Kind != wsync.EventFinished {
		t.Fatalf("événements = %+v", events)
	}
	finished := 0
	for _, ev := range events {
		if ev.Total != 3 {
			t.Errorf("Total = %d", ev.Total)
		}
		if ev.Kind == wsync.EventRepoFinished {
			finished++
			if ev.Done != finished || ev.Repo.Status != wsync.StatusSuccess {
				t.Errorf("événement = %+v", ev)
			}
		}
	}
	if events[7].Done != 3 || rep.FinishedAt.Before(rep.StartedAt) {
		t.Errorf("fin = %+v", events[7])
	}
}

// SyncAll ignore les dépôts arrêtés ; SyncRepo les synchronise (demande explicite) ; dépôt inconnu.
func TestSuiviArreteEtDepotInconnu(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	if err := e.st.SetRepositoryEnabled(t.Context(), repo.ID, false); err != nil {
		t.Fatal(err)
	}
	if rep := e.syncAll(); len(rep.Repos) != 0 {
		t.Errorf("dépôt arrêté synchronisé par SyncAll : %+v", rep.Repos)
	}
	if r := e.syncOK(repo.ID); r.NewCommits != 1 {
		t.Errorf("SyncRepo : %+v", r)
	}
	if _, err := e.eng.SyncRepo(t.Context(), 999, wsync.SyncOptions{}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("dépôt inconnu : %v", err)
	}
}

// Contexte annulé pendant la synchro : erreur globale, le dépôt en cours est clos en erreur.
func TestAnnulation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	e := newEnv(t, withParallelism(1), withWrap(func(p provider.Provider) provider.Provider {
		return &hookProvider{Provider: p, before: func(c context.Context, m string) error {
			if m == "Branches" {
				cancel()
				return c.Err()
			}
			return nil
		}}
	}))
	a := e.addRepo(1, "a")
	e.addRepo(2, "b")
	rep, err := e.eng.SyncAll(ctx, wsync.SyncOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("erreur = %v", err)
	}
	if len(rep.Repos) != 2 || rep.Repos[0].Status != wsync.StatusError || rep.Repos[0].Error != "synchro annulée" {
		t.Errorf("rapport = %+v", rep.Repos)
	}
	if run := e.lastRun(a.ID); run.Status != model.SyncError || run.Error != "synchro annulée" {
		t.Errorf("sync_run = %+v", run)
	}
	if _, err := e.st.LastSyncRun(t.Context(), 2); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("dépôt suivant commencé malgré l'annulation : %v", err)
	}
}

// Quota inconnu au départ : un appel RateLimit (gratuit) ; connu : aucun.
func TestLectureInitialeDuQuota(t *testing.T) {
	e := newEnv(t)
	e.addRepo(1, "projet")
	e.syncAll()
	if n := e.fake.Calls(providertest.MethodRateLimit); n != 1 {
		t.Errorf("RateLimit appelé %d fois, 1 attendu", n)
	}
	e.fake.SetRateLimit(provider.RateLimit{Remaining: 4000})
	e.fake.ResetCalls()
	e.syncAll()
	if n := e.fake.Calls(providertest.MethodRateLimit); n != 0 {
		t.Errorf("RateLimit appelé %d fois avec un quota connu", n)
	}
	rl, err := e.eng.RateLimit(t.Context())
	if err != nil || rl.Limit != 5000 {
		t.Errorf("RateLimit = %+v, %v", rl, err)
	}
}

// Branche par défaut changée : la nouvelle branche de référence est relue en entier (rattachée à tout
// son historique, commits déjà en base compris) ; un commit vu sur deux nouvelles branches est
// rattaché aux deux.
func TestNouvelleReferenceEtCommitsPartages(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.syncOK(repo.ID)

	e.fake.UpdateRepository(provider.Repository{ID: 1, Owner: testOwner, Name: "projet", DefaultBranch: "dev"})
	e.fake.SetBranch(1, "dev", mainC[2].SHA)
	dev := e.fake.AppendCommits(1, "dev", 1, epoch.Add(24*time.Hour), time.Hour)
	e.fake.SetBranch(1, "tp-a", dev[0].SHA)
	e.fake.SetBranch(1, "tp-b", dev[0].SHA)
	shared := e.fake.AppendCommits(1, "tp-a", 1, epoch.Add(48*time.Hour), time.Hour)
	e.fake.SetBranch(1, "tp-b", shared[0].SHA)
	e.fake.ResetCalls()

	r := e.syncOK(repo.ID)
	if r.NewCommits != 2 || r.TrackedBranches != 4 {
		t.Errorf("rapport = %+v", r)
	}
	log := e.fake.CallLog()
	if slices.ContainsFunc(log, func(c providertest.Call) bool {
		return c.Method == providertest.MethodCompareCommits && c.Head == dev[0].SHA
	}) {
		t.Errorf("dev comparée au lieu d'être relue : %+v", log)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 1 {
		t.Errorf("ListCommits appelé %d fois, 1 attendu (nouvelle référence)", n)
	}
	want := sorted(mainC[0].SHA, mainC[1].SHA, mainC[2].SHA, dev[0].SHA)
	if got := e.commitSHAs(repo.ID, "dev"); !slices.Equal(got, want) {
		t.Errorf("commits de dev = %v, %v attendus", got, want)
	}
	for _, b := range []string{"tp-a", "tp-b"} {
		if got := e.commitSHAs(repo.ID, b); !slices.Equal(got, []string{shared[0].SHA}) {
			t.Errorf("commits de %s = %v", b, got)
		}
	}
	if stored, _ := e.st.GetRepository(t.Context(), repo.ID); stored.DefaultBranch != "dev" {
		t.Errorf("branche par défaut = %q", stored.DefaultBranch)
	}
}

// Dates absentes : une seule date manquante est remplacée par l'autre ; sans aucune date, le commit
// est ignoré (et signalé dans les logs, par un compteur).
func TestCommitSansDate(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	p := provider.Person{Name: secretName, Email: secretEmail}
	a, b, c := providertest.SHA("a"), providertest.SHA("b"), providertest.SHA("c")
	e.fake.AddCommits(1,
		provider.Commit{SHA: a, Author: p, Committer: p, CommittedAt: epoch},
		provider.Commit{SHA: b, Author: p, Committer: p, AuthoredAt: epoch.Add(time.Hour), Parents: []string{a}},
		provider.Commit{SHA: c, Author: p, Committer: p, Parents: []string{b}},
	)
	e.fake.SetBranch(1, "main", c)
	if r := e.syncOK(repo.ID); r.NewCommits != 2 {
		t.Errorf("%d nouveaux commits, 2 attendus", r.NewCommits)
	}
	if v := e.commitView(repo.ID, a); !v.AuthoredAt.Equal(epoch) {
		t.Errorf("date d'écriture = %v", v.AuthoredAt)
	}
	if v := e.commitView(repo.ID, b); !v.CommittedAt.Equal(epoch.Add(time.Hour)) {
		t.Errorf("date de commit = %v", v.CommittedAt)
	}
	if !strings.Contains(e.logs.String(), "commits ignorés") {
		t.Error("commit ignoré non signalé")
	}
}

// Dépôt sans identifiant de forge connu : lu par owner/name.
func TestDepotSansIdentifiant(t *testing.T) {
	e := newEnv(t)
	e.fake.AddRepository(provider.Repository{ID: 5, Owner: testOwner, Name: "ancien"})
	e.fake.AppendCommits(5, "main", 1, epoch, time.Hour)
	repo, err := e.st.CreateRepository(t.Context(), model.Repository{Provider: model.ProviderGitHub,
		Owner: testOwner, Name: "ancien", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if r := e.syncOK(repo.ID); r.NewCommits != 1 {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 1 {
		t.Errorf("Repository appelé %d fois", n)
	}
}

// assertReference vérifie que la branche ref du dépôt repoID porte exactement want (SHA), que
// last_commit_at vaut last et que le dépôt n'est pas « vide » pour l'activité.
func (e *env) assertReference(repoID int64, ref string, want []string, last time.Time) {
	e.t.Helper()
	if got := e.commitSHAs(repoID, ref); !slices.Equal(got, sorted(want...)) {
		e.t.Errorf("commits de %s = %v, %v attendus", ref, got, sorted(want...))
	}
	stored, err := e.st.GetRepository(e.t.Context(), repoID)
	if err != nil {
		e.t.Fatalf("GetRepository : %v", err)
	}
	if !stored.LastCommitAt.Equal(last) {
		e.t.Errorf("last_commit_at = %v, %v attendu", stored.LastCommitAt, last)
	}
	res := activity.Evaluate(stored, e.lastRun(repoID), e.cfg.Activity, e.clock.Now(), time.UTC)
	if res.State == activity.StateEmpty || res.State == activity.StateError {
		e.t.Errorf("état d'activité = %q après le changement de référence", res.State)
	}
}

// Branche par défaut renommée sur la forge (main → trunk, même tête, main supprimée) : trunk est
// rattachée à tout l'historique, le dépôt garde son dernier commit et n'est pas « vide ».
func TestReferenceRenommee(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.syncOK(repo.ID)

	e.fake.UpdateRepository(provider.Repository{ID: 1, Owner: testOwner, Name: "projet", DefaultBranch: "trunk"})
	e.fake.SetBranch(1, "trunk", mainC[2].SHA)
	e.fake.DeleteBranch(1, "main")
	e.fake.ResetCalls()

	if r := e.syncOK(repo.ID); r.NewCommits != 0 || r.TrackedBranches != 1 {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodCommitStats); n != 0 {
		t.Errorf("CommitStats appelé %d fois pour des commits déjà en base", n)
	}
	if b := e.branches(repo.ID); b["main"].Tracked || !b["trunk"].Tracked {
		t.Errorf("branches = %+v", b)
	}
	e.assertReference(repo.ID, "trunk", []string{mainC[0].SHA, mainC[1].SHA, mainC[2].SHA}, mainC[2].CommittedAt)

	// Synchro suivante, rien n'a bougé : aucune nouvelle relecture.
	e.fake.ResetCalls()
	e.syncOK(repo.ID)
	if n := e.fake.Calls(providertest.MethodListCommits); n != 0 {
		t.Errorf("ListCommits appelé %d fois après le changement", n)
	}
}

// Motifs --branch qui excluent après coup la branche par défaut : la nouvelle référence (dev, déjà
// connue, tête inchangée) est rattachée à tout son historique.
func TestMotifsExcluantLaReference(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)
	e.fake.SetBranch(1, "dev", mainC[1].SHA)
	e.syncOK(repo.ID)
	if got := e.commitSHAs(repo.ID, "dev"); len(got) != 0 {
		t.Fatalf("dev, partie de main, a déjà des commits propres : %v", got)
	}

	if _, err := e.st.UpdateRepositorySettings(t.Context(), repo.ID, "", []string{"dev"}); err != nil {
		t.Fatal(err)
	}
	e.fake.ResetCalls()
	if r := e.syncOK(repo.ID); r.NewCommits != 0 || r.TrackedBranches != 1 {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 1 {
		t.Errorf("ListCommits appelé %d fois, 1 attendu", n)
	}
	e.assertReference(repo.ID, "dev", []string{mainC[0].SHA, mainC[1].SHA}, mainC[1].CommittedAt)
}
