package web

// Conventions, phase 8.2: the /conventions page (overview by repository and by contributor, milestone
// review), its navigation entry, and the extended repository section and contributor lines; never a commit
// message nor a pull request title.

import (
	"html"
	"net/http"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/store"
)

// prTitles are the titles of the pull requests of seedPulls: never shown by the conventions.
var prTitles = []string{xssTitle, html.EscapeString(xssTitle), "PR piégée", "Brouillon de PR", "PR mergée", "PR abandonnée"}

// conventionsPullsEnv returns conventionsEnv plus, in alice's repository, the pull requests, release and
// tags of seedPulls (head branch feature-x, tags v1.0 and v0.9, a pre-release v1.0).
func conventionsPullsEnv(t *testing.T, opts ...envOption) (*testEnv, seededRepos) {
	t.Helper()
	e, ids := conventionsEnv(t, opts...)
	e.seedPulls(ids.Alice, "alice", e.commitWrite(6, store.Identity{}, 0, "", "main").SHA)
	return e, ids
}

func assertNoConventionsLeak(t *testing.T, where, body string) {
	t.Helper()
	for _, m := range append(append([]string{}, conventionMessages...), prTitles...) {
		if strings.Contains(body, m) {
			t.Errorf("%s: %q shown", where, m)
		}
	}
}

func TestConventionsPageDisabled(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	w := e.get("/conventions")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "The conventions are disabled.", "conventions.enabled: true", "WATCHOO_CONVENTIONS_ENABLED=true")
	assertNotContains(t, w, `id="conventions-repos"`, `id="conventions-milestone"`, "Alice Martin")
	// No navigation entry while disabled, one when enabled.
	assertNotContains(t, e.get("/"), `href="/conventions"`)
	on, _ := conventionsEnv(t)
	assertContains(t, on.get("/"), `href="/conventions"`)
	assertContains(t, on.get("/conventions"), `href="/conventions" class="whitespace-nowrap menu-active"`, `aria-current="page">Conventions`)
	// Disabled, a milestone parameter is ignored (no data, no error).
	assertStatus(t, e.get("/conventions?milestone=v9.9.9"), http.StatusOK)
}

func TestConventionsPage(t *testing.T) {
	e, ids := conventionsPullsEnv(t)
	w := e.get("/conventions")
	assertStatus(t, w, http.StatusOK)
	body := w.Body.String()
	assertContains(t, w, `id="conventions-repos"`, `id="conventions-contributors"`, `id="conventions-milestone"`,
		"By repository", "By contributor", "Alice Martin", `href="`+repoPath(ids.Alice)+`"`,
		// alice: 1/4 conforming commits, 0/1 open and 0/1 merged PR titles, 0/2 tags.
		"1/4 (25%)", "open 0/1", "merged 0/1", "0/2",
		// Milestones: v1.0 designates v1.0.0 under a nonconforming name.
		"v1.0.0 · nonconforming name", "v2.0.0 · absent", `aria-hidden="true"`,
		// Contributors: Alice is linked to her page, never sorted by conformity.
		`href="`+contributorPath(e.contributorIDs()["Alice Martin"])+`"`, "never sorted by conformity",
		// Milestone review: first milestone selected by default; no start date: not evaluable.
		`<option value="v1.0.0" selected>`, "Review of the milestone v1.0.0", "Not evaluable: missing start date.",
		// No local clone: no trend.
		"unknown (no clone or no final tag)")
	assertNoConventionsLeak(t, "/conventions", body)
	if strings.Index(body, "Alice Martin") > strings.Index(body, "promo/bob") {
		t.Error("repositories are not in the order of the workspace")
	}

	// With a start date: the criteria of the review.
	s, _ := conventionsPullsEnv(t, func(e *testEnv) { e.cfg.Conventions.Since = "2000-01-01" })
	w = s.get("/conventions?milestone=v1.0.0")
	assertContains(t, w, "CC1", "BR1", "SV1", "SV2", "criteria conforming", "not conforming", "unknown",
		"no local clone", "✗")
	assertNoConventionsLeak(t, "/conventions?milestone", w.Body.String())
	assertContains(t, s.get("/conventions?milestone=v2.0.0"), `<option value="v2.0.0" selected>`, "Review of the milestone v2.0.0")

	// A milestone that is not configured: a bad request, not a review.
	w = e.get("/conventions?milestone=v9.9.9")
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "Milestone &#34;v9.9.9&#34; is not configured")

	// No milestone configured.
	none, _ := conventionsEnv(t, func(e *testEnv) { e.cfg.Conventions.Tags.Milestones = []string{} })
	assertContains(t, none.get("/conventions"), "No milestone configured")

	// French.
	fr, _ := conventionsPullsEnv(t, withLang("fr"))
	w = fr.get("/conventions")
	assertContains(t, w, "Par dépôt", "Par contributeur", "Revue du jalon v1.0.0", "Non évaluable : date de début manquante.",
		"ouvertes 0/1", "fusionnées 0/1", "nom non conforme", "jamais triés par conformité")
	assertContains(t, fr.get("/conventions?milestone=v9.9.9"), "Le jalon &#34;v9.9.9&#34; n&#39;est pas configuré")
}

