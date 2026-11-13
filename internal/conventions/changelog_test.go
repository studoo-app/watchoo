package conventions

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

const goodChangelog = `# Changelog

All notable changes are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.1.0] - 2026-03-01

A short introduction is allowed.

### Added

- New page.

### Fixed

- Broken link.

## [1.0.0] - 2026-01-15

### Added

- First release.

[Unreleased]: https://github.com/promo/projet/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/promo/projet/compare/v1.0.0...v1.1.0
`

// changelogChecks mesure CHANGELOG.md seul, avec les tags donnés.
func changelogChecks(content string, tags ...string) []FileCheck {
	opts := fileOpts()
	opts.Tags = tags
	return CheckChangelog(treeOf(map[string]string{"CHANGELOG.md": content}), opts)
}

// TestChangelogOfWatchoo prend le CHANGELOG de Watchoo comme cas conforme : CL1 à CL9 sans écart bloquant
// (l'ordre des catégories d'une version ancienne n'est qu'un avertissement).
func TestChangelogOfWatchoo(t *testing.T) {
	data, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, m := range regexp.MustCompile(`(?m)^## \[([0-9]+\.[0-9]+\.[0-9]+)\]`).FindAllStringSubmatch(string(data), -1) {
		tags = append(tags, "v"+m[1])
	}
	if len(tags) < 10 {
		t.Fatalf("versions lues = %q", tags)
	}
	opts := fileOpts()
	// Une version est datée du jour local de son auteur, jusqu'à un jour d'avance sur la CI (UTC) : le
	// jour « présent » est lu dans le fuseau le plus avancé (UTC+14), pour qu'une release publiée après
	// minuit à Paris ne soit pas « future » pendant la nuit UTC.
	opts.Now = time.Now().In(time.FixedZone("UTC+14", 14*3600))
	opts.Tags = append(tags, "v9.9.9-rc.1", "pas-une-version")
	opts.Version = tags[0]
	checks := CheckChangelog(treeOf(map[string]string{"CHANGELOG.md": string(data), "README.md": "x"}), opts)
	if len(checks) != 9 {
		t.Fatalf("critères = %d", len(checks))
	}
	for _, c := range checks {
		if c.Status != CriterionConform {
			t.Errorf("%s : %q %q (%q)", c.ID, c.Status, c.Codes, c.Reason)
		}
	}
	if byID(t, checks, CriterionCL4).Counts[CountVersions] != len(tags) {
		t.Errorf("CL4 : %+v", byID(t, checks, CriterionCL4))
	}
}

// TestChangelogOfWatchooCL8 vérifie que CL8 ne doit pas sa conformité aux tags fabriqués depuis le
// CHANGELOG lui-même : un tag de version sans section et une section datée sans tag sont des écarts.
func TestChangelogOfWatchooCL8(t *testing.T) {
	data, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, m := range regexp.MustCompile(`(?m)^## \[([0-9]+\.[0-9]+\.[0-9]+)\]`).FindAllStringSubmatch(string(data), -1) {
		tags = append(tags, "v"+m[1])
	}
	if len(tags) < 3 {
		t.Fatalf("versions lues = %q", tags)
	}
	opts := fileOpts()
	opts.Now = time.Now()
	// v0.0.1 n'a pas de section ; la section de tags[2] n'a plus de tag.
	opts.Tags = append(slices.Delete(slices.Clone(tags), 2, 3), "v0.0.1")
	tree := treeOf(map[string]string{"CHANGELOG.md": string(data)})
	c := byID(t, CheckChangelog(tree, opts), CriterionCL8)
	wantCheck(t, c, CriterionNonconform, FileTagWithoutSection, FileSectionWithoutTag)
	if c.Counts[FileTagWithoutSection] != 1 || c.Counts[FileSectionWithoutTag] != 1 {
		t.Errorf("compteurs de CL8 = %v", c.Counts)
	}
}

