package sync_test

// Outils communs aux tests du moteur : base SQLite temporaire, faux Provider (providertest), faux gh
// (authtest), horloge, logs capturés et contrôle qu'aucun token ni donnée personnelle n'y apparaît.

import (
	"bytes"
	"context"
	"errors"
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

// Données factices.
const (
	testOwner = "promo-demo" // organisation des dépôts (peut apparaître dans les logs)

	secretName    = "Eleve Confidentiel"
	secretEmail   = "eleve.confidentiel@example.test"
	secretLogin   = "eleve-confidentiel"
	secretMessage = "Message de commit confidentiel"
)

// forbidden liste les chaînes qui ne doivent jamais apparaître dans les logs ni dans les rapports.
var forbidden = []string{
	authtest.FakeToken, "gho_", "Bearer",
	secretName, secretEmail, secretLogin, secretMessage,
	// Identité factice de providertest.AppendCommits et githubtest.AppendCommits.
	"Étudiant Test", "etudiant@example.test", "etudiant-test",
}

// appendMessage reconnaît les messages « commit N » d'AppendCommits.
var appendMessage = regexp.MustCompile(`\bcommit [0-9]{1,5}\b`)

// epoch est la date des premiers commits factices.
var epoch = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// safeBuffer est un bytes.Buffer protégé par un verrou (logs écrits par plusieurs goroutines).
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

// clock est une horloge réglable, sûre pour un usage concurrent.
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

// env est un environnement de test du moteur.
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
	tokens []string // tokens reçus par la fabrique de Provider
	wrap   func(provider.Provider) provider.Provider

	// gitURL et reposDir : provider des clones Git locaux (withGit), utilisé quand gh est absent.
	gitURL   string
	reposDir string
	gitCalls int // Provider Git construits
}

// withGit branche le provider des clones Git locaux sur le faux serveur Git srvURL (gittest).
func withGit(srvURL string) envOption {
	return func(e *env) { e.gitURL = srvURL }
}

// envOption modifie l'environnement avant la construction du moteur.
type envOption func(*env)

// withParallelism règle sync.parallelism.
func withParallelism(n int) envOption {
	return func(e *env) { e.cfg.Sync.Parallelism = n }
}

// withExclude règle branches.exclude.
func withExclude(patterns ...string) envOption {
	return func(e *env) { e.cfg.Branches.Exclude = patterns }
}

// withWrap enveloppe le Provider construit à chaque synchro (blocage, comptage…).
func withWrap(w func(provider.Provider) provider.Provider) envOption {
	return func(e *env) { e.wrap = w }
}

// newEnv construit un environnement : base temporaire migrée, faux Provider, gh connecté, horloge
// au 20 octobre 2026, logs capturés au niveau Debug (vérifiés à la fin du test).
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

// openStore ouvre la base path (et la migre si migrate), fermée à la fin du test.
func openStore(t *testing.T, path string, migrate bool) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("store.Open : %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close : %v", err)
		}
	})
	if migrate {
		if err := st.Migrate(t.Context()); err != nil {
			t.Fatalf("Migrate : %v", err)
		}
	}
	return st
}

// newEngine construit un moteur sur st, avec le faux Provider de l'environnement.
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
		NewGitProvider: func() (provider.Provider, error) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.gitCalls++
			if e.gitURL == "" {
				return nil, errors.New("pas de dépôts Git locaux dans ce test")
			}
			return gitprov.New(gitprov.Options{Dir: e.reposDir, BaseURL: e.gitURL, Logger: logger})
		},
		Now:    e.clock.Now,
		Logger: logger,
	})
	if err != nil {
		e.t.Fatalf("wsync.New : %v", err)
	}
	return eng
}

// receivedTokens renvoie les tokens reçus par la fabrique de Provider.
func (e *env) receivedTokens() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.tokens)
}

// addRepo ajoute le dépôt id (owner testOwner) au faux Provider et en base, suivi, avec les motifs
// de branches patterns, et renvoie le dépôt enregistré.
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
		e.t.Fatalf("CreateRepository : %v", err)
	}
	return repo
}

// commit ajoute au dépôt repoID un commit de l'auteur secret, de parents parents, daté at, et
// renvoie son SHA (dérivé de seed).
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

