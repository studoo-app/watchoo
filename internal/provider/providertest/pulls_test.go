package providertest

import (
	"errors"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

func TestPullRequestsReleasesTags(t *testing.T) {
	f := New()
	f.AddRepository(provider.Repository{ID: 101, Owner: "alice", Name: "projet"})
	f.SetPullRequests(101,
		provider.PullRequest{Number: 1, State: provider.PullRequestOpen, UpdatedAt: t0.Add(time.Hour)},
		provider.PullRequest{Number: 2, State: provider.PullRequestClosed, Merged: true, UpdatedAt: t0.Add(3 * time.Hour)},
		provider.PullRequest{Number: 3, State: provider.PullRequestOpen, UpdatedAt: t0.Add(2*time.Hour + 500*time.Millisecond)},
	)
	got, err := f.PullRequests(ctx, "Alice", "Projet", provider.PullRequestsOptions{})
	if err != nil || len(got) != 3 || got[0].Number != 2 || got[2].Number != 1 {
		t.Fatalf("PullRequests: %+v, %v", got, err)
	}
	if !got[1].UpdatedAt.Equal(t0.Add(2 * time.Hour)) {
		t.Errorf("UpdatedAt not truncated to the second: %v", got[1].UpdatedAt)
	}
	got, _ = f.PullRequests(ctx, "alice", "projet", provider.PullRequestsOptions{UpdatedSince: t0.Add(2*time.Hour + 900*time.Millisecond)})
	if len(got) != 2 || got[1].Number != 3 {
		t.Errorf("UpdatedSince: %+v, want PRs 2 and 3", got)
	}
	got, _ = f.PullRequests(ctx, "alice", "projet", provider.PullRequestsOptions{Limit: 1})
	if len(got) != 1 {
		t.Errorf("Limit 1: %d PR", len(got))
	}
	if log := f.CallLog(); log[len(log)-1].PullOpts.Limit != 1 {
		t.Errorf("log without the options: %+v", log[len(log)-1])
	}

	f.SetReleases(101, provider.Release{ID: 1, TagName: "v1", CreatedAt: t0},
		provider.Release{ID: 2, TagName: "v2", CreatedAt: t0.Add(time.Hour)})
	releases, err := f.Releases(ctx, "alice", "projet")
	if err != nil || len(releases) != 2 || releases[0].ID != 2 {
		t.Errorf("Releases: %+v, %v", releases, err)
	}
	f.SetTags(101, provider.Tag{Name: "v2", SHA: SHA("b")}, provider.Tag{Name: "v1", SHA: SHA("a")})
	tags, err := f.Tags(ctx, "alice", "projet")
	if err != nil || len(tags) != 2 || tags[0].Name != "v2" {
		t.Errorf("Tags: %+v, %v", tags, err)
	}

	// Repository without anything: empty non-nil lists; unknown repository: ErrNotFound.
	f.AddRepository(provider.Repository{ID: 102, Owner: "bob", Name: "projet"})
	if p, err := f.PullRequests(ctx, "bob", "projet", provider.PullRequestsOptions{}); err != nil || p == nil || len(p) != 0 {
		t.Errorf("bob's PRs: %v, %v", p, err)
	}
	if r, err := f.Releases(ctx, "bob", "projet"); err != nil || r == nil {
		t.Errorf("bob's releases: %v, %v", r, err)
	}
	if tg, err := f.Tags(ctx, "bob", "projet"); err != nil || tg == nil {
		t.Errorf("bob's tags: %v, %v", tg, err)
	}
	if _, err := f.Tags(ctx, "carol", "projet"); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("unknown repository: %v", err)
	}

	// Injected error (e.g. a source without pull requests) and simulated quota.
	f.SetError(MethodPullRequests, provider.ErrUnsupported)
	if _, err := f.PullRequests(ctx, "alice", "projet", provider.PullRequestsOptions{}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("SetError ErrUnsupported: %v", err)
	}
	f.SetRateLimit(provider.RateLimit{Limit: 10, Remaining: 1})
	if _, err := f.Releases(ctx, "alice", "projet"); err != nil {
		t.Errorf("Releases with 1 remaining request: %v", err)
	}
	if _, err := f.Tags(ctx, "alice", "projet"); !errors.Is(err, provider.ErrRateLimited) {
		t.Errorf("Tags without quota: %v, want ErrRateLimited", err)
	}
	if f.Calls(MethodTags) != 4 {
		t.Errorf("%d Tags calls counted, want 4", f.Calls(MethodTags))
	}

	for name, fn := range map[string]func(){
		"PR without number":     func() { f.SetPullRequests(101, provider.PullRequest{State: "open", UpdatedAt: t0}) },
		"PR with unknown state": func() { f.SetPullRequests(101, provider.PullRequest{Number: 9, State: "merged", UpdatedAt: t0}) },
		"release without tag":   func() { f.SetReleases(101, provider.Release{ID: 1}) },
		"malformed tag":         func() { f.SetTags(101, provider.Tag{Name: "v1", SHA: "abc"}) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: want a panic", name)
				}
			}()
			fn()
		}()
	}
}

