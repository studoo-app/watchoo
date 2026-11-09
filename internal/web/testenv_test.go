package web

// Test environment shared by all the tests of the package (foundation; the page agents use it
// without changing it, or by adding options): temporary SQLite database filled with dummy data, real
// sync engine on a fake Provider (providertest) and a fake gh (authtest), real coordinator, full
// handler (middlewares included), requests through httptest without network. Every response and the
// log are checked: neither the fake token, nor an e-mail of the dummy data.
//
// Language: the pages are rendered in the lang setting of the test configuration, "en" by default
// (withLang changes it); the process language is English (TestMain).

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Test values.
const (
	testHost      = "127.0.0.1:8080" // listened address and Host header of the requests
	testOrigin    = "http://" + testHost
	testWorkspace = "licence-projet"
	// xssMessage is the message of a dummy commit: it must always appear escaped.
	xssMessage = "<script>alert('xss')</script> corrige le bug"
)

// testEmails are the e-mails of the dummy data: never in a response or in the log.
var testEmails = []string{"alice@example.test", "bob@example.test", "carol@example.test", "etudiant@example.test",
	"eve@example.test", "zoe@example.test", "fred@example.test", "prolifique@example.test"}

// testNow is the clock of the tests: October 27, 2026, 10:00 UTC.
var testNow = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

