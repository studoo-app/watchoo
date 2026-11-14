package web

// Diagnostic pages (phase 7.4): list, review sheet, review form, history backfill button, disabled
// diagnosis, privacy of the note and of the diff.

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/diagnostic"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
)

// withDiagnostic enables diagnostic.enabled.
func withDiagnostic() envOption { return func(e *testEnv) { e.cfg.Diagnostic.Enabled = true } }

// diagnosticEnv returns an environment with the diagnosis enabled, the seeded repositories and a
// measured commit of Alice that declares an AI co-author.
func diagnosticEnv(t *testing.T, opts ...envOption) (*testEnv, seededRepos, string) {
	t.Helper()
	e := newTestEnv(t, append([]envOption{withDiagnostic()}, opts...)...)
	ids := e.seed()
	sha := providertest.SHA("commit-" + string(rune('a'+3)))
	if _, err := e.store.WriteCommitSignals(t.Context(), ids.Alice, []store.CommitSignalWrite{
		{SHA: sha, FilesKept: 2, AdditionsKept: 40, Tools: []string{"D1:claude"}},
		{SHA: providertest.SHA("commit-" + string(rune('a'+1))), FilesKept: 1, AdditionsKept: 3},
	}, e.now); err != nil {
		t.Fatal(err)
	}
	return e, ids, sha
}

func leadURL(repoID int64, sha string) string {
	return "/diagnostic/lead?" + url.Values{"repo": {strconv.FormatInt(repoID, 10)}, "key": {"commit/" + sha}}.Encode()
}

func TestDiagnosticDisabled(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	w := e.get("/diagnostic")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "The diagnosis is disabled in this workspace")
	assertNotContains(t, w, `href="/diagnostic"`, "Measure the history")
	if w := e.get("/"); strings.Contains(w.Body.String(), `href="/diagnostic"`) {
		t.Error("navigation shows the diagnosis while it is disabled")
	}
	assertStatus(t, e.get(leadURL(ids.Alice, providertest.SHA("x"))), http.StatusNotFound)
	assertStatus(t, e.post("/diagnostic/review", url.Values{"repo": {"1"}, "key": {"commit/x"}}, false), http.StatusNotFound)
	assertStatus(t, e.post("/diagnostic/backfill", nil, false), http.StatusNotFound)
}

func TestDiagnosticList(t *testing.T) {
	e, ids, sha := diagnosticEnv(t)
	w := e.get("/diagnostic")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `href="/diagnostic"`, "A lead is never a verdict", "Measure the history",
		shortSHA(sha), "declared", "D1 claude", "not reviewed", "Reviewed leads: 0 of 1",
		"do not have enough history yet", `badge badge-sm badge-info">declared`)
	assertNotContains(t, w, xssMessage, "corrige le bug")

	// Filter by repository: Bob has no lead.
	w = e.get("/diagnostic?repo=" + strconv.FormatInt(ids.Bob, 10))
	assertContains(t, w, "No reflection lead.")

	// French.
	f, _, _ := diagnosticEnv(t, withLang("fr"))
	assertContains(t, f.get("/diagnostic"), "Diagnostic", "Une piste n&#39;est jamais un verdict", "déclaré", "non revue")
}

