package sync_test

// Line statistics through the local Git clone ("always clone", 0.5.0): with gh, the API
// (providertest) and the Git repository (gittest) describe the same history, with the same SHAs
// (mirror); without gh, the clone is the source.

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

// mirror copies into the fake Provider (repository repoID) the commits and branches of the Git
// repository testOwner/name of the server srvURL: the API and the clone then describe the same
// history. Commits already copied are not copied twice.
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
	// Parents first.
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
			e.t.Fatal("mirror: parents not found")
		}
		pending = rest
	}
	for _, b := range branches {
		e.fake.SetBranch(repoID, b.Name, b.HeadSHA)
	}
}

// stats returns the statistics in the database of the commit sha of the repository repoID.
func (e *env) stats(repoID int64, sha string) (int, int, bool) {
	e.t.Helper()
	c := e.commitView(repoID, sha)
	return c.Additions, c.Deletions, c.StatsKnown
}

// forgetStats resets in the database the statistics of the commit sha to "unknown" (database from before 0.5.0).
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

	e := newEnv(t, withGit(srv.URL)) // gh signed in: API
	repo := e.addRepo(1, "projet")
	e.mirror(srv.URL, 1, "projet")

	// First import: all statistics through the clone, no CommitStats request.
	rep := e.syncAll()
	if rep.Source.Kind != wsync.SourceAPI {
		t.Fatalf("source %q", rep.Source.Kind)
	}
	r0 := rep.Repos[0]
	if r0.Status != wsync.StatusSuccess || r0.NewCommits != 5 || r0.StatsFetched != 5 || len(r0.Warnings) != 0 {
		t.Fatalf("first import: %+v", r0)
	}
	if n := e.fake.Calls(providertest.MethodCommitStats); n != 0 {
		t.Errorf("%d CommitStats calls to the API, none expected", n)
	}
	for _, tc := range []struct {
		sha      string
		add, del int
	}{{m0, 2, 0}, {f1, 1, 0}, {m1, 1, 1}, {x, 1, 0}, {empty, 0, 0}} {
		if add, del, known := e.stats(repo.ID, tc.sha); !known || add != tc.add || del != tc.del {
			t.Errorf("commit %s: +%d −%d known=%v, want +%d −%d", provider.ShortSHA(tc.sha), add, del, known,
				tc.add, tc.del)
		}
	}
	// Clone created in repos/, on the default branch of the API.
	if got, err := os.ReadFile(filepath.Join(e.reposDir, "projet", "a.txt")); err != nil || string(got) != "a\nc\n" {
		t.Errorf("working copy: %q, %v", got, err)
	}

	// Old commit without statistics (database from before migration 00006) and new commit: both
	// are computed; the empty commit, known at 0/0, is never computed again.
	e.forgetStats(repo.ID, m0)
	m2 := r.Commit("main", "c.txt", "1\n2\n3\n", secretMessage, secretAuthor, epoch.Add(5*time.Hour))
	e.mirror(srv.URL, 1, "projet")
	e.clock.Set(e.clock.Now().Add(time.Hour))
	r0 = e.syncAll().Repos[0]
	if r0.Status != wsync.StatusSuccess || r0.NewCommits != 1 || r0.StatsFetched != 2 {
		t.Fatalf("next sync: %+v", r0)
	}
	if add, _, known := e.stats(repo.ID, m0); !known || add != 2 {
		t.Errorf("old commit: +%d known=%v", add, known)
	}
	if add, _, known := e.stats(repo.ID, m2); !known || add != 3 {
		t.Errorf("new commit: +%d known=%v", add, known)
	}
	r0 = e.syncAll().Repos[0]
	if r0.StatsFetched != 0 {
		t.Errorf("sync without change: %d statistics computed, none expected", r0.StatsFetched)
	}

	// Totals of the repository: all commits counted.
	sum, err := e.st.RepositorySummary(t.Context(), repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := (model.LineTotals{Commits: 6, KnownCommits: 6, Additions: 8, Deletions: 1}); sum.Lines != want {
		t.Errorf("totals of the repository = %+v, want %+v", sum.Lines, want)
	}
}

// More than MaxStatsPerRepo commits at once: all computed by the clone (no cap).
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
		t.Fatalf("sync of %d commits: %+v", n, r0)
	}
	sum, err := e.st.RepositorySummary(t.Context(), repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Lines.Uncounted() != 0 || sum.Lines.Additions != n+1 || sum.Lines.Deletions != n {
		t.Errorf("totaux = %+v", sum.Lines)
	}
}

