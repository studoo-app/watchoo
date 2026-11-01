package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

func TestStatus(t *testing.T) {
	h := newSyncedWorkspace(t)
	// A repository in error, one never synced, one stopped.
	st := openTestStore(t, h)
	bob := storedRepo(t, h, "bob", "projet")
	run, err := st.BeginSyncRun(context.Background(), bob.ID, testNow, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishSyncRun(context.Background(), run.ID, testNow, model.SyncError, 0, "access lost"); err != nil {
		t.Fatal(err)
	}
	h.Provider.AddRepository(provider.Repository{ID: 105, Owner: "eve", Name: "projet"})
	h.run("repo", "add", "eve/projet").mustSucceed(t)
	h.run("repo", "remove", "dave/projet", "--keep-history", "--yes").mustSucceed(t)
	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 4321, Reset: testNow.Add(20 * time.Minute)})
	rateCalls := h.Provider.Calls(providertest.MethodRateLimit)

	r := h.run("status").mustSucceed(t)
	// "just now" comes from internal/activity (Relative), a layer translated separately.
	want := fmt.Sprintf(`Workspace: licence-projet (%s)
gh account: watchoo-test on github.com (scopes: gist, read:org, repo)
Source: GitHub API (gh signed in)
GitHub quota: 4321 / 5000 requests left (resets at 12:20).

Repositories: 4
  up to date (Active)    1
  Flemme                 0
  Dead                   0
  in error               1   bob/projet
  never synced           1
  tracking stopped       1

Last sync: just now (27/10/2026 12:00)
Tracked branches: 3
`, displayPath(h.cwd))
	if r.stdout != want || r.stderr != "" {
		t.Errorf("status =\n%s\nwant:\n%s\nstderr: %s", r.stdout, want, r.stderr)
	}
	if n := h.Provider.Calls(providertest.MethodRateLimit) - rateCalls; n != 1 {
		t.Errorf("RateLimit called %d times, want 1", n)
	}

	r = h.run("status", "--json").mustSucceed(t)
	var rep statusReport
	decodeStrict(t, r.stdout, &rep)
	if !rep.GitHub.OK || rep.GitHub.Login != authtest.FakeLogin || rep.GitHub.TokenSource != "keyring" ||
		rep.RateLimit == nil || rep.RateLimit.Remaining != 4321 || rep.RateLimitError != "" ||
		rep.Repositories != (statusRepoCounts{Total: 4, Active: 1, Error: 1, NeverSynced: 1, Stopped: 1}) ||
		len(rep.Errors) != 1 || rep.Errors[0] != (statusRepoError{Repo: "bob/projet", Message: "access lost"}) ||
		rep.LastSyncedAt == nil || rep.TrackedBranches != 3 || rep.Workspace.Name != "licence-projet" {
		t.Errorf("status --json = %+v", rep)
	}
	if rep.QuotaLow {
		t.Error("status --json: quota_low true with 4321 / 5000")
	}
	assertJSONKeys(t, r.stdout, "", "workspace", "github", "source", "rate_limit", "rate_limit_error", "quota_low",
		"repositories", "errors", "last_synced_at", "tracked_branches")
	assertJSONKeys(t, r.stdout, "source", "kind", "reason")
	if rep.Source != (statusSource{Kind: wsync.SourceAPI}) {
		t.Errorf("status --json: source %+v, want api", rep.Source)
	}
	assertJSONKeys(t, r.stdout, "github", "ok", "host", "login", "token_source", "scopes", "error")
	assertJSONKeys(t, r.stdout, "repositories", "total", "active", "flemme", "dead", "error", "never_synced",
		"archived", "empty", "stopped")
	assertJSONKeys(t, r.stdout, "workspace", "name", "root")
}