func TestDiagnosticLeadAndReview(t *testing.T) {
	e, ids, sha := diagnosticEnv(t)
	w := e.get(leadURL(ids.Alice, sha))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "AI co-author trailer", "claude", `id="diag-throughput-data"`, `data-chart="daily-lines"`,
		"Neighbouring commits of the author", "Diff unavailable", "Lines behind the signals", `name="key" value="commit/`+sha+`"`)
	assertNotContains(t, w, "Reviewed by")
	assertStatus(t, e.get("/diagnostic/lead?repo=1&key=commit/absent"), http.StatusNotFound)
	assertStatus(t, e.get("/diagnostic/lead?repo=abc&key=x"), http.StatusNotFound)

	// Refused: no cause for "not assisted"; the input is kept.
	note := "<b>a vu</b> le commit en séance"
	form := url.Values{"repo": {strconv.FormatInt(ids.Alice, 10)}, "key": {"commit/" + sha},
		"decision": {"not_assisted"}, "confidence": {"high"}, "note": {note}}
	w = e.post("/diagnostic/review", form, false)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "Review not saved", "choose the cause", "&lt;b&gt;a vu&lt;/b&gt;")

	form.Set("cause", "refactoring")
	w = e.post("/diagnostic/review", form, false)
	assertStatus(t, w, http.StatusSeeOther)
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/diagnostic/lead?") || !strings.Contains(loc, "msg=saved") {
		t.Fatalf("redirect to %q", loc)
	}
	page, _, _ := strings.Cut(loc, "#") // a browser never sends the fragment
	w = e.get(page)
	assertContains(t, w, "Review saved.", "Reviewed by "+testWorkspace, "&lt;b&gt;a vu&lt;/b&gt;")
	// Reviewed: the level loses its badge, the decision gets one (list and repository page).
	for _, p := range []string{"/diagnostic", repoPath(ids.Alice)} {
		w = e.get(p)
		assertContains(t, w, `badge badge-sm badge-success">not assisted`, `text-base-content/60">declared`)
		assertNotContains(t, w, `badge badge-sm badge-info">declared`)
	}
	assertContains(t, e.get("/diagnostic"), "Reviewed leads: 1 of 1")
	reviews, err := e.store.DiagnosticReviews(t.Context(), ids.Alice)
	if r := reviews[store.ReviewKey(ids.Alice, "commit/"+sha)]; err != nil || r.Cause != "refactoring" ||
		r.Confidence != "high" || r.Reviewer != testWorkspace || r.Signals[0] != "D1" {
		t.Errorf("stored review: %+v, %v", r, err)
	}
	if strings.Contains(e.logs.String(), "a vu") {
		t.Error("the note is in the logs")
	}
	// Unknown lead.
	form.Set("key", "commit/absent")
	assertStatus(t, e.post("/diagnostic/review", form, false), http.StatusNotFound)
	form.Del("repo")
	assertStatus(t, e.post("/diagnostic/review", form, false), http.StatusBadRequest)
}

func TestDiagnosticBackfillButton(t *testing.T) {
	e, _, _ := diagnosticEnv(t)
	w := e.post("/diagnostic/backfill", nil, false)
	assertStatus(t, w, http.StatusSeeOther)
	if loc := w.Header().Get("Location"); loc != "/diagnostic?msg=backfill_started" {
		t.Fatalf("redirect to %q", loc)
	}
	e.server.backfill.wg.Wait()
	// The clones of the test environment are unreachable: every repository fails, the page says so.
	w = e.get("/diagnostic")
	assertContains(t, w, "Last history measurement", "Repositories not measured")
}

// Every signal and every level has its label, in both languages.
func TestDiagnosticLabels(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.English, i18n.French} {
		tr := i18n.For(lang)
		for _, id := range []string{"D1", "D2", "D3", "D5", "R1", "R2", "R3", "R4", "R5", "R6", "F1", "F2", "F3", "F4", "F5",
			"A1", "A2", "A3", "A4", "A5", "A6", "A7", "H1", "H2", "H3", "H4"} {
			if key := "web.diag.signal." + id; tr.T(key) == key {
				t.Errorf("%s: no label for %s", lang, key)
			}
		}
		for _, l := range []string{diagnostic.LevelDeclared, diagnostic.LevelProbable, diagnostic.LevelToReview} {
			for _, key := range []string{"web.diag.level." + l, "cli.diag.level." + l} {
				if tr.T(key) == key {
					t.Errorf("%s: no label for %s", lang, key)
				}
			}
		}
	}
}

// Repository page (1.6.0): the diagnosis card shows the score next to the levels and the signals, only when
// the diagnosis is enabled.
func TestRepoDiagnostic(t *testing.T) {
	e, ids, sha := diagnosticEnv(t)
	path := "/repos/" + strconv.FormatInt(ids.Alice, 10)
	w := e.get(path)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="repo-diagnostic"`, "Score of the repository", "A lead is never a verdict",
		"AI co-author trailer", leadURLHTML(ids.Alice, sha), `href="/diagnostic?repo=`+strconv.FormatInt(ids.Alice, 10)+`"`,
		"Reviewed leads: 0 of 1", "Latest leads (1)")
	if !strings.Contains(w.Body.String(), `<div class="stat-value tabular-nums">10</div>`) {
		t.Error("score 10 (one D1 lead) not shown")
	}

	// HTMX fragment after a sync.
	w = e.htmxWith(path, map[string]string{"HX-Target": "repo-diagnostic"})
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="repo-diagnostic"`, "Score of the repository")
	assertNotContains(t, w, `id="repo-commits"`)

	// A repository without lead.
	assertContains(t, e.get("/repos/"+strconv.FormatInt(ids.Bob, 10)), `id="repo-diagnostic"`, "No reflection lead.")

	f, ids2, _ := diagnosticEnv(t, withLang("fr"))
	assertContains(t, f.get("/repos/"+strconv.FormatInt(ids2.Alice, 10)), "Score du dépôt", "trailer de co-auteur IA")
}

