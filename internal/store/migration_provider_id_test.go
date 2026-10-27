package store

// Migration 00004 (V2) : provider_id facultatif, par reconstruction de la table repositories.

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

// childCounts compte les lignes des tables qui dépendent de repositories.
func childCounts(t *testing.T, s *Store) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{"branches", "commits", "commit_branches", "sync_runs", "pull_requests",
		"releases", "tags", "contributors", "contributor_emails"} {
		out[table] = count(t, s, "SELECT count(*) FROM "+table)
	}
	return out
}

func TestMigrationProviderIDNullableKeepsData(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 3); err != nil {
		t.Fatalf("UpTo(3) : %v", err)
	}

	// Données de la v0.2 : deux dépôts complets avec tout ce qui en dépend.
	mustExec(t, s, `INSERT INTO repositories (id, provider, provider_id, owner, name, label, default_branch, url,
		stars, open_issues, last_synced_at, last_commit_at, archived, flemme_after_days, dead_after_days, enabled,
		branch_patterns) VALUES (7, 'github', 1007, 'Promo', 'Alice', 'Projet A', 'main', 'https://github.com/Promo/Alice',
		3, 2, ?, ?, 1, 4, 9, 0, 'main,dev')`, testDate, testDate)
	mustExec(t, s, `INSERT INTO repositories (id, provider, provider_id, owner, name) VALUES (8, 'github', 1008, 'promo', 'bob')`)
	for _, repo := range []int64{7, 8} {
		br := mustExec(t, s, insertBr, repo, "main")
		ctb := mustExec(t, s, insertCtb, "github", 500+repo, "login")
		mustExec(t, s, insertMail, "e"+string(rune('0'+repo))+"@example.test", ctb)
		mustExec(t, s, insertCmt, repo, "sha1", ctb, ctb, testDate, testDate)
		mustExec(t, s, insertCB, repo, "sha1", br)
		mustExec(t, s, insertRun, repo, testDate, "success")
		mustExec(t, s, `INSERT INTO pull_requests (repo_id, number, title, state, created_at, updated_at)
			VALUES (?, 1, 't', 'open', ?, ?)`, repo, testDate, testDate)
		mustExec(t, s, `INSERT INTO releases (repo_id, provider_id, tag_name, created_at) VALUES (?, 1, 'v1', ?)`,
			repo, testDate)
		mustExec(t, s, `INSERT INTO tags (repo_id, name, sha) VALUES (?, 'v1', 'sha1')`, repo)
	}
	before := childCounts(t, s)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate : %v", err)
	}

	if after := childCounts(t, s); len(after) != len(before) {
		t.Fatal("tables manquantes")
	} else {
		for table, n := range before {
			if after[table] != n || n == 0 {
				t.Errorf("%s : %d lignes après la migration, %d avant (aucune ne doit être perdue)", table, after[table], n)
			}
		}
	}
	var fk int
	if err := s.DB().QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Errorf("clés étrangères après la migration : %d, %v", fk, err)
	}
	if n := count(t, s, "SELECT count(*) FROM pragma_foreign_key_check"); n != 0 {
		t.Errorf("%d violations de clés étrangères", n)
	}
	var check string
	if err := s.DB().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&check); err != nil || check != "ok" {
		t.Errorf("integrity_check : %q, %v", check, err)
	}

	repo, err := s.GetRepository(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	flemme, dead := 4, 9
	want := model.Repository{ID: 7, Provider: "github", ProviderID: 1007, Owner: "Promo", Name: "Alice",
		Label: "Projet A", DefaultBranch: "main", URL: "https://github.com/Promo/Alice", Stars: 3, OpenIssues: 2,
		LastSyncedAt: time.Date(2026, 10, 26, 14, 3, 0, 0, time.UTC), LastCommitAt: time.Date(2026, 10, 26, 14, 3, 0, 0, time.UTC),
		Archived: true, FlemmeAfterDays: &flemme, DeadAfterDays: &dead, Enabled: false, BranchPatterns: []string{"main", "dev"}}
	if repo.ID != want.ID || repo.ProviderID != want.ProviderID || repo.Owner != want.Owner || repo.Label != want.Label ||
		repo.URL != want.URL || repo.Stars != 3 || repo.OpenIssues != 2 || !repo.Archived || repo.Enabled ||
		*repo.FlemmeAfterDays != 4 || *repo.DeadAfterDays != 9 || len(repo.BranchPatterns) != 2 ||
		!repo.LastSyncedAt.Equal(want.LastSyncedAt) {
		t.Errorf("dépôt après migration = %+v", repo)
	}
	// Owner et name toujours comparés sans tenir compte de la casse.
	if _, err := s.GetRepositoryByName(ctx, "github", "promo", "ALICE"); err != nil {
		t.Errorf("recherche sans casse après migration : %v", err)
	}
	// La cascade marche toujours sur la nouvelle table.
	if err := s.DeleteRepository(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM commits WHERE repo_id = 8"); n != 0 {
		t.Errorf("cascade : %d commits restants", n)
	}
}

