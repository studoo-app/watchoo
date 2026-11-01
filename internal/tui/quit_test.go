package tui

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
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

// blockingProvider blocks RepositoryByID until the context is cancelled, then lets some more time
// pass before returning (the engine must then close its sync_run): if Run did not wait for the sync,
// the sync_run would still be "running" when Run returns.
type blockingProvider struct {
	provider.Provider
	entered chan struct{}
	once    gosync.Once
}

func (b *blockingProvider) RepositoryByID(ctx context.Context, id int64) (provider.Repository, error) {
	b.once.Do(func() { close(b.entered) })
	<-ctx.Done()
	time.Sleep(200 * time.Millisecond)
	return provider.Repository{}, ctx.Err()
}

// safeWriter is an output protected by a lock (written by Bubble Tea).
type safeWriter struct {
	mu  gosync.Mutex
	buf bytes.Buffer
}

func (w *safeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// Quitting during a sync: Run cancels the sync and waits for it; the sync_run is closed (not
// "running") and the repository is not locked for the next sync.
func TestRunWaitsForSyncOnQuit(t *testing.T) {
	ctx := t.Context()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	fake := providertest.New()
	fake.AddRepository(provider.Repository{ID: 7, Owner: "promo", Name: "projet", DefaultBranch: "main"})
	repo, err := st.CreateRepository(ctx, model.Repository{Provider: model.ProviderGitHub, ProviderID: 7,
		Owner: "promo", Name: "projet", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	blocking := &blockingProvider{Provider: fake, entered: make(chan struct{})}
	engine, err := wsync.New(wsync.Options{
		Store:       st,
		Auth:        authtest.NewLoggedIn(""),
		Config:      workspace.DefaultConfig("promo"),
		NewProvider: func(string) (provider.Provider, error) { return blocking, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	in, keys := io.Pipe()
	t.Cleanup(func() { _ = keys.Close() })
	deps := testDeps(st, engine)
	deps.Now = nil
	returned := make(chan error, 1)
	go func() { returned <- Run(ctx, deps, IO{In: in, Out: &safeWriter{}}) }()

	if _, err := keys.Write([]byte("s")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocking.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the sync did not start")
	}
	if _, err := keys.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return")
	}

	run, err := st.LastSyncRun(ctx, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status == model.SyncRunning {
		t.Fatalf("sync_run still \"running\" when Run returned: %+v", run)
	}
	next, err := st.BeginSyncRun(ctx, repo.ID, time.Now(), wsync.LockStaleAfter)
	if err != nil {
		t.Fatalf("BeginSyncRun after quitting: %v", err)
	}
	if next.ID == run.ID {
		t.Errorf("new sync_run expected, got %+v", next)
	}
}
