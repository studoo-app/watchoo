package git_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

var (
	alice = gittest.Author{Name: "Alice Martin", Email: "alice@example.test"}
	bob   = gittest.Author{Name: "Bob Durand", Email: "bob@example.test"}
	t0    = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
)

// newProvider renvoie un provider branché sur srv, ses clones dans un répertoire temporaire, et le
// tampon de ses logs.
func newProvider(t *testing.T, srv *gittest.Server) (*git.Provider, string, *bytes.Buffer) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repos")
	var logs bytes.Buffer
	p, err := git.New(git.Options{Dir: dir, BaseURL: srv.URL,
		Logger: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	if err != nil {
		t.Fatal(err)
	}
	return p, dir, &logs
}

// fresh renvoie un nouveau provider sur les mêmes clones (synchro suivante : nouveau fetch).
func fresh(t *testing.T, srv *gittest.Server, dir string) *git.Provider {
	t.Helper()
	p, err := git.New(git.Options{Dir: dir, BaseURL: srv.URL, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func shas(commits []provider.Commit) []string {
	out := make([]string, len(commits))
	for i, c := range commits {
		out[i] = c.SHA
	}
	return out
}

func TestNewOptions(t *testing.T) {
	if _, err := git.New(git.Options{}); err == nil {
		t.Error("répertoire des clones vide accepté")
	}
	if _, err := git.New(git.Options{Dir: t.TempDir(), Protocol: "ftp"}); err == nil {
		t.Error("protocole inconnu accepté")
	}
	p, err := git.New(git.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != model.ProviderGitHub {
		t.Errorf("Name = %q, la forge des dépôts (github) attendue", p.Name())
	}
}

func TestRepositoryAndHistory(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	m0 := r.Commit("main", "README.md", "a\nb\n", "init", alice, t0)
	m1 := r.Commit("main", "README.md", "a\nb\nc\n", "ajout c\n\ncorps du message", bob, t0.Add(time.Hour))
	r.Branch("dev", m0)
	f1 := r.Commit("dev", "src/app.go", "package app\n", "feature", alice, t0.Add(2*time.Hour))
	x := r.Merge("dev", "main", "merge main", alice, t0.Add(3*time.Hour))
	r.Tag("v1.0", m1, "")
	r.Tag("v1.1", m1, "release annotée")

	p, dir, logs := newProvider(t, srv)
	ctx := t.Context()

	meta, err := p.Repository(ctx, "alice", "projet")
	if err != nil {
		t.Fatal(err)
	}
	want := provider.Repository{Owner: "alice", Name: "projet", DefaultBranch: "main",
		URL: "https://github.com/alice/projet", Partial: true}
	if meta != want {
		t.Errorf("Repository = %+v, attendu %+v", meta, want)
	}
	// Repository ne clone rien (vérification d'accès d'un ajout, --dry-run compris).
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Repository a créé %s (err %v)", dir, err)
	}

	branches, err := p.Branches(ctx, "alice", "projet")
	if err != nil {
		t.Fatal(err)
	}
	wantBranches := []provider.Branch{{Name: "dev", HeadSHA: x}, {Name: "main", HeadSHA: m1}}
	if !slices.Equal(branches, wantBranches) {
		t.Errorf("Branches = %v, attendu %v", branches, wantBranches)
	}
	clone, _ := git.ClonePath(dir, "alice", "projet")
	if runtime.GOOS != "windows" {
		for _, d := range []string{dir, filepath.Dir(clone), clone} {
			info, err := os.Stat(d)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o700 {
				t.Errorf("%s en %o, 0700 attendu", d, info.Mode().Perm())
			}
		}
	}

	all, err := p.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if got := shas(all); !slices.Equal(got, []string{m1, m0}) {
		t.Errorf("ListCommits(main) = %v, attendu [m1 m0]", got)
	}
	c := all[0]
	if c.Message != "ajout c\n\ncorps du message" || c.Author != (provider.Person{Name: bob.Name, Email: bob.Email}) ||
		!c.AuthoredAt.Equal(t0.Add(time.Hour)) || c.AuthoredAt.Location() != time.UTC ||
		!slices.Equal(c.Parents, []string{m0}) || c.URL != "https://github.com/alice/projet/commit/"+m1 {
		t.Errorf("commit mal lu : %+v", c)
	}
	if len(all[1].Parents) != 0 || all[1].Parents == nil {
		t.Errorf("commit racine : parents %v, liste vide non nil attendue", all[1].Parents)
	}

	// HEAD du clone, SHA, Limit, Since, Stop.
	head, err := p.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	if err != nil || !slices.Equal(shas(head), []string{m1, m0}) {
		t.Errorf("ListCommits(HEAD) = %v, %v", shas(head), err)
	}
	bySHA, err := p.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Branch: x, Limit: 2})
	if err != nil || len(bySHA) != 2 || bySHA[0].SHA != x {
		t.Errorf("ListCommits(SHA, Limit 2) = %v, %v", shas(bySHA), err)
	}
	if got := bySHA[0].Parents; len(got) != 2 {
		t.Errorf("merge : %d parents, 2 attendus", len(got))
	}
	since, err := p.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Branch: "dev", Since: t0.Add(90 * time.Minute)})
	if err != nil || !slices.Equal(shas(since), []string{x, f1}) {
		t.Errorf("ListCommits(Since) = %v, %v", shas(since), err)
	}
	stopped, err := p.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Branch: "dev",
		Stop: func(sha string) bool { return sha == f1 }})
	if err != nil || !slices.Equal(shas(stopped), []string{x}) {
		t.Errorf("ListCommits(Stop) = %v, %v", shas(stopped), err)
	}
	if _, err := p.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Branch: "inconnue"}); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("branche inconnue : %v, ErrNotFound attendue", err)
	}

	// Comparaison : commits propres à dev depuis main (merge compris).
	cmp, err := p.CompareCommits(ctx, "alice", "projet", m1, x)
	if err != nil {
		t.Fatal(err)
	}
	if got := shas(cmp); !slices.Equal(got, []string{x, f1}) {
		t.Errorf("CompareCommits(m1, x) = %v, attendu [x f1]", got)
	}
	if same, err := p.CompareCommits(ctx, "alice", "projet", x, x); err != nil || len(same) != 0 || same == nil {
		t.Errorf("CompareCommits(x, x) = %v, %v ; liste vide non nil attendue", same, err)
	}
	unknown := strings.Repeat("ab", 20)
	if _, err := p.CompareCommits(ctx, "alice", "projet", unknown, x); !errors.Is(err, provider.ErrUnknownBase) {
		t.Errorf("base inconnue : %v, ErrUnknownBase attendue", err)
	}

	// Statistiques (numstat) : commit simple, commit racine, merge (par rapport au premier parent).
	for _, tc := range []struct {
		sha  string
		want provider.CommitStats
	}{
		{m1, provider.CommitStats{Additions: 1}},
		{m0, provider.CommitStats{Additions: 2}},
		{f1, provider.CommitStats{Additions: 1}},
		{x, provider.CommitStats{Additions: 1}}, // le merge apporte « c » à dev
	} {
		st, err := p.CommitStats(ctx, "alice", "projet", tc.sha)
		if err != nil || st != tc.want {
			t.Errorf("CommitStats(%s) = %+v, %v ; attendu %+v", provider.ShortSHA(tc.sha), st, err, tc.want)
		}
	}
	if _, err := p.CommitStats(ctx, "alice", "projet", unknown); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("statistiques d'un commit inconnu : %v, ErrNotFound attendue", err)
	}

	tags, err := p.Tags(ctx, "alice", "projet")
	if err != nil {
		t.Fatal(err)
	}
	if want := []provider.Tag{{Name: "v1.0", SHA: m1}, {Name: "v1.1", SHA: m1}}; !slices.Equal(tags, want) {
		t.Errorf("Tags = %v, attendu %v", tags, want)
	}

	// Ce que la source ne fournit pas.
	if _, err := p.PullRequests(ctx, "alice", "projet", provider.PullRequestsOptions{}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("PullRequests : %v, ErrUnsupported attendue", err)
	}
	if _, err := p.Releases(ctx, "alice", "projet"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("Releases : %v, ErrUnsupported attendue", err)
	}
	if _, err := p.RepositoryByID(ctx, 42); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("RepositoryByID : %v, ErrUnsupported attendue", err)
	}
	if _, err := p.RateLimit(ctx); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("RateLimit : %v, ErrUnsupported attendue", err)
	}
	if _, ok := p.LastRateLimit(); ok {
		t.Error("LastRateLimit : aucun quota attendu")
	}

	// Aucune donnée personnelle dans les logs.
	for _, secret := range []string{alice.Name, alice.Email, bob.Email, "corps du message", srv.URL} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("les logs contiennent %q :\n%s", secret, logs.String())
		}
	}
}

