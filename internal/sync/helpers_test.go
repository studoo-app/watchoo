package sync_test

// Tools shared by the engine tests: temporary SQLite database, fake Provider (providertest), fake gh
// (authtest), clock, captured logs and check that no token or personal data appears in them.

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
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

// Dummy data.
const (
	testOwner = "promo-demo" // organization of the repositories (may appear in the logs)

	secretName    = "Eleve Confidentiel"
	secretEmail   = "eleve.confidentiel@example.test"
	secretLogin   = "eleve-confidentiel"
	secretMessage = "Message de commit confidentiel"
)

// forbidden lists the strings that must never appear in the logs or in the reports.
var forbidden = []string{
	authtest.FakeToken, "gho_", "Bearer",
	secretName, secretEmail, secretLogin, secretMessage,
	// Dummy identity of providertest.AppendCommits and githubtest.AppendCommits.
	"Étudiant Test", "etudiant@example.test", "etudiant-test",
}

// appendMessage recognizes the "commit N" messages of AppendCommits.
var appendMessage = regexp.MustCompile(`\bcommit [0-9]{1,5}\b`)

// epoch is the date of the first dummy commits.
var epoch = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// safeBuffer is a bytes.Buffer protected by a lock (logs written by several goroutines).
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

// clock is an adjustable clock, safe for concurrent use.
type clock struct {
	mu gosync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

// env is a test environment of the engine.
type env struct {
	t      *testing.T
	dbPath string
	st     *store.Store
	fake   *providertest.Fake
	auth   *authtest.Fake
	clock  *clock
	logs   *safeBuffer
	cfg    workspace.Config
	eng    *wsync.Engine

	mu     gosync.Mutex
	tokens []string // tokens received by the Provider factory
	wrap   func(provider.Provider) provider.Provider

	// gitURL and reposDir: provider of the local Git clones (withGit), used when gh is missing.
	gitURL   string
	reposDir string
	gitCalls int // Git Providers built
	// gitWrap, if not nil, wraps the built Git provider (cancellation, counting…).
	gitWrap func(*gitprov.Provider) provider.Provider
	// mirrored: commits already copied from the Git repository to the fake Provider (mirror), per repository.
	mirrored map[int64]map[string]bool
}

// withGit plugs the provider of the local Git clones into the fake Git server srvURL (gittest).
func withGit(srvURL string) envOption {
	return func(e *env) { e.gitURL = srvURL }
}

// envOption changes the environment before the engine is built.
type envOption func(*env)

// withParallelism sets sync.parallelism.
func withParallelism(n int) envOption {
	return func(e *env) { e.cfg.Sync.Parallelism = n }
}

// withExclude sets branches.exclude.
func withExclude(patterns ...string) envOption {
	return func(e *env) { e.cfg.Branches.Exclude = patterns }
}

// withWrap wraps the Provider built at every sync (blocking, counting…).
func withWrap(w func(provider.Provider) provider.Provider) envOption {
	return func(e *env) { e.wrap = w }
}

// newEnv builds an environment: temporary migrated database, fake Provider, gh signed in, clock
// on October 20, 2026, logs captured at the Debug level (checked at the end of the test).
func newEnv(t *testing.T, opts ...envOption) *env {
	t.Helper()
	e := &env{
		t:      t,
		dbPath: filepath.Join(t.TempDir(), "watchoo.db"),
		fake:   providertest.New(),
		auth:   authtest.NewLoggedIn(""),
		clock:  &clock{t: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)},
		logs:   &safeBuffer{},
		cfg:    workspace.DefaultConfig("promo"),
	}
	e.reposDir = filepath.Join(t.TempDir(), "repos")
	e.fake.SetClock(e.clock.Now)
	for _, o := range opts {
		o(e)
	}
	e.st = openStore(t, e.dbPath, true)
	e.eng = e.newEngine(e.st)
	t.Cleanup(func() { assertNoLeak(t, "logs", e.logs.String()) })
	return e
}

