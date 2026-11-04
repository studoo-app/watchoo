package git_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

// CommitPatch reads the diff on the existing clone only (no clone, no fetch), cut beyond maxBytes.
func TestCommitPatch(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	c0 := r.Commit("main", "a.txt", "un\n", "m0", alice, t0)
	c1 := r.Commit("main", "a.txt", "un\ndeux\ntrois\n", "m1", alice, t0.Add(time.Hour))
	p, dir, _ := newProvider(t, srv)
	ctx := t.Context()

	// No clone yet: not found, and nothing is cloned.
	if _, _, found, err := p.CommitPatch(ctx, "alice", "projet", c1, 0); err != nil || found {
		t.Fatalf("without clone: found=%v, %v", found, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("CommitPatch created something in %s", dir)
	}
	if _, err := p.CommitStatsBatch(ctx, "alice", "projet", []string{c0}); err != nil { // creates the clone
		t.Fatal(err)
	}
	patch, trunc, found, err := p.CommitPatch(ctx, "alice", "projet", c1, 0)
	if err != nil || !found || trunc || !strings.Contains(patch, "+deux") || !strings.Contains(patch, "a/a.txt") {
		t.Errorf("patch: %q, %v, %v, %v", patch, trunc, found, err)
	}
	root, _, found, _ := p.CommitPatch(ctx, "alice", "projet", c0, 0)
	if !found || !strings.Contains(root, "+un") {
		t.Errorf("root commit patch: %q", root)
	}
	short, trunc, _, _ := p.CommitPatch(ctx, "alice", "projet", c1, 40)
	if !trunc || len(short) > 40 || (short != "" && !strings.HasSuffix(short, "\n")) {
		t.Errorf("truncated patch: %q, %v", short, trunc)
	}
	if _, _, found, err := p.CommitPatch(ctx, "alice", "projet", strings.Repeat("ab", 20), 0); err != nil || found {
		t.Errorf("unknown commit: found=%v, %v", found, err)
	}
}