// TestChangelogFutureDateTimeZone vérifie que le jour de CL4 est pris dans le fuseau de Now : à 23 h 30 UTC
// le 9 novembre, il est déjà le 10 à Paris.
func TestChangelogFutureDateTimeZone(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("fuseau Europe/Paris indisponible :", err)
	}
	content := "# Changelog\n\n## [Unreleased]\n\n## [1.2.0] - 2026-11-10\n\n### Added\n\n- a\n"
	tree := treeOf(map[string]string{"CHANGELOG.md": content})
	at := time.Date(2026, 11, 9, 23, 30, 0, 0, time.UTC)
	opts := fileOpts()
	opts.Now = at.In(paris)
	wantCheck(t, byID(t, CheckChangelog(tree, opts), CriterionCL4), CriterionConform)
	opts.Now = at
	wantCheck(t, byID(t, CheckChangelog(tree, opts), CriterionCL4), CriterionNonconform, FileFutureDate)
}

func TestChangelogConform(t *testing.T) {
	checks := changelogChecks(goodChangelog, "v1.0.0", "v1.1.0", "v1.1.0-rc.1")
	if len(checks) != 8 {
		t.Fatalf("CL9 ne doit pas être mesuré sur la tête : %d critères", len(checks))
	}
	for _, c := range checks {
		wantCheck(t, c, CriterionConform)
	}
	if got := byID(t, checks, CriterionCL6).Counts[CountSections]; got != 3 {
		t.Errorf("sections de CL6 = %d", got)
	}
}

func TestChangelogLocation(t *testing.T) {
	cases := []struct {
		files map[string]string
		code  string
		next  string // état de CL2
	}{
		{map[string]string{"CHANGELOG.md": goodChangelog}, "", CriterionConform},
		{map[string]string{"changelog.md": goodChangelog}, FileMisnamed, CriterionConform},
		{map[string]string{"CHANGELOG": goodChangelog}, FileMisnamed, CriterionConform},
		{map[string]string{"Change-Log.md": goodChangelog}, FileMisnamed, CriterionConform},
		{map[string]string{"change_log": goodChangelog}, FileMisnamed, CriterionConform},
		{map[string]string{"docs/CHANGELOG.md": goodChangelog, "a/b/changelog.md": "x"}, FileElsewhere, CriterionConform},
		{map[string]string{"app/changelog.md": goodChangelog}, FileElsewhere, CriterionConform},
		{map[string]string{"node_modules/x/CHANGELOG.md": goodChangelog, "README.md": "x"}, FileAbsent, CriterionUnknown},
		{map[string]string{"CHANGES.md": goodChangelog, "HISTORY.md": "x"}, FileAbsent, CriterionUnknown},
	}
	for _, tc := range cases {
		checks := CheckChangelog(treeOf(tc.files), fileOpts())
		cl1 := byID(t, checks, CriterionCL1)
		if tc.code == "" {
			wantCheck(t, cl1, CriterionConform)
		} else {
			wantCheck(t, cl1, CriterionNonconform, tc.code)
		}
		cl2 := byID(t, checks, CriterionCL2)
		if cl2.Status != tc.next || (tc.next == CriterionUnknown && cl2.Reason != UnknownNoFile) {
			t.Errorf("%v : CL2 = %+v", tc.files, cl2)
		}
	}
}

func TestChangelogBlobNotRead(t *testing.T) {
	tree := FileTree{Paths: []string{"CHANGELOG.md", "README.md"}, Blobs: map[string]Blob{"CHANGELOG.md": {TooLarge: true}}}
	checks := CheckChangelog(tree, fileOpts())
	wantCheck(t, checks[0], CriterionConform)
	for _, c := range checks[1:] {
		if c.Status != CriterionUnknown || c.Reason != UnknownBlobTooLarge {
			t.Errorf("%s = %+v", c.ID, c)
		}
	}
	tree.Blobs = nil
	if c := CheckChangelog(tree, fileOpts())[3]; c.Reason != UnknownBlobUnreadable {
		t.Errorf("%s = %+v", c.ID, c)
	}
}