// openStore opens the database path (and migrates it if migrate), closed at the end of the test.
func openStore(t *testing.T, path string, migrate bool) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if migrate {
		if err := st.Migrate(t.Context()); err != nil {
			t.Fatalf("Migrate: %v", err)
		}
	}
	return st
}

// newEngine builds an engine on st, with the fake Provider of the environment.
func (e *env) newEngine(st *store.Store) *wsync.Engine {
	e.t.Helper()
	logger := slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	eng, err := wsync.New(wsync.Options{
		Store:  st,
		Auth:   e.auth,
		Config: e.cfg,
		NewProvider: func(token string) (provider.Provider, error) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.tokens = append(e.tokens, token)
			var p provider.Provider = e.fake
			if e.wrap != nil {
				p = e.wrap(p)
			}
			return p, nil
		},
		NewGitProvider: func(repos []gitprov.Ref) (provider.Provider, error) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.gitCalls++
			if e.gitURL == "" {
				return nil, nil // clones disabled: without gh, "directory of the clones not set"
			}
			gp, err := gitprov.New(gitprov.Options{Dir: e.reposDir, Repos: repos, BaseURL: e.gitURL, Logger: logger})
			if err != nil || e.gitWrap == nil {
				return gp, err
			}
			return e.gitWrap(gp), nil
		},
		Now:    e.clock.Now,
		Logger: logger,
	})
	if err != nil {
		e.t.Fatalf("wsync.New: %v", err)
	}
	return eng
}

// receivedTokens returns the tokens received by the Provider factory.
func (e *env) receivedTokens() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.tokens)
}

// addRepo adds the repository id (owner testOwner) to the fake Provider and to the database, tracked,
// with the branch patterns patterns, and returns the recorded repository.
func (e *env) addRepo(id int64, name string, patterns ...string) model.Repository {
	e.t.Helper()
	e.fake.AddRepository(provider.Repository{
		ID: id, Owner: testOwner, Name: name, DefaultBranch: "main",
		URL: "https://github.com/" + testOwner + "/" + name, Private: true,
	})
	repo, err := e.st.CreateRepository(e.t.Context(), model.Repository{
		Provider: model.ProviderGitHub, ProviderID: id, Owner: testOwner, Name: name,
		Enabled: true, BranchPatterns: patterns,
	})
	if err != nil {
		e.t.Fatalf("CreateRepository: %v", err)
	}
	return repo
}

// commit adds to the repository repoID a commit of the secret author, with parents parents, dated
// at, and returns its SHA (derived from seed).
func (e *env) commit(repoID int64, seed string, at time.Time, parents ...string) string {
	e.t.Helper()
	sha := providertest.SHA(seed)
	person := provider.Person{Name: secretName, Email: secretEmail, Login: secretLogin, ID: 4242,
		AvatarURL: "https://avatars.example.test/u/4242"}
	e.fake.AddCommits(repoID, provider.Commit{
		SHA: sha, Message: secretMessage, URL: "https://github.com/" + testOwner + "/x/commit/" + sha,
		Author: person, Committer: person, AuthoredAt: at, CommittedAt: at, Parents: parents,
	})
	return sha
}

// syncAll runs a sync of all the repositories, which must succeed globally.
func (e *env) syncAll() wsync.Report {
	e.t.Helper()
	rep, err := e.eng.SyncAll(e.t.Context(), wsync.SyncOptions{})
	if err != nil {
		e.t.Fatalf("SyncAll: %v", err)
	}
	assertReportNoLeak(e.t, rep)
	return rep
}

// syncOK syncs the single repository repoID and checks the success; returns its report.
func (e *env) syncOK(repoID int64) wsync.RepoReport {
	e.t.Helper()
	rep, err := e.eng.SyncRepo(e.t.Context(), repoID, wsync.SyncOptions{})
	if err != nil {
		e.t.Fatalf("SyncRepo: %v", err)
	}
	assertReportNoLeak(e.t, rep)
	if len(rep.Repos) != 1 {
		e.t.Fatalf("SyncRepo: %d reports, want 1", len(rep.Repos))
	}
	r := rep.Repos[0]
	if r.Status != wsync.StatusSuccess {
		e.t.Fatalf("SyncRepo: status %q (%s), want success", r.Status, r.Error)
	}
	return r
}

