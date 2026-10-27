package store

// Tests des contraintes du schéma (00001_init.sql) : unicité, clés étrangères, cascades, CHECK.
// Ces tests vérifient le schéma lui-même, indépendamment des requêtes sqlc : les tables sont
// alimentées ici en SQL brut.

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

// wantCode vérifie que err est une erreur SQLite de code étendu want.
func wantCode(t *testing.T, what string, err error, want int) {
	t.Helper()
	if got := sqliteCode(err); got != want {
		t.Errorf("%s : code SQLite %d (%v), attendu %d", what, got, err, want)
	}
}

func TestRepositoriesUniqueness(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	mustCreateRepo(t, s, 100, "Promo", "Alice")

	// Même identifiant sur la forge, autre nom (ex. dépôt renommé ajouté deux fois).
	_, err := s.CreateRepository(ctx, testRepo(100, "promo", "autre"))
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("doublon (provider, provider_id) : erreur %v, attendu ErrAlreadyExists", err)
	}
	// Même owner/name à la casse près, autre identifiant.
	_, err = s.CreateRepository(ctx, testRepo(101, "PROMO", "alice"))
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("doublon (provider, owner, name) sans tenir compte de la casse : erreur %v, attendu ErrAlreadyExists", err)
	}
	// Même provider_id sur une autre forge : accepté.
	other := testRepo(100, "promo", "alice-ailleurs")
	other.Provider = "gitlab"
	if _, err := s.CreateRepository(ctx, other); err != nil {
		t.Errorf("même provider_id sur une autre forge : %v", err)
	}
	if n := count(t, s, "SELECT count(*) FROM repositories"); n != 2 {
		t.Errorf("%d dépôts, attendu 2", n)
	}
}

func TestBranchesUniqueness(t *testing.T) {
	s := newTestStore(t)
	repo1 := mustCreateRepo(t, s, 1, "promo", "alice")
	repo2 := mustCreateRepo(t, s, 2, "promo", "bob")

	mustExec(t, s, insertBr, repo1, "main")
	err := execErr(t, s, insertBr, repo1, "main")
	if !isUniqueViolation(err) {
		t.Errorf("doublon (repo_id, name) : %v, attendu une violation d'unicité", err)
	}
	// Même nom de branche dans un autre dépôt : accepté.
	mustExec(t, s, insertBr, repo2, "main")

	var tracked bool
	if err := s.DB().QueryRowContext(t.Context(),
		"SELECT tracked FROM branches WHERE repo_id = ? AND name = 'main'", repo1).Scan(&tracked); err != nil {
		t.Fatalf("lecture de tracked : %v", err)
	}
	if !tracked {
		t.Error("tracked vaut 0 par défaut, attendu 1")
	}
}

func TestContributorsUniqueness(t *testing.T) {
	s := newTestStore(t)

	mustExec(t, s, insertCtb, model.ProviderGitHub, 42, "alice")
	err := execErr(t, s, insertCtb, model.ProviderGitHub, 42, "alice-renommee")
	if !isUniqueViolation(err) {
		t.Errorf("doublon (provider, provider_user_id) : %v, attendu une violation d'unicité", err)
	}

	// Plusieurs personnes sans compte de forge (provider et provider_user_id à NULL) : acceptées.
	for _, login := range []string{"", "", ""} {
		mustExec(t, s, insertCtb, nil, nil, login)
	}
	if n := count(t, s, "SELECT count(*) FROM contributors WHERE provider IS NULL"); n != 3 {
		t.Errorf("%d contributeurs sans compte, attendu 3", n)
	}

	// provider et provider_user_id sont NULL ensemble ou renseignés ensemble.
	wantCode(t, "provider sans provider_user_id",
		execErr(t, s, insertCtb, model.ProviderGitHub, nil, "x"), sqlite3.SQLITE_CONSTRAINT_CHECK)
	wantCode(t, "provider_user_id sans provider",
		execErr(t, s, insertCtb, nil, 7, "x"), sqlite3.SQLITE_CONSTRAINT_CHECK)
}

func TestContributorEmails(t *testing.T) {
	s := newTestStore(t)
	alice := mustExec(t, s, insertCtb, model.ProviderGitHub, 1, "alice")
	bob := mustExec(t, s, insertCtb, model.ProviderGitHub, 2, "bob")

	mustExec(t, s, insertMail, "alice@example.test", alice)
	mustExec(t, s, insertMail, "alice@perso.example.test", alice)
	// Une adresse désigne une seule personne.
	err := execErr(t, s, insertMail, "alice@example.test", bob)
	if !isUniqueViolation(err) {
		t.Errorf("e-mail en double : %v, attendu une violation d'unicité", err)
	}
	// L'adresse est stockée en minuscules et n'est jamais vide.
	wantCode(t, "e-mail en majuscules",
		execErr(t, s, insertMail, "Bob@Example.test", bob), sqlite3.SQLITE_CONSTRAINT_CHECK)
	wantCode(t, "e-mail vide", execErr(t, s, insertMail, "", bob), sqlite3.SQLITE_CONSTRAINT_CHECK)
	// Contributeur inexistant.
	wantCode(t, "e-mail orphelin",
		execErr(t, s, insertMail, "fantome@example.test", 999), sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY)
}

