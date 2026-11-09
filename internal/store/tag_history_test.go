package store

// Migrations 00018 and 00019 (phase 8.4): tables tag_observations (SV3, deadlines of SV2) and
// branch_rewrites (BR4), and their store methods.

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestMigrationTagObservationsBranchRewrites(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 17); err != nil {
		t.Fatalf("UpTo(17): %v", err)
	}
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const later = "2026-10-27T08:00:00Z"
	const obs = `INSERT INTO tag_observations (repo_id, name, sha, first_seen_at, last_seen_at, ended_at, end_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	mustExec(t, s, obs, repo, "v1.0.0", sha(1), testDate, later, nil, nil)
	mustExec(t, s, obs, repo, "v0.1.0", sha(2), testDate, testDate, later, "rewritten")
	mustExec(t, s, obs, repo, "v0.1.0", sha(3), later, later, nil, nil) // reopened under another SHA
	mustExec(t, s, obs, repo, "v0.2.0", sha(4), testDate, testDate, later, "deleted")
	mustExec(t, s, obs, repo, "v0.2.0", sha(4), later, later, later, "deleted") // same couple, second period
	for _, bad := range []struct {
		name string
		args []any
	}{
		{"second open period of a name", []any{repo, "v1.0.0", sha(5), later, later, nil, nil}},
		{"empty name", []any{repo, "", sha(5), testDate, testDate, nil, nil}},
		{"short SHA", []any{repo, "v2", "abc1234", testDate, testDate, nil, nil}},
		{"uppercase SHA", []any{repo, "v2", "ABCDEF0123456789ABCDEF0123456789ABCDEF01", testDate, testDate, nil, nil}},
		{"first date", []any{repo, "v2", sha(5), "2026-10-26", testDate, nil, nil}},
		{"last date", []any{repo, "v2", sha(5), testDate, "2026-10-26 14:03:00", nil, nil}},
		{"end date", []any{repo, "v2", sha(5), testDate, testDate, "2026-10-27", "deleted"}},
		{"last before first", []any{repo, "v2", sha(5), later, testDate, nil, nil}},
		{"end before last", []any{repo, "v2", sha(5), testDate, later, testDate, "deleted"}},
		{"end without reason", []any{repo, "v2", sha(5), testDate, testDate, later, nil}},
		{"reason without end", []any{repo, "v2", sha(5), testDate, testDate, nil, "deleted"}},
		{"unknown reason", []any{repo, "v2", sha(5), testDate, testDate, later, "moved"}},
		{"unknown repository", []any{repo + 99, "v2", sha(5), testDate, testDate, nil, nil}},
	} {
		if err := execErr(t, s, obs, bad.args...); err == nil {
			t.Errorf("tag_observations: %s accepted", bad.name)
		}
	}

	const rw = `INSERT INTO branch_rewrites (repo_id, branch, old_sha, new_sha, observed_at) VALUES (?, ?, ?, ?, ?)`
	mustExec(t, s, rw, repo, "main", sha(1), sha(2), testDate)
	mustExec(t, s, rw, repo, "main", sha(1), sha(2), testDate) // the same event observed twice: two rows
	for _, bad := range []struct {
		name string
		args []any
	}{
		{"empty branch", []any{repo, "", sha(1), sha(2), testDate}},
		{"short old SHA", []any{repo, "main", "abc1234", sha(2), testDate}},
		{"uppercase new SHA", []any{repo, "main", sha(1), "ABCDEF0123456789ABCDEF0123456789ABCDEF01", testDate}},
		{"same heads", []any{repo, "main", sha(1), sha(1), testDate}},
		{"date", []any{repo, "main", sha(1), sha(2), "2026-10-26"}},
		{"unknown repository", []any{repo + 99, "main", sha(1), sha(2), testDate}},
	} {
		if err := execErr(t, s, rw, bad.args...); err == nil {
			t.Errorf("branch_rewrites: %s accepted", bad.name)
		}
	}

	// Rollback: tables removed, then recreated.
	if _, err := p.DownTo(ctx, 17); err != nil {
		t.Fatalf("DownTo(17): %v", err)
	}
	for _, name := range []string{"tag_observations", "branch_rewrites"} {
		if got := count(t, s, `SELECT count(*) FROM sqlite_master WHERE name = ?`, name); got != 0 {
			t.Errorf("table %s still present after the rollback", name)
		}
	}
	if _, err := p.UpTo(ctx, 19); err != nil {
		t.Fatalf("UpTo(19) again: %v", err)
	}
}

// observeSync applies a sync of the repository promo/name (repoID) at the date at with the tags tags.
func observeSync(t *testing.T, s *Store, repoID int64, name string, at time.Time, observe bool, tags ...model.Tag) {
	t.Helper()
	w := RepoSyncWrite{RepoID: repoID, RunID: beginRun(t, s, repoID, at), Metadata: meta("promo", name),
		SyncedAt: at, ReplaceTags: true, Tags: tags, ObserveTags: observe}
	if _, err := s.ApplyRepoSync(t.Context(), w); err != nil {
		t.Fatalf("ApplyRepoSync at %s: %v", at, err)
	}
}

// period is a compact view of an observation: name, SHA number, first, last, end (hours after t0), reason.
type period struct {
	name               string
	sha                string
	first, last, ended int
	reason             string
}

func periods(t *testing.T, s *Store, repoID int64) []period {
	t.Helper()
	obs, err := s.TagObservations(t.Context(), repoID)
	if err != nil {
		t.Fatal(err)
	}
	hours := func(at time.Time) int { return int(at.Sub(t0) / time.Hour) }
	out := []period{}
	for _, o := range obs {
		p := period{name: o.Name, sha: o.SHA, first: hours(o.FirstSeenAt), last: hours(o.LastSeenAt), ended: -1,
			reason: o.EndReason}
		if o.EndedAt != nil {
			p.ended = hours(*o.EndedAt)
		}
		out = append(out, p)
	}
	return out
}

func TestApplyRepoSyncObservesTags(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")
	at := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }
	tag := func(name string, n int) model.Tag { return model.Tag{Name: name, SHA: sha(n)} }

	// Without ObserveTags (conventions disabled, list truncated): nothing is observed.
	observeSync(t, s, repo, "alice", at(0), false, tag("v1.0.0", 1))
	if got := periods(t, s, repo); len(got) != 0 {
		t.Fatalf("observations without ObserveTags: %+v", got)
	}

	observeSync(t, s, repo, "alice", at(1), true, tag("v1.0.0", 1), tag("v1.1.0", 2), tag("v2.0.0", 3))
	observeSync(t, s, other, "bob", at(1), true, tag("v1.0.0", 9))
	// Seen again; v1.1.0 moved to another commit; v2.0.0 deleted; v3.0.0 new.
	observeSync(t, s, repo, "alice", at(2), true, tag("v1.0.0", 1), tag("v1.1.0", 4), tag("v3.0.0", 5))
	// v2.0.0 back on the same commit: a new period; nothing else changes.
	observeSync(t, s, repo, "alice", at(3), true, tag("v1.0.0", 1), tag("v1.1.0", 4), tag("v2.0.0", 3), tag("v3.0.0", 5))
	// Truncated or failed read at 4: the caller does not observe; v3.0.0 is not closed.
	observeSync(t, s, repo, "alice", at(4), false, tag("v1.0.0", 1))

	want := []period{
		{"v1.0.0", sha(1), 1, 3, -1, ""},
		{"v1.1.0", sha(2), 1, 1, 2, model.TagEndRewritten},
		{"v1.1.0", sha(4), 2, 3, -1, ""},
		{"v2.0.0", sha(3), 1, 1, 2, model.TagEndDeleted},
		{"v2.0.0", sha(3), 3, 3, -1, ""},
		{"v3.0.0", sha(5), 2, 3, -1, ""},
	}
	if got := periods(t, s, repo); !slices.Equal(got, want) {
		t.Errorf("observations =\n%+v\nwant\n%+v", got, want)
	}
	if all, err := s.TagObservations(ctx, 0); err != nil || len(all) != len(want)+1 {
		t.Errorf("all repositories: %d observations, %v", len(all), err)
	}

	// ObserveTags without the list read is refused before any write.
	w := RepoSyncWrite{RepoID: repo, RunID: beginRun(t, s, repo, at(5)), Metadata: meta("promo", "alice"),
		SyncedAt: at(5), ObserveTags: true}
	if _, err := s.ApplyRepoSync(ctx, w); err == nil {
		t.Error("ObserveTags without ReplaceTags accepted")
	}
	mustExec(t, s, "UPDATE sync_runs SET status = 'error', finished_at = ? WHERE status = 'running'", testDate)

	// Standalone method: an empty complete list closes everything as deleted.
	if err := s.ObserveTags(ctx, repo, nil, at(6)); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM tag_observations WHERE repo_id = ? AND ended_at IS NULL", repo); n != 0 {
		t.Errorf("%d open observations after an empty list", n)
	}
	if n := count(t, s, "SELECT count(*) FROM tag_observations WHERE repo_id = ? AND end_reason = 'deleted' AND ended_at = ?",
		repo, FormatTime(at(6))); n != 4 {
		t.Errorf("%d observations deleted at 6, want 4", n)
	}

	// Deleting the repository deletes its rows only.
	if err := s.DeleteRepository(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM tag_observations WHERE repo_id = ?", repo); n != 0 {
		t.Errorf("%d observations left for the deleted repository", n)
	}
	if n := count(t, s, "SELECT count(*) FROM tag_observations WHERE repo_id = ?", other); n != 1 {
		t.Errorf("%d observations for the other repository, want 1", n)
	}
	if none, err := newTestStore(t).TagObservations(ctx, 0); err != nil || none == nil || len(none) != 0 {
		t.Errorf("empty database: %v, %v", none, err)
	}
}

func TestBranchRewritesStore(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")
	at := t0.Add(2 * time.Hour)

	// Through the sync: recorded at SyncedAt, RepoID and ObservedAt ignored.
	w := RepoSyncWrite{RepoID: repo, RunID: beginRun(t, s, repo, at), Metadata: meta("promo", "alice"), SyncedAt: at,
		BranchRewrites: []model.BranchRewrite{{RepoID: other, Branch: "main", OldSHA: sha(1), NewSHA: sha(2), ObservedAt: t0}}}
	if _, err := s.ApplyRepoSync(ctx, w); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordBranchRewrite(ctx, model.BranchRewrite{RepoID: repo, Branch: "main", OldSHA: sha(2),
		NewSHA: sha(3), ObservedAt: t0.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordBranchRewrite(ctx, model.BranchRewrite{RepoID: other, Branch: "trunk", OldSHA: sha(4),
		NewSHA: sha(5), ObservedAt: t0}); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]model.BranchRewrite{
		"same heads":         {RepoID: repo, Branch: "main", OldSHA: sha(1), NewSHA: sha(1), ObservedAt: t0},
		"short SHA":          {RepoID: repo, Branch: "main", OldSHA: "abc1234", NewSHA: sha(1), ObservedAt: t0},
		"empty branch":       {RepoID: repo, OldSHA: sha(1), NewSHA: sha(2), ObservedAt: t0},
		"unknown repository": {RepoID: other + 9, Branch: "main", OldSHA: sha(1), NewSHA: sha(2), ObservedAt: t0},
	} {
		if err := s.RecordBranchRewrite(ctx, bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}

	got, err := s.BranchRewrites(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	want := []model.BranchRewrite{
		{RepoID: repo, Branch: "main", OldSHA: sha(2), NewSHA: sha(3), ObservedAt: t0.Add(time.Hour)},
		{RepoID: repo, Branch: "main", OldSHA: sha(1), NewSHA: sha(2), ObservedAt: at},
	}
	if !slices.Equal(got, want) {
		t.Errorf("rewrites = %+v, want %+v", got, want)
	}
	if all, err := s.BranchRewrites(ctx, 0); err != nil || len(all) != 3 {
		t.Errorf("all repositories: %d rewrites, %v", len(all), err)
	}
	if err := s.DeleteRepository(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM branch_rewrites"); n != 1 {
		t.Errorf("%d rewrites left, want 1 (the other repository)", n)
	}
	if none, err := newTestStore(t).BranchRewrites(ctx, 0); err != nil || none == nil || len(none) != 0 {
		t.Errorf("empty database: %v, %v", none, err)
	}
}

func TestConventionTagDates(t *testing.T) {
	s := newTestStore(t)
	repo := mustCreateRepo(t, s, 1, "promo", "alice")
	applySync(t, s, repo, []BranchState{{Name: "main", HeadSHA: sha(1)}}, cw(1, person("A", "a@example.test", "", 0), "main"))
	if err := applyExtras(t, s, repo, RepoSyncWrite{Branches: []BranchState{{Name: "main", HeadSHA: sha(1)}},
		ReplaceTags: true, Tags: []model.Tag{{Name: "v1", SHA: sha(1)}, {Name: "v2", SHA: sha(8)}}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ConventionTagDates(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got["v1"].Equal(t0.Add(time.Hour)) {
		t.Errorf("dates = %v, want v1 only, dated by its commit", got)
	}
}