func TestChangelogHeader(t *testing.T) {
	body := "\n## [Unreleased]\n"
	decl := "Based on Keep a Changelog; this project follows SemVer.\n"
	cases := []struct {
		head  string
		fr    bool
		codes []string
		state string
	}{
		{"# Changelog\n" + decl, true, nil, CriterionConform},
		{"# CHANGELOG\n" + decl, true, nil, CriterionConform},
		{"# Change log\n" + decl, true, nil, CriterionConform},
		{"# Journal des modifications\n" + decl, true, []string{FileFrenchLabel}, CriterionConform},
		{"# Journal des modifications\n" + decl, false, []string{FileH1Title}, CriterionNonconform},
		{"# Historique\n" + decl, true, []string{FileH1Title}, CriterionNonconform},
		{decl, true, []string{FileNoH1}, CriterionNonconform},
		{"# Changelog\nVoir https://keepachangelog.com/fr/1.1.0/ et https://semver.org/lang/fr/\n", true, nil, CriterionConform},
		{"# Changelog\nCe projet suit le versionnage sémantique.\n", true, []string{FileNoKeepAChangelog}, CriterionNonconform},
		{"# Changelog\nFormat [Keep a Changelog](x), gestion sémantique de version.\n", true, nil, CriterionConform},
		{"# Changelog\n", true, []string{FileNoKeepAChangelog, FileNoSemVer}, CriterionNonconform},
		// Les déclarations après la première section ne comptent pas, ni celles d'un bloc de code.
		{"# Changelog\n```\nKeep a Changelog, SemVer\n```\n", true, []string{FileNoKeepAChangelog, FileNoSemVer}, CriterionNonconform},
	}
	for _, tc := range cases {
		opts := fileOpts()
		opts.Rules.CategoriesFR = tc.fr
		checks := CheckChangelog(treeOf(map[string]string{"CHANGELOG.md": tc.head + body + decl}), opts)
		wantCheck(t, byID(t, checks, CriterionCL2), tc.state, tc.codes...)
	}
}

func TestChangelogUnreleased(t *testing.T) {
	head := "# Changelog\n\n"
	v1 := "## [1.0.0] - 2026-01-01\n\n### Added\n\n- a\n\n"
	cases := []struct {
		body  string
		fr    bool
		codes []string
		state string
	}{
		{"## [Unreleased]\n\n" + v1, true, nil, CriterionConform},
		{"## [unreleased]\n\n" + v1, true, nil, CriterionConform},
		{"## Unreleased\n\n" + v1, true, []string{FileUnreleasedNoBrackets}, CriterionNonconform},
		{v1, true, []string{FileUnreleasedAbsent}, CriterionNonconform},
		{"", true, []string{FileUnreleasedAbsent}, CriterionNonconform},
		{v1 + "## [Unreleased]\n", true, []string{FileUnreleasedMisplaced}, CriterionNonconform},
		{"## [Unreleased]\n" + v1 + "## [Unreleased]\n", true, []string{FileUnreleasedDuplicate}, CriterionNonconform},
		{"## [Non publié]\n\n" + v1, true, []string{FileFrenchLabel}, CriterionConform},
		{"## Non publié\n\n" + v1, true, []string{FileUnreleasedNoBrackets, FileFrenchLabel}, CriterionNonconform},
		{"## [Non publié]\n\n" + v1, false, []string{FileUnreleasedAbsent}, CriterionNonconform},
		{"```\n## [Unreleased]\n```\n" + v1, true, []string{FileUnreleasedAbsent}, CriterionNonconform},
	}
	for _, tc := range cases {
		opts := fileOpts()
		opts.Rules.CategoriesFR = tc.fr
		checks := CheckChangelog(treeOf(map[string]string{"CHANGELOG.md": head + tc.body}), opts)
		wantCheck(t, byID(t, checks, CriterionCL3), tc.state, tc.codes...)
	}
}

