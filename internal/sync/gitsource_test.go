package sync_test

// Dépôts Git locaux (V2) : choix de la source selon gh, ajout et synchro par clones Git (vrais dépôts
// servis par file://, gittest), métadonnées gardées, retour à l'API qui résout provider_id.

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

// secretAuthor est l'identité Git factice des commits (nom et e-mail jamais dans les logs).
var secretAuthor = gittest.Author{Name: secretName, Email: secretEmail}

func TestSourceChoice(t *testing.T) {
	tests := []struct {
		name     string
		tokenErr error
		want     wsync.SourceKind
		wantErr  error
	}{
		{"gh connecté", nil, wsync.SourceAPI, nil},
		{"gh absent", fmt.Errorf("lancement de gh : %w", auth.ErrGHNotFound), wsync.SourceGit, nil},
		{"gh non connecté", auth.ErrNotLoggedIn, wsync.SourceGit, nil},
		{"github.user inutilisable", auth.ErrUnusableUser, wsync.SourceGit, nil},
		{"gh en panne", auth.ErrTimeout, "", auth.ErrTimeout},
		{"réponse illisible", auth.ErrUnexpectedOutput, "", auth.ErrUnexpectedOutput},
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
					t.Errorf("Source : %v, %v attendue", err, tc.wantErr)
				}
				return
			}
			if err != nil || src.Kind != tc.want {
				t.Fatalf("Source = %+v, %v ; %q attendu", src, err, tc.want)
			}
			if (src.Kind == wsync.SourceGit) != (src.Reason != "") {
				t.Errorf("raison %q pour %q", src.Reason, src.Kind)
			}
			if src.Kind == wsync.SourceGit && src.Label() != "clones Git locaux" || src.Kind == wsync.SourceAPI && src.Label() != "API GitHub" {
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
	e.auth.SetTokenErr(auth.ErrNotLoggedIn) // gh installé, non connecté
	ctx := t.Context()

	// Ajout : accès vérifié par git ls-remote, sans clone (dry-run compris).
	reqs := []wsync.AddRequest{
		{Ref: repoimport.Ref{Owner: testOwner, Name: "projet"}},
		{Ref: repoimport.Ref{Owner: testOwner, Name: "PROJET"}}, // même dépôt : planifié une fois
		{Ref: repoimport.Ref{Owner: testOwner, Name: "absent"}},
	}
	dry, err := e.eng.AddRepositories(ctx, reqs, wsync.AddOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if dry[0].Outcome != wsync.OutcomeAdded || dry[1].Outcome != wsync.OutcomeAlreadyTracked ||
		dry[2].Outcome != wsync.OutcomeInaccessible {
		t.Errorf("dry-run : %v %v %v", dry[0].Outcome, dry[1].Outcome, dry[2].Outcome)
	}
	if !strings.HasPrefix(dry[2].Error, "access denied") {
		t.Errorf("message d'un dépôt inaccessible : %q", dry[2].Error)
	}
	res, err := e.eng.AddRepositories(ctx, reqs[:1], wsync.AddOptions{})
	if err != nil || res[0].Outcome != wsync.OutcomeAdded {
		t.Fatalf("ajout : %+v, %v", res, err)
	}
	repo := res[0].Repo
	if repo.ProviderID != 0 || repo.URL != "https://github.com/"+testOwner+"/projet" || repo.DefaultBranch != "main" {
		t.Errorf("dépôt ajouté sans gh : %+v", repo)
	}
	// Ajouté une seconde fois : retrouvé par owner/name.
	if again, _ := e.eng.AddRepositories(ctx, reqs[1:2], wsync.AddOptions{}); again[0].Outcome != wsync.OutcomeAlreadyTracked {
		t.Errorf("second ajout : %v", again[0].Outcome)
	}

	// Premier import.
	rep := e.syncAll()
	if rep.Source.Kind != wsync.SourceGit || rep.RateLimitKnown {
		t.Errorf("source %+v, quota connu %v", rep.Source, rep.RateLimitKnown)
	}
	if len(rep.Repos) != 1 || rep.Repos[0].Status != wsync.StatusSuccess || rep.Repos[0].NewCommits != 2 ||
		rep.Repos[0].TrackedBranches != 2 || len(rep.Repos[0].Warnings) != 0 {
		t.Fatalf("premier import : %+v", rep.Repos)
	}
	if got := e.commitSHAs(repo.ID, "dev"); !slices.Equal(got, []string{f1}) {
		t.Errorf("commits propres à dev : %v", got)
	}
	tags, err := e.st.ListTags(ctx, store.ReleaseFilter{RepoID: repo.ID})
	if err != nil || len(tags) != 1 || tags[0].SHA != m0 {
		t.Errorf("tags : %+v, %v", tags, err)
	}
	ranking, err := e.st.ContributorRanking(ctx, store.ContributorFilter{RepoID: repo.ID})
	if err != nil || len(ranking) != 1 || ranking[0].ProviderLogin != "" || ranking[0].Commits != 2 {
		t.Errorf("contributeurs (fusion par e-mail, sans login) : %+v, %v", ranking, err)
	}

	// Synchro incrémentale : un commit de plus, statistiques par numstat (sans quota).
	m1 := r.Commit("main", "README.md", "a\nb\nc\n", secretMessage+" 3", secretAuthor, epoch.Add(2*time.Hour))
	e.clock.Set(e.clock.Now().Add(time.Hour))
	rep = e.syncAll()
	if r0 := rep.Repos[0]; r0.Status != wsync.StatusSuccess || r0.NewCommits != 1 || r0.StatsFetched != 1 {
		t.Fatalf("synchro incrémentale : %+v", r0)
	}
	commits, err := e.st.ListRecentCommits(ctx, store.CommitFilter{RepoID: repo.ID, Limit: 1})
	if err != nil || len(commits) != 1 || commits[0].SHA != m1 || commits[0].Additions != 2 {
		t.Errorf("dernier commit : %+v, %v", commits, err)
	}
	got, err := e.st.GetRepository(ctx, repo.ID)
	if err != nil || got.ProviderID != 0 || got.LastSyncedAt.IsZero() || got.ForgeDetails() {
		t.Errorf("dépôt après synchro Git : %+v, %v", got, err)
	}

	// Dépôt devenu inaccessible : en erreur, « accès refusé ».
	r.Remove()
	rep = e.syncAll()
	if r0 := rep.Repos[0]; r0.Status != wsync.StatusError || !strings.Contains(r0.Error, "access denied") {
		t.Errorf("dépôt inaccessible : %+v", r0)
	}

	// gh connecté à nouveau : retour à l'API, qui résout provider_id par owner/name.
	e.fake.AddRepository(provider.Repository{ID: 9001, Owner: testOwner, Name: "projet", DefaultBranch: "main",
		URL: "https://github.com/" + testOwner + "/projet", Stars: 4, OpenIssues: 2})
	e.auth.SetTokenErr(nil)
	rep = e.syncAll()
	if rep.Source.Kind != wsync.SourceAPI || rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("retour à l'API : %+v", rep)
	}
	got, err = e.st.GetRepository(ctx, repo.ID)
	if err != nil || got.ProviderID != 9001 || got.Stars != 4 || got.OpenIssues != 2 || !got.ForgeDetails() {
		t.Errorf("dépôt après retour à l'API : %+v, %v", got, err)
	}
}

