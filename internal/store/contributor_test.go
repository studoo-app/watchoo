package store

// Lectures de la page d'un contributeur (0.5.0) et page de profil des contributeurs (migration 00005).

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

// rankingIDs renvoie l'identifiant des contributeurs auteurs, par nom.
func rankingIDs(t *testing.T, s *Store) map[string]ContributorStats {
	t.Helper()
	ranking, err := s.ContributorRanking(t.Context(), ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]ContributorStats{}
	for _, c := range ranking {
		out[c.Name] = c
	}
	return out
}

func TestContributorProfile(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, _ := viewFixture(t, s)
	ids := rankingIDs(t, s)

	p, err := s.ContributorProfile(ctx, ids["Alice"].ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Alice" || p.ProviderLogin != "alice-gh" || p.ProviderUserID != 10 || p.Emails != nil {
		t.Errorf("contributeur = %+v", p.Contributor)
	}
	if p.Identities != 1 || p.Commits != 3 || p.Additions != 7 || p.Deletions != 3 || p.Repos != 2 {
		t.Errorf("totaux = %+v, attendu 1 identité, 3 commits, +7/-3, 2 dépôts", p)
	}
	if !p.FirstCommitAt.Equal(t0.Add(time.Hour)) || !p.LastCommitAt.Equal(t0.Add(6*time.Hour)) {
		t.Errorf("premier / dernier commit = %v / %v", p.FirstCommitAt, p.LastCommitAt)
	}

	// Contributeur sans commit d'auteur (committer seulement) : totaux nuls, dates zéro.
	c := cw(7, person("Zoé", "zoe@example.test", "", 0), "main")
	c.Committer = person("Fred", "fred@example.test", "", 0)
	applySync(t, s, aliceRepo, []BranchState{{Name: "main", HeadSHA: sha(7)}}, c)
	var fred int64
	if err := s.DB().QueryRowContext(ctx, "SELECT id FROM contributors WHERE name = 'Fred'").Scan(&fred); err != nil {
		t.Fatal(err)
	}
	p, err = s.ContributorProfile(ctx, fred)
	if err != nil {
		t.Fatal(err)
	}
	if p.Commits != 0 || p.Repos != 0 || p.Identities != 1 || !p.FirstCommitAt.IsZero() || !p.LastCommitAt.IsZero() {
		t.Errorf("committer seul = %+v", p)
	}

	if _, err := s.ContributorProfile(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("contributeur inconnu : %v, attendu ErrNotFound", err)
	}
}

func TestContributorRepos(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, bobRepo := viewFixture(t, s)
	ids := rankingIDs(t, s)

	got, err := s.ContributorRepos(ctx, ids["Alice"].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].RepoID != aliceRepo || got[0].Commits != 2 || got[1].RepoID != bobRepo ||
		got[1].Additions != 7 || got[1].Deletions != 3 || got[0].Additions != 0 {
		t.Errorf("ContributorRepos(Alice) = %+v", got)
	}
	for _, g := range got {
		if g.ContributorID != ids["Alice"].ID {
			t.Errorf("ligne d'un autre contributeur : %+v", g)
		}
	}
	none, err := s.ContributorRepos(ctx, 9999)
	if err != nil || none == nil || len(none) != 0 {
		t.Errorf("contributeur inconnu : %v, %v ; attendu liste vide non nil", none, err)
	}
}

