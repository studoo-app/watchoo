package git_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

// RevList reads the commits reachable from the tracked branches of the existing clone, each once; a
// branch missing from the clone is ignored; without a clone, ErrNotFound; nothing is fetched.
func TestRevList(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	root := r.Commit("main", "a.txt", "a\n", "racine", alice, t0)
	r.Branch("dev", root)
	f1 := r.Commit("dev", "b.txt", "x\n", "f1", bob, t0.Add(time.Hour))
	m1 := r.Commit("main", "a.txt", "b\n", "m1", alice, t0.Add(2*time.Hour))
	r.Branch("other", root)
	o1 := r.Commit("other", "c.txt", "y\n", "o1", bob, t0.Add(3*time.Hour))

	p, dir, _ := newProvider(t, srv)
	ctx := t.Context()
	if _, err := p.RevList(ctx, "alice", "projet", []string{"main"}); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("without a clone: %v, want ErrNotFound", err)
	}
	if err := p.Prepare(ctx, "alice", "projet", "main"); err != nil {
		t.Fatal(err)
	}
	got, err := p.RevList(ctx, "alice", "projet", []string{"main", "dev", "gone"})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	want := []string{root, f1, m1}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("RevList(main, dev) = %v, want %v", got, want)
	}
	if got, err := p.RevList(ctx, "alice", "projet", []string{"other"}); err != nil || !slices.Equal(got, []string{o1, root}) {
		t.Errorf("RevList(other) = %v, %v", got, err)
	}

	// The clone is read as is: a commit pushed afterwards is unknown.
	later := r.Commit("main", "a.txt", "c\n", "later", alice, t0.Add(4*time.Hour))
	if got, err := fresh(t, srv, dir).RevList(ctx, "alice", "projet", []string{"main"}); err != nil || slices.Contains(got, later) {
		t.Errorf("commit not fetched: %v, %v", got, err)
	}

	if none, err := p.RevList(ctx, "alice", "projet", nil); err != nil || none == nil || len(none) != 0 {
		t.Errorf("no branch: %v, %v", none, err)
	}
	if _, err := p.RevList(ctx, "alice", "projet", []string{""}); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("empty branch: %v, want ErrInvalidRef", err)
	}
	if _, err := p.RevList(ctx, "alice", "../x", []string{"main"}); err == nil {
		t.Error("invalid repository accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.RevList(canceled, "alice", "projet", []string{"main"}); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled context: %v", err)
	}
}

// RemoteBranches lists the remote branches of the existing clone without fetching; RevRange reads the
// commits between two commits (tag interval) or from branches minus a commit, without fetching, and
// refuses a commit absent from the clone instead of ignoring it.
func TestRemoteBranchesAndRevRange(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	root := r.Commit("main", "a.txt", "a\n", "racine", alice, t0)
	m1 := r.Commit("main", "a.txt", "b\n", "m1", alice, t0.Add(time.Hour))
	r.Branch("feat/login-page", m1)
	f1 := r.Commit("feat/login-page", "b.txt", "x\n", "f1", bob, t0.Add(2*time.Hour))
	m2 := r.Commit("main", "a.txt", "c\n", "m2", alice, t0.Add(3*time.Hour))

	p, dir, _ := newProvider(t, srv)
	ctx := t.Context()
	if _, err := p.RemoteBranches(ctx, "alice", "projet"); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("RemoteBranches without a clone: %v, want ErrNotFound", err)
	}
	if _, err := p.RevRange(ctx, "alice", "projet", nil, []string{m2}, nil); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("RevRange without a clone: %v, want ErrNotFound", err)
	}
	if err := p.Prepare(ctx, "alice", "projet", "main"); err != nil {
		t.Fatal(err)
	}
	if got, err := p.RemoteBranches(ctx, "alice", "projet"); err != nil || !slices.Equal(got, []string{"feat/login-page", "main"}) {
		t.Errorf("RemoteBranches = %v, %v", got, err)
	}
	// A branch created afterwards is unknown: no fetch.
	r.Branch("later", m2)
	if got, err := fresh(t, srv, dir).RemoteBranches(ctx, "alice", "projet"); err != nil || slices.Contains(got, "later") {
		t.Errorf("branch not fetched: %v, %v", got, err)
	}

	sorted := func(s []string) []string { s = slices.Clone(s); slices.Sort(s); return s }
	if got, err := p.RevRange(ctx, "alice", "projet", nil, []string{m2}, []string{root}); err != nil ||
		!slices.Equal(sorted(got), sorted([]string{m1, m2})) {
		t.Errorf("RevRange(m2 ^root) = %v, %v", got, err)
	}
	if got, err := p.RevRange(ctx, "alice", "projet", nil, []string{m1}, nil); err != nil ||
		!slices.Equal(sorted(got), sorted([]string{root, m1})) {
		t.Errorf("RevRange(m1) = %v, %v", got, err)
	}
	if got, err := p.RevRange(ctx, "alice", "projet", []string{"main", "feat/login-page", "gone"}, nil, []string{m1}); err != nil ||
		!slices.Equal(sorted(got), sorted([]string{f1, m2})) {
		t.Errorf("RevRange(branches ^m1) = %v, %v", got, err)
	}
	if _, err := p.RevRange(ctx, "alice", "projet", []string{"gone"}, nil, []string{m1}); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("no requested branch in the clone: %v, want ErrNotFound", err)
	}
	if got, err := p.RevRange(ctx, "alice", "projet", nil, nil, []string{m1}); err != nil || got == nil || len(got) != 0 {
		t.Errorf("nothing to include: %v, %v", got, err)
	}
	missing := strings.Repeat("ab", 20)
	if _, err := p.RevRange(ctx, "alice", "projet", nil, []string{m2}, []string{missing}); err == nil {
		t.Error("RevRange accepted an excluded commit absent from the clone")
	}
	if _, err := p.RevRange(ctx, "alice", "projet", nil, []string{"HEAD"}, nil); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("non-SHA include: %v, want ErrInvalidRef", err)
	}
	if _, err := p.RevRange(ctx, "alice", "projet", []string{""}, nil, nil); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("empty branch: %v, want ErrInvalidRef", err)
	}
	if _, err := p.RemoteBranches(ctx, "alice", "../x"); err == nil {
		t.Error("invalid repository accepted")
	}
}

