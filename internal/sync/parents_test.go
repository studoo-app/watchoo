package sync_test

// Number of parents of the commits (phase 8.1): written at insertion from provider.Commit.Parents,
// backfilled by the clone for the commits already in the database without it.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// parents returns the parent count in the database of the commit sha (-1 = unknown).
func (e *env) parents(repoID int64, sha string) int {
	e.t.Helper()
	if p := e.commitView(repoID, sha).ParentCount; p != nil {
		return *p
	}
	return -1
}

// forgetParents resets in the database the parent count of every commit (database from before 00016).
func (e *env) forgetParents(repoID int64) {
	e.t.Helper()
	if _, err := e.st.DB().ExecContext(e.t.Context(),
		`UPDATE commits SET parent_count = NULL WHERE repo_id = ?`, repoID); err != nil {
		e.t.Fatal(err)
	}
}

func TestParentCountsWrittenAndBackfilled(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	m0 := r.Commit("main", "a.txt", "a\n", secretMessage, secretAuthor, epoch)
	r.Branch("dev", m0)
	f1 := r.Commit("dev", "b.txt", "x\n", secretMessage, secretAuthor, epoch.Add(time.Hour))
	m1 := r.Commit("main", "a.txt", "b\n", secretMessage, secretAuthor, epoch.Add(2*time.Hour))
	x := r.Merge("main", "dev", secretMessage, secretAuthor, epoch.Add(3*time.Hour))
	want := map[string]int{m0: 0, f1: 1, m1: 1, x: 2}

	e := newEnv(t, withGit(srv.URL)) // gh signed in: API, and the clone
	repo := e.addRepo(1, "projet")
	e.mirror(srv.URL, 1, "projet")

	// Insertion: the count comes from the commit read (API here).
	if r0 := e.syncAll().Repos[0]; r0.Status != wsync.StatusSuccess {
		t.Fatalf("first import: %+v", r0)
	}
	for sha, n := range want {
		if got := e.parents(repo.ID, sha); got != n {
			t.Errorf("commit %s: %d parents, want %d", provider.ShortSHA(sha), got, n)
		}
	}

	// Database from before migration 00016: the next sync backfills through the clone.
	e.forgetParents(repo.ID)
	e.clock.Set(e.clock.Now().Add(time.Hour))
	if r0 := e.syncAll().Repos[0]; r0.Status != wsync.StatusSuccess || len(r0.Warnings) != 0 {
		t.Fatalf("backfill: %+v", r0)
	}
	for sha, n := range want {
		if got := e.parents(repo.ID, sha); got != n {
			t.Errorf("backfilled commit %s: %d parents, want %d", provider.ShortSHA(sha), got, n)
		}
	}
}

// failParents fails the reading of the parent counts.
type failParents struct{ *gitprov.Provider }

func (failParents) ParentCounts(context.Context, string, string, []string) (map[string]int, error) {
	return nil, errors.New("rev-list failed")
}

// A failed backfill does not fail the repository: the counts stay unknown, without warning.
func TestParentCountsBackfillFailure(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	m0 := r.Commit("main", "a.txt", "a\n", secretMessage, secretAuthor, epoch)
	e := newEnv(t, withGit(srv.URL))
	e.gitWrap = func(p *gitprov.Provider) provider.Provider { return failParents{p} }
	e.eng = e.newEngine(e.st)
	repo := e.addRepo(1, "projet")
	e.mirror(srv.URL, 1, "projet")
	e.syncAll()
	e.forgetParents(repo.ID)
	e.clock.Set(e.clock.Now().Add(time.Hour))
	if r0 := e.syncAll().Repos[0]; r0.Status != wsync.StatusSuccess || len(r0.Warnings) != 0 {
		t.Fatalf("sync: %+v", r0)
	}
	if got := e.parents(repo.ID, m0); got != -1 {
		t.Errorf("parent count %d, want unknown", got)
	}
}
