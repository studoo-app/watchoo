package store

// Migration 00020 (phase 8.5): merge commit, fork origin and body sections of the pull requests, merge
// methods of the repository and marker of the full read of the pull requests; their writing by
// ApplyRepoSync and their reading (ListPullRequests, ConventionPullRequests, GetRepository).

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestMigrationPullRequestConventions(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 19); err != nil {
		t.Fatalf("UpTo(19): %v", err)
	}
	// Database at schema 00019: a repository and a merged pull request.
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	mustExec(t, s, insertPR, repo, 1, "closed", testDate, testDate, testDate)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if got := count(t, s, `SELECT count(*) FROM pull_requests
		WHERE merge_commit_sha IS NULL AND from_fork IS NULL AND body_sections IS NULL`); got != 1 {
		t.Errorf("%d pull requests with unknown new columns, want 1", got)
	}
	got, err := s.GetRepository(ctx, repo)
	if err != nil || got.AllowSquashMerge != nil || got.AllowMergeCommit != nil || got.AllowRebaseMerge != nil ||
		got.ConventionsPRsVersion != 0 {
		t.Errorf("repository after the migration: %+v, %v", got, err)
	}
	views, err := s.ListPullRequests(ctx, PullRequestFilter{RepoID: repo})
	if err != nil || len(views) != 1 || views[0].MergeCommitSHA != "" || views[0].FromFork != nil || views[0].BodySections != nil {
		t.Errorf("pull request after the migration: %+v, %v", views, err)
	}
	for stmt, bads := range map[string][]any{
		"UPDATE pull_requests SET merge_commit_sha = ?":       {"", "abc", strings.Repeat("A", 40), strings.Repeat("g", 40), strings.Repeat("a", 41)},
		"UPDATE pull_requests SET from_fork = ?":              {2, -1},
		"UPDATE pull_requests SET body_sections = ?":          {-1},
		"UPDATE repositories SET allow_squash_merge = ?":      {2},
		"UPDATE repositories SET allow_merge_commit = ?":      {-1},
		"UPDATE repositories SET allow_rebase_merge = ?":      {3},
		"UPDATE repositories SET conventions_prs_version = ?": {0, -2},
	} {
		for _, bad := range bads {
			if err := execErr(t, s, stmt, bad); err == nil {
				t.Errorf("%s with %v accepted", stmt, bad)
			}
		}
	}
	mustExec(t, s, `UPDATE pull_requests SET merge_commit_sha = ?, from_fork = 1, body_sections = 31`, sha(7))
	mustExec(t, s, `UPDATE pull_requests SET merge_commit_sha = ?`, strings.Repeat("b", 64))
	mustExec(t, s, `UPDATE repositories SET allow_squash_merge = 1, allow_merge_commit = 0, allow_rebase_merge = 0,
		conventions_prs_version = 1`)

	// Rollback: columns removed, rows kept.
	if _, err := p.DownTo(ctx, 19); err != nil {
		t.Fatalf("DownTo(19): %v", err)
	}
	if got := count(t, s, "SELECT count(*) FROM pull_requests"); got != 1 {
		t.Errorf("%d pull requests after the rollback, want 1", got)
	}
	for _, col := range []string{"merge_commit_sha", "from_fork", "body_sections"} {
		if err := execErr(t, s, "SELECT "+col+" FROM pull_requests"); err == nil {
			t.Errorf("pull_requests.%s still present", col)
		}
	}
	for _, col := range []string{"allow_squash_merge", "allow_merge_commit", "allow_rebase_merge", "conventions_prs_version"} {
		if err := execErr(t, s, "SELECT "+col+" FROM repositories"); err == nil {
			t.Errorf("repositories.%s still present", col)
		}
	}
	if _, err := p.UpTo(ctx, 20); err != nil {
		t.Fatalf("UpTo(20) again: %v", err)
	}
}

