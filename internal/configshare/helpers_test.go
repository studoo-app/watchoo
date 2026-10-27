package configshare_test

// Outils communs aux tests : espace temporaire (workspace.Init), base migrée, faux Provider, faux gh.
// Aucun réseau.

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

// testNow est la date des exports.
var testNow = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

// env est un espace de test complet.
type env struct {
	t    *testing.T
	ws   *workspace.Workspace
	st   *store.Store
	fake *providertest.Fake
	auth *authtest.Fake
	eng  *wsync.Engine
}

// isolate isole le test de l'environnement du développeur (HOME, XDG_CONFIG_HOME, WATCHOO_*).
func isolate(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("HOME", filepath.Join(base, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "xdg"))
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, workspace.EnvPrefix+"_") {
			t.Setenv(name, "")
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// newEnv crée un espace nommé name (répertoire temporaire), sa base migrée et un moteur branché sur un
// faux Provider (vide) et un faux gh connecté. L'appelant doit avoir appelé isolate.
func newEnv(t *testing.T, name string) *env {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := workspace.Init(root)
	if err != nil {
		t.Fatal(err)
	}
	ws := res.Workspace
	if err := workspace.Load(ws); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateDBFile(ws.DBPath()); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.Context(), ws.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, ws: ws, st: st, fake: providertest.New(), auth: authtest.NewLoggedIn("prof")}
	e.eng, err = wsync.New(wsync.Options{
		Store:       st,
		Auth:        e.auth,
		Config:      ws.Config,
		NewProvider: func(string) (provider.Provider, error) { return e.fake, nil },
		Logger:      slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// forge ajoute owner/name au faux Provider (identifiant id).
func (e *env) forge(id int64, owner, name string) {
	e.fake.AddRepository(provider.Repository{ID: id, Owner: owner, Name: name, DefaultBranch: "main",
		URL: "https://github.com/" + owner + "/" + name, Private: true})
}

// track ajoute owner/name à la forge et à l'espace (suivi actif), avec label et motifs.
func (e *env) track(id int64, owner, name, label string, patterns ...string) model.Repository {
	e.t.Helper()
	e.forge(id, owner, name)
	r, err := e.st.CreateRepository(context.Background(), model.Repository{
		Provider: model.ProviderGitHub, ProviderID: id, Owner: owner, Name: name, Label: label,
		URL: "https://github.com/" + owner + "/" + name, Enabled: true, BranchPatterns: patterns,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return r
}

// stop arrête le suivi du dépôt id.
func (e *env) stop(id int64) {
	e.t.Helper()
	if err := e.st.SetRepositoryEnabled(context.Background(), id, false); err != nil {
		e.t.Fatal(err)
	}
}

// repos renvoie les dépôts de l'espace, indexés par owner/name.
func (e *env) repos() map[string]model.Repository {
	e.t.Helper()
	list, err := e.st.ListRepositories(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	out := map[string]model.Repository{}
	for _, r := range list {
		out[r.FullName()] = r
	}
	return out
}

// reload relit les réglages de l'espace depuis le disque.
func (e *env) reload() workspace.Config {
	e.t.Helper()
	ws := &workspace.Workspace{Root: e.ws.Root, Dir: e.ws.Dir}
	if err := workspace.Load(ws); err != nil {
		e.t.Fatal(err)
	}
	return ws.Config
}

// readFile renvoie le contenu de path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
