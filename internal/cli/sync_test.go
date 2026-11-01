package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

func TestSyncAll(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet", "--label", "Alice Martin").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)
	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 4000, Reset: testNow.Add(30 * time.Minute)})

	r := h.run("sync").mustSucceed(t)
	if row := tableRow(t, r.stdout, "Alice Martin"); len(row) != 5 || row[1] != "ok" || row[2] != "3" || row[3] != "2" {
		t.Errorf("alice/projet line = %q", row)
	}
	if row := tableRow(t, r.stdout, "bob/projet"); row[1] != "ok" || row[2] != "1" || row[3] != "1" {
		t.Errorf("bob/projet line = %q", row)
	}
	assertContains(t, "sync", r.stdout, "REPOSITORY", "STATUS", "NEW COMMITS",
		": 2 repositories, 4 new commits.\n", "GitHub quota: ", "requests left (resets at 12:30).")
	if r.stderr != "" {
		t.Errorf("unexpected stderr:\n%s", r.stderr)
	}

	// Second sync: incremental, nothing new; in JSON.
	h.Provider.ResetCalls()
	r = h.run("sync", "--json").mustSucceed(t)
	var rep syncReport
	decodeStrict(t, r.stdout, &rep)
	if rep.Summary != (syncSummary{Repositories: 2, Success: 2}) || len(rep.Repositories) != 2 ||
		rep.Repositories[0].DisplayName != "Alice Martin" || rep.Repositories[0].Status != wsync.StatusSuccess ||
		rep.Repositories[0].TrackedBranches != 2 || rep.RateLimit == nil || rep.RateLimit.Limit != 5000 ||
		rep.StartedAt == nil || rep.FinishedAt == nil {
		t.Errorf("sync --json = %+v", rep)
	}
	if n := h.Provider.Calls(providertest.MethodListCommits) + h.Provider.Calls(providertest.MethodCompareCommits); n != 0 {
		t.Errorf("second sync: %d commit requests, want 0 (heads unchanged)", n)
	}
	assertJSONKeys(t, r.stdout, "", "started_at", "finished_at", "summary", "repositories", "rate_limit", "source")
	if rep.Source != wsync.SourceAPI {
		t.Errorf("sync --json: source %q, want api", rep.Source)
	}
	assertJSONKeys(t, r.stdout, "summary", "repositories", "success", "error", "locked", "deferred", "new_commits")
	assertJSONKeys(t, r.stdout, "repositories.0", "id", "full_name", "display_name", "status", "new_commits",
		"tracked_branches", "stats_fetched", "duration_ms", "error", "resume_at", "warnings")
	assertJSONKeys(t, r.stdout, "rate_limit", "limit", "remaining", "used", "reset")

	// The token is asked from gh again at every sync.
	if n := h.fake().TokenCalls(); n != 4 {
		t.Errorf("Token called %d times, want 4 (2 repo add, 2 sync)", n)
	}
}

func TestSyncOneRepo(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)

	r := h.run("sync", "https://github.com/bob/projet", "--json").mustSucceed(t)
	var rep syncReport
	decodeStrict(t, r.stdout, &rep)
	if len(rep.Repositories) != 1 || rep.Repositories[0].FullName != "bob/projet" || rep.Repositories[0].NewCommits != 1 {
		t.Errorf("sync bob/projet = %+v", rep)
	}
	h.run("sync", "carol/projet").mustFail(t, "repository carol/projet is not tracked in this workspace")
	h.run("sync", "pas un dépôt").mustFail(t, "pas un dépôt")
}

func TestSyncEmptyAndErrors(t *testing.T) {
	h := newWorkspace(t)
	r := h.run("sync").mustSucceed(t)
	assertContains(t, "sync without a repository", r.stdout, "No repository to sync", "watchoo repo add owner/repo")
	r = h.run("sync", "--json").mustSucceed(t)
	var rep syncReport
	decodeStrict(t, r.stdout, &rep)
	if len(rep.Repositories) != 0 || rep.Repositories == nil {
		t.Errorf("sync --json without a repository = %+v", rep)
	}

	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)

	// A repository in error does not stop the others; non-zero exit code.
	h.Provider.SetRepoError("alice", "projet", &provider.APIError{Op: "reading repository alice/projet",
		Status: 404, Err: provider.ErrNotFound})
	r = h.run("sync").mustCode(t, 1)
	if row := tableRow(t, r.stdout, "alice/projet"); row[1] != "error" || !strings.Contains(r.stdout, "not found") {
		t.Errorf("alice/projet line = %q\n%s", row, r.stdout)
	}
	if row := tableRow(t, r.stdout, "bob/projet"); row[1] != "ok" {
		t.Errorf("bob/projet line = %q", row)
	}
	assertContains(t, "sync in error", r.stderr, "error: sync finished with 1 repository in error")
	r = h.run("repo", "list").mustSucceed(t)
	if row := tableRow(t, r.stdout, "alice/projet"); row[len(row)-1] != "error" {
		t.Errorf("alice/projet state = %q, want \"error\"", row)
	}

	// gh signed out: sync through local Git clones; repositories missing from the Git server: access denied.
	h.Client = authtest.NewLoggedOut()
	h.GitURL = gittest.NewServer(t).URL
	r = h.run("sync").mustCode(t, 1)
	assertContains(t, "sync without gh", r.stdout, "access denied", "local Git clones")
}

