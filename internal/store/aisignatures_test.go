package store

// Migration 00008 (1.2.0): commits.ai_rules_version and commit_ai_signatures, and their use.

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestMigrationAISignatures(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 7); err != nil {
		t.Fatalf("UpTo(7): %v", err)
	}
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	mustExec(t, s, `INSERT INTO commits (repo_id, sha, message, authored_at, committed_at) VALUES (?, ?, 'x', ?, ?)`,
		repo, sha(1), testDate, testDate)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// A commit from before the migration waits for its analysis.
	got, err := s.CommitsToScanForAI(ctx, repo, 1, 10)
	if err != nil || len(got) != 1 || got[0].SHA != sha(1) {
		t.Errorf("CommitsToScanForAI = %v, %v", got, err)
	}
	if err := execErr(t, s, `UPDATE commits SET ai_rules_version = -1`); err == nil {
		t.Error("negative ai_rules_version accepted")
	}
	if err := execErr(t, s, `INSERT INTO commit_ai_signatures (repo_id, sha, tool, kind) VALUES (?, 'absent', 'claude', 'footer')`, repo); err == nil {
		t.Error("signature of a missing commit accepted")
	}

	// Rollback: table, index and column removed, the commit kept.
	if _, err := p.DownTo(ctx, 7); err != nil {
		t.Fatalf("DownTo(7): %v", err)
	}
	if n := count(t, s, "SELECT count(*) FROM commits"); n != 1 {
		t.Errorf("%d commits after the rollback", n)
	}
	if n := count(t, s, `SELECT count(*) FROM sqlite_master WHERE name IN ('commit_ai_signatures', 'commits_ai_rules_version')`); n != 0 {
		t.Errorf("%d objects left after the rollback", n)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate after the rollback: %v", err)
	}
}

func TestAIScansAndViews(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repo := mustCreateRepo(t, s, 1, "promo", "a")
	other := mustCreateRepo(t, s, 2, "promo", "b")
	alice := mustExec(t, s, `INSERT INTO contributors (provider_login, name) VALUES ('alice-gh', 'Alice')`)
	mustExec(t, s, `INSERT INTO contributor_emails (email, contributor_id) VALUES ('alice@example.test', ?), ('alice@perso.example.test', ?)`, alice, alice)
	for i, r := range []int64{repo, repo, repo, other} {
		mustExec(t, s, `INSERT INTO commits (repo_id, sha, author_id, message, authored_at, committed_at) VALUES (?, ?, ?, 'msg', ?, ?)`,
			r, sha(i+1), alice, testDate, testDate)
	}

	todo, err := s.CommitsToScanForAI(ctx, repo, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(todo) != 2 || todo[0].SHA != sha(1) || todo[0].AuthorLogin != "alice-gh" || len(todo[0].AuthorEmails) != 2 {
		t.Fatalf("CommitsToScanForAI = %+v", todo)
	}

	// sha(1): two tools, one of them with two kinds; sha(2): none; sha(3): not analyzed.
	if err := s.WriteAIScans(ctx, repo, 1, []AIScanResult{
		{SHA: sha(1), Signatures: []model.AISignature{{Tool: "claude", Kind: "coauthor_trailer"}, {Tool: "claude", Kind: "footer"}, {Tool: "cursor", Kind: "coauthor_trailer"}}},
		{SHA: sha(2), Signatures: []model.AISignature{}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteAIScans(ctx, other, 1, []AIScanResult{{SHA: sha(4), Signatures: []model.AISignature{{Tool: "claude", Kind: "footer"}}}}); err != nil {
		t.Fatal(err)
	}
	if todo, err = s.CommitsToScanForAI(ctx, repo, 1, 10); err != nil || len(todo) != 1 || todo[0].SHA != sha(3) {
		t.Errorf("after the scan: %+v, %v", todo, err)
	}
	// A new version of the rules analyzes everything again.
	if todo, err = s.CommitsToScanForAI(ctx, repo, 2, 10); err != nil || len(todo) != 3 {
		t.Errorf("new rules version: %+v, %v", todo, err)
	}

	views, err := s.ListRecentCommits(ctx, CommitFilter{RepoID: repo})
	if err != nil {
		t.Fatal(err)
	}
	bySHA := map[string]CommitView{}
	for _, v := range views {
		bySHA[v.SHA] = v
	}
	first := bySHA[sha(1)]
	if !first.AIScanned || len(first.AISignatures) != 3 || !reflect.DeepEqual(first.AITools(), []string{"claude", "cursor"}) {
		t.Errorf("signed commit: %+v", first)
	}
	if c := bySHA[sha(2)]; !c.AIScanned || c.AISignatures == nil || len(c.AISignatures) != 0 {
		t.Errorf("scanned commit without a signature: %+v", c)
	}
	if c := bySHA[sha(3)]; c.AIScanned || len(c.AISignatures) != 0 {
		t.Errorf("commit not analyzed: %+v", c)
	}

	// Totals: a commit with several tools counts once in Signed; the tools follow the filters.
	want := model.AITotals{Commits: 4, Scanned: 3, Signed: 2, Tools: []model.AIToolCount{{Tool: "claude", Commits: 2}, {Tool: "cursor", Commits: 1}}}
	ranking, err := s.ContributorRanking(ctx, ContributorFilter{})
	if err != nil || len(ranking) != 1 || !reflect.DeepEqual(ranking[0].AI, want) {
		t.Errorf("ranking AI = %+v, %v, want %+v", ranking, err, want)
	}
	profile, err := s.ContributorProfile(ctx, alice)
	if err != nil || !reflect.DeepEqual(profile.AI, want) {
		t.Errorf("profile AI = %+v, %v, want %+v", profile.AI, err, want)
	}
	ranking, err = s.ContributorRanking(ctx, ContributorFilter{RepoID: other})
	wantOther := model.AITotals{Commits: 1, Scanned: 1, Signed: 1, Tools: []model.AIToolCount{{Tool: "claude", Commits: 1}}}
	if err != nil || len(ranking) != 1 || !reflect.DeepEqual(ranking[0].AI, wantOther) {
		t.Errorf("ranking of the other repository = %+v, %v", ranking, err)
	}

	// Rewriting replaces the signatures; deleting the commit removes them.
	if err := s.WriteAIScans(ctx, repo, 2, []AIScanResult{{SHA: sha(1), Signatures: []model.AISignature{{Tool: "codex", Kind: "footer"}}}}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, `SELECT count(*) FROM commit_ai_signatures WHERE repo_id = ? AND sha = ?`, repo, sha(1)); n != 1 {
		t.Errorf("%d signatures after the rewrite, want 1", n)
	}
	mustExec(t, s, `DELETE FROM commits WHERE repo_id = ? AND sha = ?`, repo, sha(1))
	if n := count(t, s, `SELECT count(*) FROM commit_ai_signatures WHERE repo_id = ? AND sha = ?`, repo, sha(1)); n != 0 {
		t.Errorf("%d signatures left after the commit was deleted", n)
	}
}