// Clone unavailable with gh (location occupied): the repository succeeds, with a warning, and the
// statistics fall back to the API (nothing on the first import, then the new commits); once the
// clone is back, the commits that stayed unknown are computed.
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
		!strings.Contains(r0.Warnings[0].Message, "local Git clone unavailable, line statistics read through the API") ||
		!strings.Contains(r0.Warnings[0].Message, "clone location occupied") {
		t.Fatalf("clone occupied: %+v", r0)
	}
	// Recorded on the sync_run, with its code, for the web interface.
	if w := e.lastRun(repo.ID).Warnings; len(w) != 1 || w[0] != r0.Warnings[0] || w[0].Kind != model.WarnCloneUnavailable {
		t.Errorf("recorded warnings: %+v", w)
	}
	if _, _, known := e.stats(repo.ID, m0); known || e.fake.Calls(providertest.MethodCommitStats) != 0 {
		t.Errorf("first import without clone: statistics known=%v, %d calls", known,
			e.fake.Calls(providertest.MethodCommitStats))
	}

	// New commit: statistics requested from the API (limited fallback).
	m1 := r.Commit("main", "a.txt", "a\nb\n", secretMessage, secretAuthor, epoch.Add(time.Hour))
	e.mirror(srv.URL, 1, "projet")
	e.fake.SetStats(1, m1, provider.CommitStats{Additions: 7})
	r0 = e.syncAll().Repos[0]
	if r0.Status != wsync.StatusSuccess || r0.StatsFetched != 1 || len(r0.Warnings) != 1 {
		t.Fatalf("fallback on the API: %+v", r0)
	}
	if add, _, known := e.stats(repo.ID, m1); !known || add != 7 {
		t.Errorf("statistics through the API: +%d known=%v", add, known)
	}

	// Location freed: the clone is created and catches up the commit that stayed unknown; the one read
	// through the API keeps its lines, only its languages are read (two commits read in the clone).
	if err := os.RemoveAll(occupied); err != nil {
		t.Fatal(err)
	}
	r0 = e.syncAll().Repos[0]
	if r0.Status != wsync.StatusSuccess || len(r0.Warnings) != 0 || r0.StatsFetched != 2 {
		t.Fatalf("clone back: %+v", r0)
	}
	if w := e.lastRun(repo.ID).Warnings; len(w) != 0 {
		t.Errorf("warnings of the previous sync still shown: %+v", w)
	}
	if add, _, known := e.stats(repo.ID, m0); !known || add != 1 {
		t.Errorf("catch-up: +%d known=%v", add, known)
	}
	if add, _, _ := e.stats(repo.ID, m1); add != 7 {
		t.Errorf("commit read through the API computed again: +%d", add)
	}
	assertNoLeak(t, "logs", e.logs.String())
}

// Provider of the clones impossible to build with gh: warning for each repository, fallback.
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
		!strings.Contains(r0.Warnings[0].Message, "git not found") {
		t.Errorf("git missing with gh: %+v", r0)
	}
}

// Without gh: statistics of all commits from the first import.
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
		t.Fatalf("first import without gh: %+v", r0)
	}
	if add, _, known := e.stats(repo.ID, m0); !known || add != 2 {
		t.Errorf("m0: +%d known=%v", add, known)
	}
	if _, del, known := e.stats(repo.ID, m1); !known || del != 1 {
		t.Errorf("m1: −%d known=%v", del, known)
	}
}

// cancelStats cancels the sync during the computation of the statistics.
type cancelStats struct {
	*gitprov.Provider
	cancel context.CancelFunc
}

func (c cancelStats) CommitStatsBatch(ctx context.Context, owner, name string, shas []string) (map[string]provider.CommitStats, error) {
	c.cancel()
	<-ctx.Done()
	return nil, ctx.Err()
}

// Cancellation during the computation: the repository fails ("sync canceled"), nothing is written.
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
		t.Error("canceled sync without error")
	}
	if r0 := rep.Repos[0]; r0.Status != wsync.StatusError || r0.Error != "sync canceled" {
		t.Errorf("repository: %+v", r0)
	}
	if n, err := e.st.CountCommits(t.Context(), repo.ID); err != nil || n != 0 {
		t.Errorf("%d commits written despite the cancellation (%v)", n, err)
	}
}
