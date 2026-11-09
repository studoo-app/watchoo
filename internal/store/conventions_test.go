package store

import (
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
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

// ConventionPullRequests reads the pull requests of one repository, newest number first, with the
// contributor of the same forge account (0 when none); ConventionTags and ConventionReleases read the
// tags (by name) and releases (drafts included) of one repository.
func TestConventionPullsTagsReleases(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(1)}},
		cw(1, person("Alice", "alice@example.test", "alice", 501), "main"))

	mustExec(t, s, `INSERT INTO pull_requests (repo_id, number, title, state, merged, draft, author_login,
		author_provider_id, head_branch, created_at, updated_at) VALUES (?, ?, ?, 'open', 0, 1, ?, ?, ?, ?, ?)`,
		repoID, 1, "feat: one", "alice", 501, "feat/one-thing", testDate, testDate)
	mustExec(t, s, `INSERT INTO pull_requests (repo_id, number, title, state, merged, author_login,
		author_provider_id, head_branch, created_at, updated_at, closed_at, merged_at)
		VALUES (?, ?, ?, 'closed', 1, ?, ?, ?, ?, ?, ?, ?)`,
		repoID, 2, "Fix", "carol", 777, "fix", testDate, testDate, testDate, testDate)
	mustExec(t, s, `INSERT INTO pull_requests (repo_id, number, state, created_at, updated_at)
		VALUES (?, 1, 'open', ?, ?)`, other, testDate, testDate)

	pulls, err := s.ConventionPullRequests(ctx, repoID)
	if err != nil || len(pulls) != 2 {
		t.Fatalf("ConventionPullRequests = %+v, %v", pulls, err)
	}
	if p := pulls[0]; p.Number != 2 || !p.Merged || p.AuthorID != 0 || p.AuthorName != "" || p.HeadBranch != "fix" ||
		p.Title != "Fix" || p.MergedAt.IsZero() {
		t.Errorf("PR 2: %+v", p)
	}
	if p := pulls[1]; p.Number != 1 || !p.Draft || p.AuthorID == 0 || p.AuthorName != "Alice" ||
		p.HeadBranch != "feat/one-thing" || p.Status() != model.PullRequestStatusOpen {
		t.Errorf("PR 1: %+v", p)
	}
	if got, err := s.ConventionPullRequests(ctx, 999); err != nil || got == nil || len(got) != 0 {
		t.Errorf("unknown repository: %v, %v", got, err)
	}

	mustExec(t, s, insertTag, repoID, "v1.0.0", sha(1))
	mustExec(t, s, insertTag, repoID, "1.1.0", sha(9))
	mustExec(t, s, insertTag, other, "v9.0.0", sha(9))
	tags, err := s.ConventionTags(ctx, repoID)
	if err != nil || len(tags) != 2 || tags[0].Name != "1.1.0" || tags[1].Name != "v1.0.0" || tags[1].SHA != sha(1) {
		t.Errorf("ConventionTags = %+v, %v", tags, err)
	}
	mustExec(t, s, `INSERT INTO releases (repo_id, provider_id, tag_name, draft, prerelease, created_at)
		VALUES (?, 10, 'v1.0.0', 1, 1, ?)`, repoID, testDate)
	rels, err := s.ConventionReleases(ctx, repoID)
	if err != nil || len(rels) != 1 || rels[0].TagName != "v1.0.0" || !rels[0].Draft || !rels[0].Prerelease {
		t.Errorf("ConventionReleases = %+v, %v", rels, err)
	}
	if got, err := s.ConventionReleases(ctx, other); err != nil || got == nil || len(got) != 0 {
		t.Errorf("releases of the other repository: %v, %v", got, err)
	}
}
