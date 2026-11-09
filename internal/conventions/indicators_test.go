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

// forgeOptions are the build options of the phase 8.2 tests: default rules, branch rules and milestones.
func forgeOptions() BuildOptions {
	ms, _ := ParseMilestones(DefaultMilestones())
	return BuildOptions{Rules: DefaultRules(), Location: time.UTC, Language: LanguageFR, Milestones: ms,
		Branch: BranchRules{Types: DefaultTypes(), Exclude: DefaultBranchExclude()}}
}

// cpr builds a pull request read for the conventions.
func cpr(n int, title, head string, authorID int64, login string, created time.Time) model.ConventionPullRequest {
	return model.ConventionPullRequest{
		PullRequest: model.PullRequest{Number: n, Title: title, State: model.PullRequestOpen, HeadBranch: head,
			AuthorLogin: login, CreatedAt: created},
		AuthorID: authorID, AuthorName: login + "-name",
	}
}

// forgeFixture is a repository seen by the API, with pull requests, a clone and remote branches.
func forgeFixture() (*fakeReader, *fakeClones) {
	one := ptr(1)
	merged := cpr(2, "Update README.md", "patch-1", 11, "bob", day(6))
	merged.Merged, merged.State = true, model.PullRequestClosed
	closed := cpr(3, "feat: abandonnee", "feat/abandon-x", 10, "alice", day(6))
	closed.State = model.PullRequestClosed
	draft := cpr(4, "wip", "feat/draft", 10, "alice", day(6))
	draft.Draft = true
	bot := cpr(5, "Bump lodash", "renovate-lodash", 0, "dependabot[bot]", day(6))
	old := cpr(6, "old", "old_branch", 10, "alice", day(1))
	r := &fakeReader{
		repos: []model.Repository{{ID: 1, ProviderID: 99, Owner: "alice", Name: "projet", DefaultBranch: "main",
			ConventionsSince: "2026-09-03"}},
		branches: map[int64][]model.Branch{1: {{Name: "main", Tracked: true}, {Name: "Fix_Bug", Tracked: true}}},
		commits: map[int64][]model.ConventionCommit{1: {
			{SHA: "c1", Message: "feat: ajoute la page de connexion", ParentCount: one, AuthoredAt: day(5), AuthorID: 10, AuthorName: "Alice", OnTracked: true},
		}},
		pulls: map[int64][]model.ConventionPullRequest{1: {
			cpr(1, "feat(auth): add login page", "feat/add-login", 10, "alice", day(5)),
			merged, closed, draft, bot, old,
			cpr(7, "fix: same branch", "feat/add-login", 11, "bob", day(7)),
		}},
	}
	cl := &fakeClones{
		has:    map[string]bool{"alice/projet": true},
		reach:  map[string][]string{"main": {"c1"}},
		remote: map[string][]string{"alice/projet": {"Fix_Bug", "dependabot/npm_and_yarn/lodash-4", "feat/add-login", "feat/login", "gh-pages", "main"}},
	}
	return r, cl
}

