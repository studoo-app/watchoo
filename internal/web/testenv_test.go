package web

// Environnement de test commun à tous les tests du package (socle ; les agents des pages s'en
// servent sans le modifier, ou en ajoutant des options) : base SQLite temporaire remplie de données
// factices, vrai moteur de synchro sur un faux Provider (providertest) et un faux gh (authtest), vrai
// coordinateur, handler complet (middlewares compris), requêtes par httptest sans réseau. Chaque
// réponse et le journal sont vérifiés : ni le faux token, ni un e-mail des données factices.

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
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Valeurs des tests.
const (
	testHost      = "127.0.0.1:8080" // adresse écoutée et en-tête Host des requêtes
	testOrigin    = "http://" + testHost
	testWorkspace = "licence-projet"
	// xssMessage est le message d'un commit factice : il doit toujours apparaître échappé.
	xssMessage = "<script>alert('xss')</script> corrige le bug"
)

// testEmails sont les e-mails des données factices : jamais dans une réponse ni dans le journal.
var testEmails = []string{"alice@example.test", "bob@example.test", "carol@example.test", "etudiant@example.test"}

// testNow est l'horloge des tests : 27 octobre 2026, 10 h UTC.
var testNow = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

// safeBuffer est un tampon sûr pour un usage concurrent (journal).
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

// testEnv est un environnement de test de l'interface.
type testEnv struct {
	t       *testing.T
	store   *store.Store
	fake    *providertest.Fake // faux GitHub du moteur
	auth    *authtest.Fake
	engine  *wsync.Engine
	coord   *wsync.Coordinator
	server  *server
	handler http.Handler
	logs    *safeBuffer
	cfg     workspace.Config
	now     time.Time

	// Options appliquées avant la construction (voir envOption).
	syncer       wsync.Syncer // nil = le vrai moteur
	listenAddr   string
	webAuth      Auth
	trusted      bool
	allowedHosts []string
	interval     time.Duration
	// newProvider remplace le faux Provider du moteur (nil = e.fake) : vrai provider GitHub sur
	// githubtest pour le test de bout en bout.
	newProvider func(token string) (provider.Provider, error)
}

// envOption modifie l'environnement avant la construction du handler.
type envOption func(*testEnv)

// withSyncer remplace le moteur dans le coordinateur (synchro bloquante…).
func withSyncer(s wsync.Syncer) envOption { return func(e *testEnv) { e.syncer = s } }

// withListenAddr change l'adresse écoutée (contrôle Host, cookie CSRF).
func withListenAddr(addr string) envOption { return func(e *testEnv) { e.listenAddr = addr } }

// withAuth active l'authentification basique.
func withAuth(user, pass string) envOption {
	return func(e *testEnv) { e.webAuth = Auth{Username: user, Password: pass} }
}

// withTrustedProxy déclare un reverse proxy authentifiant.
func withTrustedProxy() envOption { return func(e *testEnv) { e.trusted = true } }

// withAllowedHosts règle web.allowed_hosts (noms publics acceptés dans l'en-tête Host).
func withAllowedHosts(hosts ...string) envOption {
	return func(e *testEnv) { e.allowedHosts = hosts }
}

// withInterval règle l'intervalle de synchro automatique du coordinateur (0 par défaut : manuelle).
func withInterval(d time.Duration) envOption { return func(e *testEnv) { e.interval = d } }

// withProvider branche le moteur sur un autre Provider que e.fake (vrai provider GitHub sur
// githubtest…).
func withProvider(f func(token string) (provider.Provider, error)) envOption {
	return func(e *testEnv) { e.newProvider = f }
}

