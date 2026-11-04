package store

// Migration 00011 (phase 7.1): commit_signals, and its use (write once, purge, cascades).

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

func TestCommitSignals(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "alice", 501)
	main := []BranchState{{Name: "main", HeadSHA: sha(3)}}
	applySync(t, s, repoID, main, cw(1, alice, "main"), cw(2, alice, "main"), cw(3, alice, "main"))
	computed := t0.Add(2000 * time.Hour)

	n, err := s.WriteCommitSignals(ctx, repoID, []CommitSignalWrite{
		{SHA: sha(1), FilesKept: 3, FilesExcluded: 1, AdditionsKept: 120, DeletionsKept: 4,
			Tools: []string{"claude", "codex"}, AgentConfig: true},
		{SHA: sha(2), FilesKept: 1, AdditionsKept: 2, BotAuthor: true},
		{SHA: sha(9), FilesKept: 1}, // commit missing from the database: ignored
	}, computed)
	if err != nil || n != 2 {
		t.Fatalf("WriteCommitSignals = %d, %v; want 2", n, err)
	}
	// Written once: a second write changes nothing.
	n, err = s.WriteCommitSignals(ctx, repoID, []CommitSignalWrite{{SHA: sha(1), FilesKept: 99}}, computed)
	if err != nil || n != 0 {
		t.Errorf("second write = %d, %v; want 0", n, err)
	}

	got, err := s.CommitSignals(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SHA != sha(2) || got[1].SHA != sha(1) {
		t.Fatalf("CommitSignals = %+v", got)
	}
	first := got[1]
	if first.FilesKept != 3 || first.FilesExcluded != 1 || first.AdditionsKept != 120 || first.DeletionsKept != 4 ||
		!slices.Equal(first.Tools, []string{"claude", "codex"}) || !first.AgentConfig || first.BotAuthor ||
		first.AuthorID == 0 || !first.AuthoredAt.Equal(t0.Add(time.Hour)) || !first.ComputedAt.Equal(computed) {
		t.Errorf("signals of commit 1 = %+v", first)
	}
	if second := got[0]; second.Tools == nil || len(second.Tools) != 0 || !second.BotAuthor {
		t.Errorf("signals of commit 2 = %+v (want an empty non-nil tool list)", second)
	}
	if c, err := s.CountCommitSignals(ctx, repoID); err != nil || c != 2 {
		t.Errorf("CountCommitSignals = %d, %v", c, err)
	}

	// Purge by authored date: commit 1 (t0+1h) goes, commit 2 (t0+2h) stays.
	purged, err := s.PurgeCommitSignals(ctx, t0.Add(90*time.Minute))
	if err != nil || purged != 1 {
		t.Errorf("PurgeCommitSignals = %d, %v; want 1", purged, err)
	}
	if c, _ := s.CountCommitSignals(ctx, repoID); c != 1 {
		t.Errorf("%d rows after the purge, want 1", c)
	}

	// CHECK constraints and cascades.
	if err := execErr(t, s, `UPDATE commit_signals SET agent_config = 2`); err == nil {
		t.Error("agent_config = 2 accepted")
	}
	if err := execErr(t, s, `UPDATE commit_signals SET files_kept = -1`); err == nil {
		t.Error("files_kept = -1 accepted")
	}
	mustExec(t, s, `DELETE FROM commits WHERE repo_id = ? AND sha = ?`, repoID, sha(2))
	if c := count(t, s, "SELECT count(*) FROM commit_signals"); c != 0 {
		t.Errorf("%d rows after the commit was deleted, want 0", c)
	}
}

