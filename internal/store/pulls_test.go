package store

// Tests des pull requests, releases et tags (V2) : contraintes du schéma (00003), écriture par
// ApplyRepoSync (création, mise à jour, remplacement, conservation), lectures et résumé des dépôts.

import (
	"errors"
	"testing"
	"time"

	sqlite3 "modernc.org/sqlite/lib"

	"github.com/studoo-app/watchoo/internal/model"
)

const (
	insertPR = `INSERT INTO pull_requests (repo_id, number, state, created_at, updated_at, merged_at)
		VALUES (?, ?, ?, ?, ?, ?)`
	insertRelease = "INSERT INTO releases (repo_id, provider_id, tag_name, created_at) VALUES (?, ?, ?, ?)"
	insertTag     = "INSERT INTO tags (repo_id, name, sha) VALUES (?, ?, ?)"
)

func TestPullRequestsSchema(t *testing.T) {
	s := newTestStore(t)
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")
	const check, fk = sqlite3.SQLITE_CONSTRAINT_CHECK, sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY

	mustExec(t, s, insertPR, repo, 1, "open", testDate, testDate, nil)
	if err := execErr(t, s, insertPR, repo, 1, "closed", testDate, testDate, nil); !isUniqueViolation(err) {
		t.Errorf("PR en double : %v, attendu une violation de clé primaire", err)
	}
	mustExec(t, s, insertPR, other, 1, "closed", testDate, testDate, testDate) // même numéro, autre dépôt
	wantCode(t, "état merged", execErr(t, s, insertPR, repo, 2, "merged", testDate, testDate, nil), check)
	wantCode(t, "numéro 0", execErr(t, s, insertPR, repo, 0, "open", testDate, testDate, nil), check)
	wantCode(t, "date mal formée", execErr(t, s, insertPR, repo, 3, "open", "2026-10-26 14:03:00", testDate, nil), check)
	wantCode(t, "merged_at mal formée", execErr(t, s, insertPR, repo, 4, "closed", testDate, testDate, "hier"), check)
	wantCode(t, "PR d'un dépôt inexistant", execErr(t, s, insertPR, 999, 1, "open", testDate, testDate, nil), fk)

	mustExec(t, s, insertRelease, repo, 10, "v1", testDate)
	if err := execErr(t, s, insertRelease, repo, 10, "v1-bis", testDate); !isUniqueViolation(err) {
		t.Errorf("release en double : %v", err)
	}
	wantCode(t, "release sans tag", execErr(t, s, insertRelease, repo, 11, "", testDate), check)
	wantCode(t, "release d'identifiant 0", execErr(t, s, insertRelease, repo, 0, "v0", testDate), check)
	wantCode(t, "release mal datée", execErr(t, s, insertRelease, repo, 12, "v2", "2026-10-26"), check)

	mustExec(t, s, insertTag, repo, "v1", sha(1))
	if err := execErr(t, s, insertTag, repo, "v1", sha(2)); !isUniqueViolation(err) {
		t.Errorf("tag en double : %v", err)
	}
	wantCode(t, "tag sans nom", execErr(t, s, insertTag, repo, "", sha(1)), check)
	wantCode(t, "tag d'un dépôt inexistant", execErr(t, s, insertTag, 999, "v1", sha(1)), fk)

	// Supprimer le dépôt supprime ses PR, releases et tags, pas ceux des autres.
	if err := s.DeleteRepository(t.Context(), repo); err != nil {
		t.Fatalf("DeleteRepository : %v", err)
	}
	for _, table := range []string{"pull_requests", "releases", "tags"} {
		if n := count(t, s, "SELECT count(*) FROM "+table+" WHERE repo_id = ?", repo); n != 0 {
			t.Errorf("%s : %d lignes restantes pour le dépôt supprimé", table, n)
		}
	}
	if n := count(t, s, "SELECT count(*) FROM pull_requests WHERE repo_id = ?", other); n != 1 {
		t.Errorf("PR de l'autre dépôt : %d, attendu 1", n)
	}
}

// pr fabrique une pull request factice n, modifiée à t0 + h heures.
func pr(n int, state string, h int) model.PullRequest {
	at := t0.Add(time.Duration(h) * time.Hour)
	p := model.PullRequest{
		Number: n, Title: "titre", State: state, AuthorLogin: "alice", AuthorProviderID: 501,
		URL: "https://example.test/pull", BaseBranch: "main", HeadBranch: "feature",
		CreatedAt: t0, UpdatedAt: at,
	}
	if state == model.PullRequestClosed {
		p.ClosedAt = at
	}
	return p
}

// applyExtras applique une synchro sans commit avec les PR, releases et tags donnés.
func applyExtras(t *testing.T, s *Store, repoID int64, w RepoSyncWrite) error {
	t.Helper()
	now := t0.Add(1000 * time.Hour)
	w.RepoID, w.RunID, w.Metadata, w.SyncedAt = repoID, beginRun(t, s, repoID, now), meta("promo", "alice"), now
	_, err := s.ApplyRepoSync(t.Context(), w)
	return err
}

