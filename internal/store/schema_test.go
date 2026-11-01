package store

// Tests of the schema constraints (00001_init.sql): uniqueness, foreign keys, cascades, CHECK.
// These tests check the schema itself, independently of the sqlc queries: the tables are
// filled here in raw SQL.

import (
	"errors"
	"testing"

	sqlite3 "modernc.org/sqlite/lib"

	"github.com/studoo-app/watchoo/internal/model"
)

const (
	testDate   = "2026-10-26T14:03:00Z"
	insertBr   = "INSERT INTO branches (repo_id, name) VALUES (?, ?)"
	insertCtb  = "INSERT INTO contributors (provider, provider_user_id, provider_login) VALUES (?, ?, ?)"
	insertMail = "INSERT INTO contributor_emails (email, contributor_id) VALUES (?, ?)"
	insertCmt  = `INSERT INTO commits (repo_id, sha, author_id, committer_id, authored_at, committed_at)
		VALUES (?, ?, ?, ?, ?, ?)`
	insertCB  = "INSERT INTO commit_branches (repo_id, sha, branch_id) VALUES (?, ?, ?)"
	insertRun = "INSERT INTO sync_runs (repo_id, started_at, status) VALUES (?, ?, ?)"
)

// wantCode checks that err is a SQLite error with the extended code want.
func wantCode(t *testing.T, what string, err error, want int) {
	t.Helper()
	if got := sqliteCode(err); got != want {
		t.Errorf("%s: SQLite code %d (%v), want %d", what, got, err, want)
	}
}

func TestRepositoriesUniqueness(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	mustCreateRepo(t, s, 100, "Promo", "Alice")

	// Same identifier on the forge, other name (e.g. renamed repository added twice).
	_, err := s.CreateRepository(ctx, testRepo(100, "promo", "autre"))
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("duplicate (provider, provider_id): error %v, want ErrAlreadyExists", err)
	}
	// Same owner/name up to case, other identifier.
	_, err = s.CreateRepository(ctx, testRepo(101, "PROMO", "alice"))
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("case-insensitive duplicate (provider, owner, name): error %v, want ErrAlreadyExists", err)
	}
	// Same provider_id on another forge: accepted.
	other := testRepo(100, "promo", "alice-ailleurs")
	other.Provider = "gitlab"
	if _, err := s.CreateRepository(ctx, other); err != nil {
		t.Errorf("same provider_id on another forge: %v", err)
	}
	if n := count(t, s, "SELECT count(*) FROM repositories"); n != 2 {
		t.Errorf("%d repositories, want 2", n)
	}
}

func TestBranchesUniqueness(t *testing.T) {
	s := newTestStore(t)
	repo1 := mustCreateRepo(t, s, 1, "promo", "alice")
	repo2 := mustCreateRepo(t, s, 2, "promo", "bob")

	mustExec(t, s, insertBr, repo1, "main")
	err := execErr(t, s, insertBr, repo1, "main")
	if !isUniqueViolation(err) {
		t.Errorf("duplicate (repo_id, name): %v, want a uniqueness violation", err)
	}
	// Same branch name in another repository: accepted.
	mustExec(t, s, insertBr, repo2, "main")

	var tracked bool
	if err := s.DB().QueryRowContext(t.Context(),
		"SELECT tracked FROM branches WHERE repo_id = ? AND name = 'main'", repo1).Scan(&tracked); err != nil {
		t.Fatalf("reading tracked: %v", err)
	}
	if !tracked {
		t.Error("tracked is 0 by default, want 1")
	}
}

func TestContributorsUniqueness(t *testing.T) {
	s := newTestStore(t)

	mustExec(t, s, insertCtb, model.ProviderGitHub, 42, "alice")
	err := execErr(t, s, insertCtb, model.ProviderGitHub, 42, "alice-renommee")
	if !isUniqueViolation(err) {
		t.Errorf("duplicate (provider, provider_user_id): %v, want a uniqueness violation", err)
	}

	// Several people without a forge account (provider and provider_user_id NULL): accepted.
	for _, login := range []string{"", "", ""} {
		mustExec(t, s, insertCtb, nil, nil, login)
	}
	if n := count(t, s, "SELECT count(*) FROM contributors WHERE provider IS NULL"); n != 3 {
		t.Errorf("%d contributors without an account, want 3", n)
	}

	// provider and provider_user_id are NULL together or set together.
	wantCode(t, "provider without provider_user_id",
		execErr(t, s, insertCtb, model.ProviderGitHub, nil, "x"), sqlite3.SQLITE_CONSTRAINT_CHECK)
	wantCode(t, "provider_user_id without provider",
		execErr(t, s, insertCtb, nil, 7, "x"), sqlite3.SQLITE_CONSTRAINT_CHECK)
}

