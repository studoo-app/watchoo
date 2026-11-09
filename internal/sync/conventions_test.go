package sync_test

// Conventions (phase 8.1): the report reads the precise scope in the existing clone (git rev-list of the
// tracked branches); without a clone, it falls back on commit_branches, which keeps rewritten commits.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/provider"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	"github.com/studoo-app/watchoo/internal/repoimport"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

func TestConventionsScopeFromClone(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	m0 := r.Commit("main", "README.md", "a\n", "feat: initialise le projet", secretAuthor, epoch)
	r.Branch("dev", m0)
	r.Commit("dev", "app.go", "package app\n", "wip", secretAuthor, epoch.Add(time.Hour))
	r.Commit("main", "README.md", "b\n", "docs: complete le README", secretAuthor, epoch.Add(2*time.Hour))

	e := newEnv(t, withGit(srv.URL))
	e.auth.SetTokenErr(auth.ErrNotLoggedIn) // no gh: the sync goes through the clone
	ctx := t.Context()
	res, err := e.eng.AddRepositories(ctx, []wsync.AddRequest{{Ref: repoimport.Ref{Owner: testOwner, Name: "projet"}}}, wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	repoID := res[0].Repo.ID
	if rep := e.syncAll(); rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("sync: %+v", rep.Repos)
	}
	clones, err := e.eng.ConventionsClones(ctx)
	if err != nil || clones == nil {
		t.Fatalf("ConventionsClones = %v, %v", clones, err)
	}
	build := func(cr conventions.CloneReader) conventions.RepoReport {
		t.Helper()
		got, err := conventions.Build(ctx, e.st, cr, conventions.Filter{RepoID: repoID},
			conventions.BuildOptions{Rules: conventions.DefaultRules()})
		if err != nil || len(got.Repos) != 1 {
			t.Fatalf("Build = %+v, %v", got, err)
		}
		return got.Repos[0]
	}
	rep := build(clones)
	if rep.Source != conventions.SourceClone || rep.Evaluated != 3 || rep.Conform != 2 ||
		rep.DefaultBranch.Evaluated != 2 || rep.DefaultBranch.Conform != 2 {
		t.Errorf("from the clone: source %s, %d/%d, default branch %d/%d", rep.Source, rep.Conform, rep.Evaluated,
			rep.DefaultBranch.Conform, rep.DefaultBranch.Evaluated)
	}

	// dev is rewritten (push --force): the clone forgets the commit, the database keeps it.
	r.Reset("dev", m0)
	e.clock.Set(e.clock.Now().Add(time.Hour))
	if rep := e.syncAll(); rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("second sync: %+v", rep.Repos)
	}
	if clones, err = e.eng.ConventionsClones(ctx); err != nil {
		t.Fatal(err)
	}
	if rep := build(clones); rep.Source != conventions.SourceClone || rep.Evaluated != 2 || rep.Conform != 2 {
		t.Errorf("after the rewrite, from the clone: %s %d/%d", rep.Source, rep.Conform, rep.Evaluated)
	}
	if rep := build(nil); rep.Source != conventions.SourceDatabase || rep.Evaluated != 3 {
		t.Errorf("after the rewrite, database fallback: %s %d/%d", rep.Source, rep.Conform, rep.Evaluated)
	}
	if strings.Contains(e.logs.String(), "wip") || strings.Contains(e.logs.String(), "initialise") {
		t.Errorf("commit message in the logs:\n%s", e.logs.String())
	}
}

// Without clones (no directory of the clones), or with a provider that cannot read them, the report
// falls back on the database.
func TestConventionsClonesUnavailable(t *testing.T) {
	e := newEnv(t)
	if cr, err := e.eng.ConventionsClones(t.Context()); err != nil || cr != nil {
		t.Errorf("clones disabled: %v, %v", cr, err)
	}
	srv := gittest.NewServer(t)
	e = newEnv(t, withGit(srv.URL))
	e.gitWrap = func(p *gitprov.Provider) provider.Provider { return failParents{p} }
	if cr, err := e.eng.ConventionsClones(t.Context()); err != nil || cr == nil {
		t.Errorf("embedded provider: %v, %v", cr, err)
	}
	e.gitWrap = func(p *gitprov.Provider) provider.Provider { return opaque{p} }
	if cr, err := e.eng.ConventionsClones(t.Context()); err != nil || cr != nil {
		t.Errorf("provider without RevList: %v, %v", cr, err)
	}
}

// opaque hides every optional method of the Git provider.
type opaque struct{ provider.Provider }

