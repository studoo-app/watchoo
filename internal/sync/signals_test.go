package sync_test

// Diagnostic signals (phase 7.1): computed at sync from the local clones when diagnostic.enabled is
// set, merges left out, nothing without the setting, purge by retention, no personal data in the logs.

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/diagnostic"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	"github.com/studoo-app/watchoo/internal/repoimport"
	"github.com/studoo-app/watchoo/internal/store"
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
		!slices.Equal(s.Tools, []string{diagnostic.SignalCoAuthor + ":" + diagnostic.ToolClaude}) || s.BotAuthor || s.AgentConfig || s.AuthorID == 0 ||
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
	// Form (phase 7.5): measured on the added lines, nothing but numbers kept.
	if f := bySHA[shas["ai"]].Form; !f.Known || f.SourceLines != 2 || f.Functions != 1 || f.CommentLines != 0 ||
		f.MessageLength == 0 {
		t.Errorf("form of the AI commit: %+v", f)
	}
	if f := bySHA[shas["bot"]].Form; !f.Known || f.SourceLines != 0 { // go.sum excluded, not a source file
		t.Errorf("form of the bot commit: %+v", f)
	}

	// Incremental: a new commit gets its signals, the others are not rewritten.
	next := r.Commit("main", "app/next.go", "package main\n// TODO: x\n", secretMessage, secretAuthor, epoch.Add(6*time.Hour))
	e.clock.Set(e.clock.Now().Add(time.Hour))
	e.syncAll()
	again, err := e.st.CommitSignals(t.Context(), repo.ID)
	if err != nil || len(again) != 6 || again[0].SHA != next || !again[0].ComputedAt.Equal(e.clock.Now()) {
		t.Fatalf("after the incremental sync: %+v, %v", again, err)
	}
	if f := again[0].Form; !f.Known || f.WIPMarkers != 1 || f.CommentLines != 1 {
		t.Errorf("phase 7.7 measures of the new commit: %+v", f)
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

// D2 on pull requests: the description is read in memory at sync, only the tools are kept, and only
// with diagnostic.enabled; D5 comes from the branch name. diagnostic.Build assembles the leads.
func TestPullRequestDeclarationsAtSync(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		opts := []envOption{}
		if enabled {
			opts = append(opts, withDiagnostic(3650))
		}
		e := newEnv(t, opts...)
		repo := e.addRepo(1, "projet")
		e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)
		agent := fakePR(1, provider.PullRequestOpen, 1)
		agent.HeadBranch = "codex/fix-login"
		agent.Body = secretMessage + "\n\n🤖 Generated with [Claude Code](https://claude.com/claude-code)"
		plain := fakePR(2, provider.PullRequestOpen, 2)
		plain.Body = secretMessage
		e.fake.SetPullRequests(1, agent, plain)
		e.syncOK(repo.ID)

		byNumber := map[int][]string{}
		for _, p := range e.pulls(repo.ID) {
			byNumber[p.Number] = p.ToolIDs
		}
		want := []string{"D2:claude", "D2:unnamed"}
		if !enabled {
			want = []string{}
		}
		if !slices.Equal(byNumber[1], want) || len(byNumber[2]) != 0 {
			t.Errorf("enabled=%v: tool_ids %v / %v, want %v / []", enabled, byNumber[1], byNumber[2], want)
		}
		if !enabled {
			continue
		}
		rep, err := diagnostic.Build(t.Context(), e.st, store.DiagnosticFilter{}, diagnostic.RhythmOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Leads) != 1 {
			t.Fatalf("leads: %+v", rep.Leads)
		}
		l := rep.Leads[0]
		wantHits := []diagnostic.Hit{{Signal: "D2", Value: "claude"}, {Signal: "D2", Value: "unnamed"},
			{Signal: "D5", Value: "codex/fix-login"}}
		if l.Kind != diagnostic.KindPullRequest || l.Level != diagnostic.LevelDeclared || l.Number != 1 ||
			l.AuthorLogin != secretLogin || !slices.Equal(l.Hits, wantHits) {
			t.Errorf("PR lead: %+v", l)
		}
		// Filters: another author, or a period after the opening, leave nothing.
		for _, f := range []store.DiagnosticFilter{{Author: "quelquun"}, {Since: epoch.Add(time.Minute)}} {
			if r, err := diagnostic.Build(t.Context(), e.st, f, diagnostic.RhythmOptions{}); err != nil || len(r.Leads) != 0 {
				t.Errorf("filter %+v: %+v, %v", f, r.Leads, err)
			}
		}
		// Retention: a PR opened before the cutoff is no longer a lead, even if its branch (D5) is still stored.
		if r, err := diagnostic.Build(t.Context(), e.st, store.DiagnosticFilter{},
			diagnostic.RhythmOptions{RetentionCutoff: epoch.Add(time.Minute)}); err != nil || len(r.Leads) != 0 {
			t.Errorf("PR outside the retention: %+v, %v", r.Leads, err)
		}
		if r, _ := diagnostic.Build(t.Context(), e.st, store.DiagnosticFilter{Author: strings.ToUpper(secretLogin)},
			diagnostic.RhythmOptions{}); len(r.Leads) != 1 {
			t.Errorf("author login, other case: %+v", r.Leads)
		}
	}
}