// Phase 8.6: PullRequestReviews (order kept, dates to the second, call log with the number, unknown PR,
// invalid number, injected error).
func TestPullRequestReviews(t *testing.T) {
	f := New()
	f.AddRepository(provider.Repository{ID: 102, Owner: "alice", Name: "projet"})
	f.SetPullRequests(102, provider.PullRequest{Number: 5, State: provider.PullRequestClosed, Merged: true, UpdatedAt: t0})
	f.SetPullRequestReviews(102, 5,
		provider.PullRequestReview{ID: 2, Reviewer: provider.Account{Login: "bob", ID: 7}, State: provider.ReviewApproved,
			SubmittedAt: t0.Add(500 * time.Millisecond)},
		provider.PullRequestReview{ID: 1, State: provider.ReviewPending},
	)
	got, err := f.PullRequestReviews(ctx, "Alice", "projet", 5)
	if err != nil || len(got) != 2 || got[0].ID != 2 || got[1].ID != 1 || !got[0].SubmittedAt.Equal(t0) {
		t.Fatalf("PullRequestReviews: %+v, %v", got, err)
	}
	if log := f.CallLog(); log[len(log)-1].Method != MethodPullReviews || log[len(log)-1].Number != 5 {
		t.Errorf("log: %+v", log[len(log)-1])
	}
	f.SetPullRequests(102, provider.PullRequest{Number: 6, State: provider.PullRequestOpen, UpdatedAt: t0})
	if got, err := f.PullRequestReviews(ctx, "alice", "projet", 6); err != nil || got == nil || len(got) != 0 {
		t.Errorf("PR without reviews: %+v, %v", got, err)
	}
	if _, err := f.PullRequestReviews(ctx, "alice", "projet", 9); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("unknown PR: %v", err)
	}
	if _, err := f.PullRequestReviews(ctx, "alice", "projet", 0); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("number 0: %v", err)
	}
	f.FailNext(MethodPullReviews, provider.ErrUnavailable)
	if _, err := f.PullRequestReviews(ctx, "alice", "projet", 5); !errors.Is(err, provider.ErrUnavailable) {
		t.Errorf("injected error: %v", err)
	}
	if n := f.Calls(MethodPullReviews); n != 5 {
		t.Errorf("%d calls, want 5", n)
	}
	for name, fn := range map[string]func(){
		"unknown PR": func() { f.SetPullRequestReviews(102, 9, provider.PullRequestReview{ID: 1, State: "APPROVED"}) },
		"no ID":      func() { f.SetPullRequestReviews(102, 5, provider.PullRequestReview{State: "APPROVED"}) },
		"no state":   func() { f.SetPullRequestReviews(102, 5, provider.PullRequestReview{ID: 1}) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			fn()
		}()
	}
}
