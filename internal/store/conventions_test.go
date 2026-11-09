package store

import (
	"slices"
	"testing"
	"time"
)

// ConventionCommits reads the commits with their message, parent count, author and committer, and
// their attachment to a tracked branch and to the default branch; author filter; newest first.
func TestConventionCommits(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "Alice@Example.test", "alice", 501)
	bob := person("Bob", "bob@example.test", "", 0)
	github := person("GitHub", "noreply@github.com", "web-flow", 19864447)
	both := []BranchState{{Name: "main", HeadSHA: sha(3)}, {Name: "dev", HeadSHA: sha(2)}}

	zero := 0
	c1, c2, c3 := cw(1, alice, "main", "dev"), cw(2, bob, "dev"), cw(3, alice, "main")
	c1.Committer, c1.ParentCount, c1.Message = github, &zero, "Initial commit"
	applySync(t, s, repoID, both, c1, c2, c3)

	got, err := s.ConventionCommits(ctx, repoID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var shas []string
	for _, c := range got {
		shas = append(shas, c.SHA)
	}
	if !slices.Equal(shas, []string{sha(3), sha(2), sha(1)}) {
		t.Fatalf("commits: %v", shas)
	}
	first := got[2]
	if first.Message != "Initial commit" || first.ParentCount == nil || *first.ParentCount != 0 ||
		first.AuthorName != "Alice" || first.AuthorLogin != "alice" || first.AuthorID == 0 ||
		!slices.Equal(first.AuthorEmails, []string{"alice@example.test"}) || first.CommitterLogin != "web-flow" ||
		!slices.Equal(first.CommitterEmails, []string{"noreply@github.com"}) || !first.OnTracked || !first.OnDefault ||
		!first.AuthoredAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("first commit: %+v", first)
	}
	if c := got[1]; c.ParentCount != nil || c.AuthorLogin != "" || !c.OnTracked || c.OnDefault {
		t.Errorf("commit of dev: %+v", c)
	}

	// dev is no longer tracked: its own commit leaves the tracked branches, its history stays.
	applySync(t, s, repoID, both[:1], c3)
	got, _ = s.ConventionCommits(ctx, repoID, 0)
	if c := got[1]; c.SHA != sha(2) || c.OnTracked {
		t.Errorf("after dev is untracked: %+v", c)
	}

	// Author filter.
	got, err = s.ConventionCommits(ctx, repoID, first.AuthorID)
	if err != nil || len(got) != 2 || got[0].SHA != sha(3) {
		t.Errorf("Alice's commits: %+v, %v", got, err)
	}
	if got, err := s.ConventionCommits(ctx, repoID+100, 0); err != nil || got == nil || len(got) != 0 {
		t.Errorf("unknown repository: %v, %v", got, err)
	}
}
