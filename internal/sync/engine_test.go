package sync_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	gosync "sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

func TestNewOptionsObligatoires(t *testing.T) {
	if _, err := wsync.New(wsync.Options{Auth: authtest.NewLoggedIn("")}); err == nil {
		t.Error("New without database: error expected")
	}
	st := openStore(t, t.TempDir()+"/w.db", true)
	if _, err := wsync.New(wsync.Options{Store: st}); err == nil {
		t.Error("New without gh access: error expected")
	}
	if _, err := wsync.New(wsync.Options{Store: st, Auth: authtest.NewLoggedIn("")}); err != nil {
		t.Errorf("New with the default GitHub provider: %v", err)
	}
}

// First import: complete list of the reference branch, comparison for the other branches, no
// statistics, commit_branches and last_commit_at filled.
func TestPremierImport(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 5, epoch, time.Hour)
	e.fake.SetBranch(1, "feature", mainC[2].SHA)
	feat := e.fake.AppendCommits(1, "feature", 2, epoch.Add(24*time.Hour), time.Hour)

	r := e.syncOK(repo.ID)
	if r.NewCommits != 7 || r.TrackedBranches != 2 || r.StatsFetched != 0 {
		t.Errorf("rapport = %+v, 7 commits, 2 branches, 0 statistique attendus", r)
	}
	if r.Duration < 0 || r.FullName != testOwner+"/projet" || r.DisplayName != testOwner+"/projet" {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 1 {
		t.Errorf("ListCommits called %d times, want 1", n)
	}
	if n := e.fake.Calls(providertest.MethodCompareCommits); n != 1 {
		t.Errorf("CompareCommits called %d times, want 1", n)
	}
	if n := e.fake.Calls(providertest.MethodCommitStats); n != 0 {
		t.Errorf("CommitStats called %d times on the first import", n)
	}
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodListCommits && c.StopSet {
			t.Error("ListCommits called with Stop on the first import")
		}
	}
	if got, want := e.commitSHAs(repo.ID, "feature"), sorted(feat[0].SHA, feat[1].SHA); !slices.Equal(got, want) {
		t.Errorf("commits of feature = %v, want %v (commits specific to the branch)", got, want)
	}
	if got := e.commitSHAs(repo.ID, "main"); len(got) != 5 {
		t.Errorf("%d commits on main, want 5", len(got))
	}
	stored, err := e.st.GetRepository(t.Context(), repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.LastCommitAt.Equal(feat[1].CommittedAt) {
		t.Errorf("last_commit_at = %v, want %v", stored.LastCommitAt, feat[1].CommittedAt)
	}
	if stored.DefaultBranch != "main" || stored.LastSyncedAt.IsZero() {
		t.Errorf("metadata = %+v", stored)
	}
	run := e.lastRun(repo.ID)
	if run.Status != model.SyncSuccess || run.NewCommits != 7 {
		t.Errorf("sync_run = %+v", run)
	}
	b := e.branches(repo.ID)
	if b["main"].HeadSHA != mainC[4].SHA || b["feature"].HeadSHA != feat[1].SHA || !b["main"].Tracked {
		t.Errorf("branches = %+v", b)
	}
	// Contributor: GitHub account of the dummy author.
	ranking, err := e.st.ContributorRanking(t.Context(), store.ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranking) != 1 || ranking[0].Commits != 7 || ranking[0].ProviderUserID != 1001 {
		t.Errorf("contributeurs = %+v", ranking)
	}
}

// Incremental sync: comparison old head → new one, statistics of the new commits, no commit
// request when nothing moved.
func TestIncremental(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	first := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.fake.SetBranch(1, "dev", first[2].SHA)
	e.syncOK(repo.ID)

	next := e.fake.AppendCommits(1, "main", 2, epoch.Add(48*time.Hour), time.Hour)
	e.fake.SetStats(1, next[0].SHA, provider.CommitStats{Additions: 10, Deletions: 2})
	e.fake.SetStats(1, next[1].SHA, provider.CommitStats{Additions: 5, Deletions: 1})
	e.fake.ResetCalls()
	r := e.syncOK(repo.ID)
	if r.NewCommits != 2 || r.StatsFetched != 2 || r.TrackedBranches != 2 {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 0 {
		t.Errorf("ListCommits called %d times on an already imported repository", n)
	}
	var compares []providertest.Call
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodCompareCommits {
			compares = append(compares, c)
		}
	}
	if len(compares) != 1 || compares[0].Base != first[2].SHA || compares[0].Head != next[1].SHA {
		t.Errorf("comparisons = %+v, want a single one (old head of main → new one)", compares)
	}
	if v := e.commitView(repo.ID, next[0].SHA); v.Additions != 10 || v.Deletions != 2 {
		t.Errorf("statistiques = +%d -%d, +10 -2 attendues", v.Additions, v.Deletions)
	}

	// Nothing moved: branches read again, no commit request.
	e.fake.ResetCalls()
	r = e.syncOK(repo.ID)
	if r.NewCommits != 0 {
		t.Errorf("%d new commits without change", r.NewCommits)
	}
	for _, m := range []providertest.Method{providertest.MethodListCommits, providertest.MethodCompareCommits,
		providertest.MethodCommitStats} {
		if n := e.fake.Calls(m); n != 0 {
			t.Errorf("%s called %d times although no head changed", m, n)
		}
	}
	if n := e.fake.Calls(providertest.MethodBranches); n != 1 {
		t.Errorf("Branches called %d times, want 1", n)
	}
}

