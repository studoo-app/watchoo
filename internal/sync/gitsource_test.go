package sync_test

// Local Git repositories (V2): choice of the source depending on gh, addition and sync through Git
// clones (real repositories served over file://, gittest), metadata kept, back to the API which resolves provider_id.

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	"github.com/studoo-app/watchoo/internal/repoimport"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// secretAuthor is the dummy Git identity of the commits (name and e-mail never in the logs).
var secretAuthor = gittest.Author{Name: secretName, Email: secretEmail}

func TestSourceChoice(t *testing.T) {
	tests := []struct {
		name     string
		tokenErr error
		want     wsync.SourceKind
		wantErr  error
	}{
		{"gh signed in", nil, wsync.SourceAPI, nil},
		{"gh missing", fmt.Errorf("launching gh: %w", auth.ErrGHNotFound), wsync.SourceGit, nil},
		{"gh not signed in", auth.ErrNotLoggedIn, wsync.SourceGit, nil},
		{"unusable github.user", auth.ErrUnusableUser, wsync.SourceGit, nil},
		{"gh broken", auth.ErrTimeout, "", auth.ErrTimeout},
		{"unreadable answer", auth.ErrUnexpectedOutput, "", auth.ErrUnexpectedOutput},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			if tc.tokenErr != nil {
				e.auth.SetTokenErr(tc.tokenErr)
			}
			src, err := e.eng.Source(t.Context())
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("Source: %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || src.Kind != tc.want {
				t.Fatalf("Source = %+v, %v; want %q", src, err, tc.want)
			}
			if (src.Kind == wsync.SourceGit) != (src.Reason != "") {
				t.Errorf("reason %q for %q", src.Reason, src.Kind)
			}
			if src.Kind == wsync.SourceGit && src.Label() != "local Git clones" || src.Kind == wsync.SourceAPI && src.Label() != "GitHub API" {
				t.Errorf("Label = %q", src.Label())
			}
		})
	}
}

