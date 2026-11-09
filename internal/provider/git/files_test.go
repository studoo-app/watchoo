package git_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

// readAll collects the blobs read by ReadBlobs.
func readAll(t *testing.T, p *git.Provider, rev string, paths []string) (map[string]string, []string, error) {
	t.Helper()
	got := map[string]string{}
	var tooLarge []string
	err := p.ReadBlobs(t.Context(), "alice", "projet", rev, paths, func(path string, data []byte, large bool) {
		if large {
			tooLarge = append(tooLarge, path)
			return
		}
		got[path] = string(data)
	})
	return got, tooLarge, err
}

// TreePaths and ReadBlobs read the tree of a commit (head or tag) through the Git objects of the existing
// clone, without fetching; a blob over the cap is reported too large; a missing path gives nothing.
func TestTreePathsAndReadBlobs(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	v1 := r.Commit("main", "CHANGELOG.md", "# Changelog\n", "c1", alice, t0)
	r.Tag("v1.0.0;$(id)", v1, "jalon")
	r.Commit("main", "documentation/fr/01-vision.md", "# Vision\n", "c2", alice, t0.Add(time.Hour))
	r.Commit("main", "big.txt", strings.Repeat("x", git.MaxBlobBytes+1), "c3", alice, t0.Add(2*time.Hour))
	head := r.Commit("main", "CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n", "c4", alice, t0.Add(3*time.Hour))

	p, dir, logs := newProvider(t, srv)
	ctx := t.Context()
	if _, _, err := p.TreePaths(ctx, "alice", "projet", head); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("without a clone: %v, want ErrNotFound", err)
	}
	if err := p.Prepare(ctx, "alice", "projet", "main"); err != nil {
		t.Fatal(err)
	}
	paths, truncated, err := p.TreePaths(ctx, "alice", "projet", head)
	if err != nil || truncated {
		t.Fatalf("TreePaths(head) = %v, %v, %v", paths, truncated, err)
	}
	slices.Sort(paths)
	if want := []string{"CHANGELOG.md", "big.txt", "documentation/fr/01-vision.md"}; !slices.Equal(paths, want) {
		t.Errorf("TreePaths(head) = %v, want %v", paths, want)
	}
	if paths, _, err := p.TreePaths(ctx, "alice", "projet", v1); err != nil || !slices.Equal(paths, []string{"CHANGELOG.md"}) {
		t.Errorf("TreePaths(tag) = %v, %v", paths, err)
	}

	got, large, err := readAll(t, p, head, []string{"CHANGELOG.md", "big.txt", "absent.md", "CHANGELOG.md",
		"documentation/fr/01-vision.md"})
	if err != nil {
		t.Fatal(err)
	}
	if got["CHANGELOG.md"] != "# Changelog\n\n## [Unreleased]\n" || got["documentation/fr/01-vision.md"] != "# Vision\n" || len(got) != 2 {
		t.Errorf("ReadBlobs(head) = %q", got)
	}
	if !slices.Equal(large, []string{"big.txt"}) {
		t.Errorf("too large = %v, want [big.txt]", large)
	}
	if got, _, err := readAll(t, p, v1, []string{"CHANGELOG.md"}); err != nil || got["CHANGELOG.md"] != "# Changelog\n" {
		t.Errorf("ReadBlobs(tag) = %q, %v", got, err)
	}
	if got, _, err := readAll(t, p, head, nil); err != nil || len(got) != 0 {
		t.Errorf("no path: %v, %v", got, err)
	}

	// The clone is read as is: a commit pushed afterwards is not in it.
	later := r.Commit("main", "README.md", "x\n", "later", alice, t0.Add(4*time.Hour))
	if _, _, err := fresh(t, srv, dir).TreePaths(ctx, "alice", "projet", later); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("commit not fetched: %v, want ErrNotFound", err)
	}
	if _, _, err := readAll(t, fresh(t, srv, dir), later, []string{"README.md"}); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("ReadBlobs of an absent commit: %v, want ErrNotFound", err)
	}

	for _, bad := range []string{"", "main", "HEAD", "--output=/tmp/x", strings.ToUpper(head)} {
		if _, _, err := p.TreePaths(ctx, "alice", "projet", bad); !errors.Is(err, provider.ErrInvalidRef) {
			t.Errorf("TreePaths(%q): %v, want ErrInvalidRef", bad, err)
		}
	}
	for _, bad := range []string{"", "a\nb", "a\x00b", "\xff"} {
		_, _, err := readAll(t, p, head, []string{bad})
		if !errors.Is(err, provider.ErrInvalidRef) {
			t.Errorf("ReadBlobs(%q): %v, want ErrInvalidRef", bad, err)
		} else if bad != "" && strings.Contains(err.Error(), bad) {
			t.Errorf("the error quotes the path: %v", err)
		}
	}
	if _, _, err := p.TreePaths(ctx, "alice", "../x", head); err == nil {
		t.Error("invalid repository accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := p.TreePaths(canceled, "alice", "projet", head); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled context: %v", err)
	}
	for _, leak := range []string{"CHANGELOG", "documentation", "big.txt", "v1.0.0", "$(id)"} {
		if strings.Contains(logs.String(), leak) {
			t.Errorf("logs contain %q", leak)
		}
	}
}