// New branch of an already synced repository: comparison from the head of the reference branch.
func TestNouvelleBranche(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.syncOK(repo.ID)

	e.fake.SetBranch(1, "tp2", mainC[1].SHA)
	tp2 := e.fake.AppendCommits(1, "tp2", 2, epoch.Add(72*time.Hour), time.Hour)
	e.fake.ResetCalls()
	r := e.syncOK(repo.ID)
	if r.NewCommits != 2 || r.TrackedBranches != 2 {
		t.Errorf("rapport = %+v", r)
	}
	log := e.fake.CallLog()
	idx := slices.IndexFunc(log, func(c providertest.Call) bool { return c.Method == providertest.MethodCompareCommits })
	if idx < 0 || log[idx].Base != mainC[2].SHA || log[idx].Head != tp2[1].SHA {
		t.Errorf("comparison expected head of main → head of tp2, log %+v", log)
	}
	if got := e.commitSHAs(repo.ID, "tp2"); !slices.Equal(got, sorted(tp2[0].SHA, tp2[1].SHA)) {
		t.Errorf("commits of tp2 = %v", got)
	}
}

// Merge that brings a commit older than the old head: the comparison finds it (a stop at the first
// known SHA would have lost it).
func TestMergeCommitsPlusAnciens(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	m0 := e.commit(1, "m0", epoch)
	m1 := e.commit(1, "m1", epoch.Add(48*time.Hour), m0)
	e.fake.SetBranch(1, "main", m1)
	e.syncOK(repo.ID)

	f1 := e.commit(1, "f1", epoch.Add(24*time.Hour), m0) // older than M1
	x := e.commit(1, "x", epoch.Add(72*time.Hour), m1, f1)
	e.fake.SetBranch(1, "main", x)
	r := e.syncOK(repo.ID)
	if r.NewCommits != 2 {
		t.Errorf("%d new commits, want 2 (merge and older merged commit)", r.NewCommits)
	}
	if got := e.commitSHAs(repo.ID, ""); !slices.Equal(got, sorted(m0, m1, f1, x)) {
		t.Errorf("commits = %v", got)
	}
}

// Force-push on a secondary branch: unknown base, fallback from the head of the reference
// branch.
func TestForcePushBranche(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.fake.SetBranch(1, "feature", mainC[2].SHA)
	feat := e.fake.AppendCommits(1, "feature", 2, epoch.Add(24*time.Hour), time.Hour)
	e.syncOK(repo.ID)

	rewritten := e.commit(1, "f1-reecrit", epoch.Add(96*time.Hour), mainC[2].SHA)
	e.fake.SetBranch(1, "feature", rewritten)
	e.fake.RemoveCommit(1, feat[1].SHA)
	e.fake.RemoveCommit(1, feat[0].SHA)
	e.fake.ResetCalls()
	r := e.syncOK(repo.ID)
	if r.NewCommits != 1 {
		t.Errorf("%d new commits, want 1", r.NewCommits)
	}
	var compares []providertest.Call
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodCompareCommits {
			compares = append(compares, c)
		}
	}
	if len(compares) != 2 || compares[0].Base != feat[1].SHA || compares[1].Base != mainC[2].SHA {
		t.Errorf("comparisons = %+v: old head then head of main expected", compares)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 0 {
		t.Errorf("ListCommits called %d times", n)
	}
	if b := e.branches(repo.ID); b["feature"].HeadSHA != rewritten {
		t.Errorf("head of feature = %s", b["feature"].HeadSHA)
	}
}

// Force-push on the reference branch: list read again without Stop, known SHAs ignored (neither
// recorded again, nor statistics).
func TestForcePushReference(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.syncOK(repo.ID)

	b3 := e.commit(1, "b3", epoch.Add(96*time.Hour), mainC[1].SHA)
	e.fake.SetBranch(1, "main", b3)
	e.fake.RemoveCommit(1, mainC[2].SHA)
	e.fake.ResetCalls()
	r := e.syncOK(repo.ID)
	if r.NewCommits != 1 || r.StatsFetched != 1 {
		t.Errorf("report = %+v: 1 new commit, 1 statistic expected", r)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 1 {
		t.Errorf("ListCommits called %d times, want 1 (fallback)", n)
	}
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodListCommits && c.StopSet {
			t.Error("fallback with Stop")
		}
		if c.Method == providertest.MethodCommitStats && c.SHA != b3 {
			t.Errorf("statistics requested for a commit already in the database %s", provider.ShortSHA(c.SHA))
		}
	}
	// The old rewritten commit stays in the database (history kept), outside a tracked branch for last_commit_at.
	if got := e.commitSHAs(repo.ID, ""); len(got) != 4 {
		t.Errorf("%d commits in the database, want 4", len(got))
	}
}

// Branch deleted on the forge: tracked = false, history kept.
func TestBrancheSupprimee(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)
	e.fake.SetBranch(1, "old", mainC[1].SHA)
	old := e.fake.AppendCommits(1, "old", 1, epoch.Add(24*time.Hour), time.Hour)
	e.syncOK(repo.ID)

	e.fake.DeleteBranch(1, "old")
	r := e.syncOK(repo.ID)
	if r.TrackedBranches != 1 {
		t.Errorf("%d branches suivies, 1 attendue", r.TrackedBranches)
	}
	b := e.branches(repo.ID)
	if b["old"].Tracked || !b["main"].Tracked {
		t.Errorf("branches = %+v", b)
	}
	if got := e.commitSHAs(repo.ID, "old"); !slices.Equal(got, []string{old[0].SHA}) {
		t.Errorf("history of old = %v", got)
	}
	stored, _ := e.st.GetRepository(t.Context(), repo.ID)
	if !stored.LastCommitAt.Equal(mainC[1].CommittedAt) {
		t.Errorf("last_commit_at = %v, want the last commit of main %v (tracked branches)", stored.LastCommitAt,
			mainC[1].CommittedAt)
	}
}

