package git_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

// CommitStatsBatch gives, in one pass, the same statistics as CommitStats: simple commit, root,
// merge (first parent), empty commit (0/0, present in the result), binary file not counted; a SHA
// missing from the clone is left out, duplicates are allowed.
func TestCommitStatsBatch(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	m0 := r.Commit("main", "a.txt", "a\nb\n", "m0", alice, t0)
	r.Branch("dev", m0)
	f1 := r.Commit("dev", "b.txt", "x\n", "f1", bob, t0.Add(time.Hour))
	m1 := r.Commit("main", "a.txt", "a\nc\n", "m1", alice, t0.Add(2*time.Hour))
	x := r.Merge("main", "dev", "merge", alice, t0.Add(3*time.Hour))
	empty := r.EmptyCommit("main", "vide", alice, t0.Add(4*time.Hour))
	bin := r.Commit("main", "img.bin", "\x00\x01\x02", "binaire", alice, t0.Add(5*time.Hour))
	unknown := strings.Repeat("ab", 20)

	p, _, logs := newProvider(t, srv)
	ctx := t.Context()
	got, err := p.CommitStatsBatch(ctx, "alice", "projet", []string{m0, f1, m1, x, empty, bin, unknown, m1})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]provider.CommitStats{
		m0:    {Additions: 2, Files: []provider.CommitFile{{Path: "a.txt", Additions: 2}}},
		f1:    {Additions: 1, Files: []provider.CommitFile{{Path: "b.txt", Additions: 1}}},
		m1:    {Additions: 1, Deletions: 1, Files: []provider.CommitFile{{Path: "a.txt", Additions: 1, Deletions: 1}}},
		x:     {Additions: 1, Files: []provider.CommitFile{{Path: "b.txt", Additions: 1}}}, // the merge brings b.txt to main
		empty: {Files: []provider.CommitFile{}},
		bin:   {Files: []provider.CommitFile{{Path: "img.bin", Binary: true}}},
	}
	if len(got) != len(want) {
		t.Errorf("CommitStatsBatch: %d results, want %d (%v)", len(got), len(want), got)
	}
	for sha, w := range want {
		st, ok := got[sha]
		if !ok || !sameStats(st, w) {
			t.Errorf("CommitStatsBatch[%s] = %+v (%v), want %+v", provider.ShortSHA(sha), st, ok, w)
		}
		if one, err := p.CommitStats(ctx, "alice", "projet", sha); err != nil || !sameStats(one, w) {
			t.Errorf("CommitStats(%s) = %+v, %v; want %+v (same rule)", provider.ShortSHA(sha), one, err, w)
		}
	}
	if _, ok := got[unknown]; ok {
		t.Error("a SHA missing from the clone is in the result")
	}
	if strings.Contains(logs.String(), "alice@example.test") || strings.Contains(logs.String(), "Alice Martin") {
		t.Errorf("personal data in the logs:\n%s", logs.String())
	}

	// Empty list: empty result, no error; invalid SHA refused before any git.
	if none, err := p.CommitStatsBatch(ctx, "alice", "projet", nil); err != nil || len(none) != 0 {
		t.Errorf("empty list: %v, %v", none, err)
	}
	if _, err := p.CommitStatsBatch(ctx, "alice", "projet", []string{"main"}); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("invalid SHA: %v, want ErrInvalidRef", err)
	}
}

// More than StatsBatchSize commits: several batches, all computed; canceled context: error.
func TestCommitStatsBatchManyCommits(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "gros", "main")
	r.Commit("main", "f.txt", "0\n", "c0", alice, t0)
	n := git.StatsBatchSize + 20
	r.ManyCommits("main", "f.txt", n, alice, t0.Add(time.Minute))
	p, dir, _ := newProvider(t, srv)
	commits, err := p.ListCommits(t.Context(), "alice", "gros", provider.ListCommitsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != n+1 {
		t.Fatalf("%d commits, want %d", len(commits), n+1)
	}
	got, err := p.CommitStatsBatch(t.Context(), "alice", "gros", shas(commits))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n+1 {
		t.Errorf("%d statistics, want %d", len(got), n+1)
	}
	for _, c := range commits[:n] { // every commit replaces one line
		if st := got[c.SHA]; st.Additions != 1 || st.Deletions != 1 || len(st.Files) != 1 {
			t.Fatalf("commit %s: %+v", provider.ShortSHA(c.SHA), st)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := fresh(t, srv, dir).CommitStatsBatch(ctx, "alice", "gros", shas(commits)); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled context: %v", err)
	}
}

// Prepare creates the clone and checks out the given default branch (the API's one), without
// ls-remote; the second time, nothing is redone.
func TestPrepareUsesGivenDefaultBranch(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	m0 := r.Commit("main", "a.txt", "main\n", "m0", alice, t0)
	r.Branch("dev", m0)
	r.Commit("dev", "a.txt", "dev\n", "d1", alice, t0.Add(time.Hour))

	p, dir, logs := newProvider(t, srv)
	if err := p.Prepare(t.Context(), "alice", "projet", "dev"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "projet", "a.txt")); got != "dev\n" {
		t.Errorf("working copy = %q, want branch dev (given)", got)
	}
	if strings.Contains(logs.String(), "commande=ls-remote") {
		t.Error("ls-remote run although the default branch was given")
	}
	if err := p.Prepare(t.Context(), "alice", "projet", "dev"); err != nil {
		t.Errorf("second Prepare: %v", err)
	}
	if err := p.Prepare(t.Context(), "alice", "projet", "a\x01b"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("invalid branch: %v", err)
	}
	// Missing repository: access denied, like the other commands.
	if err := fresh(t, srv, dir).Prepare(t.Context(), "alice", "absent", "main"); !errors.Is(err, provider.ErrAccessDenied) {
		t.Errorf("missing repository: %v", err)
	}
}

// sameStats compares two statistics, file list included (nil and empty lists differ: a clone always
// gives a non-nil list).
func sameStats(a, b provider.CommitStats) bool {
	return a.Additions == b.Additions && a.Deletions == b.Deletions && (a.Files == nil) == (b.Files == nil) &&
		slices.Equal(a.Files, b.Files)
}
