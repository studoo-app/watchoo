package store

// Tests d'ApplyRepoSync et des lectures de la synchro (données factices uniquement).

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// t0 est une date de référence fixe des tests.
var t0 = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

// sha fabrique un SHA factice de 40 caractères à partir de n.
func sha(n int) string {
	return fmt.Sprintf("%040x", n)
}

// beginRun ouvre un sync_run « running » pour repoID et renvoie son identifiant.
func beginRun(t *testing.T, s *Store, repoID int64, now time.Time) int64 {
	t.Helper()
	run, err := s.BeginSyncRun(t.Context(), repoID, now, 30*time.Minute)
	if err != nil {
		t.Fatalf("BeginSyncRun : %v", err)
	}
	return run.ID
}

// meta renvoie des métadonnées factices du dépôt.
func meta(owner, name string) RepositoryMetadata {
	return RepositoryMetadata{
		Owner: owner, Name: name, DefaultBranch: "main",
		URL: "https://example.test/" + owner + "/" + name, Stars: 3, OpenIssues: 1,
	}
}

// person renvoie une identité factice avec compte (uid != 0) ou sans compte.
func person(name, email, login string, uid int64) Identity {
	return Identity{Name: name, Email: email, Login: login, ProviderUserID: uid}
}

// cw fabrique un commit factice n, commité à t0 + n heures, avec le même auteur et committer.
func cw(n int, who Identity, branches ...string) CommitWrite {
	at := t0.Add(time.Duration(n) * time.Hour)
	return CommitWrite{
		SHA: sha(n), Message: fmt.Sprintf("message %d", n), URL: "https://example.test/c/" + sha(n),
		Author: who, Committer: who, AuthoredAt: at, CommittedAt: at, Branches: branches,
	}
}

