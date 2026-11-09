package conventions

import (
	"strings"
	"testing"
)

// fullReadme a toutes les sections de la charte, les deux liens et plus de 150 mots.
var fullReadme = "# Projet\n\n## Présentation\n\n" + strings.Repeat("Un texte de présentation du projet. ", 30) +
	"\n\n## Groupe\n\n- Équipe A\n\n## Stack et prérequis\n\nGo.\n\n## Installation\n\n```sh\nmake build\n```\n\n" +
	"## Lancement\n\nLancer.\n\n## Variables d'environnement\n\n`PORT`.\n\n## Arborescence\n\nsrc.\n\n" +
	"## Licence\n\nPrivé.\n\nVoir [la documentation](/documentation/INDEX.md) et [le journal](CHANGELOG.md).\n"

func readmeChecks(t *testing.T, files map[string]string, mutate func(*FileOptions)) []FileCheck {
	t.Helper()
	opts := fileOpts()
	if mutate != nil {
		mutate(&opts)
	}
	return CheckReadme(treeOf(files), opts)
}

func TestReadmeConform(t *testing.T) {
	checks := readmeChecks(t, map[string]string{"README.md": fullReadme, "CHANGELOG.md": "x",
		"documentation/INDEX.md": "x"}, nil)
	for _, c := range checks {
		wantCheck(t, c, CriterionConform)
	}
	rd2 := byID(t, checks, CriterionRD2)
	if rd2.Counts[CountFound] != 8 || rd2.Counts[CountFoundMask] != 255 || rd2.Counts[CountExpectedMask] != 255 {
		t.Errorf("RD2 = %v", rd2.Counts)
	}
	for _, key := range AllReadmeSections() {
		if !ReadmeSectionFound(rd2, key) {
			t.Errorf("section %q non trouvée", key)
		}
	}
	if ReadmeSectionFound(byID(t, checks, CriterionRD1), SectionInstall) || ReadmeSectionFound(rd2, "x") {
		t.Error("ReadmeSectionFound ne lit que RD2 et les clés connues")
	}
}

func TestReadmeLocation(t *testing.T) {
	cases := []struct {
		files        map[string]string
		codes        []string
		translations int
		next         string
	}{
		{map[string]string{"README.md": "x", "README.fr.md": "x", "README.en-US.md": "x"}, nil, 2, CriterionNonconform},
		{map[string]string{"readme.md": "x"}, []string{FileMisnamed}, 0, CriterionNonconform},
		{map[string]string{"README": "x"}, []string{FileMisnamed}, 0, CriterionNonconform},
		{map[string]string{"Readme.txt": "x"}, []string{FileMisnamed}, 0, CriterionNonconform},
		{map[string]string{"app/README.md": "x", "README.fr.md": "x"}, []string{FileElsewhere}, 1, CriterionNonconform},
		{map[string]string{"README.fr.md": "x"}, []string{FileAbsent, FileTranslationOnly}, 1, CriterionUnknown},
		{map[string]string{"src/main.go": "x"}, []string{FileAbsent}, 0, CriterionUnknown},
	}
	for _, tc := range cases {
		checks := readmeChecks(t, tc.files, nil)
		rd1 := byID(t, checks, CriterionRD1)
		state := CriterionConform
		if tc.codes != nil {
			state = CriterionNonconform
		}
		wantCheck(t, rd1, state, tc.codes...)
		if rd1.Counts[CountTranslations] != tc.translations {
			t.Errorf("%v : traductions = %d", tc.files, rd1.Counts[CountTranslations])
		}
		// RD2 du fichier « x » : non conforme (aucune section) ; sans fichier : inconnu.
		if rd2 := byID(t, checks, CriterionRD2); rd2.Status != tc.next {
			t.Errorf("%v : RD2 = %+v", tc.files, rd2)
		}
	}
}

func TestReadmeSections(t *testing.T) {
	cases := []struct {
		headings string
		want     []string
	}{
		{"## About\n## Team\n## Built With\n## Getting Started\n## Usage\n## Environment Variables\n" +
			"## Project Structure\n## License\n", AllReadmeSections()},
		{"## 🚀 Installation et lancement\n", []string{SectionInstall, SectionRun}},
		{"## 1. Prérequis\n### 2) Mise en place\n", []string{SectionStack, SectionInstall}},
		{"## Configuration (.env)\n", []string{SectionEnv}},
		{"## Confidentialité\n", []string{SectionLicense}},
		{"## Les membres du groupe\n", []string{SectionGroup}},
		{"## Installations diverses\n## Licences\n", nil},
		{"```\n## Installation\n```\nInstallation\n", nil},
	}
	for _, tc := range cases {
		found := readmeSectionsFound(parseMarkdown([]byte(tc.headings)))
		for _, key := range AllReadmeSections() {
			want := false
			for _, w := range tc.want {
				want = want || w == key
			}
			if found[key] != want {
				t.Errorf("%q : section %q trouvée = %v, attendu %v", tc.headings, key, found[key], want)
			}
		}
	}
}