func TestRepoDiagnosticDisabled(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	assertNotContains(t, e.get("/repos/"+strconv.FormatInt(ids.Alice, 10)), `id="repo-diagnostic"`, "Score of the repository")
}

func leadURLHTML(repoID int64, sha string) string {
	return strings.ReplaceAll(leadURL(repoID, sha), "&", "&amp;")
}

// Contributors list (1.6.0): the "AI score" column replaces the AI signatures, with the levels next to it.
func TestContributorsAIScore(t *testing.T) {
	e, _, _ := diagnosticEnv(t)
	w := e.get("/contributors")
	assertStatus(t, w, http.StatusOK)
	// Score 10: the "medium" icon written in the page, the score in its center; score 0: the icon alone.
	// The levels are only in the tooltip.
	assertContains(t, w, "AI score", `<img src="/static/score-ia.svg" alt="AI score 10, medium"`,
		`<span class="col-start-1 row-start-1 text-xs font-bold tabular-nums leading-none" aria-hidden="true">10</span>`,
		`<img src="/static/score-not-ia.svg" alt="AI score 0, no lead"`,
		`title="AI score 10 (medium): 1 declared · 0 probable · 0 to review"`, "the score is not a grade")
	assertNotContains(t, w, "AI signatures", "the diagnosis is disabled", `aria-hidden="true">0</span>`,
		`<div class="text-xs text-base-content/60">1 declared`)
	assertContains(t, e.get("/fragments/contributors"), `alt="AI score 10, medium"`)
	for _, icon := range []string{"score-ia-max.svg", "score-ia.svg", "score-not-ia.svg"} {
		w := e.get("/static/" + icon)
		assertStatus(t, w, http.StatusOK)
		if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
			t.Errorf("%s: Content-Type %q", icon, ct)
		}
	}

	f, _, _ := diagnosticEnv(t, withLang("fr"))
	assertContains(t, f.get("/contributors"), "Score IA", `alt="Score IA 10, moyen"`, "Score IA 10 (moyen) : 1 déclaré · 0 probable · 0 à regarder")
}

// Contributor page: the "AI score" card replaces the AI signatures, with the same score as the
// list (leads of the contributor's commits).
func TestContributorPageAIScore(t *testing.T) {
	e, ids, sha := diagnosticEnv(t)
	scored := 0
	for _, id := range e.contributorIDs() {
		w := e.get(fmt.Sprintf("/contributors/%d", id))
		assertStatus(t, w, http.StatusOK)
		assertContains(t, w, "AI score", "the score is not a grade")
		assertNotContains(t, w, "AI signatures", "the diagnosis is disabled")
		if strings.Contains(w.Body.String(), `alt="AI score 10, medium"`) {
			scored++
			assertContains(t, w, `title="AI score 10 (medium): 1 declared · 0 probable · 0 to review"`,
				"1 declared · 0 probable · 0 to review")
		} else {
			assertContains(t, w, `alt="AI score 0, no lead"`)
		}
	}
	if scored != 1 {
		t.Errorf("%d contributor pages with the score 10, want 1", scored)
	}

	// Commits of the contributor: the commit with a lead carries the badge of its level and the link to
	// its review sheet, the others nothing; the HTMX fragment of the history too.
	lead := leadURLHTML(ids.Alice, sha)
	marked := 0
	for _, id := range e.contributorIDs() {
		page := fmt.Sprintf("/contributors/%d", id)
		body := e.get(page).Body.String()
		if n := strings.Count(body, `href="`+lead+`"`); n > 0 {
			marked++
			if n != 1 {
				t.Errorf("%s: %d links to the lead, want 1", page, n)
			}
			assertContains(t, e.get(page), "Lead: declared")
			assertContains(t, e.htmx(page+"/commits"), `href="`+lead+`"`)
		} else if strings.Contains(body, "/diagnostic/lead?") {
			t.Errorf("%s: a lead link on a commit without lead", page)
		}
	}
	if marked != 1 {
		t.Errorf("%d contributor pages mark the commit, want 1", marked)
	}
	assertStatus(t, e.get(leadURL(ids.Alice, sha)), http.StatusOK)

	f, _, _ := diagnosticEnv(t, withLang("fr"))
	for _, id := range f.contributorIDs() {
		assertContains(t, f.get(fmt.Sprintf("/contributors/%d", id)), "Score IA", "à regarder")
	}

	// Diagnosis disabled: no badge on the commits.
	off := newTestEnv(t)
	off.seed()
	for _, id := range off.contributorIDs() {
		assertNotContains(t, off.get(fmt.Sprintf("/contributors/%d", id)), "/diagnostic/lead?", "Lead: ")
	}
}

