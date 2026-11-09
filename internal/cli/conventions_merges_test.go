package cli

// « watchoo conventions » (phase 8.5): pull request bodies (PR5), merge method (PR6), reused branches
// (BR2), allowed merge methods (BR5), pull requests from a fork set aside, commits outside a pull
// request (PR3) in text and --json. Neither a pull request body nor a title nor a commit message ever
// appears.

import (
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
)

// mergeBody is the description of PR #1: every section and "Closes #3"; never written by the command.
const mergeBody = "## Contexte\n\nLa page manque.\n\n## Ce qui change\n\nUn formulaire.\n\n## Comment tester\n\n" +
	"Ouvrir /login.\n\n## Captures\n\n![page](x.png)\n\nCloses #3"

// mergeWorkspace returns a synced workspace (API) with the conventions enabled: on alice/projet, PR #1
// open with a complete body, #2 squashed ("(#2)", conforming message), #3 open from a fork whose head
// branch is "Main", #4 merged with a merge commit from the same head branch as #2; only the squash is
// allowed. Phase 8.6: #2 approved by another account before its merge, #4 only approved by its author.
func mergeWorkspace(t *testing.T) *harness {
	t.Helper()
	h := newWorkspace(t)
	t.Setenv("WATCHOO_CONVENTIONS_ENABLED", "true")
	f := h.Provider
	seedProvider(f)
	f.UpdateRepository(provider.Repository{ID: aliceRepoID, Owner: "alice", Name: "projet", Private: true,
		URL: "https://github.example.test/alice/projet", AllowSquashMerge: provider.True,
		AllowMergeCommit: provider.False, AllowRebaseMerge: provider.False})
	squash := addCommits(f, aliceRepoID, "main", providertest.SHA("a2"),
		fakeCommit{"s2", alicePerson, 90 * time.Minute, "fix: corrige le build (#2)"})
	at := testNow.Add(-30 * time.Minute)
	merge := provider.Commit{SHA: providertest.SHA("m4"), Message: "Merge pull request #4 from alice/fix/build",
		Author: alicePerson, Committer: alicePerson, AuthoredAt: at, CommittedAt: at, Parents: []string{squash, providertest.SHA("a3")}}
	f.AddCommits(aliceRepoID, merge)
	f.SetBranch(aliceRepoID, "main", merge.SHA)
	alice := provider.Account{Login: "alice-gh", ID: 2001}
	pr := func(n int, title, head string, created time.Duration) provider.PullRequest {
		return provider.PullRequest{Number: n, Title: title, State: provider.PullRequestOpen, Author: alice,
			URL: "https://github.example.test/alice/projet/pull/" + string(rune('0'+n)), BaseBranch: "main", HeadBranch: head,
			CreatedAt: testNow.Add(-created), UpdatedAt: testNow.Add(-created), FromFork: provider.False}
	}
	merged := func(p provider.PullRequest, sha string, ago time.Duration) provider.PullRequest {
		p.State, p.Merged, p.MergeCommitSHA = provider.PullRequestClosed, true, sha
		p.ClosedAt, p.MergedAt, p.UpdatedAt = testNow.Add(-ago), testNow.Add(-ago), testNow.Add(-ago)
		return p
	}
	open := pr(1, "feat: ajoute la page de connexion", "feat/add-login", 5*time.Hour)
	open.Body = mergeBody
	fork := pr(3, "feat: ajoute un theme", "Main", 4*time.Hour)
	fork.FromFork = provider.True
	f.SetPullRequests(aliceRepoID, open,
		merged(pr(2, "fix: corrige le build", "fix/build", 30*time.Hour), squash, 90*time.Minute),
		fork,
		merged(pr(4, "fix: corrige encore le build", "fix/build", 3*time.Hour), merge.SHA, 30*time.Minute))
	bob := provider.Account{Login: "bob-reviewer", ID: 2002}
	f.SetPullRequestReviews(aliceRepoID, 2,
		provider.PullRequestReview{ID: 1, Reviewer: bob, State: provider.ReviewCommented, SubmittedAt: testNow.Add(-3 * time.Hour)},
		provider.PullRequestReview{ID: 2, Reviewer: bob, State: provider.ReviewApproved, SubmittedAt: testNow.Add(-2 * time.Hour)})
	f.SetPullRequestReviews(aliceRepoID, 4,
		provider.PullRequestReview{ID: 3, Reviewer: alice, State: provider.ReviewApproved, SubmittedAt: testNow.Add(-time.Hour)})
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)
	return h
}

