package web

// End-to-end test of phase 4: the full web interface (router, middlewares, CSRF), the real sync
// engine and the real coordinator, the real GitHub provider (internal/provider/github) plugged on the
// fake githubtest server, a fake gh (authtest) and a temporary SQLite database. No real network call
// (127.0.0.1); dummy data only. The harness checks that no response and no log contains the token
// or a dummy e-mail.

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/github"
	"github.com/studoo-app/watchoo/internal/provider/github/githubtest"
)

func TestWebEndToEnd(t *testing.T) {
	srv := githubtest.NewServer(t, githubtest.Options{Now: func() time.Time { return testNow },
		Token: authtest.FakeToken})
	t.Cleanup(srv.Close)
	cache, quota := github.NewCache(0, 0), github.NewQuota()
	e := newTestEnv(t, withProvider(func(token string) (provider.Provider, error) {
		return github.New(token, github.Options{BaseURL: srv.URL(), Cache: cache, Quota: quota,
			UserAgent: "watchoo/test", Logger: slog.Default()})
	}))

	alice := githubtest.Person{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh"}
	bob := githubtest.Person{Name: "Bob Durand", Email: "bob@example.test"} // without a GitHub account
	carol := githubtest.Person{Name: "Carol", Email: "carol@example.test", Login: "carol-gh"}
	commit := func(repo int64, msg string, p githubtest.Person, ago time.Duration, parents ...string) string {
		return srv.AddCommit(repo, githubtest.Commit{Message: msg, Author: p, AuthoredAt: testNow.Add(-ago),
			Parents: parents, Additions: 3, Deletions: 1})
	}
	// alice/projet: 60 old commits (pagination), then two recent commits, and a dev branch.
	srv.AddRepo(githubtest.Repo{ID: 1, Owner: "alice", Name: "projet", Private: true})
	old := srv.AppendCommits(1, "main", 60, testNow.Add(-80*24*time.Hour), time.Hour)
	a1 := commit(1, "e2e-alice-tests", alice, 50*time.Hour, old[len(old)-1])
	a2 := commit(1, xssMessage, alice, 2*time.Hour, a1)
	srv.SetBranch(1, "main", a2)
	srv.SetBranch(1, "dev", commit(1, "e2e-bob-essai-dev", bob, time.Hour, a2))
	// An open pull request and a tag (V2).
	srv.SetPullRequests(1, githubtest.PullRequest{Number: 7, Title: "e2e-pr-ouverte", Author: "alice-gh",
		CreatedAt: testNow.Add(-3 * time.Hour)})
	srv.SetTags(1, githubtest.Tag{Name: "v1.0", SHA: a2})
	// bob/projet: Flemme (9 days); carol/projet: Dead (42 days); cache: inaccessible.
	srv.AddRepo(githubtest.Repo{ID: 2, Owner: "bob", Name: "projet", Private: true})
	srv.SetBranch(2, "main", commit(2, "e2e-bob-init", bob, 9*24*time.Hour))
	srv.AddRepo(githubtest.Repo{ID: 3, Owner: "carol", Name: "projet", Private: true})
	srv.SetBranch(3, "main", commit(3, "e2e-carol-rendu", carol, 42*24*time.Hour))
	srv.AddRepo(githubtest.Repo{ID: 4, Owner: "carol", Name: "cache", Private: true})
	srv.SetRepoAccess(4, githubtest.AccessHidden)

	// 1. Empty workspace: the dashboard points to the settings.
	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, testWorkspace, `href="/settings"`)

	// 2. Adding the repositories from the settings (POST with CSRF); the hidden repository is refused.
	addRepo := func(ref, label, branches string) int64 {
		t.Helper()
		w := e.post("/settings/repos", url.Values{"repo": {ref}, "label": {label}, "branches": {branches}}, false)
		assertStatus(t, w, http.StatusSeeOther)
		loc, err := url.Parse(w.Header().Get("Location"))
		if err != nil || loc.Path != "/settings" || loc.Query().Get("msg") != msgAdded {
			t.Fatalf("adding %s: Location %q", ref, w.Header().Get("Location"))
		}
		id, err := strconv.ParseInt(loc.Query().Get("repo"), 10, 64)
		if err != nil || id <= 0 {
			t.Fatalf("adding %s: identifier %q", ref, loc.Query().Get("repo"))
		}
		return id
	}
	aliceID := addRepo("alice/projet", "Alice Martin", "main, dev")
	bobID := addRepo("https://github.com/bob/projet", "", "")
	carolID := addRepo("carol/projet", "", "")
	w = e.post("/settings/repos", url.Values{"repo": {"carol/cache"}}, false)
	assertStatus(t, w, http.StatusUnprocessableEntity)
	if repos, err := e.store.ListRepositories(t.Context()); err != nil || len(repos) != 3 {
		t.Fatalf("repositories in the database = %d (%v), want 3", len(repos), err)
	}
	// A request without CSRF token creates nothing.
	r := e.request(http.MethodPost, "/settings/repos", strings.NewReader("repo=carol%2Fautre"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", testOrigin)
	assertStatus(t, e.do(r), http.StatusForbidden)

	w = e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Never synced", "Alice Martin", "bob/projet")

	// 3. Manual sync from the button (HTMX), progress, then end reported by HX-Trigger.
	w = e.post("/sync", nil, true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="sync-status"`, "Sync started.")
	st := e.waitSyncIdle()
	if !st.HasReport || st.LastReport.NewCommits() != 65 {
		t.Fatalf("sync report = %+v", st.LastReport)
	}
	w = e.htmx("/fragments/sync?gen=0&run=1")
	assertStatus(t, w, http.StatusOK)
	if got := w.Header().Get("HX-Trigger"); got != SyncFinishedEvent {
		t.Errorf("HX-Trigger = %q, want %q", got, SyncFinishedEvent)
	}
	assertContains(t, w, "Last sync")
	assertNotContains(t, w, "every 2s")

	// 4. Dashboard: the three states, the feed, the escaped message.
	w = e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "badge-success", "badge-warning", "badge-error", "Slacking · 9 d", "Dead · 42 d",
		"&lt;script&gt;", "e2e-bob-essai-dev")
	assertNotContains(t, w, xssMessage, "Never synced")
	// Page content only: the sync status (navbar) may quote repositories in the tooltip of its
	// warnings (local Git clones unavailable in this test).
	body := w.Body.String()
	body = body[strings.Index(body, `id="main"`):]
	if c, b, a := strings.Index(body, "carol/projet"), strings.Index(body, "bob/projet"),
		strings.Index(body, "Alice Martin"); c < 0 || b < 0 || a < 0 || c > b || b > a {
		t.Errorf("order of the cards: carol %d, bob %d, alice %d (want Dead, Flemme, Active)", c, b, a)
	}

	// 5. Detail of alice: 63 commits (60 + 2 on main, 1 on dev), two pages of 50, branch and author filters, chart.
	alicePath := repoPath(aliceID)
	w = e.get(alicePath)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Alice Martin", `id="activity-data"`, "&lt;script&gt;", "Sync this repository",
		"#7 e2e-pr-ouverte", "1 open", ">v1.0<")
	assertNotContains(t, w, xssMessage)
	// Page 1: the 50 most recent (3 recent commits, then "commit 60" to "commit 14").
	assertContains(t, w, "commit 14", "?page=2")
	assertNotContains(t, w, "commit 13<")
	w = e.htmx(alicePath + "/commits?page=2")
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, "<html", "e2e-alice-tests", "commit 14<")
	assertContains(t, w, "commit 13", "commit 1<", "?page=1")
	w = e.htmx(alicePath + "/commits?branch=dev")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "e2e-bob-essai-dev")
	w = e.htmx(alicePath + "/commits?author=alice-gh")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "e2e-alice-tests", "&lt;script&gt;")
	assertNotContains(t, w, "e2e-bob-essai-dev", "commit 12")

	// 6. Sync of a single repository: nothing new, no commit downloaded again.
	w = e.post(repoPath(bobID)+"/sync", nil, true)
	assertStatus(t, w, http.StatusOK)
	if st := e.waitSyncIdle(); st.LastReport.NewCommits() != 0 {
		t.Errorf("second sync: %d new commits, want 0", st.LastReport.NewCommits())
	}

	// 7. Contributors: global ranking with the four authors.
	w = e.get("/contributors")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Alice Martin", "Bob Durand", "Carol", "Étudiant Test", repoPath(carolID))

	// 7 bis. A contributor's page (0.5.0): linked from the ranking, with the profile given by the
	// GitHub API (html_url of the account), their repositories and their commits.
	aliceContrib := e.contributorIDs()["Alice Martin"]
	assertContains(t, w, `href="`+contributorPath(aliceContrib)+`"`)
	w = e.get(contributorPath(aliceContrib))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `href="https://github.com/alice-gh"`, `aria-label="GitHub profile of alice-gh"`,
		repoPath(aliceID), "e2e-alice-tests")
	assertNoLeak(t, "contributor page", w.Body.String())

	// 8. Removal from the settings: tracking stopped (history kept), then deletion.
	w = e.get("/settings/repos/" + strconv.FormatInt(carolID, 10) + "/remove")
	assertStatus(t, w, http.StatusOK)
	w = e.post("/settings/repos/"+strconv.FormatInt(carolID, 10)+"/remove", url.Values{"keep_history": {"1"}}, false)
	assertStatus(t, w, http.StatusSeeOther)
	if repo, err := e.store.GetRepository(t.Context(), carolID); err != nil || repo.Enabled {
		t.Errorf("carol after the tracking stopped: %+v (%v)", repo, err)
	}
	assertStatus(t, e.get(repoPath(carolID)), http.StatusOK) // history kept
	w = e.post("/settings/repos/"+strconv.FormatInt(bobID, 10)+"/remove", nil, false)
	assertStatus(t, w, http.StatusSeeOther)
	assertStatus(t, e.get(repoPath(bobID)), http.StatusNotFound)
	w = e.get("/settings")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Alice Martin", "carol/projet")
	if main := w.Body.String()[strings.Index(w.Body.String(), `id="main"`):]; strings.Contains(main, "bob/projet") {
		t.Error("bob/projet still in the settings after its deletion")
	}

	// 9. The fake GitHub only received reads, all with the token (never sent to the browser).
	for _, req := range srv.Requests() {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			t.Errorf("GitHub request %s %s: the access must remain read-only", req.Method, req.Path)
		}
		if !req.ValidToken {
			t.Errorf("GitHub request %s without the expected token", req.Path)
		}
	}
}

// End to end with the conventions enabled (1.7.0): a repository with a conforming commit, two commits
// without type and a merge (excluded), synced from the button through the real GitHub provider; the
// repository section and the contributor line show the rates, never a commit message. No local clone
// here: the scope falls back on the database.
func TestWebEndToEndConventions(t *testing.T) {
	srv := githubtest.NewServer(t, githubtest.Options{Now: func() time.Time { return testNow },
		Token: authtest.FakeToken})
	t.Cleanup(srv.Close)
	cache, quota := github.NewCache(0, 0), github.NewQuota()
	e := newTestEnv(t, withConventions(), withProvider(func(token string) (provider.Provider, error) {
		return github.New(token, github.Options{BaseURL: srv.URL(), Cache: cache, Quota: quota,
			UserAgent: "watchoo/test", Logger: slog.Default()})
	}))
	alice := githubtest.Person{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh"}
	commit := func(msg string, ago time.Duration, parents ...string) string {
		return srv.AddCommit(1, githubtest.Commit{Message: msg, Author: alice, AuthoredAt: testNow.Add(-ago),
			Parents: parents, Additions: 3, Deletions: 1})
	}
	srv.AddRepo(githubtest.Repo{ID: 1, Owner: "alice", Name: "projet", Private: true})
	c1 := commit("e2e-premier-jet", 5*time.Hour)
	c2 := commit("feat: e2e-ajoute la page", 4*time.Hour, c1)
	d1 := commit("e2e-essai-dev", 3*time.Hour, c1)
	merge := commit("Merge branch 'dev' e2e-fusion", 2*time.Hour, c2, d1)
	srv.SetBranch(1, "main", merge)
	srv.SetBranch(1, "dev", d1)
	// Phase 8.2: a pull request whose title is not conventional, from a head branch without type, and a
	// tag that designates the milestone v1.0.0 under a nonconforming name.
	srv.SetPullRequests(1, githubtest.PullRequest{Number: 7, Title: "e2e-titre-de-pr", Author: "alice-gh",
		Head: "Ma_Branche", CreatedAt: testNow.Add(-90 * time.Minute)})
	srv.SetTags(1, githubtest.Tag{Name: "1.0.0", SHA: c2})

	w := e.post("/settings/repos", url.Values{"repo": {"alice/projet"}}, false)
	assertStatus(t, w, http.StatusSeeOther)
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	id, err := strconv.ParseInt(loc.Query().Get("repo"), 10, 64)
	if err != nil {
		t.Fatalf("repository identifier %q", loc.Query().Get("repo"))
	}
	assertStatus(t, e.post("/sync", nil, true), http.StatusOK)
	if st := e.waitSyncIdle(); !st.HasReport || st.LastReport.NewCommits() != 4 {
		t.Fatalf("sync report = %+v", st.LastReport)
	}

	messages := []string{"e2e-premier-jet", "e2e-ajoute", "e2e-essai-dev", "e2e-fusion"}
	w = e.get(repoPath(id))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="repo-conventions"`, "1/3", "33%", "Excluded commits, counted apart: 1 (merges 1)",
		"<code>no_type</code>", "Latest non-conforming commits (2 of 2)", "may include rewritten commits")
	section := conventionsSection(t, w.Body.String())
	for _, m := range messages {
		if strings.Contains(section, m) {
			t.Errorf("commit message %q in the conventions section", m)
		}
	}
	// Refresh of the fragment after a sync.
	w = e.htmxWith(repoPath(id), map[string]string{"HX-Target": repoConventionsID})
	assertContains(t, w, `id="repo-conventions"`, "1/3")
	assertNotContains(t, w, messages...)

	w = e.get("/contributors/" + strconv.FormatInt(e.contributorIDs()["Alice Martin"], 10))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="contributor-conventions"`, "Conforming commits 1/3 (33%)", "1 excluded",
		`id="contributor-conventions-pulls"`, "open 0/1")

	// Phase 8.2: the branch, the pull request (number and codes, never its title) and the tag.
	w = e.get(repoPath(id))
	assertContains(t, w, "Ma_Branche", "<code>"+conventions.BranchUppercase+"</code>", "<code>underscore</code>", "#7",
		"1.0.0 · not conforming", "<code>no_v</code>", "v1.0.0 · nonconforming name")
	// The pull requests section (V2) shows the title; the conventions section never does.
	if strings.Contains(conventionsSection(t, w.Body.String()), "e2e-titre-de-pr") {
		t.Error("pull request title in the conventions section")
	}
	w = e.get("/conventions")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="conventions-repos"`, "1/3 (33%)", "open 0/1", "Review of the milestone v1.0.0",
		"Not evaluable: missing start date.")
	assertNotContains(t, w, append(messages, "e2e-titre-de-pr")...)
}
