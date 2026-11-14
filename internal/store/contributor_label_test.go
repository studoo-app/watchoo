package store

// Migration 00022 (1.8.0): names given to the contributors (contributors.label), never written by the
// sync, kept by a merge, read by the views.

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestMigrationContributorLabels(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 21); err != nil {
		t.Fatalf("UpTo(21): %v", err)
	}
	mustExec(t, s, `INSERT INTO contributors (provider_login, name) VALUES ('alice42', 'Alice')`)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM contributors WHERE label = ''"); got != 1 {
		t.Errorf("%d contributors with an empty label, want 1", got)
	}
	if err := execErr(t, s, "UPDATE contributors SET label = ?", strings.Repeat("é", 101)); err == nil {
		t.Error("label of 101 characters accepted")
	}
	mustExec(t, s, "UPDATE contributors SET label = ?", strings.Repeat("é", 100))

	// Rollback: column removed, rows kept.
	if _, err := p.DownTo(ctx, 21); err != nil {
		t.Fatalf("DownTo(21): %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM contributors"); got != 1 {
		t.Errorf("%d contributors after the rollback, want 1", got)
	}
	if err := execErr(t, s, "SELECT label FROM contributors"); err == nil {
		t.Error("contributors.label still present")
	}
	if _, err := p.UpTo(ctx, 22); err != nil {
		t.Fatalf("UpTo(22) again: %v", err)
	}
}

// contributorID returns the identifier of the contributor whose login or name is who.
func contributorID(t *testing.T, s *Store, who string) int64 {
	t.Helper()
	var id int64
	if err := s.DB().QueryRowContext(t.Context(),
		"SELECT id FROM contributors WHERE provider_login = ? OR name = ?", who, who).Scan(&id); err != nil {
		t.Fatalf("contributor %q: %v", who, err)
	}
	return id
}

func TestContributorLabels(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	main := []BranchState{{Name: "main", HeadSHA: sha(9)}}
	applySync(t, s, repoID, main,
		cw(1, person("Alice", "alice@example.test", "alice42", 42), "main"),
		cw(2, person("Bob", "bob@example.test", "", 0), "main"))
	alice, bob := contributorID(t, s, "alice42"), contributorID(t, s, "Bob")

	got, err := s.SetContributorLabel(ctx, alice, "  Alice Martin  ")
	if err != nil || got != "Alice Martin" {
		t.Fatalf("SetContributorLabel = %q, %v", got, err)
	}
	for _, bad := range []string{"Alice\nMartin", strings.Repeat("a", 101)} {
		if _, err := s.SetContributorLabel(ctx, alice, bad); err == nil {
			t.Errorf("label %q accepted", bad)
		}
	}
	if _, err := s.SetContributorLabel(ctx, alice, "Alice\x07"); !errors.Is(err, model.ErrContributorLabelInvalid) {
		t.Errorf("control character: %v, want ErrContributorLabelInvalid", err)
	}
	if _, err := s.SetContributorLabel(ctx, 999, "X"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown contributor: %v, want ErrNotFound", err)
	}

	// The sync never writes the label: a new commit with a new name and login keeps it.
	applySync(t, s, repoID, main, cw(3, person("A. Martin", "alice@example.test", "alice-m", 42), "main"))
	labels, err := s.ContributorLabels(ctx)
	if err != nil || len(labels) != 1 || labels[alice] != "Alice Martin" {
		t.Fatalf("ContributorLabels = %v, %v", labels, err)
	}

	// The views carry it: ranking, profile, commits.
	ranking, err := s.ContributorRanking(ctx, ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range ranking {
		if c.ID == alice && (c.Label != "Alice Martin" || c.DisplayName() != "Alice Martin" || c.Name != "A. Martin") {
			t.Errorf("ranking: %+v", c.Contributor)
		}
	}
	prof, err := s.ContributorProfile(ctx, alice)
	if err != nil || prof.Label != "Alice Martin" || prof.CommitName() != "A. Martin" {
		t.Errorf("profile: %+v, %v", prof.Contributor, err)
	}
	page, err := s.CommitPage(ctx, CommitFilter{AuthorID: alice}, 1, 10)
	if err != nil || len(page.Commits) != 2 || page.Commits[0].AuthorDisplayName() != "Alice Martin" {
		t.Errorf("commits: %+v, %v", page.Commits, err)
	}
	// The author filters accept it, exactly and without case.
	ids, err := s.MatchingContributorIDs(ctx, "alice martin")
	if err != nil || len(ids) != 1 || ids[0] != alice {
		t.Errorf("MatchingContributorIDs = %v, %v", ids, err)
	}
	recent, err := s.ListRecentCommits(ctx, CommitFilter{Author: "ALICE MARTIN"})
	if err != nil || len(recent) != 2 {
		t.Errorf("RecentCommits by label: %d commits, %v", len(recent), err)
	}

	list, err := s.LabeledContributors(ctx)
	if err != nil || len(list) != 2 || list[0].ID != alice || list[0].Commits != 2 || list[0].Identities != 1 ||
		list[1].ID != bob || list[1].Label != "" {
		t.Errorf("LabeledContributors = %+v, %v", list, err)
	}

	// Empty: the label is removed.
	if got, err := s.SetContributorLabel(ctx, alice, "   "); err != nil || got != "" {
		t.Errorf("removal = %q, %v", got, err)
	}
	if labels, _ := s.ContributorLabels(ctx); len(labels) != 0 {
		t.Errorf("labels after removal: %v", labels)
	}
}

func TestContributorLabelKeptByMerge(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	main := []BranchState{{Name: "main", HeadSHA: sha(9)}}
	// Dave without account (local clone), named in the web interface, then seen with his account.
	applySync(t, s, repoID, main, cw(1, person("Dave", "dave@school.test", "", 0), "main"))
	if _, err := s.SetContributorLabel(ctx, contributorID(t, s, "Dave"), "David Durand"); err != nil {
		t.Fatal(err)
	}
	applySync(t, s, repoID, main, cw(2, person("Dave D.", "dave@school.test", "dave", 42), "main"))
	if got := count(t, s, "SELECT count(*) FROM contributors"); got != 1 {
		t.Fatalf("%d contributors after merge, want 1", got)
	}
	if got := count(t, s, "SELECT count(*) FROM contributors WHERE provider_login = 'dave' AND label = 'David Durand'"); got != 1 {
		t.Error("label of the absorbed contributor lost")
	}

	// The contributor with an account keeps its own label.
	applySync(t, s, repoID, main, cw(3, person("Erin", "erin@school.test", "", 0), "main"))
	erinGit := contributorID(t, s, "Erin")
	if _, err := s.SetContributorLabel(ctx, erinGit, "Erin (git)"); err != nil {
		t.Fatal(err)
	}
	applySync(t, s, repoID, main, cw(4, person("Erin", "erin@other.test", "erin", 77), "main"))
	if _, err := s.SetContributorLabel(ctx, contributorID(t, s, "erin"), "Erin Petit"); err != nil {
		t.Fatal(err)
	}
	applySync(t, s, repoID, main, cw(5, person("Erin", "erin@school.test", "erin", 77), "main"))
	if got := count(t, s, "SELECT count(*) FROM contributors WHERE provider_login = 'erin' AND label = 'Erin Petit'"); got != 1 {
		t.Error("label of the contributor with an account replaced by the merge")
	}
}