func TestProviderIDNullable(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	// Deux dépôts sans identifiant de forge (NULL) : acceptés par UNIQUE (provider, provider_id).
	a := mustCreateRepo(t, s, 0, "promo", "alice")
	mustCreateRepo(t, s, 0, "promo", "bob")
	if n := count(t, s, "SELECT count(*) FROM repositories WHERE provider_id IS NULL"); n != 2 {
		t.Errorf("%d dépôts à provider_id NULL, attendu 2", n)
	}
	got, err := s.GetRepository(ctx, a)
	if err != nil || got.ProviderID != 0 || got.ForgeDetails() {
		t.Errorf("dépôt sans identifiant : %+v, %v", got, err)
	}
	if _, err := s.GetRepositoryByProviderID(ctx, "github", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetRepositoryByProviderID(0) : %v, ErrNotFound attendue", err)
	}
	// 0 et les valeurs négatives sont refusés en SQL brut (CHECK).
	err = execErr(t, s, "INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 0, 'x', 'y')")
	wantCode(t, "provider_id 0", err, sqlite3.SQLITE_CONSTRAINT_CHECK)

	// Une synchro par l'API résout l'identifiant ; une synchro par clone Git le garde.
	run, err := s.BeginSyncRun(ctx, a, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{RepoID: a, RunID: run.ID, SyncedAt: time.Now().UTC(),
		Metadata: RepositoryMetadata{ProviderID: 4242, Owner: "promo", Name: "alice", DefaultBranch: "main",
			URL: "https://github.com/promo/alice", Stars: 5, OpenIssues: 1, Archived: true}}); err != nil {
		t.Fatal(err)
	}
	run, err = s.BeginSyncRun(ctx, a, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	synced := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{RepoID: a, RunID: run.ID, SyncedAt: synced,
		Metadata: RepositoryMetadata{Partial: true, Owner: "ignore", Name: "ignore", DefaultBranch: "trunk"}}); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetRepository(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProviderID != 4242 || got.Owner != "promo" || got.Name != "alice" || got.Stars != 5 || got.OpenIssues != 1 ||
		!got.Archived || got.URL != "https://github.com/promo/alice" || got.DefaultBranch != "trunk" ||
		!got.LastSyncedAt.Equal(synced) {
		t.Errorf("après synchro API puis Git : %+v", got)
	}
	// Branche par défaut inconnue (dépôt vide) : gardée.
	run, err = s.BeginSyncRun(ctx, a, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{RepoID: a, RunID: run.ID, SyncedAt: synced,
		Metadata: RepositoryMetadata{Partial: true}}); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetRepository(ctx, a); got.DefaultBranch != "trunk" {
		t.Errorf("branche par défaut écrasée : %q", got.DefaultBranch)
	}
}