func TestStatusEmptyWorkspace(t *testing.T) {
	h := newWorkspace(t)
	r := h.run("status").mustSucceed(t)
	assertContains(t, "empty status", r.stdout, "gh account: watchoo-test", "GitHub quota: 5000 / 5000",
		"No tracked repository.")
	assertNotContains(t, "empty status", r.stdout, "Last sync")
}

// Rate limit below 10 %: status reports the safeguard (same rule as the engine), in text and in JSON;
// after the reset, nothing any more.
func TestStatusQuotaLow(t *testing.T) {
	h := newWorkspace(t)
	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 300, Reset: testNow.Add(20 * time.Minute)})
	r := h.run("status").mustSucceed(t)
	assertContains(t, "status low quota", r.stdout,
		"GitHub quota: 300 / 5000 requests left (resets at 12:20).\n"+
			"Low quota: syncs postponed until 12:20 (less than 10% of the quota left).\n")
	r = h.run("status", "--json").mustSucceed(t)
	var rep statusReport
	decodeStrict(t, r.stdout, &rep)
	if !rep.QuotaLow || rep.RateLimit == nil || rep.RateLimit.Remaining != 300 {
		t.Errorf("status --json low quota = %+v", rep)
	}

	h.Provider.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 500, Reset: testNow.Add(20 * time.Minute)})
	r = h.run("status").mustSucceed(t)
	assertNotContains(t, "status quota at 10 %", r.stdout, "Low quota")
}

// gh signed out: status reports it without failing, without asking for the rate limit, and announces
// the local Git clones as the source of the syncs.
func TestStatusLoggedOut(t *testing.T) {
	h := newWorkspace(t)
	h.Client = authtest.NewLoggedOut()
	r := h.run("status").mustSucceed(t)
	assertContains(t, "status signed out", r.stdout, "gh account: checking gh: ", "gh auth login",
		"Source: local Git clones", "GitHub quota: unknown (not applicable: local Git clones, no quota)")
	r = h.run("status", "--json").mustSucceed(t)
	var rep statusReport
	decodeStrict(t, r.stdout, &rep)
	if rep.GitHub.OK || rep.GitHub.Error == "" || rep.RateLimit != nil || rep.RateLimitError == "" ||
		rep.GitHub.Scopes == nil || rep.Source.Kind != wsync.SourceGit || rep.Source.Reason == "" {
		t.Errorf("status --json signed out = %+v", rep)
	}
	if len(h.tokens) != 0 {
		t.Error("the provider was built although gh is signed out")
	}

	// Unreadable rate limit: reported, without failure.
	h.Client = authtest.NewLoggedIn("")
	h.Provider.SetError(providertest.MethodRateLimit, provider.ErrUnavailable)
	r = h.run("status").mustSucceed(t)
	assertContains(t, "status quota in error", r.stdout, "GitHub quota: unknown (reading the quota: GitHub is unreachable")
}

// On a terminal, status starts with the header card; never in JSON nor off a terminal.
func TestStatusCard(t *testing.T) {
	h := newWorkspace(t)
	h.Terminal = func() TermInfo { return wideTerminal }
	r := h.run("status").mustSucceed(t)
	card := cardFor(wideTerminal, `Status of workspace "licence-projet"`, i18n.T("cli.core.tip.status"))
	if !strings.HasPrefix(r.stdout, card+"\n") {
		t.Errorf("status without the card at the top:\n%s", r.stdout)
	}
	assertContains(t, "status on a terminal", ansiSeq.ReplaceAllString(r.stdout, ""),
		i18n.T("cli.core.tip.status"), "gh account: watchoo-test")
	r = h.run("status", "--json").mustSucceed(t)
	if !strings.HasPrefix(r.stdout, "{") {
		t.Errorf("status --json with a card:\n%s", r.stdout)
	}
	h.Terminal = nil
	if r := h.run("status").mustSucceed(t); strings.Contains(r.stdout, "╭") || strings.Contains(r.stdout, i18n.T("cli.core.tip.status")) {
		t.Errorf("redirected status with a card:\n%s", r.stdout)
	}
}
