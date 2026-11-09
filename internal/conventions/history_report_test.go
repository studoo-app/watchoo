package conventions

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// historyClones adds HistoryReader to fakeClones: ancestry read from reach (a commit absent from reach is
// an error, like a commit absent from the clone), last commit touching the CHANGELOG per head.
type historyClones struct {
	*fakeClones
	touch    map[string]string // head -> last commit touching the CHANGELOG
	touchErr error
	asked    int
}

func (h *historyClones) IsAncestor(_ context.Context, _, _, ancestor, descendant string) (bool, error) {
	h.asked++
	set, ok := h.reach[descendant]
	if _, known := h.reach[ancestor]; !ok || !known {
		return false, errors.New("bad object")
	}
	return slices.Contains(set, ancestor), nil
}

func (h *historyClones) LastTouch(_ context.Context, _, _, head, file string) (string, bool, error) {
	if file != ChangelogFile {
		return "", false, errors.New("unexpected path")
	}
	if h.touchErr != nil {
		return "", false, h.touchErr
	}
	sha, ok := h.touch[head]
	return sha, ok, nil
}

func historyFixture(t *testing.T) (*fakeReader, *historyClones, BuildOptions) {
	t.Helper()
	r, cl := intervalFixture()
	r.branches[1] = []model.Branch{{Name: "main", Tracked: true, HeadSHA: "c5"}}
	r.tags[1] = []model.Tag{{Name: "v0.1.0", SHA: "c2"}, {Name: "v1.0.0", SHA: "c4"}, {Name: "v1.0.1", SHA: "c5"},
		{Name: "v1.0.1-rc1", SHA: "c4"}, {Name: "v1.1.0-rc.1", SHA: "c5"}}
	at := func(d int) time.Time { return time.Date(2026, 9, d, 8, 0, 0, 0, time.UTC) }
	ended := at(12)
	r.obs = map[int64][]model.TagObservation{1: {
		{RepoID: 1, Name: "v0.1.0", SHA: "c2", FirstSeenAt: at(6), LastSeenAt: at(25)},
		// v1.0.0 seen for the first time after its due date (September 10), a read on September 10 did not see it.
		{RepoID: 1, Name: "v1.0.0", SHA: "c3", FirstSeenAt: at(11), LastSeenAt: at(11), EndedAt: &ended,
			EndReason: model.TagEndRewritten},
		{RepoID: 1, Name: "v1.0.0", SHA: "c4", FirstSeenAt: at(12), LastSeenAt: at(25)},
		{RepoID: 1, Name: "v1.0.1", SHA: "c5", FirstSeenAt: at(10), LastSeenAt: at(25)},
	}}
	r.rewrites = map[int64][]model.BranchRewrite{1: {
		{RepoID: 1, Branch: "main", OldSHA: "old1", NewSHA: "c4", ObservedAt: at(9)},
		{RepoID: 1, Branch: "main", OldSHA: "old2", NewSHA: "c5", ObservedAt: at(20)},
	}}
	r.tagDates = map[int64]map[string]time.Time{1: {"v1.0.0": day(8), "v1.0.1": day(9), "v0.1.0": day(6)}}
	opts := forgeOptions()
	ms, err := ParseMilestones([]string{"v0.1.0", "v1.0.0@2026-09-10", "v1.0.1@2026-09-10"})
	if err != nil {
		t.Fatal(err)
	}
	opts.Milestones = ms
	return r, &historyClones{fakeClones: cl, touch: map[string]string{"c5": "c3"}}, opts
}