// branch_patterns and branches.exclude; reference branch = first tracked branch if the default
// branch is not tracked.
func TestMotifsEtExclusions(t *testing.T) {
	e := newEnv(t, withExclude("dependabot/*"))
	repo := e.addRepo(1, "projet", "release/*", "dependabot/*", "dev")
	mainC := e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)
	for _, name := range []string{"dev", "release/1", "dependabot/npm", "wip"} {
		e.fake.SetBranch(1, name, mainC[1].SHA)
	}
	rel := e.fake.AppendCommits(1, "release/1", 1, epoch.Add(24*time.Hour), time.Hour)

	r := e.syncOK(repo.ID)
	if r.TrackedBranches != 2 {
		t.Errorf("%d branches suivies, 2 attendues (dev, release/1)", r.TrackedBranches)
	}
	b := e.branches(repo.ID)
	if _, ok := b["main"]; ok {
		t.Error("main tracked despite the patterns")
	}
	if _, ok := b["dependabot/npm"]; ok {
		t.Error("excluded branch tracked")
	}
	// dev is the reference (first tracked branch, order of the forge): complete list; release/1
	// is compared from dev.
	if n := e.fake.Calls(providertest.MethodListCommits); n != 1 {
		t.Errorf("ListCommits called %d times", n)
	}
	if got := e.commitSHAs(repo.ID, "release/1"); !slices.Equal(got, []string{rel[0].SHA}) {
		t.Errorf("commits of release/1 = %v", got)
	}
	if got := e.commitSHAs(repo.ID, ""); len(got) != 3 {
		t.Errorf("%d commits, 3 attendus", len(got))
	}
}

// Repository renamed then transferred: tracked by identifier, owner/name updated.
func TestDepotRenomme(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.syncOK(repo.ID)

	e.fake.UpdateRepository(provider.Repository{ID: 1, Owner: "autre-orga", Name: "projet-final",
		DefaultBranch: "main", Stars: 3, OpenIssues: 2, Archived: true})
	r := e.syncOK(repo.ID)
	if r.FullName != "autre-orga/projet-final" || r.DisplayName != "autre-orga/projet-final" {
		t.Errorf("rapport = %+v", r)
	}
	stored, _ := e.st.GetRepository(t.Context(), repo.ID)
	if stored.Owner != "autre-orga" || stored.Name != "projet-final" || stored.Stars != 3 ||
		stored.OpenIssues != 2 || !stored.Archived {
		t.Errorf("repository in the database = %+v", stored)
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("Repository (by name) called %d times: RepositoryByID expected", n)
	}
}

// Empty repository: success without commit.
func TestDepotVide(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "vide")
	r := e.syncOK(repo.ID)
	if r.NewCommits != 0 || r.TrackedBranches != 0 {
		t.Errorf("rapport = %+v", r)
	}
	stored, _ := e.st.GetRepository(t.Context(), repo.ID)
	if stored.LastSyncedAt.IsZero() || !stored.LastCommitAt.IsZero() {
		t.Errorf("repository = %+v", stored)
	}
	// First commit pushed afterwards: first import.
	e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)
	if r := e.syncOK(repo.ID); r.NewCommits != 2 || r.StatsFetched != 0 {
		t.Errorf("rapport = %+v", r)
	}
}

// Private repository that became inaccessible (404) and repository under SSO: error recorded, the
// others go on.
func TestDepotInaccessibleEtSSO(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	a := e.addRepo(1, "a-perdu")
	b := e.addRepo(2, "b-sso")
	c := e.addRepo(3, "c-ok")
	e.fake.AppendCommits(3, "main", 1, epoch, time.Hour)
	e.fake.RemoveRepository(1)
	e.fake.SetRepoError(testOwner, "b-sso", &provider.SSORequiredError{Organization: testOwner,
		URL: "https://github.com/orgs/x/sso?authorization_request=secret"})

	rep := e.syncAll()
	if len(rep.Repos) != 3 {
		t.Fatalf("%d rapports", len(rep.Repos))
	}
	if r := rep.Repos[0]; r.Status != wsync.StatusError || !strings.Contains(r.Error, "not found or account without access") {
		t.Errorf("inaccessible repository: %+v", r)
	}
	if r := rep.Repos[1]; r.Status != wsync.StatusError || !strings.Contains(r.Error, "SSO") ||
		strings.Contains(r.Error, "authorization_request") {
		t.Errorf("SSO repository: %+v", r)
	}
	if r := rep.Repos[2]; r.Status != wsync.StatusSuccess || r.NewCommits != 1 {
		t.Errorf("accessible repository: %+v", r)
	}
	if rep.Count(wsync.StatusError) != 2 || rep.NewCommits() != 1 {
		t.Errorf("Count/NewCommits = %d/%d", rep.Count(wsync.StatusError), rep.NewCommits())
	}
	for _, id := range []int64{a.ID, b.ID} {
		run := e.lastRun(id)
		if run.Status != model.SyncError || run.Error == "" || run.FinishedAt.IsZero() {
			t.Errorf("sync_run of repository %d = %+v", id, run)
		}
	}
	if run := e.lastRun(c.ID); run.Status != model.SyncSuccess {
		t.Errorf("sync_run of the accessible repository = %+v", run)
	}
	stored, _ := e.st.GetRepository(t.Context(), a.ID)
	if !stored.LastSyncedAt.IsZero() {
		t.Error("last_synced_at written despite the failure")
	}
}