func TestApplyRepoSyncPullRequestConventions(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	yes, no := true, false
	mask := func(v int64) *int64 { return &v }

	// Insertion: merge commit, fork origin and body sections written; unknown values stay NULL.
	merged := pr(1, model.PullRequestClosed, 2)
	merged.Merged, merged.MergedAt = true, merged.UpdatedAt
	merged.MergeCommitSHA, merged.FromFork, merged.BodySections = sha(9), &no, mask(17)
	fork := pr(2, model.PullRequestOpen, 3)
	fork.FromFork, fork.BodySections = &yes, mask(0)
	unknown := pr(3, model.PullRequestOpen, 1)
	if err := applyExtras(t, s, repoID, RepoSyncWrite{PullRequests: []model.PullRequest{merged, fork, unknown}}); err != nil {
		t.Fatalf("ApplyRepoSync: %v", err)
	}
	byNumber := func() map[int]model.PullRequest {
		t.Helper()
		views, err := s.ListPullRequests(ctx, PullRequestFilter{RepoID: repoID})
		if err != nil {
			t.Fatal(err)
		}
		out := map[int]model.PullRequest{}
		for _, v := range views {
			out[v.Number] = v.PullRequest
		}
		conv, err := s.ConventionPullRequests(ctx, repoID)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range conv {
			l := out[c.Number]
			if c.MergeCommitSHA != l.MergeCommitSHA || !equalBool(c.FromFork, l.FromFork) || !equalInt64(c.BodySections, l.BodySections) {
				t.Errorf("PR %d: ConventionPullRequests %+v differs from ListPullRequests %+v", c.Number, c.PullRequest, l)
			}
		}
		return out
	}
	got := byNumber()
	if p := got[1]; p.MergeCommitSHA != sha(9) || !equalBool(p.FromFork, &no) || !equalInt64(p.BodySections, mask(17)) {
		t.Errorf("merged PR = %+v", p)
	}
	if p := got[2]; p.MergeCommitSHA != "" || !equalBool(p.FromFork, &yes) || !equalInt64(p.BodySections, mask(0)) {
		t.Errorf("fork PR = %+v", p)
	}
	if p := got[3]; p.MergeCommitSHA != "" || p.FromFork != nil || p.BodySections != nil {
		t.Errorf("unknown PR = %+v", p)
	}

	// Update: an unknown origin or mask keeps the known value; the merge commit is replaced.
	fork.FromFork, fork.BodySections, fork.UpdatedAt = nil, nil, fork.UpdatedAt.Add(time.Hour)
	if err := applyExtras(t, s, repoID, RepoSyncWrite{PullRequests: []model.PullRequest{fork}}); err != nil {
		t.Fatalf("ApplyRepoSync: %v", err)
	}
	if p := byNumber()[2]; !equalBool(p.FromFork, &yes) || !equalInt64(p.BodySections, mask(0)) {
		t.Errorf("fork PR after an unknown read = %+v", p)
	}

	// Full read: only the three columns of the PRs already in the database; tool_ids kept; an absent PR
	// ignored; the marker written.
	mustExec(t, s, `UPDATE pull_requests SET tool_ids = 'D2:claude' WHERE repo_id = ? AND number = 3`, repoID)
	err := applyExtras(t, s, repoID, RepoSyncWrite{
		PullRequestConventions: []PullRequestConventionsWrite{
			{Number: 3, MergeCommitSHA: strings.Repeat("c", 64), FromFork: &no, BodySections: mask(6)},
			{Number: 2, FromFork: nil, BodySections: nil}, // unknown: known values kept
			{Number: 99, FromFork: &yes},                  // not in the database: ignored
		},
		PullRequestsVersion: 1,
	})
	if err != nil {
		t.Fatalf("ApplyRepoSync (full read): %v", err)
	}
	got = byNumber()
	if p := got[3]; p.MergeCommitSHA != strings.Repeat("c", 64) || !equalBool(p.FromFork, &no) || !equalInt64(p.BodySections, mask(6)) ||
		len(p.ToolIDs) != 1 || p.Title != "titre" {
		t.Errorf("PR 3 after the full read = %+v", p)
	}
	if p := got[2]; !equalBool(p.FromFork, &yes) || !equalInt64(p.BodySections, mask(0)) {
		t.Errorf("PR 2 after the full read = %+v", p)
	}
	if n := count(t, s, "SELECT count(*) FROM pull_requests WHERE repo_id = ?", repoID); n != 3 {
		t.Errorf("%d pull requests, want 3 (absent PR not created)", n)
	}
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil || repo.ConventionsPRsVersion != 1 {
		t.Errorf("ConventionsPRsVersion = %+v, %v", repo, err)
	}
	// 0: marker unchanged.
	if err := applyExtras(t, s, repoID, RepoSyncWrite{}); err != nil {
		t.Fatal(err)
	}
	if repo, _ := s.GetRepository(ctx, repoID); repo.ConventionsPRsVersion != 1 {
		t.Errorf("marker changed by a sync without full read: %d", repo.ConventionsPRsVersion)
	}

	// Invalid values are refused before any write.
	next := int64(100)
	for name, w := range map[string]RepoSyncWrite{
		"short merge commit":     {PullRequests: []model.PullRequest{func() model.PullRequest { p := pr(5, "open", 1); p.MergeCommitSHA = "abc"; return p }()}},
		"uppercase merge":        {PullRequestConventions: []PullRequestConventionsWrite{{Number: 1, MergeCommitSHA: strings.Repeat("A", 40)}}},
		"negative mask":          {PullRequests: []model.PullRequest{func() model.PullRequest { p := pr(5, "open", 1); p.BodySections = mask(-1); return p }()}},
		"negative mask (read)":   {PullRequestConventions: []PullRequestConventionsWrite{{Number: 1, BodySections: mask(-4)}}},
		"number 0 (full read)":   {PullRequestConventions: []PullRequestConventionsWrite{{Number: 0}}},
		"negative rules version": {PullRequestsVersion: -1},
	} {
		// A refused write leaves its sync_run open: one repository per case.
		next++
		other := mustCreateRepo(t, s, next, "promo", fmt.Sprintf("refused-%d", next))
		if err := applyExtras(t, s, other, w); err == nil {
			t.Errorf("%s accepted", name)
		}
		if n := count(t, s, "SELECT count(*) FROM pull_requests WHERE repo_id = ?", other); n != 0 {
			t.Errorf("%s: %d pull requests written", name, n)
		}
	}
}

