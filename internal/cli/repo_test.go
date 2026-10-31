package cli

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// openTestStore ouvre la base de l'espace du harness, pour vérifier ce que les commandes ont écrit.
func openTestStore(t *testing.T, h *harness) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(h.cwd, ".watchoo", "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// storedRepo renvoie le dépôt owner/name de la base.
func storedRepo(t *testing.T, h *harness, owner, name string) model.Repository {
	t.Helper()
	repo, err := openTestStore(t, h).GetRepositoryByName(context.Background(), model.ProviderGitHub, owner, name)
	if err != nil {
		t.Fatalf("dépôt %s/%s : %v", owner, name, err)
	}
	return repo
}

func TestRepoAdd(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)

	r := h.run("repo", "add", "alice/projet", "--label", "Alice Martin", "--branch", "main, dev,release/*").mustSucceed(t)
	want := "Repository alice/projet added (label \"Alice Martin\", branches: main, dev, release/*).\n" +
		"Run \"watchoo sync\" to sync it.\n"
	if r.stdout != want || r.stderr != "" {
		t.Errorf("repo add = %q (stderr %q), attendu %q", r.stdout, r.stderr, want)
	}
	repo := storedRepo(t, h, "alice", "projet")
	if repo.Label != "Alice Martin" || !reflect.DeepEqual(repo.BranchPatterns, []string{"main", "dev", "release/*"}) ||
		!repo.Enabled || repo.ProviderID != aliceRepoID || !repo.LastSyncedAt.IsZero() {
		t.Errorf("dépôt enregistré = %+v", repo)
	}
	// Le token est demandé à gh et passé au provider ; aucune synchro n'est lancée.
	if len(h.tokens) != 1 || h.tokens[0] != authtest.FakeToken {
		t.Errorf("tokens passés au provider = %d", len(h.tokens))
	}
	if n := h.Provider.Calls(providertest.MethodRepository); n != 1 {
		t.Errorf("Repository appelé %d fois, attendu 1", n)
	}
	if n := h.Provider.Calls(providertest.MethodBranches); n != 0 {
		t.Errorf("repo add a lancé une synchro (Branches appelé %d fois)", n)
	}

	// Déjà suivi, rien de nouveau (casse différente, URL) : rien à changer.
	r = h.run("repo", "add", "https://github.com/Alice/Projet.git").mustSucceed(t)
	assertContains(t, "repo add déjà suivi", r.stdout, "Repository alice/projet already tracked: nothing to change")
	assertNotContains(t, "repo add déjà suivi", r.stdout, "watchoo sync")

	// --branch "" revient à toutes les branches ; le label est gardé.
	r = h.run("repo", "add", "alice/projet", "--branch", "", "--json").mustSucceed(t)
	var rep repoAddReport
	decodeStrict(t, r.stdout, &rep)
	if rep.Outcome != wsync.OutcomeUpdated || rep.Repository.Label != "Alice Martin" ||
		len(rep.Repository.BranchPatterns) != 0 || rep.Repository.FullName != "alice/projet" {
		t.Errorf("repo add --branch \"\" --json = %+v", rep)
	}
	assertJSONKeys(t, r.stdout, "", "outcome", "repository")
	assertJSONKeys(t, r.stdout, "repository", repositoryKeys...)
	if got := storedRepo(t, h, "alice", "projet").BranchPatterns; len(got) != 0 {
		t.Errorf("motifs après --branch \"\" = %v, attendu toutes les branches", got)
	}
}

func TestRepoAddErrors(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "pas un dépôt").mustFail(t, "pas un dépôt")
	h.run("repo", "add", "alice/projet", "--branch", "main,[").mustFail(t, "--branch", "motif de branche")
	h.run("repo", "add", "alice/projet", "--label", "a\x1b]0;titre\x07b").mustFail(t,
		"--label : caractère de contrôle interdit")
	h.run("repo", "add", "alice/projet", "--label", "ligne1\nligne2").mustFail(t, "--label : caractère de contrôle")
	h.run("repo", "add", "carol/projet").mustFail(t, "repository carol/projet inaccessible",
		"repository not found or account without access")
	h.run("repo", "add", "carol/projet", "--json").mustFail(t, "inaccessible")

	h.Provider.SetRepoError("bob", "projet", &provider.SSORequiredError{Organization: "promo"})
	h.run("repo", "add", "bob/projet").mustFail(t, "repository bob/projet inaccessible")

	// gh déconnecté : vérification par git (clone local), dépôt absent du serveur Git : accès
	// refusé, rien d'enregistré.
	h.Client = authtest.NewLoggedOut()
	h.GitURL = gittest.NewServer(t).URL
	h.run("repo", "add", "alice/projet").mustFail(t, "repository alice/projet inaccessible", "access denied")
	if repos, err := openTestStore(t, h).ListRepositories(context.Background()); err != nil || len(repos) != 0 {
		t.Errorf("dépôts enregistrés malgré les erreurs : %v, %v", repos, err)
	}
}