func TestForeignKeysRejectOrphans(t *testing.T) {
	s := newTestStore(t)
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	branch := mustExec(t, s, insertBr, repo, "main")
	mustExec(t, s, insertCmt, repo, "aaa", nil, nil, testDate, testDate)

	const fk = sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY
	wantCode(t, "branche d'un dépôt inexistant", execErr(t, s, insertBr, 999, "main"), fk)
	wantCode(t, "commit d'un dépôt inexistant",
		execErr(t, s, insertCmt, 999, "bbb", nil, nil, testDate, testDate), fk)
	wantCode(t, "commit d'un auteur inexistant",
		execErr(t, s, insertCmt, repo, "ccc", 999, nil, testDate, testDate), fk)
	wantCode(t, "commit d'un committer inexistant",
		execErr(t, s, insertCmt, repo, "ddd", nil, 999, testDate, testDate), fk)
	wantCode(t, "commit_branches vers un commit inexistant",
		execErr(t, s, insertCB, repo, "inconnu", branch), fk)
	wantCode(t, "commit_branches vers une branche inexistante",
		execErr(t, s, insertCB, repo, "aaa", 999), fk)
	wantCode(t, "sync_runs d'un dépôt inexistant",
		execErr(t, s, insertRun, 999, testDate, string(model.SyncRunning)), fk)

	// Un commit n'est enregistré qu'une fois par dépôt.
	err := execErr(t, s, insertCmt, repo, "aaa", nil, nil, testDate, testDate)
	if !isUniqueViolation(err) {
		t.Errorf("commit en double : %v, attendu une violation de clé primaire", err)
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
		t.Fatalf("DeleteRepository : %v", err)
	}

	for _, table := range []string{"branches", "commits", "commit_branches", "sync_runs"} {
		if n := count(t, s, "SELECT count(*) FROM "+table+" WHERE repo_id = ?", repo); n != 0 {
			t.Errorf("%s : %d lignes restantes pour le dépôt supprimé", table, n)
		}
		if n := count(t, s, "SELECT count(*) FROM "+table+" WHERE repo_id = ?", other); n != 1 {
			t.Errorf("%s : %d lignes pour l'autre dépôt, attendu 1", table, n)
		}
	}
	// Les contributeurs sont partagés entre dépôts : ils restent.
	if n := count(t, s, "SELECT count(*) FROM contributors"); n != 1 {
		t.Errorf("%d contributeurs après suppression du dépôt, attendu 1", n)
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

	// Supprimer une branche retire ses appartenances, pas les commits.
	mustExec(t, s, "DELETE FROM branches WHERE id = ?", dev)
	if n := count(t, s, "SELECT count(*) FROM commit_branches"); n != 1 {
		t.Errorf("%d appartenances après suppression de la branche, attendu 1", n)
	}
	if n := count(t, s, "SELECT count(*) FROM commits"); n != 2 {
		t.Errorf("%d commits après suppression de la branche, attendu 2", n)
	}

	// Supprimer un commit retire ses appartenances.
	mustExec(t, s, "DELETE FROM commits WHERE repo_id = ? AND sha = 'aaa'", repo)
	if n := count(t, s, "SELECT count(*) FROM commit_branches"); n != 0 {
		t.Errorf("%d appartenances après suppression du commit, attendu 0", n)
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
		t.Errorf("commit après suppression des contributeurs : %d ligne(s) avec auteur et committer à NULL, attendu 1", n)
	}
	if n := count(t, s, "SELECT count(*) FROM contributor_emails"); n != 0 {
		t.Errorf("%d e-mails restants après suppression du contributeur, attendu 0", n)
	}
}

func TestDateAndBooleanChecks(t *testing.T) {
	s := newTestStore(t)
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	const check = sqlite3.SQLITE_CONSTRAINT_CHECK

	for _, bad := range []string{
		"2026-10-26 14:03:00",       // espace au lieu de T, sans Z
		"2026-10-26T14:03:00.123Z",  // fraction de seconde
		"2026-10-26T14:03:00+02:00", // décalage horaire
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