func TestGitSourceAddAndSync(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	m0 := r.Commit("main", "README.md", "a\n", secretMessage, secretAuthor, epoch)
	r.Branch("dev", m0)
	f1 := r.Commit("dev", "app.go", "package app\n", secretMessage+" 2", secretAuthor, epoch.Add(time.Hour))
	r.Tag("v1", m0, "")

	e := newEnv(t, withGit(srv.URL))
	e.auth.SetTokenErr(auth.ErrNotLoggedIn) // gh installed, not signed in
	ctx := t.Context()

	// Addition: access checked by git ls-remote, without clone (dry-run included).
	reqs := []wsync.AddRequest{
		{Ref: repoimport.Ref{Owner: testOwner, Name: "projet"}},
		{Ref: repoimport.Ref{Owner: testOwner, Name: "PROJET"}}, // same repository: planned once
		{Ref: repoimport.Ref{Owner: testOwner, Name: "absent"}},
	}
	dry, err := e.eng.AddRepositories(ctx, reqs, wsync.AddOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if dry[0].Outcome != wsync.OutcomeAdded || dry[1].Outcome != wsync.OutcomeAlreadyTracked ||
		dry[2].Outcome != wsync.OutcomeInaccessible {
		t.Errorf("dry-run: %v %v %v", dry[0].Outcome, dry[1].Outcome, dry[2].Outcome)
	}
	if !strings.HasPrefix(dry[2].Error, "access denied") {
		t.Errorf("message of an inaccessible repository: %q", dry[2].Error)
	}
	res, err := e.eng.AddRepositories(ctx, reqs[:1], wsync.AddOptions{})
	if err != nil || res[0].Outcome != wsync.OutcomeAdded {
		t.Fatalf("addition: %+v, %v", res, err)
	}
	repo := res[0].Repo
	if repo.ProviderID != 0 || repo.URL != "https://github.com/"+testOwner+"/projet" || repo.DefaultBranch != "main" {
		t.Errorf("repository added without gh: %+v", repo)
	}
	// Added a second time: found by owner/name.
	if again, _ := e.eng.AddRepositories(ctx, reqs[1:2], wsync.AddOptions{}); again[0].Outcome != wsync.OutcomeAlreadyTracked {
		t.Errorf("second addition: %v", again[0].Outcome)
	}

	// First import.
	rep := e.syncAll()
	if rep.Source.Kind != wsync.SourceGit || rep.RateLimitKnown {
		t.Errorf("source %+v, known quota %v", rep.Source, rep.RateLimitKnown)
	}
	if len(rep.Repos) != 1 || rep.Repos[0].Status != wsync.StatusSuccess || rep.Repos[0].NewCommits != 2 ||
		rep.Repos[0].TrackedBranches != 2 || len(rep.Repos[0].Warnings) != 0 {
		t.Fatalf("first import: %+v", rep.Repos)
	}
	if got := e.commitSHAs(repo.ID, "dev"); !slices.Equal(got, []string{f1}) {
		t.Errorf("commits specific to dev: %v", got)
	}
	tags, err := e.st.ListTags(ctx, store.ReleaseFilter{RepoID: repo.ID})
	if err != nil || len(tags) != 1 || tags[0].SHA != m0 {
		t.Errorf("tags: %+v, %v", tags, err)
	}
	ranking, err := e.st.ContributorRanking(ctx, store.ContributorFilter{RepoID: repo.ID})
	if err != nil || len(ranking) != 1 || ranking[0].ProviderLogin != "" || ranking[0].Commits != 2 {
		t.Errorf("contributors (merged by e-mail, without login): %+v, %v", ranking, err)
	}

	// Incremental sync: one more commit, statistics by numstat (without quota).
	m1 := r.Commit("main", "README.md", "a\nb\nc\n", secretMessage+" 3", secretAuthor, epoch.Add(2*time.Hour))
	e.clock.Set(e.clock.Now().Add(time.Hour))
	rep = e.syncAll()
	if r0 := rep.Repos[0]; r0.Status != wsync.StatusSuccess || r0.NewCommits != 1 || r0.StatsFetched != 1 {
		t.Fatalf("incremental sync: %+v", r0)
	}
	commits, err := e.st.ListRecentCommits(ctx, store.CommitFilter{RepoID: repo.ID, Limit: 1})
	if err != nil || len(commits) != 1 || commits[0].SHA != m1 || commits[0].Additions != 2 {
		t.Errorf("last commit: %+v, %v", commits, err)
	}
	got, err := e.st.GetRepository(ctx, repo.ID)
	if err != nil || got.ProviderID != 0 || got.LastSyncedAt.IsZero() || got.ForgeDetails() {
		t.Errorf("repository after Git sync: %+v, %v", got, err)
	}

	// Repository that became inaccessible: in error, "access denied".
	r.Remove()
	rep = e.syncAll()
	if r0 := rep.Repos[0]; r0.Status != wsync.StatusError || !strings.Contains(r0.Error, "access denied") {
		t.Errorf("inaccessible repository: %+v", r0)
	}

	// gh signed in again: back to the API, which resolves provider_id by owner/name.
	e.fake.AddRepository(provider.Repository{ID: 9001, Owner: testOwner, Name: "projet", DefaultBranch: "main",
		URL: "https://github.com/" + testOwner + "/projet", Stars: 4, OpenIssues: 2})
	e.auth.SetTokenErr(nil)
	rep = e.syncAll()
	if rep.Source.Kind != wsync.SourceAPI || rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("back to the API: %+v", rep)
	}
	got, err = e.st.GetRepository(ctx, repo.ID)
	if err != nil || got.ProviderID != 9001 || got.Stars != 4 || got.OpenIssues != 2 || !got.ForgeDetails() {
		t.Errorf("repository after going back to the API: %+v, %v", got, err)
	}
}

