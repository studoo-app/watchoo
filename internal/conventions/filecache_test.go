package conventions

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// fakeFiles is an in-memory FileReader: one tree per revision.
type fakeFiles struct {
	trees     map[string]map[string]string // rev → path → content
	truncated bool
	large     map[string]bool
	err       error
	asked     []string // paths asked to ReadBlobs
}

func (f *fakeFiles) TreePaths(_ context.Context, _, _, rev string) ([]string, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	var out []string
	for p := range f.trees[rev] {
		out = append(out, p)
	}
	slices.Sort(out)
	return out, f.truncated, nil
}

func (f *fakeFiles) ReadBlobs(_ context.Context, _, _, rev string, paths []string,
	fn func(path string, data []byte, tooLarge bool)) error {
	f.asked = append(f.asked, paths...)
	for _, p := range paths {
		if f.large[p] {
			fn(p, nil, true)
		} else if c, ok := f.trees[rev][p]; ok {
			fn(p, []byte(c), false)
		}
	}
	return nil
}

func TestReadFileTree(t *testing.T) {
	ctx := t.Context()
	f := &fakeFiles{trees: map[string]map[string]string{"r": {
		"CHANGELOG.md": "# Changelog\n", "README.md": "# Projet\n", "main.go": "package main\n",
		"documentation/INDEX.md": "# Index\n", "documentation/fr/01-vision.md": "# Vision\n",
	}}, large: map[string]bool{"README.md": true}}
	tree, err := ReadFileTree(ctx, f, "o", "n", "r", DefaultFileRules())
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Paths) != 5 || tree.Truncated {
		t.Errorf("paths = %v, truncated = %v", tree.Paths, tree.Truncated)
	}
	if slices.Contains(f.asked, "main.go") {
		t.Errorf("a file the criteria do not read was asked: %v", f.asked)
	}
	if string(tree.Blobs["CHANGELOG.md"].Data) != "# Changelog\n" || !tree.Blobs["README.md"].TooLarge ||
		string(tree.Blobs["documentation/fr/01-vision.md"].Data) != "# Vision\n" {
		t.Errorf("blobs = %+v", tree.Blobs)
	}

	f.truncated, f.asked = true, nil
	if tree, err := ReadFileTree(ctx, f, "o", "n", "r", DefaultFileRules()); err != nil || !tree.Truncated || len(f.asked) != 0 {
		t.Errorf("truncated tree: %+v, %v, asked %v", tree, err, f.asked)
	}
	f.err = errors.New("boom")
	if _, err := ReadFileTree(ctx, f, "o", "n", "r", DefaultFileRules()); err == nil {
		t.Error("error not returned")
	}
}

func TestFileTargets(t *testing.T) {
	ms, err := ParseMilestones([]string{"v1.0.0", "v2.0.0@2026-12-01", "v3.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	tags := []model.Tag{{Name: "v2.0.0", SHA: "b"}, {Name: "1.0.0", SHA: "a"}, {Name: "v0.1.0", SHA: "z"},
		{Name: "v3.0.0", SHA: ""}}
	got := FileTargets("h", tags, ms)
	want := []FileTarget{{Kind: model.ConventionRevHead, SHA: "h"}, {Kind: model.ConventionRevTag, Tag: "1.0.0", SHA: "a"},
		{Kind: model.ConventionRevTag, Tag: "v2.0.0", SHA: "b"}}
	if !slices.Equal(got, want) {
		t.Errorf("FileTargets = %+v, want %+v", got, want)
	}
	if got := FileTargets("", nil, ms); len(got) != 0 {
		t.Errorf("no head, no tag: %+v", got)
	}
	opts := FileOptionsFor(want[2], "o/n", tags, DefaultFileRules(), time.Time{})
	if opts.Version != "v2.0.0" || opts.Repo != "o/n" || len(opts.Tags) != 4 {
		t.Errorf("FileOptionsFor = %+v", opts)
	}
}

func TestFileCacheEncodingAndFreshness(t *testing.T) {
	tree := FileTree{Paths: []string{"CHANGELOG.md", "README.md"}, Blobs: map[string]Blob{
		"CHANGELOG.md": {Data: []byte("# Changelog\n\n## [Unreleased]\n")}, "README.md": {Data: []byte("# Projet secret Alice\n")}}}
	res, _ := CheckFiles(tree, FileOptions{Rules: DefaultFileRules(), Repo: "o/n"})
	raw, err := EncodeFileResults(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"CHANGELOG.md", "README", "secret", "Alice"} {
		if strings.Contains(raw, leak) {
			t.Errorf("encoded results contain %q: %s", leak, raw)
		}
	}
	back, err := DecodeFileResults(raw)
	if err != nil || back.RulesVersion != FileRulesVersion || len(back.Changelog) != len(res.Changelog) {
		t.Errorf("DecodeFileResults = %+v, %v", back, err)
	}
	if _, err := DecodeFileResults("{"); err == nil {
		t.Error("invalid JSON accepted")
	}

	rules := DefaultFileRules()
	tags := []model.Tag{{Name: "v1.0.0"}}
	hash := FileConfigHash(rules, "alice/projet", tags)
	row := model.RepositoryConventions{RevSHA: "s", RulesVersion: FileRulesVersion, ConfigHash: hash}
	if !FileCacheFresh(row, "s", hash) {
		t.Error("fresh row reported stale")
	}
	rules.ReadmeMinWords = 10
	for name, fresh := range map[string]bool{
		"other commit": FileCacheFresh(row, "t", hash),
		"other rules":  FileCacheFresh(row, "s", FileConfigHash(rules, "alice/projet", tags)),
		"new tag":      FileCacheFresh(row, "s", FileConfigHash(DefaultFileRules(), "alice/projet", append(tags, model.Tag{Name: "v1.1.0"}))),
		"renamed":      FileCacheFresh(row, "s", FileConfigHash(DefaultFileRules(), "alice/nouveau", tags)),
		"old version":  FileCacheFresh(model.RepositoryConventions{RevSHA: "s", RulesVersion: FileRulesVersion - 1, ConfigHash: hash}, "s", hash),
	} {
		if fresh {
			t.Errorf("%s: reported fresh", name)
		}
	}
	if FileConfigHash(DefaultFileRules(), "alice/projet", append(tags, model.Tag{Name: "pas-une-version"})) != hash {
		t.Error("a tag that designates no version changes the hash")
	}
	if FileConfigHash(DefaultFileRules(), "Alice/Projet", tags) != hash {
		t.Error("the case of the repository name changes the hash")
	}
}
