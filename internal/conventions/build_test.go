package conventions

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// fakeReader is an in-memory Reader.
type fakeReader struct {
	repos       []model.Repository
	branches    map[int64][]model.Branch
	commits     map[int64][]model.ConventionCommit
	pulls       map[int64][]model.ConventionPullRequest
	tags        map[int64][]model.Tag
	releases    map[int64][]model.Release
	files       map[int64][]model.RepositoryConventions
	obs         map[int64][]model.TagObservation
	rewrites    map[int64][]model.BranchRewrite
	tagDates    map[int64]map[string]time.Time
	errHistory  error
	errRepos    error
	errBranches error
	errCommits  error
	errPulls    error
	errTags     error
	errReleases error
	errFiles    error
	calls       []int64 // authorID of each ConventionCommits call
}

func (f *fakeReader) ConventionPullRequests(_ context.Context, id int64) ([]model.ConventionPullRequest, error) {
	return f.pulls[id], f.errPulls
}

func (f *fakeReader) ConventionTags(_ context.Context, id int64) ([]model.Tag, error) {
	return f.tags[id], f.errTags
}

func (f *fakeReader) ConventionReleases(_ context.Context, id int64) ([]model.Release, error) {
	return f.releases[id], f.errReleases
}

func (f *fakeReader) RepositoryConventions(_ context.Context, id int64) ([]model.RepositoryConventions, error) {
	return f.files[id], f.errFiles
}

func (f *fakeReader) TagObservations(_ context.Context, id int64) ([]model.TagObservation, error) {
	return f.obs[id], f.errHistory
}

func (f *fakeReader) BranchRewrites(_ context.Context, id int64) ([]model.BranchRewrite, error) {
	return f.rewrites[id], f.errHistory
}

func (f *fakeReader) ConventionTagDates(_ context.Context, id int64) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	for k, v := range f.tagDates[id] {
		out[k] = v
	}
	return out, f.errHistory
}

func (f *fakeReader) ListRepositories(context.Context) ([]model.Repository, error) {
	return f.repos, f.errRepos
}

func (f *fakeReader) ListBranches(_ context.Context, id int64) ([]model.Branch, error) {
	return f.branches[id], f.errBranches
}

func (f *fakeReader) ConventionCommits(_ context.Context, repoID, authorID int64) ([]model.ConventionCommit, error) {
	f.calls = append(f.calls, authorID)
	if f.errCommits != nil {
		return nil, f.errCommits
	}
	out := []model.ConventionCommit{}
	for _, c := range f.commits[repoID] {
		if authorID == 0 || c.AuthorID == authorID {
			out = append(out, c)
		}
	}
	return out, nil
}

// fakeClones is an in-memory CloneReader: commits reachable per branch, and per commit for RevRange
// (the commit itself included).
type fakeClones struct {
	has       map[string]bool
	reach     map[string][]string // branch or SHA -> reachable SHAs
	remote    map[string][]string // "owner/name" -> remote branches
	err       error               // RevList and RevRange
	remoteErr error
	asked     [][]string
	ranges    [][3][]string      // branches, include, exclude of each RevRange call
	cancel    context.CancelFunc // called by RevList if not nil
	// cancelRange is called by RevRange if not nil.
	cancelRange context.CancelFunc
}

func (f *fakeClones) HasClone(owner, name string) bool { return f.has[owner+"/"+name] }

func (f *fakeClones) RemoteBranches(_ context.Context, owner, name string) ([]string, error) {
	return f.remote[owner+"/"+name], f.remoteErr
}

func (f *fakeClones) RevRange(_ context.Context, _, _ string, branches, include, exclude []string) ([]string, error) {
	f.ranges = append(f.ranges, [3][]string{slices.Clone(branches), slices.Clone(include), slices.Clone(exclude)})
	if f.cancelRange != nil {
		f.cancelRange()
	}
	if f.err != nil {
		return nil, f.err
	}
	// Like the git provider: requested branches all absent from the clone, nothing included: error.
	if len(branches) > 0 && len(include) == 0 &&
		!slices.ContainsFunc(branches, func(b string) bool { _, ok := f.reach[b]; return ok }) {
		return nil, errors.New("no requested branch in the clone")
	}
	in, out := map[string]bool{}, map[string]bool{}
	for _, b := range branches {
		for _, s := range f.reach[b] {
			in[s] = true
		}
	}
	for _, list := range []struct {
		keys []string
		set  map[string]bool
	}{{include, in}, {exclude, out}} {
		for _, k := range list.keys {
			shas, ok := f.reach[k]
			if !ok {
				return nil, errors.New("bad object " + k)
			}
			for _, s := range shas {
				list.set[s] = true
			}
		}
	}
	res := []string{}
	for s := range in {
		if !out[s] {
			res = append(res, s)
		}
	}
	slices.Sort(res)
	return res, nil
}