func TestRepoListEmpty(t *testing.T) {
	h := newWorkspace(t)
	r := h.run("repo", "list").mustSucceed(t)
	assertContains(t, "repo list vide", r.stdout, "No repository tracked", "watchoo repo add owner/repo")
	r = h.run("repo", "list", "--json").mustSucceed(t)
	if r.stdout != "{\n  \"repositories\": []\n}\n" {
		t.Errorf("repo list --json vide = %q", r.stdout)
	}
	if len(h.tokens) != 0 || h.fake().TokenCalls() != 0 {
		t.Error("repo list a demandé le token")
	}
}

func TestRepoList(t *testing.T) {
	h := newSyncedWorkspace(t)
	h.run("repo", "add", "carol/projet").mustFail(t, "inaccessible")
	h.Provider.AddRepository(provider.Repository{ID: 105, Owner: "eve", Name: "projet"})
	h.run("repo", "add", "eve/projet").mustSucceed(t) // jamais synchronisé

	r := h.run("repo", "list").mustSucceed(t)
	// (Les dates et états courts viennent de internal/activity : hors du périmètre de ce groupe.)
	want := "REPOSITORY    LAST COMMIT  AUTHOR      STATE\n" +
		"Alice Martin  1 h ago      Bob Durand  -\n" +
		"bob/projet    9 d ago      Bob Durand  ~ 9 d\n" +
		"dave/projet   42 d ago     Dave Petit  ⚠ 42 d\n" +
		"eve/projet    never        -           never\n"
	if r.stdout != want {
		t.Errorf("repo list =\n%q\nattendu :\n%q", r.stdout, want)
	}

	// Terminal sans Unicode : symboles et en-têtes ASCII ; avec couleurs : état gris en gris.
	// (Les dates et états courts viennent de internal/activity : hors du périmètre de ce groupe.)
	h.Terminal = func() TermInfo { return TermInfo{IsTerminal: true, Width: 100, Color: true} }
	r = h.run("repo", "list").mustSucceed(t)
	plain := ansiSeq.ReplaceAllString(r.stdout, "")
	assertContains(t, "repo list ASCII", plain, "REPOSITORY", "STATE", "! 42 d")
	assertNotContains(t, "repo list ASCII", plain, "⚠", "É")
	for _, line := range strings.Split(r.stdout, "\n") {
		if strings.Contains(line, "eve/projet") && !strings.Contains(line, "\x1b[") {
			t.Errorf("ligne grise sans couleur : %q", line)
		}
		if strings.Contains(line, "bob/projet") && strings.Contains(line, "\x1b[") {
			t.Errorf("ligne active colorée : %q", line)
		}
	}

	// Terminal ASCII en français : la variante sans accents des clés i18n (Depot, Etat).
	restore := i18n.SetProcessForTest(i18n.French)
	r = h.run("repo", "list").mustSucceed(t)
	restore()
	plain = ansiSeq.ReplaceAllString(r.stdout, "")
	assertContains(t, "repo list ASCII français", plain, "DEPOT", "ETAT")
	assertNotContains(t, "repo list ASCII français", plain, "É")
	h.Terminal = func() TermInfo { return TermInfo{IsTerminal: true, Width: 100, Unicode: true} }
	if r := h.run("repo", "list").mustSucceed(t); strings.Contains(r.stdout, "\x1b[") {
		t.Errorf("couleurs sans TermInfo.Color :\n%s", r.stdout)
	}
	h.Terminal = nil

	r = h.run("repo", "list", "--json").mustSucceed(t)
	var rep repoListReport
	decodeStrict(t, r.stdout, &rep)
	if len(rep.Repositories) != 4 {
		t.Fatalf("%d dépôts en JSON, attendu 4", len(rep.Repositories))
	}
	alice, eve := rep.Repositories[0], rep.Repositories[3]
	if alice.DisplayName != "Alice Martin" || alice.Activity.State != "active" || alice.Commits != 3 ||
		alice.TrackedBranches != 2 || alice.LastCommit == nil || alice.LastCommit.Author.Name != "Bob Durand" ||
		alice.LastSync == nil || alice.LastSync.Status != "success" || alice.LastCommitAt == nil ||
		*alice.Activity.DaysSinceCommit != 0 {
		t.Errorf("alice/projet en JSON = %+v", alice)
	}
	if rep.Repositories[2].Activity.State != "dead" || *rep.Repositories[2].Activity.DaysSinceCommit != 42 {
		t.Errorf("dave/projet en JSON = %+v", rep.Repositories[2].Activity)
	}
	if eve.Activity.State != "never_synced" || !eve.Activity.Gray || eve.Activity.DaysSinceCommit != nil ||
		eve.LastCommit != nil || eve.LastSync != nil || eve.LastSyncedAt != nil {
		t.Errorf("eve/projet en JSON = %+v", eve)
	}
	assertJSONKeys(t, r.stdout, "", "repositories")
	assertJSONKeys(t, r.stdout, "repositories.0", append(repositoryKeys,
		"activity", "commits", "tracked_branches", "last_commit", "last_sync")...)
	assertJSONKeys(t, r.stdout, "repositories.0.activity", "state", "state_label", "gray",
		"days_since_last_commit", "flemme_after_days", "dead_after_days")
	assertJSONKeys(t, r.stdout, "repositories.0.last_commit", "repository", "sha", "short_sha", "message", "url",
		"author", "authored_at", "committed_at", "additions", "deletions", "stats_known")
	assertJSONKeys(t, r.stdout, "repositories.0.last_commit.author", "name", "login", "display_name")
	assertJSONKeys(t, r.stdout, "repositories.0.last_sync", "status", "started_at", "finished_at", "new_commits", "error")
}

