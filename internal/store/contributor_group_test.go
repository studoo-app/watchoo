package store

// Migration 00023 (1.9.0): contributor groups. Attaching, detaching, refusals, sync and automatic
// merge keep commits.person_id and commit_signals.person_id equal to the principal of the author.

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestMigrationContributorGroups(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 22); err != nil {
		t.Fatalf("UpTo(22): %v", err)
	}
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	who := mustExec(t, s, `INSERT INTO contributors (provider_login, name) VALUES ('alice', 'Alice')`)
	mustExec(t, s, `INSERT INTO commits (repo_id, sha, author_id, authored_at, committed_at) VALUES (?, ?, ?, ?, ?)`,
		repo, sha(1), who, testDate, testDate)
	mustExec(t, s, `INSERT INTO commits (repo_id, sha, authored_at, committed_at) VALUES (?, ?, ?, ?)`,
		repo, sha(2), testDate, testDate)
	mustExec(t, s, `INSERT INTO commit_signals (repo_id, sha, author_id, authored_at, computed_at) VALUES (?, ?, ?, ?, ?)`,
		repo, sha(1), who, testDate, testDate)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM commits WHERE person_id IS author_id"); got != 2 {
		t.Errorf("%d commits with person_id = author_id, want 2", got)
	}
	if got := count(t, s, "SELECT count(*) FROM commit_signals WHERE person_id = ?", who); got != 1 {
		t.Errorf("%d signals with the person, want 1", got)
	}
	if err := execErr(t, s, "UPDATE contributors SET primary_id = id"); err == nil {
		t.Error("contributor attached to itself accepted")
	}

	if _, err := p.DownTo(ctx, 22); err != nil {
		t.Fatalf("DownTo(22): %v", err)
	}
	for _, q := range []string{"SELECT primary_id FROM contributors", "SELECT person_id FROM commits", "SELECT person_id FROM commit_signals"} {
		if err := execErr(t, s, q); err == nil {
			t.Errorf("%s: column still present", q)
		}
	}
	if got := count(t, s, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'commit_signals_author_date'`); got != 1 {
		t.Error("index commit_signals_author_date not recreated")
	}
	if got := count(t, s, "SELECT count(*) FROM commits"); got != 2 {
		t.Errorf("%d commits after the rollback, want 2", got)
	}
	if _, err := p.UpTo(ctx, 23); err != nil {
		t.Fatalf("UpTo(23) again: %v", err)
	}
}

// personOf returns the person_id of commit n of repository repoID (0 if NULL).
func personOf(t *testing.T, s *Store, repoID int64, n int) int64 {
	t.Helper()
	var id *int64
	if err := s.DB().QueryRowContext(t.Context(), "SELECT person_id FROM commits WHERE repo_id = ? AND sha = ?", repoID, sha(n)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id == nil {
		return 0
	}
	return *id
}

func TestAttachDetachContributor(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	main := []BranchState{{Name: "main", HeadSHA: sha(9)}}
	applySync(t, s, repoID, main,
		cw(1, person("Alice", "alice@school.test", "alice", 1), "main"),
		cw(2, person("Alice", "alice@perso.test", "alice-perso", 2), "main"),
		cw(3, person("alice m", "am@laptop.test", "", 0), "main"),
		cw(4, person("Bob", "bob@school.test", "bob", 3), "main"))
	alice, perso, laptop, bob := contributorID(t, s, "alice"), contributorID(t, s, "alice-perso"),
		contributorID(t, s, "alice m"), contributorID(t, s, "bob")
	if _, err := s.WriteCommitSignals(ctx, repoID, []CommitSignalWrite{{SHA: sha(2), FilesKept: 1}}, t0); err != nil {
		t.Fatal(err)
	}
	for n, want := range map[int]int64{1: alice, 2: perso, 3: laptop, 4: bob} {
		if got := personOf(t, s, repoID, n); got != want {
			t.Errorf("commit %d before grouping: person %d, want %d", n, got, want)
		}
	}

	for _, id := range []int64{perso, laptop} {
		if err := s.AttachContributor(ctx, id, alice); err != nil {
			t.Fatalf("attach %d: %v", id, err)
		}
	}
	for n, want := range map[int]int64{1: alice, 2: alice, 3: alice, 4: bob} {
		if got := personOf(t, s, repoID, n); got != want {
			t.Errorf("commit %d after grouping: person %d, want %d", n, got, want)
		}
	}
	if got := count(t, s, "SELECT count(*) FROM commit_signals WHERE sha = ? AND person_id = ? AND author_id = ?", sha(2), alice, perso); got != 1 {
		t.Error("signal of the attached account not moved to the person")
	}
	// The diagnosis reads the person as the author (histories, medians, day leads per person).
	views, err := s.CommitSignalViews(ctx, repoID)
	if err != nil || len(views) != 1 || views[0].AuthorID != alice || views[0].AuthorLogin != "alice-perso" {
		t.Errorf("signal views after grouping: %+v, %v", views, err)
	}
	if ids, err := s.MatchingContributorIDs(ctx, "alice-perso"); err != nil || len(ids) != 1 || ids[0] != alice {
		t.Errorf("MatchingContributorIDs(attached login) = %v, %v; want the person", ids, err)
	}
	if labels, err := s.ContributorLabels(ctx); err != nil || labels[alice] != "Alice" {
		t.Errorf("display override of the principal = %v, %v", labels, err)
	}
	// The sync gives the person to the new commits of an attached account.
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(5)}},
		cw(5, person("Alice", "alice@perso.test", "alice-perso", 2), "main"))
	if got := personOf(t, s, repoID, 5); got != alice {
		t.Errorf("new commit of an attached account: person %d, want %d", got, alice)
	}
	if p, err := s.PrimaryOf(ctx, perso); err != nil || p != alice {
		t.Errorf("PrimaryOf(perso) = %d, %v", p, err)
	}
	accounts, err := s.AttachedAccounts(ctx)
	if err != nil || len(accounts[alice]) != 2 || accounts[alice][0].ID != perso || accounts[alice][0].Commits != 2 {
		t.Errorf("AttachedAccounts = %+v, %v", accounts, err)
	}

	// Refusals.
	for name, tc := range map[string]struct {
		id, primary int64
		want        error
	}{
		"self":               {bob, bob, ErrGroupSelf},
		"already attached":   {perso, bob, ErrGroupAttached},
		"has accounts":       {alice, bob, ErrGroupHasAccounts},
		"target is attached": {bob, perso, ErrGroupNotPrimary},
		"unknown account":    {999, alice, ErrNotFound},
		"unknown principal":  {bob, 999, ErrNotFound},
	} {
		if err := s.AttachContributor(ctx, tc.id, tc.primary); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
		}
	}
	if err := s.DetachContributor(ctx, bob); !errors.Is(err, ErrGroupNotAttached) {
		t.Errorf("detach a principal: %v", err)
	}
	if err := s.DetachContributor(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("detach unknown: %v", err)
	}

	// Detaching restores exactly.
	if err := s.DetachContributor(ctx, perso); err != nil {
		t.Fatal(err)
	}
	for n, want := range map[int]int64{1: alice, 2: perso, 3: alice, 5: perso} {
		if got := personOf(t, s, repoID, n); got != want {
			t.Errorf("commit %d after detaching: person %d, want %d", n, got, want)
		}
	}
	if got := count(t, s, "SELECT count(*) FROM commit_signals WHERE sha = ? AND person_id = ?", sha(2), perso); got != 1 {
		t.Error("signal not given back to the detached account")
	}
}

func TestMergeKeepsGroups(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	main := []BranchState{{Name: "main", HeadSHA: sha(9)}}

	// A Git identity without account attached to Alice's principal account, then seen with another
	// account (Dave's laptop is merged into the "dave2" account): the account joins Alice's group.
	applySync(t, s, repoID, main,
		cw(1, person("Alice", "alice@school.test", "alice", 1), "main"),
		cw(2, person("laptop", "laptop@home.test", "", 0), "main"))
	alice, laptop := contributorID(t, s, "alice"), contributorID(t, s, "laptop")
	if err := s.AttachContributor(ctx, laptop, alice); err != nil {
		t.Fatal(err)
	}
	applySync(t, s, repoID, main, cw(3, person("Alice 2", "laptop@home.test", "alice2", 7), "main"))
	alice2 := contributorID(t, s, "alice2")
	if p, err := s.PrimaryOf(ctx, alice2); err != nil || p != alice {
		t.Errorf("merged account: principal %d, %v; want %d", p, err, alice)
	}
	for n := 1; n <= 3; n++ {
		if got := personOf(t, s, repoID, n); got != alice {
			t.Errorf("commit %d: person %d, want %d", n, got, alice)
		}
	}

	// A principal without account, with an attached account, absorbed by an account: the attached
	// account follows to the new principal.
	applySync(t, s, repoID, main,
		cw(4, person("Carol", "carol@school.test", "", 0), "main"),
		cw(5, person("Carol GH", "carol@gh.test", "carol-gh", 8), "main"))
	carolGit, carolGH := contributorID(t, s, "Carol"), contributorID(t, s, "carol-gh")
	if err := s.AttachContributor(ctx, carolGH, carolGit); err != nil {
		t.Fatal(err)
	}
	applySync(t, s, repoID, main, cw(6, person("Carol", "carol@school.test", "carol", 9), "main"))
	carol := contributorID(t, s, "carol")
	if p, _ := s.PrimaryOf(ctx, carolGH); p != carol {
		t.Errorf("attached account after the merge of its principal: principal %d, want %d", p, carol)
	}
	if p, _ := s.PrimaryOf(ctx, carol); p != carol {
		t.Errorf("absorbing account: principal %d, want itself", p)
	}
	for n := 4; n <= 6; n++ {
		if got := personOf(t, s, repoID, n); got != carol {
			t.Errorf("commit %d: person %d, want %d", n, got, carol)
		}
	}

	// The absorbing account was attached to the absorbed one: it becomes the principal.
	applySync(t, s, repoID, main,
		cw(7, person("Erin", "erin@school.test", "", 0), "main"),
		cw(8, person("Erin GH", "erin@gh.test", "erin", 10), "main"))
	erinGit, erin := contributorID(t, s, "Erin"), contributorID(t, s, "erin")
	if err := s.AttachContributor(ctx, erin, erinGit); err != nil {
		t.Fatal(err)
	}
	applySync(t, s, repoID, main, cw(9, person("Erin GH", "erin@school.test", "erin", 10), "main"))
	if p, _ := s.PrimaryOf(ctx, erin); p != erin {
		t.Errorf("erin: principal %d, want itself", p)
	}
	for n := 7; n <= 9; n++ {
		if got := personOf(t, s, repoID, n); got != erin {
			t.Errorf("commit %d: person %d, want %d", n, got, erin)
		}
	}
}
