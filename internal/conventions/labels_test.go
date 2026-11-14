package conventions

import (
	"context"
	"errors"
	"testing"

	"github.com/studoo-app/watchoo/internal/model"
)

// labelReader is a fakeReader that knows the names given to the contributors (1.8.0).
type labelReader struct {
	*fakeReader
	labels map[int64]string
	err    error
}

func (l labelReader) ContributorLabels(context.Context) (map[int64]string, error) {
	return l.labels, l.err
}

func TestBuildLabels(t *testing.T) {
	r := buildFixture()
	r.pulls = map[int64][]model.ConventionPullRequest{1: {{
		PullRequest: model.PullRequest{Number: 3, Title: "feat: page", State: "open", HeadBranch: "feat/page"},
		AuthorID:    11, AuthorName: "bob"}}}
	// Without labels (a Reader that does not know them): nothing changes.
	plain, err := Build(t.Context(), r, nil, Filter{}, BuildOptions{Rules: DefaultRules()})
	if err != nil {
		t.Fatal(err)
	}
	byID := func(b BuildReport, id int64) ContributorReport {
		for _, c := range b.Contributors {
			if c.AuthorID == id {
				return c
			}
		}
		t.Fatalf("no contributor %d", id)
		return ContributorReport{}
	}
	if plain.Labels != nil || byID(plain, 10).DisplayAuthor() != "Alice" || byID(plain, 11).DisplayAuthor() != "bob" {
		t.Fatalf("without labels: %v, %+v", plain.Labels, plain.Contributors)
	}

	got, err := Build(t.Context(), labelReader{fakeReader: buildFixture(), labels: map[int64]string{11: "Zoé Durand", 10: "Anne Alice"}},
		nil, Filter{}, BuildOptions{Rules: DefaultRules()})
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by displayed name: "Anne Alice" before "Zoé Durand" (without labels, bob has no name and comes first).
	if bob, alice := byID(got, 11), byID(got, 10); bob.DisplayAuthor() != "Zoé Durand" || bob.AuthorLogin != "bob" ||
		alice.DisplayAuthor() != "Anne Alice" || alice.AuthorName != "Alice" {
		t.Errorf("contributors: %+v", got.Contributors)
	}
	order := func(b BuildReport) (ids []int64) {
		for _, c := range b.Contributors {
			if c.AuthorID != 0 {
				ids = append(ids, c.AuthorID)
			}
		}
		return ids
	}
	if o := order(got); len(o) != 2 || o[0] != 10 || o[1] != 11 {
		t.Errorf("order with labels: %v, want [10 11]", o)
	}
	if o := order(plain); len(o) != 2 || o[0] != 11 {
		t.Errorf("order without labels: %v, want bob (11) first", o)
	}
	// The evaluation is unchanged: same rates as without labels.
	if got.Total.Evaluated != plain.Total.Evaluated || got.Total.Conform != plain.Total.Conform {
		t.Errorf("total with labels %+v, without %+v", got.Total, plain.Total)
	}
	for _, c := range got.Repos[0].Commits {
		want := map[int64]string{10: "Anne Alice", 11: "Zoé Durand", 0: ""}[c.AuthorID]
		if c.AuthorLabel != want {
			t.Errorf("commit %s: label %q, want %q", c.SHA, c.AuthorLabel, want)
		}
	}
	if got.DisplayAuthor(11, "bob", "bob") != "Zoé Durand" || got.DisplayAuthor(99, "", "carol") != "carol" {
		t.Error("DisplayAuthor")
	}

	if _, err := Build(t.Context(), labelReader{fakeReader: buildFixture(), err: errors.New("boom")},
		nil, Filter{}, BuildOptions{Rules: DefaultRules()}); err == nil {
		t.Error("label read error ignored")
	}
}

func TestApplyLabelsLines(t *testing.T) {
	b := BuildReport{Labels: map[int64]string{7: "Gaël"}, Repos: []RepoReport{{
		Pulls:    PullReport{Pulls: []PullLine{{Number: 1, AuthorID: 7, AuthorLogin: "gael"}, {Number: 2, AuthorLogin: "x"}}},
		Arrivals: ArrivalReport{Lines: []ArrivalLine{{SHA: "a", AuthorID: 7, AuthorName: "g"}}},
		Branches: BranchReport{Branches: []BranchLine{{AuthorIDs: []int64{3, 7}, AuthorNames: []string{"c", "g"}, AuthorLogins: []string{"", ""}}}},
	}}}
	b.applyLabels()
	rep := b.Repos[0]
	if rep.Pulls.Pulls[0].DisplayAuthor() != "Gaël" || rep.Pulls.Pulls[1].DisplayAuthor() != "x" {
		t.Errorf("pulls: %+v", rep.Pulls.Pulls)
	}
	if rep.Arrivals.Lines[0].DisplayAuthor() != "Gaël" {
		t.Errorf("arrivals: %+v", rep.Arrivals.Lines)
	}
	br := rep.Branches.Branches[0]
	if br.AuthorLabel(0) != "" || br.AuthorLabel(1) != "Gaël" || br.AuthorLabel(5) != "" {
		t.Errorf("branch labels: %v", br.AuthorLabels)
	}
	if (BranchLine{}).AuthorLabel(0) != "" {
		t.Error("BranchLine without labels")
	}
}