func TestFetchFollowsRemote(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	m0 := r.Commit("main", "a.txt", "1\n", "m0", alice, t0)
	m1 := r.Commit("main", "a.txt", "2\n", "m1", alice, t0.Add(time.Hour))
	r.Branch("old", m0)
	r.Tag("v0", m0, "")

	p, dir, _ := newProvider(t, srv)
	ctx := t.Context()
	if _, err := p.Branches(ctx, "alice", "projet"); err != nil {
		t.Fatal(err)
	}

	// Côté distant : branche supprimée, tag supprimé, historique réécrit, nouvelle branche par défaut.
	r.DeleteBranch("old")
	r.Reset("main", m0)
	m2 := r.Commit("main", "a.txt", "3\n", "m2", bob, t0.Add(2*time.Hour))
	r.Commit("trunk", "b.txt", "x\n", "t", bob, t0.Add(3*time.Hour))
	r.SetDefaultBranch("trunk")

	// Le même Provider ne refait pas de fetch (une mise à jour par synchro).
	if b, _ := p.Branches(ctx, "alice", "projet"); len(b) != 2 {
		t.Errorf("même Provider : %v, branches inchangées attendues", b)
	}

	p2 := fresh(t, srv, dir)
	meta, err := p2.Repository(ctx, "alice", "projet")
	if err != nil || meta.DefaultBranch != "trunk" {
		t.Errorf("Repository après changement : %+v, %v", meta, err)
	}
	branches, err := p2.Branches(ctx, "alice", "projet")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(branches))
	for i, b := range branches {
		names[i] = b.Name
	}
	if !slices.Equal(names, []string{"main", "trunk"}) || branches[0].HeadSHA != m2 {
		t.Errorf("branches après fetch = %v", branches)
	}
	// m1 a disparu de main mais reste dans le clone : la comparaison réussit (commits de m2 absents de m1).
	if cmp, err := p2.CompareCommits(ctx, "alice", "projet", m1, m2); err != nil || !slices.Equal(shas(cmp), []string{m2}) {
		t.Errorf("CompareCommits après force-push = %v, %v", shas(cmp), err)
	}
	r.Tag("v1", m2, "")
	if tags, err := fresh(t, srv, dir).Tags(ctx, "alice", "projet"); err != nil || len(tags) != 2 {
		t.Errorf("tags après fetch = %v, %v", tags, err)
	}
}