// Phase 8.2 in clone-only mode: tags read in the database (written by the sync from the clone), remote
// branches and tag intervals read in the clone without fetching; pull requests and releases unknown.
func TestConventionsTagsAndBranchesFromClone(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	c1 := r.Commit("main", "README.md", "a\n", "feat: initialise le projet", secretAuthor, epoch)
	r.Tag("v0.1.0", c1, "")
	r.Commit("main", "README.md", "b\n", "wip", secretAuthor, epoch.Add(time.Hour))
	c3 := r.Commit("main", "README.md", "c\n", "fix: corrige le titre", secretAuthor, epoch.Add(2*time.Hour))
	r.Tag("v1.0.0", c3, "version 1") // annotated: the database keeps the commit
	r.Branch("feat/add-login", c3)
	r.Branch("Mauvaise_Branche", c3)
	r.Commit("main", "README.md", "d\n", "docs: complete le README", secretAuthor, epoch.Add(3*time.Hour))

	e := newEnv(t, withGit(srv.URL))
	e.auth.SetTokenErr(auth.ErrNotLoggedIn)
	ctx := t.Context()
	res, err := e.eng.AddRepositories(ctx, []wsync.AddRequest{{Ref: repoimport.Ref{Owner: testOwner, Name: "projet"}}}, wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep := e.syncAll(); rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("sync: %+v", rep.Repos)
	}
	clones, err := e.eng.ConventionsClones(ctx)
	if err != nil || clones == nil {
		t.Fatalf("ConventionsClones = %v, %v", clones, err)
	}
	ms, _ := conventions.ParseMilestones([]string{"v1.0.0", "v2.0.0"})
	opts := conventions.BuildOptions{Rules: conventions.DefaultRules(), Milestones: ms, LocalOnly: true, Since: "2020-01-01",
		Branch: conventions.BranchRules{Types: conventions.DefaultTypes(), Exclude: conventions.DefaultBranchExclude()}}
	f := conventions.Filter{RepoID: res[0].Repo.ID}
	got, err := conventions.Build(ctx, e.st, clones, f, opts)
	if err != nil || len(got.Repos) != 1 {
		t.Fatalf("Build = %+v, %v", got, err)
	}
	rep := got.Repos[0]
	if rep.Tags.Checked != 2 || rep.Tags.Conform != 2 || rep.Tags.Milestones[0].Status != conventions.MilestonePresent ||
		rep.Tags.Milestones[1].Status != conventions.MilestoneAbsent || rep.Tags.Releases.Known {
		t.Errorf("tags: %+v", rep.Tags)
	}
	var branches []string
	for _, b := range rep.Branches.Branches {
		branches = append(branches, b.Name)
	}
	if rep.Branches.Source != conventions.SourceClone || strings.Join(branches, ",") != "Mauvaise_Branche,feat/add-login" ||
		rep.Branches.Conform != 1 {
		t.Errorf("branches: %s %v", rep.Branches.Source, branches)
	}
	if rep.Pulls.Known || rep.Pulls.Reason != conventions.UnknownNoAPI {
		t.Errorf("pulls: %+v", rep.Pulls)
	}
	iv := rep.Intervals
	if !iv.Known || iv.LastTag != "v1.0.0" || len(iv.Intervals) != 2 || iv.Intervals[0].Evaluated != 1 ||
		iv.Intervals[1].Evaluated != 2 || iv.Intervals[1].Conform != 1 || iv.Intervals[1].Missing != 0 ||
		iv.SinceLastTag.Evaluated != 1 {
		t.Errorf("intervals: %+v", iv)
	}

	review, err := conventions.ReviewMilestone(ctx, e.st, clones, f, "v1.0.0", opts, time.Time{})
	if err != nil || len(review.Repos) != 1 {
		t.Fatalf("ReviewMilestone = %+v, %v", review, err)
	}
	if p := review.Repos[0]; p.Interval == nil || p.Interval.From != "v0.1.0" || p.Interval.Evaluated != 2 ||
		p.Conform != 2 || len(p.Criteria) != 4 {
		t.Errorf("review: %+v", p)
	}
	if strings.Contains(e.logs.String(), "wip") || strings.Contains(e.logs.String(), "corrige") {
		t.Errorf("commit message in the logs:\n%s", e.logs.String())
	}
}

// ConventionsLocalOnly: clone only for SourceGit, API mode assumed when the source cannot be read, error
// only for an ended context.
func TestConventionsLocalOnly(t *testing.T) {
	src := func(s wsync.Source, err error) func(context.Context) (wsync.Source, error) {
		return func(context.Context) (wsync.Source, error) { return s, err }
	}
	for name, tc := range map[string]struct {
		source func(context.Context) (wsync.Source, error)
		want   bool
	}{
		"git":        {src(wsync.Source{Kind: wsync.SourceGit}, nil), true},
		"api":        {src(wsync.Source{Kind: wsync.SourceAPI}, nil), false},
		"unreadable": {src(wsync.Source{}, errors.New("gh broken")), false},
	} {
		if got, err := wsync.ConventionsLocalOnly(t.Context(), tc.source); err != nil || got != tc.want {
			t.Errorf("%s: %v, %v", name, got, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := wsync.ConventionsLocalOnly(ctx, src(wsync.Source{}, context.Canceled)); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
}
