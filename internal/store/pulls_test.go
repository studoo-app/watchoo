package store

// Tests of pull requests, releases and tags (V2): schema constraints (00003), writing by
// ApplyRepoSync (creation, update, replacement, preservation), reads and repository summary.

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
		t.Errorf("duplicate PR: %v, want a primary key violation", err)
	}
	mustExec(t, s, insertPR, other, 1, "closed", testDate, testDate, testDate) // same number, other repository
	wantCode(t, "merged state", execErr(t, s, insertPR, repo, 2, "merged", testDate, testDate, nil), check)
	wantCode(t, "number 0", execErr(t, s, insertPR, repo, 0, "open", testDate, testDate, nil), check)
	wantCode(t, "malformed date", execErr(t, s, insertPR, repo, 3, "open", "2026-10-26 14:03:00", testDate, nil), check)
	wantCode(t, "malformed merged_at", execErr(t, s, insertPR, repo, 4, "closed", testDate, testDate, "yesterday"), check)
	wantCode(t, "PR of a nonexistent repository", execErr(t, s, insertPR, 999, 1, "open", testDate, testDate, nil), fk)

	mustExec(t, s, insertRelease, repo, 10, "v1", testDate)
	if err := execErr(t, s, insertRelease, repo, 10, "v1-bis", testDate); !isUniqueViolation(err) {
		t.Errorf("duplicate release: %v", err)
	}
	wantCode(t, "release without tag", execErr(t, s, insertRelease, repo, 11, "", testDate), check)
	wantCode(t, "release d'identifiant 0", execErr(t, s, insertRelease, repo, 0, "v0", testDate), check)
	wantCode(t, "badly dated release", execErr(t, s, insertRelease, repo, 12, "v2", "2026-10-26"), check)

	mustExec(t, s, insertTag, repo, "v1", sha(1))
	if err := execErr(t, s, insertTag, repo, "v1", sha(2)); !isUniqueViolation(err) {
		t.Errorf("duplicate tag: %v", err)
	}
	wantCode(t, "tag without name", execErr(t, s, insertTag, repo, "", sha(1)), check)
	wantCode(t, "tag of a nonexistent repository", execErr(t, s, insertTag, 999, "v1", sha(1)), fk)

	// Deleting the repository deletes its PRs, releases and tags, not those of the others.
	if err := s.DeleteRepository(t.Context(), repo); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}
	for _, table := range []string{"pull_requests", "releases", "tags"} {
		if n := count(t, s, "SELECT count(*) FROM "+table+" WHERE repo_id = ?", repo); n != 0 {
			t.Errorf("%s: %d rows left for the deleted repository", table, n)
		}
	}
	if n := count(t, s, "SELECT count(*) FROM pull_requests WHERE repo_id = ?", other); n != 1 {
		t.Errorf("PRs of the other repository: %d, want 1", n)
	}
}

// pr builds a dummy pull request n, updated at t0 + h hours.
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

