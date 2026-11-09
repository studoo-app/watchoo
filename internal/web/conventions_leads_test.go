package web

// Conventions, phase 8.7: leads (CL11, CL12, RD5, DOC6, DOC7, DOC9, DOC10, TL4, TL5) in a folded block of
// the repository section, never in a grid, never with a conform or not conform badge; nothing in
// /conventions nor on a contributor's page; labels of every new key in both languages.

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

// leadsBlock returns the folded block of the leads of a conventions section.
func leadsBlock(t *testing.T, section string) string {
	t.Helper()
	start := strings.Index(section, `id="repo-conventions-leads"`)
	if start < 0 {
		t.Fatal("no leads block")
	}
	end := strings.Index(section[start:], "</details>")
	if end < 0 {
		t.Fatal("leads block not closed")
	}
	return section[start : start+end]
}

func TestRepoConventionsLeads(t *testing.T) {
	e, ids := conventionsEnv(t)
	// Nothing computed yet, no clone: every lead unknown.
	block := leadsBlock(t, conventionsSection(t, e.get(repoPath(ids.Alice)).Body.String()))
	for _, want := range []string{"Leads: 0 to look at", "Leads invite to reread, they do not judge",
		`id="repo-conventions-lead-CL11"`, "CHANGELOG comparison links", `id="repo-conventions-lead-TL5"`,
		"Effect of the tooling", "unknown", "(no local clone)"} {
		if !strings.Contains(block, want) {
			t.Errorf("leads block without %q", want)
		}
	}

	e.cacheFiles(ids.Alice)
	section := conventionsSection(t, e.get(repoPath(ids.Alice)).Body.String())
	block = leadsBlock(t, section)
	for _, want := range []string{`<details class="collapse collapse-arrow bg-base-200/40 rounded-box" id="repo-conventions-leads">`,
		"to look at", "badge-info", "<code>missing_link</code> heading without a comparison link (1 item)",
		"0/1 version heading(s) with a comparison link", "Copy of git log in the CHANGELOG", "not applicable",
		"(sample too small)", "Documentation freshness", "(no local clone)"} {
		// The opening tag of the block is in the section, before the block; the rest must be in the block.
		in := block
		if strings.HasPrefix(want, "<details") {
			in = section
		}
		if !strings.Contains(in, want) {
			t.Errorf("leads block without %q", want)
		}
	}
	// A lead never judges: neither a conform nor a not conform badge in the block.
	for _, judged := range []string{"badge-success", "badge-error", "criteria conform"} {
		if strings.Contains(block, judged) {
			t.Errorf("leads block with %q", judged)
		}
	}
	assertNoConventionsLeak(t, "leads block", block)

	// Neither /conventions nor the contributor page shows a lead.
	assertNotContains(t, e.get("/conventions"), "repo-conventions-leads", "CHANGELOG comparison links", "missing_link")
	assertNotContains(t, e.get(contributorPath(e.contributorIDs()["Alice Martin"])), "repo-conventions-leads",
		"CHANGELOG comparison links")

	fr, ids2 := conventionsEnv(t, withLang("fr"))
	fr.cacheFiles(ids2.Alice)
	block = leadsBlock(t, conventionsSection(t, fr.get(repoPath(ids2.Alice)).Body.String()))
	for _, want := range []string{"Pistes : ", " à relire", "elles ne jugent pas", "Liens de comparaison du CHANGELOG",
		"titre sans lien de comparaison (1 élément)", "sans objet", "(effectif trop petit)", "(pas de clone local)"} {
		if !strings.Contains(block, want) {
			t.Errorf("French leads block without %q", want)
		}
	}
}

