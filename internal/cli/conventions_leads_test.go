package cli

import (
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

// leadsRepo creates alice/projet on srv: a CHANGELOG with one comparison link out of two headings,
// commitlint and a CI workflow that cites it, a README with a command block, an INDEX with a declared
// date and a document with a broken image and a header value, then a code commit.
func leadsRepo(srv *gittest.Server) {
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
	r.Commit("main", "documentation/vision.md", "# Vision\n\nAuteur : Personne Secrète\n\n![schéma](media/absent.png)\n",
		"docs: ajoute la vision", who, at(57))
	r.Commit("main", "src/main.go", "package main\n", "feat: ajoute le programme", who, at(10))
}

// Leads (phase 8.7): tree leads cached by the sync on the local clone, history leads (DOC9, DOC10, TL5)
// and broken images in the detailed view of a repository only; outside every grid.
func TestConventionsLeadsLocalClone(t *testing.T) {
	srv := gittest.NewServer(t)
	leadsRepo(srv)
	h := newWorkspace(t)
	h.Client = authtest.NewLoggedOut()
	h.GitURL = srv.URL
	t.Setenv("WATCHOO_CONVENTIONS_ENABLED", "true")
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)

	// Overview: the leads to look at only, the command of the detail, no path.
	out := h.run("conventions").mustSucceed(t).stdout
	assertContains(t, "conventions (leads)", out, "Leads (they invite to reread, they do not judge): ",
		"CL11 to look at — CHANGELOG comparison links: missing_link (heading without a comparison link, 1) · 1/2 version heading(s) with a comparison link",
		"details of every lead: watchoo conventions alice/projet")
	assertNotContains(t, "conventions (leads)", out, "absent.png", "DOC9 ", "Personne Secrète")
	var rep conventionsReport
	decodeStrict(t, h.run("conventions", "--json").mustSucceed(t).stdout, &rep)
	leads := rep.Repositories[0].Leads
	if len(leads.Items) != len(conventions.LeadCriteria) || len(leads.BrokenImages) != 0 {
		t.Fatalf("overview leads: %+v", leads)
	}
	byID := map[string]conventionsLeadJSON{}
	for _, l := range leads.Items {
		byID[l.ID] = l
	}
	if l := byID["DOC9"]; l.Status != "unknown" || l.Reason == nil || *l.Reason != "detail_only" {
		t.Errorf("DOC9 overview: %+v", l)
	}
	if l := byID["CL11"]; l.Status != "review" || l.Counts["titles"] != 2 || l.Counts["linked"] != 1 {
		t.Errorf("CL11: %+v", l)
	}
	// No lead in a grid: the CHANGELOG grid keeps its criteria only.
	for _, c := range rep.Repositories[0].Changelog.Criteria {
		if c.ID == "CL11" || c.ID == "CL12" {
			t.Errorf("lead %s in the CHANGELOG grid", c.ID)
		}
	}

	// Detailed view: every lead, the history leads and the broken image.
	detail := h.run("conventions", "alice/projet").mustSucceed(t).stdout
	assertContains(t, "conventions alice/projet (leads)", detail,
		"RD5 nothing to look at — Installation in the README · 1 command block(s) under installation or run",
		"DOC7 to look at — Diagrams and images: broken_image (broken relative images, 1)",
		"DOC9 nothing to look at — Documentation freshness · 1 commit(s) outside the documentation since its last commit",
		"DOC10 to look at — Dates declared in the INDEX: declared_older",
		"TL4 nothing to look at — CI checking the conventions · 1 workflow(s), checks cited: commitlint",
		"TL5 not applicable — Effect of the tooling (sample too small)", "tooling introduced on ",
		"Broken images (DOC7, 1): documentation/vision.md → media/absent.png")
	assertNotContains(t, "conventions alice/projet (leads)", detail, "Personne Secrète", "2020-01-01")
	js := h.run("conventions", "alice/projet", "--json").mustSucceed(t).stdout
	decodeStrict(t, js, &rep)
	leads = rep.Repositories[0].Leads
	byID = map[string]conventionsLeadJSON{}
	for _, l := range leads.Items {
		byID[l.ID] = l
	}
	if len(leads.BrokenImages) != 1 || leads.BrokenImages[0].Source != "documentation/vision.md" ||
		leads.BrokenImages[0].Target != "media/absent.png" {
		t.Errorf("broken images: %+v", leads.BrokenImages)
	}
	if l := byID["DOC9"]; l.Status != "none" || l.Dates["docs"].IsZero() || l.Dates["code"].IsZero() ||
		l.Counts["commits_since"] != 1 {
		t.Errorf("DOC9: %+v", l)
	}
	if l := byID["DOC10"]; l.Status != "review" || l.Counts["compared"] != 1 || l.Counts["declared_older"] != 1 {
		t.Errorf("DOC10: %+v", l)
	}
	if l := byID["TL5"]; l.Status != "not_applicable" || l.Reason == nil || *l.Reason != "too_few" ||
		l.Dates["introduced"].IsZero() {
		t.Errorf("TL5: %+v", l)
	}
	if leads.ToReview < 3 {
		t.Errorf("to_review = %d", leads.ToReview)
	}
	assertNotContains(t, "conventions alice/projet --json (leads)", js, "Personne Secrète", "ajoute la vision")

	// With --contributor: no lead in the text.
	out = h.run("conventions", "--contributor", "Alice Martin").mustSucceed(t).stdout
	assertNotContains(t, "conventions --contributor (leads)", out, "Leads (")

	defer i18n.SetProcessForTest(i18n.French)()
	fr := h.run("conventions", "alice/projet").mustSucceed(t).stdout
	assertContains(t, "conventions (leads, fr)", fr, "Pistes (elles invitent à relire, elles ne jugent pas) : ",
		"CL11 à relire — Liens de comparaison du CHANGELOG : missing_link (titre sans lien de comparaison, 1)",
		"TL5 sans objet — Effet de l'outillage (effectif trop petit)",
		"Images cassées (DOC7, 1) : documentation/vision.md → media/absent.png")
}

// Every new reason of the CLI has its label in both languages.
func TestConventionsLeadReasons(t *testing.T) {
	en, fr := i18n.For(i18n.English), i18n.For(i18n.French)
	for _, r := range []string{conventions.UnknownDetailOnly, conventions.UnknownTooFew, conventions.UnknownIntroductionUnknown,
		conventions.UnknownNoTooling, conventions.UnknownNoDateColumn, conventions.UnknownNoVersion} {
		key := "cli.conv.reason." + r
		if en.T(key) == key || fr.T(key) == key || en.T(key) == fr.T(key) {
			t.Errorf("%s: %q / %q", key, en.T(key), fr.T(key))
		}
	}
}
