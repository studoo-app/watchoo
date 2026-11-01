package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
)

// repoCount returns the number of repositories in the database.
func (e *testEnv) repoCount() int {
	e.t.Helper()
	repos, err := e.store.ListRepositories(e.t.Context())
	if err != nil {
		e.t.Fatal(err)
	}
	return len(repos)
}

// mustRepo reads repository id.
func (e *testEnv) mustRepo(id int64) model.Repository {
	e.t.Helper()
	r, err := e.store.GetRepository(e.t.Context(), id)
	if err != nil {
		e.t.Fatalf("GetRepository(%d): %v", id, err)
	}
	return r
}

// assertRedirect checks a 303 redirection to want.
func assertRedirect(t *testing.T, w interface {
	Header() http.Header
}, code int, want string) {
	t.Helper()
	if code != http.StatusSeeOther {
		t.Fatalf("code %d, want 303", code)
	}
	if got := w.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestSettingsPage(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	if _, err := e.store.UpdateRepositorySettings(t.Context(), ids.Bob, "<script>bob</script>", []string{"main", "release/*"}); err != nil {
		t.Fatal(err)
	}

	w := e.get("/settings")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		"<title>Settings · "+testWorkspace+" · Watchoo</title>",
		`aria-current="page">Settings`,
		"Alice Martin", "promo/alice", "promo/dave", "all branches", "main, release/*",
		"Active", "Dead · 40 d", "Never synced",
		`action="/settings/repos/`+strconv.FormatInt(ids.Alice, 10)+`"`,
		`href="/settings/repos/`+strconv.FormatInt(ids.Carol, 10)+`/remove"`,
		`action="/settings/repos"`, `action="/settings/interval"`, `action="/sync"`,
		`name="csrf_token"`, "Sync now",
		// Quota loaded after the page.
		`hx-get="/fragments/quota" hx-trigger="load"`,
		// Interval: manual only in the test environment, file at 30 min.
		"Current interval: <strong>manual only</strong>", "this form never rewrites config.yaml",
		"&lt;script&gt;bob&lt;/script&gt;",
	)
	assertNotContains(t, w, "<script>bob", "Resume tracking")
}

func TestSettingsFlashMessages(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	w := e.get("/settings?msg=saved&repo=" + strconv.FormatInt(ids.Alice, 10))
	assertContains(t, w, "The repository &#34;Alice Martin&#34; is changed")
	// Unknown code: no message, nothing from the request is copied.
	w = e.get("/settings?msg=%3Cscript%3E&repo=abc")
	assertStatus(t, w, http.StatusOK)
	assertNotContains(t, w, "alert-info", "&lt;script&gt;")
	// Unknown repository: message without a name.
	w = e.get("/settings?msg=added&repo=999")
	assertContains(t, w, "The repository is tracked")
}