// applySync ouvre un sync_run puis applique une synchro avec les branches et commits donnés.
func applySync(t *testing.T, s *Store, repoID int64, branches []BranchState, commits ...CommitWrite) RepoSyncResult {
	t.Helper()
	now := t0.Add(1000 * time.Hour)
	res, err := s.ApplyRepoSync(t.Context(), RepoSyncWrite{
		RepoID: repoID, RunID: beginRun(t, s, repoID, now), Metadata: meta("promo", "alice"),
		Branches: branches, Commits: commits, SyncedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ApplyRepoSync : %v", err)
	}
	return res
}

func TestApplyRepoSyncFirstImport(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice-old")
	alice := person("Alice", "Alice@Example.TEST", "alice", 501)

	now := t0.Add(48 * time.Hour)
	runID := beginRun(t, s, repoID, now)
	c1, c2 := cw(1, alice, "main"), cw(2, alice, "main")
	c1.Additions, c1.Deletions = 10, 2
	res, err := s.ApplyRepoSync(ctx, RepoSyncWrite{
		RepoID: repoID, RunID: runID, Metadata: meta("Promo2", "alice"),
		Branches: []BranchState{{Name: "main", HeadSHA: sha(2)}},
		Commits:  []CommitWrite{c2, c1}, SyncedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ApplyRepoSync : %v", err)
	}
	if res.NewCommits != 2 || res.TrackedBranches != 1 || !res.LastCommitAt.Equal(c2.CommittedAt) {
		t.Errorf("résultat %+v, attendu 2 nouveaux, 1 branche, dernier commit %v", res, c2.CommittedAt)
	}

	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Owner != "Promo2" || repo.Name != "alice" || repo.DefaultBranch != "main" || repo.Stars != 3 ||
		repo.OpenIssues != 1 || !repo.LastSyncedAt.Equal(now.Add(time.Minute)) || !repo.LastCommitAt.Equal(c2.CommittedAt) {
		t.Errorf("dépôt après synchro : %+v", repo)
	}

	run, err := s.LastSyncRun(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if run.ID != runID || run.Status != model.SyncSuccess || run.NewCommits != 2 || !run.FinishedAt.Equal(now.Add(time.Minute)) {
		t.Errorf("sync_run après synchro : %+v", run)
	}

	n, err := s.CountCommits(ctx, repoID)
	if err != nil || n != 2 {
		t.Errorf("CountCommits = %d, %v ; attendu 2", n, err)
	}
	if got := count(t, s, "SELECT count(*) FROM contributors"); got != 1 {
		t.Errorf("%d contributeurs, attendu 1 (auteur et committer identiques)", got)
	}
	var email string
	if err := s.DB().QueryRowContext(ctx, "SELECT email FROM contributor_emails").Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email != "alice@example.test" {
		t.Errorf("e-mail enregistré %q, attendu en minuscules", email)
	}
	var adds, dels int
	if err := s.DB().QueryRowContext(ctx, "SELECT additions, deletions FROM commits WHERE sha = ?", sha(1)).Scan(&adds, &dels); err != nil {
		t.Fatal(err)
	}
	if adds != 10 || dels != 2 {
		t.Errorf("statistiques %d/%d, attendu 10/2", adds, dels)
	}
}

func TestApplyRepoSyncIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "", 0)
	main := []BranchState{{Name: "main", HeadSHA: sha(2)}}

	applySync(t, s, repoID, main, cw(1, alice, "main"), cw(2, alice, "main"))

	// Le même commit revient avec d'autres données : il n'est pas modifié.
	changed := cw(1, person("Autre", "autre@example.test", "", 0), "main")
	changed.Message = "modifié"
	changed.Additions = 99
	res := applySync(t, s, repoID, main, changed, cw(2, alice, "main"))
	if res.NewCommits != 0 {
		t.Errorf("NewCommits = %d, attendu 0", res.NewCommits)
	}
	var msg string
	var adds int
	if err := s.DB().QueryRowContext(ctx, "SELECT message, additions FROM commits WHERE sha = ?", sha(1)).Scan(&msg, &adds); err != nil {
		t.Fatal(err)
	}
	if msg != "message 1" || adds != 0 {
		t.Errorf("commit déjà en base modifié : %q, %d", msg, adds)
	}
	if got := count(t, s, "SELECT count(*) FROM contributors"); got != 1 {
		t.Errorf("%d contributeurs, attendu 1 (identités des commits connus ignorées)", got)
	}
	if got := count(t, s, "SELECT count(*) FROM commit_branches"); got != 2 {
		t.Errorf("%d lignes commit_branches, attendu 2 (sans doublon)", got)
	}
	run, _ := s.LastSyncRun(ctx, repoID)
	if run.NewCommits != 0 || run.Status != model.SyncSuccess {
		t.Errorf("dernier sync_run %+v", run)
	}
}

func TestApplyRepoSyncCommitBranches(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	bob := person("Bob", "bob@example.test", "", 0)
	both := []BranchState{{Name: "main", HeadSHA: sha(1)}, {Name: "dev", HeadSHA: sha(2)}}

	// Un SHA en double dans la liste : branches réunies.
	applySync(t, s, repoID, both, cw(1, bob, "main"), cw(1, bob, "dev"), cw(2, bob, "dev"))
	if got := count(t, s, "SELECT count(*) FROM commits"); got != 2 {
		t.Errorf("%d commits, attendu 2", got)
	}
	if got := count(t, s, "SELECT count(*) FROM commit_branches WHERE sha = ?", sha(1)); got != 2 {
		t.Errorf("commit 1 sur %d branches, attendu 2", got)
	}

	// Un commit connu vu sur une nouvelle branche : seule la ligne commit_branches est ajoutée.
	three := append(slices.Clone(both), BranchState{Name: "feature", HeadSHA: sha(2)})
	res := applySync(t, s, repoID, three, cw(2, bob, "feature"))
	if res.NewCommits != 0 || res.TrackedBranches != 3 {
		t.Errorf("résultat %+v", res)
	}
	if got := count(t, s, "SELECT count(*) FROM commit_branches WHERE sha = ?", sha(2)); got != 2 {
		t.Errorf("commit 2 sur %d branches, attendu 2", got)
	}

	branches, err := s.ListBranches(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range branches {
		names = append(names, b.Name)
	}
	if !slices.Equal(names, []string{"dev", "feature", "main"}) {
		t.Errorf("branches %v, attendu triées par nom", names)
	}
}

func TestApplyRepoSyncUntracksBranches(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	bob := person("Bob", "bob@example.test", "", 0)

	applySync(t, s, repoID,
		[]BranchState{{Name: "main", HeadSHA: sha(1)}, {Name: "dev", HeadSHA: sha(5)}},
		cw(1, bob, "main"), cw(5, bob, "dev"))
	repo, _ := s.GetRepository(ctx, repoID)
	if !repo.LastCommitAt.Equal(t0.Add(5 * time.Hour)) {
		t.Errorf("last_commit_at %v, attendu celui du commit de dev", repo.LastCommitAt)
	}

	// dev disparaît : elle n'est plus suivie, son historique reste, last_commit_at ne compte plus ses commits.
	res := applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(1)}})
	if res.TrackedBranches != 1 || !res.LastCommitAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("résultat %+v", res)
	}
	branches, _ := s.ListBranches(ctx, repoID)
	for _, b := range branches {
		if want := b.Name == "main"; b.Tracked != want {
			t.Errorf("branche %s : tracked = %v, attendu %v", b.Name, b.Tracked, want)
		}
	}
	if got := count(t, s, "SELECT count(*) FROM commits"); got != 2 {
		t.Errorf("%d commits, attendu 2 (historique gardé)", got)
	}

	// dev revient : elle est de nouveau suivie (même ligne, tête mise à jour).
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(1)}, {Name: "dev", HeadSHA: sha(6)}},
		cw(6, bob, "dev"))
	branches, _ = s.ListBranches(ctx, repoID)
	if len(branches) != 2 || !branches[0].Tracked || branches[0].HeadSHA != sha(6) {
		t.Errorf("branches après retour de dev : %+v", branches)
	}
}

