package conventions

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// reviewOptions are forgeOptions with a start date and milestones v0.1.0, v1.0.0, v2.0.0 (due on
// September 20) and v3.0.0 (no due date).
func reviewOptions(t *testing.T) BuildOptions {
	t.Helper()
	opts := forgeOptions()
	opts.Since = "2026-09-01"
	ms, err := ParseMilestones([]string{"v0.1.0", "v1.0.0", "v2.0.0@2026-09-20", "v3.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	opts.Milestones = ms
	return opts
}

// statuses returns "ID:status:reason" for each criterion.
func statuses(cs []Criterion) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID+":"+c.Status+":"+c.Reason)
	}
	return out
}

// fileTail returns the milestone file criteria (phase 8.3), all unknown for reason.
func fileTail(reason string) []string {
	out := []string{}
	for _, id := range []string{"CL9", "CL8", "RD1", "RD2", "RD3", "DOC1", "DOC3"} {
		out = append(out, id+":unknown:"+reason)
	}
	return out
}

func TestReviewMilestone(t *testing.T) {
	r, cl := intervalFixture()
	r.pulls = map[int64][]model.ConventionPullRequest{1: {cpr(1, "feat: x", "feat/add-login", 10, "alice", day(5))}}
	r.repos[0].ProviderID = 1
	opts := reviewOptions(t)
	after := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	// v1.0.0: interval v0.1.0..v1.0.0 has a nonconforming commit (c4).
	rv, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if err != nil {
		t.Fatal(err)
	}
	if rv.Milestone.Name != "v1.0.0" || len(rv.Repos) != 1 {
		t.Fatalf("review: %+v", rv)
	}
	p := rv.Repos[0]
	// No file result cached yet (phase 8.3): the file criteria are "not computed".
	want := append([]string{"CC1:nonconform:", "BR1:conform:", "SV1:conform:", "SV2:conform:"}, fileTail(UnknownNotComputed)...)
	if !p.Evaluable || p.SinceDate != "2026-09-01" || !slices.Equal(statuses(p.Criteria), want) || p.Conform != 3 || p.Unknown != 7 {
		t.Errorf("v1.0.0: %v %+v", statuses(p.Criteria), p)
	}
	if iv := p.Interval; iv == nil || iv.From != "v0.1.0" || iv.To != "v1.0.0" || iv.Evaluated != 2 || iv.Conform != 1 || iv.Missing != 1 {
		t.Errorf("v1.0.0 interval: %+v", p.Interval)
	}
	if p.Tag == nil || p.Tag.Name != "v1.0.0" || p.Branches.Checked != 1 || p.Milestone.Status != MilestonePresent {
		t.Errorf("v1.0.0 details: %+v", p)
	}

	// v0.1.0: first interval, from the start of the history: c1 conform, c2 "wip".
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v0.1.0", opts, after)
	if p := rv.Repos[0]; p.Interval == nil || p.Interval.From != "" || p.Interval.Evaluated != 2 || p.Criteria[0].Status != CriterionNonconform {
		t.Errorf("v0.1.0: %+v", p.Interval)
	}

	// v2.0.0, tag absent, due date passed: tracked branches since v1.0.0, written before the end of the
	// due date (c5, September 9); SV1 unknown (no tag name to judge, due date passed), SV2 nonconform.
	rv, err = ReviewMilestone(t.Context(), r, cl, Filter{}, "v2.0.0", opts, after)
	if err != nil {
		t.Fatal(err)
	}
	p = rv.Repos[0]
	want = append([]string{"CC1:conform:", "BR1:conform:", "SV1:unknown:tag_absent", "SV2:nonconform:"},
		fileTail(UnknownTagAbsent)...)
	if !slices.Equal(statuses(p.Criteria), want) || p.Conform != 2 || p.Unknown != 8 || p.Tag != nil {
		t.Errorf("v2.0.0: %v", statuses(p.Criteria))
	}
	if iv := p.Interval; iv == nil || iv.From != "v1.0.0" || iv.To != "" || iv.Due != "2026-09-20" || iv.Evaluated != 1 {
		t.Errorf("v2.0.0 interval: %+v", p.Interval)
	}
	if last := cl.ranges[len(cl.ranges)-1]; !slices.Equal(last[0], []string{"main"}) || last[1] != nil || !slices.Equal(last[2], []string{"c4"}) {
		t.Errorf("v2.0.0 range: %v", last)
	}

	// The due date excludes later commits: September 8 ends before c5 (September 9).
	opts.Milestones[2].Due = "2026-09-08"
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v2.0.0", opts, after)
	if c := rv.Repos[0].Criteria[0]; c.Status != CriterionUnknown || c.Reason != UnknownNoCommits {
		t.Errorf("v2.0.0 before c5: %+v", c)
	}

	// Before the due date: CC1 and SV2 wait.
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v2.0.0", opts, time.Date(2026, 9, 8, 23, 0, 0, 0, time.UTC))
	want = append([]string{"CC1:unknown:not_due", "BR1:conform:", "SV1:unknown:not_due", "SV2:unknown:not_due"},
		fileTail(UnknownNotDue)...)
	if p := rv.Repos[0]; !slices.Equal(statuses(p.Criteria), want) || p.Unknown != 10 || p.Interval != nil {
		t.Errorf("v2.0.0 before the due date: %v", statuses(p.Criteria))
	}

	// v3.0.0: no tag, no due date.
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v3.0.0", opts, after)
	want = append([]string{"CC1:unknown:no_tag", "BR1:conform:", "SV1:unknown:no_tag", "SV2:nonconform:"},
		fileTail(UnknownNoTag)...)
	if p := rv.Repos[0]; !slices.Equal(statuses(p.Criteria), want) {
		t.Errorf("v3.0.0: %v", statuses(p.Criteria))
	}
}

