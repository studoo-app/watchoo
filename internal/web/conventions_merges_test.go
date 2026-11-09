package web

// Conventions, phase 8.5: description (PR5) and merge (PR6) badges of the pull requests, reused branches
// (BR2), allowed merge methods (BR5), pull requests from a fork set aside, commits outside a pull request
// (PR3) on the repository page, in /conventions and on the contributor page; labels in both languages;
// never a pull request description, title or commit message.

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// mergeTitles are the titles of the pull requests of seedMerges: never shown.
var mergeTitles = []string{"Titre secret un", "Titre secret deux", "Titre secret fork", "Titre secret quatre",
	"corrige le build"}

// seedMerges records, for alice's repository, a sync by the API with a squash commit (#2), a merge
// commit (#4, same head branch as #2), PR #1 open with a complete description, #3 open from a fork, and
// only the squash allowed.
func (e *testEnv) seedMerges(repoID int64) {
	e.t.Helper()
	ctx := e.t.Context()
	run, err := e.store.BeginSyncRun(ctx, repoID, e.now.Add(-20*time.Minute), wsync.LockStaleAfter)
	if err != nil {
		e.t.Fatal(err)
	}
	alice := store.Identity{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh", ProviderUserID: 10}
	one, two := 1, 2
	squash := e.commitWrite(8, alice, 3*time.Hour, "fix: corrige le build (#2)", "main")
	squash.ParentCount = &one
	merge := e.commitWrite(9, alice, 2*time.Hour, "Merge pull request #4 from alice-gh/fix-build", "main")
	merge.ParentCount = &two
	at := e.now.Add(-4 * time.Hour)
	full := conventions.CheckPRBody("## Context\n\nWhy.\n\n## Changes\n\nWhat.\n\n## How to test\n\nRun.\n\n## Screenshots\n\nNone.\n\nCloses #9")
	empty := int64(0)
	no, yes := false, true
	pull := func(n int, title, head string) model.PullRequest {
		return model.PullRequest{Number: n, Title: title, State: model.PullRequestOpen, AuthorLogin: "alice-gh",
			AuthorProviderID: 10, URL: "https://github.com/promo/alice/pull/" + string(rune('0'+n)), BaseBranch: "main",
			HeadBranch: head, CreatedAt: at, UpdatedAt: at.Add(time.Duration(n) * time.Minute), FromFork: &no,
			BodySections: &empty}
	}
	merged := func(p model.PullRequest, sha string) model.PullRequest {
		p.State, p.Merged, p.MergeCommitSHA = model.PullRequestClosed, true, sha
		p.ClosedAt, p.MergedAt = p.UpdatedAt, p.UpdatedAt
		return p
	}
	open := pull(1, "feat: Titre secret un", "feat/add-login")
	open.BodySections = &full
	fork := pull(3, "feat: Titre secret fork", "Main")
	fork.FromFork = &yes
	if _, err := e.store.ApplyRepoSync(ctx, store.RepoSyncWrite{
		RepoID: repoID, RunID: run.ID,
		Metadata: store.RepositoryMetadata{Owner: "promo", Name: "alice", DefaultBranch: "main",
			URL: "https://github.com/promo/alice", AllowSquashMerge: &yes, AllowMergeCommit: &no, AllowRebaseMerge: &no},
		Branches: []store.BranchState{{Name: "main", HeadSHA: merge.SHA}},
		Commits:  []store.CommitWrite{squash, merge},
		PullRequests: []model.PullRequest{open,
			merged(pull(2, "fix: Titre secret deux", "fix/build"), squash.SHA), fork,
			merged(pull(4, "fix: Titre secret quatre", "fix/build"), merge.SHA)},
		PullRequestsVersion: conventions.PRRulesVersion,
		SyncedAt:            e.now.Add(-19 * time.Minute),
	}); err != nil {
		e.t.Fatalf("ApplyRepoSync: %v", err)
	}
}

func assertNoMergeLeak(t *testing.T, where, body string) {
	t.Helper()
	for _, m := range append(append([]string{}, conventionMessages...), mergeTitles...) {
		if strings.Contains(body, m) {
			t.Errorf("%s: %q shown", where, m)
		}
	}
}

func TestRepoConventionsMerges(t *testing.T) {
	e, ids := conventionsEnv(t)
	e.seedMerges(ids.Alice)
	w := e.get(repoPath(ids.Alice))
	assertStatus(t, w, http.StatusOK)
	section := conventionsSection(t, w.Body.String())
	for _, want := range []string{`id="repo-conventions-merges"`, "Merges",
		"Commits outside pull requests: unknown (no local clone).",
		"Pull request descriptions: 1/3 conforming (0 unknown).",
		"Merged pull requests squashed with a conforming message: 1/2", "squash 1, merge commit 1",
		"Head branches merged more than once (1):", "fix/build</span> (#2, #4)",
		`id="repo-conventions-merge-methods"`, "squash only", "set aside", "from a fork 1",
		// Badges of the pull requests: description and merge.
		"Pull requests to look at: title, description, merge or review", "missing: context, changes, how to test, screenshots",
		"PR description", "Merge", "merge commit", "attributed to the author of the commit only"} {
		if !strings.Contains(section, want) {
			t.Errorf("conventions section without %q", want)
		}
	}
	// The fork's head branch is not evaluated by BR1.
	if strings.Contains(section, ">Main<") {
		t.Error("head branch of a fork in BR1")
	}
	assertNoMergeLeak(t, "repository page", section)

	w = e.get("/conventions")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Outside PR, merge methods", "Commits outside PR", "squash only", "unknown (no local clone)",
		"no commit outside a pull request")
	assertNoMergeLeak(t, "/conventions", w.Body.String())

	w = e.get(contributorPath(e.contributorIDs()["Alice Martin"]))
	assertContains(t, w, `id="contributor-conventions-outside-pr"`, "Commits outside a pull request: unknown")

	fr, ids2 := conventionsEnv(t, withLang("fr"))
	fr.seedMerges(ids2.Alice)
	section = conventionsSection(t, fr.get(repoPath(ids2.Alice)).Body.String())
	for _, want := range []string{"Fusions", "Commits hors pull request : inconnu (pas de clone local).",
		"Descriptions des pull requests : 1/3 conformes (0 inconnues).",
		"Pull requests fusionnées en squash au message conforme : 1/2", "Branches de tête fusionnées plus d&#39;une fois (1) :",
		"squash seul", "issues d&#39;un fork 1", "manque : contexte"} {
		if !strings.Contains(section, want) {
			t.Errorf("French conventions section without %q", want)
		}
	}
}

