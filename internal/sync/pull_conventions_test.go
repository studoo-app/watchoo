package sync_test

// Conventions, phase 8.5: fields read in the responses already requested (merge_commit_sha, fork origin,
// mask of the body sections, merge methods), with no extra request in steady state; one full reread of
// the PR list when the conventions are on and the repository marker is missing; the body is never kept
// in the database nor written in the logs.

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/github"
	"github.com/studoo-app/watchoo/internal/provider/github/githubtest"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/repoimport"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// secretBody is a confidential PR body marker: never in the logs, the reports nor the database.
const secretBody = "Corps de PR confidentiel"

func init() { forbidden = append(forbidden, secretBody) }

// convBody is a PR body with the four sections and a closing keyword.
const convBody = "## Contexte\n" + secretBody + "\n\n## Ce qui change\nTout.\n\n## Comment tester\n`go test`\n\n" +
	"## Captures\nAucune.\n\nCloses #12\n"

// convPRs returns three PRs: 1 merged (squash SHA, full body), 2 open from a fork, 3 closed without merge.
func convPRs(h int) []provider.PullRequest {
	merged := fakePR(1, provider.PullRequestClosed, h)
	merged.Merged, merged.MergedAt = true, merged.ClosedAt
	merged.MergeCommitSHA, merged.FromFork, merged.Body = providertest.SHA("squash-1"), provider.False, convBody
	fork := fakePR(2, provider.PullRequestOpen, h+1)
	fork.FromFork, fork.MergeCommitSHA = provider.True, providertest.SHA("test-merge") // open: SHA dropped
	closed := fakePR(3, provider.PullRequestClosed, h+2)
	return []provider.PullRequest{merged, fork, closed}
}

// convPulls returns the PRs of the repository repoID as read by the conventions, by number.
func (e *env) convPulls(repoID int64) map[int]model.ConventionPullRequest {
	e.t.Helper()
	list, err := e.st.ConventionPullRequests(e.t.Context(), repoID)
	if err != nil {
		e.t.Fatalf("ConventionPullRequests: %v", err)
	}
	out := make(map[int]model.ConventionPullRequest, len(list))
	for _, pr := range list {
		out[pr.Number] = pr
	}
	return out
}

// prsVersion returns the marker of the full reread of the repository repoID.
func (e *env) prsVersion(repoID int64) int {
	e.t.Helper()
	r, err := e.st.GetRepository(e.t.Context(), repoID)
	if err != nil {
		e.t.Fatalf("GetRepository: %v", err)
	}
	return r.ConventionsPRsVersion
}

// assertBodyNotStored checks that the body marker is in no file of the database.
func (e *env) assertBodyNotStored() {
	e.t.Helper()
	for _, suffix := range []string{"", "-wal"} {
		b, err := os.ReadFile(e.dbPath + suffix)
		if err != nil {
			continue
		}
		if strings.Contains(string(b), secretBody) {
			e.t.Errorf("PR body found in %s", e.dbPath+suffix)
		}
	}
}

// checkConvFields checks the phase 8.5 fields of the PRs of convPRs in the database.
func checkConvFields(t *testing.T, got map[int]model.ConventionPullRequest) {
	t.Helper()
	all := int64(conventions.PRBodyContext | conventions.PRBodyChanges | conventions.PRBodyTesting |
		conventions.PRBodyScreenshots | conventions.PRBodyCloses)
	m := got[1]
	if m.MergeCommitSHA != providertest.SHA("squash-1") || m.FromFork == nil || *m.FromFork ||
		m.BodySections == nil || *m.BodySections != all {
		t.Errorf("merged PR: SHA %q, fork %v, sections %v", m.MergeCommitSHA, m.FromFork, m.BodySections)
	}
	f := got[2]
	if f.MergeCommitSHA != "" || f.FromFork == nil || !*f.FromFork || f.BodySections == nil || *f.BodySections != 0 {
		t.Errorf("open PR from a fork: SHA %q, fork %v, sections %v", f.MergeCommitSHA, f.FromFork, f.BodySections)
	}
	if c := got[3]; c.FromFork != nil || c.MergeCommitSHA != "" {
		t.Errorf("closed PR of unknown origin: SHA %q, fork %v", c.MergeCommitSHA, c.FromFork)
	}
}

