package sync_test

// Pull requests, releases and tags (V2): first pass, incremental sync of the PRs (UpdatedSince),
// replacement of releases and tags, isolated failure (warning, commits kept), low and exhausted
// quota, source without this notion (ErrUnsupported), cancellation.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// secretTitle is a confidential PR title: never in the logs or the reports.
const secretTitle = "Titre de PR confidentiel"

func init() { forbidden = append(forbidden, secretTitle) }

// fakePR returns a dummy PR n, updated at epoch + h hours, by the secret author.
func fakePR(n int, state string, h int) provider.PullRequest {
	at := epoch.Add(time.Duration(h) * time.Hour)
	pr := provider.PullRequest{
		Number: n, Title: secretTitle, State: state, URL: "https://github.example.test/pull",
		Author:    provider.Account{Login: secretLogin, ID: 4242},
		CreatedAt: epoch, UpdatedAt: at, BaseBranch: "main", HeadBranch: "feature",
	}
	if state == provider.PullRequestClosed {
		pr.ClosedAt = at
	}
	return pr
}

// pulls returns the PRs in the database of the repository repoID, from the most recently updated to the oldest.
func (e *env) pulls(repoID int64) []store.PullRequestView {
	e.t.Helper()
	views, err := e.st.ListPullRequests(e.t.Context(), store.PullRequestFilter{RepoID: repoID})
	if err != nil {
		e.t.Fatalf("ListPullRequests: %v", err)
	}
	return views
}

// pullCalls returns the options of the PullRequests calls received by the fake Provider.
func (e *env) pullCalls() []provider.PullRequestsOptions {
	var out []provider.PullRequestsOptions
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodPullRequests {
			out = append(out, c.PullOpts)
		}
	}
	return out
}

func TestPullRequestsIncremental(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)
	merged := fakePR(2, provider.PullRequestClosed, 3)
	merged.Merged, merged.MergedAt = true, merged.ClosedAt
	e.fake.SetPullRequests(1, fakePR(1, provider.PullRequestOpen, 1), merged)
	e.fake.SetReleases(1, provider.Release{ID: 7, TagName: "v1", CreatedAt: epoch, PublishedAt: epoch})
	e.fake.SetTags(1, provider.Tag{Name: "v1", SHA: providertest.SHA("x")})

	r := e.syncOK(repo.ID)
	if r.PullRequests != 2 || len(r.Warnings) != 0 || r.Warnings == nil {
		t.Errorf("first pass: %d PRs, warnings %v", r.PullRequests, r.Warnings)
	}
	if got := e.pulls(repo.ID); len(got) != 2 || got[0].Number != 2 || got[0].Status() != model.PullRequestStatusMerged ||
		got[1].AuthorLogin != secretLogin || got[1].AuthorProviderID != 4242 {
		t.Errorf("PRs in the database: %+v", got)
	}
	if calls := e.pullCalls(); len(calls) != 1 || !calls[0].UpdatedSince.IsZero() {
		t.Errorf("first call: %+v, want zero UpdatedSince", calls)
	}

	// Next sync: only the PRs updated since the most recent date in the database.
	reopened := fakePR(1, provider.PullRequestClosed, 5)
	e.fake.SetPullRequests(1, reopened, fakePR(3, provider.PullRequestOpen, 4))
	e.fake.ResetCalls()
	r = e.syncOK(repo.ID)
	if calls := e.pullCalls(); len(calls) != 1 || !calls[0].UpdatedSince.Equal(epoch.Add(3*time.Hour)) {
		t.Errorf("incremental call: %+v, want UpdatedSince = %v", calls, epoch.Add(3*time.Hour))
	}
	// PR 2 (same second as the resume point) is read again, without duplicate.
	if r.PullRequests != 3 {
		t.Errorf("incremental sync: %d PRs read, want 3", r.PullRequests)
	}
	got := e.pulls(repo.ID)
	if len(got) != 3 || got[0].Number != 1 || got[0].State != model.PullRequestClosed {
		t.Errorf("PRs after update: %+v", got)
	}

	// Releases and tags replaced: the release deleted on the forge disappears.
	e.fake.SetReleases(1)
	e.syncOK(repo.ID)
	if rel, _ := e.st.ListReleases(t.Context(), store.ReleaseFilter{RepoID: repo.ID}); len(rel) != 0 {
		t.Errorf("releases after deletion on the forge: %+v", rel)
	}
	if tags, _ := e.st.ListTags(t.Context(), store.ReleaseFilter{RepoID: repo.ID}); len(tags) != 1 {
		t.Errorf("tags: %+v", tags)
	}
}