func TestChangelogVersionHeadings(t *testing.T) {
	cases := []struct {
		heading string
		codes   []string
		state   string
	}{
		{"[1.2.0] - 2026-01-01", nil, CriterionConform},
		{"[1.2.0-rc.1] - 2026-01-01", nil, CriterionConform},
		{"[1.2.0] - 2026-01-01 [YANKED]", nil, CriterionConform},
		{"[1.2.0](https://github.com/promo/projet/releases/tag/v1.2.0) - 2026-01-01", nil, CriterionConform},
		{"[1.2.0] – 2026-01-01", nil, CriterionConform},
		{"[1.2.0] - 2026-11-09", nil, CriterionConform}, // le jour même
		{"[v1.2.0] - 2026-01-01", []string{FileVPrefix}, CriterionConform},
		{"1.2.0 - 2026-01-01", []string{FileNoBrackets}, CriterionNonconform},
		{"v1.2.0 - 2026-01-01", []string{FileNoBrackets, FileVPrefix}, CriterionNonconform},
		{"[1.2.0] - 01/02/2026", []string{FileDateFR}, CriterionNonconform},
		{"[1.2.0] - 1.2.2026", []string{FileDateFR}, CriterionNonconform},
		{"[1.2.0]", []string{FileNoDate}, CriterionNonconform},
		{"[1.2.0] [YANKED]", []string{FileNoDate}, CriterionNonconform},
		{"[1.2.0] - 2026-02-30", []string{FileInvalidDate}, CriterionNonconform},
		{"[1.2.0] - 2026/02/03", []string{FileInvalidDate}, CriterionNonconform},
		{"[1.2.0] - janvier 2026", []string{FileInvalidDate}, CriterionNonconform},
		{"[1.2.0] - 2026-11-10", []string{FileFutureDate}, CriterionNonconform},
		{"[1.2] - 2026-01-01", []string{FileInvalidVersion}, CriterionNonconform},
		{"Notes", []string{FileNoBrackets, FileNoDate, FileInvalidVersion}, CriterionNonconform},
	}
	for _, tc := range cases {
		content := "# Changelog\n\n## [Unreleased]\n\n## " + tc.heading + "\n\n### Added\n\n- a\n"
		c := byID(t, changelogChecks(content), CriterionCL4)
		wantCheck(t, c, tc.state, tc.codes...)
		if c.Counts[CountVersions] != 1 {
			t.Errorf("%q : versions = %d", tc.heading, c.Counts[CountVersions])
		}
	}
	// Les compteurs donnent le nombre de versions par code.
	content := "## [Unreleased]\n## 1.0.2 - 2026-01-03\n## 1.0.1 - 01/01/2026\n## [1.0.0] - 2026-01-01\n"
	c := byID(t, changelogChecks(content), CriterionCL4)
	if c.Counts[FileNoBrackets] != 2 || c.Counts[FileDateFR] != 1 || c.Counts[CountVersions] != 3 {
		t.Errorf("compteurs = %v", c.Counts)
	}
}

func TestChangelogOrder(t *testing.T) {
	cases := []struct {
		versions string
		codes    []string
	}{
		{"## [2.0.0] - 2026-03-01\n## [2.0.0-rc.1] - 2026-02-01\n## [1.0.0] - 2026-01-01\n", nil},
		{"## [1.0.0] - 2026-01-01\n## [1.1.0] - 2026-01-01\n", []string{FileVersionOrder}},
		{"## [1.0.0] - 2026-01-01\n## [1.0.0] - 2025-01-01\n", []string{FileVersionOrder}},
		{"## [1.1.0] - 2026-01-01\n## [1.0.0] - 2026-02-01\n", []string{FileDateOrder}},
		{"## [1.1.0] - 2026-01-01\n## [1.2] - 2025-06-01\n## [1.0.0] - 2025-01-01\n", nil},
		{"## [1.1.0] - 2026-01-01\n## [1.0.0]\n## [0.9.0] - 2026-02-01\n", []string{FileDateOrder}},
	}
	for _, tc := range cases {
		c := byID(t, changelogChecks("## [Unreleased]\n"+tc.versions), CriterionCL5)
		if tc.codes == nil {
			wantCheck(t, c, CriterionConform)
		} else {
			wantCheck(t, c, CriterionNonconform, tc.codes...)
		}
	}
}

