package sync_test

// Languages of a repository (1.1.0) through the engine: read from the clone with gh, kept with a warning
// when the clone cannot give them.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	"github.com/studoo-app/watchoo/internal/store"
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
		!strings.Contains(r0.Warnings[0].Message, "languages not computed") {
		t.Errorf("repository: %+v", r0)
	}
	if got, err := e.st.ListLanguages(ctx, repo.ID); err != nil || len(got) != 2 {
		t.Errorf("languages after the failure: %+v, %v", got, err)
	}
}

// Languages of the commits (1.2.0): read from the clone with the line statistics at the first import,
// caught up once for a database from before 1.2.0 (lines kept), and summed per contributor.
func TestSyncCommitLanguages(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	goSHA := r.Commit("main", "main.go", "package main\n\nfunc main() {}\n", secretMessage, secretAuthor, epoch)
	r.Commit("main", "index.php", "<?php\n", secretMessage+" 2", secretAuthor, epoch.Add(time.Hour))
	e := newEnv(t, withGit(srv.URL))
	repo := e.addRepo(1, "projet")
	e.mirror(srv.URL, 1, "projet")
	ctx := t.Context()

	e.syncAll()
	languages := func() string {
		t.Helper()
		var got string
		if err := e.st.DB().QueryRowContext(ctx, `SELECT COALESCE(group_concat(language || ':' || additions, ','), '')
			FROM (SELECT language, additions FROM commit_languages WHERE repo_id = ? ORDER BY language)`, repo.ID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := languages(); got != "Go:3,PHP:1" {
		t.Errorf("languages after the first import = %q", got)
	}

	// Database from before 1.2.0: the next sync, without any new commit, reads the languages again and
	// keeps the known lines.
	if _, err := e.st.DB().ExecContext(ctx, `DELETE FROM commit_languages`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.DB().ExecContext(ctx, `UPDATE commits SET languages_version = 0, additions = 42 WHERE sha = ?`, goSHA); err != nil {
		t.Fatal(err)
	}
	if r0 := e.syncAll().Repos[0]; r0.Status != wsync.StatusSuccess || r0.StatsFetched != 1 || r0.NewCommits != 0 {
		t.Fatalf("catch-up: %+v", r0)
	}
	if got := languages(); got != "Go:3" {
		t.Errorf("languages after the catch-up = %q (the commit read before keeps nothing new)", got)
	}
	if add, _, _ := e.stats(repo.ID, goSHA); add != 42 {
		t.Errorf("known lines recomputed: +%d", add)
	}
	if n := e.syncAll().Repos[0].StatsFetched; n != 0 {
		t.Errorf("third sync: %d commits read again", n)
	}

	ranking, err := e.st.ContributorRanking(ctx, store.ContributorFilter{})
	if err != nil || len(ranking) != 1 {
		t.Fatalf("ranking = %+v, %v", ranking, err)
	}
	if l := ranking[0].Languages; l.Main() != "Go" || l.Scanned != 2 || l.Pending() != 0 {
		t.Errorf("languages of the contributor = %+v", l)
	}
}