// Conventions on from the first sync: the first read is already complete (marker set), the next sync
// is incremental with no extra request.
func TestPullConventionsFirstPass(t *testing.T) {
	e := newEnv(t, withConventions())
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.SetPullRequests(1, convPRs(1)...)

	r := e.syncOK(repo.ID)
	if r.PullRequests != 3 {
		t.Errorf("first pass: %d PRs, want 3", r.PullRequests)
	}
	checkConvFields(t, e.convPulls(repo.ID))
	if v := e.prsVersion(repo.ID); v != conventions.PRRulesVersion {
		t.Errorf("marker %d, want %d", v, conventions.PRRulesVersion)
	}

	e.fake.ResetCalls()
	e.syncOK(repo.ID)
	if calls := e.pullCalls(); len(calls) != 1 || calls[0].UpdatedSince.IsZero() {
		t.Errorf("steady state: %+v, want one incremental call", calls)
	}
	e.assertBodyNotStored()
}

// Conventions off: the fields are filled at insertion (they cost nothing) but no full reread; turned on
// later: one full reread fills the PRs already in the database, then incremental again.
func TestPullConventionsReread(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	old := convPRs(1)
	// Read without the phase 8.5 fields, as before the phase (same update date afterwards).
	bare := make([]provider.PullRequest, len(old))
	for i, pr := range old {
		pr.MergeCommitSHA, pr.FromFork, pr.Body = "", provider.Unknown, ""
		bare[i] = pr
	}
	e.fake.SetPullRequests(1, bare...)
	e.syncOK(repo.ID)
	if got := e.convPulls(repo.ID)[1]; got.MergeCommitSHA != "" || got.FromFork != nil ||
		got.BodySections == nil || *got.BodySections != 0 {
		t.Errorf("before the reread: %+v", got.PullRequest)
	}

	// Conventions off: incremental, no marker.
	e.fake.ResetCalls()
	e.syncOK(repo.ID)
	if calls := e.pullCalls(); len(calls) != 1 || calls[0].UpdatedSince.IsZero() {
		t.Errorf("conventions off: %+v, want one incremental call", calls)
	}
	if v := e.prsVersion(repo.ID); v != 0 {
		t.Errorf("marker %d with the conventions off, want 0", v)
	}

	// Conventions on: one full read, which replaces the incremental read.
	e.fake.SetPullRequests(1, old...)
	newer := fakePR(4, provider.PullRequestOpen, 10)
	newer.Body = convBody
	e.fake.SetPullRequests(1, newer)
	e.cfg.Conventions.Enabled = true
	e.eng = e.newEngine(e.st)
	e.fake.ResetCalls()
	r := e.syncOK(repo.ID)
	if calls := e.pullCalls(); len(calls) != 1 || !calls[0].UpdatedSince.IsZero() {
		t.Errorf("reread: %+v, want one call without UpdatedSince", calls)
	}
	// PR 3 (same second as the resume point) and PR 4 are written in full; 1 and 2 only completed.
	if r.PullRequests != 2 {
		t.Errorf("reread: %d PRs counted, want 2 (only the updated ones)", r.PullRequests)
	}
	got := e.convPulls(repo.ID)
	checkConvFields(t, got)
	if len(got) != 4 || got[1].Title != secretTitle {
		t.Errorf("PRs after the reread: %d, title kept %v", len(got), got[1].Title == secretTitle)
	}
	if v := e.prsVersion(repo.ID); v != conventions.PRRulesVersion {
		t.Errorf("marker %d after the reread, want %d", v, conventions.PRRulesVersion)
	}

	e.fake.ResetCalls()
	e.syncOK(repo.ID)
	if calls := e.pullCalls(); len(calls) != 1 || calls[0].UpdatedSince.IsZero() {
		t.Errorf("after the reread: %+v, want one incremental call", calls)
	}
	e.assertBodyNotStored()
}

// Interrupted reread (exhausted quota): marker not set, retried at the next sync.
func TestPullConventionsRereadInterrupted(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.SetPullRequests(1, convPRs(1)...)
	e.syncOK(repo.ID)

	e.cfg.Conventions.Enabled = true
	e.eng = e.newEngine(e.st)
	reset := time.Date(2026, 10, 20, 11, 5, 0, 0, time.UTC)
	e.fake.FailNext(providertest.MethodPullRequests, &provider.APIError{Status: 403, Err: &provider.RateLimitError{Reset: reset}})
	r := e.syncOK(repo.ID)
	if len(r.Warnings) != 1 {
		t.Errorf("warnings: %+v", r.Warnings)
	}
	if v := e.prsVersion(repo.ID); v != 0 {
		t.Errorf("marker %d after an interrupted reread, want 0", v)
	}

	e.fake.ResetCalls()
	e.syncOK(repo.ID)
	if calls := e.pullCalls(); len(calls) != 1 || !calls[0].UpdatedSince.IsZero() {
		t.Errorf("retry: %+v, want one call without UpdatedSince", calls)
	}
	if v := e.prsVersion(repo.ID); v != conventions.PRRulesVersion {
		t.Errorf("marker %d after the retry, want %d", v, conventions.PRRulesVersion)
	}
}