func TestAIScoreIcon(t *testing.T) {
	funcs := diagnosticFuncs(i18n.For(i18n.English))
	icon := funcs["aiScoreIcon"].(func(int) string)
	tier := funcs["aiScoreTier"].(func(int) string)
	for score, want := range map[int][2]string{0: {"/static/score-not-ia.svg", "no lead"}, 1: {"/static/score-ia.svg", "medium"},
		29: {"/static/score-ia.svg", "medium"}, 30: {"/static/score-ia-max.svg", "high"}} {
		if got := [2]string{icon(score), tier(score)}; got != want {
			t.Errorf("score %d: %v, want %v", score, got, want)
		}
	}
}

// The evidence rows follow the signals of the lead, once each, and leave out the signals without lines.
func TestEvidenceRows(t *testing.T) {
	tr := i18n.For(i18n.French)
	hits := []diagnostic.Hit{{Signal: "A1"}, {Signal: "A5"}, {Signal: "H1"}, {Signal: "H1", Value: "x"}}
	ev := map[string]diagnostic.Evidence{
		"A5": {Lines: []diagnostic.EvidenceLine{{Path: "a.go", Line: 3, Text: "// ════"}}},
		"H1": {Lines: []diagnostic.EvidenceLine{{Path: "a.go", Line: 4, Text: "// TODO"}}, More: 2},
		"F1": {Lines: []diagnostic.EvidenceLine{{Path: "a.go", Line: 3}}}, // not a signal of the lead
	}
	rows := evidenceRows(tr, hits, ev)
	if len(rows) != 2 || rows[0].Signal != "A5" || rows[1].Signal != "H1" || rows[1].More != 2 ||
		rows[0].Description != "lignes de commentaire décoratives" {
		t.Errorf("evidence rows: %+v", rows)
	}
}

// Every decision has its own badge color in the list of the leads.
func TestDiagnosticDecisionClass(t *testing.T) {
	class := diagnosticFuncs(i18n.For(i18n.English))["diagDecisionClass"].(func(string) string)
	want := map[string]string{diagnostic.DecisionAssisted: "badge-primary", diagnostic.DecisionNotAssisted: "badge-success",
		diagnostic.DecisionUndetermined: "badge-neutral"}
	for d, c := range want {
		if got := class(d); got != c {
			t.Errorf("diagDecisionClass(%q) = %q, want %q", d, got, c)
		}
	}
}

// TestDiagnosticContributorLabel checks that the leads show the name given to the author (1.8.0) in
// the list, on the review sheet and on the repository page, while the detection keeps the Git name.
func TestDiagnosticContributorLabel(t *testing.T) {
	e, ids, sha := diagnosticEnv(t)
	if _, err := e.store.SetContributorLabel(t.Context(), e.contributorIDs()["Alice Martin"], "Alice M. (TP2)"); err != nil {
		t.Fatal(err)
	}
	assertContains(t, e.get("/diagnostic"), "Alice M. (TP2)", shortSHA(sha))
	assertContains(t, e.get(leadURL(ids.Alice, sha)), "Alice M. (TP2)")
	assertContains(t, e.get(repoPath(ids.Alice)), "Alice M. (TP2)")
	// The author filter accepts the label.
	rep, err := diagnostic.Build(t.Context(), e.store, store.DiagnosticFilter{Author: "alice m. (tp2)"}, diagnostic.RhythmOptions{})
	if err != nil || len(rep.Leads) != 1 || rep.Leads[0].AuthorLabel != "Alice M. (TP2)" ||
		rep.Leads[0].AuthorName != "Alice Martin" || rep.Leads[0].AuthorDisplayName() != "Alice M. (TP2)" {
		t.Errorf("leads by label: %+v, %v", rep.Leads, err)
	}
}
