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

// LastCommit, CountCommits and FirstAdd read the history of the existing clone for the leads of phase 8.7:
// paths taken literally (pathspec magic in a path is not interpreted), exclusions, directories, the oldest
// addition of a file deleted then added again; head missing or no clone:
// ErrNotFound; neither path nor SHA in the errors or logs; GIT_LITERAL_PATHSPECS inherited is ignored.
func TestLeadHistory(t *testing.T) {
	t.Setenv("GIT_LITERAL_PATHSPECS", "1")
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	c1 := r.Commit("main", "documentation/INDEX.md", "# Index\n", "docs: index", alice, t0)
	c2 := r.Commit("main", "src/main.go", "package main\n", "feat: main", alice, t0.Add(time.Hour))
	c3 := r.Commit("main", "CHANGELOG.md", "# Changelog\n", "docs: changelog", alice, t0.Add(2*time.Hour))
	c4 := r.Commit("main", "src/main.go", "package main\n\n", "fix: main", alice, t0.Add(3*time.Hour))
	c5 := r.Commit("main", ":(glob)*", "magic\n", "chore: magic", alice, t0.Add(4*time.Hour))
	added := r.Commit("main", "commitlint.config.js", "module.exports = {}\n", "chore: commitlint", alice, t0.Add(5*time.Hour))
	r.RemoveFile("main", "commitlint.config.js", "chore: remove commitlint", alice, t0.Add(6*time.Hour))
	r.Commit("main", "commitlint.config.js", "module.exports = {x: 1}\n", "chore: commitlint again", alice,
		t0.Add(7*time.Hour))
	head := r.Commit("main", "commitlint.config.js", "module.exports = {x: 2}\n", "chore: commitlint edit", alice,
		t0.Add(8*time.Hour))

	p, _, logs := newProvider(t, srv)
	ctx := t.Context()
	if _, _, _, err := p.LastCommit(ctx, "alice", "projet", head, nil, nil); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("LastCommit without a clone: %v, want ErrNotFound", err)
	}
	if _, err := p.CountCommits(ctx, "alice", "projet", head, c1, nil, nil); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("CountCommits without a clone: %v, want ErrNotFound", err)
	}
	if _, _, _, err := p.FirstAdd(ctx, "alice", "projet", head, "CHANGELOG.md"); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("FirstAdd without a clone: %v, want ErrNotFound", err)
	}
	if err := p.Prepare(ctx, "alice", "projet", "main"); err != nil {
		t.Fatal(err)
	}

	docs := []string{"documentation", "CHANGELOG.md"}
	for _, tc := range []struct {
		name             string
		include, exclude []string
		want             string
		at               time.Time
	}{
		{"directory", []string{"documentation"}, nil, c1, t0},
		{"file", []string{"CHANGELOG.md"}, nil, c3, t0.Add(2 * time.Hour)},
		{"several", docs, nil, c3, t0.Add(2 * time.Hour)},
		{"literal magic", []string{":(glob)*"}, nil, c5, t0.Add(4 * time.Hour)},
		{"code", nil, append(docs, "commitlint.config.js", ":(glob)*"), c4, t0.Add(3 * time.Hour)},
		{"all", nil, nil, head, t0.Add(8 * time.Hour)},
	} {
		got, at, found, err := p.LastCommit(ctx, "alice", "projet", head, tc.include, tc.exclude)
		if err != nil || !found || got != tc.want || !at.Equal(tc.at) || at.Location() != time.UTC {
			t.Errorf("%s: LastCommit = %q, %v, %v, %v; want %q at %v", tc.name, got, at, found, err, tc.want, tc.at)
		}
	}
	if got, _, found, err := p.LastCommit(ctx, "alice", "projet", head, []string{"README.md"}, nil); err != nil || found ||
		got != "" {
		t.Errorf("never touched: %q, %v, %v", got, found, err)
	}
	if got, _, found, err := p.LastCommit(ctx, "alice", "projet", c2, []string{"CHANGELOG.md"}, nil); err != nil || found ||
		got != "" {
		t.Errorf("before its commit: %q, %v, %v", got, found, err)
	}

	// Commits outside the documentation since its last commit (c3): c4, c5 and the four commitlint
	// commits; the documentation excluded, c3 itself never counted.
	if n, err := p.CountCommits(ctx, "alice", "projet", head, c3, nil, docs); err != nil || n != 6 {
		t.Errorf("CountCommits = %d, %v; want 6", n, err)
	}
	if n, err := p.CountCommits(ctx, "alice", "projet", head, c1, []string{"src"}, nil); err != nil || n != 2 {
		t.Errorf("CountCommits on src = %d, %v; want 2", n, err)
	}
	if n, err := p.CountCommits(ctx, "alice", "projet", head, head, nil, nil); err != nil || n != 0 {
		t.Errorf("CountCommits from head = %d, %v; want 0", n, err)
	}

	// The oldest addition of a file deleted then added again.
	sha, at, found, err := p.FirstAdd(ctx, "alice", "projet", head, "commitlint.config.js")
	if err != nil || !found || sha != added || !at.Equal(t0.Add(5*time.Hour)) {
		t.Errorf("FirstAdd = %q, %v, %v, %v; want %q", sha, at, found, err, added)
	}
	if sha, _, found, err := p.FirstAdd(ctx, "alice", "projet", head, "CHANGELOG.md"); err != nil || !found || sha != c3 {
		t.Errorf("FirstAdd CHANGELOG = %q, %v, %v; want %q", sha, found, err, c3)
	}
	if sha, _, found, err := p.FirstAdd(ctx, "alice", "projet", head, ":(glob)*"); err != nil || !found || sha != c5 {
		t.Errorf("FirstAdd literal = %q, %v, %v; want %q", sha, found, err, c5)
	}
	if sha, _, found, err := p.FirstAdd(ctx, "alice", "projet", c2, "CHANGELOG.md"); err != nil || found || sha != "" {
		t.Errorf("FirstAdd before = %q, %v, %v", sha, found, err)
	}

	// Errors: unknown head or since, invalid SHA or path, canceled context; neither path nor SHA in them.
	unknown := strings.Repeat("cd", 20)
	if _, _, _, err := p.LastCommit(ctx, "alice", "projet", unknown, []string{"CHANGELOG.md"}, nil); !errors.Is(err,
		provider.ErrNotFound) || strings.Contains(err.Error(), "CHANGELOG") {
		t.Errorf("unknown head: %v", err)
	}
	if _, err := p.CountCommits(ctx, "alice", "projet", head, unknown, nil, nil); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("unknown since: %v", err)
	}
	if _, _, _, err := p.FirstAdd(ctx, "alice", "projet", unknown, "CHANGELOG.md"); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("FirstAdd unknown head: %v", err)
	}
	if _, _, _, err := p.LastCommit(ctx, "alice", "projet", "main", nil, nil); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("invalid head: %v", err)
	}
	if _, _, _, err := p.LastCommit(ctx, "alice", "projet", head, []string{"a\nb"}, nil); !errors.Is(err,
		provider.ErrInvalidRef) {
		t.Errorf("invalid path: %v", err)
	}
	if _, _, _, err := p.FirstAdd(ctx, "alice", "projet", head, ""); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("empty path: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, _, err := p.LastCommit(canceled, "alice", "projet", head, nil, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled context: %v", err)
	}
	for _, s := range []string{"CHANGELOG", "documentation", "commitlint", head, c3} {
		if strings.Contains(logs.String(), s) {
			t.Errorf("%q in the logs:\n%s", s, logs.String())
		}
	}
}

// FirstAdd: an addition merged from a branch counts.
func TestFirstAddMerged(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	r.Commit("main", "a.txt", "a\n", "c1", alice, t0)
	r.Branch("dev", r.Head("main"))
	added := r.Commit("dev", ".husky/commit-msg", "npx commitlint --edit\n", "chore: husky", bob, t0.Add(time.Hour))
	r.Commit("main", "b.txt", "b\n", "c2", alice, t0.Add(2*time.Hour))
	head := r.Merge("main", "dev", "Merge dev", alice, t0.Add(3*time.Hour))

	p, _, _ := newProvider(t, srv)
	ctx := t.Context()
	if err := p.Prepare(ctx, "alice", "projet", "main"); err != nil {
		t.Fatal(err)
	}
	if sha, _, found, err := p.FirstAdd(ctx, "alice", "projet", head, ".husky/commit-msg"); err != nil || !found ||
		sha != added {
		t.Errorf("merged addition = %q, %v, %v; want %q", sha, found, err, added)
	}
}