func TestChangelogCategories(t *testing.T) {
	cases := []struct {
		body  string
		fr    bool
		codes []string
		state string
	}{
		{"### Added\n- a\n### Changed\n- b\n### Deprecated\n- c\n### Removed\n- d\n### Fixed\n- e\n### Security\n- f\n",
			true, nil, CriterionConform},
		{"### added\n- a\n### FIXED\n- b\n", true, nil, CriterionConform},
		{"### Fixed\n- a\n### Added\n- b\n", true, []string{FileCategoryOrder}, CriterionConform},
		{"### Ajouté\n- a\n### Corrigé\n- b\n### Sécurité\n- c\n", true, []string{FileFrenchLabel}, CriterionConform},
		{"### Ajouté\n- a\n", false, []string{FileUnknownCategory}, CriterionNonconform},
		{"### Features\n- a\n", true, []string{FileUnknownCategory}, CriterionNonconform},
		{"### Added\n- a\n### Added\n- b\n", true, []string{FileDuplicateCategory}, CriterionNonconform},
		{"### Added\n- a\n### Ajouté\n- b\n", true, []string{FileDuplicateCategory, FileFrenchLabel}, CriterionNonconform},
		{"- hors catégorie\n### Added\n- a\n", true, []string{FileBulletOutsideCategory}, CriterionNonconform},
		{"Un paragraphe d'introduction.\n\n### Added\n- a\n#### Détail\n- b\n", true, nil, CriterionConform},
		{"```\n- dans le code\n### Nope\n```\n### Added\n- a\n", true, nil, CriterionConform},
	}
	for _, tc := range cases {
		opts := fileOpts()
		opts.Rules.CategoriesFR = tc.fr
		content := "# Changelog\n\n## [Unreleased]\n\n## [1.0.0] - 2026-01-01\n\n" + tc.body
		checks := CheckChangelog(treeOf(map[string]string{"CHANGELOG.md": content}), opts)
		wantCheck(t, byID(t, checks, CriterionCL6), tc.state, tc.codes...)
	}
	// [Unreleased] est évalué aussi ; les compteurs donnent le nombre de sections par code.
	content := "## [Unreleased]\n- en vrac\n## [1.0.0] - 2026-01-01\n- en vrac\n### Added\n- a\n"
	c := byID(t, changelogChecks(content), CriterionCL6)
	if c.Counts[FileBulletOutsideCategory] != 2 || c.Counts[CountSections] != 2 {
		t.Errorf("compteurs = %v", c.Counts)
	}
}

func TestChangelogEmptyEntries(t *testing.T) {
	cases := []struct {
		body  string
		codes []string
	}{
		{"## [1.0.0] - 2026-01-01\n### Added\n- a\n", nil},
		{"## [1.0.0] - 2026-01-01\n", []string{FileEmptyVersion}},
		{"## [1.0.0] - 2026-01-01\nTexte sans puce.\n", []string{FileEmptyVersion}},
		{"## [1.0.0] - 2026-01-01\n### Added\n### Fixed\n- a\n", []string{FileEmptyCategory}},
		{"## [1.0.0] - 2026-01-01\n### Added\n-\n- TODO\n- todo.\n- a\n", []string{FileEmptyBullet}},
		{"## [1.0.0] - 2026-01-01\n### Added\n- TODO: écrire\n", nil},
	}
	for _, tc := range cases {
		// [Unreleased] vide, catégorie vide et « TODO » compris, n'est pas un écart.
		content := "## [Unreleased]\n### Added\n- TODO\n### Fixed\n" + tc.body
		c := byID(t, changelogChecks(content), CriterionCL7)
		if tc.codes == nil {
			wantCheck(t, c, CriterionConform)
		} else {
			wantCheck(t, c, CriterionNonconform, tc.codes...)
		}
	}
	c := byID(t, changelogChecks("## [1.0.0] - 2026-01-01\n### Added\n-\n- TODO\n"), CriterionCL7)
	if c.Counts[FileEmptyBullet] != 2 {
		t.Errorf("compteurs = %v", c.Counts)
	}
}