func TestRepoConventionsPhase82(t *testing.T) {
	e, ids := conventionsPullsEnv(t)
	w := e.get(repoPath(ids.Alice))
	assertStatus(t, w, http.StatusOK)
	section := conventionsSection(t, w.Body.String())
	for _, want := range []string{
		// BR1: the head branch feature-x has no type prefix; no clone: tracked branches of the database.
		`id="repo-conventions-branches"`, "0/1 branches conforming", "feature-x", "<code>no_slash</code>",
		"no &#34;type/&#34; prefix", "#1, #2, #3, #4", "partial list",
		// PR1: number and codes, never the title; the draft and the closed PR are not evaluated.
		`id="repo-conventions-pulls"`, "open 0/1", "merged 0/1", "#1", "#3", "<code>no_type</code>",
		"Not evaluated:", "drafts 1", "closed without merge 1", "neither the title nor the description is ever shown",
		// SV1, SV2, SV6.
		`id="repo-conventions-tags"`, "0/2 tags conforming", "v1.0 · not conforming", "<code>missing_component</code>",
		"v1.0.0 · nonconforming name", "(v1.0)", "v2.0.0 · absent", "Releases (information):", "inconsistent pre-release flag",
		// Intervals: no clone.
		`id="repo-conventions-intervals"`, "Unknown: no local clone.",
		// CC6, CC7.
		`id="repo-conventions-traits"`, "Footers: 0 of 4 commits; issues cited: 0", "Language of the subjects",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("conventions section without %q", want)
		}
	}
	assertNoConventionsLeak(t, "repository page", section)

	fr, ids2 := conventionsPullsEnv(t, withLang("fr"))
	section = conventionsSection(t, fr.get(repoPath(ids2.Alice)).Body.String())
	for _, want := range []string{"0/1 branches conformes", "pas de préfixe « type/ »", "ouvertes 0/1",
		"brouillons 1", "0/2 tags conformes", "nom non conforme", "drapeau de pré-version incohérent", "Inconnu : pas de clone local."} {
		if !strings.Contains(section, want) {
			t.Errorf("French conventions section without %q", want)
		}
	}
}

func TestRepoConventionsLocalOnly(t *testing.T) {
	// gh not signed in: the workspace syncs from the clones only; PRs and releases are unknown.
	e, ids := conventionsPullsEnv(t, func(e *testEnv) { e.auth = authtest.NewLoggedOut() })
	section := conventionsSection(t, e.get(repoPath(ids.Alice)).Body.String())
	for _, want := range []string{"Unknown: no data from the GitHub API.", "unknown (no data from the GitHub API)"} {
		if !strings.Contains(section, want) {
			t.Errorf("conventions section without %q", want)
		}
	}
	assertContains(t, e.get("/conventions"), "from the local clones only")
}

func TestContributorConventionsPhase82(t *testing.T) {
	e, _ := conventionsPullsEnv(t)
	alice := e.contributorIDs()["Alice Martin"]
	w := e.get(contributorPath(alice))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="contributor-conventions-pulls"`, "Conforming pull request titles:", "open 0/1", "merged 0/1",
		`id="contributor-conventions-trend"`, "before / since the last final tag", "unknown (no clone or no final tag)")
	body := w.Body.String()
	start, end := strings.Index(body, `id="contributor-conventions"`), strings.Index(body, `id="contributor-conventions-trend"`)
	if start < 0 || end < start {
		t.Fatal("no conventions lines on the contributor page")
	}
	assertNoConventionsLeak(t, "contributor page", body[start:end+200])

	// Bob has no pull request.
	assertContains(t, e.get(contributorPath(e.contributorIDs()["Bob Durand"])), "no pull request evaluated")

	fr, _ := conventionsPullsEnv(t, withLang("fr"))
	assertContains(t, fr.get(contributorPath(fr.contributorIDs()["Alice Martin"])), "Titres de pull request conformes :",
		"ouvertes 0/1", "avant / depuis le dernier tag final")
}

// Every code, status and reason of phase 8.2 has its web label in both languages (keys built at run time).
func TestConventionsLabelsPhase82(t *testing.T) {
	families := map[string][]string{
		"web.conv.branch_code.":      conventions.AllBranchCodes,
		"web.conv.tag_code.":         conventions.AllTagCodes,
		"web.conv.pr_default.":       conventions.AllPRTitleCodes,
		"web.conv.pr_skip.":          conventions.AllPRSkips,
		"web.conv.release_code.":     conventions.AllReleaseCodes,
		"web.conv.wording.":          conventions.AllWordingCodes,
		"web.conv.criterion.":        conventions.AllCriteria,
		"web.conv.milestone_status.": {conventions.MilestonePresent, conventions.MilestoneNonconformingName, conventions.MilestoneAbsent},
		"web.conv.criterion_status.": {conventions.CriterionConform, conventions.CriterionNonconform, conventions.CriterionUnknown},
		"web.conv.language.":         {conventions.LanguageFR, conventions.LanguageEN},
		"web.conv.reason.": {conventions.UnknownNoClone, conventions.UnknownCloneUnreadable, conventions.UnknownNoAPI,
			conventions.UnknownFiltered, conventions.UnknownNoFinalTag, conventions.UnknownNoSince, conventions.UnknownNoTag,
			conventions.UnknownNotDue, conventions.UnknownTagAbsent, conventions.UnknownNoCommits, conventions.UnknownPartial, conventions.UnknownNoBranch},
	}
	en, fr := i18n.For(i18n.English), i18n.For(i18n.French)
	same := map[string]bool{"web.conv.pr_skip.bot": true, "web.conv.milestone_status.absent": true}
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
}
