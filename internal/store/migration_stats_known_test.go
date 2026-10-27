package store

// Migration 00006 (0.5.0) : commits.stats_known, et son usage (écriture, pagination, totaux).

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestMigrationStatsKnown(t *testing.T) {
	ctx := t.Context()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	p, err := goose.NewProvider(MigrationDialect, s.DB(), migrations.FS, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 5); err != nil {
		t.Fatalf("UpTo(5) : %v", err)
	}
	// Base au schéma 00005 : un commit lu par l'API (lignes non nulles), deux à 0/0 (jamais lus, ou
	// réellement vides : indiscernables).
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	for i, st := range [][2]int{{4, 1}, {0, 0}, {0, 3}, {0, 0}} {
		mustExec(t, s, `INSERT INTO commits (repo_id, sha, authored_at, committed_at, additions, deletions)
			VALUES (?, ?, ?, ?, ?, ?)`, repo, sha(i+1), testDate, testDate, st[0], st[1])
	}

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate : %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM commits WHERE stats_known = 1"); got != 2 {
		t.Errorf("%d commits connus après la migration, attendu 2 (lignes non nulles)", got)
	}
	unknown, err := s.UnknownStatsSHAs(ctx, repo, "", 10)
	if err != nil || !slices.Equal(unknown, []string{sha(2), sha(4)}) {
		t.Errorf("UnknownStatsSHAs = %v, %v", unknown, err)
	}
	if got := count(t, s, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'commits_stats_unknown'`); got != 1 {
		t.Error("index commits_stats_unknown absent")
	}
	if err := execErr(t, s, `UPDATE commits SET stats_known = 2`); err == nil {
		t.Error("stats_known = 2 accepté")
	}

	// Retour arrière : colonne et index retirés, lignes et valeurs gardées.
	if _, err := p.DownTo(ctx, 5); err != nil {
		t.Fatalf("DownTo(5) : %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM commits WHERE additions = 4 AND deletions = 1"); got != 1 {
		t.Error("valeurs perdues au retour arrière")
	}
	if got := count(t, s, "SELECT count(*) FROM commits"); got != 4 {
		t.Errorf("%d commits après le retour arrière, attendu 4", got)
	}
	if err := execErr(t, s, "SELECT stats_known FROM commits"); err == nil {
		t.Error("colonne stats_known encore présente")
	}
	if got := count(t, s, `SELECT count(*) FROM sqlite_master WHERE name = 'commits_stats_unknown'`); got != 0 {
		t.Error("index encore présent")
	}
	if _, err := p.UpTo(ctx, 6); err != nil {
		t.Fatalf("UpTo(6) de nouveau : %v", err)
	}
}

func TestApplyRepoSyncStats(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "alice", 501)
	main := []BranchState{{Name: "main", HeadSHA: sha(3)}}

	// Nouveau commit connu (même à 0/0), nouveau commit inconnu.
	c1, c2, c3 := cw(1, alice, "main"), cw(2, alice, "main"), cw(3, alice, "main")
	c1.Additions, c1.Deletions, c1.StatsKnown = 5, 1, true
	c2.StatsKnown = true // commit vide : 0/0 connu
	applySync(t, s, repoID, main, c1, c2, c3)
	unknown, err := s.UnknownStatsSHAs(ctx, repoID, "", 10)
	if err != nil || !slices.Equal(unknown, []string{sha(3)}) {
		t.Fatalf("UnknownStatsSHAs = %v, %v", unknown, err)
	}
	views, err := s.ListRecentCommits(ctx, CommitFilter{RepoID: repoID})
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, v := range views {
		known[v.SHA] = v.StatsKnown
	}
	if !known[sha(1)] || !known[sha(2)] || known[sha(3)] {
		t.Errorf("stats_known lus : %v", known)
	}

	// Statistiques d'un commit déjà en base (et d'un SHA inconnu, ignoré), dans la même transaction.
	now := t0.Add(2000 * time.Hour)
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{
		RepoID: repoID, RunID: beginRun(t, s, repoID, now), Metadata: meta("promo", "alice"), Branches: main,
		Stats:    []CommitStatsWrite{{SHA: sha(3), Additions: 7, Deletions: 2}, {SHA: sha(99), Additions: 1}},
		SyncedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if unknown, _ := s.UnknownStatsSHAs(ctx, repoID, "", 10); len(unknown) != 0 {
		t.Errorf("encore inconnus : %v", unknown)
	}
	sum, err := s.RepositorySummary(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if want := (model.LineTotals{Commits: 3, KnownCommits: 3, Additions: 12, Deletions: 3}); sum.Lines != want {
		t.Errorf("totaux du dépôt = %+v, attendu %+v", sum.Lines, want)
	}

	// Statistiques invalides : refusées avant toute écriture.
	for _, bad := range []CommitStatsWrite{{SHA: ""}, {SHA: sha(1), Additions: -1}} {
		if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{
			RepoID: repoID, RunID: beginRun(t, s, repoID, now.Add(time.Hour)), Metadata: meta("promo", "alice"),
			Branches: main, Stats: []CommitStatsWrite{bad}, SyncedAt: now.Add(time.Hour),
		}); err == nil {
			t.Errorf("statistiques %+v acceptées", bad)
		}
		mustExec(t, s, `UPDATE sync_runs SET status = 'error', finished_at = ? WHERE status = 'running'`, testDate)
	}
	if _, err := s.UnknownStatsSHAs(ctx, repoID, "", 0); err == nil {
		t.Error("limite 0 acceptée")
	}
}

// Pagination des commits inconnus et totaux partiels (contributeurs, dépôt).
func TestUnknownStatsPagingAndTotals(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "alice", 501)
	var commits []CommitWrite
	for i := 1; i <= 5; i++ {
		c := cw(i, alice, "main")
		if i <= 2 {
			c.Additions, c.StatsKnown = 10, true
		}
		commits = append(commits, c)
	}
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(5)}}, commits...)

	var all []string
	after := ""
	for {
		page, err := s.UnknownStatsSHAs(ctx, repoID, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page...)
		if len(page) < 2 {
			break
		}
		after = page[len(page)-1]
	}
	if !slices.Equal(all, []string{sha(3), sha(4), sha(5)}) {
		t.Errorf("pages = %v", all)
	}

	ranking, err := s.ContributorRanking(ctx, ContributorFilter{})
	if err != nil || len(ranking) != 1 {
		t.Fatalf("classement : %v, %v", ranking, err)
	}
	if l := ranking[0].Lines(); !l.Known() || l.Uncounted() != 3 || l.Additions != 20 {
		t.Errorf("totaux du contributeur = %+v", l)
	}
	profile, err := s.ContributorProfile(ctx, ranking[0].ID)
	if err != nil || profile.Lines().Uncounted() != 3 {
		t.Errorf("fiche : %+v, %v", profile.Lines(), err)
	}
	repos, err := s.ContributorRepos(ctx, ranking[0].ID)
	if err != nil || len(repos) != 1 || repos[0].Lines().KnownCommits != 2 {
		t.Errorf("par dépôt : %+v, %v", repos, err)
	}
	sums, err := s.RepositorySummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if total := SumLines(sums); total.Commits != 5 || total.KnownCommits != 2 || total.Additions != 20 {
		t.Errorf("totaux de l'espace = %+v", total)
	}
	if (model.LineTotals{Commits: 2}).Known() {
		t.Error("totaux sans commit connu : Known vrai")
	}
}