func TestConventionsMergeFlow(t *testing.T) {
	h := mergeWorkspace(t)
	never := append([]string{"La page manque", "Un formulaire", "Ouvrir /login", "Closes #3", "corrige le build",
		"ajoute la page de connexion", "ajoute un theme", "corrige encore"}, conventionMessages...)
	out := h.run("conventions").mustSucceed(t).stdout
	assertContains(t, "conventions (8.5)", out,
		"set aside: from a fork 1",
		"pull request bodies: 1/3 conform (0 unknown)",
		`missing: #4 (context, changes, how to test, screenshots, "Closes #n"), #2 (`,
		"merges: 1/2 squashed with a conforming message (squash 1, merge commit 1, rebase 0, unknown 0)",
		"reused branches (1): fix/build (#2, #4)",
		"allowed merge methods: squash (conform)",
		"commits outside PR: unknown (no local clone)",
		"merged pull requests reviewed by another account: 1/2 (0 not read yet)",
		"merged without review (1): #4 (",
		"1/2, 2 review(s) received",
		"PR BODIES", "OUTSIDE PR", "PR REVIEWED")
	assertNotContains(t, "conventions (8.5)", out, never...)
	// No reviewer login is kept, so none is shown.
	assertNotContains(t, "conventions (8.6)", out, "bob-reviewer")
	// The fork's head branch is not a branch of the repository.
	assertNotContains(t, "conventions (8.5)", out, "Main (#3)")

	js := h.run("conventions", "--json").mustSucceed(t).stdout
	assertNotContains(t, "conventions --json (8.5)", js, never...)
	assertNotContains(t, "conventions --json (8.6)", js, "bob-reviewer")
	assertJSONKeys(t, js, "repositories.0.pull_requests", "known", "reason", "open", "merged", "skipped", "items", "methods",
		"merged_without_review")
	assertJSONKeys(t, js, "repositories.0.pull_requests.merged", "evaluated", "conform", "default", "body_known",
		"body_conform", "merge_known", "squash", "merge_conform", "review_known", "reviewed", "reviews_received")
	assertJSONKeys(t, js, "repositories.0.pull_requests.items.0", "number", "category", "author", "head_branch", "codes",
		"default_codes", "conform", "body", "merge", "review")
	assertJSONKeys(t, js, "repositories.0.pull_requests.items.0.review", "known", "reviews", "approvals", "reviewed")
	assertJSONKeys(t, js, "repositories.0.pull_requests.merged_without_review.0", "number", "author")
	assertJSONKeys(t, js, "repositories.0.pull_requests.items.0.body", "known", "found", "missing", "closes", "conform")
	assertJSONKeys(t, js, "repositories.0.pull_requests.items.0.merge", "method", "message_conform", "conform")
	assertJSONKeys(t, js, "repositories.0.branches.reused", "known", "reason", "items")
	assertJSONKeys(t, js, "repositories.0.branches.reused.items.0", "name", "pull_requests")
	assertJSONKeys(t, js, "repositories.0.merge_methods", "known", "reason", "squash", "merge", "rebase", "conform")
	assertJSONKeys(t, js, "repositories.0.arrivals", "known", "reason", "mode", "complete", "by_pr", "outside_pr",
		"undetermined", "reasons", "outside")
	assertJSONKeys(t, js, "contributors.0.arrivals", "by_pr", "outside_pr", "undetermined")
	assertJSONKeys(t, js, "contributors.0.pull_requests.merged", "evaluated", "conform", "default", "body_known",
		"body_conform", "merge_known", "squash", "merge_conform", "review_known", "reviewed", "reviews_received")
	var rep conventionsReport
	decodeStrict(t, js, &rep)
	a := rep.Repositories[0]
	p := a.PullRequests
	if p.Skipped["fork"] != 1 || p.Merged.Squash != 1 || p.Merged.MergeConform != 1 || p.Merged.MergeKnown != 2 ||
		p.Open.BodyKnown != 1 || p.Open.BodyConform != 1 || p.Methods["squash"] != 1 || p.Methods["merge"] != 1 {
		t.Errorf("pull requests: %+v", p)
	}
	// PR4: #2 reviewed (bob approved after a comment), #4 approved by its author only.
	if p.Merged.ReviewKnown != 2 || p.Merged.Reviewed != 1 || p.Merged.ReviewsReceived != 2 ||
		len(p.MergedWithoutReview) != 1 || p.MergedWithoutReview[0].Number != 4 {
		t.Errorf("reviews: %+v %+v", p.Merged, p.MergedWithoutReview)
	}
	for _, it := range p.Items {
		switch it.Number {
		case 1:
			if !it.Body.Conform || len(it.Body.Found) != 4 || !it.Body.Closes || it.Merge != nil {
				t.Errorf("#1: %+v", it)
			}
		case 2:
			if it.Merge == nil || it.Merge.Method != "squash" || !it.Merge.Conform || !it.Body.Known || it.Body.Conform ||
				it.Review != (conventionsReviewJSON{Known: true, Reviews: 2, Approvals: 1, Reviewed: true}) {
				t.Errorf("#2: %+v", it)
			}
		case 4:
			if it.Review != (conventionsReviewJSON{Known: true}) {
				t.Errorf("#4: %+v", it.Review)
			}
		case 3:
			t.Errorf("fork evaluated: %+v", it)
		}
	}
	if !a.Branches.Reused.Known || len(a.Branches.Reused.Items) != 1 || a.Branches.Reused.Items[0].Name != "fix/build" {
		t.Errorf("reused: %+v", a.Branches.Reused)
	}
	if m := a.MergeMethods; !m.Known || !m.Conform || m.Squash == nil || !*m.Squash || *m.Merge || *m.Rebase {
		t.Errorf("merge methods: %+v", m)
	}
	if ar := a.Arrivals; ar.Known || *ar.Reason != "no_clone" || len(ar.Outside) != 0 || len(ar.Reasons) != 11 {
		t.Errorf("arrivals: %+v", ar)
	}

	// Milestone review (phase 8.6): PR4 on the pull requests merged before the end of the due date (no tag v2.0.0).
	t.Setenv("WATCHOO_CONVENTIONS_SINCE", "2020-01-01")
	t.Setenv("WATCHOO_CONVENTIONS_TAGS_MILESTONES", "v2.0.0@2026-10-27")
	ms := h.run("conventions", "--milestone", "v2.0.0").mustSucceed(t).stdout
	assertContains(t, "--milestone (8.6)", ms,
		"PR4  not conform — 1/2 merged pull requests reviewed (0 not read yet) (#4)")
	assertNotContains(t, "--milestone (8.6)", ms, "bob-reviewer")
	mjs := h.run("conventions", "--milestone", "v2.0.0", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, mjs, "milestone.repositories.0.reviews", "merged", "known", "reviewed", "unreviewed")
	var mrep conventionsReport
	decodeStrict(t, mjs, &mrep)
	if m := mrep.Milestone.Repositories[0]; m.Reviews == nil || m.Reviews.Merged != 2 || m.Reviews.Known != 2 ||
		m.Reviews.Reviewed != 1 || len(m.Reviews.Unreviewed) != 1 || m.Reviews.Unreviewed[0] != 4 || m.Criteria[3].ID != "PR4" ||
		m.Criteria[3].Status != "nonconform" {
		t.Errorf("milestone PR4: %+v", m)
	}

	defer i18n.SetProcessForTest(i18n.French)()
	ms = h.run("conventions", "--milestone", "v2.0.0").mustSucceed(t).stdout
	assertContains(t, "--milestone (8.6, fr)", ms, "1/2 pull requests fusionnées relues (0 pas encore lues) (#4)")
	t.Setenv("WATCHOO_CONVENTIONS_TAGS_MILESTONES", "")
	fr := h.run("conventions").mustSucceed(t).stdout
	assertContains(t, "conventions (8.5, fr)", fr, "écartées : issues d'un fork 1",
		"corps des pull requests : 1/3 conformes (0 inconnus)",
		"fusions : 1/2 en squash au message conforme (squash 1, merge commit 1, rebase 0, inconnue 0)",
		"branches réutilisées (1) : fix/build (#2, #4)", "méthodes de fusion autorisées : squash (conforme)",
		"commits hors PR : inconnu (pas de clone local)",
		"pull requests fusionnées relues par un autre compte : 1/2 (0 pas encore lues)",
		"fusionnées sans relecture (1) : #4 (", "1/2, 2 relecture(s) reçue(s)",
		"CORPS DE PR", "HORS PR", "PR RELUES")
	assertNotContains(t, "conventions (8.5, fr)", fr, never...)
}

