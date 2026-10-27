package sync_test

// Scénario de bout en bout : le moteur avec le VRAI provider GitHub (internal/provider/github), contre
// le faux serveur githubtest. Aucun appel réseau réel (127.0.0.1).

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/github"
	"github.com/studoo-app/watchoo/internal/provider/github/githubtest"
	"github.com/studoo-app/watchoo/internal/repoimport"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

func TestBoutEnBoutGitHub(t *testing.T) {
	clk := &clock{t: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)}
	srv := githubtest.NewServer(t, githubtest.Options{Now: clk.Now})
	t.Cleanup(srv.Close)
	logs := &safeBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { assertNoLeak(t, "logs", logs.String()) })

	st := openStore(t, filepath.Join(t.TempDir(), "watchoo.db"), true)
	cache, quota := github.NewCache(0, 0), github.NewQuota()
	gh := authtest.NewLoggedIn("") // token = githubtest.Token
	eng, err := wsync.New(wsync.Options{
		Store: st, Auth: gh, Config: workspace.DefaultConfig("promo"), Now: clk.Now, Logger: logger,
		NewProvider: func(token string) (provider.Provider, error) {
			return github.New(token, github.Options{BaseURL: srv.URL(), Cache: cache, Quota: quota,
				UserAgent: "watchoo/test", Logger: logger})
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	srv.AddRepo(githubtest.Repo{ID: 10, Owner: testOwner, Name: "projet", Private: true})
	srv.AddRepo(githubtest.Repo{ID: 11, Owner: testOwner, Name: "cache", Private: true})
	srv.SetRepoAccess(11, githubtest.AccessHidden)
	srv.AddRepo(githubtest.Repo{ID: 12, Owner: testOwner, Name: "vide", Private: true})
	mainSHAs := srv.AppendCommits(10, "main", 120, epoch, time.Minute) // plusieurs pages
	srv.SetBranch(10, "feature", mainSHAs[50])
	srv.AppendCommits(10, "feature", 2, epoch.Add(24*time.Hour), time.Hour)

	add, err := eng.AddRepositories(t.Context(), []wsync.AddRequest{
		{Line: 1, Ref: repoimport.Ref{Owner: testOwner, Name: "projet"}, Label: "Projet"},
		{Line: 2, Ref: repoimport.Ref{Owner: testOwner, Name: "cache"}},
		{Line: 3, Ref: repoimport.Ref{Owner: testOwner, Name: "vide"}},
	}, wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if add[0].Outcome != wsync.OutcomeAdded || add[1].Outcome != wsync.OutcomeInaccessible ||
		add[2].Outcome != wsync.OutcomeAdded {
		t.Fatalf("ajouts = %+v", add)
	}

	rep, err := eng.SyncAll(t.Context(), wsync.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertReportNoLeak(t, rep)
	if len(rep.Repos) != 2 {
		t.Fatalf("rapports = %+v", rep.Repos)
	}
	projet, vide := rep.Repos[0], rep.Repos[1]
	if projet.Status != wsync.StatusSuccess || projet.NewCommits != 122 || projet.TrackedBranches != 2 ||
		projet.DisplayName != "Projet" {
		t.Errorf("projet = %+v", projet)
	}
	if vide.Status != wsync.StatusSuccess || vide.NewCommits != 0 {
		t.Errorf("vide = %+v", vide)
	}

	// Renommage + nouveaux commits avec statistiques ; feature inchangée.
	srv.RenameRepo(10, testOwner, "projet-final")
	head := mainSHAs[len(mainSHAs)-1]
	c1 := srv.AddCommit(10, githubtest.Commit{Message: secretMessage,
		Author:     githubtest.Person{Name: secretName, Email: secretEmail, Login: secretLogin},
		AuthoredAt: epoch.Add(72 * time.Hour), Parents: []string{head}, Additions: 7, Deletions: 3})
	srv.SetBranch(10, "main", c1)
	srv.ResetRequests()
	rep, err = eng.SyncAll(t.Context(), wsync.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertReportNoLeak(t, rep)
	projet = rep.Repos[0]
	if projet.Status != wsync.StatusSuccess || projet.NewCommits != 1 || projet.StatsFetched != 1 ||
		projet.FullName != testOwner+"/projet-final" {
		t.Errorf("seconde synchro = %+v", projet)
	}
	if !rep.RateLimitKnown || rep.RateLimit.Limit != githubtest.DefaultRateLimit {
		t.Errorf("quota = %+v", rep.RateLimit)
	}
	for _, r := range srv.Requests() {
		if r.Method != "GET" {
			t.Errorf("requête %s %s", r.Method, r.Path)
		}
		if strings.HasSuffix(r.Path, "/commits") {
			t.Errorf("liste des commits relue sur un dépôt déjà importé : %s", r.Path)
		}
	}
	views, err := st.ListRecentCommits(t.Context(), store.CommitFilter{RepoID: add[0].Repo.ID, Limit: 1})
	if err != nil || len(views) != 1 || views[0].SHA != c1 || views[0].Additions != 7 || views[0].Deletions != 3 ||
		views[0].AuthorLogin != secretLogin {
		t.Errorf("dernier commit = %+v, %v", views, err)
	}
	stored, _ := st.GetRepository(t.Context(), add[0].Repo.ID)
	if stored.Name != "projet-final" {
		t.Errorf("dépôt = %+v", stored)
	}

	// Dépôt devenu inaccessible : erreur enregistrée, sans fuite.
	srv.SetRepoAccess(10, githubtest.AccessSSO)
	rep, err = eng.SyncAll(t.Context(), wsync.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertReportNoLeak(t, rep)
	if r := rep.Repos[0]; r.Status != wsync.StatusError || !strings.Contains(r.Error, "SSO") ||
		strings.Contains(r.Error, "authorization_request") {
		t.Errorf("SSO = %+v", r)
	}
	run, _ := st.LastSyncRun(t.Context(), add[0].Repo.ID)
	if run.Status != model.SyncError {
		t.Errorf("sync_run = %+v", run)
	}
	if n := gh.TokenCalls(); n != 4 {
		t.Errorf("Token appelé %d fois, 4 attendus (un ajout, trois synchros)", n)
	}
	if rl, err := eng.RateLimit(t.Context()); err != nil || rl.Limit != githubtest.DefaultRateLimit {
		t.Errorf("RateLimit = %+v, %v", rl, err)
	}
}