func TestEmptyRepository(t *testing.T) {
	srv := gittest.NewServer(t)
	srv.Create("alice", "vide", "main")
	p, _, _ := newProvider(t, srv)
	ctx := t.Context()
	meta, err := p.Repository(ctx, "alice", "vide")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Owner != "alice" || !meta.Partial {
		t.Errorf("Repository(vide) = %+v", meta)
	}
	if b, err := p.Branches(ctx, "alice", "vide"); err != nil || len(b) != 0 || b == nil {
		t.Errorf("Branches(vide) = %v, %v ; liste vide non nil attendue", b, err)
	}
	for _, branch := range []string{"", "main"} {
		if c, err := p.ListCommits(ctx, "alice", "vide", provider.ListCommitsOptions{Branch: branch}); err != nil || len(c) != 0 || c == nil {
			t.Errorf("ListCommits(vide, %q) = %v, %v ; liste vide attendue", branch, c, err)
		}
	}
	if tags, err := p.Tags(ctx, "alice", "vide"); err != nil || len(tags) != 0 || tags == nil {
		t.Errorf("Tags(vide) = %v, %v", tags, err)
	}
}

func TestInvalidRefsRunNothing(t *testing.T) {
	// Un git introuvable prouve qu'aucune commande n'est lancée : seule la validation répond.
	p, err := git.New(git.Options{Dir: t.TempDir(), GitPath: filepath.Join(t.TempDir(), "absent")})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	sha := strings.Repeat("a", 40)
	calls := map[string]func() error{
		"owner": func() error { _, err := p.Repository(ctx, "-x", "projet"); return err },
		"name":  func() error { _, err := p.Branches(ctx, "alice", ".."); return err },
		"branche": func() error {
			_, err := p.ListCommits(ctx, "alice", "p", provider.ListCommitsOptions{Branch: "a\nb"})
			return err
		},
		"base":       func() error { _, err := p.CompareCommits(ctx, "alice", "p", "HEAD", sha); return err },
		"head":       func() error { _, err := p.CompareCommits(ctx, "alice", "p", sha, "--all"); return err },
		"stats":      func() error { _, err := p.CommitStats(ctx, "alice", "p", "main"); return err },
		"tags":       func() error { _, err := p.Tags(ctx, "../..", "p"); return err },
		"id":         func() error { _, err := p.RepositoryByID(ctx, 0); return err },
		"pulls":      func() error { _, err := p.PullRequests(ctx, "a/b", "p", provider.PullRequestsOptions{}); return err },
		"releases":   func() error { _, err := p.Releases(ctx, "alice", "a b"); return err },
		"clone path": func() error { _, err := git.ClonePath(t.TempDir(), "alice", "../x"); return err },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, provider.ErrInvalidRef) {
			t.Errorf("%s : %v, ErrInvalidRef attendue", name, err)
		}
	}
}