// Low quota before the sync: all repositories are deferred, without sync_run.
func TestQuotaBasReporte(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	reset := e.clock.Now().Add(40 * time.Minute)
	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 499, Reset: reset})

	rep := e.syncAll()
	r := rep.Repos[0]
	if r.Status != wsync.StatusDeferred || !r.ResumeAt.Equal(reset) || r.Duration != 0 {
		t.Errorf("rapport = %+v", r)
	}
	if r.Error != "deferred: low quota until 10:40 UTC" || r.DeferReason != wsync.DeferLowQuota {
		t.Errorf("message = %q", r.Error)
	}
	if _, err := e.st.LastSyncRun(t.Context(), repo.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("sync_run created for a deferred repository: %v", err)
	}
	if !rep.RateLimitKnown || rep.RateLimit.Remaining != 499 {
		t.Errorf("quota of the report = %+v (%v)", rep.RateLimit, rep.RateLimitKnown)
	}
	// Just above the threshold (exactly 10 %): synced.
	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 500, Reset: reset})
	if r := e.syncAll().Repos[0]; r.Status != wsync.StatusSuccess {
		t.Errorf("at 10 %%: %+v", r)
	}
	// Window over: the observed state is renewed, nothing is deferred.
	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 10, Reset: reset})
	e.clock.Set(reset.Add(time.Minute))
	if r := e.syncAll().Repos[0]; r.Status != wsync.StatusSuccess {
		t.Errorf("after the reset: %+v", r)
	}
}

// The quota goes below 10 % during the sync: the following repositories are deferred.
func TestQuotaBasEnCours(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	e.addRepo(1, "a")
	e.addRepo(2, "b")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.AppendCommits(2, "main", 1, epoch, time.Hour)
	e.fake.SetRateLimit(provider.RateLimit{Limit: 100, Remaining: 12})

	rep := e.syncAll()
	if rep.Repos[0].Status != wsync.StatusSuccess || rep.Repos[1].Status != wsync.StatusDeferred {
		t.Errorf("rapport = %+v", rep.Repos)
	}
	if rep.RateLimit.Remaining != 9 {
		t.Errorf("remaining quota = %d, want 9 (3 requests for the first repository)", rep.RateLimit.Remaining)
	}
}

// Exhausted quota (RateLimitError): the current repository fails, the following ones are deferred until the
// resume.
func TestRateLimitErrorArrete(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	for i, name := range []string{"a", "b", "c"} {
		e.addRepo(int64(i+1), name)
		e.fake.AppendCommits(int64(i+1), "main", 1, epoch, time.Hour)
	}
	reset := time.Date(2026, 10, 20, 11, 5, 0, 0, time.UTC)
	e.fake.FailNext(providertest.MethodBranches, &provider.APIError{Op: "list of branches", Status: 403,
		Err: &provider.RateLimitError{Reset: reset}})

	rep := e.syncAll()
	if r := rep.Repos[0]; r.Status != wsync.StatusError || !strings.Contains(r.Error, "rate limit") {
		t.Errorf("first repository: %+v", r)
	}
	for _, r := range rep.Repos[1:] {
		if r.Status != wsync.StatusDeferred || !r.ResumeAt.Equal(reset) ||
			r.Error != "deferred: quota exhausted until 11:05 UTC" || r.DeferReason != wsync.DeferQuotaExhausted {
			t.Errorf("next repository: %+v", r)
		}
	}
	if n := e.fake.Calls(providertest.MethodRepositoryByID); n != 1 {
		t.Errorf("RepositoryByID called %d times after the exhausted quota", n)
	}
	if run := e.lastRun(1); run.Status != model.SyncError {
		t.Errorf("sync_run = %+v", run)
	}
}

// Secondary limit without Reset: resume = now + Retry-After.
func TestRateLimitSecondaire(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	e.addRepo(1, "a")
	e.addRepo(2, "b")
	e.fake.FailNext(providertest.MethodRepositoryByID, &provider.RateLimitError{Secondary: true, RetryAfter: time.Minute})
	rep := e.syncAll()
	if r := rep.Repos[1]; r.Status != wsync.StatusDeferred || !r.ResumeAt.Equal(e.clock.Now().Add(time.Minute)) {
		t.Errorf("rapport = %+v", r)
	}
}

// Lock: a recent sync_run blocks the repository; a stale sync_run is closed and the sync goes through.
func TestVerrou(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	now := e.clock.Now()
	if _, err := e.st.BeginSyncRun(t.Context(), repo.ID, now.Add(-10*time.Minute), wsync.LockStaleAfter); err != nil {
		t.Fatal(err)
	}
	rep := e.syncAll()
	if r := rep.Repos[0]; r.Status != wsync.StatusLocked || r.Error != "already being synced" {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodRepositoryByID); n != 0 {
		t.Errorf("locked repository queried %d times", n)
	}

	e.clock.Set(now.Add(25 * time.Minute)) // the running one is 35 min old: stale
	if r := e.syncAll().Repos[0]; r.Status != wsync.StatusSuccess {
		t.Errorf("after expiry: %+v", r)
	}
}