func TestBuildHistory(t *testing.T) {
	r, cl, opts := historyFixture(t)
	rep, err := Build(t.Context(), r, cl, Filter{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	p := rep.Repos[0]

	// BR4: most recent first, without attribution.
	if rw := p.Rewrites; !rw.Known || rw.Count != 2 || rw.Events[0].OldSHA != "old2" || !rw.Last.Equal(rw.Events[0].ObservedAt) {
		t.Errorf("BR4: %+v", rw)
	}
	// SV3: v1.0.0 rewritten, the others stable.
	if h := p.TagHistory; !h.Observed || h.Counts[TagHistoryRewritten] != 1 || h.Counts[TagHistoryStable] != 2 ||
		!h.Since.Equal(time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("SV3: %+v", h)
	}
	// SV2 deadlines: v0.1.0 has no due date; v1.0.0 late (a read on September 10 did not see it); v1.0.1
	// first seen on September 10, before the end of the due date (midnight the next day).
	ms := p.Tags.Milestones
	if ms[0].Deadline.Status != "" || ms[1].Deadline.Status != DeadlineLate || ms[2].Deadline.Status != DeadlineOnTime ||
		!ms[1].Deadline.CommitAt.Equal(day(8)) {
		t.Errorf("deadlines: %+v", ms)
	}
	// SV4: free form, never an error.
	if pr := p.Prereleases; !pr.Known || pr.Checked != 2 || pr.Conform != 1 || len(pr.Notes) != 1 ||
		pr.Notes[0] != (VersionNote{Code: PrereleaseFreeForm, Tag: "v1.0.1-rc1"}) {
		t.Errorf("SV4: %+v", pr)
	}
	// SV7: consistent history; v0.1.0 -> v1.0.0 is a single increment.
	if pc := p.Precedence; !pc.Known || pc.Pairs != 2 || !pc.Conform() || len(pc.Notes) != 0 {
		t.Errorf("SV7: %+v", pc)
	}
	// SV5: v0.1.0..v1.0.0 has a commit missing from the database (zz): undetermined; v1.0.0..v1.0.1
	// brings a "feat" (c5) with only a PATCH: under-versioned.
	b := p.Bumps
	if !b.Known || len(b.Lines) != 2 || b.Lines[0].Status != BumpUndetermined || b.Lines[0].Reason != UnknownPartial ||
		b.Lines[1].Status != BumpUnder || b.Lines[1].Expected != BumpMinor || b.Lines[1].Actual != BumpPatch ||
		b.Counts[BumpUnder] != 1 {
		t.Errorf("SV5: %+v", b)
	}
	// CL10: since c3 (last commit touching the CHANGELOG): c4 nonconforming, c5 "feat", zz missing.
	if f := p.Freshness; !f.Known || f.Since != "c3" || f.Feat != 1 || f.Fix != 0 || f.Missing != 1 || f.Grayed {
		t.Errorf("CL10: %+v", f)
	}

	// Ancestry read once per pair.
	asked := cl.asked
	if _, err := Build(t.Context(), r, cl, Filter{}, opts); err != nil || cl.asked-asked != asked {
		t.Errorf("ancestry reads: %d then %d", asked, cl.asked-asked)
	}
}

func TestBuildHistoryUnknown(t *testing.T) {
	r, cl, opts := historyFixture(t)

	// Restricted to contributors: the clone indicators are not computed.
	rep, err := Build(t.Context(), r, cl, Filter{AuthorIDs: []int64{10}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	p := rep.Repos[0]
	if p.Prereleases.Known || p.Prereleases.Reason != UnknownFiltered || p.Bumps.Known || p.Bumps.Reason != UnknownFiltered ||
		p.Precedence.Known || p.Precedence.Reason != UnknownFiltered || p.Freshness.Known || p.Freshness.Reason != UnknownFiltered ||
		p.Bumps.Lines == nil || p.Prereleases.Notes == nil || p.Precedence.Notes == nil {
		t.Errorf("filtered: %+v %+v %+v %+v", p.Prereleases, p.Bumps, p.Precedence, p.Freshness)
	}

	// A clone reader without history: like no clone.
	rep, _ = Build(t.Context(), r, cl.fakeClones, Filter{}, opts)
	p = rep.Repos[0]
	if p.Bumps.Known || p.Bumps.Reason != UnknownNoClone || p.Precedence.Known || p.Freshness.Reason != UnknownNoClone {
		t.Errorf("without history: %+v %+v %+v", p.Bumps, p.Precedence, p.Freshness)
	}

	// No observation: SV3 unknown, deadlines undetermined.
	r.obs, r.rewrites = nil, nil
	rep, _ = Build(t.Context(), r, cl, Filter{}, opts)
	p = rep.Repos[0]
	if p.TagHistory.Observed || p.TagHistory.Reason != UnknownNotObserved || p.Rewrites.Count != 0 || !p.Rewrites.Known ||
		p.Rewrites.Events == nil || p.Tags.Milestones[1].Deadline.Status != DeadlineUndetermined {
		t.Errorf("no observation: %+v %+v %+v", p.TagHistory, p.Rewrites, p.Tags.Milestones[1].Deadline)
	}

	// CHANGELOG never touched, then unreadable, then default head unknown.
	cl.touch = map[string]string{}
	if rep, _ = Build(t.Context(), r, cl, Filter{}, opts); rep.Repos[0].Freshness.Reason != UnknownNoFile {
		t.Errorf("no CHANGELOG: %+v", rep.Repos[0].Freshness)
	}
	cl.touchErr = errors.New("git")
	if rep, _ = Build(t.Context(), r, cl, Filter{}, opts); rep.Repos[0].Freshness.Reason != UnknownCloneUnreadable {
		t.Errorf("unreadable: %+v", rep.Repos[0].Freshness)
	}
	cl.touchErr = nil
	r.branches[1] = []model.Branch{{Name: "main", Tracked: true}}
	if rep, _ = Build(t.Context(), r, cl, Filter{}, opts); rep.Repos[0].Freshness.Reason != UnknownCloneUnreadable {
		t.Errorf("head unknown: %+v", rep.Repos[0].Freshness)
	}

	// Database errors stop the computation.
	r.errHistory = errors.New("db")
	if _, err := Build(t.Context(), r, cl, Filter{}, opts); err == nil {
		t.Error("database error ignored")
	}
	opts.Since = "2026-09-01"
	if _, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, time.Time{}); err == nil {
		t.Error("database error ignored by the review")
	}
}

func TestReviewMilestoneHistory(t *testing.T) {
	r, cl, opts := historyFixture(t)
	opts.Since = "2026-09-01"
	after := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	crit := func(rv MilestoneReview, id string) string {
		i := slices.IndexFunc(rv.Repos[0].Criteria, func(c Criterion) bool { return c.ID == id })
		c := rv.Repos[0].Criteria[i]
		return c.Status + ":" + c.Reason
	}

	// v1.0.0: late and rewritten.
	rv, err := ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if err != nil {
		t.Fatal(err)
	}
	if crit(rv, CriterionSV2) != "nonconform:" || crit(rv, CriterionSV3) != "nonconform:" ||
		rv.Repos[0].Milestone.Deadline.Status != DeadlineLate || rv.Repos[0].History == nil ||
		rv.Repos[0].History.Status != TagHistoryRewritten {
		t.Errorf("v1.0.0: %v %+v", statuses(rv.Repos[0].Criteria), rv.Repos[0].History)
	}
	// v1.0.1: on time, no rewrite observed.
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.1", opts, after)
	if crit(rv, CriterionSV2) != "conform:" || crit(rv, CriterionSV3) != "conform:" {
		t.Errorf("v1.0.1: %v", statuses(rv.Repos[0].Criteria))
	}
	// Undetermined: no read between the commit and the end of the due date.
	r.obs[1] = slices.DeleteFunc(r.obs[1], func(o model.TagObservation) bool { return o.Name == "v0.1.0" || o.Name == "v1.0.1" })
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.0", opts, after)
	if crit(rv, CriterionSV2) != "unknown:"+UnknownDeadline {
		t.Errorf("undetermined: %v", statuses(rv.Repos[0].Criteria))
	}
	// Tag never observed: SV3 unknown.
	rv, _ = ReviewMilestone(t.Context(), r, cl, Filter{}, "v1.0.1", opts, after)
	if crit(rv, CriterionSV3) != "unknown:"+UnknownNotObserved || rv.Repos[0].History != nil {
		t.Errorf("not observed: %v", statuses(rv.Repos[0].Criteria))
	}
}