func TestApplyRepoSyncEmptyRepository(t *testing.T) {
	s := newTestStore(t)
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	res := applySync(t, s, repoID, nil)
	if res.NewCommits != 0 || res.TrackedBranches != 0 || !res.LastCommitAt.IsZero() {
		t.Errorf("résultat %+v pour un dépôt vide", res)
	}
	repo, _ := s.GetRepository(t.Context(), repoID)
	if repo.LastSyncedAt.IsZero() || !repo.LastCommitAt.IsZero() {
		t.Errorf("dépôt vide après synchro : %+v", repo)
	}
}

func TestApplyRepoSyncIdentityRules(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	main := []BranchState{{Name: "main", HeadSHA: sha(9)}}

	anon := person("Anonyme", "", "", 0)
	c1 := cw(1, anon, "main")
	// Auteur sans compte, committer avec compte (ex. merge via l'interface de GitHub).
	c2 := cw(2, person("Carol", "carol@example.test", "", 0), "main")
	c2.Committer = Identity{Name: "GitHub", Email: "noreply@example.test", Login: "web-flow", ProviderUserID: 900, AvatarURL: "https://example.test/a.png"}
	applySync(t, s, repoID, main, c1, c2)

	var author, committer *int64
	if err := s.DB().QueryRowContext(ctx, "SELECT author_id, committer_id FROM commits WHERE sha = ?", sha(1)).Scan(&author, &committer); err != nil {
		t.Fatal(err)
	}
	if author != nil || committer != nil {
		t.Errorf("ni compte ni e-mail : author_id %v, committer_id %v, attendu NULL", author, committer)
	}
	if got := count(t, s, "SELECT count(*) FROM contributors WHERE name = 'Anonyme'"); got != 0 {
		t.Error("un contributeur a été créé pour une identité sans compte ni e-mail")
	}
	if got := count(t, s, "SELECT count(*) FROM contributors"); got != 2 {
		t.Errorf("%d contributeurs, attendu 2", got)
	}
	if got := count(t, s, `SELECT count(*) FROM contributors
		WHERE provider = 'github' AND provider_user_id = 900 AND provider_login = 'web-flow'
		AND avatar_url = 'https://example.test/a.png'`); got != 1 {
		t.Error("committer avec compte mal enregistré")
	}
	if got := count(t, s, "SELECT count(*) FROM contributors WHERE provider IS NULL AND name = 'Carol'"); got != 1 {
		t.Error("auteur sans compte mal enregistré")
	}
}