func TestRepoRemove(t *testing.T) {
	h := newSyncedWorkspace(t)

	// (Le message de suivi non trouvé vient de session.go, « error : » de root.go : hors du périmètre
	// de ce groupe.)
	// Hors terminal, --yes est obligatoire ; rien n'est supprimé.
	h.run("repo", "remove", "bob/projet").mustFail(t, "run again with --yes")
	storedRepo(t, h, "bob", "projet")

	// Dépôt non suivi.
	h.run("repo", "remove", "carol/projet", "--yes").mustFail(t,
		"repository carol/projet is not tracked in this workspace", "watchoo repo list")

	// Terminal : la réponse « n » (ou la fin de l'entrée) annule. (Le suffixe « [y/N] » et le préfixe
	// « error: » viennent de confirm.go et root.go : hors du périmètre de ce groupe.)
	h.StdinTerminal = true
	for _, answer := range []string{"n\n", "", "peut-être\n"} {
		h.Stdin = answer
		r := h.run("repo", "remove", "bob/projet").mustCode(t, 1)
		if !strings.HasPrefix(r.stderr, "Stop tracking bob/projet and delete its history (1 commit)? [y/N] ") ||
			!strings.HasSuffix(r.stderr, "error: removal cancelled: nothing was modified\n") || r.stdout != "" {
			t.Errorf("réponse %q : stdout %q, stderr %q", answer, r.stdout, r.stderr)
		}
	}
	storedRepo(t, h, "bob", "projet")

	// « oui » : suppression du dépôt et de son historique.
	h.Stdin = "Oui\n"
	r := h.run("repo", "remove", "bob/projet").mustSucceed(t)
	assertContains(t, "repo remove", r.stdout, "Repository bob/projet removed: tracking stopped, history deleted (1 commit).")
	st := openTestStore(t, h)
	if _, err := st.GetRepositoryByName(context.Background(), model.ProviderGitHub, "bob", "projet"); err == nil {
		t.Error("bob/projet est encore en base")
	}

	// --keep-history : suivi arrêté, historique gardé, état « arrêté ».
	h.StdinTerminal = false
	r = h.run("repo", "remove", "alice/projet", "--keep-history", "--yes", "--json").mustSucceed(t)
	var rep repoRemoveReport
	decodeStrict(t, r.stdout, &rep)
	if rep.Action != "stopped" || rep.Repository.Enabled || rep.DeletedCommits != 0 {
		t.Errorf("repo remove --keep-history --json = %+v", rep)
	}
	assertJSONKeys(t, r.stdout, "", "action", "repository", "deleted_commits")
	if n, err := st.CountCommits(context.Background(), storedRepo(t, h, "alice", "projet").ID); err != nil || n != 3 {
		t.Errorf("commits gardés = %d, %v ; attendu 3", n, err)
	}
	r = h.run("repo", "list").mustSucceed(t)
	if row := tableRow(t, r.stdout, "Alice Martin"); row[len(row)-1] != "stopped" {
		t.Errorf("ligne d'alice/projet = %q, attendu l.état « stopped »", row)
	}
	// Une synchro de tout l'espace ignore le dépôt arrêté.
	r = h.run("sync", "--json").mustSucceed(t)
	assertNotContains(t, "sync après arrêt", r.stdout, "alice/projet")

	r = h.run("repo", "remove", "alice/projet", "--keep-history").mustSucceed(t)
	assertContains(t, "repo remove déjà arrêté", r.stdout, "already stopped: nothing changed")

	// repo add reprend le suivi.
	r = h.run("repo", "add", "alice/projet").mustSucceed(t)
	assertContains(t, "repo add après arrêt", r.stdout, "already tracked, updated")
	if !storedRepo(t, h, "alice", "projet").Enabled {
		t.Error("repo add n'a pas repris le suivi")
	}

	// Suppression avec --yes et --json.
	r = h.run("repo", "remove", "dave/projet", "-y", "--json").mustSucceed(t)
	decodeStrict(t, r.stdout, &rep)
	if rep.Action != "deleted" || rep.DeletedCommits != 1 || rep.Repository.FullName != "dave/projet" {
		t.Errorf("repo remove --json = %+v", rep)
	}
}
