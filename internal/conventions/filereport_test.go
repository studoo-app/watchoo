package conventions

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// fileClones is a clone that can also read the files of a revision.
type fileClones struct {
	*fakeClones
	*fakeFiles
}

const testChangelog = "# Changelog\n\nAll notable changes are documented here. The format is based on " +
	"[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to " +
	"[Semantic Versioning](https://semver.org/spec/v2.0.0.html).\n\n## [Unreleased]\n\n" +
	"## [1.0.0] - 2026-10-01\n\n### Added\n\n- Première version.\n"

// fileFixture returns a reader and a clone of alice/projet: head h1 of main (documentation with an
// unreferenced document and a dead link), tag v1.0.0 on t1 (README without installation section).
func fileFixture(t *testing.T) (*fakeReader, *fileClones, BuildOptions) {
	t.Helper()
	head := goodDocs()
	head["README.md"] = fullReadme
	head["CHANGELOG.md"] = testChangelog
	head["documentation/03-annexes/oubli.md"] = "# Oubli\n\n[mort](absent.md)\n"
	tag := goodDocs()
	tag["README.md"] = "# Projet\n\n## Présentation\n\nTexte.\n\n[doc](documentation/INDEX.md) [log](CHANGELOG.md)\n"
	tag["CHANGELOG.md"] = testChangelog
	r := &fakeReader{
		repos:    []model.Repository{{ID: 1, Owner: "alice", Name: "projet", DefaultBranch: "main"}},
		branches: map[int64][]model.Branch{1: {{Name: "main", Tracked: true, HeadSHA: "h1"}}},
		tags:     map[int64][]model.Tag{1: {{Name: "v1.0.0", SHA: "t1"}}},
	}
	cl := &fileClones{
		fakeClones: &fakeClones{has: map[string]bool{"alice/projet": true}, reach: map[string][]string{"main": {}}},
		fakeFiles:  &fakeFiles{trees: map[string]map[string]string{"h1": head, "t1": tag}},
	}
	opts := reviewOptions(t)
	opts.Files = DefaultFileRules()
	return r, cl, opts
}

// cacheRow computes the file criteria of a revision of the fixture, as the sync does.
func cacheRow(t *testing.T, r *fakeReader, cl *fileClones, target FileTarget, opts BuildOptions) model.RepositoryConventions {
	t.Helper()
	tags := r.tags[1]
	tree, err := ReadFileTree(t.Context(), cl, "alice", "projet", target.SHA, opts.Files)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := CheckFiles(tree, FileOptionsFor(target, "alice/projet", tags, opts.Files,
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)))
	raw, err := EncodeFileResults(res)
	if err != nil {
		t.Fatal(err)
	}
	return model.RepositoryConventions{RepoID: 1, RevKind: target.Kind, TagName: target.Tag, RevSHA: target.SHA,
		RulesVersion: FileRulesVersion, ConfigHash: FileConfigHash(opts.Files, "alice/projet", tags), Results: raw,
		ComputedAt: time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)}
}

func familyStatuses(f FileReport) map[string]string {
	out := map[string]string{}
	for _, fam := range f.Results.Families() {
		for _, c := range fam.Checks {
			out[c.ID] = c.Status + ":" + c.Reason
		}
	}
	return out
}