// Every code, status and reason of phase 8.5 has its web label in both languages (keys built at run time).
func TestConventionsLabelsPhase85(t *testing.T) {
	families := map[string][]string{
		"web.conv.merge_method.":   conventions.AllMergeMethods,
		"web.conv.arrival_reason.": conventions.AllArrivalReasons,
		"web.conv.pr_section.":     append(conventions.AllPRBodySections(), conventions.PRSectionCloses),
		"web.conv.reason.":         {conventions.UnknownNoPushAccess, conventions.UnknownUndetermined},
		"web.conv.pr_skip.":        {conventions.PRSkipFork},
		"web.conv.criterion.":      {conventions.CriterionPR3},
	}
	en, fr := i18n.For(i18n.English), i18n.For(i18n.French)
	same := map[string]bool{"web.conv.merge_method.squash": true, "web.conv.merge_method.merge": true,
		"web.conv.merge_method.rebase": true}
	for prefix, codes := range families {
		for _, c := range codes {
			key := prefix + c
			if en.T(key) == key || fr.T(key) == key {
				t.Errorf("no label for %s", key)
			} else if en.T(key) == fr.T(key) && !same[key] {
				t.Errorf("%s: same label in English and French", key)
			}
		}
	}
	// The CLI labels of the same codes, in both languages.
	for _, c := range conventions.AllArrivalReasons {
		if key := "cli.conv.arrival_reason." + c; en.T(key) == key || fr.T(key) == key || en.T(key) == fr.T(key) {
			t.Errorf("CLI label %s", key)
		}
	}
	for _, c := range conventions.AllMergeMethods {
		if key := "cli.conv.merge_method." + c; en.T(key) == key || fr.T(key) == key {
			t.Errorf("CLI label %s", key)
		}
	}
	for _, c := range append(conventions.AllPRBodySections(), conventions.PRSectionCloses) {
		if key := "cli.conv.pr_section." + c; en.T(key) == key || fr.T(key) == key || en.T(key) == fr.T(key) {
			t.Errorf("CLI label %s", key)
		}
	}
	for _, c := range []string{conventions.UnknownNoPushAccess, conventions.UnknownUndetermined} {
		if key := "cli.conv.reason." + c; en.T(key) == key || fr.T(key) == key || en.T(key) == fr.T(key) {
			t.Errorf("CLI label %s", key)
		}
	}
}