// An error on a list does not make the repository fail: commits recorded, warning, nothing
// erased; the other lists are read; the next sync resumes at the same point.
func TestPullRequestsFailureIsolated(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.SetPullRequests(1, fakePR(1, provider.PullRequestOpen, 1))
	e.fake.SetReleases(1, provider.Release{ID: 7, TagName: "v1", CreatedAt: epoch})
	e.syncOK(repo.ID)

	e.fake.AppendCommits(1, "main", 1, epoch.Add(5*time.Hour), time.Hour)
	e.fake.SetPullRequests(1, fakePR(2, provider.PullRequestOpen, 6))
	e.fake.SetReleases(1, provider.Release{ID: 8, TagName: "v2", CreatedAt: epoch})
	unavailable := &provider.APIError{Op: "list of pull requests of promo-demo/projet", Method: "GET",
		Path: "/repos/promo-demo/projet/pulls", Status: 502, Err: provider.ErrUnavailable}
	e.fake.FailNext(providertest.MethodPullRequests, unavailable)
	e.fake.FailNext(providertest.MethodTags, &provider.APIError{Status: 404, Err: provider.ErrNotFound})
	r := e.syncOK(repo.ID)
	if r.NewCommits != 1 {
		t.Errorf("new commits: %d, want 1 (commits kept despite the failure of the PRs)", r.NewCommits)
	}
	if len(r.Warnings) != 2 || !strings.HasPrefix(r.Warnings[0].Message, "pull requests not synced: ") ||
		!strings.Contains(r.Warnings[0].Message, "HTTP 502") || !strings.HasPrefix(r.Warnings[1].Message, "tags not synced: ") {
		t.Errorf("warnings: %q", r.Warnings)
	}
	if got := e.pulls(repo.ID); len(got) != 1 || got[0].Number != 1 {
		t.Errorf("PRs in the database after the failure: %+v (kept, without the new one)", got)
	}
	if rel, _ := e.st.ListReleases(t.Context(), store.ReleaseFilter{RepoID: repo.ID}); len(rel) != 1 || rel[0].TagName != "v2" {
		t.Errorf("releases read despite the failure of the PRs: %+v", rel)
	}
	if run := e.lastRun(repo.ID); run.Status != model.SyncSuccess {
		t.Errorf("sync_run = %+v, want success", run)
	}
	if !strings.Contains(e.logs.String(), "liste non synchronisée") { // message of the Warn log
		t.Error("failure not logged")
	}

	// Next sync: resumes at the same point, PR 2 is fetched.
	e.fake.ResetCalls()
	r = e.syncOK(repo.ID)
	if len(r.Warnings) != 0 || len(e.pulls(repo.ID)) != 2 {
		t.Errorf("resume: warnings %q, PRs %+v", r.Warnings, e.pulls(repo.ID))
	}
	if calls := e.pullCalls(); len(calls) != 1 || !calls[0].UpdatedSince.Equal(epoch.Add(time.Hour)) {
		t.Errorf("resume: %+v", calls)
	}
}

