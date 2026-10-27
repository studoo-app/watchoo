package store

// Tests des vues de lecture : derniers commits, classement des contributeurs, résumé par dépôt.

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// viewFixture remplit deux dépôts :
//   - alice (label « Alice Martin ») : commits 1 et 2 sur main (Alice, compte 10), 3 sur main et dev
//     (Bob, sans compte), 4 sur dev seulement (Bob), 5 sans auteur sur main ;
//   - bob : commit 6 sur main (Alice), statistiques connues.
func viewFixture(t *testing.T, s *Store) (aliceRepo, bobRepo int64) {
	t.Helper()
	ctx := t.Context()
	aliceRepo = mustCreateRepo(t, s, 1, "promo", "alice")
	if _, err := s.UpdateRepositorySettings(ctx, aliceRepo, "Alice Martin", nil); err != nil {
		t.Fatal(err)
	}
	bobRepo = mustCreateRepo(t, s, 2, "promo", "bob")

	alice := person("Alice", "Alice@Example.test", "alice-gh", 10)
	bob := person("Bob", "bob@example.test", "", 0)
	anon := person("", "", "", 0)
	applySync(t, s, aliceRepo,
		[]BranchState{{Name: "main", HeadSHA: sha(5)}, {Name: "dev", HeadSHA: sha(4)}},
		cw(1, alice, "main"), cw(2, alice, "main"), cw(3, bob, "main", "dev"), cw(4, bob, "dev"), cw(5, anon, "main"))

	now := t0.Add(1000 * time.Hour)
	c6 := cw(6, alice, "main")
	c6.Additions, c6.Deletions = 7, 3
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{
		RepoID: bobRepo, RunID: beginRun(t, s, bobRepo, now), Metadata: meta("promo", "bob"),
		Branches: []BranchState{{Name: "main", HeadSHA: sha(6)}}, Commits: []CommitWrite{c6}, SyncedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return aliceRepo, bobRepo
}

// shasOf renvoie les SHA d'une liste de commits, dans l'ordre.
func shasOf(views []CommitView) []string {
	out := make([]string, 0, len(views))
	for _, v := range views {
		out = append(out, v.SHA)
	}
	return out
}

func TestListRecentCommitsFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, bobRepo := viewFixture(t, s)

	tests := []struct {
		name   string
		filter CommitFilter
		want   []string
	}{
		{"tous", CommitFilter{}, []string{sha(6), sha(5), sha(4), sha(3), sha(2), sha(1)}},
		{"limite", CommitFilter{Limit: 2}, []string{sha(6), sha(5)}},
		{"dépôt", CommitFilter{RepoID: bobRepo}, []string{sha(6)}},
		{"branche", CommitFilter{RepoID: aliceRepo, Branch: "dev"}, []string{sha(4), sha(3)}},
		{"branche main", CommitFilter{Branch: "main"}, []string{sha(6), sha(5), sha(3), sha(2), sha(1)}},
		{"branche inconnue", CommitFilter{Branch: "nope"}, []string{}},
		{"période", CommitFilter{Since: t0.Add(4 * time.Hour)}, []string{sha(6), sha(5), sha(4)}},
		{"auteur par login, casse ignorée", CommitFilter{Author: "ALICE-GH"}, []string{sha(6), sha(2), sha(1)}},
		{"auteur par nom", CommitFilter{Author: "bob"}, []string{sha(4), sha(3)}},
		{"auteur par e-mail", CommitFilter{Author: "alice@EXAMPLE.test", RepoID: aliceRepo}, []string{sha(2), sha(1)}},
		{"auteur inconnu", CommitFilter{Author: "zoé"}, []string{}},
		{"combiné", CommitFilter{Author: "Alice", Branch: "main", Since: t0.Add(2 * time.Hour), Limit: 1}, []string{sha(6)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ListRecentCommits(ctx, tt.filter)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("liste nil, attendu une liste non nil")
			}
			if !slices.Equal(shasOf(got), tt.want) {
				t.Errorf("SHA %v, attendu %v", shasOf(got), tt.want)
			}
		})
	}
}

func TestListRecentCommitsView(t *testing.T) {
	s := newTestStore(t)
	aliceRepo, _ := viewFixture(t, s)
	got, err := s.ListRecentCommits(t.Context(), CommitFilter{RepoID: aliceRepo})
	if err != nil {
		t.Fatal(err)
	}
	byS := map[string]CommitView{}
	for _, v := range got {
		byS[v.SHA] = v
	}
	c1 := byS[sha(1)]
	if c1.RepoOwner != "promo" || c1.RepoName != "alice" || c1.RepoDisplayName() != "Alice Martin" ||
		c1.AuthorName != "Alice" || c1.AuthorLogin != "alice-gh" || c1.Message != "message 1" ||
		!c1.CommittedAt.Equal(t0.Add(time.Hour)) || c1.AuthorID == 0 || c1.AuthorID != c1.CommitterID {
		t.Errorf("vue du commit 1 : %+v", c1)
	}
	c5 := byS[sha(5)]
	if c5.AuthorID != 0 || c5.AuthorDisplayName() != "inconnu" {
		t.Errorf("commit sans auteur : %+v", c5)
	}
}

