package sync_test

// Languages of a repository (1.1.0) through the engine: read from the clone with gh, kept with a warning
// when the clone cannot give them.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/provider"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// failLanguages cannot read the languages (the rest of the clone works).
type failLanguages struct{ *gitprov.Provider }

func (failLanguages) Languages(context.Context, string, string) ([]provider.Language, error) {
	return nil, errors.New("ls-tree illisible")
}

func TestSyncLanguages(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	r.Commit("main", "main.go", strings.Repeat("g", 750), secretMessage, secretAuthor, epoch)
	r.Commit("main", "index.html", strings.Repeat("h", 250), secretMessage+" 2", secretAuthor, epoch)
	e := newEnv(t, withGit(srv.URL))
	repo := e.addRepo(1, "projet")
	e.mirror(srv.URL, 1, "projet")
	ctx := t.Context()

	rep, err := e.eng.SyncAll(ctx, wsync.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if w := rep.Repos[0].Warnings; len(w) != 0 {
		t.Errorf("warnings: %v", w)
	}
	got, err := e.st.ListLanguages(ctx, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Language != "Go" || got[0].Permille() != 750 || got[1].Language != "HTML" {
		t.Fatalf("languages = %+v", got)
	}

	// Unreadable: the repository stays successful, with a warning, and keeps its languages.
	e.gitWrap = func(p *gitprov.Provider) provider.Provider { return failLanguages{p} }
	e.eng = e.newEngine(e.st)
	rep, err = e.eng.SyncAll(ctx, wsync.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r0 := rep.Repos[0]
	if r0.Status != wsync.StatusSuccess || len(r0.Warnings) != 1 ||
		!strings.Contains(r0.Warnings[0], "languages not computed") {
		t.Errorf("repository: %+v", r0)
	}
	if got, err := e.st.ListLanguages(ctx, repo.ID); err != nil || len(got) != 2 {
		t.Errorf("languages after the failure: %+v, %v", got, err)
	}
}
