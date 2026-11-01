package configshare_test

// Helpers shared by the tests: temporary workspace (workspace.Init), migrated database, fake Provider,
// fake gh. No network.

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

// testNow is the date of the exports.
var testNow = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

// env is a complete test workspace.
type env struct {
	t    *testing.T
	ws   *workspace.Workspace
	st   *store.Store
	fake *providertest.Fake
	auth *authtest.Fake
	eng  *wsync.Engine
}

// isolate isolates the test from the developer's environment (HOME, XDG_CONFIG_HOME, WATCHOO_*).
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

// newEnv creates a workspace named name (temporary directory), its migrated database and an engine
// plugged into a fake (empty) Provider and a signed-in fake gh. The caller must have called isolate.
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

// forge adds owner/name to the fake Provider (identifier id).
func (e *env) forge(id int64, owner, name string) {
	e.fake.AddRepository(provider.Repository{ID: id, Owner: owner, Name: name, DefaultBranch: "main",
		URL: "https://github.com/" + owner + "/" + name, Private: true})
}

// track adds owner/name to the forge and to the workspace (active tracking), with label and patterns.
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

// stop stops the tracking of the repository id.
func (e *env) stop(id int64) {
	e.t.Helper()
	if err := e.st.SetRepositoryEnabled(context.Background(), id, false); err != nil {
		e.t.Fatal(err)
	}
}

// repos returns the repositories of the workspace, indexed by owner/name.
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

// reload reads the workspace's settings again from the disk.
func (e *env) reload() workspace.Config {
	e.t.Helper()
	ws := &workspace.Workspace{Root: e.ws.Root, Dir: e.ws.Dir}
	if err := workspace.Load(ws); err != nil {
		e.t.Fatal(err)
	}
	return ws.Config
}

// readFile returns the content of path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