// syncAll lance une synchro de tous les dépôts, qui doit réussir globalement.
func (e *env) syncAll() wsync.Report {
	e.t.Helper()
	rep, err := e.eng.SyncAll(e.t.Context(), wsync.SyncOptions{})
	if err != nil {
		e.t.Fatalf("SyncAll : %v", err)
	}
	assertReportNoLeak(e.t, rep)
	return rep
}

// syncOK synchronise le seul dépôt repoID et vérifie le succès ; renvoie son rapport.
func (e *env) syncOK(repoID int64) wsync.RepoReport {
	e.t.Helper()
	rep, err := e.eng.SyncRepo(e.t.Context(), repoID, wsync.SyncOptions{})
	if err != nil {
		e.t.Fatalf("SyncRepo : %v", err)
	}
	assertReportNoLeak(e.t, rep)
	if len(rep.Repos) != 1 {
		e.t.Fatalf("SyncRepo : %d rapports, 1 attendu", len(rep.Repos))
	}
	r := rep.Repos[0]
	if r.Status != wsync.StatusSuccess {
		e.t.Fatalf("SyncRepo : statut %q (%s), success attendu", r.Status, r.Error)
	}
	return r
}

// commitSHAs renvoie les SHA en base du dépôt repoID, filtrés par branche si branch n'est pas vide,
// triés.
func (e *env) commitSHAs(repoID int64, branch string) []string {
	e.t.Helper()
	views, err := e.st.ListRecentCommits(e.t.Context(), store.CommitFilter{RepoID: repoID, Branch: branch})
	if err != nil {
		e.t.Fatalf("ListRecentCommits : %v", err)
	}
	shas := make([]string, 0, len(views))
	for _, v := range views {
		shas = append(shas, v.SHA)
	}
	slices.Sort(shas)
	return shas
}

// commitView renvoie le commit sha du dépôt repoID.
func (e *env) commitView(repoID int64, sha string) store.CommitView {
	e.t.Helper()
	views, err := e.st.ListRecentCommits(e.t.Context(), store.CommitFilter{RepoID: repoID})
	if err != nil {
		e.t.Fatalf("ListRecentCommits : %v", err)
	}
	for _, v := range views {
		if v.SHA == sha {
			return v
		}
	}
	e.t.Fatalf("commit %s absent de la base", provider.ShortSHA(sha))
	return store.CommitView{}
}

// branches renvoie les branches en base du dépôt repoID, par nom.
func (e *env) branches(repoID int64) map[string]model.Branch {
	e.t.Helper()
	list, err := e.st.ListBranches(e.t.Context(), repoID)
	if err != nil {
		e.t.Fatalf("ListBranches : %v", err)
	}
	out := make(map[string]model.Branch, len(list))
	for _, b := range list {
		out[b.Name] = b
	}
	return out
}

// lastRun renvoie le dernier sync_run du dépôt repoID.
func (e *env) lastRun(repoID int64) model.SyncRun {
	e.t.Helper()
	run, err := e.st.LastSyncRun(e.t.Context(), repoID)
	if err != nil {
		e.t.Fatalf("LastSyncRun : %v", err)
	}
	return run
}

// sorted renvoie une copie triée de shas.
func sorted(shas ...string) []string {
	out := slices.Clone(shas)
	slices.Sort(out)
	return out
}

// assertNoLeak vérifie que s ne contient ni token ni donnée personnelle factice.
func assertNoLeak(t *testing.T, what, s string) {
	t.Helper()
	for _, f := range forbidden {
		if strings.Contains(s, f) {
			t.Errorf("%s : contient %q :\n%s", what, f, s)
		}
	}
	if m := appendMessage.FindString(s); m != "" {
		t.Errorf("%s : contient un message de commit %q :\n%s", what, m, s)
	}
}

// assertReportNoLeak vérifie les messages d'un rapport.
func assertReportNoLeak(t *testing.T, rep wsync.Report) {
	t.Helper()
	for _, r := range rep.Repos {
		assertNoLeak(t, "rapport "+r.FullName, r.Error+" "+r.FullName+" "+r.DisplayName+" "+strings.Join(r.Warnings, " "))
	}
}

// ----- Provider enveloppé -----

// hookProvider enveloppe un Provider et appelle before avant chaque méthode interrogeant la forge.
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