func TestApplyRepoSyncPullRequestsReleasesTags(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")

	if last, err := s.LatestPullRequestUpdate(ctx, repoID); err != nil || !last.IsZero() {
		t.Fatalf("LatestPullRequestUpdate sans PR : %v, %v", last, err)
	}
	merged := pr(2, model.PullRequestClosed, 3)
	merged.Merged, merged.MergedAt = true, merged.UpdatedAt
	draft := pr(3, model.PullRequestOpen, 2)
	draft.Draft = true
	err := applyExtras(t, s, repoID, RepoSyncWrite{
		PullRequests:    []model.PullRequest{pr(1, model.PullRequestOpen, 1), merged, draft, pr(4, model.PullRequestClosed, 4)},
		ReplaceReleases: true,
		Releases: []model.Release{
			{ProviderID: 10, TagName: "v1", CreatedAt: t0, PublishedAt: t0.Add(time.Hour)},
			{ProviderID: 11, TagName: "v2", CreatedAt: t0.Add(5 * time.Hour), Draft: true},
			{ProviderID: 10, TagName: "v1", CreatedAt: t0}, // doublon ignoré
		},
		ReplaceTags: true,
		Tags:        []model.Tag{{Name: "v1", SHA: sha(1)}, {Name: "v0", SHA: sha(99)}},
	})
	if err != nil {
		t.Fatalf("ApplyRepoSync : %v", err)
	}
	if last, err := s.LatestPullRequestUpdate(ctx, repoID); err != nil || !last.Equal(t0.Add(4*time.Hour)) {
		t.Errorf("LatestPullRequestUpdate = %v, %v", last, err)
	}
	counts, err := s.PullRequestCounts(ctx, repoID)
	if err != nil || counts != (PullRequestCounts{Open: 2, Merged: 1, Closed: 1}) {
		t.Errorf("PullRequestCounts = %+v, %v", counts, err)
	}

	for _, tt := range []struct {
		status model.PullRequestStatus
		want   []int
	}{
		{"", []int{4, 2, 3, 1}},
		{model.PullRequestStatusOpen, []int{3, 1}},
		{model.PullRequestStatusMerged, []int{2}},
		{model.PullRequestStatusClosed, []int{4}},
	} {
		views, err := s.ListPullRequests(ctx, PullRequestFilter{RepoID: repoID, Status: tt.status})
		if err != nil {
			t.Fatalf("ListPullRequests(%q) : %v", tt.status, err)
		}
		var got []int
		for _, v := range views {
			got = append(got, v.Number)
			if v.RepoDisplayName() != "promo/alice" {
				t.Errorf("dépôt %q", v.RepoDisplayName())
			}
		}
		if !equalInts(got, tt.want) {
			t.Errorf("ListPullRequests(%q) = %v, attendu %v", tt.status, got, tt.want)
		}
	}
	if _, err := s.ListPullRequests(ctx, PullRequestFilter{Status: "autre"}); err == nil {
		t.Error("état inconnu accepté")
	}
	views, _ := s.ListPullRequests(ctx, PullRequestFilter{Status: model.PullRequestStatusMerged})
	if len(views) != 1 || views[0].Status() != model.PullRequestStatusMerged || !views[0].MergedAt.Equal(merged.MergedAt) ||
		views[0].AuthorLogin != "alice" || views[0].AuthorProviderID != 501 || views[0].HeadBranch != "feature" {
		t.Errorf("PR mergée relue : %+v", views)
	}
	if limited, _ := s.ListPullRequests(ctx, PullRequestFilter{Limit: 1}); len(limited) != 1 || limited[0].Number != 4 {
		t.Errorf("Limit 1 : %+v", limited)
	}

	releases, err := s.ListReleases(ctx, ReleaseFilter{RepoID: repoID})
	if err != nil || len(releases) != 2 || releases[0].ProviderID != 11 || !releases[1].Date().Equal(t0.Add(time.Hour)) {
		t.Errorf("ListReleases = %+v, %v", releases, err)
	}
	tags, err := s.ListTags(ctx, ReleaseFilter{RepoID: repoID})
	if err != nil || len(tags) != 2 || tags[0].Name != "v1" || !tags[0].CommittedAt.IsZero() {
		t.Errorf("ListTags (commits inconnus) = %+v, %v", tags, err)
	}

	// Mise à jour d'une PR, releases et tags inchangés quand ils ne sont pas relus.
	reopened := pr(4, model.PullRequestOpen, 6)
	reopened.Title = "nouveau titre"
	if err := applyExtras(t, s, repoID, RepoSyncWrite{PullRequests: []model.PullRequest{reopened}}); err != nil {
		t.Fatalf("ApplyRepoSync (mise à jour) : %v", err)
	}
	views, _ = s.ListPullRequests(ctx, PullRequestFilter{RepoID: repoID, Limit: 1})
	if len(views) != 1 || views[0].Number != 4 || views[0].Title != "nouveau titre" || views[0].State != model.PullRequestOpen ||
		!views[0].ClosedAt.IsZero() {
		t.Errorf("PR mise à jour : %+v", views)
	}
	if n := count(t, s, "SELECT count(*) FROM pull_requests"); n != 4 {
		t.Errorf("%d PR, attendu 4 (aucune supprimée)", n)
	}
	if n := count(t, s, "SELECT count(*) FROM releases"); n != 2 {
		t.Errorf("%d releases, attendu 2 (inchangées)", n)
	}

	// Remplacement : une release et un tag supprimés sur la forge disparaissent ; tag daté par son commit.
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(1)}}, cw(1, person("A", "a@example.test", "", 0), "main"))
	if err := applyExtras(t, s, repoID, RepoSyncWrite{
		Branches:        []BranchState{{Name: "main", HeadSHA: sha(1)}},
		ReplaceReleases: true, Releases: []model.Release{},
		ReplaceTags: true, Tags: []model.Tag{{Name: "v1", SHA: sha(1)}},
	}); err != nil {
		t.Fatalf("ApplyRepoSync (remplacement) : %v", err)
	}
	if n := count(t, s, "SELECT count(*) FROM releases"); n != 0 {
		t.Errorf("%d releases après remplacement par une liste vide, attendu 0", n)
	}
	tags, _ = s.ListTags(ctx, ReleaseFilter{})
	if len(tags) != 1 || !tags[0].CommittedAt.Equal(t0.Add(time.Hour)) || tags[0].CommitURL == "" {
		t.Errorf("tag daté par son commit : %+v", tags)
	}
}

