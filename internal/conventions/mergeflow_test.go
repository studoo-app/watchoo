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

// fpClones is a fakeClones that also reads the first parent (FirstParentReader).
type fpClones struct {
	*fakeClones
	chains map[string][]string // branch or head -> first-parent SHAs
	fpErr  error
	fpAsk  []fpCall
}

type fpCall struct {
	branch, head string
	exclude      []string
	since, until time.Time
}

func (f *fpClones) FirstParent(_ context.Context, _, _, branch, head string, exclude []string, since, until time.Time) ([]string, error) {
	f.fpAsk = append(f.fpAsk, fpCall{branch, head, exclude, since, until})
	if f.fpErr != nil {
		return nil, f.fpErr
	}
	return f.chains[branch+head], nil
}

// mergeFixture is a repository seen by the API with merged pull requests (squash, merge commit), a fork, a
// reused branch and a first-parent chain mixing arrivals by PR and outside PR.
func mergeFixture() (*fakeReader, *fpClones) {
	gh := func(c model.ConventionCommit) model.ConventionCommit {
		c.CommitterLogin, c.CommitterEmails = "web-flow", []string{"noreply@github.com"}
		return c
	}
	human := func(c model.ConventionCommit) model.ConventionCommit {
		c.CommitterLogin, c.CommitterEmails = c.AuthorLogin, []string{c.AuthorLogin + "@example.org"}
		return c
	}
	commit := func(sha, msg string, parents int, id int64, login string, d int) model.ConventionCommit {
		return model.ConventionCommit{SHA: sha, Message: msg, ParentCount: ptr(parents), AuthoredAt: day(d),
			CommittedAt: day(d), AuthorID: id, AuthorName: login + "-name", AuthorLogin: login, OnTracked: true, OnDefault: true}
	}
	sq, mc, direct, web := sha40('1'), sha40('2'), sha40('3'), sha40('4')
	all := int64(PRBodyContext | PRBodyChanges | PRBodyTesting | PRBodyScreenshots | PRBodyCloses)
	merged := func(n int, title, head string, id int64, login, sha string, mask *int64) model.ConventionPullRequest {
		pr := cpr(n, title, head, id, login, day(5))
		pr.Merged, pr.State, pr.BaseBranch, pr.MergedAt, pr.MergeCommitSHA = true, model.PullRequestClosed, "main", day(6), sha
		pr.FromFork, pr.BodySections = boolp(false), mask
		return pr
	}
	fork := cpr(3, "feat: from a fork", "Main", 12, "carol", day(5))
	fork.FromFork = boolp(true)
	r := &fakeReader{
		repos: []model.Repository{{ID: 1, ProviderID: 99, Owner: "alice", Name: "projet", DefaultBranch: "main",
			AllowSquashMerge: boolp(true), AllowMergeCommit: boolp(false), AllowRebaseMerge: boolp(false),
			ConventionsPRsVersion: PRRulesVersion}},
		branches: map[int64][]model.Branch{1: {{Name: "main", Tracked: true}}},
		commits: map[int64][]model.ConventionCommit{1: {
			gh(commit(sq, "feat: ajoute la connexion (#1)", 1, 10, "alice", 9)),
			gh(commit(mc, "Merge pull request #2 from bob/feat/add-notes", 2, 11, "bob", 8)),
			human(commit(direct, "fix: push direct", 1, 10, "alice", 7)),
			gh(commit(web, "Update README.md", 1, 11, "bob", 4)),
		}},
		pulls: map[int64][]model.ConventionPullRequest{1: {
			merged(1, "feat: ajoute la connexion", "feat/add-login", 10, "alice", sq, &all),
			merged(2, "feat: ajoute les notes", "feat/add-notes", 11, "bob", mc, nil),
			fork,
			merged(4, "fix: corrige les notes", "feat/add-notes", 11, "bob", "", nil),
		}},
	}
	cl := &fpClones{
		fakeClones: &fakeClones{has: map[string]bool{"alice/projet": true},
			reach:  map[string][]string{"main": {sq, mc, direct, web}},
			remote: map[string][]string{"alice/projet": {"main", "feat/add-login"}}},
		chains: map[string][]string{"main": {sq, mc, direct, web}},
	}
	return r, cl
}