func TestSyncDeferredAndLocked(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)

	// Rate limit below 10 %: repositories deferred, resume time in local time (UTC here).
	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 100, Reset: testNow.Add(45 * time.Minute)})
	r := h.run("sync").mustSucceed(t)
	row := tableRow(t, r.stdout, "alice/projet")
	if row[1] != "deferred" || !strings.Contains(r.stdout, "deferred: low quota until 12:45") {
		t.Errorf("sync low quota = %q\n%s", row, r.stdout)
	}
	assertContains(t, "sync low quota", r.stdout, "2 deferred")
	r = h.run("sync", "--json").mustSucceed(t)
	var rep syncReport
	decodeStrict(t, r.stdout, &rep)
	if rep.Summary.Deferred != 2 || rep.Repositories[0].ResumeAt == nil ||
		!rep.Repositories[0].ResumeAt.Equal(testNow.Add(45*time.Minute)) {
		t.Errorf("sync --json low quota = %+v", rep)
	}

	// Lock: a recent "running" sync_run makes the repository ignored.
	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 5000})
	st := openTestStore(t, h)
	alice := storedRepo(t, h, "alice", "projet")
	if _, err := st.BeginSyncRun(context.Background(), alice.ID, testNow.Add(-time.Minute), wsync.LockStaleAfter); err != nil {
		t.Fatal(err)
	}
	r = h.run("sync").mustSucceed(t)
	if row := tableRow(t, r.stdout, "alice/projet"); row[1] != "skipped" ||
		!strings.Contains(r.stdout, "already being synced") {
		t.Errorf("locked sync = %q\n%s", row, r.stdout)
	}
	assertContains(t, "locked sync", r.stdout, "1 already running")
}

func TestSyncCanceled(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := h.runContext(ctx, "sync")
	r.mustFail(t)
	assertContains(t, "canceled sync", r.stderr, "command interrupted")
}

func TestDurationText(t *testing.T) {
	// English: "." decimal separator (French gives "0,0 s", "1,2 s").
	for d, want := range map[time.Duration]string{
		-time.Second:                         "0 s",
		0:                                    "0.0 s",
		1234 * time.Millisecond:              "1.2 s",
		12 * time.Second:                     "12 s",
		3*time.Minute + 5*time.Second:        "3 min 05 s",
		75*time.Minute + 59*time.Second:      "75 min 59 s",
		9*time.Second + 990*time.Millisecond: "9.9 s",
	} {
		if got := durationText(d); got != want {
			t.Errorf("durationText(%s) = %q, want %q", d, got, want)
		}
	}
}

// pluralT: singular for 1, and also for 0 in French ("0 nouveau commit"), plural otherwise ("0 new
// commits" in English). Not parallel: the process language is shared.
func TestPluralT(t *testing.T) {
	for n, want := range map[int]string{0: "0 new commits", 1: "1 new commit", 2: "2 new commits"} {
		if got := pluralT(n, "cli.sync.count_new_commit_one", "cli.sync.count_new_commit_many"); got != want {
			t.Errorf("pluralT(%d) = %q, want %q", n, got, want)
		}
	}
	defer i18n.SetProcessForTest(i18n.French)()
	for n, want := range map[int]string{0: "0 nouveau commit", 1: "1 nouveau commit", 2: "2 nouveaux commits"} {
		if got := pluralT(n, "cli.sync.count_new_commit_one", "cli.sync.count_new_commit_many"); got != want {
			t.Errorf("pluralT(%d) in French = %q, want %q", n, got, want)
		}
	}
}

// syncDetail translates the reason of a deferral from RepoReport.DeferReason; without it (report
// written by an older version), the reason quoted by the message is recognized in both languages.
func TestSyncDetailDeferred(t *testing.T) {
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	s := &session{loc: time.UTC, deps: Deps{Now: func() time.Time { return now }}}
	resume := now.Add(time.Hour)
	for _, tc := range []struct {
		r    wsync.RepoReport
		want string
	}{
		{wsync.RepoReport{DeferReason: wsync.DeferQuotaExhausted, Error: "x"}, "deferred: quota exhausted until 11:00"},
		{wsync.RepoReport{DeferReason: wsync.DeferLowQuota}, "deferred: low quota until 11:00"},
		{wsync.RepoReport{Error: "reporté : quota épuisé jusqu'à 11:00 UTC"}, "deferred: quota exhausted until 11:00"},
		{wsync.RepoReport{Error: "deferred: quota exhausted until 11:00 UTC"}, "deferred: quota exhausted until 11:00"},
		{wsync.RepoReport{Error: "reporté : quota bas jusqu'à 11:00 UTC"}, "deferred: low quota until 11:00"},
	} {
		tc.r.Status, tc.r.ResumeAt = wsync.StatusDeferred, resume
		if got := s.syncDetail(tc.r); got != tc.want {
			t.Errorf("syncDetail(%q, %q) = %q, want %q", tc.r.DeferReason, tc.r.Error, got, tc.want)
		}
	}
	defer i18n.SetProcessForTest(i18n.French)()
	r := wsync.RepoReport{Status: wsync.StatusDeferred, ResumeAt: resume, DeferReason: wsync.DeferQuotaExhausted}
	if got := s.syncDetail(r); got != "reporté : quota épuisé jusqu'à 11:00" {
		t.Errorf("syncDetail in French = %q", got)
	}
}