// commitSHAs returns the SHAs in the database of the repository repoID, filtered by branch if branch
// is not empty, sorted.
func (e *env) commitSHAs(repoID int64, branch string) []string {
	e.t.Helper()
	views, err := e.st.ListRecentCommits(e.t.Context(), store.CommitFilter{RepoID: repoID, Branch: branch})
	if err != nil {
		e.t.Fatalf("ListRecentCommits: %v", err)
	}
	shas := make([]string, 0, len(views))
	for _, v := range views {
		shas = append(shas, v.SHA)
	}
	slices.Sort(shas)
	return shas
}

// commitView returns the commit sha of the repository repoID.
func (e *env) commitView(repoID int64, sha string) store.CommitView {
	e.t.Helper()
	views, err := e.st.ListRecentCommits(e.t.Context(), store.CommitFilter{RepoID: repoID})
	if err != nil {
		e.t.Fatalf("ListRecentCommits: %v", err)
	}
	for _, v := range views {
		if v.SHA == sha {
			return v
		}
	}
	e.t.Fatalf("commit %s missing from the database", provider.ShortSHA(sha))
	return store.CommitView{}
}

// branches returns the branches in the database of the repository repoID, by name.
func (e *env) branches(repoID int64) map[string]model.Branch {
	e.t.Helper()
	list, err := e.st.ListBranches(e.t.Context(), repoID)
	if err != nil {
		e.t.Fatalf("ListBranches: %v", err)
	}
	out := make(map[string]model.Branch, len(list))
	for _, b := range list {
		out[b.Name] = b
	}
	return out
}

// lastRun returns the last sync_run of the repository repoID.
func (e *env) lastRun(repoID int64) model.SyncRun {
	e.t.Helper()
	run, err := e.st.LastSyncRun(e.t.Context(), repoID)
	if err != nil {
		e.t.Fatalf("LastSyncRun: %v", err)
	}
	return run
}

// sorted returns a sorted copy of shas.
func sorted(shas ...string) []string {
	out := slices.Clone(shas)
	slices.Sort(out)
	return out
}

// assertNoLeak checks that s contains neither token nor dummy personal data.
func assertNoLeak(t *testing.T, what, s string) {
	t.Helper()
	for _, f := range forbidden {
		if strings.Contains(s, f) {
			t.Errorf("%s: contains %q:\n%s", what, f, s)
		}
	}
	if m := appendMessage.FindString(s); m != "" {
		t.Errorf("%s: contains a commit message %q:\n%s", what, m, s)
	}
}

// assertReportNoLeak checks the messages of a report.
func assertReportNoLeak(t *testing.T, rep wsync.Report) {
	t.Helper()
	for _, r := range rep.Repos {
		assertNoLeak(t, "report "+r.FullName, r.Error+" "+r.FullName+" "+r.DisplayName+" "+strings.Join(r.WarningMessages(), " "))
	}
}

// ----- Wrapped Provider -----

// hookProvider wraps a Provider and calls before before every method querying the forge.
type hookProvider struct {
	provider.Provider
	before func(ctx context.Context, method string) error
}

func (h *hookProvider) hook(ctx context.Context, m string) error {
	if h.before == nil {
		return nil
	}
	return h.before(ctx, m)
}

func (h *hookProvider) RepositoryByID(ctx context.Context, id int64) (provider.Repository, error) {
	if err := h.hook(ctx, "RepositoryByID"); err != nil {
		return provider.Repository{}, err
	}
	return h.Provider.RepositoryByID(ctx, id)
}

func (h *hookProvider) Branches(ctx context.Context, owner, name string) ([]provider.Branch, error) {
	if err := h.hook(ctx, "Branches"); err != nil {
		return nil, err
	}
	return h.Provider.Branches(ctx, owner, name)
}
