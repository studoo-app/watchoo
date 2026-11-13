package web

// Conventions, phase 8.3: file criteria (CHANGELOG, README, documentation, tooling) read from the cache of
// the sync, grids of the repository section, summary column of /conventions, file criteria of the
// milestone review; labels of every new code in both languages.

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
)

// fileTree is an in-memory tree: CHANGELOG without [Unreleased], README, documentation without INDEX.
func fileTree() conventions.FileTree {
	files := map[string]string{
		"CHANGELOG.md":                       "# Changelog\n\nKeep a Changelog, SemVer.\n\n## [1.0.0] - 2026-10-01\n\n### Added\n\n- Version.\n",
		"README.md":                          "# Projet\n\n## Installation\n\n`make`\n",
		"documentation/01-cadrage/vision.md": "# Vision\n",
	}
	tree := conventions.FileTree{Blobs: map[string]conventions.Blob{}}
	for p, c := range files {
		tree.Paths = append(tree.Paths, p)
		tree.Blobs[p] = conventions.Blob{Data: []byte(c)}
	}
	slices.Sort(tree.Paths)
	return tree
}

// cacheFiles writes the file criteria of fileTree as the result of the sync for the head of the default
// branch of repoID.
func (e *testEnv) cacheFiles(repoID int64) {
	e.t.Helper()
	ctx := e.t.Context()
	branches, err := e.store.ListBranches(ctx, repoID)
	if err != nil {
		e.t.Fatal(err)
	}
	head := ""
	for _, b := range branches {
		if b.Name == "main" {
			head = b.HeadSHA
		}
	}
	tags, err := e.store.ConventionTags(ctx, repoID)
	if err != nil {
		e.t.Fatal(err)
	}
	repo, err := e.store.GetRepository(ctx, repoID)
	if err != nil {
		e.t.Fatal(err)
	}
	rules := e.cfg.Conventions.FileRules()
	res, _ := conventions.CheckFiles(fileTree(), conventions.FileOptions{Rules: rules, Repo: repo.FullName(), Now: testNow})
	raw, err := conventions.EncodeFileResults(res)
	if err != nil {
		e.t.Fatal(err)
	}
	err = e.store.SaveRepositoryConventions(ctx, model.RepositoryConventions{RepoID: repoID, RevKind: model.ConventionRevHead,
		RevSHA: head, RulesVersion: conventions.FileRulesVersion, ConfigHash: conventions.FileConfigHash(rules, repo.FullName(), tags),
		Results: raw, ComputedAt: testNow.Add(-time.Hour)})
	if err != nil {
		e.t.Fatal(err)
	}
}

func TestRepoConventionsFiles(t *testing.T) {
	e, ids := conventionsEnv(t)
	// Nothing computed yet, and no clone: unknown.
	section := conventionsSection(t, e.get(repoPath(ids.Alice)).Body.String())
	for _, want := range []string{`id="repo-conventions-files"`, "File criteria unknown: no local clone.",
		"CHANGELOG · 0/8 criteria conform, 8 unknown", "Tooling · 0/3 criteria conform, 3 unknown"} {
		if !strings.Contains(section, want) {
			t.Errorf("conventions section without %q", want)
		}
	}

	e.cacheFiles(ids.Alice)
	section = conventionsSection(t, e.get(repoPath(ids.Alice)).Body.String())
	for _, want := range []string{"Measured by the sync on the commit ", "CHANGELOG · 6/8 criteria conform, 0 unknown",
		"<code>unreleased_absent</code> no [Unreleased] section", "README · 1/4 criteria conform, 0 unknown",
		"sections found 1/8", "Documentation · 3/6 criteria conform, 2 unknown", "INDEX present",
		"<code>absent</code> file not found", "(file absent)", "Tooling · 0/3 criteria conform, 0 unknown",
		"Path lists unavailable: no local clone.", "credited to nobody"} {
		if !strings.Contains(section, want) {
			t.Errorf("conventions section without %q", want)
		}
	}
	assertNoConventionsLeak(t, "repository section", section)

	// Overview: a summary per family, the reason when unknown.
	w := e.get("/conventions")
	assertContains(t, w, "Files", "CHANGELOG 6/8", "README 1/4", "Documentation 3/6 · 2 unknown", "Tooling 0/3",
		"unknown (no local clone)")

	// The contributor page never shows a file criterion (repository indicators).
	assertNotContains(t, e.get(contributorPath(e.contributorIDs()["Alice Martin"])), "CHANGELOG", "repo-conventions-files")

	fr, ids2 := conventionsEnv(t, withLang("fr"))
	fr.cacheFiles(ids2.Alice)
	section = conventionsSection(t, fr.get(repoPath(ids2.Alice)).Body.String())
	for _, want := range []string{"Fichiers : CHANGELOG, README, documentation, outillage",
		"CHANGELOG · 6/8 critères conformes, 0 inconnus", "aucune section [Unreleased]", "Outillage · 0/3",
		"sections trouvées 1/8", "Listes de chemins indisponibles : pas de clone local.", "attribués à personne"} {
		if !strings.Contains(section, want) {
			t.Errorf("French conventions section without %q", want)
		}
	}
}