// newTestEnv construit l'environnement, sans données (voir seed).
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
		t.Fatalf("store.Open : %v", err)
	}
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate : %v", err)
	}
	e.store = st
	newProvider := e.newProvider
	if newProvider == nil {
		newProvider = func(string) (provider.Provider, error) { return e.fake, nil }
	}
	e.engine, err = wsync.New(wsync.Options{
		Store:       st,
		Auth:        e.auth,
		Config:      e.cfg,
		NewProvider: newProvider,
		Now:         clock,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("wsync.New : %v", err)
	}
	syncer := e.syncer
	if syncer == nil {
		syncer = e.engine
	}
	e.coord, err = wsync.NewCoordinator(wsync.CoordinatorOptions{
		Syncer: syncer, Interval: e.interval, LastSync: st.LastSyncFinishedAt, Now: clock, Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewCoordinator : %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.coord.Start(ctx)
	t.Cleanup(func() {
		cancel()
		if err := e.coord.Close(5 * time.Second); err != nil {
			t.Errorf("Close du coordinateur : %v", err)
		}
		if err := st.Close(); err != nil {
			t.Errorf("Close de la base : %v", err)
		}
		assertNoLeak(t, "journal", e.logs.String())
	})

	e.server, err = newServer(Options{
		Store: st, Sync: e.coord, Repos: e.engine, Config: e.cfg, Location: time.UTC,
		Now: clock, Logger: logger, Version: "v0.0.0-test", ListenAddr: e.listenAddr,
		Auth: e.webAuth, TrustedProxy: e.trusted, AllowedHosts: e.allowedHosts,
	})
	if err != nil {
		t.Fatalf("newServer : %v", err)
	}
	e.handler = e.server.routes()
	return e
}

// Identifiants des dépôts créés par seed.
type seededRepos struct {
	Alice int64 // « Alice Martin » (promo/alice) : Actif, commit d'hier, message xssMessage
	Bob   int64 // promo/bob : Flemme (dernier commit il y a 9 jours)
	Carol int64 // promo/carol : Dead (dernier commit il y a 40 jours)
	Dave  int64 // promo/dave : jamais synchronisé
}

// seed remplit la base avec quatre dépôts et leurs commits (sans passer par le moteur).
func (e *testEnv) seed() seededRepos {
	e.t.Helper()
	alice := store.Identity{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh",
		ProviderUserID: 10, AvatarURL: "https://avatars.githubusercontent.com/u/10?v=4"}
	bob := store.Identity{Name: "Bob Durand", Email: "bob@example.test"}
	carol := store.Identity{Name: "Carol", Email: "carol@example.test", Login: "carol-gh", ProviderUserID: 30,
		AvatarURL: "https://evil.example.test/pixel.png"}
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

// addRepo crée le dépôt promo/name (identifiant de forge providerID) en base et dans le faux
// Provider.
func (e *testEnv) addRepo(providerID int64, name, label string) int64 {
	e.t.Helper()
	e.fake.AddRepository(provider.Repository{ID: providerID, Owner: "promo", Name: name, DefaultBranch: "main",
		URL: "https://github.com/promo/" + name, Private: true})
	r, err := e.store.CreateRepository(e.t.Context(), model.Repository{
		Provider: model.ProviderGitHub, ProviderID: providerID, Owner: "promo", Name: name, Label: label,
		URL: "https://github.com/promo/" + name, Enabled: true,
	})
	if err != nil {
		e.t.Fatalf("CreateRepository : %v", err)
	}
	return r.ID
}

// commitWrite fabrique un commit factice n, daté de testNow - ago.
func (e *testEnv) commitWrite(n int, who store.Identity, ago time.Duration, msg string, branches ...string) store.CommitWrite {
	at := e.now.Add(-ago)
	sha := providertest.SHA("commit-" + string(rune('a'+n)))
	return store.CommitWrite{SHA: sha, Message: msg, URL: "https://github.com/promo/x/commit/" + sha,
		Author: who, Committer: who, AuthoredAt: at, CommittedAt: at, Branches: branches}
}

// applySync enregistre une synchro réussie du dépôt repoID avec commits (branches main et dev).
func (e *testEnv) applySync(repoID int64, name string, commits ...store.CommitWrite) {
	e.t.Helper()
	ctx := e.t.Context()
	run, err := e.store.BeginSyncRun(ctx, repoID, e.now.Add(-time.Hour), wsync.LockStaleAfter)
	if err != nil {
		e.t.Fatalf("BeginSyncRun : %v", err)
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
		Branches: branches, Commits: commits, SyncedAt: e.now.Add(-time.Hour + time.Minute),
	}); err != nil {
		e.t.Fatalf("ApplyRepoSync : %v", err)
	}
}

// request construit une requête vers le handler, avec l'en-tête Host attendu.
func (e *testEnv) request(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequestWithContext(e.t.Context(), method, target, body)
	r.Host = testHost
	return r
}

// do envoie r au handler et renvoie la réponse, après avoir vérifié qu'elle ne contient ni le faux
// token ni un e-mail des données factices.
func (e *testEnv) do(r *http.Request) *httptest.ResponseRecorder {
	e.t.Helper()
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, r)
	assertNoLeak(e.t, r.Method+" "+r.URL.Path, w.Body.String())
	for name, values := range w.Header() {
		assertNoLeak(e.t, "en-tête "+name, strings.Join(values, " "))
	}
	return w
}

// get envoie GET target (page complète).
func (e *testEnv) get(target string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(e.request(http.MethodGet, target, nil))
}

// htmx envoie GET target avec l'en-tête HX-Request (fragment).
func (e *testEnv) htmx(target string) *httptest.ResponseRecorder {
	e.t.Helper()
	r := e.request(http.MethodGet, target, nil)
	r.Header.Set("HX-Request", "true")
	return e.do(r)
}

// csrfCookie fait un GET et renvoie le cookie CSRF posé par le serveur.
func (e *testEnv) csrfCookie() *http.Cookie {
	e.t.Helper()
	w := e.get("/fragments/sync")
	for _, c := range w.Result().Cookies() {
		if strings.HasPrefix(c.Name, "watchoo_csrf") {
			return c
		}
	}
	e.t.Fatalf("aucun cookie CSRF posé par GET /fragments/sync (en-têtes %v)", w.Header())
	return nil
}

// post envoie un formulaire POST valide (même origine, cookie et jeton CSRF), en HTMX si htmx.
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

// assertNoLeak vérifie que s ne contient ni le faux token, ni un e-mail des données factices.
func assertNoLeak(t *testing.T, what, s string) {
	t.Helper()
	if strings.Contains(s, authtest.FakeToken) {
		t.Errorf("%s : contient le token", what)
	}
	for _, email := range testEmails {
		if strings.Contains(strings.ToLower(s), email) {
			t.Errorf("%s : contient l'e-mail %s", what, email)
		}
	}
}

// assertStatus vérifie le code de réponse.
func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("code %d, attendu %d\n%s", w.Code, want, w.Body.String())
	}
}

// assertContains vérifie que le corps contient chacun des extraits.
func assertContains(t *testing.T, w *httptest.ResponseRecorder, parts ...string) {
	t.Helper()
	body := w.Body.String()
	for _, p := range parts {
		if !strings.Contains(body, p) {
			t.Errorf("corps sans %q :\n%s", p, body)
		}
	}
}

// assertNotContains vérifie que le corps ne contient aucun des extraits.
func assertNotContains(t *testing.T, w *httptest.ResponseRecorder, parts ...string) {
	t.Helper()
	body := w.Body.String()
	for _, p := range parts {
		if strings.Contains(body, p) {
			t.Errorf("corps avec %q :\n%s", p, body)
		}
	}
}

// waitSyncIdle attend la fin de la synchro en cours du coordinateur.
func (e *testEnv) waitSyncIdle() wsync.SyncState {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := e.coord.State(); !st.Running {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatal("la synchro ne s'est pas terminée")
	return wsync.SyncState{}
}