// Local clones (no gh): PR3 read on the first parent of the default branch, on the messages of GitHub;
// the commits pushed directly are outside a pull request, attributed to their author; the milestone
// review judges PR3 = 0 on its interval.
func TestConventionsArrivalsLocalClone(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	alice := gittest.Author{Name: "Alice Martin", Email: "alice@example.test"}
	github := gittest.Author{Name: "GitHub", Email: "noreply@github.com"}
	r.Commit("main", "a.txt", "1\n", "feat: initialise le projet", alice, testNow.Add(-50*time.Hour))
	r.Branch("feat/menu", "main")
	r.Commit("feat/menu", "b.txt", "1\n", "feat: ajoute le menu", alice, testNow.Add(-45*time.Hour))
	merge := r.Merge("main", "feat/menu", "Merge pull request #1 from alice/feat/menu", github, testNow.Add(-40*time.Hour))
	r.Tag("v1.0.0", "main", "")
	direct := r.Commit("main", "a.txt", "2\n", "fix: corrige en direct", alice, testNow.Add(-30*time.Hour))
	r.Tag("v2.0.0", "main", "")

	h := newWorkspace(t)
	h.Client = authtest.NewLoggedOut()
	h.GitURL = srv.URL
	t.Setenv("WATCHOO_CONVENTIONS_ENABLED", "true")
	t.Setenv("WATCHOO_CONVENTIONS_SINCE", "2020-01-01")
	t.Setenv("WATCHOO_CONVENTIONS_TAGS_MILESTONES", "v1.0.0,v2.0.0")
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)

	out := h.run("conventions").mustSucceed(t).stdout
	assertContains(t, "conventions (arrivals)", out,
		"commits outside PR on the default branch: 1 of 2 (by PR 1, undetermined 0); clone only: read on the messages of GitHub",
		provider.ShortSHA(direct)+" Alice Martin (author): direct commit on the branch",
		"allowed merge methods: unknown (no data from the GitHub API)")
	assertNotContains(t, "conventions (arrivals)", out, provider.ShortSHA(merge)+" ", "corrige en direct", "ajoute le menu")
	js := h.run("conventions", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, js, "repositories.0.arrivals.outside.0", "sha", "short_sha", "reason", "author")
	var rep conventionsReport
	decodeStrict(t, js, &rep)
	a := rep.Repositories[0].Arrivals
	if !a.Known || a.Mode != "clone" || a.Complete || a.ByPR != 1 || a.OutsidePR != 1 || len(a.Outside) != 1 ||
		a.Outside[0].SHA != direct || a.Outside[0].Reason != "human_committer" || a.Outside[0].Author.Name != "Alice Martin" ||
		a.Reasons["github_merge"] != 1 {
		t.Errorf("arrivals: %+v", a)
	}
	if c := rep.Contributors[0].Arrivals; c.OutsidePR != 1 {
		t.Errorf("contributor arrivals: %+v", c)
	}

	ms := h.run("conventions", "--milestone", "v1.0.0").mustSucceed(t).stdout
	assertContains(t, "--milestone v1.0.0", ms, "PR3  conform — 0 outside PR of 1 commits (0 undetermined)")
	ms = h.run("conventions", "--milestone", "v2.0.0").mustSucceed(t).stdout
	assertContains(t, "--milestone v2.0.0", ms, "PR3  not conform — 1 outside PR of 1 commits (0 undetermined) ("+
		provider.ShortSHA(direct)+")")
	mjs := h.run("conventions", "--milestone", "v2.0.0", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, mjs, "milestone.repositories.0.arrivals", "known", "reason", "mode", "complete", "by_pr",
		"outside_pr", "undetermined", "reasons", "outside")
	assertJSONKeys(t, mjs, "milestone.repositories.0.arrivals.outside.0", "sha", "short_sha", "reason", "author")
	var mrep conventionsReport
	decodeStrict(t, mjs, &mrep)
	if m := mrep.Milestone.Repositories[0]; m.Arrivals == nil || m.Arrivals.OutsidePR != 1 || m.Criteria[2].ID != "PR3" ||
		m.Criteria[2].Status != "nonconform" {
		t.Errorf("milestone arrivals: %+v", m.Arrivals)
	}
	defer i18n.SetProcessForTest(i18n.French)()
	fr := h.run("conventions").mustSucceed(t).stdout
	assertContains(t, "conventions (arrivals, fr)", fr,
		"commits hors PR sur la branche par défaut : 1 sur 2 (par PR 1, indéterminés 0) ; clone seul : lu sur les messages de GitHub",
		provider.ShortSHA(direct)+" Alice Martin (auteur) : commit direct sur la branche")
}