func TestContributorEmails(t *testing.T) {
	s := newTestStore(t)
	alice := mustExec(t, s, insertCtb, model.ProviderGitHub, 1, "alice")
	bob := mustExec(t, s, insertCtb, model.ProviderGitHub, 2, "bob")

	mustExec(t, s, insertMail, "alice@example.test", alice)
	mustExec(t, s, insertMail, "alice@perso.example.test", alice)
	// An address designates a single person.
	err := execErr(t, s, insertMail, "alice@example.test", bob)
	if !isUniqueViolation(err) {
		t.Errorf("duplicate e-mail: %v, want a uniqueness violation", err)
	}
	// The address is stored in lowercase and is never empty.
	wantCode(t, "uppercase e-mail",
		execErr(t, s, insertMail, "Bob@Example.test", bob), sqlite3.SQLITE_CONSTRAINT_CHECK)
	wantCode(t, "empty e-mail", execErr(t, s, insertMail, "", bob), sqlite3.SQLITE_CONSTRAINT_CHECK)
	// Nonexistent contributor.
	wantCode(t, "orphan e-mail",
		execErr(t, s, insertMail, "fantome@example.test", 999), sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY)
}

func TestForeignKeysRejectOrphans(t *testing.T) {
	s := newTestStore(t)
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	branch := mustExec(t, s, insertBr, repo, "main")
	mustExec(t, s, insertCmt, repo, "aaa", nil, nil, testDate, testDate)

	const fk = sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY
	wantCode(t, "branch of a nonexistent repository", execErr(t, s, insertBr, 999, "main"), fk)
	wantCode(t, "commit of a nonexistent repository",
		execErr(t, s, insertCmt, 999, "bbb", nil, nil, testDate, testDate), fk)
	wantCode(t, "commit of a nonexistent author",
		execErr(t, s, insertCmt, repo, "ccc", 999, nil, testDate, testDate), fk)
	wantCode(t, "commit of a nonexistent committer",
		execErr(t, s, insertCmt, repo, "ddd", nil, 999, testDate, testDate), fk)
	wantCode(t, "commit_branches to a nonexistent commit",
		execErr(t, s, insertCB, repo, "inconnu", branch), fk)
	wantCode(t, "commit_branches to a nonexistent branch",
		execErr(t, s, insertCB, repo, "aaa", 999), fk)
	wantCode(t, "sync_runs of a nonexistent repository",
		execErr(t, s, insertRun, 999, testDate, string(model.SyncRunning)), fk)

	// A commit is recorded only once per repository.
	err := execErr(t, s, insertCmt, repo, "aaa", nil, nil, testDate, testDate)
	if !isUniqueViolation(err) {
		t.Errorf("duplicate commit: %v, want a primary key violation", err)
	}
}

func TestDeleteRepositoryCascades(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")
	author := mustExec(t, s, insertCtb, model.ProviderGitHub, 10, "alice")

	for _, r := range []int64{repo, other} {
		branch := mustExec(t, s, insertBr, r, "main")
		mustExec(t, s, insertCmt, r, "aaa", author, author, testDate, testDate)
		mustExec(t, s, insertCB, r, "aaa", branch)
		mustExec(t, s, insertRun, r, testDate, string(model.SyncSuccess))
	}

	if err := s.DeleteRepository(ctx, repo); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}

	for _, table := range []string{"branches", "commits", "commit_branches", "sync_runs"} {
		if n := count(t, s, "SELECT count(*) FROM "+table+" WHERE repo_id = ?", repo); n != 0 {
			t.Errorf("%s: %d rows left for the deleted repository", table, n)
		}
		if n := count(t, s, "SELECT count(*) FROM "+table+" WHERE repo_id = ?", other); n != 1 {
			t.Errorf("%s: %d rows for the other repository, want 1", table, n)
		}
	}
	// The contributors are shared between repositories: they stay.
	if n := count(t, s, "SELECT count(*) FROM contributors"); n != 1 {
		t.Errorf("%d contributors after deleting the repository, want 1", n)
	}
}