// List at the cap (possibly truncated): marker not set.
func TestPullConventionsRereadTruncated(t *testing.T) {
	e := newEnv(t, withConventions())
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	pulls := make([]provider.PullRequest, 0, provider.MaxPullRequests)
	for n := 1; n <= provider.MaxPullRequests; n++ {
		pulls = append(pulls, fakePR(n, provider.PullRequestOpen, 1))
	}
	e.fake.SetPullRequests(1, pulls...)
	e.syncOK(repo.ID)
	if v := e.prsVersion(repo.ID); v != 0 {
		t.Errorf("marker %d after a truncated list, want 0", v)
	}
}

// Merge methods: read in GET /repos (no extra request), NULL when unknown; the loss of the push right
// makes them unknown again.
func TestPullConventionsMergeMethods(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	meta := provider.Repository{ID: 1, Owner: testOwner, Name: "projet", DefaultBranch: "main",
		URL: "https://github.com/" + testOwner + "/projet", Private: true,
		AllowSquashMerge: provider.True, AllowMergeCommit: provider.False, AllowRebaseMerge: provider.False}
	e.fake.UpdateRepository(meta)
	e.syncOK(repo.ID)
	got, err := e.st.GetRepository(t.Context(), repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AllowSquashMerge == nil || !*got.AllowSquashMerge || got.AllowMergeCommit == nil || *got.AllowMergeCommit ||
		got.AllowRebaseMerge == nil || *got.AllowRebaseMerge {
		t.Errorf("merge methods: %v %v %v", got.AllowSquashMerge, got.AllowMergeCommit, got.AllowRebaseMerge)
	}

	meta.AllowSquashMerge, meta.AllowMergeCommit, meta.AllowRebaseMerge = provider.Unknown, provider.Unknown, provider.Unknown
	e.fake.UpdateRepository(meta)
	e.syncOK(repo.ID)
	if got, _ = e.st.GetRepository(t.Context(), repo.ID); got.AllowSquashMerge != nil || got.AllowMergeCommit != nil ||
		got.AllowRebaseMerge != nil {
		t.Errorf("merge methods after the loss of the right: %v %v %v", got.AllowSquashMerge, got.AllowMergeCommit,
			got.AllowRebaseMerge)
	}
}

// End to end with the REAL GitHub provider against githubtest: conventions on, the phase 8.5 fields are
// read in GET /repos and GET /pulls with no extra request (only the reviews of the merged PR, phase 8.6,
// once), and the body never reaches the database nor the logs.
func TestPullConventionsGitHub(t *testing.T) {
	clk := &clock{t: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)}
	srv := githubtest.NewServer(t, githubtest.Options{Now: clk.Now})
	t.Cleanup(srv.Close)
	logs := &safeBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { assertNoLeak(t, "logs", logs.String()) })

	dbPath := filepath.Join(t.TempDir(), "watchoo.db")
	st := openStore(t, dbPath, true)
	cfg := workspace.DefaultConfig("promo")
	cfg.Conventions.Enabled = true
	cache, quota := github.NewCache(0, 0), github.NewQuota()
	eng, err := wsync.New(wsync.Options{
		Store: st, Auth: authtest.NewLoggedIn(""), Config: cfg, Now: clk.Now, Logger: logger,
		NewProvider: func(token string) (provider.Provider, error) {
			return github.New(token, github.Options{BaseURL: srv.URL(), Cache: cache, Quota: quota,
				UserAgent: "watchoo/test", Logger: logger})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	yes, no := true, false
	srv.AddRepo(githubtest.Repo{ID: 10, Owner: testOwner, Name: "projet", Private: true, Push: true,
		AllowSquashMerge: &yes, AllowMergeCommit: &no, AllowRebaseMerge: &no})
	srv.AppendCommits(10, "main", 2, epoch, time.Hour)
	sha := githubtest.FakeSHA("squash")
	srv.SetPullRequests(10,
		githubtest.PullRequest{Number: 1, Title: secretTitle, Closed: true, MergedAt: epoch.Add(2 * time.Hour),
			MergeCommitSHA: sha, Body: convBody, CreatedAt: epoch, UpdatedAt: epoch.Add(2 * time.Hour)},
		githubtest.PullRequest{Number: 2, Title: secretTitle, Fork: true, CreatedAt: epoch, UpdatedAt: epoch.Add(3 * time.Hour)},
		githubtest.PullRequest{Number: 3, Title: secretTitle, HeadRepoDeleted: true, Closed: true,
			CreatedAt: epoch, UpdatedAt: epoch.Add(4 * time.Hour)},
	)
	// Phase 8.6: one approval by another account before the merge; neither the reviewer's login nor the
	// review body reach the database or the logs.
	srv.SetReviews(10, 1, githubtest.Review{ID: 1, Reviewer: secretReviewer, State: "APPROVED",
		SubmittedAt: epoch.Add(time.Hour), Body: secretBody})
	add, err := eng.AddRepositories(t.Context(), []wsync.AddRequest{{Ref: repoimport.Ref{Owner: testOwner, Name: "projet"}}},
		wsync.AddOptions{})
	if err != nil || add[0].Outcome != wsync.OutcomeAdded {
		t.Fatalf("add: %+v, %v", add, err)
	}
	repoID := add[0].Repo.ID
	pullsPath := "/repos/" + testOwner + "/projet/pulls"

	for pass := 1; pass <= 2; pass++ {
		srv.ResetRequests()
		rep, err := eng.SyncAll(t.Context(), wsync.SyncOptions{})
		if err != nil || rep.Repos[0].Status != wsync.StatusSuccess {
			t.Fatalf("sync %d: %+v, %v", pass, rep.Repos, err)
		}
		assertReportNoLeak(t, rep)
		if n := srv.Count(pullsPath); n != 1 {
			t.Errorf("sync %d: %d requests on %s, want 1", pass, n, pullsPath)
		}
		for _, r := range srv.Requests() {
			if r.Method != "GET" {
				t.Errorf("request %s %s", r.Method, r.Path)
			}
			// Phase 8.6: the reviews of the merged PR 1 are read once, on the first pass (PR 2 comes from
			// a fork, PR 3 is closed without merge).
			if strings.Contains(r.Path, "/pulls/") && (pass != 1 || r.Path != pullsPath+"/1/reviews") {
				t.Errorf("sync %d: extra request %s", pass, r.Path)
			}
		}
	}
	repo, err := st.GetRepository(t.Context(), repoID)
	if err != nil {
		t.Fatal(err)
	}
	if repo.ConventionsPRsVersion != conventions.PRRulesVersion || repo.AllowSquashMerge == nil || !*repo.AllowSquashMerge ||
		repo.AllowMergeCommit == nil || *repo.AllowMergeCommit || repo.AllowRebaseMerge == nil || *repo.AllowRebaseMerge {
		t.Errorf("repository: marker %d, methods %v %v %v", repo.ConventionsPRsVersion, repo.AllowSquashMerge,
			repo.AllowMergeCommit, repo.AllowRebaseMerge)
	}
	list, err := st.ConventionPullRequests(t.Context(), repoID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]model.ConventionPullRequest{}
	for _, pr := range list {
		got[pr.Number] = pr
	}
	if m := got[1]; m.MergeCommitSHA != sha || m.FromFork == nil || *m.FromFork || m.BodySections == nil ||
		*m.BodySections == 0 {
		t.Errorf("merged PR: %q %v %v", m.MergeCommitSHA, m.FromFork, m.BodySections)
	}
	if m := got[1]; m.ApprovalsByOthers == nil || *m.ApprovalsByOthers != 1 || m.ReviewsByOthers == nil ||
		*m.ReviewsByOthers != 1 {
		t.Errorf("merged PR reviews: %v %v", m.ReviewsByOthers, m.ApprovalsByOthers)
	}
	for _, n := range []int{2, 3} {
		if f := got[n]; f.FromFork == nil || !*f.FromFork {
			t.Errorf("PR %d: fork %v, want true", n, f.FromFork)
		}
	}
	for _, suffix := range []string{"", "-wal"} {
		if b, err := os.ReadFile(dbPath + suffix); err == nil && strings.Contains(string(b), secretBody) {
			t.Errorf("PR body found in %s", dbPath+suffix)
		}
		if b, err := os.ReadFile(dbPath + suffix); err == nil && strings.Contains(string(b), secretReviewer) {
			t.Errorf("reviewer's login found in %s", dbPath+suffix)
		}
	}
}
