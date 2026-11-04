package git_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

type addedLine struct {
	sha, path, line string
	hunk            bool
}

// AddedLines reads the added lines on the existing clone only, per commit and per file, without context.
func TestAddedLines(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	c0 := r.Commit("main", "a.go", "package a\n\nfunc A() {}\n", "m0", alice, t0)
	c1 := r.Commit("main", "a.go", "package a\n\n// B doc\nfunc B() {}\n\nfunc A() {}\n+++ x\n", "m1", alice, t0.Add(time.Hour))
	c2 := r.Commit("main", "d d/é.py", "x = 1\n", "m2", alice, t0.Add(2*time.Hour))
	c3 := r.Commit("main", "a.go", "package a\n", "m3", alice, t0.Add(3*time.Hour)) // deletions only
	p, dir, _ := newProvider(t, srv)
	ctx := t.Context()

	collect := func(shas ...string) ([]addedLine, error) {
		var got []addedLine
		err := p.AddedLines(ctx, "alice", "projet", shas, func(sha, path, line string, hunk bool) {
			got = append(got, addedLine{sha, path, line, hunk})
		})
		return got, err
	}
	// No clone yet: an error, and nothing is cloned.
	if _, err := collect(c0); err == nil {
		t.Fatal("AddedLines without a clone: no error")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("AddedLines created something in %s", dir)
	}
	if _, err := p.CommitStatsBatch(ctx, "alice", "projet", []string{c0}); err != nil { // creates the clone
		t.Fatal(err)
	}
	got, err := collect(c0, c1, c2, c3, c0, strings.Repeat("ab", 20))
	if err != nil {
		t.Fatal(err)
	}
	want := []addedLine{
		{c0, "a.go", "package a", true}, {c0, "a.go", "", false}, {c0, "a.go", "func A() {}", false},
		{c1, "a.go", "// B doc", true}, {c1, "a.go", "func B() {}", false}, {c1, "a.go", "", false},
		{c1, "a.go", "+++ x", true},
		{c2, "d d/é.py", "x = 1", true},
	}
	// diff-tree lists the commits in input order (sorted SHAs): compare per commit.
	for _, sha := range []string{c0, c1, c2, c3} {
		g := slices.DeleteFunc(slices.Clone(got), func(l addedLine) bool { return l.sha != sha })
		w := slices.DeleteFunc(slices.Clone(want), func(l addedLine) bool { return l.sha != sha })
		if !slices.Equal(g, w) {
			t.Errorf("added lines of %s:\n got %+v\nwant %+v", sha[:7], g, w)
		}
	}
}