func TestBuildFilesNotComputed(t *testing.T) {
	r, cl, opts := fileFixture(t)
	rep, err := Build(t.Context(), r, cl, Filter{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	f := rep.Repos[0].Files
	if f.Known || f.Reason != UnknownNotComputed || f.Lists != nil {
		t.Fatalf("not computed: %+v", f)
	}
	// Every criterion of the head is unknown, CL9 excluded (milestone trees only).
	st := familyStatuses(f)
	if len(st) != 21 || st["CL1"] != "unknown:not_computed" || st["TL3"] != "unknown:not_computed" || st["CL9"] != "" {
		t.Errorf("statuses: %v", st)
	}
	for _, fam := range f.Results.Families() {
		if c, u := fam.Tally(); c != 0 || u != len(fam.Checks) {
			t.Errorf("%s: %d/%d", fam.Name, c, u)
		}
	}
	// Without a clone: "no clone" rather than "not computed".
	rep, _ = Build(t.Context(), r, nil, Filter{}, opts)
	if f := rep.Repos[0].Files; f.Reason != UnknownNoClone {
		t.Errorf("no clone: %+v", f)
	}
}

func TestBuildFilesFromCache(t *testing.T) {
	r, cl, opts := fileFixture(t)
	r.files = map[int64][]model.RepositoryConventions{1: {cacheRow(t, r, cl, FileTarget{Kind: model.ConventionRevHead, SHA: "h1"}, opts)}}
	cl.fakeFiles.asked = nil
	rep, err := Build(t.Context(), r, cl, Filter{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	f := rep.Repos[0].Files
	if !f.Known || f.Reason != "" || f.SHA != "h1" || f.ComputedAt.IsZero() || f.Lists != nil {
		t.Fatalf("cached: %+v", f)
	}
	st := familyStatuses(f)
	if st["CL1"] != "conform:" || st["RD1"] != "conform:" || st["DOC3"] != "nonconform:" || st["DOC4"] != "nonconform:" {
		t.Errorf("statuses: %v", st)
	}
	// The overview reads no tree: only the cache.
	if len(cl.fakeFiles.asked) != 0 {
		t.Errorf("tree read without detail: %v", cl.fakeFiles.asked)
	}
	// Detail without a repository filter: ignored.
	opts.FileDetail = true
	rep, _ = Build(t.Context(), r, cl, Filter{}, opts)
	if rep.Repos[0].Files.Lists != nil {
		t.Error("lists computed for the overview")
	}

	// Detail of the repository: lists recomputed from the clone, at the cached revision.
	rep, err = Build(t.Context(), r, cl, Filter{RepoID: 1}, opts)
	if err != nil {
		t.Fatal(err)
	}
	l := rep.Repos[0].Files.Lists
	if l == nil || !slices.Equal(l.Unreferenced, []string{"documentation/03-annexes/oubli.md"}) || len(l.DeadLinks) != 1 ||
		l.DeadLinks[0].Target != "absent.md" {
		t.Fatalf("lists: %+v", l)
	}
	// Detail, clone unreadable: lists unknown, cached results kept.
	cl.fakeFiles.err = errors.New("boom")
	rep, err = Build(t.Context(), r, cl, Filter{RepoID: 1}, opts)
	if f := rep.Repos[0].Files; err != nil || !f.Known || f.Lists != nil || f.ListsReason != UnknownCloneUnreadable {
		t.Errorf("unreadable: %v %+v", err, f)
	}
	// Detail, clone that cannot read files: no clone.
	rep, _ = Build(t.Context(), r, cl.fakeClones, Filter{RepoID: 1}, opts)
	if f := rep.Repos[0].Files; !f.Known || f.ListsReason != UnknownNoClone {
		t.Errorf("no file reader: %+v", f)
	}
}

func TestBuildFilesStale(t *testing.T) {
	r, cl, opts := fileFixture(t)
	fresh := cacheRow(t, r, cl, FileTarget{Kind: model.ConventionRevHead, SHA: "h1"}, opts)
	for name, mutate := range map[string]func(*model.RepositoryConventions, *BuildOptions){
		"other head":    func(_ *model.RepositoryConventions, _ *BuildOptions) { r.branches[1][0].HeadSHA = "h2" },
		"rules version": func(row *model.RepositoryConventions, _ *BuildOptions) { row.RulesVersion = FileRulesVersion + 1 },
		"settings":      func(_ *model.RepositoryConventions, o *BuildOptions) { o.Files.ReadmeMinWords = 10 },
		"undecodable":   func(row *model.RepositoryConventions, _ *BuildOptions) { row.Results = "[" },
		"new tag": func(_ *model.RepositoryConventions, _ *BuildOptions) {
			r.tags[1] = append(r.tags[1], model.Tag{Name: "v1.1.0", SHA: "h1"})
		},
		"other encoding": func(row *model.RepositoryConventions, _ *BuildOptions) { row.Results = `{"rules_version":99}` },
	} {
		t.Run(name, func(t *testing.T) {
			r.branches[1][0].HeadSHA = "h1"
			r.tags[1] = []model.Tag{{Name: "v1.0.0", SHA: "t1"}}
			row, o := fresh, opts
			mutate(&row, &o)
			r.files = map[int64][]model.RepositoryConventions{1: {row}}
			rep, err := Build(t.Context(), r, cl, Filter{}, o)
			if err != nil {
				t.Fatal(err)
			}
			if f := rep.Repos[0].Files; f.Known || f.Reason != UnknownStale || familyStatuses(f)["RD1"] != "unknown:stale" {
				t.Errorf("%+v", f)
			}
		})
	}
	// Head unknown in the database (default branch not tracked; the sync measures the forge head and
	// deletes the row when the forge has no default branch): only the rules version and the fingerprint count.
	r.branches[1][0].HeadSHA = ""
	r.tags[1] = []model.Tag{{Name: "v1.0.0", SHA: "t1"}}
	r.files = map[int64][]model.RepositoryConventions{1: {fresh}}
	rep, _ := Build(t.Context(), r, cl, Filter{}, opts)
	if !rep.Repos[0].Files.Known {
		t.Errorf("unknown head: %+v", rep.Repos[0].Files)
	}
}

func TestBuildFilesFilteredAndErrors(t *testing.T) {
	r, cl, opts := fileFixture(t)
	r.files = map[int64][]model.RepositoryConventions{1: {cacheRow(t, r, cl, FileTarget{Kind: model.ConventionRevHead, SHA: "h1"}, opts)}}
	// Repository indicators are never attributed to a person.
	rep, err := Build(t.Context(), r, cl, Filter{AuthorIDs: []int64{10}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if f := rep.Repos[0].Files; f.Known || f.Reason != UnknownFiltered || familyStatuses(f)["DOC3"] != "unknown:filtered" {
		t.Errorf("filtered: %+v", f)
	}
	r.errFiles = errors.New("db")
	if _, err := Build(t.Context(), r, cl, Filter{}, opts); err == nil {
		t.Error("database error ignored")
	}
}

func TestReviewMilestoneFiles(t *testing.T) {
	r, cl, opts := fileFixture(t)
	after := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	tagRow := cacheRow(t, r, cl, FileTarget{Kind: model.ConventionRevTag, Tag: "v1.0.0", SHA: "t1"}, opts)
	headRow := cacheRow(t, r, cl, FileTarget{Kind: model.ConventionRevHead, SHA: "h1"}, opts)
	r.files = map[int64][]model.RepositoryConventions{1: {headRow, tagRow}}
	cl.fakeFiles.asked = nil

	rv, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if err != nil {
		t.Fatal(err)
	}
	p := rv.Repos[0]
	got := statuses(p.Criteria)[6:]
	// Tree of the tag: CHANGELOG section present, README without installation section, INDEX exhaustive.
	want := []string{"CL9:conform:", "CL8:conform:", "RD1:conform:", "RD2:nonconform:", "RD3:conform:",
		"DOC1:conform:", "DOC3:conform:"}
	if !slices.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	if f := p.Files; f == nil || !f.Known || f.SHA != "t1" || !slices.Equal(f.ReadmeFound, []string{SectionPresentation}) ||
		len(f.ReadmeExpected) != len(AllReadmeSections()) {
		t.Errorf("milestone files: %+v", p.Files)
	}
	if len(cl.fakeFiles.asked) != 0 {
		t.Errorf("tree read by the review: %v", cl.fakeFiles.asked)
	}

	// Tag moved since the computation: to recompute.
	r.tags[1] = []model.Tag{{Name: "v1.0.0", SHA: "t2"}}
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if got := statuses(rv.Repos[0].Criteria)[6:]; !slices.Equal(got, fileTail(UnknownStale)) {
		t.Errorf("moved tag: %v", got)
	}
	// Not computed yet.
	r.tags[1] = []model.Tag{{Name: "v1.0.0", SHA: "t1"}}
	r.files = map[int64][]model.RepositoryConventions{1: {headRow}}
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if got := statuses(rv.Repos[0].Criteria)[6:]; !slices.Equal(got, fileTail(UnknownNotComputed)) ||
		rv.Repos[0].Files.Reason != UnknownNotComputed {
		t.Errorf("not computed: %v", got)
	}
	// README with installation on the tag tree: RD2 conform in the review.
	tag := cl.trees["t1"]
	tag["README.md"] = fullReadme
	r.files = map[int64][]model.RepositoryConventions{1: {cacheRow(t, r, cl,
		FileTarget{Kind: model.ConventionRevTag, Tag: "v1.0.0", SHA: "t1"}, opts)}}
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if c := rv.Repos[0].Criteria[9]; c.ID != CriterionRD2 || c.Status != CriterionConform {
		t.Errorf("RD2 install: %+v", c)
	}
	r.errFiles = errors.New("db")
	if _, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after); err == nil {
		t.Error("database error ignored")
	}
}