// Build measures PR3, PR5, PR6, BR2 and BR5 and excludes the forks from BR1 and PR1.
func TestBuildMergeFlow(t *testing.T) {
	r, cl := mergeFixture()
	opts := forgeOptions()
	opts.PRBody = DefaultPRBodyRules()
	got, err := Build(t.Context(), r, cl, Filter{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	p := got.Repos[0]

	// Fork: skipped from PR1 and absent from BR1 ("Main" would be nonconforming).
	if p.Pulls.Skipped[PRSkipFork] != 1 || slices.ContainsFunc(p.Pulls.Pulls, func(l PullLine) bool { return l.Number == 3 }) {
		t.Errorf("fork pulls: %+v", p.Pulls.Skipped)
	}
	if slices.ContainsFunc(p.Branches.Branches, func(b BranchLine) bool { return b.Name == "Main" }) {
		t.Errorf("fork branch in BR1: %+v", p.Branches.Branches)
	}
	// PR5 and PR6.
	byNumber := map[int]PullLine{}
	for _, l := range p.Pulls.Pulls {
		byNumber[l.Number] = l
	}
	if l := byNumber[1]; !l.Body.Known || !l.Body.Conform || l.Merge != (MergeResult{Method: MergeMethodSquash, MessageConform: true, Conform: true}) {
		t.Errorf("PR #1: %+v", l)
	}
	if l := byNumber[2]; l.Body.Known || l.Merge.Method != MergeMethodMerge || l.Merge.Conform {
		t.Errorf("PR #2: %+v", l)
	}
	if l := byNumber[4]; l.Merge.Method != MergeMethodUnknown {
		t.Errorf("PR #4 without merge commit: %+v", l)
	}
	if m := p.Pulls.Merged; m.Evaluated != 3 || m.BodyKnown != 1 || m.BodyConform != 1 || m.MergeKnown != 2 || m.Squash != 1 || m.MergeConform != 1 {
		t.Errorf("merged counts: %+v", m)
	}
	wantMethods := map[string]int{MergeMethodSquash: 1, MergeMethodMerge: 1, MergeMethodRebase: 0, MergeMethodUnknown: 1}
	if !reflect.DeepEqual(p.Pulls.Methods, wantMethods) {
		t.Errorf("methods: %v", p.Pulls.Methods)
	}
	// BR2: feat/add-notes merged twice.
	if !p.Branches.ReusedKnown || !reflect.DeepEqual(p.Branches.Reused, []ReusedBranch{{Name: "feat/add-notes", PRs: []int{2, 4}}}) {
		t.Errorf("reused: %+v", p.Branches.Reused)
	}
	// BR5: squash only.
	if mm := p.MergeMethods; !mm.Known || !mm.Conform || mm.Reason != "" {
		t.Errorf("merge methods: %+v", mm)
	}
	// PR3: squash and GitHub merge by PR; direct push and web edit (complete PR list) outside PR.
	a := p.Arrivals
	if !a.Known || !a.API || !a.Complete || a.ByPR != 2 || a.OutsidePR != 2 || a.Undetermined != 0 {
		t.Errorf("arrivals: %+v", a)
	}
	if a.Reasons[ArrivalMergeCommitSHA] != 2 || a.Reasons[ArrivalHumanCommitter] != 1 || a.Reasons[ArrivalWebEdit] != 1 ||
		len(a.Reasons) != len(AllArrivalReasons) {
		t.Errorf("reasons: %v", a.Reasons)
	}
	if out := a.Outside(); len(out) != 2 || out[0].SHA != sha40('3') || out[0].AuthorLogin != "alice" || out[1].AuthorLogin != "bob" {
		t.Errorf("outside: %+v", out)
	}
	if call := cl.fpAsk[0]; call.branch != "main" || call.head != "" || call.exclude != nil {
		t.Errorf("first parent call: %+v", call)
	}
	for _, c := range got.Contributors {
		if c.AuthorLogin == "alice" && c.Arrivals != (ArrivalTally{ByPR: 1, OutsidePR: 1}) {
			t.Errorf("alice arrivals: %+v", c.Arrivals)
		}
		if c.AuthorLogin == "bob" && c.Arrivals != (ArrivalTally{ByPR: 1, OutsidePR: 1}) {
			t.Errorf("bob arrivals: %+v", c.Arrivals)
		}
	}

	// Restricted to bob: only his commits and his reused branch; PR6 of his PR read on all commits.
	r.calls = nil
	got, err = Build(t.Context(), r, cl, Filter{AuthorIDs: []int64{11}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	p = got.Repos[0]
	if a := p.Arrivals; a.Total() != 2 || slices.ContainsFunc(a.Lines, func(l ArrivalLine) bool { return l.AuthorID != 11 }) {
		t.Errorf("bob arrivals: %+v", a)
	}
	if len(p.Branches.Reused) != 1 || p.Pulls.Merged.Evaluated != 2 {
		t.Errorf("bob pulls: %+v %+v", p.Branches.Reused, p.Pulls.Merged)
	}

	// The PR list was not read in full: a commit of GitHub without PR is undetermined.
	r.repos[0].ConventionsPRsVersion = 0
	got, _ = Build(t.Context(), r, cl, Filter{}, opts)
	if a := got.Repos[0].Arrivals; a.Complete || a.OutsidePR != 1 || a.Reasons[ArrivalUnmatched] != 1 {
		t.Errorf("incomplete: %+v", a)
	}
	// Merge methods not returned (no push access).
	r.repos[0].AllowRebaseMerge = nil
	got, _ = Build(t.Context(), r, cl, Filter{}, opts)
	if mm := got.Repos[0].MergeMethods; mm.Known || mm.Reason != UnknownNoPushAccess {
		t.Errorf("no push: %+v", mm)
	}
}

// Without the API (clone only), PR5, PR6, BR2 and BR5 are unknown and PR3 falls back on the messages of
// GitHub; without a first-parent reader or with an unreadable clone, PR3 is unknown.
func TestBuildMergeFlowDegraded(t *testing.T) {
	r, cl := mergeFixture()
	opts := forgeOptions()
	opts.LocalOnly = true
	got, err := Build(t.Context(), r, cl, Filter{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	p := got.Repos[0]
	if p.Branches.ReusedKnown || len(p.Branches.Reused) != 0 || p.MergeMethods.Reason != UnknownNoAPI || p.Pulls.Known {
		t.Errorf("clone only: %+v %+v", p.Branches, p.MergeMethods)
	}
	// Squash suffix and "Merge pull request" by PR; direct push outside; "Update README.md" of GitHub
	// undetermined.
	if a := p.Arrivals; !a.Known || a.API || a.ByPR != 2 || a.OutsidePR != 1 || a.Reasons[ArrivalCloneOnly] != 1 {
		t.Errorf("clone only arrivals: %+v", a)
	}

	got, _ = Build(t.Context(), r, cl.fakeClones, Filter{}, forgeOptions())
	if a := got.Repos[0].Arrivals; a.Known || a.Reason != UnknownNoClone || a.Lines == nil {
		t.Errorf("no first-parent reader: %+v", a)
	}
	cl.fpErr = errors.New("git")
	got, _ = Build(t.Context(), r, cl, Filter{}, forgeOptions())
	if a := got.Repos[0].Arrivals; a.Known || a.Reason != UnknownCloneUnreadable {
		t.Errorf("unreadable: %+v", a)
	}
	got, _ = Build(t.Context(), r, nil, Filter{}, forgeOptions())
	if a := got.Repos[0].Arrivals; a.Known || a.Reason != UnknownNoClone {
		t.Errorf("no clone: %+v", a)
	}
}

// The milestone review judges PR3 = 0 on the first parent of the milestone interval.
func TestReviewMilestonePR3(t *testing.T) {
	r, base := intervalFixture()
	r.repos[0].ProviderID = 1
	for i := range r.commits[1] {
		c := &r.commits[1][i]
		c.CommitterLogin, c.CommitterEmails = "web-flow", []string{"noreply@github.com"}
		c.Message = "Merge pull request #1 from x/y"
		c.ParentCount = ptr(2)
	}
	cl := &fpClones{fakeClones: base, chains: map[string][]string{"c4": {"c4", "c3"}, "main": {"c5", "c4"}}}
	opts := reviewOptions(t)
	after := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	pr3 := func(rv MilestoneReview) Criterion { return rv.Repos[0].Criteria[2] }

	rv, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if err != nil {
		t.Fatal(err)
	}
	if c := pr3(rv); c.ID != CriterionPR3 || c.Status != CriterionConform || rv.Repos[0].Arrivals == nil || rv.Repos[0].Arrivals.ByPR != 2 {
		t.Errorf("v1.0.0 conform: %+v %+v", c, rv.Repos[0].Arrivals)
	}
	call := cl.fpAsk[len(cl.fpAsk)-1]
	if call.head != "c4" || call.branch != "" || !slices.Equal(call.exclude, []string{"c2"}) || !call.since.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("v1.0.0 call: %+v", call)
	}

	// Due-date bound (tag absent): default branch until the end of the due date.
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v2.0.0", opts, after)
	call = cl.fpAsk[len(cl.fpAsk)-1]
	if call.branch != "main" || call.head != "" || !slices.Equal(call.exclude, []string{"c4"}) || call.until.IsZero() {
		t.Errorf("v2.0.0 call: %+v", call)
	}
	if c := pr3(rv); c.Status != CriterionConform {
		t.Errorf("v2.0.0: %+v", c)
	}

	// A direct push: nonconform; a commit absent from the database: undetermined, unknown.
	r.commits[1][1].CommitterLogin, r.commits[1][1].CommitterEmails = "bob", []string{"bob@example.org"} // c4
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if c := pr3(rv); c.Status != CriterionNonconform {
		t.Errorf("direct push: %+v", c)
	}
	cl.chains["c4"] = []string{"c3", "zz"}
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if c := pr3(rv); c.Status != CriterionUnknown || c.Reason != UnknownUndetermined {
		t.Errorf("undetermined: %+v", c)
	}
	cl.chains["c4"] = []string{}
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if c := pr3(rv); c.Status != CriterionUnknown || c.Reason != UnknownNoCommits {
		t.Errorf("no commits: %+v", c)
	}
	cl.fpErr = errors.New("git")
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if c := pr3(rv); c.Status != CriterionUnknown || c.Reason != UnknownCloneUnreadable {
		t.Errorf("unreadable: %+v", c)
	}
	// Canceled context: an error, never an unknown criterion.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ReviewMilestone(ctx, r, cl, Filter{}, "v1.0.0", opts, after); err == nil {
		t.Error("canceled context accepted")
	}
}