func TestBuildPullsAndBranches(t *testing.T) {
	r, cl := forgeFixture()
	got, err := Build(t.Context(), r, cl, Filter{}, forgeOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := got.Repos[0]

	// PR1 and PR2: open and merged apart; closed, draft, bot and before the start date skipped.
	pulls := p.Pulls
	if !pulls.Known || pulls.Reason != "" || pulls.Open != (PRCount{Evaluated: 2, Conform: 2}) ||
		pulls.Merged != (PRCount{Evaluated: 1, Conform: 0, Default: 1}) {
		t.Errorf("pulls: %+v", pulls)
	}
	wantSkipped := map[string]int{PRSkipClosed: 1, PRSkipDraft: 1, PRSkipBot: 1, PRSkipFork: 0, PRSkipBeforeSince: 1}
	if !reflect.DeepEqual(pulls.Skipped, wantSkipped) {
		t.Errorf("skipped: %v", pulls.Skipped)
	}
	var numbers []int
	for _, l := range pulls.Pulls {
		numbers = append(numbers, l.Number)
	}
	if !slices.Equal(numbers, []int{7, 2, 1}) {
		t.Errorf("evaluated pulls: %v", numbers)
	}
	if l := pulls.Pulls[1]; l.Category != PRCategoryMerged || l.Conform || l.AuthorID != 11 || l.HeadBranch != "patch-1" ||
		!slices.Equal(l.Default, []string{PRTitleGitHubDefault}) || !slices.Contains(l.Codes, CodeGitHubDefault) {
		t.Errorf("PR 2: %+v", l)
	}

	// BR1: remote branches of the clone and PR head branches (bots excluded), each name once, without
	// the default branch and the exclusions.
	br := p.Branches
	var names []string
	for _, b := range br.Branches {
		names = append(names, b.Name)
	}
	wantNames := []string{"Fix_Bug", "feat/abandon-x", "feat/add-login", "feat/draft", "feat/login", "old_branch", "patch-1"}
	if br.Source != SourceClone || !slices.Equal(names, wantNames) {
		t.Errorf("branches: %s %v", br.Source, names)
	}
	if br.Checked != 7 || br.Conform != 4 || br.Review != 2 {
		t.Errorf("branch counts: %+v", br)
	}
	login := br.Branches[2]
	if !login.InClone || !slices.Equal(login.PRs, []int{1, 7}) || !slices.Equal(login.AuthorIDs, []int64{10, 11}) ||
		!slices.Equal(login.AuthorLogins, []string{"alice", "bob"}) || len(login.Codes) != 0 {
		t.Errorf("feat/add-login: %+v", login)
	}
	if b := br.Branches[0]; b.InClone != true || !slices.Equal(b.Codes, []string{BranchUppercase, BranchUnderscore, BranchNoSlash}) ||
		len(b.AuthorIDs) != 0 || b.PRs == nil {
		t.Errorf("Fix_Bug: %+v", b)
	}
	if b := br.Branches[6]; b.InClone || !slices.Equal(b.PRs, []int{2}) || !slices.Equal(b.Codes, []string{BranchNoSlash}) {
		t.Errorf("patch-1: %+v", b)
	}

	// Contributors: PR1 of their PRs, BR1 of their head branches; a PR author without a commit appears.
	byID := map[int64]ContributorReport{}
	for _, c := range got.Contributors {
		byID[c.AuthorID] = c
	}
	alice, bob := byID[10], byID[11]
	if alice.Pulls.Open != (PRCount{Evaluated: 1, Conform: 1}) || alice.Branches != (BranchTally{Checked: 4, Conform: 3, Review: 1}) ||
		alice.Evaluated != 1 {
		t.Errorf("alice: %+v", alice)
	}
	if bob.AuthorLogin != "bob" || bob.AuthorName != "bob-name" || bob.Evaluated != 0 ||
		bob.Pulls != (PRTally{Open: PRCount{Evaluated: 1, Conform: 1}, Merged: PRCount{Evaluated: 1, Default: 1}}) ||
		bob.Branches != (BranchTally{Checked: 2, Conform: 1}) {
		t.Errorf("bob: %+v", bob)
	}
	if _, ok := byID[0]; ok {
		t.Error("a PR without contributor was attributed")
	}

	// Contributor filter: only the filtered contributors, their PRs and their head branches; nobody else
	// is shown (Alice's PR 1 on the shared branch included), unattributed branches are left out.
	got, err = Build(t.Context(), r, cl, Filter{AuthorIDs: []int64{11}}, forgeOptions())
	if err != nil || len(got.Contributors) != 1 || got.Contributors[0].AuthorID != 11 {
		t.Fatalf("filtered: %+v, %v", got.Contributors, err)
	}
	fp := got.Repos[0].Pulls
	numbers = nil
	for _, l := range fp.Pulls {
		numbers = append(numbers, l.Number)
		if l.AuthorID != 11 {
			t.Errorf("filtered: PR %d of another author", l.Number)
		}
	}
	if !slices.Equal(numbers, []int{7, 2}) || fp.Open != (PRCount{Evaluated: 1, Conform: 1}) ||
		fp.Merged != (PRCount{Evaluated: 1, Default: 1}) || fp.Skipped[PRSkipDraft] != 0 || fp.Skipped[PRSkipBot] != 0 {
		t.Errorf("filtered pulls: %v %+v", numbers, fp)
	}
	fb := got.Repos[0].Branches
	names = nil
	for _, b := range fb.Branches {
		names = append(names, b.Name)
		if !slices.Equal(b.AuthorIDs, []int64{11}) || !slices.Equal(b.AuthorLogins, []string{"bob"}) {
			t.Errorf("filtered branch %s: %+v", b.Name, b)
		}
	}
	if !slices.Equal(names, []string{"feat/add-login", "patch-1"}) || fb.Checked != 2 || fb.Conform != 1 ||
		!slices.Equal(fb.Branches[0].PRs, []int{7}) || fb.Source != SourceClone {
		t.Errorf("filtered branches: %v %+v", names, fb)
	}
}

func TestBuildPullsUnknown(t *testing.T) {
	for name, tweak := range map[string]func(*fakeReader, *BuildOptions){
		"clone only workspace":  func(_ *fakeReader, o *BuildOptions) { o.LocalOnly = true },
		"never seen by the API": func(r *fakeReader, _ *BuildOptions) { r.repos[0].ProviderID = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			r, cl := forgeFixture()
			opts := forgeOptions()
			tweak(r, &opts)
			r.errPulls, r.errReleases = errors.New("not read"), errors.New("not read")
			got, err := Build(t.Context(), r, cl, Filter{}, opts)
			if err != nil {
				t.Fatal(err)
			}
			p := got.Repos[0]
			if p.Pulls.Known || p.Pulls.Reason != UnknownNoAPI || len(p.Pulls.Pulls) != 0 || p.Pulls.Skipped[PRSkipDraft] != 0 {
				t.Errorf("pulls: %+v", p.Pulls)
			}
			if p.Tags.Releases.Known || p.Tags.Releases.Reason != UnknownNoAPI || p.Tags.Releases.Notes == nil {
				t.Errorf("releases: %+v", p.Tags.Releases)
			}
			// Without the PRs, only the remote branches of the clone.
			if p.Branches.Checked != 3 {
				t.Errorf("branches: %+v", p.Branches)
			}
		})
	}
}