// Two engines on the same database (two processes): the second one sees the repository locked while
// the first one syncs it.
func TestVerrouDeuxMoteurs(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once gosync.Once
	e := newEnv(t, withWrap(func(p provider.Provider) provider.Provider {
		return &hookProvider{Provider: p, before: func(ctx context.Context, m string) error {
			if m == "Branches" {
				once.Do(func() {
					close(entered)
					<-release
				})
			}
			return nil
		}}
	}))
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)

	second := e.newEngine(openStore(t, e.dbPath, false))
	done := make(chan wsync.Report)
	go func() {
		rep, err := e.eng.SyncRepo(context.Background(), repo.ID, wsync.SyncOptions{})
		if err != nil {
			t.Errorf("SyncRepo: %v", err)
		}
		done <- rep
	}()
	<-entered
	rep, err := second.SyncAll(t.Context(), wsync.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r := rep.Repos[0]; r.Status != wsync.StatusLocked {
		t.Errorf("second engine: %+v", r)
	}
	close(release)
	if r := (<-done).Repos[0]; r.Status != wsync.StatusSuccess || r.NewCommits != 2 {
		t.Errorf("first engine: %+v", r)
	}
}

// Parallelism bounded by sync.parallelism (and by 4 at most).
func TestParallelismeBorne(t *testing.T) {
	for _, tc := range []struct{ setting, want int }{{2, 2}, {9, 4}, {0, 1}} {
		var cur, peak atomic.Int32
		e := newEnv(t, withParallelism(tc.setting), withWrap(func(p provider.Provider) provider.Provider {
			return &hookProvider{Provider: p, before: func(ctx context.Context, m string) error {
				if m == "RepositoryByID" {
					n := cur.Add(1)
					for {
						old := peak.Load()
						if n <= old || peak.CompareAndSwap(old, n) {
							break
						}
					}
					// Waits until tc.want repositories are there at the same time (or 5 s at most): on
					// a slow machine, a simple pause is not enough to make them overlap.
					for deadline := time.Now().Add(5 * time.Second); cur.Load() < int32(tc.want) && time.Now().Before(deadline); {
						time.Sleep(time.Millisecond)
					}
					time.Sleep(10 * time.Millisecond)
					cur.Add(-1)
				}
				return nil
			}}
		}))
		for i := range 8 {
			e.addRepo(int64(i+1), "depot-"+string(rune('a'+i)))
		}
		rep := e.syncAll()
		if rep.Count(wsync.StatusSuccess) != 8 {
			t.Errorf("parallelism %d: %d successes", tc.setting, rep.Count(wsync.StatusSuccess))
		}
		if got := int(peak.Load()); got != tc.want {
			t.Errorf("parallelism %d: at most %d repositories at the same time, want %d", tc.setting, got, tc.want)
		}
		// Order of the report: the one of ListRepositories.
		for i, r := range rep.Repos {
			if r.FullName != testOwner+"/depot-"+string(rune('a'+i)) {
				t.Errorf("rapport %d = %s", i, r.FullName)
			}
		}
	}
}

// Statistics: at most 100 per repository and per sync.
func TestStatistiquesPlafond(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.syncOK(repo.ID)
	e.fake.AppendCommits(1, "main", 150, epoch.Add(time.Hour), time.Minute)
	r := e.syncOK(repo.ID)
	if r.NewCommits != 150 || r.StatsFetched != wsync.MaxStatsPerRepo {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodCommitStats); n != 100 {
		t.Errorf("CommitStats called %d times", n)
	}
	// The most recent commits are served first.
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodCommitStats {
			v := e.commitView(repo.ID, c.SHA)
			if v.CommittedAt.Before(epoch.Add(time.Hour + 50*time.Minute)) {
				t.Errorf("statistics of an old commit %v", v.CommittedAt)
			}
		}
	}
}

// Statistics: never when the remaining quota goes below 10 %.
func TestStatistiquesQuota(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.syncOK(repo.ID)
	e.fake.AppendCommits(1, "main", 10, epoch.Add(time.Hour), time.Minute)
	// 105 left: RepositoryByID, Branches, CompareCommits (102), then statistics at 102, 101 and
	// 100 (exactly 10 %); at 99, nothing more.
	e.fake.SetRateLimit(provider.RateLimit{Limit: 1000, Remaining: 105})
	r := e.syncOK(repo.ID)
	if r.NewCommits != 10 || r.StatsFetched != 3 {
		t.Errorf("report = %+v: 3 statistics expected", r)
	}
}

// Statistics: a quota exhausted during the statistics keeps the repository successful and defers the
// following ones; another error stops only the statistics.
func TestStatistiquesErreurs(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	a := e.addRepo(1, "a")
	e.addRepo(2, "b")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.AppendCommits(2, "main", 1, epoch, time.Hour)
	e.syncAll()
	e.fake.AppendCommits(1, "main", 3, epoch.Add(time.Hour), time.Minute)

	e.fake.FailNext(providertest.MethodCommitStats, &provider.RateLimitError{Reset: e.clock.Now().Add(time.Hour)})
	rep := e.syncAll()
	if r := rep.Repos[0]; r.Status != wsync.StatusSuccess || r.NewCommits != 3 || r.StatsFetched != 1 {
		t.Errorf("repository a: %+v", r)
	}
	if r := rep.Repos[1]; r.Status != wsync.StatusDeferred {
		t.Errorf("repository b: %+v", r)
	}

	e.fake.AppendCommits(1, "main", 3, epoch.Add(2*time.Hour), time.Minute)
	e.fake.FailNext(providertest.MethodCommitStats, &provider.APIError{Op: "statistiques", Err: provider.ErrUnavailable})
	if r := e.syncOK(a.ID); r.NewCommits != 3 || r.StatsFetched != 1 {
		t.Errorf("transient error: %+v", r)
	}
}

