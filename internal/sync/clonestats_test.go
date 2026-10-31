package sync_test

// Statistiques de lignes par le clone Git local (« toujours cloner », 0.5.0) : avec gh, l'API
// (providertest) et le dépôt Git (gittest) décrivent le même historique, aux mêmes SHA (mirror) ;
// sans gh, le clone est la source.

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// mirror recopie dans le faux Provider (dépôt repoID) les commits et les branches du dépôt Git
// testOwner/name du serveur srvURL : l'API et le clone décrivent alors le même historique. Les commits
// déjà recopiés ne le sont pas deux fois.
func (e *env) mirror(srvURL string, repoID int64, name string) {
	e.t.Helper()
	gp, err := gitprov.New(gitprov.Options{Dir: filepath.Join(e.t.TempDir(), "miroir"), BaseURL: srvURL,
		Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		e.t.Fatal(err)
	}
	ctx := e.t.Context()
	branches, err := gp.Branches(ctx, testOwner, name)
	if err != nil {
		e.t.Fatal(err)
	}
	if e.mirrored == nil {
		e.mirrored = make(map[int64]map[string]bool)
	}
	if e.mirrored[repoID] == nil {
		e.mirrored[repoID] = make(map[string]bool)
	}
	known := e.mirrored[repoID]
	var pending []provider.Commit
	seen := make(map[string]bool)
	for _, b := range branches {
		commits, err := gp.ListCommits(ctx, testOwner, name, provider.ListCommitsOptions{Branch: b.HeadSHA})
		if err != nil {
			e.t.Fatal(err)
		}
		for _, c := range commits {
			if !known[c.SHA] && !seen[c.SHA] {
				seen[c.SHA] = true
				pending = append(pending, c)
			}
		}
	}
	// Parents d'abord.
	for len(pending) > 0 {
		var rest []provider.Commit
		for _, c := range pending {
			ready := true
			for _, p := range c.Parents {
				ready = ready && known[p]
			}
			if !ready {
				rest = append(rest, c)
				continue
			}
			e.fake.AddCommits(repoID, c)
			known[c.SHA] = true
		}
		if len(rest) == len(pending) {
			e.t.Fatal("miroir : parents introuvables")
		}
		pending = rest
	}
	for _, b := range branches {
		e.fake.SetBranch(repoID, b.Name, b.HeadSHA)
	}
}

// stats renvoie les statistiques en base du commit sha du dépôt repoID.
func (e *env) stats(repoID int64, sha string) (int, int, bool) {
	e.t.Helper()
	c := e.commitView(repoID, sha)
	return c.Additions, c.Deletions, c.StatsKnown
}

// forgetStats remet en base les statistiques du commit sha à « inconnues » (base d'avant la 0.5.0).
func (e *env) forgetStats(repoID int64, sha string) {
	e.t.Helper()
	if _, err := e.st.DB().ExecContext(e.t.Context(),
		`UPDATE commits SET additions = 0, deletions = 0, stats_known = 0 WHERE repo_id = ? AND sha = ?`,
		repoID, sha); err != nil {
		e.t.Fatal(err)
	}
}

func TestCloneStatsWithGH(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	m0 := r.Commit("main", "a.txt", "a\nb\n", secretMessage, secretAuthor, epoch)
	r.Branch("dev", m0)
	f1 := r.Commit("dev", "b.txt", "x\n", secretMessage, secretAuthor, epoch.Add(time.Hour))
	m1 := r.Commit("main", "a.txt", "a\nc\n", secretMessage, secretAuthor, epoch.Add(2*time.Hour))
	x := r.Merge("main", "dev", secretMessage, secretAuthor, epoch.Add(3*time.Hour))
	empty := r.EmptyCommit("main", secretMessage, secretAuthor, epoch.Add(4*time.Hour))

	e := newEnv(t, withGit(srv.URL)) // gh connecté : API
	repo := e.addRepo(1, "projet")
	e.mirror(srv.URL, 1, "projet")

	// Premier import : toutes les statistiques par le clone, aucune requête CommitStats.
	rep := e.syncAll()
	if rep.Source.Kind != wsync.SourceAPI {
		t.Fatalf("source %q", rep.Source.Kind)
	}
	r0 := rep.Repos[0]
	if r0.Status != wsync.StatusSuccess || r0.NewCommits != 5 || r0.StatsFetched != 5 || len(r0.Warnings) != 0 {
		t.Fatalf("premier import : %+v", r0)
	}
	if n := e.fake.Calls(providertest.MethodCommitStats); n != 0 {
		t.Errorf("%d appels CommitStats à l'API, aucun attendu", n)
	}
	for _, tc := range []struct {
		sha      string
		add, del int
	}{{m0, 2, 0}, {f1, 1, 0}, {m1, 1, 1}, {x, 1, 0}, {empty, 0, 0}} {
		if add, del, known := e.stats(repo.ID, tc.sha); !known || add != tc.add || del != tc.del {
			t.Errorf("commit %s : +%d −%d connu=%v, attendu +%d −%d", provider.ShortSHA(tc.sha), add, del, known,
				tc.add, tc.del)
		}
	}
	// Clone créé dans repos/, sur la branche par défaut de l'API.
	if got, err := os.ReadFile(filepath.Join(e.reposDir, "projet", "a.txt")); err != nil || string(got) != "a\nc\n" {
		t.Errorf("copie de travail : %q, %v", got, err)
	}

	// Commit ancien sans statistiques (base d'avant la migration 00006) et nouveau commit : les deux
	// sont calculés ; le commit vide, connu à 0/0, n'est jamais recalculé.
	e.forgetStats(repo.ID, m0)
	m2 := r.Commit("main", "c.txt", "1\n2\n3\n", secretMessage, secretAuthor, epoch.Add(5*time.Hour))
	e.mirror(srv.URL, 1, "projet")
	e.clock.Set(e.clock.Now().Add(time.Hour))
	r0 = e.syncAll().Repos[0]
	if r0.Status != wsync.StatusSuccess || r0.NewCommits != 1 || r0.StatsFetched != 2 {
		t.Fatalf("synchro suivante : %+v", r0)
	}
	if add, _, known := e.stats(repo.ID, m0); !known || add != 2 {
		t.Errorf("commit ancien : +%d connu=%v", add, known)
	}
	if add, _, known := e.stats(repo.ID, m2); !known || add != 3 {
		t.Errorf("nouveau commit : +%d connu=%v", add, known)
	}
	r0 = e.syncAll().Repos[0]
	if r0.StatsFetched != 0 {
		t.Errorf("synchro sans changement : %d statistiques calculées, aucune attendue", r0.StatsFetched)
	}

	// Totaux du dépôt : tous les commits comptés.
	sum, err := e.st.RepositorySummary(t.Context(), repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := (model.LineTotals{Commits: 6, KnownCommits: 6, Additions: 8, Deletions: 1}); sum.Lines != want {
		t.Errorf("totaux du dépôt = %+v, attendu %+v", sum.Lines, want)
	}
}

// Plus de MaxStatsPerRepo commits d'un coup : tous calculés par le clone (aucun plafond).
func TestCloneStatsNoCap(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "gros", "main")
	r.Commit("main", "f.txt", "0\n", secretMessage, secretAuthor, epoch)
	e := newEnv(t, withGit(srv.URL))
	repo := e.addRepo(1, "gros")
	e.mirror(srv.URL, 1, "gros")
	e.syncAll()

	n := wsync.MaxStatsPerRepo + 20
	r.ManyCommits("main", "f.txt", n, secretAuthor, epoch.Add(time.Hour))
	e.mirror(srv.URL, 1, "gros")
	r0 := e.syncAll().Repos[0]
	if r0.NewCommits != n || r0.StatsFetched != n {
		t.Fatalf("synchro de %d commits : %+v", n, r0)
	}
	sum, err := e.st.RepositorySummary(t.Context(), repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Lines.Uncounted() != 0 || sum.Lines.Additions != n+1 || sum.Lines.Deletions != n {
		t.Errorf("totaux = %+v", sum.Lines)
	}
}

// Clone indisponible avec gh (emplacement occupé) : le dépôt réussit, avec un avertissement, et les
// statistiques reviennent à l'API (rien au premier import, puis les nouveaux commits) ; le clone
// revenu, les commits restés inconnus sont calculés.
func TestCloneUnavailableFallsBackToAPI(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	m0 := r.Commit("main", "a.txt", "a\n", secretMessage, secretAuthor, epoch)
	e := newEnv(t, withGit(srv.URL))
	repo := e.addRepo(1, "projet")
	e.mirror(srv.URL, 1, "projet")
	occupied := filepath.Join(e.reposDir, "projet")
	if err := os.MkdirAll(occupied, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(occupied, "perso.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	r0 := e.syncAll().Repos[0]
	if r0.Status != wsync.StatusSuccess || len(r0.Warnings) != 1 ||
		!strings.Contains(r0.Warnings[0], "clone Git local indisponible, statistiques de lignes lues par l'API") ||
		!strings.Contains(r0.Warnings[0], "clone location occupied") {
		t.Fatalf("clone occupé : %+v", r0)
	}
	if _, _, known := e.stats(repo.ID, m0); known || e.fake.Calls(providertest.MethodCommitStats) != 0 {
		t.Errorf("premier import sans clone : statistiques connues=%v, %d appels", known,
			e.fake.Calls(providertest.MethodCommitStats))
	}

	// Nouveau commit : statistiques demandées à l'API (repli limité).
	m1 := r.Commit("main", "a.txt", "a\nb\n", secretMessage, secretAuthor, epoch.Add(time.Hour))
	e.mirror(srv.URL, 1, "projet")
	e.fake.SetStats(1, m1, provider.CommitStats{Additions: 7})
	r0 = e.syncAll().Repos[0]
	if r0.Status != wsync.StatusSuccess || r0.StatsFetched != 1 || len(r0.Warnings) != 1 {
		t.Fatalf("repli sur l'API : %+v", r0)
	}
	if add, _, known := e.stats(repo.ID, m1); !known || add != 7 {
		t.Errorf("statistiques par l'API : +%d connu=%v", add, known)
	}

	// Emplacement libéré : le clone est créé et rattrape le commit resté inconnu ; celui lu par
	// l'API n'est pas recalculé.
	if err := os.RemoveAll(occupied); err != nil {
		t.Fatal(err)
	}
	r0 = e.syncAll().Repos[0]
	if r0.Status != wsync.StatusSuccess || len(r0.Warnings) != 0 || r0.StatsFetched != 1 {
		t.Fatalf("clone revenu : %+v", r0)
	}
	if add, _, known := e.stats(repo.ID, m0); !known || add != 1 {
		t.Errorf("rattrapage : +%d connu=%v", add, known)
	}
	if add, _, _ := e.stats(repo.ID, m1); add != 7 {
		t.Errorf("commit lu par l'API recalculé : +%d", add)
	}
	assertNoLeak(t, "logs", e.logs.String())
}

// Provider des clones impossible à construire avec gh : avertissement pour chaque dépôt, repli.
func TestCloneProviderErrorWithGH(t *testing.T) {
	e := newEnv(t)
	e.addRepo(1, "projet")
	e.commit(1, "c0", epoch)
	e.fake.SetBranch(1, "main", providertest.SHA("c0"))
	eng, err := wsync.New(wsync.Options{Store: e.st, Auth: e.auth, Config: e.cfg,
		NewProvider: func(string) (provider.Provider, error) { return e.fake, nil },
		NewGitProvider: func([]gitprov.Ref) (provider.Provider, error) {
			return nil, gitprov.ErrGitNotFound
		},
		Now: e.clock.Now, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := eng.SyncAll(t.Context(), wsync.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r0 := rep.Repos[0]; r0.Status != wsync.StatusSuccess || len(r0.Warnings) != 1 ||
		!strings.Contains(r0.Warnings[0], "git not found") {
		t.Errorf("git absent avec gh : %+v", r0)
	}
}

// Sans gh : statistiques de tous les commits dès le premier import.
func TestGitSourceStatsFromFirstImport(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	m0 := r.Commit("main", "a.txt", "a\nb\n", secretMessage, secretAuthor, epoch)
	m1 := r.Commit("main", "a.txt", "a\n", secretMessage, secretAuthor, epoch.Add(time.Hour))
	e := newEnv(t, withGit(srv.URL))
	e.auth.SetTokenErr(auth.ErrNotLoggedIn)
	repo, err := e.st.CreateRepository(t.Context(), model.Repository{Provider: model.ProviderGitHub,
		Owner: testOwner, Name: "projet", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	r0 := e.syncAll().Repos[0]
	if r0.Status != wsync.StatusSuccess || r0.StatsFetched != 2 || len(r0.Warnings) != 0 {
		t.Fatalf("premier import sans gh : %+v", r0)
	}
	if add, _, known := e.stats(repo.ID, m0); !known || add != 2 {
		t.Errorf("m0 : +%d connu=%v", add, known)
	}
	if _, del, known := e.stats(repo.ID, m1); !known || del != 1 {
		t.Errorf("m1 : −%d connu=%v", del, known)
	}
}

// cancelStats annule la synchro pendant le calcul des statistiques.
type cancelStats struct {
	*gitprov.Provider
	cancel context.CancelFunc
}

func (c cancelStats) CommitStatsBatch(ctx context.Context, owner, name string, shas []string) (map[string]provider.CommitStats, error) {
	c.cancel()
	<-ctx.Done()
	return nil, ctx.Err()
}

// Annulation pendant le calcul : le dépôt échoue (« synchro annulée »), rien n'est écrit.
func TestCloneStatsCanceled(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	r.Commit("main", "a.txt", "a\n", secretMessage, secretAuthor, epoch)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	e := newEnv(t, withGit(srv.URL))
	e.gitWrap = func(p *gitprov.Provider) provider.Provider { return cancelStats{Provider: p, cancel: cancel} }
	e.eng = e.newEngine(e.st)
	repo := e.addRepo(1, "projet")
	e.mirror(srv.URL, 1, "projet")
	rep, err := e.eng.SyncAll(ctx, wsync.SyncOptions{})
	if err == nil {
		t.Error("synchro annulée sans erreur")
	}
	if r0 := rep.Repos[0]; r0.Status != wsync.StatusError || r0.Error != "synchro annulée" {
		t.Errorf("dépôt : %+v", r0)
	}
	if n, err := e.st.CountCommits(t.Context(), repo.ID); err != nil || n != 0 {
		t.Errorf("%d commits écrits malgré l'annulation (%v)", n, err)
	}
}
