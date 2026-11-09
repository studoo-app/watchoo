package git_test

import (
	"context"
	"errors"
	"slices"
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