func TestReviewMilestoneNonconformingTag(t *testing.T) {
	r, cl := intervalFixture()
	r.tags[1] = append(r.tags[1], model.Tag{Name: "V2.0.0", SHA: "c5"})
	r.branches[1] = append(r.branches[1], model.Branch{Name: "Fix_Bug", Tracked: true})
	rv, err := ReviewMilestone(t.Context(), r, nil, Filter{}, "v2.0.0", reviewOptions(t), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	p := rv.Repos[0]
	want := append([]string{"CC1:unknown:no_clone", "BR1:nonconform:", "SV1:nonconform:", "SV2:nonconform:"},
		fileTail(UnknownNoClone)...)
	if !slices.Equal(statuses(p.Criteria), want) || p.Tag == nil || p.Tag.Name != "V2.0.0" || p.Branches.Source != SourceDatabase {
		t.Errorf("V2.0.0 without a clone: %v %+v", statuses(p.Criteria), p)
	}
	// With the clone: interval v1.0.0..V2.0.0 (c5).
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v2.0.0", reviewOptions(t), time.Time{})
	if iv := rv.Repos[0].Interval; iv == nil || iv.From != "v1.0.0" || iv.To != "V2.0.0" || iv.Evaluated != 1 || iv.Conform != 1 {
		t.Errorf("V2.0.0 interval: %+v", iv)
	}
}

func TestReviewMilestoneUnknown(t *testing.T) {
	r, cl := intervalFixture()
	opts := reviewOptions(t)
	after := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	if _, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v9.0.0", opts, after); !errors.Is(err, ErrUnknownMilestone) {
		t.Errorf("unknown milestone: %v", err)
	}

	// No start date: not evaluable, without criteria; SV2 is still read.
	noSince := opts
	noSince.Since = ""
	rv, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", noSince, after)
	if err != nil {
		t.Fatal(err)
	}
	if p := rv.Repos[0]; p.Evaluable || p.Reason != UnknownNoSince || p.Criteria == nil || len(p.Criteria) != 0 ||
		p.Milestone.Status != MilestonePresent {
		t.Errorf("no start date: %+v", p)
	}
	// The repository start date is enough.
	r.repos[0].ConventionsSince = "2026-09-02"
	if rv, _ := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", noSince, after); !rv.Repos[0].Evaluable ||
		rv.Repos[0].SinceDate != "2026-09-02" {
		t.Errorf("repository start date: %+v", rv.Repos[0])
	}
	r.repos[0].ConventionsSince = ""

	// Partial interval: every commit read conforms, but zz is absent from the database.
	r.commits[1][1].Message = "feat: ajoute des choses"
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if c := rv.Repos[0].Criteria[0]; c.Status != CriterionUnknown || c.Reason != UnknownPartial {
		t.Errorf("partial: %+v", c)
	}
	// Unreadable clone.
	cl.err = errors.New("rev-list failed")
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if c := rv.Repos[0].Criteria[0]; c.Reason != UnknownCloneUnreadable || rv.Repos[0].Interval != nil {
		t.Errorf("unreadable clone: %+v", c)
	}
	cl.err = nil
	// Tag absent, due date passed, but no tracked branch in the clone: unreadable, not "no commits".
	r.branches[1] = []model.Branch{{Name: "gone", Tracked: true}}
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v2.0.0", opts, after)
	if c := rv.Repos[0].Criteria[0]; c.Status != CriterionUnknown || c.Reason != UnknownCloneUnreadable {
		t.Errorf("tracked branches absent from the clone: %+v", c)
	}
	r.branches[1] = []model.Branch{{Name: "main", Tracked: true}}
	// No branch to evaluate.
	cl.remote = nil
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if c := rv.Repos[0].Criteria[1]; c.Status != CriterionUnknown || c.Reason != UnknownNoBranch {
		t.Errorf("no branch: %+v", c)
	}
	// Repository filter.
	r.repos = append(r.repos, model.Repository{ID: 2, Owner: "bob", Name: "tp"})
	if rv, err := ReviewMilestone(t.Context(), r, cl, Filter{RepoID: 2}, "v1.0.0", opts, after); err != nil ||
		len(rv.Repos) != 1 || rv.Repos[0].Repo.ID != 2 || rv.Repos[0].Milestone.Status != MilestoneAbsent {
		t.Errorf("filter: %+v, %v", rv, err)
	}
}