// D4 context and commit leads, on a real Git repository (local clones).
func TestAgentContextAndCommitLeads(t *testing.T) {
	srv := gittest.NewServer(t)
	_, shas := signalRepo(t, srv, epoch)
	e := newEnv(t, withGit(srv.URL), withDiagnostic(3650))
	repo, _ := addAndSync(t, e)

	rep, err := diagnostic.Build(t.Context(), e.st, store.DiagnosticFilter{RepoID: repo.ID}, diagnostic.RhythmOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Contexts) != 1 || !rep.Contexts[0].Since.Equal(epoch.Add(4*time.Hour)) {
		t.Errorf("D4 context: %+v", rep.Contexts)
	}
	if len(rep.Leads) != 2 || rep.Leads[0].SHA != shas["bot"] || rep.Leads[1].SHA != shas["ai"] {
		t.Fatalf("commit leads: %+v", rep.Leads)
	}
	if h := rep.Leads[0].Hits; len(h) != 1 || h[0].Signal != diagnostic.SignalBot || h[0].Value != "dependabot[bot]" {
		t.Errorf("bot lead: %+v", h)
	}
	if h := rep.Leads[1].Hits; !slices.Equal(h, []diagnostic.Hit{{Signal: "D1", Value: "claude"}}) ||
		rep.Leads[1].URL == "" || rep.Leads[1].Kind != diagnostic.KindCommit {
		t.Errorf("AI lead: %+v", rep.Leads[1])
	}

	// Without agent configuration file: no context.
	srv2 := gittest.NewServer(t)
	r2 := srv2.Create(testOwner, "projet", "main")
	r2.Commit("main", "main.go", "package main\n", secretMessage, secretAuthor, epoch)
	e2 := newEnv(t, withGit(srv2.URL), withDiagnostic(3650))
	repo2, _ := addAndSync(t, e2)
	if ctxs, err := e2.st.AgentConfigContexts(t.Context(), repo2.ID); err != nil || len(ctxs) != 0 {
		t.Errorf("context without agent file: %+v, %v", ctxs, err)
	}
}

// History backfill: commits synced before the diagnosis was enabled are measured afterwards, on the
// clones, merges excluded, once only, within the retention period.
func TestBackfillSignals(t *testing.T) {
	srv := gittest.NewServer(t)
	_, shas := signalRepo(t, srv, epoch)
	e := newEnv(t, withGit(srv.URL))
	repo, _ := addAndSync(t, e)
	ctx := t.Context()
	if _, err := e.eng.BackfillSignals(ctx, 0); !errors.Is(err, wsync.ErrDiagnosticDisabled) {
		t.Fatalf("backfill with the diagnosis disabled: %v", err)
	}

	// Short retention: everything is too old.
	e.cfg.Diagnostic.Enabled, e.cfg.Diagnostic.RetentionDays = true, 1
	res, err := e.newEngine(e.st).BackfillSignals(ctx, 0)
	if err != nil || len(res) != 1 || res[0].Measured != 0 || res[0].Error != "" {
		t.Fatalf("backfill outside the retention: %+v, %v", res, err)
	}

	// Without clone, the backfill creates it first (otherwise it reads the existing clone, without fetch).
	if err := os.RemoveAll(e.reposDir); err != nil {
		t.Fatal(err)
	}
	e.cfg.Diagnostic.RetentionDays = 3650
	// A row written before phase 7.5 (form not measured): completed by the backfill.
	if n, err := e.st.WriteCommitSignals(ctx, repo.ID, []store.CommitSignalWrite{{SHA: shas["ai"], FilesKept: 1}},
		e.clock.Now()); err != nil || n != 1 {
		t.Fatalf("row without form: %d, %v", n, err)
	}
	eng := e.newEngine(e.st)
	res, err = eng.BackfillSignals(ctx, repo.ID)
	if err != nil || len(res) != 1 || res[0].Measured != 5 || res[0].Merges != 1 || res[0].Missing != 0 || res[0].Error != "" {
		t.Fatalf("backfill: %+v, %v", res, err)
	}
	got, err := e.st.CommitSignals(ctx, repo.ID)
	if err != nil || len(got) != 5 {
		t.Fatalf("signals after the backfill: %d, %v", len(got), err)
	}
	for _, s := range got {
		if !s.Form.Known {
			t.Errorf("form of %s not measured by the backfill", s.SHA)
		}
		if s.SHA == shas["merge"] {
			t.Error("the merge commit was measured")
		}
		if s.SHA == shas["ai"] && !slices.Equal(s.Tools, []string{"D1:claude"}) {
			t.Errorf("AI commit after the backfill: %+v", s)
		}
	}
	if ctxs, _ := e.st.AgentConfigContexts(ctx, repo.ID); len(ctxs) != 1 {
		t.Errorf("D4 context after the backfill: %+v", ctxs)
	}
	// Second pass: nothing left to measure.
	if res, err := eng.BackfillSignals(ctx, 0); err != nil || res[0].Measured != 0 || res[0].Merges != 1 {
		t.Errorf("second backfill: %+v, %v", res, err)
	}
}