func TestSettingsAddRepository(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	e.fake.AddRepository(provider.Repository{ID: 50, Owner: "promo", Name: "eve", DefaultBranch: "main",
		URL: "https://github.com/promo/eve", Private: true})
	before := e.fake.Calls(providertest.MethodListCommits)

	w := e.post("/settings/repos", url.Values{"repo": {"https://github.com/promo/eve"}, "label": {"Ève"},
		"branches": {"main, dev,main"}}, false)
	repo, err := e.store.GetRepositoryByName(t.Context(), model.ProviderGitHub, "promo", "eve")
	if err != nil {
		t.Fatalf("repository not created: %v", err)
	}
	assertRedirect(t, w, w.Code, "/settings?msg=added&repo="+strconv.FormatInt(repo.ID, 10))
	if repo.Label != "Ève" || strings.Join(repo.BranchPatterns, ",") != "main,dev" || !repo.Enabled {
		t.Errorf("saved repository: %+v", repo)
	}
	// No sync started.
	if e.fake.Calls(providertest.MethodListCommits) != before || e.coord.State().Generation != 0 {
		t.Error("the addition started a sync")
	}
	w = e.get(w.Header().Get("Location"))
	assertContains(t, w, "The repository &#34;Ève&#34; is tracked")

	// Already tracked, without change; then with a new label (HTMX: HX-Redirect).
	w = e.post("/settings/repos", url.Values{"repo": {"promo/eve"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=already&repo="+strconv.FormatInt(repo.ID, 10))
	w = e.post("/settings/repos", url.Values{"repo": {"promo/eve"}, "label": {"Eve B."}}, true)
	assertStatus(t, w, http.StatusOK)
	if got := w.Header().Get("HX-Redirect"); got != "/settings?msg=updated&repo="+strconv.FormatInt(repo.ID, 10) {
		t.Errorf("HX-Redirect = %q", got)
	}
	if got := e.mustRepo(repo.ID); got.Label != "Eve B." || strings.Join(got.BranchPatterns, ",") != "main,dev" {
		t.Errorf("update: %+v (the branches must not change without input)", got)
	}
}

func TestSettingsAddRepositoryErrors(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	count := e.repoCount()
	cases := []struct {
		name   string
		form   url.Values
		status int
		want   []string
	}{
		{"invalid reference", url.Values{"repo": {"<b>not a repository</b>"}}, http.StatusBadRequest,
			[]string{"is invalid", `value="&lt;b&gt;not a repository&lt;/b&gt;"`}},
		{"URL with credentials", url.Values{"repo": {"https://user:secret@github.com/promo/x"}}, http.StatusBadRequest,
			[]string{"identifiants interdits"}},
		{"label with a control character", url.Values{"repo": {"promo/eve"}, "label": {"a\x1b[31mb"}}, http.StatusBadRequest,
			[]string{"caractère de contrôle"}},
		{"invalid branch pattern", url.Values{"repo": {"promo/eve"}, "branches": {"main,[x"}}, http.StatusBadRequest,
			[]string{"branches: motif de branche"}},
		{"inaccessible repository", url.Values{"repo": {"promo/unknown"}, "label": {"Unknown"}}, http.StatusUnprocessableEntity,
			[]string{"Repository promo/unknown inaccessible", `value="Unknown"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := e.post("/settings/repos", tc.form, false)
			assertStatus(t, w, tc.status)
			assertContains(t, w, append(tc.want, "Add a repository", "alert-error")...)
			assertNotContains(t, w, "secret", "<b>not")
			if got := e.repoCount(); got != count {
				t.Errorf("%d repositories after the failure, want %d", got, count)
			}
		})
	}

	// GitHub error (network…): generic message, details in the log only.
	e.fake.SetError(providertest.MethodRepository, errors.New("connection refused"))
	w := e.post("/settings/repos", url.Values{"repo": {"promo/eve"}}, false)
	assertStatus(t, w, http.StatusBadGateway)
	assertContains(t, w, "Adding the repository promo/eve is not possible for now")
	assertNotContains(t, w, "connection refused")
}

func TestSettingsUpdateRepository(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	target := "/settings/repos/" + strconv.FormatInt(ids.Bob, 10)

	w := e.post(target, url.Values{"label": {"Bob D."}, "branches": {"main, feature/*"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=saved&repo="+strconv.FormatInt(ids.Bob, 10))
	if got := e.mustRepo(ids.Bob); got.Label != "Bob D." || strings.Join(got.BranchPatterns, ",") != "main,feature/*" {
		t.Errorf("repository after the change: %+v", got)
	}

	// Invalid input: nothing changes, the form of this repository is opened again with the error.
	w = e.post(target, url.Values{"label": {"ok"}, "branches": {"[x"}}, false)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "Change refused", `open>`, `value="[x"`)
	if got := e.mustRepo(ids.Bob); got.Label != "Bob D." {
		t.Errorf("repository changed despite the error: %+v", got)
	}

	// Empty: no label any more, all branches.
	w = e.post(target, url.Values{"label": {""}, "branches": {" "}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=saved&repo="+strconv.FormatInt(ids.Bob, 10))
	if got := e.mustRepo(ids.Bob); got.Label != "" || len(got.BranchPatterns) != 0 {
		t.Errorf("repository after the reset: %+v", got)
	}

	// Unknown repository: 404, valid input or not.
	assertStatus(t, e.post("/settings/repos/999", url.Values{"label": {"x"}}, false), http.StatusNotFound)
	assertStatus(t, e.post("/settings/repos/999", url.Values{"branches": {"[x"}}, false), http.StatusNotFound)
}

func TestSettingsRemoveRepository(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	confirm := "/settings/repos/" + strconv.FormatInt(ids.Alice, 10) + "/remove"

	// The confirmation changes nothing.
	w := e.get(confirm)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Remove &#34;Alice Martin&#34;?", "3 commits in the database", `name="keep_history" value="1"`,
		`action="`+confirm+`"`, "Delete the repository and its history", "Stop tracking, keep the history")
	if !e.mustRepo(ids.Alice).Enabled {
		t.Fatal("confirmation GET: tracking stopped")
	}
	assertContains(t, w, "the commits in the database (3 commits)")
	assertStatus(t, e.get("/settings/repos/999/remove"), http.StatusNotFound)

	// Non-regression: agreement with 0 commits.
	empty := e.addRepo(99, "empty", "Empty repository")
	w = e.get("/settings/repos/" + strconv.FormatInt(empty, 10) + "/remove")
	assertContains(t, w, "the commits in the database (0 commits)")

	// Keep the history: tracking stopped.
	w = e.post(confirm, url.Values{"keep_history": {"1"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=stopped&repo="+strconv.FormatInt(ids.Alice, 10))
	if e.mustRepo(ids.Alice).Enabled {
		t.Error("tracking not stopped")
	}
	w = e.post(confirm, url.Values{"keep_history": {"1"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=already_stopped&repo="+strconv.FormatInt(ids.Alice, 10))
	w = e.get("/settings")
	assertContains(t, w, "Tracking stopped", "Resume tracking", `name="repo" value="promo/alice"`)
	assertContains(t, e.get(confirm), "tracking already stopped")

	// Resume tracking: like "repo add".
	w = e.post("/settings/repos", url.Values{"repo": {"promo/alice"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=updated&repo="+strconv.FormatInt(ids.Alice, 10))
	if !e.mustRepo(ids.Alice).Enabled {
		t.Error("tracking not resumed")
	}

	// Delete with the history.
	w = e.post(confirm, nil, false)
	assertRedirect(t, w, w.Code, "/settings?msg=deleted")
	if _, err := e.store.GetRepository(t.Context(), ids.Alice); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("repository still in the database: %v", err)
	}
	assertStatus(t, e.post(confirm, nil, false), http.StatusNotFound)
	assertContains(t, e.get("/settings?msg=deleted"), "The repository and its history are deleted")
}

func TestSettingsRemoveRefusedDuringSync(t *testing.T) {
	gate := newGateSyncer()
	e := newTestEnv(t, withSyncer(gate))
	ids := e.seed()
	assertStatus(t, e.post("/sync", nil, true), http.StatusOK)
	<-gate.started

	confirm := "/settings/repos/" + strconv.FormatInt(ids.Bob, 10) + "/remove"
	// The button is not disabled at rendering (it would stay so after the sync): the POST refuses.
	page := e.get(confirm)
	assertContains(t, page, "A sync was running", "Delete the repository and its history")
	assertNotContains(t, page, " disabled")
	w := e.post(confirm, nil, false)
	assertStatus(t, w, http.StatusConflict)
	assertContains(t, w, "Deletion not possible")
	e.mustRepo(ids.Bob)
	// Stopping the tracking remains possible.
	w = e.post(confirm, url.Values{"keep_history": {"1"}}, false)
	assertRedirect(t, w, w.Code, "/settings?msg=stopped&repo="+strconv.FormatInt(ids.Bob, 10))

	close(gate.release)
	e.waitSyncIdle()
	w = e.post(confirm, nil, false)
	assertRedirect(t, w, w.Code, "/settings?msg=deleted")
}

func TestSettingsInterval(t *testing.T) {
	e := newTestEnv(t)
	cases := []struct {
		input string
		want  time.Duration
	}{
		{"30m", 30 * time.Minute},
		{"45", 45 * time.Minute},
		{"1h30m", 90 * time.Minute},
		{"0", 0},
	}
	for _, tc := range cases {
		w := e.post("/settings/interval", url.Values{"interval": {tc.input}}, false)
		assertRedirect(t, w, w.Code, "/settings?msg=interval")
		if got := e.coord.Interval(); got != tc.want {
			t.Errorf("%q: interval %s, want %s", tc.input, got, tc.want)
		}
	}

	assertStatus(t, e.post("/settings/interval", url.Values{"interval": {"2h"}}, false), http.StatusSeeOther)
	w := e.get("/settings?msg=interval")
	assertContains(t, w, "Current interval: <strong>2 h</strong>", "Next sync",
		"changed for this process", "config.yaml is not changed")

	for _, input := range []string{"5m", "-1h", "abc", ""} {
		w := e.post("/settings/interval", url.Values{"interval": {input}}, false)
		assertStatus(t, w, http.StatusBadRequest)
		assertContains(t, w, "alert-error")
		if got := e.coord.Interval(); got != 2*time.Hour {
			t.Errorf("%q: interval changed to %s", input, got)
		}
	}
	assertContains(t, e.post("/settings/interval", url.Values{"interval": {"5m"}}, false), "minimum de 15 min")
	// GET on the action: 405, nothing changes.
	assertStatus(t, e.get("/settings/interval?interval=0"), http.StatusMethodNotAllowed)
	if e.coord.Interval() != 2*time.Hour {
		t.Error("GET changed the interval")
	}
}

func TestParseIntervalInput(t *testing.T) {
	ok := map[string]time.Duration{"0": 0, "15": 15 * time.Minute, " 1h ": time.Hour, "90m": 90 * time.Minute}
	for in, want := range ok {
		if got, err := parseIntervalInput(en, in); err != nil || got != want {
			t.Errorf("parseIntervalInput(%q) = %s, %v; want %s", in, got, err, want)
		}
	}
	for _, in := range []string{"", "  ", "abc", "-5", "1j", "99999999"} {
		if _, err := parseIntervalInput(en, in); err == nil {
			t.Errorf("parseIntervalInput(%q) without error", in)
		}
	}
	if _, err := parseIntervalInput(en, "abc"); err == nil || !strings.Contains(err.Error(), `unreadable interval "abc"`) {
		t.Errorf("English error = %v", err)
	}
	if _, err := parseIntervalInput(i18n.For(i18n.French), "abc"); err == nil || !strings.Contains(err.Error(), `intervalle "abc" illisible`) {
		t.Errorf("French error = %v", err)
	}
}

func TestSettingsQuota(t *testing.T) {
	e := newTestEnv(t)
	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 4000, Reset: testNow.Add(30 * time.Minute)})
	w := e.htmx("/fragments/quota")
	assertStatus(t, w, http.StatusOK)
	if !strings.HasPrefix(strings.TrimSpace(w.Body.String()), `<div id="settings-quota"`) {
		t.Errorf("unexpected fragment:\n%s", w.Body.String())
	}
	assertContains(t, w, ">4000</span>", "/ 5000 requests left", "80%", ">10:30</time>",
		`hx-trigger="sync-finished from:body"`)
	assertNotContains(t, w, "quota low", "<html")

	e.fake.SetRateLimit(provider.RateLimit{Limit: 5000, Remaining: 100, Reset: testNow.Add(30 * time.Minute)})
	assertContains(t, e.htmx("/fragments/quota"), "quota low", "syncs deferred until then")

	// Unreadable token: quota unknown, with the message, without token.
	e.auth.SetTokenErr(errors.New("gh is not signed in"))
	w = e.htmx("/fragments/quota")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "quota unknown", "gh is not signed in")

	// gh not signed in: local Git clones, without quota (V2).
	e.auth.SetTokenErr(auth.ErrNotLoggedIn)
	w = e.htmx("/fragments/quota")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "not applicable", "local Git clones", "gh auth login")
	assertNotContains(t, w, "quota unknown")
}

func TestSettingsActionsRequireCSRF(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	e.fake.AddRepository(provider.Repository{ID: 50, Owner: "promo", Name: "eve", DefaultBranch: "main"})
	count := e.repoCount()
	id := strconv.FormatInt(ids.Bob, 10)
	targets := map[string]url.Values{
		"/settings/repos":                   {"repo": {"promo/eve"}},
		"/settings/repos/" + id:             {"label": {"pirate"}},
		"/settings/repos/" + id + "/remove": {},
		"/settings/interval":                {"interval": {"0"}},
	}
	for target, form := range targets {
		// Without token or cookie.
		r := e.request(http.MethodPost, target, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", testOrigin)
		assertStatus(t, e.do(r), http.StatusForbidden)

		// Valid token, but foreign origin.
		ck := e.csrfCookie()
		f := url.Values{CSRFField: {ck.Value}}
		for k, v := range form {
			f[k] = v
		}
		r = e.request(http.MethodPost, target, strings.NewReader(f.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "http://evil.example.test")
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		r.AddCookie(ck)
		assertStatus(t, e.do(r), http.StatusForbidden)
	}
	if got := e.repoCount(); got != count {
		t.Errorf("%d repositories, want %d", got, count)
	}
	if got := e.mustRepo(ids.Bob); got.Label != "" || !got.Enabled {
		t.Errorf("repository changed without CSRF: %+v", got)
	}
	if e.coord.Interval() != 0 {
		t.Error("interval changed without CSRF")
	}
}

// TestSettingsQuotaCanceled checks that an interrupted quota read (abandoned request, serve
// shutdown: canceled context) is only logged at debug level.
func TestSettingsQuotaCanceled(t *testing.T) {
	e := newTestEnv(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := e.request(http.MethodGet, "/fragments/quota", nil).WithContext(ctx)
	r.Header.Set("HX-Request", "true")
	e.do(r)
	logs := e.logs.String()
	if !strings.Contains(logs, `level=DEBUG msg="lecture du quota GitHub interrompue"`) ||
		strings.Contains(logs, "quota GitHub illisible") {
		t.Errorf("unexpected log:\n%s", logs)
	}

	// A real error remains a warning.
	e.auth.SetTokenErr(errors.New("gh is not signed in"))
	e.htmx("/fragments/quota")
	if !strings.Contains(e.logs.String(), `level=WARN msg="quota GitHub illisible"`) {
		t.Errorf("warning missing:\n%s", e.logs.String())
	}
}