func TestReviewMilestoneErrors(t *testing.T) {
	boom := errors.New("boom")
	opts := reviewOptions(t)
	after := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for name, set := range map[string]func(*fakeReader){
		"repositories": func(r *fakeReader) { r.errRepos = boom },
		"tags":         func(r *fakeReader) { r.errTags = boom },
		"commits":      func(r *fakeReader) { r.errCommits = boom },
		"pulls":        func(r *fakeReader) { r.errPulls = boom; r.repos[0].ProviderID = 1 },
		"branches":     func(r *fakeReader) { r.errBranches = boom },
	} {
		r, cl := intervalFixture()
		set(r)
		if name == "branches" { // tag absent with a due date: tracked branches read in the database
			if _, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v2.0.0", opts, after); !errors.Is(err, boom) {
				t.Errorf("%s: %v", name, err)
			}
			continue
		}
		if _, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after); !errors.Is(err, boom) {
			t.Errorf("%s: %v", name, err)
		}
	}
	r, cl := intervalFixture()
	bad := opts
	bad.Since = "septembre"
	if _, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", bad, after); !errors.Is(err, ErrInvalidSince) {
		t.Errorf("invalid start date: %v", err)
	}
	bad = opts
	bad.Branch.Pattern = "("
	if _, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", bad, after); !errors.Is(err, ErrInvalidBranchPattern) {
		t.Errorf("invalid branch pattern: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cl.cancelRange = cancel
	if _, err := ReviewMilestone(ctx, r, cl, Filter{}, "v1.0.0", opts, after); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
}
