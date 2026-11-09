package store

// Migration 00017 (phase 8.3): table repository_conventions, cache of the file criteria of the
// conventions, and its store methods.

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestMigrationRepositoryConventions(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 16); err != nil {
		t.Fatalf("UpTo(16): %v", err)
	}
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	const insert = `INSERT INTO repository_conventions (repo_id, rev_kind, tag_name, rev_sha, rules_version,
		config_hash, results, computed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	mustExec(t, s, insert, repo, "head", "", sha(1), 1, "h", "{}", testDate)
	mustExec(t, s, insert, repo, "tag", "v1.0.0", sha(2), 1, "h", `{"changelog":[]}`, testDate)
	for _, bad := range []struct {
		name string
		args []any
	}{
		{"unknown kind", []any{repo, "branch", "x", sha(3), 1, "h", "{}", testDate}},
		{"head with a tag name", []any{repo, "head", "v2", sha(3), 1, "h", "{}", testDate}},
		{"tag without a name", []any{repo, "tag", "", sha(3), 1, "h", "{}", testDate}},
		{"short SHA", []any{repo, "tag", "v3", "abc1234", 1, "h", "{}", testDate}},
		{"uppercase SHA", []any{repo, "tag", "v3", "ABCDEF0123456789ABCDEF0123456789ABCDEF01", 1, "h", "{}", testDate}},
		{"rules version 0", []any{repo, "tag", "v3", sha(3), 0, "h", "{}", testDate}},
		{"empty hash", []any{repo, "tag", "v3", sha(3), 1, "", "{}", testDate}},
		{"results not an object", []any{repo, "tag", "v3", sha(3), 1, "h", "[]", testDate}},
		{"results not JSON", []any{repo, "tag", "v3", sha(3), 1, "h", "{", testDate}},
		{"date", []any{repo, "tag", "v3", sha(3), 1, "h", "{}", "2026-09-01"}},
		{"second head", []any{repo, "head", "", sha(3), 1, "h", "{}", testDate}},
		{"unknown repository", []any{repo + 99, "tag", "v3", sha(3), 1, "h", "{}", testDate}},
	} {
		if err := execErr(t, s, insert, bad.args...); err == nil {
			t.Errorf("%s accepted", bad.name)
		}
	}

	// Rollback: table removed, then recreated.
	if _, err := p.DownTo(ctx, 16); err != nil {
		t.Fatalf("DownTo(16): %v", err)
	}
	if got := count(t, s, `SELECT count(*) FROM sqlite_master WHERE name = 'repository_conventions'`); got != 0 {
		t.Error("table still present after the rollback")
	}
	if _, err := p.UpTo(ctx, 17); err != nil {
		t.Fatalf("UpTo(17) again: %v", err)
	}
}

func TestRepositoryConventionsStore(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	other := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 2, 'promo', 'b')`)
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	row := func(id int64, kind, tag string, i int) model.RepositoryConventions {
		return model.RepositoryConventions{RepoID: id, RevKind: kind, TagName: tag, RevSHA: sha(i), RulesVersion: 1,
			ConfigHash: "h", Results: `{"rules_version":1}`, ComputedAt: at}
	}
	for _, r := range []model.RepositoryConventions{
		row(repo, model.ConventionRevTag, "v2.0.0", 3), row(repo, model.ConventionRevHead, "", 1),
		row(repo, model.ConventionRevTag, "v1.0.0", 2), row(other, model.ConventionRevHead, "", 4),
	} {
		if err := s.SaveRepositoryConventions(ctx, r); err != nil {
			t.Fatalf("SaveRepositoryConventions(%+v): %v", r, err)
		}
	}
	// Replaced for the same revision.
	updated := row(repo, model.ConventionRevHead, "", 5)
	updated.ConfigHash, updated.RulesVersion = "h2", 2
	if err := s.SaveRepositoryConventions(ctx, updated); err != nil {
		t.Fatal(err)
	}
	bad := row(repo, "branch", "x", 6)
	if err := s.SaveRepositoryConventions(ctx, bad); err == nil {
		t.Error("unknown kind accepted")
	}

	got, err := s.RepositoryConventions(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	keys := func(rows []model.RepositoryConventions) []string {
		var out []string
		for _, r := range rows {
			out = append(out, r.RevKind+":"+r.TagName)
		}
		return out
	}
	if want := []string{"head:", "tag:v1.0.0", "tag:v2.0.0"}; !slices.Equal(keys(got), want) {
		t.Errorf("rows = %v, want %v", keys(got), want)
	}
	if got[0] != updated {
		t.Errorf("head = %+v, want %+v", got[0], updated)
	}
	if all, err := s.RepositoryConventions(ctx, 0); err != nil || len(all) != 4 {
		t.Errorf("all repositories: %d rows, %v", len(all), err)
	}

	if n, err := s.DeleteRepositoryConventionTags(ctx, repo, []string{"v2.0.0", "v9.0.0"}); err != nil || n != 1 {
		t.Errorf("DeleteRepositoryConventionTags(keep v2.0.0) = %d, %v", n, err)
	}
	if got, _ := s.RepositoryConventions(ctx, repo); !slices.Equal(keys(got), []string{"head:", "tag:v2.0.0"}) {
		t.Errorf("after the deletion: %v", keys(got))
	}
	if n, err := s.DeleteRepositoryConventionTags(ctx, repo, nil); err != nil || n != 1 {
		t.Errorf("DeleteRepositoryConventionTags(nil) = %d, %v", n, err)
	}
	if got, _ := s.RepositoryConventions(ctx, repo); !slices.Equal(keys(got), []string{"head:"}) {
		t.Errorf("head kept: %v", keys(got))
	}
	if n, err := s.DeleteRepositoryConventionHead(ctx, repo); err != nil || n != 1 {
		t.Errorf("DeleteRepositoryConventionHead = %d, %v", n, err)
	}
	if got, _ := s.RepositoryConventions(ctx, repo); len(got) != 0 {
		t.Errorf("after the head deletion: %v", keys(got))
	}

	// Deleting the repository deletes its rows only.
	if err := s.DeleteRepository(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM repository_conventions WHERE repo_id = ?", repo); n != 0 {
		t.Errorf("%d rows left for the deleted repository", n)
	}
	if n := count(t, s, "SELECT count(*) FROM repository_conventions WHERE repo_id = ?", other); n != 1 {
		t.Errorf("%d rows for the other repository, want 1", n)
	}
	if none, err := newTestStore(t).RepositoryConventions(ctx, 0); err != nil || none == nil || len(none) != 0 {
		t.Errorf("empty database: %v, %v", none, err)
	}
}