// Source without pull requests or releases (future local Git provider): ignored without warning,
// nothing erased, the tags are read.
func TestPullRequestsUnsupported(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.SetReleases(1, provider.Release{ID: 7, TagName: "v1", CreatedAt: epoch})
	e.syncOK(repo.ID)

	e.fake.SetError(providertest.MethodPullRequests, provider.ErrUnsupported)
	e.fake.SetError(providertest.MethodReleases, provider.ErrUnsupported)
	e.fake.SetTags(1, provider.Tag{Name: "v1", SHA: providertest.SHA("x")})
	r := e.syncOK(repo.ID)
	if len(r.Warnings) != 0 {
		t.Errorf("warnings for a source without PRs: %q", r.Warnings)
	}
	if rel, _ := e.st.ListReleases(t.Context(), store.ReleaseFilter{}); len(rel) != 1 {
		t.Errorf("releases erased: %+v", rel)
	}
	if tags, _ := e.st.ListTags(t.Context(), store.ReleaseFilter{}); len(tags) != 1 {
		t.Errorf("tags not read: %+v", tags)
	}
}

// Low quota: PRs, releases and tags not requested; quota exhausted during the read: repository
// succeeded, remaining lists not read, following repositories deferred.
func TestPullRequestsQuota(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	e.addRepo(1, "a")
	e.addRepo(2, "b")
	for _, id := range []int64{1, 2} {
		e.fake.AppendCommits(id, "main", 1, epoch, time.Hour)
	}
	reset := time.Date(2026, 10, 20, 11, 5, 0, 0, time.UTC)
	e.fake.FailNext(providertest.MethodReleases, &provider.APIError{Status: 403, Err: &provider.RateLimitError{Reset: reset}})
	rep := e.syncAll()
	first := rep.Repos[0]
	if first.Status != wsync.StatusSuccess || len(first.Warnings) != 1 ||
		!strings.HasPrefix(first.Warnings[0].Message, "releases and tags not synced: ") {
		t.Errorf("first repository: %+v", first)
	}
	if n := e.fake.Calls(providertest.MethodTags); n != 0 {
		t.Errorf("Tags called %d times after the exhausted quota", n)
	}
	if second := rep.Repos[1]; second.Status != wsync.StatusDeferred || !second.ResumeAt.Equal(reset) {
		t.Errorf("next repository: %+v", second)
	}

	// Low quota before the lists (but not before the repository): nothing is requested.
	e2 := newEnv(t)
	repo := e2.addRepo(1, "a")
	e2.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e2.fake.SetRateLimit(provider.RateLimit{Limit: 100, Remaining: 12}) // 9 after repository, branches and commits
	r := e2.syncOK(repo.ID)
	if n := e2.fake.Calls(providertest.MethodPullRequests) + e2.fake.Calls(providertest.MethodReleases) +
		e2.fake.Calls(providertest.MethodTags); n != 0 {
		t.Errorf("%d lists requested under the low quota", n)
	}
	if len(r.Warnings) != 1 || r.Warnings[0].Message != "pull requests, releases and tags not synced: low quota" {
		t.Errorf("warnings: %q", r.Warnings)
	}
}

// Cancellation during the read of the PRs: the repository fails (sync canceled), nothing is written.
func TestPullRequestsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := newEnv(t, withWrap(func(p provider.Provider) provider.Provider {
		return &pullsHook{Provider: p, before: cancel}
	}))
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	rep, err := e.eng.SyncRepo(ctx, repo.ID, wsync.SyncOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("SyncRepo: %v, want context.Canceled", err)
	}
	if r := rep.Repos[0]; r.Status != wsync.StatusError || r.Error != "sync canceled" {
		t.Errorf("report: %+v", r)
	}
	if got := e.commitSHAs(repo.ID, ""); len(got) != 0 {
		t.Errorf("commits written despite the cancellation: %d", len(got))
	}
}

// pullsHook calls before before PullRequests.
type pullsHook struct {
	provider.Provider
	before func()
}

func (h *pullsHook) PullRequests(ctx context.Context, owner, name string, opts provider.PullRequestsOptions) ([]provider.PullRequest, error) {
	h.before()
	return h.Provider.PullRequests(ctx, owner, name, opts)
}