func TestDeleteBranchAndCommitCascades(t *testing.T) {
	s := newTestStore(t)
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	mainBranch := mustExec(t, s, insertBr, repo, "main")
	dev := mustExec(t, s, insertBr, repo, "dev")
	mustExec(t, s, insertCmt, repo, "aaa", nil, nil, testDate, testDate)
	mustExec(t, s, insertCmt, repo, "bbb", nil, nil, testDate, testDate)
	mustExec(t, s, insertCB, repo, "aaa", mainBranch)
	mustExec(t, s, insertCB, repo, "aaa", dev)
	mustExec(t, s, insertCB, repo, "bbb", dev)

	// Deleting a branch removes its memberships, not the commits.
	mustExec(t, s, "DELETE FROM branches WHERE id = ?", dev)
	if n := count(t, s, "SELECT count(*) FROM commit_branches"); n != 1 {
		t.Errorf("%d memberships after deleting the branch, want 1", n)
	}
	if n := count(t, s, "SELECT count(*) FROM commits"); n != 2 {
		t.Errorf("%d commits after deleting the branch, want 2", n)
	}

	// Deleting a commit removes its memberships.
	mustExec(t, s, "DELETE FROM commits WHERE repo_id = ? AND sha = 'aaa'", repo)
	if n := count(t, s, "SELECT count(*) FROM commit_branches"); n != 0 {
		t.Errorf("%d memberships after deleting the commit, want 0", n)
	}
}

func TestDeleteContributorKeepsCommits(t *testing.T) {
	s := newTestStore(t)
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	author := mustExec(t, s, insertCtb, model.ProviderGitHub, 10, "alice")
	committer := mustExec(t, s, insertCtb, model.ProviderGitHub, 11, "bot")
	mustExec(t, s, insertMail, "alice@example.test", author)
	mustExec(t, s, insertCmt, repo, "aaa", author, committer, testDate, testDate)

	mustExec(t, s, "DELETE FROM contributors WHERE id IN (?, ?)", author, committer)

	if n := count(t, s, "SELECT count(*) FROM commits WHERE author_id IS NULL AND committer_id IS NULL"); n != 1 {
		t.Errorf("commit after deleting the contributors: %d row(s) with author and committer NULL, want 1", n)
	}
	if n := count(t, s, "SELECT count(*) FROM contributor_emails"); n != 0 {
		t.Errorf("%d e-mails left after deleting the contributor, want 0", n)
	}
}

func TestDateAndBooleanChecks(t *testing.T) {
	s := newTestStore(t)
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	const check = sqlite3.SQLITE_CONSTRAINT_CHECK

	for _, bad := range []string{
		"2026-10-26 14:03:00",       // space instead of T, without Z
		"2026-10-26T14:03:00.123Z",  // fraction of a second
		"2026-10-26T14:03:00+02:00", // time offset
		"",
	} {
		wantCode(t, "authored_at "+bad,
			execErr(t, s, insertCmt, repo, "sha-"+bad, nil, nil, bad, testDate), check)
		wantCode(t, "started_at "+bad,
			execErr(t, s, insertRun, repo, bad, string(model.SyncRunning)), check)
		wantCode(t, "last_synced_at "+bad,
			execErr(t, s, "UPDATE repositories SET last_synced_at = ? WHERE id = ?", bad, repo), check)
	}
	wantCode(t, "archived = 2",
		execErr(t, s, "UPDATE repositories SET archived = 2 WHERE id = ?", repo), check)
	wantCode(t, "flemme_after_days = 0",
		execErr(t, s, "UPDATE repositories SET flemme_after_days = 0 WHERE id = ?", repo), check)
	wantCode(t, "sha vide", execErr(t, s, insertCmt, repo, "", nil, nil, testDate, testDate), check)
}
