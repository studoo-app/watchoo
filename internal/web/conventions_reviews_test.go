package web

// Conventions, phase 8.6: reviews of the pull requests (PR4) on the repository page (merges block and
// badge per pull request), in /conventions (repositories, contributors, milestone review) and on the
// contributor page; labels in both languages; counters only, never a reviewer, a title or a message.

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/store"
)

// seedReviews records the review counters of seedMerges' merged pull requests: #2 approved by another
// account (two reviews), #4 reviewed once without approval.
func (e *testEnv) seedReviews(repoID int64) {
	e.t.Helper()
	for _, w := range []store.PullRequestReviewsWrite{
		{RepoID: repoID, Number: 2, ReviewsByOthers: 2, ApprovalsByOthers: 1, ApprovalsByContributors: 1, CheckedAt: e.now},
		{RepoID: repoID, Number: 4, ReviewsByOthers: 1, CheckedAt: e.now},
	} {
		if _, err := e.store.SetPullRequestReviews(e.t.Context(), w); err != nil {
			e.t.Fatalf("SetPullRequestReviews: %v", err)
		}
	}
}

// reviewsEnv is an environment whose clock is the day after the merges, with a milestone v9.0.0 due the
// day of the merges (no tag): its interval holds the two merged pull requests.
func reviewsEnv(t *testing.T, opts ...envOption) (*testEnv, seededRepos) {
	t.Helper()
	opts = append([]envOption{func(e *testEnv) {
		e.now = time.Date(2026, 10, 28, 3, 0, 0, 0, time.UTC)
		e.cfg.Conventions.Since = "2000-01-01"
		e.cfg.Conventions.Tags.Milestones = []string{"v9.0.0@2026-10-27"}
	}}, opts...)
	e, ids := conventionsEnv(t, opts...)
	e.seedMerges(ids.Alice)
	e.seedReviews(ids.Alice)
	return e, ids
}

func TestRepoConventionsReviews(t *testing.T) {
	e, ids := reviewsEnv(t)
	section := conventionsSection(t, e.get(repoPath(ids.Alice)).Body.String())
	for _, want := range []string{`id="repo-conventions-reviews"`,
		"Merged pull requests reviewed by another account: 1/2 (0 not read yet).", "Merged without review (1): #4 (",
		"Only counters are kept, never a reviewer.",
		// Badges of the pull requests: review column, #4 listed.
		"Review", "merged without review", "title, description, merge or review"} {
		if !strings.Contains(section, want) {
			t.Errorf("conventions section without %q", want)
		}
	}
	assertNoMergeLeak(t, "repository page", section)

	w := e.get("/conventions")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Merged PRs reviewed", "1/2 reviewed", "Reviews received", "1/2 reviewed · 3 received",
		"the reviews given are not counted")
	// Milestone review: PR4 on the card of the repository, and the result.
	w = e.get("/conventions?milestone=v9.0.0")
	assertContains(t, w, "PR4", "merged pull requests reviewed",
		"PR4: 1/2 merged pull requests reviewed (0 not read yet) (#4)")
	assertNoMergeLeak(t, "/conventions", w.Body.String())

	w = e.get(contributorPath(e.contributorIDs()["Alice Martin"]))
	assertContains(t, w, `id="contributor-conventions-reviews"`,
		"Their merged pull requests reviewed by another account: 1/2; reviews received on their pull requests: 3")

	fr, ids2 := reviewsEnv(t, withLang("fr"))
	section = conventionsSection(t, fr.get(repoPath(ids2.Alice)).Body.String())
	for _, want := range []string{"Pull requests fusionnées relues par un autre compte : 1/2 (0 pas encore lues).",
		"Fusionnées sans relecture (1) : #4 (", "Relecture", "fusionnée sans relecture"} {
		if !strings.Contains(section, want) {
			t.Errorf("French conventions section without %q", want)
		}
	}
	assertContains(t, fr.get("/conventions"), "PR fusionnées relues", "1/2 relues · 3 reçues")
	assertContains(t, fr.get("/conventions?milestone=v9.0.0"),
		"PR4 : 1/2 pull requests fusionnées relues (0 pas encore lues) (#4)")
	assertContains(t, fr.get(contributorPath(fr.contributorIDs()["Alice Martin"])),
		"Ses pull requests fusionnées relues par un autre compte : 1/2 ; relectures reçues sur ses pull requests : 3")
}

// Before any read of the reviews: unknown, nothing listed, no badge "merged without review".
func TestRepoConventionsReviewsUnread(t *testing.T) {
	e, ids := conventionsEnv(t)
	e.seedMerges(ids.Alice)
	section := conventionsSection(t, e.get(repoPath(ids.Alice)).Body.String())
	for _, want := range []string{"Merged pull requests reviewed by another account: 0/0 (2 not read yet).",
		"reviews not read yet"} {
		if !strings.Contains(section, want) {
			t.Errorf("conventions section without %q", want)
		}
	}
	if strings.Contains(section, "Merged without review") {
		t.Error("merged without review listed before any read")
	}
	assertContains(t, e.get(contributorPath(e.contributorIDs()["Alice Martin"])),
		"Reviews of their pull requests: none read")
}

// Only the open pull request #1 has its reviews read: the reviews received are shown alone, never a
// "0/0" rate of merged pull requests (same rule as the CLI).
func TestConventionsReviewsOpenOnly(t *testing.T) {
	for _, lang := range []string{"en", "fr"} {
		e, ids := conventionsEnv(t, withLang(lang), func(e *testEnv) { e.cfg.Conventions.Since = "2000-01-01" })
		e.seedMerges(ids.Alice)
		if _, err := e.store.SetPullRequestReviews(t.Context(), store.PullRequestReviewsWrite{
			RepoID: ids.Alice, Number: 1, ReviewsByOthers: 2, CheckedAt: e.now}); err != nil {
			t.Fatalf("SetPullRequestReviews: %v", err)
		}
		conv, contributor := "2 received", "Reviews received on their pull requests: 2 (no merged pull request read yet)"
		if lang == "fr" {
			conv, contributor = "2 reçues", "Relectures reçues sur ses pull requests : 2 (aucune pull request fusionnée lue)"
		}
		w := e.get("/conventions")
		assertContains(t, w, conv)
		if body := w.Body.String(); strings.Contains(body, "0/0 reviewed ·") || strings.Contains(body, "0/0 relues ·") {
			t.Errorf("%s: /conventions shows a 0/0 rate for the contributor", lang)
		}
		assertContains(t, e.get(contributorPath(e.contributorIDs()["Alice Martin"])), contributor)
	}
}

// Every code and reason of phase 8.6 has its web and CLI label in both languages (keys built at run time).
func TestConventionsLabelsPhase86(t *testing.T) {
	reasons := []string{conventions.UnknownReviewsUnchecked, conventions.UnknownNoMergedPR, conventions.UnknownNoTagDate}
	families := map[string][]string{
		"web.conv.criterion.": {conventions.CriterionPR4},
		"web.conv.reason.":    reasons,
		"cli.conv.reason.":    reasons,
	}
	en, fr := i18n.For(i18n.English), i18n.For(i18n.French)
	for prefix, codes := range families {
		for _, c := range codes {
			key := prefix + c
			if en.T(key) == key || fr.T(key) == key {
				t.Errorf("no label for %s", key)
			} else if en.T(key) == fr.T(key) {
				t.Errorf("%s: same label in English and French", key)
			}
		}
	}
}