func TestMigrationCommitSignals(t *testing.T) {
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
	// Existing database at 00010 with a commit: the migration adds an empty table.
	if _, err := p.UpTo(ctx, 10); err != nil {
		t.Fatalf("UpTo(10): %v", err)
	}
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	mustExec(t, s, `INSERT INTO commits (repo_id, sha, authored_at, committed_at) VALUES (?, ?, ?, ?)`,
		repo, sha(1), testDate, testDate)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if c := count(t, s, "SELECT count(*) FROM commit_signals"); c != 0 {
		t.Errorf("%d rows after the migration, want 0", c)
	}
	for _, idx := range []string{"commit_signals_author_date", "commit_signals_date"} {
		if c := count(t, s, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, idx); c != 1 {
			t.Errorf("index %s missing", idx)
		}
	}
	// Rollback: table and indexes removed, commits kept; up again.
	if _, err := p.DownTo(ctx, 10); err != nil {
		t.Fatalf("DownTo(10): %v", err)
	}
	if c := count(t, s, `SELECT count(*) FROM sqlite_master WHERE name LIKE 'commit_signals%'`); c != 0 {
		t.Errorf("%d schema objects left after the rollback", c)
	}
	if c := count(t, s, "SELECT count(*) FROM commits"); c != 1 {
		t.Errorf("%d commits after the rollback, want 1", c)
	}
	if _, err := p.UpTo(ctx, 11); err != nil {
		t.Fatalf("UpTo(11) again: %v", err)
	}
}