// Every lead, status, code, header, date, new reason and list has its label in both languages.
func TestConventionsLabelsPhase87(t *testing.T) {
	families := map[string][]string{
		"conv.lead.":        conventions.LeadCriteria,
		"conv.lead_status.": {conventions.LeadNone, conventions.LeadReview, conventions.LeadNotApplicable, conventions.CriterionUnknown},
		"conv.lead_code.":   conventions.AllLeadCodes,
		"conv.lead_header.": conventions.AllHeaderFields,
		"conv.lead_date.":   conventions.AllLeadDates,
		"conv.lead_detail.": {"CL11", "CL12", "RD5", "env_example_present", "env_example_absent", "DOC6", "DOC6_fields",
			"DOC7", "media_present", "media_absent", "DOC9", "DOC10", "DOC10_over", "TL4", "TL4_none", "TL5", "TL5_sources"},
		"web.conv.reason.": {conventions.UnknownDetailOnly, conventions.UnknownTooFew, conventions.UnknownIntroductionUnknown,
			conventions.UnknownNoTooling, conventions.UnknownNoDateColumn, conventions.UnknownNoVersion},
		"web.conv.list.": {"broken_images"},
		"web.conv.":      {"leads_title", "leads_note", "lead_count.one", "lead_count.other"},
	}
	en, fr := i18n.For(i18n.English), i18n.For(i18n.French)
	// Same word in both languages on purpose.
	same := map[string]bool{"conv.lead_header.date": true, "conv.lead_header.version": true,
		"conv.lead_detail.TL4_none": true}
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

// End to end, phase 8.7: a repository tracked by local clone (gh signed out); the sync caches the leads of
// the tree, the repository page computes DOC9, DOC10 and TL5 on the history of the clone and lists the
// broken image. Neither a path, a header value nor a commit subject reaches the logs.
func TestWebEndToEndConventionsLeads(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	who := gittest.Author{Name: "Alice Martin", Email: "alice@example.test"}
	at := func(h int) time.Time { return testNow.Add(-time.Duration(h) * time.Hour) }
	r.Commit("main", "CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n## [1.0.0] - 2026-10-01\n\n### Added\n\n"+
		"- Première version.\n\n[Unreleased]: https://github.com/alice/projet/compare/v1.0.0...HEAD\n",
		"docs: ajoute le changelog", who, at(80))
	r.Commit("main", "commitlint.config.js", "module.exports = { extends: ['@commitlint/config-conventional'] };\n",
		"chore: ajoute commitlint", who, at(70))
	r.Commit("main", ".github/workflows/ci.yml", "on: pull_request\njobs:\n  lint:\n    runs-on: ubuntu-latest\n"+
		"    steps:\n      - uses: wagoid/commitlint-github-action@v6\n", "ci: verifie les commits", who, at(69))
	r.Commit("main", "README.md", "# Projet\n\n## Installation\n\n```bash\nmake\n```\n", "docs: ajoute le readme", who, at(60))
	r.Commit("main", "documentation/INDEX.md", "# Index\n\n| Document | Dernière mise à jour |\n|---|---|\n"+
		"| [Vision](vision.md) | 2020-01-01 |\n", "docs: ajoute l'index", who, at(58))
	r.Commit("main", "documentation/vision.md", "# Vision\n\nAuteur : Personne Secrète\n\n![schéma](media/e2e-absent.png)\n",
		"docs: ajoute la vision", who, at(57))
	r.Commit("main", "src/main.go", "package main\n", "feat: ajoute le programme", who, at(10))

	e := newTestEnv(t, withConventions(), withGitURL(srv.URL), func(e *testEnv) { e.auth = authtest.NewLoggedOut() })
	w := e.post("/settings/repos", url.Values{"repo": {"alice/projet"}}, false)
	assertStatus(t, w, http.StatusSeeOther)
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	id, err := strconv.ParseInt(loc.Query().Get("repo"), 10, 64)
	if err != nil {
		t.Fatalf("repository identifier %q", loc.Query().Get("repo"))
	}
	assertStatus(t, e.post("/sync", nil, true), http.StatusOK)
	if st := e.waitSyncIdle(); !st.HasReport {
		t.Fatalf("sync report = %+v", st.LastReport)
	}

	w = e.get(repoPath(id))
	assertStatus(t, w, http.StatusOK)
	block := leadsBlock(t, conventionsSection(t, w.Body.String()))
	for _, want := range []string{"1/2 version heading(s) with a comparison link", "1 command block(s) under installation or run",
		"<code>broken_image</code>", "Broken images (DOC7): 1", `id="repo-conventions-list-broken_images"`,
		"documentation/vision.md → media/e2e-absent.png", "1 commit(s) outside the documentation since its last commit",
		"last documentation commit on ", "code on ", "<code>declared_older</code>", "1 INDEX row(s), 1 compared",
		"checks cited: commitlint", "tooling introduced on ", "(sample too small)"} {
		if !strings.Contains(block, want) {
			t.Errorf("leads block without %q", want)
		}
	}
	// No header value nor INDEX cell is shown.
	if strings.Contains(block, "Personne Secrète") || strings.Contains(block, "2020-01-01") {
		t.Error("header value or INDEX cell in the leads block")
	}
	// /conventions: no lead, no path.
	assertNotContains(t, e.get("/conventions"), "e2e-absent.png", "repo-conventions-leads")
	for _, leak := range []string{"e2e-absent.png", "vision.md", "Personne Secrète", "commitlint", "ajoute la vision",
		"ajoute le programme"} {
		if strings.Contains(e.logs.String(), leak) {
			t.Errorf("%q in the logs", leak)
		}
	}
}
