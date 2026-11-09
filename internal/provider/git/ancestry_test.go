package git_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

// IsAncestor reads the ancestry in the existing clone: ancestor, same commit, not an ancestor (branch
// rewritten, sibling branch); a commit missing from the clone gives ErrNotFound (unknown, never "not an
// ancestor"); without a clone, ErrNotFound; nothing is fetched.
func TestIsAncestor(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	c1 := r.Commit("main", "a.txt", "a\n", "c1", alice, t0)
	c2 := r.Commit("main", "a.txt", "b\n", "c2", alice, t0.Add(time.Hour))
	r.Branch("dev", c1)
	d1 := r.Commit("dev", "b.txt", "x\n", "d1", bob, t0.Add(2*time.Hour))

	p, dir, logs := newProvider(t, srv)
	ctx := t.Context()
	if _, err := p.IsAncestor(ctx, "alice", "projet", c1, c2); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("without a clone: %v, want ErrNotFound", err)
	}
	if err := p.Prepare(ctx, "alice", "projet", "main"); err != nil {
		t.Fatal(err)
	}
	// main rewritten after Prepare: the old head stays in the clone.
	r.Reset("main", c1)
	c3 := r.Commit("main", "a.txt", "c\n", "c3", alice, t0.Add(3*time.Hour))
	p = fresh(t, srv, dir) // next sync: new fetch
	if err := p.Prepare(ctx, "alice", "projet", "main"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		anc, desc string
		want      bool
	}{
		{"ancestor", c1, c2, true},
		{"same commit", c2, c2, true},
		{"descendant", c2, c1, false},
		{"sibling branch", d1, c2, false},
		{"rewritten head", c2, c3, false},
		{"after rewrite", c1, c3, true},
	} {
		got, err := p.IsAncestor(ctx, "alice", "projet", tc.anc, tc.desc)
		if err != nil || got != tc.want {
			t.Errorf("%s: IsAncestor = %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}

	// A commit missing from the clone (pushed after the last fetch, or unknown): unknown.
	later := r.Commit("main", "a.txt", "d\n", "later", alice, t0.Add(4*time.Hour))
	unknown := strings.Repeat("ab", 20)
	for _, sha := range []string{later, unknown} {
		if _, err := fresh(t, srv, dir).IsAncestor(ctx, "alice", "projet", c1, sha); !errors.Is(err, provider.ErrNotFound) {
			t.Errorf("missing descendant: %v, want ErrNotFound", err)
		}
		if _, err := fresh(t, srv, dir).IsAncestor(ctx, "alice", "projet", sha, c1); !errors.Is(err, provider.ErrNotFound) {
			t.Errorf("missing ancestor: %v, want ErrNotFound", err)
		}
	}
	if strings.Contains(logs.String(), c1) || strings.Contains(logs.String(), "alice@example.test") {
		t.Errorf("SHA or personal data in the logs:\n%s", logs.String())
	}

	// Invalid SHA or repository, canceled context.
	if _, err := p.IsAncestor(ctx, "alice", "projet", "main", c1); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("invalid SHA: %v, want ErrInvalidRef", err)
	}
	if _, err := p.IsAncestor(ctx, "alice", "projet", c1, "--all"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("option as SHA: %v, want ErrInvalidRef", err)
	}
	if _, err := p.IsAncestor(ctx, "alice", "../x", c1, c2); err == nil {
		t.Error("invalid repository accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.IsAncestor(canceled, "alice", "projet", c1, c2); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled context: %v", err)
	}
}

// LastTouch returns the last commit of the history of head that changed a file, found false when none
// did or when the file is missing from head (deleted); head missing from the clone, or no clone: ErrNotFound; the path never appears in the errors.
func TestLastTouch(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	r.Commit("main", "a.txt", "a\n", "c1", alice, t0)
	cl := r.Commit("main", "CHANGELOG.md", "# Changelog\n", "docs: changelog", alice, t0.Add(time.Hour))
	head := r.Commit("main", "a.txt", "b\n", "c3", alice, t0.Add(2*time.Hour))
	r.Commit("main", ":(glob)*", "magic\n", "c4", alice, t0.Add(3*time.Hour))
	last := r.Head("main")
	gone := r.RemoveFile("main", "CHANGELOG.md", "chore: remove changelog", alice, t0.Add(4*time.Hour))

	p, _, logs := newProvider(t, srv)
	ctx := t.Context()
	if _, _, err := p.LastTouch(ctx, "alice", "projet", head, "CHANGELOG.md"); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("without a clone: %v, want ErrNotFound", err)
	}
	if err := p.Prepare(ctx, "alice", "projet", "main"); err != nil {
		t.Fatal(err)
	}
	if got, found, err := p.LastTouch(ctx, "alice", "projet", head, "CHANGELOG.md"); err != nil || !found || got != cl {
		t.Errorf("LastTouch = %q, %v, %v; want %q", got, found, err, cl)
	}
	if got, found, err := p.LastTouch(ctx, "alice", "projet", head, "README.md"); err != nil || found || got != "" {
		t.Errorf("file never touched = %q, %v, %v", got, found, err)
	}
	// The path is literal: pathspec magic is not interpreted.
	if got, found, err := p.LastTouch(ctx, "alice", "projet", last, ":(glob)*"); err != nil || !found || got != last {
		t.Errorf("literal path = %q, %v, %v; want %q", got, found, err, last)
	}
	if got, found, err := p.LastTouch(ctx, "alice", "projet", head, ":(glob)*"); err != nil || found {
		t.Errorf("literal path before its commit = %q, %v, %v", got, found, err)
	}
	// A file deleted from head has no last change: its deletion does not count.
	if got, found, err := p.LastTouch(ctx, "alice", "projet", gone, "CHANGELOG.md"); err != nil || found || got != "" {
		t.Errorf("deleted file = %q, %v, %v", got, found, err)
	}
	if got, found, err := p.LastTouch(ctx, "alice", "projet", gone, "a.txt"); err != nil || !found || got != head {
		t.Errorf("file still present = %q, %v, %v; want %q", got, found, err, head)
	}
	unknown := strings.Repeat("cd", 20)
	_, _, err := p.LastTouch(ctx, "alice", "projet", unknown, "CHANGELOG.md")
	if !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("unknown head: %v, want ErrNotFound", err)
	}
	if err != nil && strings.Contains(err.Error(), "CHANGELOG") {
		t.Errorf("path in the error: %v", err)
	}
	if strings.Contains(logs.String(), "CHANGELOG") {
		t.Errorf("path in the logs:\n%s", logs.String())
	}
	if _, _, err := p.LastTouch(ctx, "alice", "projet", "HEAD", "CHANGELOG.md"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("invalid head: %v, want ErrInvalidRef", err)
	}
	if _, _, err := p.LastTouch(ctx, "alice", "projet", head, ""); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("empty path: %v, want ErrInvalidRef", err)
	}
}