func TestDeclaredSignalsStore(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")
	alice := person("Alice", "alice@example.test", "alice", 501)
	bob := person("Bob", "bob@example.test", "bob", 502)
	main := []BranchState{{Name: "main", HeadSHA: sha(3)}}
	applySync(t, s, repoID, main, cw(1, alice, "main"), cw(2, alice, "main"), cw(3, bob, "main"))
	computed := t0.Add(2000 * time.Hour)
	if _, err := s.WriteCommitSignals(ctx, repoID, []CommitSignalWrite{
		{SHA: sha(1), Tools: []string{"D1:claude"}, Form: model.CommitForm{Known: true}},
		{SHA: sha(2), FilesKept: 4, Form: model.CommitForm{Known: true}}, // no declarative signal: not listed
		{SHA: sha(3), BotAuthor: true, Form: model.CommitForm{Known: true}},
	}, computed); err != nil {
		t.Fatal(err)
	}

	all, err := s.CommitSignalViews(ctx, 0)
	if err != nil || len(all) != 3 || all[0].SHA != sha(1) || all[2].SHA != sha(3) {
		t.Fatalf("CommitSignalViews = %+v, %v", all, err)
	}
	if v := all[0]; !slices.Equal(v.Tools, []string{"D1:claude"}) || v.AuthorLogin != "alice" || v.AuthorName != "Alice" ||
		v.URL == "" || v.RepoFullName() != "promo/alice" {
		t.Errorf("view of commit 1: %+v", v)
	}
	if v, err := s.CommitSignalViews(ctx, other); err != nil || len(v) != 0 || v == nil {
		t.Errorf("other repository: %+v, %v", v, err)
	}
	for author, want := range map[string]int{"ALICE": 1, "Bob@Example.test": 1, "Bob": 1, "personne": 0, "": 0} {
		ids, err := s.MatchingContributorIDs(ctx, author)
		if err != nil || len(ids) != want || ids == nil {
			t.Errorf("MatchingContributorIDs(%q) = %v, %v; want %d", author, ids, err, want)
		}
	}

	// History backfill: commits without signals, from a date, page by page.
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(5)}}, cw(4, alice, "main"), cw(5, alice, "main"))
	page, err := s.CommitsWithoutSignals(ctx, repoID, time.Time{}, "", 1)
	if err != nil || len(page) != 1 || page[0].SHA != sha(4) || page[0].Message != "message 4" || page[0].AuthorLogin != "alice" {
		t.Fatalf("CommitsWithoutSignals page 1 = %+v, %v", page, err)
	}
	page, _ = s.CommitsWithoutSignals(ctx, repoID, time.Time{}, sha(4), 10)
	if len(page) != 1 || page[0].SHA != sha(5) {
		t.Errorf("page 2 = %+v", page)
	}
	if page, _ := s.CommitsWithoutSignals(ctx, repoID, t0.Add(5*time.Hour), "", 10); len(page) != 1 || page[0].SHA != sha(5) {
		t.Errorf("since: %+v", page)
	}

	// Form not measured (row written before phase 7.5): listed again, then completed once.
	if n, err := s.WriteCommitSignals(ctx, repoID, []CommitSignalWrite{{SHA: sha(4), FilesKept: 1}}, computed); err != nil || n != 1 {
		t.Fatalf("write without form = %d, %v", n, err)
	}
	if page, _ := s.CommitsWithoutSignals(ctx, repoID, time.Time{}, "", 10); len(page) != 2 || page[0].SHA != sha(4) {
		t.Errorf("row without form not listed again: %+v", page)
	}
	form := model.CommitForm{Known: true, SourceLines: 40, CommentLines: 10, Functions: 3, DocumentedFunctions: 2,
		Identifiers: 120, LexicalDiversity: 0.75, HasDiversity: true, TestLines: 5, MessageLength: 42,
		MessageBullets: 2, MessageConventional: true, DecorativeLines: 3, FrWords: 12, FrWordsUnaccented: 11,
		WIPMarkers: 1, ShortIdentifiers: 4, PrivateDefs: 2, MessageRevert: true, MessageFix: true}
	for _, want := range []int{1, 0} { // completed, then never modified again
		n, err := s.WriteCommitSignals(ctx, repoID, []CommitSignalWrite{{SHA: sha(4), FilesKept: 2, Form: form}}, computed)
		if err != nil || n != want {
			t.Fatalf("write with form = %d, %v; want %d", n, err, want)
		}
	}
	if page, _ := s.CommitsWithoutSignals(ctx, repoID, time.Time{}, "", 10); len(page) != 1 || page[0].SHA != sha(5) {
		t.Errorf("after the form: %+v", page)
	}
	views, _ := s.CommitSignalViews(ctx, repoID)
	if i := slices.IndexFunc(views, func(v CommitSignalView) bool { return v.SHA == sha(4) }); i < 0 ||
		views[i].Form != form || views[i].FilesKept != 2 {
		t.Errorf("view of commit 4: %+v", views)
	}

	// D4 context: replaced at each computation, NULL = none, listed by date.
	if err := s.SetAgentConfigSince(ctx, repoID, t0.Add(5*time.Hour), computed); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAgentConfigSince(ctx, other, time.Time{}, computed); err != nil {
		t.Fatal(err)
	}
	ctxs, err := s.AgentConfigContexts(ctx, 0)
	if err != nil || len(ctxs) != 1 || ctxs[0].RepoID != repoID || !ctxs[0].Since.Equal(t0.Add(5*time.Hour)) {
		t.Fatalf("AgentConfigContexts = %+v, %v", ctxs, err)
	}
	if err := s.SetAgentConfigSince(ctx, repoID, t0.Add(time.Hour), computed); err != nil {
		t.Fatal(err)
	}
	if ctxs, _ := s.AgentConfigContexts(ctx, repoID); len(ctxs) != 1 || !ctxs[0].Since.Equal(t0.Add(time.Hour)) {
		t.Errorf("context after update: %+v", ctxs)
	}
	if ctxs, _ := s.AgentConfigContexts(ctx, other); len(ctxs) != 0 || ctxs == nil {
		t.Errorf("repository without agent file: %+v", ctxs)
	}

	// Tool identifiers of pull requests: written with the PR, read back, purged by opening date.
	_, err = s.ApplyRepoSync(ctx, RepoSyncWrite{
		RepoID: repoID, RunID: beginRun(t, s, repoID, computed), Metadata: meta("promo", "alice"), Branches: main,
		PullRequests: []model.PullRequest{
			{Number: 1, State: model.PullRequestOpen, HeadBranch: "codex/x", CreatedAt: t0, UpdatedAt: t0,
				ToolIDs: []string{"D2:claude", "D2:unnamed"}},
			{Number: 2, State: model.PullRequestOpen, CreatedAt: t0.Add(10 * time.Hour), UpdatedAt: t0.Add(10 * time.Hour),
				ToolIDs: []string{"D2:codex"}},
		},
		SyncedAt: computed,
	})
	if err != nil {
		t.Fatal(err)
	}
	pulls, err := s.ListPullRequests(ctx, PullRequestFilter{RepoID: repoID})
	if err != nil || len(pulls) != 2 || !slices.Equal(pulls[1].ToolIDs, []string{"D2:claude", "D2:unnamed"}) {
		t.Fatalf("ListPullRequests = %+v, %v", pulls, err)
	}
	if n, err := s.PurgePullRequestTools(ctx, t0.Add(5*time.Hour)); err != nil || n != 1 {
		t.Errorf("PurgePullRequestTools = %d, %v; want 1", n, err)
	}
	pulls, _ = s.ListPullRequests(ctx, PullRequestFilter{RepoID: repoID})
	if len(pulls[1].ToolIDs) != 0 || pulls[1].ToolIDs == nil || !slices.Equal(pulls[0].ToolIDs, []string{"D2:codex"}) {
		t.Errorf("after the purge: %+v / %+v", pulls[0].ToolIDs, pulls[1].ToolIDs)
	}

	// Deleting the repository deletes its context.
	mustExec(t, s, `DELETE FROM repositories WHERE id = ?`, repoID)
	if c := count(t, s, "SELECT count(*) FROM repository_signals WHERE repo_id = ?", repoID); c != 0 {
		t.Errorf("%d context rows after the repository was deleted", c)
	}
}