func TestApplyRepoSyncMergesNoAccountContributor(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	main := []BranchState{{Name: "main", HeadSHA: sha(9)}}

	// Synchro 1 : Dave committe sans compte lié, avec deux adresses (deux contributeurs sans compte),
	// et un compte Erin possède une autre adresse.
	c1 := cw(1, person("Dave", "dave@school.test", "", 0), "main")
	c2 := cw(2, person("", "Dave@Perso.test", "", 0), "main")
	c3 := cw(3, person("Erin", "erin@example.test", "erin", 77), "main")
	applySync(t, s, repoID, main, c1, c2, c3)
	if got := count(t, s, "SELECT count(*) FROM contributors"); got != 3 {
		t.Fatalf("%d contributeurs, attendu 3", got)
	}

	// Synchro 2 : deux commits liés au compte de Dave, l'un avec l'adresse de l'école, l'autre avec
	// l'adresse perso : les deux contributeurs sans compte sont fusionnés dans le compte.
	c4 := cw(4, person("Dave D.", "DAVE@school.test", "dave", 42), "main")
	c4.Author.AvatarURL = "https://example.test/dave.png"
	c5 := cw(5, person("Dave D.", "dave@perso.test", "dave", 42), "main")
	res := applySync(t, s, repoID, main, c4, c5)
	if res.NewCommits != 2 {
		t.Errorf("NewCommits = %d, attendu 2", res.NewCommits)
	}

	if got := count(t, s, "SELECT count(*) FROM contributors"); got != 2 {
		t.Errorf("%d contributeurs après fusion, attendu 2 (Dave, Erin)", got)
	}
	var daveID int64
	if err := s.DB().QueryRowContext(ctx, `SELECT id FROM contributors
		WHERE provider = 'github' AND provider_user_id = 42 AND name = 'Dave D.' AND provider_login = 'dave'
		AND avatar_url = 'https://example.test/dave.png'`).Scan(&daveID); err != nil {
		t.Fatalf("contributeur avec compte de Dave : %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM contributor_emails WHERE contributor_id = ?", daveID); got != 2 {
		t.Errorf("Dave a %d adresses, attendu 2", got)
	}
	if got := count(t, s, "SELECT count(*) FROM commits WHERE author_id = ? AND committer_id = ?", daveID, daveID); got != 4 {
		t.Errorf("%d commits réattribués à Dave, attendu 4", got)
	}

	// Une adresse tenue par un autre compte est simplement réattribuée (Erin n'est pas fusionnée).
	c6 := cw(6, person("Dave D.", "erin@example.test", "dave", 42), "main")
	applySync(t, s, repoID, main, c6)
	if got := count(t, s, "SELECT count(*) FROM contributors"); got != 2 {
		t.Errorf("%d contributeurs, attendu 2 (aucune fusion entre comptes)", got)
	}
	if got := count(t, s, "SELECT count(*) FROM contributor_emails WHERE email = 'erin@example.test' AND contributor_id = ?", daveID); got != 1 {
		t.Error("adresse non réattribuée au dernier compte qui l'utilise")
	}
	if got := count(t, s, "SELECT count(*) FROM commits WHERE sha = ? AND author_id <> ?", sha(3), daveID); got != 1 {
		t.Error("les commits d'Erin ne doivent pas changer d'auteur")
	}
}

func TestApplyRepoSyncMergeWithinOneSync(t *testing.T) {
	s := newTestStore(t)
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	main := []BranchState{{Name: "main", HeadSHA: sha(9)}}
	noAccount := person("Fay", "fay@example.test", "", 0)
	withAccount := person("", "fay@example.test", "fay", 13)

	// Ordre par date : sans compte, puis avec compte (fusion), puis de nouveau sans compte : le
	// troisième commit doit viser le contributeur avec compte, pas l'ancien supprimé.
	applySync(t, s, repoID, main, cw(3, noAccount, "main"), cw(1, noAccount, "main"), cw(2, withAccount, "main"))
	if got := count(t, s, "SELECT count(*) FROM contributors"); got != 1 {
		t.Errorf("%d contributeurs, attendu 1", got)
	}
	if got := count(t, s, `SELECT count(*) FROM commits c JOIN contributors ct ON ct.id = c.author_id
		WHERE ct.provider_user_id = 13 AND ct.name = 'Fay'`); got != 3 {
		t.Errorf("%d commits rattachés au compte (avec le nom repris), attendu 3", got)
	}
}

func TestApplyRepoSyncLatestNameWins(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	main := []BranchState{{Name: "main", HeadSHA: sha(9)}}
	applySync(t, s, repoID, main,
		cw(3, person("Nom récent", "gil@example.test", "", 0), "main"),
		cw(1, person("Ancien nom", "gil@example.test", "", 0), "main"),
		cw(2, person("Nom récent", "gil@example.test", "", 0), "main"))
	var name string
	if err := s.DB().QueryRowContext(ctx, "SELECT name FROM contributors").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Nom récent" {
		t.Errorf("nom %q, attendu celui du commit le plus récent", name)
	}

	// Un compte change de login : mis à jour, sans doublon.
	applySync(t, s, repoID, main, cw(4, person("Hugo", "", "hugo-old", 5), "main"))
	applySync(t, s, repoID, main, cw(5, person("", "", "hugo-new", 5), "main"))
	if got := count(t, s, "SELECT count(*) FROM contributors WHERE provider_user_id = 5 AND provider_login = 'hugo-new' AND name = 'Hugo'"); got != 1 {
		t.Error("login non mis à jour, ou nom effacé par un nom vide")
	}
}