func TestGitSourceKeepsForgeMetadata(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "suivi", "main")
	r.Commit("main", "f.txt", "1\n", secretMessage, secretAuthor, epoch)

	e := newEnv(t, withGit(srv.URL))
	ctx := t.Context()
	// Dépôt connu par l'API (identifiant, étoiles, issues, archivé), puis synchronisé sans gh.
	repo, err := e.st.CreateRepository(ctx, model.Repository{Provider: model.ProviderGitHub, ProviderID: 77,
		Owner: testOwner, Name: "suivi", URL: "https://github.com/" + testOwner + "/suivi", Stars: 12,
		OpenIssues: 3, Archived: true, Enabled: true, DefaultBranch: "ancienne"})
	if err != nil {
		t.Fatal(err)
	}
	e.auth.SetTokenErr(fmt.Errorf("lancement : %w", auth.ErrGHNotFound))
	rep := e.syncAll()
	if rep.Repos[0].Status != wsync.StatusSuccess || rep.Repos[0].NewCommits != 1 {
		t.Fatalf("synchro Git d'un dépôt connu par l'API : %+v", rep.Repos[0])
	}
	got, err := e.st.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProviderID != 77 || got.Stars != 12 || got.OpenIssues != 3 || !got.Archived ||
		got.URL != repo.URL || got.DefaultBranch != "main" {
		t.Errorf("métadonnées après synchro Git : %+v (identifiant, étoiles, issues, archivé et URL gardés ; branche par défaut à jour)", got)
	}
	if len(e.fake.CallLog()) != 0 {
		t.Error("l'API a été appelée sans gh")
	}
}

// CloneRefs range les dépôts par ordre d'ajout (identifiant croissant), suivis ou arrêtés : c'est
// l'ordre de la règle des homonymes des clones.
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
			t.Errorf("CloneRefs[%d] = %s/%s, attendu %s", i, r.Owner, r.Name, want[i])
		}
	}
	if repos[0].ID != 3 {
		t.Error("CloneRefs a modifié la liste reçue")
	}
}

// Contributeur connu avant la 0.5.0 (login enregistré, page de profil vide) : la synchro suivante
// complète la page de profil à partir du login, par l'API comme par les clones Git locaux, sans
// requête ni login dans les logs (assertNoLeak).
func TestSyncBackfillsProfileURL(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	r.Commit("main", "README.md", "a\n", secretMessage, secretAuthor, epoch)

	e := newEnv(t, withGit(srv.URL))
	ctx := t.Context()
	repo := e.addRepo(1, "projet")
	// Auteur avec login, sans ProfileURL (comme avant la 0.5.0).
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
		t.Errorf("après la première synchro : profile_url = %q, attendu %q", got, want)
	}

	// Base d'avant la 0.5.0 : aucun nouveau commit, l'adresse est quand même complétée.
	clear()
	if rep := e.syncOK(repo.ID); rep.NewCommits != 0 {
		t.Fatalf("%d nouveaux commits, aucun attendu", rep.NewCommits)
	}
	if got, want := profileURL(), "https://github.example.test/"+secretLogin; got != want {
		t.Errorf("synchro par l'API : profile_url = %q, attendu %q", got, want)
	}

	// Sans gh : clones Git locaux, même complément (adresse github.com).
	clear()
	e.auth.SetTokenErr(auth.ErrNotLoggedIn)
	rep := e.syncAll()
	if rep.Source.Kind != wsync.SourceGit {
		t.Fatalf("source %q, clones Git attendus", rep.Source.Kind)
	}
	if got, want := profileURL(), "https://github.com/"+secretLogin; got != want {
		t.Errorf("synchro par clones Git : profile_url = %q, attendu %q", got, want)
	}
}