func TestMigrationDeclaredSignals(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 11); err != nil {
		t.Fatalf("UpTo(11): %v", err)
	}
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	mustExec(t, s, `INSERT INTO pull_requests (repo_id, number, state, created_at, updated_at) VALUES (?, 1, 'open', ?, ?)`,
		repo, testDate, testDate)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if c := count(t, s, "SELECT count(*) FROM pull_requests WHERE tool_ids = ''"); c != 1 {
		t.Errorf("existing PR: tool_ids not empty (%d)", c)
	}
	if err := execErr(t, s, `INSERT INTO repository_signals (repo_id, agent_config_since, computed_at) VALUES (?, 'hier', ?)`,
		repo, testDate); err == nil {
		t.Error("badly formatted date accepted")
	}
	if _, err := p.DownTo(ctx, 11); err != nil {
		t.Fatalf("DownTo(11): %v", err)
	}
	if err := execErr(t, s, "SELECT tool_ids FROM pull_requests"); err == nil {
		t.Error("tool_ids still present after the rollback")
	}
	if c := count(t, s, "SELECT count(*) FROM pull_requests"); c != 1 {
		t.Errorf("%d PRs after the rollback, want 1", c)
	}
	if _, err := p.UpTo(ctx, 12); err != nil {
		t.Fatalf("UpTo(12) again: %v", err)
	}
}

// After a merge of contributors (commits reassigned, old contributor deleted), the signals follow the
// author of the commit: the rhythm history of the learner is kept.
func TestCommitSignalsFollowCommitAuthor(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	old := person("Alice", "alice@example.test", "", 0)
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(1)}}, cw(1, old, "main"))
	if _, err := s.WriteCommitSignals(ctx, repoID, []CommitSignalWrite{{SHA: sha(1), FilesKept: 1}}, t0); err != nil {
		t.Fatal(err)
	}
	oldID := int64(count(t, s, "SELECT author_id FROM commits WHERE sha = ?", sha(1)))
	newID := mustExec(t, s, insertCtb, "github", 777, "alice-gh")
	mustExec(t, s, "UPDATE commits SET author_id = ? WHERE author_id = ?", newID, oldID)
	mustExec(t, s, "DELETE FROM contributors WHERE id = ?", oldID)

	views, err := s.CommitSignalViews(ctx, repoID)
	if err != nil || len(views) != 1 || views[0].AuthorID != newID || views[0].AuthorLogin != "alice-gh" {
		t.Errorf("signals after the merge: %+v, %v (want author %d)", views, err, newID)
	}
	if got, err := s.CommitSignals(ctx, repoID); err != nil || len(got) != 1 || got[0].AuthorID != newID {
		t.Errorf("CommitSignals after the merge: %+v, %v", got, err)
	}
}