func TestBuildBranchesDatabase(t *testing.T) {
	r, cl := forgeFixture()
	opts := forgeOptions()
	opts.LocalOnly = true
	// No clone: tracked branches of the database.
	got, err := Build(t.Context(), r, nil, Filter{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if b := got.Repos[0].Branches; b.Source != SourceDatabase || b.Checked != 1 || b.Branches[0].Name != "Fix_Bug" || !b.Branches[0].InClone {
		t.Errorf("without a clone: %+v", b)
	}
	// Unreadable clone: same fallback.
	cl.remoteErr = errors.New("for-each-ref failed")
	got, err = Build(t.Context(), r, cl, Filter{}, opts)
	if err != nil || got.Repos[0].Branches.Source != SourceDatabase {
		t.Errorf("unreadable clone: %+v, %v", got.Repos[0].Branches, err)
	}
	// Database error without a clone.
	r.errBranches = errors.New("boom")
	if _, err := Build(t.Context(), r, nil, Filter{}, opts); !errors.Is(err, r.errBranches) {
		t.Errorf("database error: %v", err)
	}
	// Ended context while reading the remote branches.
	r.errBranches = nil
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := buildBranches(ctx, r, cl, r.repos[0], nil, mustChecker(t, opts.Branch)); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
	// Invalid branch rules (configuration not validated).
	opts.Branch.Pattern = "("
	if _, err := Build(t.Context(), r, nil, Filter{}, opts); !errors.Is(err, ErrInvalidBranchPattern) {
		t.Errorf("invalid pattern: %v", err)
	}
}

func mustChecker(t *testing.T, r BranchRules) *BranchChecker {
	t.Helper()
	c, err := NewBranchChecker(r)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestBuildTags(t *testing.T) {
	r, _ := forgeFixture()
	r.tags = map[int64][]model.Tag{1: {
		{Name: "v1.0.0", SHA: "c1"}, {Name: "1.1.0", SHA: "c1"}, {Name: "V2.0.0", SHA: "c1"},
		{Name: "v2.0.0-rc.1", SHA: "c1"}, {Name: "v1.0.0+b", SHA: "c1"},
	}}
	r.releases = map[int64][]model.Release{1: {
		{TagName: "v1.0.0"}, {TagName: "v9.9.9"}, {TagName: "v2.0.0-rc.1"}, {TagName: "v3.0.0", Draft: true},
	}}
	got, err := Build(t.Context(), r, nil, Filter{}, forgeOptions())
	if err != nil {
		t.Fatal(err)
	}
	tg := got.Repos[0].Tags
	var names []string
	for _, x := range tg.Tags {
		names = append(names, x.Name)
	}
	if tg.Checked != 5 || tg.Conform != 3 || !slices.Equal(names, []string{"1.1.0", "V2.0.0", "v1.0.0", "v1.0.0+b", "v2.0.0-rc.1"}) {
		t.Errorf("SV1: %d/%d %v", tg.Conform, tg.Checked, names)
	}
	var states []string
	for _, m := range tg.Milestones {
		states = append(states, m.Name+":"+m.Status+":"+m.Tag)
	}
	want := []string{"v1.0.0:present:v1.0.0", "v2.0.0:nonconforming_name:V2.0.0", "v3.0.0:absent:"}
	if !slices.Equal(states, want) {
		t.Errorf("SV2: %v", states)
	}
	wantNotes := []ReleaseNote{
		{Code: ReleaseMilestoneMissing, Tag: "V2.0.0"},
		{Code: ReleaseTagMissing, Tag: "v9.9.9"},
		{Code: ReleasePrerelease, Tag: "v2.0.0-rc.1"},
	}
	if !tg.Releases.Known || !reflect.DeepEqual(tg.Releases.Notes, wantNotes) {
		t.Errorf("SV6: %+v", tg.Releases)
	}
	for _, field := range []*error{&r.errTags, &r.errReleases, &r.errPulls} {
		*field = errors.New("boom")
		if _, err := Build(t.Context(), r, nil, Filter{}, forgeOptions()); !errors.Is(err, *field) {
			t.Errorf("read error: %v", err)
		}
		*field = nil
	}
}

// intervalFixture is a linear history c1..c5 on main, v0.1.0 on c2 and v1.0.0 on c4; zz is in the
// v1.0.0 interval of the clone but absent from the database.
func intervalFixture() (*fakeReader, *fakeClones) {
	one := ptr(1)
	commit := func(sha, msg string, d int, id int64, name string) model.ConventionCommit {
		return model.ConventionCommit{SHA: sha, Message: msg, ParentCount: one, AuthoredAt: day(d), AuthorID: id,
			AuthorName: name, OnTracked: true, OnDefault: true}
	}
	r := &fakeReader{
		repos:    []model.Repository{{ID: 1, Owner: "alice", Name: "projet", DefaultBranch: "main"}},
		branches: map[int64][]model.Branch{1: {{Name: "main", Tracked: true}}},
		commits: map[int64][]model.ConventionCommit{1: {
			commit("c5", "feat: ajoute la page des notes", 9, 11, "Bob"),
			commit("c4", "Added stuff", 8, 11, "Bob"),
			commit("c3", "fix: corrige le calcul", 7, 10, "Alice"),
			commit("c2", "wip", 6, 11, "Bob"),
			commit("c1", "feat: initialise le projet\n\nCloses #1", 5, 10, "Alice"),
		}},
		tags: map[int64][]model.Tag{1: {{Name: "v0.1.0", SHA: "c2"}, {Name: "v1.0.0", SHA: "c4"}, {Name: "v1.1.0-rc.1", SHA: "c5"}}},
	}
	cl := &fakeClones{
		has: map[string]bool{"alice/projet": true},
		reach: map[string][]string{
			"main": {"c1", "c2", "c3", "c4", "c5", "zz"},
			"c1":   {"c1"}, "c2": {"c1", "c2"}, "c3": {"c1", "c2", "c3"},
			"c4": {"c1", "c2", "c3", "c4", "zz"}, "c5": {"c1", "c2", "c3", "c4", "c5", "zz"},
		},
		remote: map[string][]string{"alice/projet": {"main"}},
	}
	return r, cl
}

func TestBuildIntervalsAndTrend(t *testing.T) {
	r, cl := intervalFixture()
	got, err := Build(t.Context(), r, cl, Filter{}, forgeOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := got.Repos[0]
	iv := p.Intervals
	if !iv.Known || iv.Reason != "" || iv.LastTag != "v1.0.0" || len(iv.Intervals) != 2 {
		t.Fatalf("intervals: %+v", iv)
	}
	first, second := iv.Intervals[0], iv.Intervals[1]
	if first.From != "" || first.To != "v0.1.0" || !first.Known || first.Evaluated != 2 || first.Conform != 1 || first.Missing != 0 {
		t.Errorf("first interval: %+v", first)
	}
	if second.From != "v0.1.0" || second.To != "v1.0.0" || second.Evaluated != 2 || second.Conform != 1 || second.Missing != 1 {
		t.Errorf("second interval: %+v", second)
	}
	if iv.SinceLastTag.Evaluated != 1 || iv.SinceLastTag.Conform != 1 {
		t.Errorf("since the last tag: %+v", iv.SinceLastTag)
	}
	if !reflect.DeepEqual(cl.ranges[1:], [][3][]string{{nil, {"c2"}, nil}, {nil, {"c4"}, {"c2"}}}) {
		t.Errorf("ranges read: %v", cl.ranges)
	}

	// CC5 to CC7 per repository: c4 is in the past tense; c1 cites an issue in a footer.
	tr := p.Traits
	if tr.Evaluated != 5 || tr.Wording[WordingPastTense] != 1 || tr.Footer != 1 || tr.Issue != 1 {
		t.Errorf("traits: %+v", tr)
	}
	if p.Language.Expected != LanguageFR || p.Language.FR+p.Language.EN+p.Language.Unknown != 5 {
		t.Errorf("language: %+v", p.Language)
	}

	// CC3 trend: Bob 0/2 before v1.0.0, 1/1 since; Alice 2/2 before.
	var alice, bob ContributorReport
	for _, c := range got.Contributors {
		switch c.AuthorID {
		case 10:
			alice = c
		case 11:
			bob = c
		}
	}
	if !bob.Trend.Known() || bob.Trend.Repos != 1 || bob.Trend.Before.Evaluated != 2 || bob.Trend.Before.Conform != 0 ||
		bob.Trend.Since.Evaluated != 1 || bob.Trend.Since.Conform != 1 || bob.Trend.Unmeasured != 0 {
		t.Errorf("bob's trend: %+v", bob.Trend)
	}
	if alice.Trend.Before.Conform != 2 || alice.Trend.Since.Evaluated != 0 || alice.Traits.Issue != 1 {
		t.Errorf("alice: %+v", alice)
	}

	// Contributor filter: the trend is placed, the repository intervals are not computed.
	got, err = Build(t.Context(), r, cl, Filter{AuthorIDs: []int64{11}}, forgeOptions())
	if err != nil {
		t.Fatal(err)
	}
	if iv := got.Repos[0].Intervals; iv.Known || iv.Reason != UnknownFiltered || len(iv.Intervals) != 0 || iv.LastTag != "v1.0.0" {
		t.Errorf("filtered intervals: %+v", iv)
	}
	if b := got.Contributors[0]; b.Trend.Since.Evaluated != 1 || b.Trend.Before.Evaluated != 2 {
		t.Errorf("filtered trend: %+v", b.Trend)
	}
}

func TestBuildIntervalsUnknown(t *testing.T) {
	r, cl := intervalFixture()
	check := func(name string, cr CloneReader, reason string) {
		t.Helper()
		got, err := Build(t.Context(), r, cr, Filter{}, forgeOptions())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		iv := got.Repos[0].Intervals
		if iv.Known || iv.Reason != reason || iv.Intervals == nil || len(iv.Intervals) != 0 {
			t.Errorf("%s: %+v", name, iv)
		}
		for _, c := range got.Contributors {
			if c.Trend.Known() || c.Trend.Unmeasured != c.Evaluated {
				t.Errorf("%s: trend of %d: %+v", name, c.AuthorID, c.Trend)
			}
		}
	}
	check("no clone", nil, UnknownNoClone)
	check("clone of another repository", &fakeClones{}, UnknownNoClone)
	cl.err = errors.New("rev-list failed")
	check("unreadable clone", cl, UnknownCloneUnreadable)
	cl.err = nil
	r.tags[1] = []model.Tag{{Name: "1.0.0", SHA: "c4"}, {Name: "v2.0.0-rc.1", SHA: "c5"}}
	check("no final tag", cl, UnknownNoFinalTag)

	// An interval whose tag commit is missing from the clone is unknown on its own.
	r.tags[1] = []model.Tag{{Name: "v0.1.0", SHA: "c2"}, {Name: "v1.0.0", SHA: "c4"}, {Name: "v2.0.0", SHA: "c5"}}
	delete(cl.reach, "c2")
	got, err := Build(t.Context(), r, cl, Filter{}, forgeOptions())
	if err != nil {
		t.Fatal(err)
	}
	var known []bool
	for _, l := range got.Repos[0].Intervals.Intervals {
		known = append(known, l.Known)
	}
	if !slices.Equal(known, []bool{false, false, true}) {
		t.Errorf("intervals known: %v", known)
	}

	// Ended context while reading an interval.
	ctx, cancel := context.WithCancel(t.Context())
	cl.reach["c2"] = []string{"c1", "c2"}
	cl.cancelRange = cancel
	if _, err := Build(ctx, r, cl, Filter{}, forgeOptions()); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
}