// Identity merge: an author first known by their e-mail only, then by their GitHub account, becomes
// a single contributor; a commit without account nor e-mail stays without author.
func TestFusionIdentites(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	noAccount := provider.Person{Name: secretName, Email: "Eleve.Confidentiel@Example.Test"}
	c1 := providertest.SHA("c1")
	e.fake.AddCommits(1, provider.Commit{SHA: c1, Message: secretMessage, Author: noAccount, Committer: noAccount,
		AuthoredAt: epoch, CommittedAt: epoch})
	anonymous := provider.Person{Name: "Sans Email"}
	c2 := providertest.SHA("c2")
	e.fake.AddCommits(1, provider.Commit{SHA: c2, Message: secretMessage, Author: anonymous, Committer: anonymous,
		AuthoredAt: epoch.Add(time.Hour), CommittedAt: epoch.Add(time.Hour), Parents: []string{c1}})
	e.fake.SetBranch(1, "main", c2)
	e.syncOK(repo.ID)

	c3 := e.commit(1, "c3", epoch.Add(2*time.Hour), c2) // account 4242, same e-mail
	e.fake.SetBranch(1, "main", c3)
	e.syncOK(repo.ID)

	ranking, err := e.st.ContributorRanking(t.Context(), store.ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranking) != 1 || ranking[0].Commits != 2 || ranking[0].ProviderUserID != 4242 ||
		ranking[0].ProviderLogin != secretLogin {
		t.Errorf("ranking = %+v: a single contributor (account 4242, 2 commits) expected", ranking)
	}
	if v := e.commitView(repo.ID, c2); v.AuthorID != 0 {
		t.Errorf("commit without account nor e-mail: author %d, none expected", v.AuthorID)
	}
}