func TestDiagnosticReviews(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")
	r := model.DiagnosticReview{RepoID: repoID, LeadKey: "commit/" + sha(1), Kind: "commit", LeadDate: t0,
		Signals: []string{"D1", "R2"}, Decision: "not_assisted", Cause: "refactoring", Confidence: "high",
		Note: "Explication donnée en séance", Reviewer: "promo", ReviewedAt: t0.Add(time.Hour)}
	if err := s.SaveDiagnosticReview(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDiagnosticReview(ctx, model.DiagnosticReview{RepoID: other, LeadKey: "pull_request/3",
		Kind: "pull_request", LeadDate: t0.Add(48 * time.Hour), Signals: []string{}, Decision: "assisted",
		Confidence: "low", ReviewedAt: t0}); err != nil {
		t.Fatal(err)
	}
	all, err := s.DiagnosticReviews(ctx, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("DiagnosticReviews = %+v, %v", all, err)
	}
	got := all[ReviewKey(repoID, "commit/"+sha(1))]
	if got.Decision != "not_assisted" || got.Cause != "refactoring" || !slices.Equal(got.Signals, []string{"D1", "R2"}) ||
		got.Note != r.Note || !got.LeadDate.Equal(t0) || !got.ReviewedAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("review: %+v", got)
	}
	// Done again: replaced.
	r.Decision, r.Cause, r.Note = "assisted", "", ""
	if err := s.SaveDiagnosticReview(ctx, r); err != nil {
		t.Fatal(err)
	}
	if mine, _ := s.DiagnosticReviews(ctx, repoID); len(mine) != 1 || mine[ReviewKey(repoID, r.LeadKey)].Decision != "assisted" {
		t.Errorf("replaced review: %+v", mine)
	}
	// Unknown values refused by the database.
	bad := r
	bad.Decision = "guilty"
	if err := s.SaveDiagnosticReview(ctx, bad); err == nil {
		t.Error("unknown decision accepted")
	}
	// Purge by lead date, cascade with the repository.
	if n, err := s.PurgeDiagnosticReviews(ctx, t0.Add(time.Hour)); err != nil || n != 1 {
		t.Errorf("PurgeDiagnosticReviews = %d, %v; want 1", n, err)
	}
	mustExec(t, s, "DELETE FROM repositories WHERE id = ?", other)
	if c := count(t, s, "SELECT count(*) FROM diagnostic_reviews"); c != 0 {
		t.Errorf("%d reviews left", c)
	}
}

func TestMigrationDiagnosticReviews(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 13); err != nil {
		t.Fatalf("UpTo(13): %v", err)
	}
	if err := execErr(t, s, `INSERT INTO diagnostic_reviews (repo_id, lead_key, kind, lead_date, decision, confidence, reviewed_at)
		VALUES (1, 'x', 'week', ?, 'assisted', 'low', ?)`, testDate, testDate); err == nil {
		t.Error("unknown kind accepted")
	}
	if _, err := p.DownTo(ctx, 12); err != nil {
		t.Fatalf("DownTo(12): %v", err)
	}
	if c := count(t, s, `SELECT count(*) FROM sqlite_master WHERE name LIKE 'diagnostic_reviews%'`); c != 0 {
		t.Errorf("%d schema objects left after the rollback", c)
	}
	if _, err := p.UpTo(ctx, 13); err != nil {
		t.Fatalf("UpTo(13) again: %v", err)
	}
}

// Migration 00015 (phase 7.7): new form measures; the forms already measured are measured again (form_known
// back to 0), and the rollback keeps the rows.
func TestMigrationAbsoluteSignals(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 14); err != nil {
		t.Fatalf("UpTo(14): %v", err)
	}
	repo := mustExec(t, s, `INSERT INTO repositories (provider, provider_id, owner, name) VALUES ('github', 1, 'promo', 'a')`)
	mustExec(t, s, `INSERT INTO commits (repo_id, sha, authored_at, committed_at) VALUES (?, ?, ?, ?)`,
		repo, sha(1), testDate, testDate)
	mustExec(t, s, `INSERT INTO commit_signals (repo_id, sha, authored_at, computed_at, form_known) VALUES (?, ?, ?, ?, 1)`,
		repo, sha(1), testDate, testDate)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if c := count(t, s, "SELECT count(*) FROM commit_signals WHERE form_known = 0 AND wip_markers = 0"); c != 1 {
		t.Errorf("form not reset by the migration (%d)", c)
	}
	if err := execErr(t, s, "UPDATE commit_signals SET short_identifiers = -1"); err == nil {
		t.Error("negative measure accepted")
	}
	if _, err := p.DownTo(ctx, 14); err != nil {
		t.Fatalf("DownTo(14): %v", err)
	}
	if err := execErr(t, s, "SELECT wip_markers FROM commit_signals"); err == nil {
		t.Error("wip_markers still present after the rollback")
	}
	if c := count(t, s, "SELECT count(*) FROM commit_signals"); c != 1 {
		t.Errorf("%d rows after the rollback, want 1", c)
	}
}