func (f *fakeClones) RevList(_ context.Context, _, _ string, branches []string) ([]string, error) {
	f.asked = append(f.asked, slices.Clone(branches))
	if f.cancel != nil {
		f.cancel()
	}
	if f.err != nil {
		return nil, f.err
	}
	var out []string
	for _, b := range branches {
		out = append(out, f.reach[b]...)
	}
	return out, nil
}

func ptr(n int) *int { return &n }

func buildFixture() *fakeReader {
	one, two, zero := ptr(1), ptr(2), ptr(0)
	return &fakeReader{
		repos: []model.Repository{
			{ID: 1, Owner: "alice", Name: "projet", DefaultBranch: "main"},
			{ID: 2, Owner: "bob", Name: "tp", DefaultBranch: "main", ConventionsSince: "2026-09-05"},
		},
		branches: map[int64][]model.Branch{
			1: {{Name: "main", Tracked: true}, {Name: "dev", Tracked: true}, {Name: "old", Tracked: false}},
		},
		commits: map[int64][]model.ConventionCommit{
			1: {
				{SHA: "a4", Message: "wip", ParentCount: one, AuthoredAt: day(9), AuthorID: 10, AuthorName: "Alice", OnTracked: true},
				{SHA: "a3", Message: "Merge branch 'dev'", ParentCount: two, AuthoredAt: day(8), AuthorID: 10, AuthorName: "Alice", OnTracked: true, OnDefault: true},
				{SHA: "a2", Message: "feat: Ajoute la page", AuthoredAt: day(7), AuthorID: 11, AuthorLogin: "bob", OnTracked: true, OnDefault: true},
				{SHA: "x1", Message: "feat: branche abandonnee", ParentCount: one, AuthoredAt: day(6), AuthorID: 11, AuthorLogin: "bob"},
				{SHA: "a1", Message: "Initial commit", ParentCount: zero, AuthoredAt: day(5), CommitterLogin: "web-flow", OnTracked: true, OnDefault: true},
			},
			2: {
				{SHA: "b2", Message: "fix: corrige le calcul", ParentCount: one, AuthoredAt: day(6), AuthorID: 11, AuthorLogin: "bob", OnTracked: true, OnDefault: true},
				{SHA: "b1", Message: "maj", ParentCount: one, AuthoredAt: day(4), AuthorID: 11, AuthorLogin: "bob", OnTracked: true, OnDefault: true},
			},
		},
	}
}

