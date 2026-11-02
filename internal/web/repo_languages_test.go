package web

// Languages and share of the contributors on a repository page (1.1.0).

import (
	"net/http"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

func TestRepoLanguages(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	path := repoPath(ids.Alice)

	w := e.get(path)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="repo-languages"`, "No language yet")

	ctx := t.Context()
	run, err := e.store.BeginSyncRun(ctx, ids.Alice, e.now.Add(-30*time.Minute), wsync.LockStaleAfter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ApplyRepoSync(ctx, store.RepoSyncWrite{
		RepoID: ids.Alice, RunID: run.ID,
		Metadata:         store.RepositoryMetadata{Owner: "promo", Name: "alice", DefaultBranch: "main"},
		ReplaceLanguages: true,
		Languages:        []model.RepositoryLanguage{{Language: "Go", Bytes: 624}, {Language: "HTML", Bytes: 376}},
		SyncedAt:         e.now.Add(-29 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	w = e.get(path)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Go", "62.4%", "HTML", "37.6%", `value="624" max="1000"`, "Share of the commits of this repository")

	// Fragment alone, reloaded at the end of a sync.
	f := e.htmxWith(path, map[string]string{"HX-Target": repoLanguagesID})
	assertStatus(t, f, http.StatusOK)
	assertContains(t, f, `id="repo-languages"`, "62.4%")
	assertNotContains(t, f, `id="repo-contributors"`)
}
