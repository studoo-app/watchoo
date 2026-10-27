package git_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

// CommitStatsBatch donne, en un passage, les mêmes statistiques que CommitStats : commit simple,
// racine, merge (premier parent), commit vide (0/0, présent dans le résultat), fichier binaire non
// compté ; un SHA absent du clone est omis, les doublons sont permis.
func TestCommitStatsBatch(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	m0 := r.Commit("main", "a.txt", "a\nb\n", "m0", alice, t0)
	r.Branch("dev", m0)
	f1 := r.Commit("dev", "b.txt", "x\n", "f1", bob, t0.Add(time.Hour))
	m1 := r.Commit("main", "a.txt", "a\nc\n", "m1", alice, t0.Add(2*time.Hour))
	x := r.Merge("main", "dev", "merge", alice, t0.Add(3*time.Hour))
	empty := r.EmptyCommit("main", "vide", alice, t0.Add(4*time.Hour))
	bin := r.Commit("main", "img.bin", "\x00\x01\x02", "binaire", alice, t0.Add(5*time.Hour))
	unknown := strings.Repeat("ab", 20)

	p, _, logs := newProvider(t, srv)
	ctx := t.Context()
	got, err := p.CommitStatsBatch(ctx, "alice", "projet", []string{m0, f1, m1, x, empty, bin, unknown, m1})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]provider.CommitStats{
		m0:    {Additions: 2},
		f1:    {Additions: 1},
		m1:    {Additions: 1, Deletions: 1},
		x:     {Additions: 1}, // le merge apporte b.txt à main
		empty: {},
		bin:   {},
	}
	if len(got) != len(want) {
		t.Errorf("CommitStatsBatch : %d résultats, attendu %d (%v)", len(got), len(want), got)
	}
	for sha, w := range want {
		st, ok := got[sha]
		if !ok || st != w {
			t.Errorf("CommitStatsBatch[%s] = %+v (%v), attendu %+v", provider.ShortSHA(sha), st, ok, w)
		}
		if one, err := p.CommitStats(ctx, "alice", "projet", sha); err != nil || one != w {
			t.Errorf("CommitStats(%s) = %+v, %v ; attendu %+v (même règle)", provider.ShortSHA(sha), one, err, w)
		}
	}
	if _, ok := got[unknown]; ok {
		t.Error("un SHA absent du clone figure dans le résultat")
	}
	if strings.Contains(logs.String(), "alice@example.test") || strings.Contains(logs.String(), "Alice Martin") {
		t.Errorf("donnée personnelle dans les logs :\n%s", logs.String())
	}

	// Liste vide : résultat vide, sans erreur ; SHA invalide refusé avant tout git.
	if none, err := p.CommitStatsBatch(ctx, "alice", "projet", nil); err != nil || len(none) != 0 {
		t.Errorf("liste vide : %v, %v", none, err)
	}
	if _, err := p.CommitStatsBatch(ctx, "alice", "projet", []string{"main"}); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("SHA invalide : %v, ErrInvalidRef attendue", err)
	}
}

// Plus de StatsBatchSize commits : plusieurs paquets, tous calculés ; contexte annulé : erreur.
func TestCommitStatsBatchManyCommits(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "gros", "main")
	r.Commit("main", "f.txt", "0\n", "c0", alice, t0)
	n := git.StatsBatchSize + 20
	r.ManyCommits("main", "f.txt", n, alice, t0.Add(time.Minute))
	p, dir, _ := newProvider(t, srv)
	commits, err := p.ListCommits(t.Context(), "alice", "gros", provider.ListCommitsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != n+1 {
		t.Fatalf("%d commits, attendu %d", len(commits), n+1)
	}
	got, err := p.CommitStatsBatch(t.Context(), "alice", "gros", shas(commits))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n+1 {
		t.Errorf("%d statistiques, attendu %d", len(got), n+1)
	}
	for _, c := range commits[:n] { // chaque commit remplace une ligne
		if st := got[c.SHA]; st != (provider.CommitStats{Additions: 1, Deletions: 1}) {
			t.Fatalf("commit %s : %+v", provider.ShortSHA(c.SHA), st)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := fresh(t, srv, dir).CommitStatsBatch(ctx, "alice", "gros", shas(commits)); !errors.Is(err, context.Canceled) {
		t.Errorf("contexte annulé : %v", err)
	}
}

// Prepare crée le clone et extrait la branche par défaut donnée (celle de l'API), sans ls-remote ;
// une seconde fois, rien n'est refait.
func TestPrepareUsesGivenDefaultBranch(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	m0 := r.Commit("main", "a.txt", "main\n", "m0", alice, t0)
	r.Branch("dev", m0)
	r.Commit("dev", "a.txt", "dev\n", "d1", alice, t0.Add(time.Hour))

	p, dir, logs := newProvider(t, srv)
	if err := p.Prepare(t.Context(), "alice", "projet", "dev"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "projet", "a.txt")); got != "dev\n" {
		t.Errorf("copie de travail = %q, branche dev (donnée) attendue", got)
	}
	if strings.Contains(logs.String(), "commande=ls-remote") {
		t.Error("ls-remote lancé alors que la branche par défaut était donnée")
	}
	if err := p.Prepare(t.Context(), "alice", "projet", "dev"); err != nil {
		t.Errorf("second Prepare : %v", err)
	}
	if err := p.Prepare(t.Context(), "alice", "projet", "a\x01b"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("branche invalide : %v", err)
	}
	// Dépôt absent : accès refusé, comme les autres commandes.
	if err := fresh(t, srv, dir).Prepare(t.Context(), "alice", "absent", "main"); !errors.Is(err, provider.ErrAccessDenied) {
		t.Errorf("dépôt absent : %v", err)
	}
}