func TestBuildDatabase(t *testing.T) {
	r := buildFixture()
	got, err := Build(t.Context(), r, nil, Filter{}, BuildOptions{Rules: DefaultRules(), Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	if got.RulesVersion != RulesVersion || len(got.Repos) != 2 {
		t.Fatalf("Build = %+v", got)
	}
	p := got.Repos[0]
	// a4 (wip) and a2 (warning only) are evaluated; a3 merge, a1 GitHub initial commit; x1 off the tracked branches.
	if p.Source != SourceDatabase || p.Evaluated != 2 || p.Conform != 1 || p.Excluded[ExclusionMerge] != 1 ||
		p.Excluded[ExclusionInitialCommit] != 1 || p.ParentsUnknown != 1 || p.SinceDate != "" || p.SinceOrigin != "" {
		t.Errorf("repository 1: %+v", p)
	}
	if p.DefaultBranch.Evaluated != 1 || p.DefaultBranch.Conform != 1 || p.DefaultBranch.ExcludedTotal() != 2 {
		t.Errorf("default branch: %+v", p.DefaultBranch)
	}
	if shas := commitSHAs(p.Commits); !slices.Equal(shas, []string{"a4", "a3", "a2", "a1"}) {
		t.Errorf("commits: %v", shas)
	}
	if c := p.Commits[0]; c.AuthorName != "Alice" || c.AuthorID != 10 || c.Conform || !slices.Equal(c.Codes, []string{CodeForbidden}) || !c.Evaluated() {
		t.Errorf("commit a4: %+v", c)
	}
	if c := p.Commits[1]; c.Evaluated() || c.Excluded != ExclusionMerge {
		t.Errorf("commit a3: %+v", c)
	}

	// Repository 2: its own start date, at midnight in the workspace time zone.
	q := got.Repos[1]
	if q.SinceDate != "2026-09-05" || q.SinceOrigin != SinceOriginRepository || q.BeforeSince != 1 ||
		q.Evaluated != 1 || q.Conform != 1 || !q.Since.Equal(time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("repository 2: %+v", q)
	}
	if !q.Commits[1].BeforeSince || q.Commits[1].Evaluated() {
		t.Errorf("commit b1: %+v", q.Commits[1])
	}

	// Total and contributors (the commit before the start date is not counted).
	if got.Total.Evaluated != 3 || got.Total.Conform != 2 || got.Total.ExcludedTotal() != 2 {
		t.Errorf("total: %+v", got.Total)
	}
	var names []string
	for _, c := range got.Contributors {
		names = append(names, c.AuthorName+"/"+c.AuthorLogin)
	}
	if !slices.Equal(names, []string{"/", "/bob", "Alice/"}) {
		t.Errorf("contributors: %v", names)
	}
	if bob := got.Contributors[1]; bob.AuthorID != 11 || bob.Evaluated != 2 || bob.Conform != 2 ||
		len(bob.Issues) != 1 || bob.Issues[0].Code != CodeDescCapitalized {
		t.Errorf("bob: %+v", bob)
	}
	if unknown := got.Contributors[0]; unknown.AuthorID != 0 || unknown.Evaluated != 0 || unknown.Excluded[ExclusionInitialCommit] != 1 {
		t.Errorf("unknown author: %+v", unknown)
	}

	// Workspace start date: shared by the repositories without their own.
	got, err = Build(t.Context(), r, nil, Filter{RepoID: 1}, BuildOptions{Rules: DefaultRules(), Since: "2026-09-08"})
	if err != nil || len(got.Repos) != 1 {
		t.Fatalf("Build(repo 1) = %+v, %v", got, err)
	}
	if p := got.Repos[0]; p.SinceOrigin != SinceOriginWorkspace || p.SinceDate != "2026-09-08" || p.BeforeSince != 2 || p.Evaluated != 1 {
		t.Errorf("workspace date: %+v", p)
	}
}

func TestBuildAuthors(t *testing.T) {
	r := buildFixture()
	got, err := Build(t.Context(), r, nil, Filter{AuthorIDs: []int64{11, 0, 11}}, BuildOptions{Rules: DefaultRules()})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Contributors) != 1 || got.Contributors[0].AuthorID != 11 || got.Total.Evaluated != 2 {
		t.Errorf("author 11: %+v", got)
	}
	if !slices.Equal(r.calls, []int64{11, 11}) {
		t.Errorf("reads: %v, want one per repository for author 11", r.calls)
	}
	got, err = Build(t.Context(), r, nil, Filter{AuthorIDs: []int64{}}, BuildOptions{Rules: DefaultRules()})
	if err != nil || len(got.Repos) != 2 || got.Total.Evaluated != 0 || len(got.Contributors) != 0 || len(got.Repos[0].Commits) != 0 {
		t.Errorf("no author: %+v, %v", got, err)
	}
	// Several authors: merged from the most recent commit.
	got, err = Build(t.Context(), r, nil, Filter{RepoID: 1, AuthorIDs: []int64{11, 10}}, BuildOptions{Rules: DefaultRules()})
	if err != nil {
		t.Fatal(err)
	}
	if shas := commitSHAs(got.Repos[0].Commits); !slices.Equal(shas, []string{"a4", "a3", "a2"}) {
		t.Errorf("two authors: %v", shas)
	}
}

func TestBuildClone(t *testing.T) {
	r := buildFixture()
	cl := &fakeClones{
		has:   map[string]bool{"alice/projet": true},
		reach: map[string][]string{"main": {"a3", "a2", "a1"}, "dev": {"x1", "a2", "a1"}},
	}
	got, err := Build(t.Context(), r, cl, Filter{}, BuildOptions{Rules: DefaultRules()})
	if err != nil {
		t.Fatal(err)
	}
	p := got.Repos[0]
	// The clone has x1 (still on dev) and no longer a4 (rewritten): both differ from the database.
	if p.Source != SourceClone || !slices.Equal(commitSHAs(p.Commits), []string{"a3", "a2", "x1", "a1"}) ||
		p.Evaluated != 2 || p.Conform != 2 || p.DefaultBranch.Evaluated != 1 {
		t.Errorf("from the clone: %+v", p)
	}
	if got.Repos[1].Source != SourceDatabase {
		t.Errorf("repository without a clone: %s", got.Repos[1].Source)
	}
	if !reflect.DeepEqual(cl.asked, [][]string{{"main", "dev"}, {"main"}}) {
		t.Errorf("branches read: %v", cl.asked)
	}

	// No tracked branch: the default branch; no default branch either: nothing in scope.
	r.branches[1] = nil
	cl.asked = nil
	if _, err := Build(t.Context(), r, cl, Filter{RepoID: 1}, BuildOptions{Rules: DefaultRules()}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cl.asked, [][]string{{"main"}, {"main"}}) {
		t.Errorf("without tracked branch: %v", cl.asked)
	}
	r.repos[0].DefaultBranch = ""
	cl.asked = nil
	got, err = Build(t.Context(), r, cl, Filter{RepoID: 1}, BuildOptions{Rules: DefaultRules()})
	if err != nil || got.Repos[0].Source != SourceClone || len(got.Repos[0].Commits) != 0 || len(cl.asked) != 0 {
		t.Errorf("without any branch: %+v, %v, %v", got.Repos[0], cl.asked, err)
	}

	// Unreadable clone: database fallback.
	r = buildFixture()
	cl.err = errors.New("rev-list failed")
	got, err = Build(t.Context(), r, cl, Filter{RepoID: 1}, BuildOptions{Rules: DefaultRules()})
	if err != nil || got.Repos[0].Source != SourceDatabase || got.Repos[0].Evaluated != 2 {
		t.Errorf("unreadable clone: %+v, %v", got.Repos[0], err)
	}

	// Context ended while reading the clone.
	ctx, cancel := context.WithCancel(t.Context())
	cl.err, cl.cancel = nil, cancel
	if _, err := Build(ctx, r, cl, Filter{RepoID: 1}, BuildOptions{Rules: DefaultRules()}); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
}

func TestBuildErrors(t *testing.T) {
	boom := errors.New("boom")
	opts := BuildOptions{Rules: DefaultRules()}
	r := buildFixture()
	r.errRepos = boom
	if _, err := Build(t.Context(), r, nil, Filter{}, opts); !errors.Is(err, boom) {
		t.Errorf("repositories: %v", err)
	}
	r = buildFixture()
	r.errCommits = boom
	if _, err := Build(t.Context(), r, nil, Filter{}, opts); !errors.Is(err, boom) {
		t.Errorf("commits: %v", err)
	}
	if _, err := Build(t.Context(), r, nil, Filter{AuthorIDs: []int64{10}}, opts); !errors.Is(err, boom) {
		t.Errorf("commits of an author: %v", err)
	}
	r = buildFixture()
	r.errBranches = boom
	cl := &fakeClones{has: map[string]bool{"alice/projet": true}}
	if _, err := Build(t.Context(), r, cl, Filter{}, opts); !errors.Is(err, boom) {
		t.Errorf("branches: %v", err)
	}
	r = buildFixture()
	if _, err := Build(t.Context(), r, nil, Filter{}, BuildOptions{Rules: DefaultRules(), Since: "septembre"}); !errors.Is(err, ErrInvalidSince) {
		t.Errorf("invalid date: %v", err)
	}
}

func TestTallyMerge(t *testing.T) {
	var total Tally
	a := Aggregate([]Commit{
		{SHA: "1", Message: "wip", ParentsKnown: true, ParentCount: 1},
		{SHA: "2", Message: "feat: Ajoute", ParentsKnown: true, ParentCount: 1},
	}, Options{Rules: DefaultRules()}).Tally
	b := Aggregate([]Commit{
		{SHA: "3", Message: "wip", ParentsKnown: true, ParentCount: 1},
		{SHA: "4", Message: "Merge branch 'x'", ParentsKnown: true, ParentCount: 2},
		{SHA: "5", Message: "feat: ok", ParentsKnown: true, ParentCount: 1},
	}, Options{Rules: DefaultRules()}).Tally
	total.Merge(b)
	total.Merge(a)
	if total.Evaluated != 4 || total.Conform != 2 || total.Excluded[ExclusionMerge] != 1 {
		t.Errorf("Merge = %+v", total)
	}
	want := []Issue{
		{Code: CodeForbidden, Severity: SeverityBlocking, Count: 2, SHAs: []string{"3", "1"}},
		{Code: CodeDescCapitalized, Severity: SeverityWarning, Count: 1, SHAs: []string{"2"}},
	}
	if !reflect.DeepEqual(total.Issues, want) {
		t.Errorf("Issues = %+v", total.Issues)
	}
}

func commitSHAs(lines []CommitLine) []string {
	out := make([]string, len(lines))
	for i, c := range lines {
		out[i] = c.SHA
	}
	return out
}
