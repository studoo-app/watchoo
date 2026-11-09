package git_test

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

// ParentCounts reads the number of parents in the existing clone: root (0), ordinary commit (1),
// merge (2); a SHA missing from the clone is left out, duplicates are allowed; without a clone,
// ErrNotFound; nothing is fetched.
func TestParentCounts(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	root := r.Commit("main", "a.txt", "a\n", "racine", alice, t0)
	r.Branch("dev", root)
	f1 := r.Commit("dev", "b.txt", "x\n", "f1", bob, t0.Add(time.Hour))
	m1 := r.Commit("main", "a.txt", "b\n", "m1", alice, t0.Add(2*time.Hour))
	merge := r.Merge("main", "dev", "merge", alice, t0.Add(3*time.Hour))
	unknown := strings.Repeat("ab", 20)

	p, dir, logs := newProvider(t, srv)
	ctx := t.Context()
	if _, err := p.ParentCounts(ctx, "alice", "projet", []string{root}); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("without a clone: %v, want ErrNotFound", err)
	}
	if err := p.Prepare(ctx, "alice", "projet", "main"); err != nil {
		t.Fatal(err)
	}
	got, err := p.ParentCounts(ctx, "alice", "projet", []string{root, f1, m1, merge, unknown, m1})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{root: 0, f1: 1, m1: 1, merge: 2}
	if !maps.Equal(got, want) {
		t.Errorf("ParentCounts = %v, want %v", got, want)
	}

	// The clone is read as is: a commit pushed after Prepare is unknown.
	later := r.Commit("main", "a.txt", "c\n", "later", alice, t0.Add(4*time.Hour))
	if got, err := fresh(t, srv, dir).ParentCounts(ctx, "alice", "projet", []string{later}); err != nil || len(got) != 0 {
		t.Errorf("commit not fetched: %v, %v", got, err)
	}
	if strings.Contains(logs.String(), "alice@example.test") || strings.Contains(logs.String(), "racine") {
		t.Errorf("personal data in the logs:\n%s", logs.String())
	}

	// Empty list, invalid SHA or repository, canceled context.
	if none, err := p.ParentCounts(ctx, "alice", "projet", nil); err != nil || len(none) != 0 {
		t.Errorf("empty list: %v, %v", none, err)
	}
	if _, err := p.ParentCounts(ctx, "alice", "projet", []string{"main"}); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("invalid SHA: %v, want ErrInvalidRef", err)
	}
	if _, err := p.ParentCounts(ctx, "alice", "../x", []string{root}); err == nil {
		t.Error("invalid repository accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.ParentCounts(canceled, "alice", "projet", []string{root}); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled context: %v", err)
	}
}
