package tui

// Détail d'un contributeur (0.5.0).

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// withContributorDetail ajoute à data la fiche d'Alice (contributeur 1) : deux dépôts, des commits
// avec et sans lignes connues, de l'activité cette semaine et il y a deux semaines.
func withContributorDetail(data *fakeData) {
	data.profiles = map[int64]store.ContributorProfile{
		1: {Contributor: model.Contributor{ID: 1, Name: "Alice", ProviderLogin: "alice-gh", ProviderUserID: 10,
			ProfileURL: "https://github.com/alice-gh"}, Identities: 2, Commits: 5, Additions: 40, Deletions: 7, KnownCommits: 3, Repos: 2,
			FirstCommitAt: testNow.Add(-30 * 24 * time.Hour), LastCommitAt: testNow.Add(-2 * time.Hour)},
		2: {Contributor: model.Contributor{ID: 2, Name: "Bob"}, Identities: 1},
	}
	data.contribRepos = map[int64][]store.ContributorRepoStats{
		1: {{ContributorID: 1, RepoID: 1, RepoOwner: "promo", RepoName: "alice", RepoLabel: "Alice Martin", Commits: 4,
			Additions: 40, Deletions: 7, KnownCommits: 4, LastCommitAt: testNow.Add(-2 * time.Hour)},
			{ContributorID: 1, RepoID: 3, RepoOwner: "promo", RepoName: "chloe", Commits: 1,
				LastCommitAt: testNow.Add(-42 * 24 * time.Hour)}},
	}
	data.repoTimes = map[int64][]model.RepoCommitTime{
		1: {{RepoID: 1, CommittedAt: testNow.Add(-2 * time.Hour)}, {RepoID: 1, CommittedAt: testNow.Add(-3 * time.Hour)},
			{RepoID: 1, CommittedAt: testNow.Add(-14 * 24 * time.Hour)}},
	}
	data.commits[0].AuthorID = 1
	data.commits[0].Additions, data.commits[0].Deletions, data.commits[0].StatsKnown = 12, 3, true
}

func TestContributorDetail(t *testing.T) {
	data := newFakeData()
	withContributorDetail(data)
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	l.press("3", "enter")
	l.until(func(m Model) bool { return m.cdetail.open && !m.cdetail.loading })
	screen := l.screen()
	mustContain(t, screen, "Alice (@alice-gh)", "échap pour revenir", "@alice-gh",
		"2 adresses rattachées (non affichées)", "5 commits dans 2 dépôts, +40/-7 (2 non comptés)", "30 d ago", "2 h ago",
		"12 semaines", "3 commits", "Dépôts", "Alice Martin", "+40/-7", "promo/chloe",
		"Derniers commits", "Ajoute la page d'accueil", "+12/-3", "Lignes +/- : comptées dans le clone")
	mustNotContain(t, screen, "rouge", "https://github.com/alice-gh", "@example")
	// Courbe d'activité de tous ses dépôts : 2 commits cette semaine, 1 il y a deux semaines.
	if line := lineWith(screen, "▁▁▁▁▁▁▁▁▁▅▁█"); !strings.Contains(line, "12 semaines") {
		t.Errorf("courbe globale = %q", line)
	}
	// Ligne du dépôt Dead : état du dépôt, aucune activité sur la période.
	if line := lineWith(screen, "promo/chloe"); !strings.Contains(line, "⚠ 42 d") || !strings.Contains(line, " - ") {
		t.Errorf("ligne de promo/chloe = %q", line)
	}
	data.mu.Lock()
	last := data.commitFilters[len(data.commitFilters)-1]
	data.mu.Unlock()
	if last.AuthorID != 1 || last.Limit != contribDetailCommits {
		t.Errorf("filtre des commits = %+v", last)
	}

	// Défilement, filtre refusé, relecture, puis retour à la liste.
	l.press("j", "/")
	if l.m.filtering {
		t.Error("le filtre s'est ouvert dans le détail")
	}
	l.press("r")
	l.until(func(m Model) bool { return !m.cdetail.loading })
	l.press("esc")
	if l.m.cdetail.open {
		t.Error("échap n'a pas fermé le détail du contributeur")
	}
	mustContain(t, l.screen(), "CONTRIBUTEUR", "LOGIN")

	// Contributeur sans compte ni commit d'auteur.
	l.press("j", "enter")
	l.until(func(m Model) bool { return m.cdetail.open && !m.cdetail.loading })
	mustContain(t, l.screen(), "Bob", "aucun compte GitHub connu", "Aucun commit dont il ou elle est l'auteur.")
	// Changer de vue ferme le détail.
	l.press("1")
	if l.m.cdetail.open {
		t.Error("changer de vue n'a pas fermé le détail")
	}
}

