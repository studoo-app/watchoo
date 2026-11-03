package store

// Migration 00009 (1.2.0): commits.languages_version and commit_languages, their writing by ApplyRepoSync
// and the languages of the contributors.

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestMigrationCommitLanguages(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 8); err != nil {
		t.Fatalf("UpTo(8): %v", err)
	}
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	mustExec(t, s, `INSERT INTO commits (repo_id, sha, message, authored_at, committed_at, additions, stats_known)
		VALUES (?, ?, 'x', ?, ?, 3, 1)`, repo, sha(1), testDate, testDate)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// A commit from before the migration, with known lines, waits for its languages only.
	if got, err := s.UnknownStatsSHAs(ctx, repo, 1, "", 10); err != nil || !reflect.DeepEqual(got, []string{sha(1)}) {
		t.Errorf("UnknownStatsSHAs(version 1) = %v, %v", got, err)
	}
	if got, err := s.UnknownStatsSHAs(ctx, repo, 0, "", 10); err != nil || len(got) != 0 {
		t.Errorf("UnknownStatsSHAs(version 0) = %v, %v", got, err)
	}
	for _, q := range []string{
		`UPDATE commits SET languages_version = -1`,
		`INSERT INTO commit_languages (repo_id, sha, language, additions) VALUES (1, 'absent', 'Go', 1)`,
		`INSERT INTO commit_languages (repo_id, sha, language, additions) VALUES (1, '` + sha(1) + `', 'Go', 0)`,
		`INSERT INTO commit_languages (repo_id, sha, language, additions) VALUES (1, '` + sha(1) + `', '', 1)`,
	} {
		if err := execErr(t, s, q); err == nil {
			t.Errorf("accepted: %s", q)
		}
	}

	// Rollback: table, index and column removed, the commit kept.
	if _, err := p.DownTo(ctx, 8); err != nil {
		t.Fatalf("DownTo(8): %v", err)
	}
	if n := count(t, s, "SELECT count(*) FROM commits"); n != 1 {
		t.Errorf("%d commits after the rollback", n)
	}
	if n := count(t, s, `SELECT count(*) FROM sqlite_master WHERE name IN ('commit_languages', 'commits_languages_version')`); n != 0 {
		t.Errorf("%d objects left after the rollback", n)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate after the rollback: %v", err)
	}
}

// commitLanguages returns the languages recorded for the commit n of repoID.
func commitLanguages(t *testing.T, s *Store, repoID int64, n int) map[string]int {
	t.Helper()
	rows, err := s.DB().QueryContext(t.Context(), `SELECT language, additions FROM commit_languages WHERE repo_id = ? AND sha = ?`,
		repoID, sha(n))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var lang string
		var add int
		if err := rows.Scan(&lang, &add); err != nil {
			t.Fatal(err)
		}
		out[lang] = add
	}
	return out
}

func TestApplyRepoSyncCommitLanguages(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "", 0)
	main := []BranchState{{Name: "main", HeadSHA: sha(3)}}

	// New commits: languages written with the version; nil = unknown, empty = analyzed with no line.
	c1, c2, c3 := cw(1, alice, "main"), cw(2, alice, "main"), cw(3, alice, "main")
	c1.Additions, c1.StatsKnown, c1.Languages = 5, true, map[string]int{"Go": 4, "HTML": 1, "Zero": 0}
	c2.StatsKnown, c2.Languages = true, map[string]int{}
	now := t0.Add(1000 * time.Hour)
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{RepoID: repo, RunID: beginRun(t, s, repo, now), Metadata: meta("promo", "alice"),
		Branches: main, Commits: []CommitWrite{c1, c2, c3}, LanguagesVersion: 1, SyncedAt: now}); err != nil {
		t.Fatal(err)
	}
	if got := commitLanguages(t, s, repo, 1); !reflect.DeepEqual(got, map[string]int{"Go": 4, "HTML": 1}) {
		t.Errorf("languages of c1 = %v", got)
	}
	if n := count(t, s, `SELECT count(*) FROM commits WHERE languages_version = 1`); n != 2 {
		t.Errorf("%d commits analyzed, want 2 (c1, c2)", n)
	}
	if got, err := s.UnknownStatsSHAs(ctx, repo, 1, "", 10); err != nil || !reflect.DeepEqual(got, []string{sha(3)}) {
		t.Errorf("pending = %v, %v", got, err)
	}

	// Catch-up through Stats: c3 gets lines and languages; c1, already known, keeps its lines and gets
	// its languages rewritten (new rules version); an unknown SHA is ignored.
	now = now.Add(time.Hour)
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{RepoID: repo, RunID: beginRun(t, s, repo, now), Metadata: meta("promo", "alice"),
		Branches: main, Stats: []CommitStatsWrite{
			{SHA: sha(3), Additions: 7, Languages: map[string]int{"PHP": 7}},
			{SHA: sha(1), Additions: 99, Languages: map[string]int{"Go": 5}},
			{SHA: sha(9), Additions: 1, Languages: map[string]int{"Go": 1}},
		}, LanguagesVersion: 2, SyncedAt: now}); err != nil {
		t.Fatal(err)
	}
	if got := commitLanguages(t, s, repo, 1); !reflect.DeepEqual(got, map[string]int{"Go": 5}) {
		t.Errorf("languages of c1 after the catch-up = %v", got)
	}
	if n := count(t, s, `SELECT additions FROM commits WHERE sha = ?`, sha(1)); n != 5 {
		t.Errorf("known lines of c1 recomputed: +%d", n)
	}
	if n := count(t, s, `SELECT additions FROM commits WHERE sha = ?`, sha(3)); n != 7 {
		t.Errorf("lines of c3: +%d", n)
	}
	if n := count(t, s, `SELECT count(*) FROM commit_languages WHERE sha = ?`, sha(9)); n != 0 {
		t.Error("languages of an unknown commit written")
	}

	// Languages without a version: refused before any writing.
	c4 := cw(4, alice, "main")
	c4.Languages = map[string]int{"Go": 1}
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{RepoID: repo, RunID: beginRun(t, s, repo, now.Add(time.Hour)),
		Metadata: meta("promo", "alice"), Branches: main, Commits: []CommitWrite{c4}, SyncedAt: now}); err == nil {
		t.Error("languages without a version accepted")
	}

	// Cascade: the languages disappear with the repository.
	if err := s.DeleteRepository(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, `SELECT count(*) FROM commit_languages`); n != 0 {
		t.Errorf("%d languages left after the deletion", n)
	}
}