// FirstParent follows the first parent of the default branch (merged commits of a branch are not
// listed), without the root commit, filtered by committer date, from a branch or a commit minus
// excluded commits; a missing branch or commit is an error, never an empty list.
func TestFirstParent(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	root := r.Commit("main", "a.txt", "a\n", "racine", alice, t0)
	m1 := r.Commit("main", "a.txt", "b\n", "m1", alice, t0.Add(time.Hour))
	r.Branch("feat/x", m1)
	f1 := r.Commit("feat/x", "b.txt", "x\n", "f1", bob, t0.Add(2*time.Hour))
	merge := r.Merge("main", "feat/x", "Merge pull request #1 from bob/feat/x", alice, t0.Add(3*time.Hour))
	m2 := r.Commit("main", "a.txt", "c\n", "m2", alice, t0.Add(4*time.Hour))

	p, _, _ := newProvider(t, srv)
	ctx := t.Context()
	if _, err := p.FirstParent(ctx, "alice", "projet", "main", "", nil, time.Time{}, time.Time{}); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("without a clone: %v, want ErrNotFound", err)
	}
	if err := p.Prepare(ctx, "alice", "projet", "main"); err != nil {
		t.Fatal(err)
	}
	got, err := p.FirstParent(ctx, "alice", "projet", "main", "", nil, time.Time{}, time.Time{})
	if want := []string{m2, merge, m1}; err != nil || !slices.Equal(got, want) {
		t.Errorf("FirstParent(main) = %v, %v; want %v (no root, no f1)", got, err, want)
	}
	if slices.Contains(got, f1) || slices.Contains(got, root) {
		t.Error("merged branch commit or root listed")
	}
	got, err = p.FirstParent(ctx, "alice", "projet", "main", "", nil, t0.Add(150*time.Minute), time.Time{})
	if want := []string{m2, merge}; err != nil || !slices.Equal(got, want) {
		t.Errorf("since = %v, %v; want %v", got, err, want)
	}
	got, err = p.FirstParent(ctx, "alice", "projet", "main", "", nil, time.Time{}, t0.Add(210*time.Minute))
	if want := []string{merge, m1}; err != nil || !slices.Equal(got, want) {
		t.Errorf("until = %v, %v; want %v", got, err, want)
	}
	got, err = p.FirstParent(ctx, "alice", "projet", "", merge, []string{m1}, time.Time{}, time.Time{})
	if want := []string{merge}; err != nil || !slices.Equal(got, want) {
		t.Errorf("head merge ^m1 = %v, %v; want %v", got, err, want)
	}
	if _, err := p.FirstParent(ctx, "alice", "projet", "gone", "", nil, time.Time{}, time.Time{}); err == nil {
		t.Error("missing branch: no error")
	}
	missing := strings.Repeat("ab", 20)
	if _, err := p.FirstParent(ctx, "alice", "projet", "", missing, nil, time.Time{}, time.Time{}); err == nil {
		t.Error("missing head commit: no error")
	}
	if _, err := p.FirstParent(ctx, "alice", "projet", "", "", nil, time.Time{}, time.Time{}); err == nil {
		t.Error("neither branch nor head accepted")
	}
	if _, err := p.FirstParent(ctx, "alice", "projet", "main", "", []string{"zz"}, time.Time{}, time.Time{}); err == nil {
		t.Error("invalid exclude accepted")
	}
}