func TestContributorRanking(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, _ := viewFixture(t, s)

	got, err := s.ContributorRanking(ctx, ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d contributeurs classés, attendu 2 (commit sans auteur ignoré) : %+v", len(got), got)
	}
	alice, bob := got[0], got[1]
	if alice.Name != "Alice" || alice.ProviderLogin != "alice-gh" || alice.Provider != model.ProviderGitHub ||
		alice.ProviderUserID != 10 || alice.Commits != 3 || alice.Repos != 2 || alice.Additions != 7 ||
		alice.Deletions != 3 || !alice.LastCommitAt.Equal(t0.Add(6*time.Hour)) || alice.Emails != nil {
		t.Errorf("Alice : %+v", alice)
	}
	if bob.Name != "Bob" || bob.Commits != 2 || bob.Repos != 1 || bob.Provider != "" ||
		!bob.LastCommitAt.Equal(t0.Add(4*time.Hour)) {
		t.Errorf("Bob : %+v", bob)
	}

	// Égalité de commits : le dernier commit le plus récent passe devant.
	got, err = s.ContributorRanking(ctx, ContributorFilter{RepoID: aliceRepo})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Bob" || got[0].Commits != 2 || got[1].Name != "Alice" || got[1].Commits != 2 {
		t.Errorf("classement du dépôt alice : %+v", got)
	}

	got, err = s.ContributorRanking(ctx, ContributorFilter{Since: t0.Add(5 * time.Hour)})
	if err != nil || len(got) != 1 || got[0].Name != "Alice" || got[0].Commits != 1 {
		t.Errorf("depuis t0+5h : %+v, %v", got, err)
	}
	got, err = s.ContributorRanking(ctx, ContributorFilter{Limit: 1})
	if err != nil || len(got) != 1 {
		t.Errorf("limite 1 : %+v, %v", got, err)
	}
	got, err = s.ContributorRanking(ctx, ContributorFilter{Since: t0.Add(9999 * time.Hour)})
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("aucun commit : %+v, %v ; attendu une liste vide non nil", got, err)
	}
}

func TestRepositorySummaries(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, bobRepo := viewFixture(t, s)
	never := mustCreateRepo(t, s, 3, "aaa", "jamais")

	// Une synchro en erreur après la synchro réussie de bob.
	run := beginRun(t, s, bobRepo, t0.Add(2000*time.Hour))
	if err := s.FinishSyncRun(ctx, run, t0.Add(2000*time.Hour), model.SyncError, 0, "échec factice"); err != nil {
		t.Fatal(err)
	}

	got, err := s.RepositorySummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != never || got[1].ID != aliceRepo || got[2].ID != bobRepo {
		t.Fatalf("résumés %+v, attendu triés par owner puis name", got)
	}

	n := got[0]
	if n.Commits != 0 || n.TrackedBranches != 0 || n.LastCommit.SHA != "" || n.LastRun.ID != 0 {
		t.Errorf("dépôt jamais synchronisé : %+v", n)
	}

	a := got[1]
	if a.Label != "Alice Martin" || a.Commits != 5 || a.TrackedBranches != 2 {
		t.Errorf("résumé alice : %+v", a)
	}
	// Commit le plus récent sur les branches suivies : 5 (sans auteur).
	if a.LastCommit.SHA != sha(5) || a.LastCommit.RepoDisplayName() != "Alice Martin" || a.LastCommit.AuthorDisplayName() != "inconnu" {
		t.Errorf("dernier commit d'alice : %+v", a.LastCommit)
	}
	if a.LastRun.Status != model.SyncSuccess || a.LastRun.NewCommits != 5 {
		t.Errorf("dernière synchro d'alice : %+v", a.LastRun)
	}

	b := got[2]
	if b.LastCommit.SHA != sha(6) || b.LastCommit.AuthorName != "Alice" || b.LastCommit.AuthorLogin != "alice-gh" {
		t.Errorf("dernier commit de bob : %+v", b.LastCommit)
	}
	if b.LastRun.ID != run || b.LastRun.Status != model.SyncError || b.LastRun.Error != "échec factice" {
		t.Errorf("dernière synchro de bob : %+v", b.LastRun)
	}

	// Le dernier commit ne compte que les branches suivies : sans dev, le commit 4 n'est plus candidat
	// (ici le 5 reste le plus récent) ; sans main, il n'y a plus que dev.
	applySync(t, s, aliceRepo, []BranchState{{Name: "dev", HeadSHA: sha(4)}})
	one, err := s.RepositorySummary(ctx, aliceRepo)
	if err != nil {
		t.Fatal(err)
	}
	if one.TrackedBranches != 1 || one.LastCommit.SHA != sha(4) || !one.LastCommitAt.Equal(t0.Add(4*time.Hour)) {
		t.Errorf("résumé d'alice suivant dev seule : %+v", one)
	}
	if _, err := s.RepositorySummary(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("dépôt inexistant : %v, attendu ErrNotFound", err)
	}
}

func TestRepositorySummariesEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.RepositorySummaries(t.Context())
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("RepositorySummaries = %v, %v ; attendu une liste vide non nil", got, err)
	}
}

func TestRepositorySummaryTieBreakBySHA(t *testing.T) {
	s := newTestStore(t)
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	who := person("Nina", "nina@example.test", "", 0)
	a, b := cw(1, who, "main"), cw(2, who, "main")
	b.CommittedAt = a.CommittedAt
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(2)}}, b, a)
	got, err := s.RepositorySummary(t.Context(), repoID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastCommit.SHA != sha(1) {
		t.Errorf("dernier commit %s, attendu le SHA le plus petit en cas d'égalité", got.LastCommit.SHA)
	}
}