// The token is asked again to gh at every sync and passed to the factory; a failure of gh is a
// global error (empty report, end event sent).
func TestTokenRedemande(t *testing.T) {
	e := newEnv(t)
	e.addRepo(1, "projet")
	e.auth.SetTokens("gho_fake_premier", "gho_fake_second")
	e.syncAll()
	e.syncAll()
	if _, err := e.eng.RateLimit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := e.receivedTokens(); !slices.Equal(got, []string{"gho_fake_premier", "gho_fake_second", "gho_fake_second"}) {
		t.Errorf("received tokens = %v", got)
	}
	if n := e.auth.TokenCalls(); n != 3 {
		t.Errorf("Token called %d times, want 3", n)
	}

	// gh installed but broken (timeout): error, without switching to the Git clones.
	e.auth.SetTokenErr(auth.ErrTimeout)
	var kinds []wsync.EventKind
	rep, err := e.eng.SyncAll(t.Context(), wsync.SyncOptions{Progress: func(ev wsync.Event) { kinds = append(kinds, ev.Kind) }})
	if !errors.Is(err, auth.ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
	if rep.Repos == nil || len(rep.Repos) != 0 {
		t.Errorf("report = %+v, want empty", rep.Repos)
	}
	if !slices.Equal(kinds, []wsync.EventKind{wsync.EventStarted, wsync.EventFinished}) {
		t.Errorf("events = %v", kinds)
	}
	if _, err := e.eng.RateLimit(t.Context()); !errors.Is(err, auth.ErrTimeout) {
		t.Errorf("RateLimit: %v", err)
	}
	if _, err := e.eng.AddRepositories(t.Context(), nil, wsync.AddOptions{}); !errors.Is(err, auth.ErrTimeout) {
		t.Errorf("AddRepositories: %v", err)
	}
}

// An error that would quote the token is masked in the report and in sync_runs.
func TestTokenMasque(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.FailNext(providertest.MethodRepositoryByID, errors.New("unexpected outage "+authtest.FakeToken))
	rep := e.syncAll()
	if r := rep.Repos[0]; r.Status != wsync.StatusError || !strings.Contains(r.Error, "[token redacted]") {
		t.Errorf("rapport = %+v", r)
	}
	if run := e.lastRun(repo.ID); strings.Contains(run.Error, authtest.FakeToken) {
		t.Errorf("token recorded in sync_runs: %q", run.Error)
	}
}

// Progress: start, start and end of every repository, end; increasing Done, known Total.
func TestProgression(t *testing.T) {
	e := newEnv(t, withParallelism(4))
	for i := range 3 {
		e.addRepo(int64(i+1), "p"+string(rune('a'+i)))
	}
	var (
		mu     gosync.Mutex
		events []wsync.Event
		busy   atomic.Bool
	)
	rep, err := e.eng.SyncAll(t.Context(), wsync.SyncOptions{Progress: func(ev wsync.Event) {
		if !busy.CompareAndSwap(false, true) {
			t.Error("simultaneous Progress calls")
		}
		time.Sleep(time.Millisecond)
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
		busy.Store(false)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 8 || events[0].Kind != wsync.EventStarted || events[7].Kind != wsync.EventFinished {
		t.Fatalf("events = %+v", events)
	}
	finished := 0
	for _, ev := range events {
		if ev.Total != 3 {
			t.Errorf("Total = %d", ev.Total)
		}
		if ev.Kind == wsync.EventRepoFinished {
			finished++
			if ev.Done != finished || ev.Repo.Status != wsync.StatusSuccess {
				t.Errorf("event = %+v", ev)
			}
		}
	}
	if events[7].Done != 3 || rep.FinishedAt.Before(rep.StartedAt) {
		t.Errorf("fin = %+v", events[7])
	}
}

// SyncAll ignores the stopped repositories; SyncRepo syncs them (explicit request); unknown repository.
func TestSuiviArreteEtDepotInconnu(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	if err := e.st.SetRepositoryEnabled(t.Context(), repo.ID, false); err != nil {
		t.Fatal(err)
	}
	if rep := e.syncAll(); len(rep.Repos) != 0 {
		t.Errorf("stopped repository synced by SyncAll: %+v", rep.Repos)
	}
	if r := e.syncOK(repo.ID); r.NewCommits != 1 {
		t.Errorf("SyncRepo: %+v", r)
	}
	if _, err := e.eng.SyncRepo(t.Context(), 999, wsync.SyncOptions{}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown repository: %v", err)
	}
}

// Context canceled during the sync: global error, the current repository is closed in error.
func TestAnnulation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	e := newEnv(t, withParallelism(1), withWrap(func(p provider.Provider) provider.Provider {
		return &hookProvider{Provider: p, before: func(c context.Context, m string) error {
			if m == "Branches" {
				cancel()
				return c.Err()
			}
			return nil
		}}
	}))
	a := e.addRepo(1, "a")
	e.addRepo(2, "b")
	rep, err := e.eng.SyncAll(ctx, wsync.SyncOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v", err)
	}
	if len(rep.Repos) != 2 || rep.Repos[0].Status != wsync.StatusError || rep.Repos[0].Error != "sync canceled" {
		t.Errorf("rapport = %+v", rep.Repos)
	}
	if run := e.lastRun(a.ID); run.Status != model.SyncError || run.Error != "sync canceled" {
		t.Errorf("sync_run = %+v", run)
	}
	if _, err := e.st.LastSyncRun(t.Context(), 2); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("next repository started despite the cancellation: %v", err)
	}
}

// Unknown quota at the start: one RateLimit call (free); known: none.
func TestLectureInitialeDuQuota(t *testing.T) {
	e := newEnv(t)
	e.addRepo(1, "projet")
	e.syncAll()
	if n := e.fake.Calls(providertest.MethodRateLimit); n != 1 {
		t.Errorf("RateLimit called %d times, want 1", n)
	}
	e.fake.SetRateLimit(provider.RateLimit{Remaining: 4000})
	e.fake.ResetCalls()
	e.syncAll()
	if n := e.fake.Calls(providertest.MethodRateLimit); n != 0 {
		t.Errorf("RateLimit called %d times with a known quota", n)
	}
	rl, err := e.eng.RateLimit(t.Context())
	if err != nil || rl.Limit != 5000 {
		t.Errorf("RateLimit = %+v, %v", rl, err)
	}
}

// Default branch changed: the new reference branch is read again in full (linked to its whole
// history, commits already in the database included); a commit seen on two new branches is
// linked to both.
func TestNouvelleReferenceEtCommitsPartages(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.syncOK(repo.ID)

	e.fake.UpdateRepository(provider.Repository{ID: 1, Owner: testOwner, Name: "projet", DefaultBranch: "dev"})
	e.fake.SetBranch(1, "dev", mainC[2].SHA)
	dev := e.fake.AppendCommits(1, "dev", 1, epoch.Add(24*time.Hour), time.Hour)
	e.fake.SetBranch(1, "tp-a", dev[0].SHA)
	e.fake.SetBranch(1, "tp-b", dev[0].SHA)
	shared := e.fake.AppendCommits(1, "tp-a", 1, epoch.Add(48*time.Hour), time.Hour)
	e.fake.SetBranch(1, "tp-b", shared[0].SHA)
	e.fake.ResetCalls()

	r := e.syncOK(repo.ID)
	if r.NewCommits != 2 || r.TrackedBranches != 4 {
		t.Errorf("rapport = %+v", r)
	}
	log := e.fake.CallLog()
	if slices.ContainsFunc(log, func(c providertest.Call) bool {
		return c.Method == providertest.MethodCompareCommits && c.Head == dev[0].SHA
	}) {
		t.Errorf("dev compared instead of being read again: %+v", log)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 1 {
		t.Errorf("ListCommits called %d times, want 1 (new reference)", n)
	}
	want := sorted(mainC[0].SHA, mainC[1].SHA, mainC[2].SHA, dev[0].SHA)
	if got := e.commitSHAs(repo.ID, "dev"); !slices.Equal(got, want) {
		t.Errorf("commits of dev = %v, want %v", got, want)
	}
	for _, b := range []string{"tp-a", "tp-b"} {
		if got := e.commitSHAs(repo.ID, b); !slices.Equal(got, []string{shared[0].SHA}) {
			t.Errorf("commits of %s = %v", b, got)
		}
	}
	if stored, _ := e.st.GetRepository(t.Context(), repo.ID); stored.DefaultBranch != "dev" {
		t.Errorf("default branch = %q", stored.DefaultBranch)
	}
}

// Missing dates: a single missing date is replaced by the other one; without any date, the commit
// is ignored (and reported in the logs, by a counter).
func TestCommitSansDate(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	p := provider.Person{Name: secretName, Email: secretEmail}
	a, b, c := providertest.SHA("a"), providertest.SHA("b"), providertest.SHA("c")
	e.fake.AddCommits(1,
		provider.Commit{SHA: a, Author: p, Committer: p, CommittedAt: epoch},
		provider.Commit{SHA: b, Author: p, Committer: p, AuthoredAt: epoch.Add(time.Hour), Parents: []string{a}},
		provider.Commit{SHA: c, Author: p, Committer: p, Parents: []string{b}},
	)
	e.fake.SetBranch(1, "main", c)
	if r := e.syncOK(repo.ID); r.NewCommits != 2 {
		t.Errorf("%d new commits, want 2", r.NewCommits)
	}
	if v := e.commitView(repo.ID, a); !v.AuthoredAt.Equal(epoch) {
		t.Errorf("authored date = %v", v.AuthoredAt)
	}
	if v := e.commitView(repo.ID, b); !v.CommittedAt.Equal(epoch.Add(time.Hour)) {
		t.Errorf("commit date = %v", v.CommittedAt)
	}
	if !strings.Contains(e.logs.String(), "commits ignorés") { // message of the Warn log
		t.Error("ignored commit not reported")
	}
}

// Repository without a known forge identifier: read by owner/name.
func TestDepotSansIdentifiant(t *testing.T) {
	e := newEnv(t)
	e.fake.AddRepository(provider.Repository{ID: 5, Owner: testOwner, Name: "ancien"})
	e.fake.AppendCommits(5, "main", 1, epoch, time.Hour)
	repo, err := e.st.CreateRepository(t.Context(), model.Repository{Provider: model.ProviderGitHub,
		Owner: testOwner, Name: "ancien", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if r := e.syncOK(repo.ID); r.NewCommits != 1 {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 1 {
		t.Errorf("Repository called %d times", n)
	}
}

// assertReference checks that the branch ref of the repository repoID carries exactly want (SHAs),
// that last_commit_at is last and that the repository is not "empty" for the activity.
func (e *env) assertReference(repoID int64, ref string, want []string, last time.Time) {
	e.t.Helper()
	if got := e.commitSHAs(repoID, ref); !slices.Equal(got, sorted(want...)) {
		e.t.Errorf("commits of %s = %v, want %v", ref, got, sorted(want...))
	}
	stored, err := e.st.GetRepository(e.t.Context(), repoID)
	if err != nil {
		e.t.Fatalf("GetRepository: %v", err)
	}
	if !stored.LastCommitAt.Equal(last) {
		e.t.Errorf("last_commit_at = %v, want %v", stored.LastCommitAt, last)
	}
	res := activity.Evaluate(stored, e.lastRun(repoID), e.cfg.Activity, e.clock.Now(), time.UTC)
	if res.State == activity.StateEmpty || res.State == activity.StateError {
		e.t.Errorf("activity state = %q after the change of reference", res.State)
	}
}

// Default branch renamed on the forge (main → trunk, same head, main deleted): trunk is linked to
// the whole history, the repository keeps its last commit and is not "empty".
func TestReferenceRenommee(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 3, epoch, time.Hour)
	e.syncOK(repo.ID)

	e.fake.UpdateRepository(provider.Repository{ID: 1, Owner: testOwner, Name: "projet", DefaultBranch: "trunk"})
	e.fake.SetBranch(1, "trunk", mainC[2].SHA)
	e.fake.DeleteBranch(1, "main")
	e.fake.ResetCalls()

	if r := e.syncOK(repo.ID); r.NewCommits != 0 || r.TrackedBranches != 1 {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodCommitStats); n != 0 {
		t.Errorf("CommitStats called %d times for commits already in the database", n)
	}
	if b := e.branches(repo.ID); b["main"].Tracked || !b["trunk"].Tracked {
		t.Errorf("branches = %+v", b)
	}
	e.assertReference(repo.ID, "trunk", []string{mainC[0].SHA, mainC[1].SHA, mainC[2].SHA}, mainC[2].CommittedAt)

	// Next sync, nothing moved: no new read.
	e.fake.ResetCalls()
	e.syncOK(repo.ID)
	if n := e.fake.Calls(providertest.MethodListCommits); n != 0 {
		t.Errorf("ListCommits called %d times after the change", n)
	}
}

// --branch patterns that afterwards exclude the default branch: the new reference (dev, already
// known, unchanged head) is linked to its whole history.
func TestMotifsExcluantLaReference(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	mainC := e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)
	e.fake.SetBranch(1, "dev", mainC[1].SHA)
	e.syncOK(repo.ID)
	if got := e.commitSHAs(repo.ID, "dev"); len(got) != 0 {
		t.Fatalf("dev, branched off main, already has its own commits: %v", got)
	}

	if _, err := e.st.UpdateRepositorySettings(t.Context(), repo.ID, "", []string{"dev"}); err != nil {
		t.Fatal(err)
	}
	e.fake.ResetCalls()
	if r := e.syncOK(repo.ID); r.NewCommits != 0 || r.TrackedBranches != 1 {
		t.Errorf("rapport = %+v", r)
	}
	if n := e.fake.Calls(providertest.MethodListCommits); n != 1 {
		t.Errorf("ListCommits called %d times, want 1", n)
	}
	e.assertReference(repo.ID, "dev", []string{mainC[0].SHA, mainC[1].SHA}, mainC[1].CommittedAt)
}