func TestContributorLanguages(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoA := mustCreateRepo(t, s, 1, "promo", "a")
	repoB := mustCreateRepo(t, s, 2, "promo", "b")
	alice := mustExec(t, s, `INSERT INTO contributors (name) VALUES ('Alice')`)
	bob := mustExec(t, s, `INSERT INTO contributors (name) VALUES ('Bob')`)
	old, recent := "2026-01-01T10:00:00Z", "2026-03-01T10:00:00Z"
	for i, c := range []struct {
		repo, author int64
		at           string
		version      int
		langs        map[string]int
	}{
		{repoA, alice, old, 1, map[string]int{"PHP": 30, "Twig": 5}},
		{repoA, alice, recent, 1, map[string]int{"Twig": 10}},
		{repoB, alice, recent, 1, map[string]int{"Go": 40}},
		{repoB, alice, recent, 0, nil}, // not analyzed yet
		{repoA, bob, recent, 1, map[string]int{}},
	} {
		mustExec(t, s, `INSERT INTO commits (repo_id, sha, author_id, message, authored_at, committed_at, languages_version)
			VALUES (?, ?, ?, 'msg', ?, ?, ?)`, c.repo, sha(i+1), c.author, c.at, c.at, c.version)
		for lang, n := range c.langs {
			mustExec(t, s, `INSERT INTO commit_languages (repo_id, sha, language, additions) VALUES (?, ?, ?, ?)`,
				c.repo, sha(i+1), lang, n)
		}
	}

	ranking, err := s.ContributorRanking(ctx, ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]model.LanguageTotals{}
	for _, c := range ranking {
		byID[c.ID] = c.Languages
	}
	want := model.LanguageTotals{Commits: 4, Scanned: 3, Lines: []model.LanguageLines{
		{Language: "Go", Lines: 40}, {Language: "PHP", Lines: 30}, {Language: "Twig", Lines: 15}}}
	if !reflect.DeepEqual(byID[alice], want) {
		t.Errorf("languages of Alice = %+v, want %+v", byID[alice], want)
	}
	if got := byID[alice]; got.Main() != "Go" || got.Total() != 85 || got.Pending() != 1 || got.Share(got.Lines[0]) != 471 {
		t.Errorf("Main %q, Total %d, Pending %d, Share %d", got.Main(), got.Total(), got.Pending(), got.Share(got.Lines[0]))
	}
	if got := byID[bob]; !got.Known() || len(got.Lines) != 0 || got.Lines == nil || got.Main() != "" {
		t.Errorf("languages of Bob = %+v", got)
	}

	// Filters of the ranking: repository, then period.
	ranking, err = s.ContributorRanking(ctx, ContributorFilter{RepoID: repoA, Since: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range ranking {
		if c.ID == alice && !reflect.DeepEqual(c.Languages.Lines, []model.LanguageLines{{Language: "Twig", Lines: 10}}) {
			t.Errorf("filtered languages of Alice = %+v", c.Languages)
		}
	}

	// Page of the contributor: totals and main language per repository.
	p, err := s.ContributorProfile(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Languages, want) {
		t.Errorf("profile languages = %+v", p.Languages)
	}
	repos, err := s.ContributorRepos(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	main := map[int64]string{}
	for _, r := range repos {
		main[r.RepoID] = r.MainLanguage
	}
	if main[repoA] != "PHP" || main[repoB] != "Go" {
		t.Errorf("main languages = %v", main)
	}

	// Merge of contributors: the languages follow the commits.
	mustExec(t, s, `UPDATE commits SET author_id = ? WHERE author_id = ?`, alice, bob)
	if p, err = s.ContributorProfile(ctx, alice); err != nil || p.Languages.Scanned != 4 {
		t.Errorf("after the merge: %+v, %v", p.Languages, err)
	}
}