func TestAccessDeniedWithoutPrompt(t *testing.T) {
	srv := gittest.NewServer(t)
	p, dir, _ := newProvider(t, srv)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	// Dépôt absent du « serveur » : git échoue sans rien demander (aucune entrée standard).
	if _, err := p.Repository(ctx, "alice", "absent"); !errors.Is(err, provider.ErrAccessDenied) {
		t.Errorf("Repository(absent) : %v, ErrAccessDenied attendue", err)
	} else if !strings.Contains(err.Error(), "accès refusé") || strings.Contains(err.Error(), srv.URL) {
		t.Errorf("message : %q", err)
	}
	if _, err := p.Branches(ctx, "alice", "absent"); !errors.Is(err, provider.ErrAccessDenied) {
		t.Errorf("Branches(absent) : %v, ErrAccessDenied attendue", err)
	}
	clone, _ := git.ClonePath(dir, "alice", "absent")
	if _, err := os.Stat(clone); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("un clone raté a laissé %s", clone)
	}
	entries, _ := os.ReadDir(filepath.Dir(clone))
	if len(entries) != 0 {
		t.Errorf("restes de clone : %v", entries)
	}
}

func TestHTTPSWithoutCredentialsFailsFast(t *testing.T) {
	gittest.Skip(t)
	// Port fermé de la boucle locale : erreur réseau, sans invite ni service extérieur.
	p, err := git.New(git.Options{Dir: t.TempDir(), BaseURL: "https://127.0.0.1:1",
		NetworkTimeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Repository(t.Context(), "alice", "projet"); !errors.Is(err, provider.ErrUnavailable) {
		t.Errorf("hôte injoignable : %v, ErrUnavailable attendue", err)
	}
}

func TestConcurrentEnsure(t *testing.T) {
	srv := gittest.NewServer(t)
	repos := []string{"a", "b", "c", "d"}
	for i, name := range repos {
		r := srv.Create("alice", name, "main")
		r.Commit("main", "f.txt", strings.Repeat("x\n", i+1), "c", alice, t0)
	}
	p, _, _ := newProvider(t, srv)
	var wg gosync.WaitGroup
	errs := make(chan error, 4*len(repos))
	for range 4 {
		for _, name := range repos {
			wg.Go(func() {
				if _, err := p.ListCommits(t.Context(), "alice", name, provider.ListCommitsOptions{}); err != nil {
					errs <- err
				}
			})
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestRemoveClone(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("Alice", "Projet", "main")
	r.Commit("main", "f", "1\n", "c", alice, t0)
	r2 := srv.Create("Alice", "autre", "main")
	r2.Commit("main", "f", "1\n", "c", alice, t0)
	p, dir, _ := newProvider(t, srv)
	for _, name := range []string{"Projet", "autre"} {
		if _, err := p.Branches(t.Context(), "Alice", name); err != nil {
			t.Fatal(err)
		}
	}
	clone, _ := git.ClonePath(dir, "alice", "projet")
	if clone != filepath.Join(dir, "alice", "projet.git") {
		t.Errorf("ClonePath = %s", clone)
	}
	if err := git.RemoveClone(dir, "ALICE", "projet"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(clone); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("clone toujours là : %v", err)
	}
	if err := git.RemoveClone(dir, "alice", "autre"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "alice")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("répertoire du propriétaire vide gardé : %v", err)
	}
	if err := git.RemoveClone(dir, "alice", "absent"); err != nil {
		t.Errorf("clone absent : %v", err)
	}
}

func TestCheck(t *testing.T) {
	gittest.Skip(t)
	if err := git.Check(t.Context(), ""); err != nil {
		t.Errorf("Check(git) : %v", err)
	}
	if err := git.Check(t.Context(), filepath.Join(t.TempDir(), "absent")); !errors.Is(err, git.ErrGitNotFound) {
		t.Errorf("Check(absent) : %v, ErrGitNotFound attendue", err)
	}
}