// safeBuffer is a buffer safe for concurrent use (log).
type safeBuffer struct {
	mu  gosync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// testEnv is a test environment of the interface.
type testEnv struct {
	t       *testing.T
	store   *store.Store
	fake    *providertest.Fake // fake GitHub of the engine
	auth    *authtest.Fake
	engine  *wsync.Engine
	coord   *wsync.Coordinator
	server  *server
	handler http.Handler
	logs    *safeBuffer
	cfg     workspace.Config
	now     time.Time

	// Options applied before the construction (see envOption).
	syncer       wsync.Syncer // nil = the real engine
	listenAddr   string
	webAuth      Auth
	trusted      bool
	allowedHosts []string
	interval     time.Duration
	// newProvider replaces the fake Provider of the engine (nil = e.fake): real GitHub provider on
	// githubtest for the end-to-end test.
	newProvider func(token string) (provider.Provider, error)
	// updates and updateDocker: new version banner (0.6.0).
	updates      UpdateSource
	updateDocker bool
	// gitURL is the base URL of the local Git clones (gittest server); empty: an empty "server".
	gitURL string
}

// envOption changes the environment before the construction of the handler.
type envOption func(*testEnv)

// withSyncer replaces the engine in the coordinator (blocking sync…).
func withSyncer(s wsync.Syncer) envOption { return func(e *testEnv) { e.syncer = s } }

// withListenAddr changes the listened address (Host check, CSRF cookie).
func withListenAddr(addr string) envOption { return func(e *testEnv) { e.listenAddr = addr } }

// withAuth enables basic authentication.
func withAuth(user, pass string) envOption {
	return func(e *testEnv) { e.webAuth = Auth{Username: user, Password: pass} }
}

// withTrustedProxy declares an authenticating reverse proxy.
func withTrustedProxy() envOption { return func(e *testEnv) { e.trusted = true } }

// withAllowedHosts sets web.allowed_hosts (public names accepted in the Host header).
func withAllowedHosts(hosts ...string) envOption {
	return func(e *testEnv) { e.allowedHosts = hosts }
}

// withInterval sets the automatic sync interval of the coordinator (0 by default: manual).
func withInterval(d time.Duration) envOption { return func(e *testEnv) { e.interval = d } }

// withLang sets the lang setting of the test configuration (language of the pages).
func withLang(lang string) envOption { return func(e *testEnv) { e.cfg.Lang = lang } }

// withProvider plugs the engine on another Provider than e.fake (real GitHub provider on
// githubtest…).
func withProvider(f func(token string) (provider.Provider, error)) envOption {
	return func(e *testEnv) { e.newProvider = f }
}

// withGitURL plugs the local Git clones on a gittest server (synced without gh).
func withGitURL(url string) envOption { return func(e *testEnv) { e.gitURL = url } }

// withUpdates plugs the source of the new version banner (docker: Docker image text).
func withUpdates(src UpdateSource, docker bool) envOption {
	return func(e *testEnv) { e.updates, e.updateDocker = src, docker }
}

// newTestEnv builds the environment, without data (see seed).
func newTestEnv(t *testing.T, opts ...envOption) *testEnv {
	t.Helper()
	e := &testEnv{
		t:          t,
		fake:       providertest.New(),
		auth:       authtest.NewLoggedIn(""),
		logs:       &safeBuffer{},
		cfg:        workspace.DefaultConfig(testWorkspace),
		now:        testNow,
		listenAddr: testHost,
	}
	e.cfg.Timezone = "UTC"
	for _, o := range opts {
		o(e)
	}
	clock := func() time.Time { return e.now }
	e.fake.SetClock(clock)
	logger := slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "watchoo.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	e.store = st
	reposDir, gitURL := filepath.Join(t.TempDir(), "repos"), e.gitURL
	if gitURL == "" {
		gitURL = "file://" + filepath.ToSlash(t.TempDir())
	}
	newProvider := e.newProvider
	if newProvider == nil {
		newProvider = func(string) (provider.Provider, error) { return e.fake, nil }
	}
	e.engine, err = wsync.New(wsync.Options{
		Store:       st,
		Auth:        e.auth,
		Config:      e.cfg,
		NewProvider: newProvider,
		// Without gh: local Git clones to an empty "server" (no network, access refused), or to the gittest
		// server of withGitURL; the same clone directory for the sync and the reading of the clones.
		NewGitProvider: func(repos []gitprov.Ref) (provider.Provider, error) {
			return gitprov.New(gitprov.Options{Dir: reposDir, Repos: repos, BaseURL: gitURL, Logger: logger})
		},
		Now:    clock,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("wsync.New: %v", err)
	}
	syncer := e.syncer
	if syncer == nil {
		syncer = e.engine
	}
	e.coord, err = wsync.NewCoordinator(wsync.CoordinatorOptions{
		Syncer: syncer, Interval: e.interval, LastSync: st.LastSyncFinishedAt, Now: clock, Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.coord.Start(ctx)
	t.Cleanup(func() {
		cancel()
		if err := e.coord.Close(5 * time.Second); err != nil {
			t.Errorf("Close of the coordinator: %v", err)
		}
		if err := st.Close(); err != nil {
			t.Errorf("Close of the database: %v", err)
		}
		assertNoLeak(t, "log", e.logs.String())
	})

	e.server, err = newServer(Options{
		Store: st, Sync: e.coord, Repos: e.engine, Config: e.cfg, Location: time.UTC,
		Now: clock, Logger: logger, Version: "v0.0.0-test", ListenAddr: e.listenAddr,
		Auth: e.webAuth, TrustedProxy: e.trusted, AllowedHosts: e.allowedHosts,
		Updates: e.updates, UpdateDocker: e.updateDocker,
	})
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	e.handler = e.server.routes()
	return e
}

// Identifiers of the repositories created by seed.
type seededRepos struct {
	Alice int64 // "Alice Martin" (promo/alice): Active, commit of yesterday, message xssMessage
	Bob   int64 // promo/bob: Flemme (last commit 9 days ago)
	Carol int64 // promo/carol: Dead (last commit 40 days ago)
	Dave  int64 // promo/dave: never synced
}

// seed fills the database with four repositories and their commits (without the engine).
// Contributors: Alice (forge account 10, profile https://github.com/alice-gh), Bob (Git identity
// only, without account), Carol (account 30, avatar on another host and refused profile address,
// "javascript:").
func (e *testEnv) seed() seededRepos {
	e.t.Helper()
	alice := store.Identity{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh",
		ProviderUserID: 10, AvatarURL: "https://avatars.githubusercontent.com/u/10?v=4",
		ProfileURL: "https://github.com/alice-gh"}
	bob := store.Identity{Name: "Bob Durand", Email: "bob@example.test"}
	carol := store.Identity{Name: "Carol", Email: "carol@example.test", Login: "carol-gh", ProviderUserID: 30,
		AvatarURL: "https://evil.example.test/pixel.png", ProfileURL: "javascript:alert(1)"}
	ids := seededRepos{
		Alice: e.addRepo(1, "alice", "Alice Martin"),
		Bob:   e.addRepo(2, "bob", ""),
		Carol: e.addRepo(3, "carol", ""),
		Dave:  e.addRepo(4, "dave", ""),
	}
	e.applySync(ids.Alice, "alice",
		e.commitWrite(1, alice, 30*24*time.Hour, "premier commit", "main"),
		e.commitWrite(2, bob, 3*24*time.Hour, "ajoute le README", "main", "dev"),
		e.commitWrite(3, alice, 24*time.Hour, xssMessage, "main"),
	)
	e.applySync(ids.Bob, "bob", e.commitWrite(4, bob, 9*24*time.Hour, "travail de Bob", "main"))
	e.applySync(ids.Carol, "carol", e.commitWrite(5, carol, 40*24*time.Hour, "dernier signe de vie", "main"))
	return ids
}

// addRepo creates the repository promo/name (forge identifier providerID) in the database and in the
// fake Provider.
func (e *testEnv) addRepo(providerID int64, name, label string) int64 {
	e.t.Helper()
	e.fake.AddRepository(provider.Repository{ID: providerID, Owner: "promo", Name: name, DefaultBranch: "main",
		URL: "https://github.com/promo/" + name, Private: true})
	r, err := e.store.CreateRepository(e.t.Context(), model.Repository{
		Provider: model.ProviderGitHub, ProviderID: providerID, Owner: "promo", Name: name, Label: label,
		URL: "https://github.com/promo/" + name, Enabled: true,
	})
	if err != nil {
		e.t.Fatalf("CreateRepository: %v", err)
	}
	return r.ID
}

// commitWrite builds a dummy commit n, dated testNow - ago.
func (e *testEnv) commitWrite(n int, who store.Identity, ago time.Duration, msg string, branches ...string) store.CommitWrite {
	at := e.now.Add(-ago)
	sha := providertest.SHA("commit-" + string(rune('a'+n)))
	return store.CommitWrite{SHA: sha, Message: msg, URL: "https://github.com/promo/x/commit/" + sha,
		Author: who, Committer: who, AuthoredAt: at, CommittedAt: at, Branches: branches}
}

// applySync saves a successful sync of repository repoID with commits (main and dev branches).
func (e *testEnv) applySync(repoID int64, name string, commits ...store.CommitWrite) {
	e.t.Helper()
	ctx := e.t.Context()
	run, err := e.store.BeginSyncRun(ctx, repoID, e.now.Add(-time.Hour), wsync.LockStaleAfter)
	if err != nil {
		e.t.Fatalf("BeginSyncRun: %v", err)
	}
	branches := []store.BranchState{{Name: "main", HeadSHA: commits[len(commits)-1].SHA}}
	for _, c := range commits {
		for _, b := range c.Branches {
			if b == "dev" {
				branches = append(branches, store.BranchState{Name: "dev", HeadSHA: c.SHA})
			}
		}
	}
	if _, err := e.store.ApplyRepoSync(ctx, store.RepoSyncWrite{
		RepoID: repoID, RunID: run.ID,
		Metadata: store.RepositoryMetadata{Owner: "promo", Name: name, DefaultBranch: "main",
			URL: "https://github.com/promo/" + name},
		Branches: branches, Commits: commits, LanguagesVersion: 1, SyncedAt: e.now.Add(-time.Hour + time.Minute),
	}); err != nil {
		e.t.Fatalf("ApplyRepoSync: %v", err)
	}
}

// request builds a request to the handler, with the expected Host header.
func (e *testEnv) request(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequestWithContext(e.t.Context(), method, target, body)
	r.Host = testHost
	return r
}

// do sends r to the handler and returns the response, after checking that it contains neither the
// fake token nor an e-mail of the dummy data.
func (e *testEnv) do(r *http.Request) *httptest.ResponseRecorder {
	e.t.Helper()
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, r)
	assertNoLeak(e.t, r.Method+" "+r.URL.Path, w.Body.String())
	for name, values := range w.Header() {
		assertNoLeak(e.t, "header "+name, strings.Join(values, " "))
	}
	return w
}

// get sends GET target (full page).
func (e *testEnv) get(target string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(e.request(http.MethodGet, target, nil))
}

// htmx sends GET target with the HX-Request header (fragment).
func (e *testEnv) htmx(target string) *httptest.ResponseRecorder {
	e.t.Helper()
	r := e.request(http.MethodGet, target, nil)
	r.Header.Set("HX-Request", "true")
	return e.do(r)
}

// csrfCookie makes a GET and returns the CSRF cookie set by the server.
func (e *testEnv) csrfCookie() *http.Cookie {
	e.t.Helper()
	w := e.get("/fragments/sync")
	for _, c := range w.Result().Cookies() {
		if strings.HasPrefix(c.Name, "watchoo_csrf") {
			return c
		}
	}
	e.t.Fatalf("no CSRF cookie set by GET /fragments/sync (headers %v)", w.Header())
	return nil
}

// post sends a valid POST form (same origin, cookie and CSRF token), as HTMX if htmx.
func (e *testEnv) post(target string, form url.Values, htmx bool) *httptest.ResponseRecorder {
	e.t.Helper()
	ck := e.csrfCookie()
	if form == nil {
		form = url.Values{}
	}
	form.Set(CSRFField, ck.Value)
	r := e.request(http.MethodPost, target, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.AddCookie(ck)
	if htmx {
		r.Header.Set("HX-Request", "true")
	}
	return e.do(r)
}

// assertNoLeak checks that s contains neither the fake token, nor an e-mail of the dummy data.
func assertNoLeak(t *testing.T, what, s string) {
	t.Helper()
	if strings.Contains(s, authtest.FakeToken) {
		t.Errorf("%s: contains the token", what)
	}
	for _, email := range testEmails {
		if strings.Contains(strings.ToLower(s), email) {
			t.Errorf("%s: contains the e-mail %s", what, email)
		}
	}
}

// assertStatus checks the response code.
func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("code %d, want %d\n%s", w.Code, want, w.Body.String())
	}
}

// assertContains checks that the body contains each of the parts.
func assertContains(t *testing.T, w *httptest.ResponseRecorder, parts ...string) {
	t.Helper()
	body := w.Body.String()
	for _, p := range parts {
		if !strings.Contains(body, p) {
			t.Errorf("body without %q:\n%s", p, body)
		}
	}
}

// assertNotContains checks that the body contains none of the parts.
func assertNotContains(t *testing.T, w *httptest.ResponseRecorder, parts ...string) {
	t.Helper()
	body := w.Body.String()
	for _, p := range parts {
		if strings.Contains(body, p) {
			t.Errorf("body with %q:\n%s", p, body)
		}
	}
}

// waitSyncIdle waits for the end of the running sync of the coordinator.
func (e *testEnv) waitSyncIdle() wsync.SyncState {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := e.coord.State(); !st.Running {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatal("the sync did not finish")
	return wsync.SyncState{}
}