func TestChangelogTags(t *testing.T) {
	sections := "## [Unreleased]\n## [1.1.0] - 2026-02-01\n- a\n## [1.0.1] - 2026-01-15 [YANKED]\n- a\n" +
		"## [1.0.0]\n- a\n## [0.9.0] - 2025-12-01\n- a\n"
	cases := []struct {
		tags   []string
		codes  []string
		counts map[string]int
	}{
		{[]string{"v1.1.0", "v1.0.0", "v0.9.0"}, nil, nil},
		// Un tag non conforme qui désigne la version compte ; une pré-version sans section, non.
		{[]string{"1.1.0", "V1.0", "v0.9.0", "v1.2.0-rc.1", "release"}, nil, nil},
		{[]string{"v1.1.0", "v1.0.0", "v0.9.0", "v2.0.0", "v2.0.0+build"}, []string{FileTagWithoutSection},
			map[string]int{FileTagWithoutSection: 1}},
		// Section datée sans tag (0.9.0) ; section non datée (1.0.0) ou retirée (1.0.1) exemptée.
		{[]string{"v1.1.0"}, []string{FileSectionWithoutTag}, map[string]int{FileSectionWithoutTag: 1}},
		{nil, []string{FileSectionWithoutTag}, map[string]int{FileSectionWithoutTag: 2}},
	}
	for _, tc := range cases {
		c := byID(t, changelogChecks(sections, tc.tags...), CriterionCL8)
		if tc.codes == nil {
			wantCheck(t, c, CriterionConform)
		} else {
			wantCheck(t, c, CriterionNonconform, tc.codes...)
		}
		for k, v := range tc.counts {
			if c.Counts[k] != v {
				t.Errorf("%q : %s = %d, attendu %d", tc.tags, k, c.Counts[k], v)
			}
		}
	}
	if c := byID(t, changelogChecks("## [Unreleased]\n"), CriterionCL8); c.Status != CriterionConform {
		t.Errorf("sans tag ni version : %+v", c)
	}
}

func TestChangelogMilestone(t *testing.T) {
	atTag := "# Changelog\n\n## [Unreleased]\n\n## [1.0.0] - 2026-01-15\n\n### Added\n\n- a\n\n" +
		"## [0.9.0] - 2025-12-01\n\n### Added\n\n- b\n"
	run := func(content, version string, tags ...string) []FileCheck {
		opts := fileOpts()
		opts.Version, opts.Tags = version, tags
		return CheckChangelog(treeOf(map[string]string{"CHANGELOG.md": content}), opts)
	}
	// Au tag v1.0.0, les tags postérieurs ne réclament pas de section (CL8).
	checks := run(atTag, "v1.0.0", "v0.9.0", "v1.0.0", "v2.0.0", "v1.1.0")
	wantCheck(t, byID(t, checks, CriterionCL9), CriterionConform)
	wantCheck(t, byID(t, checks, CriterionCL8), CriterionConform)
	// Section absente au tag : CL9 non conforme ; un titre mal formé désigne quand même la version.
	wantCheck(t, byID(t, run(strings.Replace(atTag, "[1.0.0]", "[0.9.5]", 1), "v1.0.0"), CriterionCL9),
		CriterionNonconform, FileVersionMissing)
	wantCheck(t, byID(t, run(strings.Replace(atTag, "[1.0.0] - 2026-01-15", "v1.0.0", 1), "v1.0.0"), CriterionCL9),
		CriterionConform)
	// Sections postérieures au jalon sans tag : ignorées par CL8 au tag du jalon.
	later := strings.Replace(atTag, "## [1.0.0]", "## [1.1.0] - 2026-02-01\n- c\n## [1.0.0]", 1)
	wantCheck(t, byID(t, run(later, "v1.0.0", "v0.9.0", "v1.0.0"), CriterionCL8), CriterionConform)
	c := byID(t, run(atTag, "release-1"), CriterionCL9)
	if c.Status != CriterionUnknown || c.Reason != UnknownNoTag {
		t.Errorf("jalon illisible : %+v", c)
	}
	if c := byID(t, run(atTag, "v1.0.0"), CriterionCL1); c.Status != CriterionConform {
		t.Errorf("CL1 = %+v", c)
	}
	if got := len(run(atTag, "v1.0.0")); got != 9 {
		t.Errorf("critères au jalon = %d", got)
	}
	nf := CheckChangelog(treeOf(map[string]string{"README.md": "x"}), FileOptions{Version: "v1.0.0"})
	if c := byID(t, nf, CriterionCL9); c.Status != CriterionUnknown || c.Reason != UnknownNoFile {
		t.Errorf("sans CHANGELOG : %+v", c)
	}
}