func TestApplyRepoSyncRollsBackOnError(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	mustCreateRepo(t, s, 2, "promo", "bob")
	now := t0.Add(time.Hour)
	runID := beginRun(t, s, repoID, now)

	// owner/name déjà pris par un autre dépôt : ErrAlreadyExists, rien n'est écrit.
	_, err := s.ApplyRepoSync(ctx, RepoSyncWrite{
		RepoID: repoID, RunID: runID, Metadata: meta("promo", "BOB"),
		Branches: []BranchState{{Name: "main", HeadSHA: sha(1)}},
		Commits:  []CommitWrite{cw(1, person("Ida", "ida@example.test", "", 0), "main")},
		SyncedAt: now,
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("erreur %v, attendu ErrAlreadyExists", err)
	}
	for _, table := range []string{"branches", "commits", "contributors", "contributor_emails", "commit_branches"} {
		if got := count(t, s, "SELECT count(*) FROM "+table); got != 0 {
			t.Errorf("table %s : %d lignes après échec, attendu 0", table, got)
		}
	}
	run, _ := s.LastSyncRun(ctx, repoID)
	if run.Status != model.SyncRunning {
		t.Errorf("sync_run %s après échec, attendu running", run.Status)
	}
	repo, _ := s.GetRepository(ctx, repoID)
	if repo.Name != "alice" || !repo.LastSyncedAt.IsZero() {
		t.Errorf("dépôt modifié malgré l'échec : %+v", repo)
	}

	// La base reste utilisable : la connexion n'est pas restée en transaction.
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{
		RepoID: repoID, RunID: runID, Metadata: meta("promo", "alice"), SyncedAt: now,
	}); err != nil {
		t.Errorf("synchro après un échec : %v", err)
	}
}