func TestCommitFilterAuthorID(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	viewFixture(t, s)
	ids := rankingIDs(t, s)

	list, err := s.ListRecentCommits(ctx, CommitFilter{AuthorID: ids["Bob"].ID})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{sha(4), sha(3)}; !slices.Equal(shasOf(list), want) {
		t.Errorf("commits de Bob = %v, attendu %v", shasOf(list), want)
	}
	page, err := s.CommitPage(ctx, CommitFilter{AuthorID: ids["Alice"].ID}, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || page.Pages != 2 || !slices.Equal(shasOf(page.Commits), []string{sha(6), sha(2)}) {
		t.Errorf("page des commits d'Alice = %+v", page)
	}
	if page.Commits[0].RepoName != "bob" || page.Commits[0].Additions != 7 {
		t.Errorf("commit = %+v", page.Commits[0])
	}
}

func TestContributorCommitTimes(t *testing.T) {
	s := newTestStore(t)
	_, bobRepo := viewFixture(t, s)
	ids := rankingIDs(t, s)

	got, err := s.ContributorCommitTimes(t.Context(), ids["Alice"].ID, t0.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(got, func(a, b model.RepoCommitTime) int { return a.CommittedAt.Compare(b.CommittedAt) })
	want := []model.RepoCommitTime{{RepoID: got[0].RepoID, CommittedAt: t0.Add(2 * time.Hour)},
		{RepoID: bobRepo, CommittedAt: t0.Add(6 * time.Hour)}}
	if !slices.Equal(got, want) || got[0].RepoID == bobRepo {
		t.Errorf("ContributorCommitTimes = %v, attendu %v", got, want)
	}
}

func TestPullRequestFilterAuthor(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	at := t0
	pr := func(n int, author int64) model.PullRequest {
		return model.PullRequest{Number: n, Title: "pr", State: model.PullRequestOpen, AuthorLogin: "x",
			AuthorProviderID: author, CreatedAt: at, UpdatedAt: at.Add(time.Duration(n) * time.Minute)}
	}
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{RepoID: repo, RunID: beginRun(t, s, repo, t0), Metadata: meta("promo", "alice"),
		PullRequests: []model.PullRequest{pr(1, 10), pr(2, 20), pr(3, 10)}, SyncedAt: t0}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListPullRequests(ctx, PullRequestFilter{AuthorProviderID: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Number != 3 || got[1].Number != 1 {
		t.Errorf("PR du compte 10 = %+v", got)
	}
}

func TestContributorProfileURL(t *testing.T) {
	s := newTestStore(t)
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	main := []BranchState{{Name: "main", HeadSHA: sha(3)}}
	alice := person("Alice", "alice@example.test", "alice-gh", 10)

	// Premier commit sans adresse (contributeur déjà en base avant 0.5.0), puis avec : remplie.
	applySync(t, s, repo, main, cw(1, alice, "main"))
	if got := rankingIDs(t, s)["Alice"].ProfileURL; got != "" {
		t.Errorf("profil = %q, attendu vide", got)
	}
	withURL := alice
	withURL.ProfileURL = "https://forge.example.test/alice-gh"
	applySync(t, s, repo, main, cw(2, withURL, "main"))
	if got := rankingIDs(t, s)["Alice"].ProfileURL; got != withURL.ProfileURL {
		t.Errorf("profil = %q, attendu %q", got, withURL.ProfileURL)
	}
	// Une identité sans adresse ne l'efface pas.
	applySync(t, s, repo, main, cw(3, alice, "main"))
	stats := rankingIDs(t, s)["Alice"]
	if stats.ProfileURL != withURL.ProfileURL {
		t.Errorf("profil effacé : %q", stats.ProfileURL)
	}
	list, err := s.ListRecentCommits(t.Context(), CommitFilter{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if list[0].AuthorProfileURL != withURL.ProfileURL {
		t.Errorf("commit : AuthorProfileURL = %q", list[0].AuthorProfileURL)
	}
	sums, err := s.RepositorySummaries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if sums[0].LastCommit.AuthorProfileURL != withURL.ProfileURL {
		t.Errorf("dernier commit : AuthorProfileURL = %q", sums[0].LastCommit.AuthorProfileURL)
	}
	p, err := s.ContributorProfile(t.Context(), stats.ID)
	if err != nil || p.ProfileURL != withURL.ProfileURL {
		t.Errorf("fiche : %q, %v", p.ProfileURL, err)
	}
}

// TestMigrationProfileURL applique la migration 00005 sur une base au schéma 00004 qui a des
// contributeurs : aucun n'est perdu, l'adresse est vide (aucune n'est devinée), et le retour arrière
// retire la colonne.
func TestMigrationProfileURL(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 4); err != nil {
		t.Fatalf("UpTo(4) : %v", err)
	}
	mustExec(t, s, `INSERT INTO contributors (provider, provider_user_id, provider_login, name) VALUES ('github', 10, 'alice-gh', 'Alice')`)
	mustExec(t, s, `INSERT INTO contributors (name) VALUES ('Bob')`)
	if _, err := p.UpTo(ctx, 5); err != nil {
		t.Fatalf("UpTo(5) : %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM contributors WHERE profile_url = ''"); got != 2 {
		t.Errorf("%d contributeurs à profile_url vide, attendu 2", got)
	}
	if _, err := p.DownTo(ctx, 4); err != nil {
		t.Fatalf("DownTo(4) : %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM pragma_table_info('contributors') WHERE name = 'profile_url'"); got != 0 {
		t.Error("retour arrière : colonne profile_url toujours là")
	}
	if got := count(t, s, "SELECT count(*) FROM contributors"); got != 2 {
		t.Errorf("retour arrière : %d contributeurs, attendu 2", got)
	}
}