func TestGitSourceKeepsForgeMetadata(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "suivi", "main")
	r.Commit("main", "f.txt", "1\n", secretMessage, secretAuthor, epoch)

	e := newEnv(t, withGit(srv.URL))
	ctx := t.Context()
	// Repository known by the API (identifier, stars, issues, archived), then synced without gh.
	repo, err := e.st.CreateRepository(ctx, model.Repository{Provider: model.ProviderGitHub, ProviderID: 77,
		Owner: testOwner, Name: "suivi", URL: "https://github.com/" + testOwner + "/suivi", Stars: 12,
		OpenIssues: 3, Archived: true, Enabled: true, DefaultBranch: "old"})
	if err != nil {
		t.Fatal(err)
	}
	e.auth.SetTokenErr(fmt.Errorf("launching: %w", auth.ErrGHNotFound))
	rep := e.syncAll()
	if rep.Repos[0].Status != wsync.StatusSuccess || rep.Repos[0].NewCommits != 1 {
		t.Fatalf("Git sync of a repository known by the API: %+v", rep.Repos[0])
	}
	got, err := e.st.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProviderID != 77 || got.Stars != 12 || got.OpenIssues != 3 || !got.Archived ||
		got.URL != repo.URL || got.DefaultBranch != "main" {
		t.Errorf("metadata after Git sync: %+v (identifier, stars, issues, archived and URL kept; default branch up to date)", got)
	}
	if len(e.fake.CallLog()) != 0 {
		t.Error("the API was called without gh")
	}
}

// CloneRefs sorts the repositories by order of addition (increasing identifier), tracked or stopped:
// it is the order of the homonym rule of the clones.
func TestCloneRefsOrder(t *testing.T) {
	repos := []model.Repository{
		{ID: 3, Owner: "carol", Name: "projet"},
		{ID: 1, Owner: "bob", Name: "projet", Enabled: false},
		{ID: 2, Owner: "alice", Name: "projet", Enabled: true},
	}
	got := wsync.CloneRefs(repos)
	want := []string{"bob/projet", "alice/projet", "carol/projet"}
	if len(got) != len(want) {
		t.Fatalf("CloneRefs = %v", got)
	}
	for i, r := range got {
		if r.Owner+"/"+r.Name != want[i] {
			t.Errorf("CloneRefs[%d] = %s/%s, want %s", i, r.Owner, r.Name, want[i])
		}
	}
	if repos[0].ID != 3 {
		t.Error("CloneRefs changed the received list")
	}
}

// Contributor known before 0.5.0 (login recorded, empty profile page): the next sync fills the
// profile page from the login, through the API as through the local Git clones, without request
// or login in the logs (assertNoLeak).
func TestSyncBackfillsProfileURL(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	r.Commit("main", "README.md", "a\n", secretMessage, secretAuthor, epoch)

	e := newEnv(t, withGit(srv.URL))
	ctx := t.Context()
	repo := e.addRepo(1, "projet")
	// Author with login, without ProfileURL (as before 0.5.0).
	e.fake.SetBranch(repo.ID, "main", e.commit(repo.ID, "c1", epoch))

	profileURL := func() string {
		t.Helper()
		var url string
		if err := e.st.DB().QueryRowContext(ctx,
			"SELECT profile_url FROM contributors WHERE provider_login = ?", secretLogin).Scan(&url); err != nil {
			t.Fatal(err)
		}
		return url
	}
	clear := func() {
		t.Helper()
		if _, err := e.st.DB().ExecContext(ctx, "UPDATE contributors SET profile_url = ''"); err != nil {
			t.Fatal(err)
		}
	}

	e.syncOK(repo.ID)
	if got, want := profileURL(), "https://github.example.test/"+secretLogin; got != want {
		t.Errorf("after the first sync: profile_url = %q, want %q", got, want)
	}

	// Database from before 0.5.0: no new commit, the address is filled anyway.
	clear()
	if rep := e.syncOK(repo.ID); rep.NewCommits != 0 {
		t.Fatalf("%d new commits, none expected", rep.NewCommits)
	}
	if got, want := profileURL(), "https://github.example.test/"+secretLogin; got != want {
		t.Errorf("sync through the API: profile_url = %q, want %q", got, want)
	}

	// Without gh: local Git clones, same fill (github.com address).
	clear()
	e.auth.SetTokenErr(auth.ErrNotLoggedIn)
	rep := e.syncAll()
	if rep.Source.Kind != wsync.SourceGit {
		t.Fatalf("source %q, clones Git attendus", rep.Source.Kind)
	}
	if got, want := profileURL(), "https://github.com/"+secretLogin; got != want {
		t.Errorf("sync through Git clones: profile_url = %q, want %q", got, want)
	}
}