func TestApplyRepoSyncMergeMethods(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	yes, no := true, false
	sync := func(m RepositoryMetadata) model.Repository {
		t.Helper()
		now := t0.Add(1000 * time.Hour)
		if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{RepoID: repoID, RunID: beginRun(t, s, repoID, now),
			Metadata: m, SyncedAt: now}); err != nil {
			t.Fatalf("ApplyRepoSync: %v", err)
		}
		repo, err := s.GetRepository(ctx, repoID)
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	m := meta("promo", "alice")
	m.AllowSquashMerge, m.AllowMergeCommit, m.AllowRebaseMerge = &yes, &no, &no
	repo := sync(m)
	if !equalBool(repo.AllowSquashMerge, &yes) || !equalBool(repo.AllowMergeCommit, &no) || !equalBool(repo.AllowRebaseMerge, &no) {
		t.Errorf("merge methods = %v %v %v", repo.AllowSquashMerge, repo.AllowMergeCommit, repo.AllowRebaseMerge)
	}
	// Local clone (Partial): left as they are.
	if repo := sync(RepositoryMetadata{Partial: true}); !equalBool(repo.AllowSquashMerge, &yes) {
		t.Errorf("partial sync changed the merge methods: %v", repo.AllowSquashMerge)
	}
	// Push right lost: unknown again (NULL), never the old value.
	if repo := sync(meta("promo", "alice")); repo.AllowSquashMerge != nil || repo.AllowMergeCommit != nil || repo.AllowRebaseMerge != nil {
		t.Errorf("unknown merge methods kept the old values: %v %v %v", repo.AllowSquashMerge, repo.AllowMergeCommit,
			repo.AllowRebaseMerge)
	}
}

// ConventionCommits reads the committer date (PR3, phase 8.5).
func TestConventionCommitsCommittedAt(t *testing.T) {
	s := newTestStore(t)
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "alice", 501)
	c := cw(1, alice, "main")
	c.CommittedAt = c.AuthoredAt.Add(90 * time.Minute)
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(1)}}, c)
	got, err := s.ConventionCommits(t.Context(), repoID, 0)
	if err != nil || len(got) != 1 || !got[0].CommittedAt.Equal(c.CommittedAt) || got[0].CommittedAt.Location() != time.UTC {
		t.Errorf("ConventionCommits = %+v, %v", got, err)
	}
}

func equalBool(a, b *bool) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func equalInt64(a, b *int64) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