func TestApplyRepoSyncValidation(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	now := t0
	runID := beginRun(t, s, repoID, now)
	who := person("Jo", "jo@example.test", "", 0)
	main := []BranchState{{Name: "main", HeadSHA: sha(1)}}
	valid := func() RepoSyncWrite {
		return RepoSyncWrite{RepoID: repoID, RunID: runID, Metadata: meta("promo", "alice"), Branches: main, SyncedAt: now}
	}

	tests := map[string]func(w *RepoSyncWrite){
		"branche inconnue": func(w *RepoSyncWrite) { w.Commits = []CommitWrite{cw(1, who, "dev")} },
		"SHA vide":         func(w *RepoSyncWrite) { c := cw(1, who, "main"); c.SHA = ""; w.Commits = []CommitWrite{c} },
		"date absente": func(w *RepoSyncWrite) {
			c := cw(1, who, "main")
			c.CommittedAt = time.Time{}
			w.Commits = []CommitWrite{c}
		},
		"branche en double":   func(w *RepoSyncWrite) { w.Branches = append(slices.Clone(main), main...) },
		"métadonnées vides":   func(w *RepoSyncWrite) { w.Metadata = RepositoryMetadata{} },
		"fin absente":         func(w *RepoSyncWrite) { w.SyncedAt = time.Time{} },
		"synchro inexistante": func(w *RepoSyncWrite) { w.RunID = 9999 },
		"dépôt inexistant":    func(w *RepoSyncWrite) { w.RepoID = 9999 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			w := valid()
			mutate(&w)
			if _, err := s.ApplyRepoSync(ctx, w); err == nil {
				t.Fatal("erreur attendue")
			} else if strings.Contains(err.Error(), "jo@example.test") || strings.Contains(err.Error(), "Jo") {
				t.Errorf("l'erreur contient une donnée personnelle : %v", err)
			}
		})
	}
	if got := count(t, s, "SELECT count(*) FROM commits"); got != 0 {
		t.Errorf("%d commits écrits malgré les erreurs", got)
	}
	// La synchro inexistante et le dépôt inexistant sont signalés par ErrNotFound.
	w := valid()
	w.RunID = 9999
	if _, err := s.ApplyRepoSync(ctx, w); !errors.Is(err, ErrNotFound) {
		t.Errorf("synchro inexistante : %v, attendu ErrNotFound", err)
	}
	w = valid()
	w.RepoID = 9999
	if _, err := s.ApplyRepoSync(ctx, w); !errors.Is(err, ErrNotFound) {
		t.Errorf("dépôt inexistant : %v, attendu ErrNotFound", err)
	}
}

func TestExistingCommitSHAs(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")
	who := person("Kim", "kim@example.test", "", 0)
	main := []BranchState{{Name: "main", HeadSHA: sha(1)}}

	// Plus d'un paquet de SHA.
	var commits []CommitWrite
	for i := range 1200 {
		if i%2 == 0 {
			commits = append(commits, cw(i, who, "main"))
		}
	}
	applySync(t, s, repoID, main, commits...)

	var shas []string
	for i := range 1200 {
		shas = append(shas, sha(i), sha(i)) // doublons permis
	}
	known, err := s.ExistingCommitSHAs(ctx, repoID, shas)
	if err != nil {
		t.Fatal(err)
	}
	if len(known) != 600 || !known[sha(0)] || known[sha(1)] || !known[sha(1198)] {
		t.Errorf("%d SHA connus, attendu 600 (pairs seulement)", len(known))
	}
	known, err = s.ExistingCommitSHAs(ctx, other, shas)
	if err != nil || len(known) != 0 {
		t.Errorf("autre dépôt : %d SHA connus, %v ; attendu aucun", len(known), err)
	}
	known, err = s.ExistingCommitSHAs(ctx, repoID, nil)
	if err != nil || known == nil || len(known) != 0 {
		t.Errorf("liste vide : %v, %v ; attendu une map vide non nil", known, err)
	}
}

func TestListBranchesAndCountCommitsEmpty(t *testing.T) {
	s := newTestStore(t)
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	branches, err := s.ListBranches(t.Context(), repoID)
	if err != nil || branches == nil || len(branches) != 0 {
		t.Errorf("ListBranches = %v, %v ; attendu une liste vide non nil", branches, err)
	}
	if n, err := s.CountCommits(t.Context(), repoID); err != nil || n != 0 {
		t.Errorf("CountCommits = %d, %v", n, err)
	}
}

func TestDeleteRepositoryAfterSyncCascades(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(1)}},
		cw(1, person("Lou", "lou@example.test", "", 0), "main"))
	if err := s.DeleteRepository(ctx, repoID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"branches", "commits", "commit_branches", "sync_runs"} {
		if got := count(t, s, "SELECT count(*) FROM "+table); got != 0 {
			t.Errorf("table %s : %d lignes après suppression du dépôt", table, got)
		}
	}
	// Les contributeurs sont partagés entre dépôts : ils restent.
	if got := count(t, s, "SELECT count(*) FROM contributors"); got != 1 {
		t.Errorf("%d contributeurs, attendu 1", got)
	}
}
