package sync_test

// Conventions (phase 8.1): the report reads the precise scope in the existing clone (git rev-list of the
// tracked branches); without a clone, it falls back on commit_branches, which keeps rewritten commits.

import (
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