// Stale result: the settings changed since the computation (readme.min_words 10 here, 150 in the cache).
func TestRepoConventionsFilesStale(t *testing.T) {
	e, ids := conventionsEnv(t, func(e *testEnv) { e.cfg.Conventions.Readme.MinWords = 10 })
	repo, err := e.store.GetRepository(t.Context(), ids.Alice)
	if err != nil {
		t.Fatal(err)
	}
	rules := e.cfg.Conventions.FileRules()
	rules.ReadmeMinWords = 150
	res, _ := conventions.CheckFiles(fileTree(), conventions.FileOptions{Rules: rules})
	raw, err := conventions.EncodeFileResults(res)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.SaveRepositoryConventions(t.Context(), model.RepositoryConventions{RepoID: ids.Alice,
		RevKind: model.ConventionRevHead, RevSHA: strings.Repeat("a", 40), RulesVersion: conventions.FileRulesVersion,
		ConfigHash: conventions.FileConfigHash(rules, repo.FullName(), nil), Results: raw, ComputedAt: testNow}); err != nil {
		t.Fatal(err)
	}
	assertContains(t, e.get(repoPath(ids.Alice)),
		"File criteria unknown: to recompute (rules, settings or revision changed), run watchoo sync.")
}

// The milestone review adds the file criteria, with their labels, measured on the tree of the tag.
func TestConventionsReviewFiles(t *testing.T) {
	e, _ := conventionsPullsEnv(t, func(e *testEnv) { e.cfg.Conventions.Since = "2020-01-01" })
	w := e.get("/conventions")
	// One card per repository: each criterion with its identifier and its label (to look at, or among the
	// conforming ones), never a column per criterion.
	assertContains(t, w, "CL9", "CHANGELOG section of the milestone", "RD2", "installation section of the README",
		"DOC3", "INDEX exhaustive", "no commit outside a pull request", "/14 criteria conforming")
	assertNotContains(t, w, "<th scope=\"col\">CL9")
}

// Every file code, file criterion and new reason has its label in both languages (keys built at run time).
func TestConventionsLabelsPhase83(t *testing.T) {
	var criteria []string
	for _, fam := range [][]string{conventions.ChangelogCriteria, conventions.ReadmeCriteria, conventions.DocsCriteria,
		conventions.ToolingCriteria} {
		criteria = append(criteria, fam...)
	}
	families := map[string][]string{
		"conv.file_code.":      conventions.AllFileCodes,
		"conv.criterion.":      criteria,
		"conv.family.":         {conventions.FamilyChangelog, conventions.FamilyReadme, conventions.FamilyDocs, conventions.FamilyTooling},
		"conv.readme_section.": conventions.AllReadmeSections(),
		"web.conv.criterion.":  conventions.AllCriteria,
		"web.conv.reason.": {conventions.UnknownEmptyRepository, conventions.UnknownTreeTruncated, conventions.UnknownNoFile,
			conventions.UnknownBlobTooLarge, conventions.UnknownBlobUnreadable, conventions.UnknownNoDocsDir,
			conventions.UnknownUnreadDocuments, conventions.UnknownNotComputed, conventions.UnknownStale},
		"web.conv.list.": {"unreferenced", "duplicate_rows", "dead_links", "bad_names", "archives", "office_documents"},
	}
	en, fr := i18n.For(i18n.English), i18n.For(i18n.French)
	// Same word in both languages on purpose.
	same := map[string]bool{"conv.family.changelog": true, "conv.family.readme": true, "conv.family.docs": true,
		"conv.criterion.DOC8": true, "conv.readme_section.install": true}
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