func TestReadmeExpectedSections(t *testing.T) {
	content := "# P\n## Installation\n## Lancement\n"
	checks := readmeChecks(t, map[string]string{"README.md": content}, func(o *FileOptions) {
		o.Rules.ReadmeSections = []string{SectionRun, SectionInstall, "inconnue"}
	})
	rd2 := byID(t, checks, CriterionRD2)
	wantCheck(t, rd2, CriterionConform)
	if rd2.Counts[CountExpected] != 2 || rd2.Counts[CountExpectedMask] != 1<<3|1<<4 {
		t.Errorf("RD2 = %v", rd2.Counts)
	}
	// Toutes les sections : six manquent ; un fichier LICENSE vaut la section licence.
	checks = readmeChecks(t, map[string]string{"README.md": content, "LICENSE": "MIT"}, nil)
	rd2 = byID(t, checks, CriterionRD2)
	wantCheck(t, rd2, CriterionNonconform, FileMissingSection)
	if rd2.Counts[FileMissingSection] != 5 || rd2.Counts[CountLicenseFile] != 1 || rd2.Counts[CountFound] != 3 {
		t.Errorf("RD2 = %v", rd2.Counts)
	}
	if !ReadmeSectionFound(rd2, SectionLicense) || ReadmeSectionFound(rd2, SectionEnv) {
		t.Error("masque inattendu")
	}
	// Aucune section attendue : conforme.
	checks = readmeChecks(t, map[string]string{"README.md": "x"}, func(o *FileOptions) { o.Rules.ReadmeSections = nil })
	wantCheck(t, byID(t, checks, CriterionRD2), CriterionConform)
}

func TestReadmeLinks(t *testing.T) {
	tree := map[string]string{"CHANGELOG.md": "x", "documentation/INDEX.md": "x", "documentation/01-a/b.md": "x"}
	cases := []struct {
		links string
		codes []string
	}{
		{"[i](documentation/INDEX.md) [c](CHANGELOG.md)", nil},
		{"[i](./documentation/INDEX.md#top) [c](/CHANGELOG.md)", nil},
		{"[i]: /documentation/INDEX.md\n[c]: <CHANGELOG.md>", nil},
		{"[i](https://github.com/promo/projet/blob/main/documentation/INDEX.md) " +
			"[c](https://github.com/Promo/projet/blob/v1.0.0/CHANGELOG.md)", nil},
		{"[i](documentation/) [c](CHANGELOG.md)", []string{FileIndexLinkFolder}},
		{"[i](documentation) [i](documentation/INDEX.md) [c](CHANGELOG.md)", nil},
		{"[i](documentation/index.md) [c](changelog.md)", []string{FileIndexLinkDead, FileChangelogLinkDead}},
		{"[i](docs/INDEX.md) [c](docs/CHANGELOG.md)", []string{FileIndexLinkDead, FileChangelogLinkDead}},
		{"[c](CHANGELOG.md)", []string{FileIndexLinkAbsent}},
		{"[i](https://github.com/autre/projet/blob/main/documentation/INDEX.md) [c](https://exemple.fr/CHANGELOG.md)",
			[]string{FileIndexLinkAbsent, FileChangelogLinkAbsent}},
		{"![i](documentation/INDEX.md) `[c](CHANGELOG.md)`", []string{FileIndexLinkAbsent, FileChangelogLinkAbsent}},
		{"[autre](documentation/01-a/b.md) [site](site/index.md)", []string{FileIndexLinkAbsent, FileChangelogLinkAbsent}},
	}
	for _, tc := range cases {
		files := map[string]string{"README.md": "# P\n\n" + tc.links + "\n"}
		for k, v := range tree {
			files[k] = v
		}
		rd3 := byID(t, readmeChecks(t, files, nil), CriterionRD3)
		if tc.codes == nil {
			wantCheck(t, rd3, CriterionConform)
		} else {
			wantCheck(t, rd3, CriterionNonconform, tc.codes...)
		}
	}
	// Dossier de documentation absent : le lien vers le dossier est mort.
	rd3 := byID(t, readmeChecks(t, map[string]string{"README.md": "[i](documentation/) [c](CHANGELOG.md)",
		"CHANGELOG.md": "x"}, nil), CriterionRD3)
	wantCheck(t, rd3, CriterionNonconform, FileIndexLinkDead)
	// Dossier de documentation configuré.
	rd3 = byID(t, readmeChecks(t, map[string]string{"README.md": "[i](docs/INDEX.md) [c](CHANGELOG.md)",
		"CHANGELOG.md": "x", "docs/INDEX.md": "x"}, func(o *FileOptions) { o.Rules.DocsDir = "docs" }), CriterionRD3)
	wantCheck(t, rd3, CriterionConform)
}

func TestReadmeWords(t *testing.T) {
	short := "# Projet\n\nTrop court.\n\n```\n" + strings.Repeat("code ", 500) + "\n```\n"
	rd4 := byID(t, readmeChecks(t, map[string]string{"README.md": short}, nil), CriterionRD4)
	wantCheck(t, rd4, CriterionNonconform, FileTooShort)
	if rd4.Counts[CountWords] != 3 || rd4.Counts[CountMinWords] != 150 {
		t.Errorf("RD4 = %v", rd4.Counts)
	}
	rd4 = byID(t, readmeChecks(t, map[string]string{"README.md": short}, func(o *FileOptions) {
		o.Rules.ReadmeMinWords = 3
	}), CriterionRD4)
	wantCheck(t, rd4, CriterionConform)
}

func TestReadmeBlobNotRead(t *testing.T) {
	tree := FileTree{Paths: []string{"README.md"}, Blobs: map[string]Blob{"README.md": {TooLarge: true}}}
	checks := CheckReadme(tree, fileOpts())
	wantCheck(t, checks[0], CriterionConform)
	for _, c := range checks[1:] {
		if c.Status != CriterionUnknown || c.Reason != UnknownBlobTooLarge {
			t.Errorf("%s = %+v", c.ID, c)
		}
	}
}
