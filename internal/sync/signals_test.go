package sync_test

// Diagnostic signals (phase 7.1): computed at sync from the local clones when diagnostic.enabled is
// set, merges left out, nothing without the setting, purge by retention, no personal data in the logs.

import (
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/diagnostic"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	"github.com/studoo-app/watchoo/internal/repoimport"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// withDiagnostic enables diagnostic.enabled with the given retention (in days).
func withDiagnostic(retentionDays int) envOption {
	return func(e *env) {
		e.cfg.Diagnostic.Enabled = true
		e.cfg.Diagnostic.RetentionDays = retentionDays
	}
}

// signalRepo creates a repository with, on main: a commit co-written with an AI tool (trailer), a
// commit by a bot touching an excluded file only, a binary file, an agent configuration file, and a
// merge of a branch dev (one commit). Dates: from base, one hour apart. Returns the SHAs by role.
func signalRepo(t *testing.T, srv *gittest.Server, base time.Time) (*gittest.Repo, map[string]string) {
	t.Helper()
	bot := gittest.Author{Name: "dependabot[bot]", Email: "bot@example.test"}
	r := srv.Create(testOwner, "projet", "main")
	shas := map[string]string{}
	shas["ai"] = r.Commit("main", "app/main.go", "package main\n\nfunc main() {}\n",
		secretMessage+"\n\nCo-authored-by: Claude <noreply@anthropic.com>", secretAuthor, base)
	shas["bot"] = r.Commit("main", "go.sum", "a v1\nb v2\n", secretMessage, bot, base.Add(time.Hour))
	shas["binary"] = r.Commit("main", "logo.png", "\x00\x01\x02", secretMessage, secretAuthor, base.Add(2*time.Hour))
	r.Branch("dev", shas["binary"])
	shas["dev"] = r.Commit("dev", "app/dev.go", "package main\n", secretMessage, secretAuthor, base.Add(3*time.Hour))
	shas["agents"] = r.Commit("main", "AGENTS.md", "# Agents\n", secretMessage, secretAuthor, base.Add(4*time.Hour))
	shas["merge"] = r.Merge("main", "dev", secretMessage, secretAuthor, base.Add(5*time.Hour))
	return r, shas
}

// addAndSync adds the repository through the local clones (gh not signed in) and syncs it.
func addAndSync(t *testing.T, e *env) (model.Repository, wsync.Report) {
	t.Helper()
	e.auth.SetTokenErr(auth.ErrNotLoggedIn)
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{{Ref: repoimport.Ref{Owner: testOwner, Name: "projet"}}},
		wsync.AddOptions{})
	if err != nil || res[0].Outcome != wsync.OutcomeAdded {
		t.Fatalf("addition: %+v, %v", res, err)
	}
	rep := e.syncAll()
	if len(rep.Repos) != 1 || rep.Repos[0].Status != wsync.StatusSuccess || len(rep.Repos[0].Warnings) != 0 {
		t.Fatalf("sync: %+v", rep.Repos)
	}
	assertReportNoLeak(t, rep)
	return res[0].Repo, rep
}

func TestSignalsComputedAtSync(t *testing.T) {
	srv := gittest.NewServer(t)
	r, shas := signalRepo(t, srv, epoch)
	e := newEnv(t, withGit(srv.URL), withDiagnostic(3650)) // 10 years: nothing purged
	repo, _ := addAndSync(t, e)

	got, err := e.st.CommitSignals(t.Context(), repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	bySHA := map[string]model.CommitSignal{}
	for _, s := range got {
		bySHA[s.SHA] = s
	}
	if len(got) != 5 {
		t.Errorf("%d commits with signals, want 5 (the merge left out): %+v", len(got), got)
	}
	if _, ok := bySHA[shas["merge"]]; ok {
		t.Error("the merge commit has signals")
	}
	if s := bySHA[shas["ai"]]; s.FilesKept != 1 || s.AdditionsKept != 3 || s.FilesExcluded != 0 ||
		!slices.Equal(s.Tools, []string{diagnostic.ToolClaude}) || s.BotAuthor || s.AgentConfig || s.AuthorID == 0 ||
		!s.AuthoredAt.Equal(epoch) || !s.ComputedAt.Equal(e.clock.Now()) {
		t.Errorf("AI commit: %+v", s)
	}
	if s := bySHA[shas["bot"]]; s.FilesKept != 0 || s.FilesExcluded != 1 || s.AdditionsKept != 0 || !s.BotAuthor ||
		len(s.Tools) != 0 {
		t.Errorf("bot commit (go.sum excluded): %+v", s)
	}
	if s := bySHA[shas["binary"]]; s.FilesKept != 0 || s.FilesExcluded != 1 {
		t.Errorf("binary file: %+v", s)
	}
	if s := bySHA[shas["dev"]]; s.FilesKept != 1 || s.AdditionsKept != 1 {
		t.Errorf("commit of dev: %+v", s)
	}
	if s := bySHA[shas["agents"]]; !s.AgentConfig || s.FilesKept != 1 {
		t.Errorf("agent configuration file: %+v", s)
	}

	// Incremental: a new commit gets its signals, the others are not rewritten.
	next := r.Commit("main", "app/next.go", "package main\n// x\n", secretMessage, secretAuthor, epoch.Add(6*time.Hour))
	e.clock.Set(e.clock.Now().Add(time.Hour))
	e.syncAll()
	again, err := e.st.CommitSignals(t.Context(), repo.ID)
	if err != nil || len(again) != 6 || again[0].SHA != next || !again[0].ComputedAt.Equal(e.clock.Now()) {
		t.Fatalf("after the incremental sync: %+v, %v", again, err)
	}
	for _, s := range again[1:] {
		if !s.ComputedAt.Equal(e.clock.Now().Add(-time.Hour)) {
			t.Errorf("signals of %s recomputed", s.SHA)
		}
	}
}

func TestSignalsDisabledByDefault(t *testing.T) {
	srv := gittest.NewServer(t)
	r, _ := signalRepo(t, srv, epoch)
	e := newEnv(t, withGit(srv.URL))
	repo, _ := addAndSync(t, e)
	if n, err := e.st.CountCommitSignals(t.Context(), repo.ID); err != nil || n != 0 {
		t.Errorf("%d commits with signals without diagnostic.enabled (%v), want 0", n, err)
	}
	// The line statistics are still computed as before.
	if v := e.commitView(repo.ID, r.Head("main")); !v.StatsKnown {
		t.Errorf("statistics of the last commit unknown: %+v", v)
	}
}

func TestSignalsPurgedAfterRetention(t *testing.T) {
	srv := gittest.NewServer(t)
	e := newEnv(t, withGit(srv.URL), withDiagnostic(2))
	// The repository spans six hours ending three days before the clock: with a retention of 2
	// days, everything written is purged at the end of the sync; a recent commit stays.
	r, shas := signalRepo(t, srv, e.clock.Now().Add(-3*24*time.Hour-6*time.Hour))
	recent := r.Commit("main", "app/recent.go", "package main\n", secretMessage,
		secretAuthor, e.clock.Now().Add(-time.Hour))
	repo, _ := addAndSync(t, e)
	got, err := e.st.CommitSignals(t.Context(), repo.ID)
	if err != nil || len(got) != 1 || got[0].SHA != recent {
		t.Errorf("signals after the purge: %+v, %v; want only the recent commit", got, err)
	}
	if _, ok := shas["ai"]; !ok {
		t.Fatal("test repository incomplete")
	}
}