func TestContributorDetailError(t *testing.T) {
	data := newFakeData()
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	data.mu.Lock()
	data.err = errors.New("base illisible")
	data.mu.Unlock()
	l.press("3", "enter")
	l.until(func(m Model) bool { return m.cdetail.open && !m.cdetail.loading })
	mustContain(t, l.screen(), "lecture impossible : base illisible")
}

func TestContributorDetailWithRealStore(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	repo, err := st.CreateRepository(ctx, model.Repository{Provider: model.ProviderGitHub, ProviderID: 42,
		Owner: "promo", Name: "zoe", Label: "Zoé Leroy", URL: "https://github.com/promo/zoe", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.BeginSyncRun(ctx, repo.ID, testNow.Add(-time.Minute), wsync.LockStaleAfter)
	if err != nil {
		t.Fatal(err)
	}
	zoe := store.Identity{Name: "Zoé", Email: "zoe@example.test", Login: "zoe-gh", ProviderUserID: 7,
		ProfileURL: "https://github.com/zoe-gh"}
	sha := "cafe000000000000000000000000000000000001"
	if _, err := st.ApplyRepoSync(ctx, store.RepoSyncWrite{
		RepoID: repo.ID, RunID: run.ID,
		Metadata: store.RepositoryMetadata{Owner: "promo", Name: "zoe", DefaultBranch: "main",
			URL: "https://github.com/promo/zoe"},
		Branches: []store.BranchState{{Name: "main", HeadSHA: sha}},
		Commits: []store.CommitWrite{{SHA: sha, Message: "Premier rendu", Author: zoe, Committer: zoe,
			AuthoredAt: testNow.Add(-3 * time.Hour), CommittedAt: testNow.Add(-3 * time.Hour),
			Additions: 9, Deletions: 1, StatsKnown: true, Branches: []string{"main"}}},
		SyncedAt: testNow,
	}); err != nil {
		t.Fatal(err)
	}
	l := newLoop(t, testDeps(st, &fakeSyncer{}))
	l.press("3", "enter")
	l.until(func(m Model) bool { return m.cdetail.open && !m.cdetail.loading })
	screen := l.screen()
	mustContain(t, screen, "Zoé (@zoe-gh)", "1 adresse rattachée", "1 commit dans 1 dépôt, +9/-1",
		"Zoé Leroy", "Premier rendu", "cafe000")
	mustNotContain(t, screen, "zoe@example.test", "https://github.com/zoe-gh")
	if line := lineWith(screen, "Zoé Leroy"); !strings.Contains(line, "▁▁▁▁▁▁▁▁▁▁▁█") {
		t.Errorf("ligne du dépôt = %q", line)
	}
}

// Lignes d'un commit (« - » si inconnues, « +0/-0 » si nulles) et totaux (« (N non comptés) »).
func TestLineFormats(t *testing.T) {
	for _, tc := range []struct {
		got, want string
	}{
		{commitLines(model.Commit{}), "-"},
		{commitLines(model.Commit{StatsKnown: true}), "+0/-0"},
		{commitLines(model.Commit{StatsKnown: true, Additions: 4, Deletions: 1}), "+4/-1"},
		{lineTotals(model.LineTotals{Commits: 3}), "-"},
		{lineTotals(model.LineTotals{Commits: 3, KnownCommits: 3, Additions: 5}), "+5/-0"},
		{lineTotals(model.LineTotals{Commits: 3, KnownCommits: 1, Additions: 5, Deletions: 2}), "+5/-2 (2 non comptés)"},
	} {
		if tc.got != tc.want {
			t.Errorf("%q, attendu %q", tc.got, tc.want)
		}
	}
}