func TestApplyRepoSyncRejectsInvalidExtras(t *testing.T) {
	s := newTestStore(t)
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	noDate := pr(1, model.PullRequestOpen, 1)
	noDate.UpdatedAt = time.Time{}
	for name, w := range map[string]RepoSyncWrite{
		"PR sans numéro":    {PullRequests: []model.PullRequest{pr(0, model.PullRequestOpen, 1)}},
		"PR d'état inconnu": {PullRequests: []model.PullRequest{pr(1, "merged", 1)}},
		"PR sans date":      {PullRequests: []model.PullRequest{noDate}},
		"release sans tag":  {ReplaceReleases: true, Releases: []model.Release{{ProviderID: 1, CreatedAt: t0}}},
		"tag sans SHA":      {ReplaceTags: true, Tags: []model.Tag{{Name: "v1"}}},
	} {
		w.PullRequests = append(w.PullRequests, pr(7, model.PullRequestOpen, 1))
		if err := applyExtras(t, s, repoID, w); err == nil {
			t.Errorf("%s : aucune erreur", name)
		}
		if n := count(t, s, "SELECT count(*) FROM pull_requests"); n != 0 {
			t.Fatalf("%s : %d PR écrites malgré l'erreur", name, n)
		}
		// Le sync_run ouvert reste « running » : on le clôt pour le test suivant.
		mustExec(t, s, "UPDATE sync_runs SET status = 'error', finished_at = ? WHERE status = 'running'", testDate)
	}
}

func TestRepositorySummaryPullsAndRelease(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")
	if err := applyExtras(t, s, repoID, RepoSyncWrite{
		PullRequests:    []model.PullRequest{pr(1, model.PullRequestOpen, 1), pr(2, model.PullRequestOpen, 2), pr(3, model.PullRequestClosed, 3)},
		ReplaceReleases: true,
		Releases: []model.Release{
			{ProviderID: 1, TagName: "v1", CreatedAt: t0, PublishedAt: t0.Add(time.Hour)},
			{ProviderID: 2, TagName: "v2", CreatedAt: t0.Add(2 * time.Hour), PublishedAt: t0.Add(2 * time.Hour)},
			{ProviderID: 3, TagName: "v3", CreatedAt: t0.Add(9 * time.Hour), Draft: true},
		},
	}); err != nil {
		t.Fatal(err)
	}
	sum, err := s.RepositorySummary(ctx, repoID)
	if err != nil || sum.OpenPullRequests != 2 || sum.LatestRelease.TagName != "v2" {
		t.Errorf("résumé = open %d, release %+v, %v", sum.OpenPullRequests, sum.LatestRelease, err)
	}
	all, err := s.RepositorySummaries(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("RepositorySummaries : %v", err)
	}
	for _, r := range all {
		if r.ID == other && (r.OpenPullRequests != 0 || r.LatestRelease.ProviderID != 0) {
			t.Errorf("dépôt sans PR ni release : %+v", r)
		}
	}
	if _, err := s.RepositorySummary(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("dépôt inconnu : %v", err)
	}
}

// equalInts compare deux listes d'entiers (nil et vide sont égales).
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