// applyExtras applies a sync without commits with the given PRs, releases and tags.
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
		t.Fatalf("LatestPullRequestUpdate without PR: %v, %v", last, err)
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
			{ProviderID: 10, TagName: "v1", CreatedAt: t0}, // ignored duplicate
		},
		ReplaceTags: true,
		Tags:        []model.Tag{{Name: "v1", SHA: sha(1)}, {Name: "v0", SHA: sha(99)}},
	})
	if err != nil {
		t.Fatalf("ApplyRepoSync: %v", err)
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
			t.Fatalf("ListPullRequests(%q): %v", tt.status, err)
		}
		var got []int
		for _, v := range views {
			got = append(got, v.Number)
			if v.RepoDisplayName() != "promo/alice" {
				t.Errorf("repository %q", v.RepoDisplayName())
			}
		}
		if !equalInts(got, tt.want) {
			t.Errorf("ListPullRequests(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
	if _, err := s.ListPullRequests(ctx, PullRequestFilter{Status: "autre"}); err == nil {
		t.Error("unknown state accepted")
	}
	views, _ := s.ListPullRequests(ctx, PullRequestFilter{Status: model.PullRequestStatusMerged})
	if len(views) != 1 || views[0].Status() != model.PullRequestStatusMerged || !views[0].MergedAt.Equal(merged.MergedAt) ||
		views[0].AuthorLogin != "alice" || views[0].AuthorProviderID != 501 || views[0].HeadBranch != "feature" {
		t.Errorf("merged PR read back: %+v", views)
	}
	if limited, _ := s.ListPullRequests(ctx, PullRequestFilter{Limit: 1}); len(limited) != 1 || limited[0].Number != 4 {
		t.Errorf("Limit 1: %+v", limited)
	}

	releases, err := s.ListReleases(ctx, ReleaseFilter{RepoID: repoID})
	if err != nil || len(releases) != 2 || releases[0].ProviderID != 11 || !releases[1].Date().Equal(t0.Add(time.Hour)) {
		t.Errorf("ListReleases = %+v, %v", releases, err)
	}
	tags, err := s.ListTags(ctx, ReleaseFilter{RepoID: repoID})
	if err != nil || len(tags) != 2 || tags[0].Name != "v1" || !tags[0].CommittedAt.IsZero() {
		t.Errorf("ListTags (unknown commits) = %+v, %v", tags, err)
	}

	// Update of a PR, releases and tags unchanged when they are not read again.
	reopened := pr(4, model.PullRequestOpen, 6)
	reopened.Title = "nouveau titre"
	if err := applyExtras(t, s, repoID, RepoSyncWrite{PullRequests: []model.PullRequest{reopened}}); err != nil {
		t.Fatalf("ApplyRepoSync (update): %v", err)
	}
	views, _ = s.ListPullRequests(ctx, PullRequestFilter{RepoID: repoID, Limit: 1})
	if len(views) != 1 || views[0].Number != 4 || views[0].Title != "nouveau titre" || views[0].State != model.PullRequestOpen ||
		!views[0].ClosedAt.IsZero() {
		t.Errorf("updated PR: %+v", views)
	}
	if n := count(t, s, "SELECT count(*) FROM pull_requests"); n != 4 {
		t.Errorf("%d PRs, want 4 (none deleted)", n)
	}
	if n := count(t, s, "SELECT count(*) FROM releases"); n != 2 {
		t.Errorf("%d releases, want 2 (unchanged)", n)
	}

	// Replacement: a release and a tag deleted on the forge disappear; tag dated by its commit.
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(1)}}, cw(1, person("A", "a@example.test", "", 0), "main"))
	if err := applyExtras(t, s, repoID, RepoSyncWrite{
		Branches:        []BranchState{{Name: "main", HeadSHA: sha(1)}},
		ReplaceReleases: true, Releases: []model.Release{},
		ReplaceTags: true, Tags: []model.Tag{{Name: "v1", SHA: sha(1)}},
	}); err != nil {
		t.Fatalf("ApplyRepoSync (replacement): %v", err)
	}
	if n := count(t, s, "SELECT count(*) FROM releases"); n != 0 {
		t.Errorf("%d releases after replacement by an empty list, want 0", n)
	}
	tags, _ = s.ListTags(ctx, ReleaseFilter{})
	if len(tags) != 1 || !tags[0].CommittedAt.Equal(t0.Add(time.Hour)) || tags[0].CommitURL == "" {
		t.Errorf("tag dated by its commit: %+v", tags)
	}
}

func TestApplyRepoSyncRejectsInvalidExtras(t *testing.T) {
	s := newTestStore(t)
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	noDate := pr(1, model.PullRequestOpen, 1)
	noDate.UpdatedAt = time.Time{}
	for name, w := range map[string]RepoSyncWrite{
		"PR without number":   {PullRequests: []model.PullRequest{pr(0, model.PullRequestOpen, 1)}},
		"PR of unknown state": {PullRequests: []model.PullRequest{pr(1, "merged", 1)}},
		"PR without date":     {PullRequests: []model.PullRequest{noDate}},
		"release without tag": {ReplaceReleases: true, Releases: []model.Release{{ProviderID: 1, CreatedAt: t0}}},
		"tag without SHA":     {ReplaceTags: true, Tags: []model.Tag{{Name: "v1"}}},
	} {
		w.PullRequests = append(w.PullRequests, pr(7, model.PullRequestOpen, 1))
		if err := applyExtras(t, s, repoID, w); err == nil {
			t.Errorf("%s: no error", name)
		}
		if n := count(t, s, "SELECT count(*) FROM pull_requests"); n != 0 {
			t.Fatalf("%s: %d PRs written despite the error", name, n)
		}
		// The open sync_run stays "running": it is closed for the next test.
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
		t.Errorf("summary = open %d, release %+v, %v", sum.OpenPullRequests, sum.LatestRelease, err)
	}
	all, err := s.RepositorySummaries(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("RepositorySummaries: %v", err)
	}
	for _, r := range all {
		if r.ID == other && (r.OpenPullRequests != 0 || r.LatestRelease.ProviderID != 0) {
			t.Errorf("repository without PR or release: %+v", r)
		}
	}
	if _, err := s.RepositorySummary(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown repository: %v", err)
	}
}

// equalInts compares two lists of integers (nil and empty are equal).
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
