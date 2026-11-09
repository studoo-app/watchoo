package github

// Phase 8.5 (project conventions): fields read in the responses already requested, with no extra
// request: merge_commit_sha (merged PRs only), fork origin of the PR (head.repo / base.repo), body, and
// the merge methods of the repository (returned only to an account with the push right).

import (
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/github/githubtest"
)

func TestPullRequestsConventionFields(t *testing.T) {
	e := newScEnv(t)
	const id, name = 2101, "projet-conv"
	path := e.repo(id, name) + "/pulls"
	t0 := e.t0
	sha := strings.Repeat("ab", 20)
	e.srv.SetPullRequests(id,
		// Merged, same repository.
		githubtest.PullRequest{Number: 1, Closed: true, MergedAt: t0.Add(time.Hour), MergeCommitSHA: sha,
			Body: "## Contexte\nsecret", CreatedAt: t0, UpdatedAt: t0.Add(time.Hour)},
		// Open: GitHub's test merge commit is not kept.
		githubtest.PullRequest{Number: 2, MergeCommitSHA: strings.Repeat("cd", 20), Fork: true,
			CreatedAt: t0, UpdatedAt: t0.Add(2 * time.Hour)},
		// Merged from a deleted fork, malformed SHA (dropped).
		githubtest.PullRequest{Number: 3, Closed: true, MergedAt: t0.Add(3 * time.Hour), MergeCommitSHA: "abc",
			HeadRepoDeleted: true, CreatedAt: t0, UpdatedAt: t0.Add(3 * time.Hour)},
		// Merged without merge_commit_sha.
		githubtest.PullRequest{Number: 4, Closed: true, MergedAt: t0.Add(4 * time.Hour), CreatedAt: t0,
			UpdatedAt: t0.Add(4 * time.Hour)},
	)
	got, err := e.provider(testToken).PullRequests(t.Context(), scOwner, name, provider.PullRequestsOptions{})
	if err != nil {
		t.Fatalf("PullRequests: %v", err)
	}
	byNum := map[int]provider.PullRequest{}
	for _, pr := range got {
		byNum[pr.Number] = pr
	}
	if pr := byNum[1]; pr.MergeCommitSHA != sha || pr.FromFork != provider.False || pr.Body != "## Contexte\nsecret" {
		t.Errorf("merged PR: SHA %q, fork %v, body %q", pr.MergeCommitSHA, pr.FromFork, pr.Body)
	}
	if pr := byNum[2]; pr.MergeCommitSHA != "" || pr.FromFork != provider.True {
		t.Errorf("open PR from a fork: SHA %q (want empty), fork %v", pr.MergeCommitSHA, pr.FromFork)
	}
	if pr := byNum[3]; pr.MergeCommitSHA != "" || pr.FromFork != provider.True {
		t.Errorf("PR of a deleted fork: SHA %q (want empty), fork %v", pr.MergeCommitSHA, pr.FromFork)
	}
	if pr := byNum[4]; pr.MergeCommitSHA != "" || pr.FromFork != provider.False {
		t.Errorf("PR without merge_commit_sha: SHA %q, fork %v", pr.MergeCommitSHA, pr.FromFork)
	}
	if n := e.srv.Count(path); n != 1 {
		t.Errorf("%d requests on %s, want 1 (no extra request)", n, path)
	}

	// Minimal response without base.repo: origin unknown; head.repo without identifier: unknown.
	e.srv.Inject(path, githubtest.Response{Body: `[
		{"number":5,"state":"open","updated_at":"2026-10-01T00:00:00Z","head":{"ref":"x","repo":null}},
		{"number":6,"state":"open","updated_at":"2026-10-01T00:00:00Z","base":{"ref":"main","repo":{"id":7}},"head":{"ref":"x","repo":{"name":"r"}}}
	]`, Times: 1})
	got, err = e.provider(testToken).PullRequests(t.Context(), scOwner, name, provider.PullRequestsOptions{})
	if err != nil || len(got) != 2 {
		t.Fatalf("PullRequests (injected): %v, %v", got, err)
	}
	for _, pr := range got {
		if pr.FromFork != provider.Unknown {
			t.Errorf("PR #%d: fork %v, want Unknown", pr.Number, pr.FromFork)
		}
	}
}

func TestRepositoryMergeMethods(t *testing.T) {
	e := newScEnv(t)
	const id, name = 2102, "projet-merge"
	path := e.repo(id, name)
	p := e.provider(testToken)
	yes, no := true, false

	// Without the push right, GitHub does not return them: unknown, even if they are set.
	e.srv.SetMergeMethods(id, false, &yes, &no, &no)
	repo, err := p.Repository(t.Context(), scOwner, name)
	if err != nil {
		t.Fatalf("Repository: %v", err)
	}
	if repo.AllowSquashMerge != provider.Unknown || repo.AllowMergeCommit != provider.Unknown ||
		repo.AllowRebaseMerge != provider.Unknown {
		t.Errorf("without push: %v %v %v, want Unknown", repo.AllowSquashMerge, repo.AllowMergeCommit, repo.AllowRebaseMerge)
	}

	// With the push right: the values, false included (not confused with absent).
	e.srv.SetMergeMethods(id, true, &yes, &no, &no)
	for _, get := range []func() (provider.Repository, error){
		func() (provider.Repository, error) { return p.Repository(t.Context(), scOwner, name) },
		func() (provider.Repository, error) { return p.RepositoryByID(t.Context(), id) },
	} {
		repo, err := get()
		if err != nil {
			t.Fatalf("Repository: %v", err)
		}
		if repo.AllowSquashMerge != provider.True || repo.AllowMergeCommit != provider.False ||
			repo.AllowRebaseMerge != provider.False {
			t.Errorf("with push: %v %v %v, want True False False", repo.AllowSquashMerge, repo.AllowMergeCommit,
				repo.AllowRebaseMerge)
		}
	}
	// Only one of them returned: the others stay unknown.
	e.srv.SetMergeMethods(id, true, nil, &yes, nil)
	repo, err = p.Repository(t.Context(), scOwner, name)
	if err != nil || repo.AllowSquashMerge != provider.Unknown || repo.AllowMergeCommit != provider.True ||
		repo.AllowRebaseMerge != provider.Unknown {
		t.Errorf("partial: %+v, %v", repo, err)
	}
	if n := e.srv.Count(path); n != 3 {
		t.Errorf("%d requests on %s, want 3 (one per call)", n, path)
	}
}

func TestOptionalBool(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		in   *bool
		want provider.OptionalBool
	}{{nil, provider.Unknown}, {&yes, provider.True}, {&no, provider.False}}
	for _, c := range cases {
		got := provider.OptionalBoolOf(c.in)
		if got != c.want {
			t.Errorf("OptionalBoolOf(%v) = %v, want %v", c.in, got, c.want)
		}
		back := got.Ptr()
		if (back == nil) != (c.in == nil) || (back != nil && *back != *c.in) {
			t.Errorf("Ptr of %v = %v", got, back)
		}
	}
	if provider.KnownBool(true) != provider.True || provider.KnownBool(false) != provider.False {
		t.Error("KnownBool")
	}
}
