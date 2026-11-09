package conventions

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

const goodIndex = "# Index\n\n| Document | Thème | Version | Dernière mise à jour | Responsable |\n" +
	"|---|---|---|---|---|\n" +
	"| [Vision](01-cadrage/vision.md) | Cadrage | 1.0 | 2026-10-01 | Alice Martin |\n" +
	"| [Architecture](02-conception/architecture.md) | Conception | 1.1 | 2026-10-02 | Bob Durand |\n"

// goodDocs est une documentation conforme à la charte.
func goodDocs() map[string]string {
	return map[string]string{
		"README.md":                                   "# P\n\n[doc](documentation/INDEX.md)\n",
		"documentation/INDEX.md":                      goodIndex,
		"documentation/01-cadrage/vision.md":          "# Vision\n\nVoir [l'architecture](../02-conception/architecture.md#a).\n",
		"documentation/02-conception/architecture.md": "# Archi\n\n![schéma](../Media/schema-global.png) [code](/src/main.go)\n",
		"documentation/Media/schema-global.png":       "",
		"documentation/Media/notes.md":                "notes",
		"documentation/.gitkeep":                      "",
		"src/main.go":                                 "package main",
	}
}

func docsChecks(t *testing.T, files map[string]string, mutate func(*FileOptions)) ([]FileCheck, FileLists) {
	t.Helper()
	opts := fileOpts()
	if mutate != nil {
		mutate(&opts)
	}
	return CheckDocs(treeOf(files), opts)
}

func TestDocsConform(t *testing.T) {
	checks, lists := docsChecks(t, goodDocs(), nil)
	ids := make([]string, 0, len(checks))
	for _, c := range checks {
		ids = append(ids, c.ID)
		wantCheck(t, c, CriterionConform)
	}
	if !reflect.DeepEqual(ids, DocsCriteria) {
		t.Errorf("ordre = %q", ids)
	}
	if !reflect.DeepEqual(lists, FileLists{}) {
		t.Errorf("listes = %+v", lists)
	}
	doc2 := byID(t, checks, CriterionDOC2)
	if doc2.Counts[CountRows] != 2 || doc2.Counts[CountColumns] != 5 || doc2.Counts[CountTables] != 1 {
		t.Errorf("DOC2 = %v", doc2.Counts)
	}
	if got := byID(t, checks, CriterionDOC3).Counts[CountDocuments]; got != 2 {
		t.Errorf("documents = %d (INDEX et Media/ exclus)", got)
	}
	doc4 := byID(t, checks, CriterionDOC4)
	if doc4.Counts[CountLinks] != 5 || doc4.Counts[CountSources] != 4 {
		t.Errorf("DOC4 = %v", doc4.Counts)
	}
	split, _ := CheckIndex(treeOf(goodDocs()), fileOpts())
	tree, _ := CheckDocsTree(treeOf(goodDocs()), fileOpts())
	if !reflect.DeepEqual(append(split, tree...), checks) {
		t.Error("CheckDocs = CheckIndex + CheckDocsTree")
	}
}

func TestDocsIndexLocation(t *testing.T) {
	cases := []struct {
		path string
		code string
	}{
		{"documentation/INDEX.md", ""},
		{"documentation/index.md", FileMisnamed},
		{"Documentation/INDEX.md", FileMisnamed},
		{"docs/INDEX.md", FileElsewhere},
		{"doc/index.md", FileElsewhere},
		{"INDEX.md", FileElsewhere},
		{"site/index.md", FileAbsent},
		{"index.md", FileAbsent},
	}
	for _, tc := range cases {
		checks, _ := docsChecks(t, map[string]string{tc.path: goodIndex, "src/a.go": ""}, nil)
		doc1 := byID(t, checks, CriterionDOC1)
		if tc.code == "" {
			wantCheck(t, doc1, CriterionConform)
		} else {
			wantCheck(t, doc1, CriterionNonconform, tc.code)
		}
		doc2 := byID(t, checks, CriterionDOC2)
		if tc.code == FileAbsent {
			if doc2.Status != CriterionUnknown || doc2.Reason != UnknownNoFile {
				t.Errorf("%s : DOC2 = %+v", tc.path, doc2)
			}
		} else if doc2.Status != CriterionConform {
			t.Errorf("%s : DOC2 = %+v", tc.path, doc2)
		}
	}
	// Dossier configuré.
	checks, _ := docsChecks(t, map[string]string{"docs/INDEX.md": goodIndex}, func(o *FileOptions) { o.Rules.DocsDir = "docs" })
	wantCheck(t, byID(t, checks, CriterionDOC1), CriterionConform)
}

func TestDocsIndexTable(t *testing.T) {
	cases := []struct {
		index string
		codes []string
		rows  int
	}{
		{goodIndex, nil, 2},
		{"| File | Topic | Version | Last updated | Owner |\n|--|--|--|--|--|\n| a | b | c | d | e |\n", nil, 1},
		{"| Fichier | Sujet | V | Date | Auteur |\n| :-- | :-: | --: | --- | --- |\n| a | b | c | d | e |\n", nil, 1},
		{"| Responsable | Document | Mise à jour | Version | Thème |\n|-|-|-|-|-|\n| a | b | c | d | e |\n", nil, 1},
		// Deux tableaux complets : les lignes s'additionnent.
		{goodIndex + "\n## Annexes\n\n" + goodIndex, nil, 4},
		{"| Document | Thème | Version | Date |\n|---|---|---|---|\n| a | b | c | d |\n", []string{FileMissingColumn}, 0},
		{"| Document | Thème | Version | Dernière mise à jour | Responsable |\n|---|---|---|---|---|\n", []string{FileNoRows}, 0},
		{"# Index\n\n- [a](a.md)\n", []string{FileNoTable}, 0},
		{"```\n" + goodIndex + "```\n", []string{FileNoTable}, 0},
	}
	for _, tc := range cases {
		checks, _ := docsChecks(t, map[string]string{"documentation/INDEX.md": tc.index}, nil)
		doc2 := byID(t, checks, CriterionDOC2)
		if tc.codes == nil {
			wantCheck(t, doc2, CriterionConform)
		} else {
			wantCheck(t, doc2, CriterionNonconform, tc.codes...)
		}
		if doc2.Counts[CountRows] != tc.rows {
			t.Errorf("%q : lignes = %d, attendu %d", tc.index, doc2.Counts[CountRows], tc.rows)
		}
	}
	checks, _ := docsChecks(t, map[string]string{"documentation/INDEX.md": cases[5].index}, nil)
	if c := byID(t, checks, CriterionDOC2); c.Counts[FileMissingColumn] != 1 || c.Counts[CountColumns] != 4 {
		t.Errorf("DOC2 = %v", c.Counts)
	}
}

func TestDocsIndexExhaustive(t *testing.T) {
	files := goodDocs()
	files["documentation/03-tests/plan.md"] = "plan"
	files["documentation/03-tests/Plan Secondaire.MD"] = "plan"
	files["documentation/INDEX.md"] = goodIndex +
		"| [Vision bis](./01-cadrage/vision.md) | Cadrage | 1.0 | 2026-10-01 | Alice Martin |\n" +
		"\nAussi : [le plan](03-tests/plan.md).\n"
	checks, lists := docsChecks(t, files, nil)
	doc3 := byID(t, checks, CriterionDOC3)
	wantCheck(t, doc3, CriterionNonconform, FileUnreferenced, FileDuplicateRow)
	if doc3.Counts[CountDocuments] != 4 || doc3.Counts[FileUnreferenced] != 1 || doc3.Counts[FileDuplicateRow] != 1 {
		t.Errorf("DOC3 = %v", doc3.Counts)
	}
	if !reflect.DeepEqual(lists.Unreferenced, []string{"documentation/03-tests/Plan Secondaire.MD"}) ||
		!reflect.DeepEqual(lists.DuplicateRows, []string{"documentation/01-cadrage/vision.md"}) {
		t.Errorf("listes = %+v", lists)
	}
	// Un lien mal cassé ne référence pas le document.
	files = goodDocs()
	files["documentation/INDEX.md"] = strings.Replace(goodIndex, "01-cadrage/vision.md", "01-Cadrage/vision.md", 1)
	checks, lists = docsChecks(t, files, nil)
	wantCheck(t, byID(t, checks, CriterionDOC3), CriterionNonconform, FileUnreferenced)
	if len(lists.DeadLinks) != 1 || !lists.DeadLinks[0].CaseMismatch {
		t.Errorf("DOC4 = %+v", lists.DeadLinks)
	}
	// INDEX à la racine : la documentation reste celle de docs.dir.
	files = goodDocs()
	delete(files, "documentation/INDEX.md")
	files["INDEX.md"] = strings.ReplaceAll(goodIndex, "](0", "](documentation/0")
	checks, _ = docsChecks(t, files, nil)
	wantCheck(t, byID(t, checks, CriterionDOC1), CriterionNonconform, FileElsewhere)
	wantCheck(t, byID(t, checks, CriterionDOC3), CriterionConform)
	// INDEX hors du dossier configuré (docs/) : les documents sont cherchés dans docs/.
	files = map[string]string{"docs/INDEX.md": goodIndex, "docs/01-cadrage/vision.md": "x", "docs/02-conception/architecture.md": "x",
		"docs/03-x/orphelin.md": "x"}
	checks, lists = docsChecks(t, files, nil)
	wantCheck(t, byID(t, checks, CriterionDOC3), CriterionNonconform, FileUnreferenced)
	if !reflect.DeepEqual(lists.Unreferenced, []string{"docs/03-x/orphelin.md"}) {
		t.Errorf("non référencés = %q", lists.Unreferenced)
	}
}

func TestDocsDeadLinks(t *testing.T) {
	files := goodDocs()
	files["README.md"] = "[doc](documentation/INDEX.md) [mort](src/absent.go) [casse](SRC/main.go) [ext](https://x.fr/y) " +
		"[ancre](#a) [dossier](src/) ![img](absente.png)\n[def]: documentation/manquant.md\n"
	files["documentation/01-cadrage/vision.md"] = "[haut](../../../hors.md)\n```\n[code](absent.md)\n```\n"
	checks, lists := docsChecks(t, files, nil)
	doc4 := byID(t, checks, CriterionDOC4)
	wantCheck(t, doc4, CriterionNonconform, FileDeadLink, FileCaseMismatch)
	if doc4.Counts[FileDeadLink] != 3 || doc4.Counts[FileCaseMismatch] != 1 {
		t.Errorf("DOC4 = %v", doc4.Counts)
	}
	want := []DeadLink{
		{Source: "README.md", Target: "src/absent.go", Resolved: "src/absent.go"},
		{Source: "README.md", Target: "SRC/main.go", Resolved: "SRC/main.go", CaseMismatch: true},
		{Source: "README.md", Target: "documentation/manquant.md", Resolved: "documentation/manquant.md"},
		{Source: "documentation/01-cadrage/vision.md", Target: "../../../hors.md", Resolved: "../hors.md"},
	}
	if !reflect.DeepEqual(lists.DeadLinks, want) {
		t.Errorf("liens morts = %+v", lists.DeadLinks)
	}
}

func TestDocsDeadLinksUnread(t *testing.T) {
	tree := treeOf(goodDocs())
	tree.Blobs["documentation/01-cadrage/vision.md"] = Blob{TooLarge: true}
	checks, _ := CheckDocs(tree, fileOpts())
	doc4 := byID(t, checks, CriterionDOC4)
	if doc4.Status != CriterionUnknown || doc4.Reason != UnknownUnreadDocuments || doc4.Counts[CountUnread] != 1 {
		t.Errorf("DOC4 = %+v", doc4)
	}
	// Un lien mort parmi les documents lus suffit à conclure.
	tree.Blobs["README.md"] = Blob{Data: []byte("[x](absent.md)")}
	checks, _ = CheckDocs(tree, fileOpts())
	wantCheck(t, byID(t, checks, CriterionDOC4), CriterionNonconform, FileDeadLink)
	// Au-delà du plafond, les documents ne sont pas lus.
	files := map[string]string{"documentation/INDEX.md": "x"}
	for i := range MaxDocuments + 2 {
		files["documentation/01-x/d"+strings.Repeat("a", i/26)+string(rune('a'+i%26))+".md"] = "x"
	}
	checks, _ = docsChecks(t, files, nil)
	if c := byID(t, checks, CriterionDOC4); c.Counts[CountUnread] != 2 || c.Status != CriterionUnknown {
		t.Errorf("DOC4 = %+v", c)
	}
}

func TestDocsNaming(t *testing.T) {
	files := goodDocs()
	files["documentation/3-tests/plan.md"] = ""
	files["documentation/04-Tests/plan.md"] = ""
	files["documentation/05-tests_unit/plan.md"] = ""
	files["documentation/06-tests/sous/plan.md"] = ""
	files["documentation/01-cadrage/Mon Fichier.md"] = ""
	files["documentation/01-cadrage/fiche_1.md"] = ""
	files["documentation/01-cadrage/présentation.md"] = ""
	files["documentation/01-cadrage/README.md"] = ""
	files["documentation/01-cadrage/sans-extension"] = ""
	files["documentation/media/Capture 1.PNG"] = ""
	files["documentation/Media/Sous Dossier/a.png"] = ""
	checks, lists := docsChecks(t, files, nil)
	doc5 := byID(t, checks, CriterionDOC5)
	wantCheck(t, doc5, CriterionNonconform, FileDirName, FileFileName)
	wantBad := []string{
		"documentation/01-cadrage/Mon Fichier.md",
		"documentation/01-cadrage/fiche_1.md",
		"documentation/01-cadrage/présentation.md",
		"documentation/01-cadrage/sans-extension",
		"documentation/04-Tests/",
		"documentation/05-tests_unit/",
		"documentation/06-tests/sous/",
		"documentation/3-tests/",
		"documentation/media/Capture 1.PNG",
	}
	if !reflect.DeepEqual(lists.BadNames, wantBad) {
		t.Errorf("noms = %q", lists.BadNames)
	}
	if doc5.Counts[FileDirName] != 4 || doc5.Counts[FileFileName] != 5 {
		t.Errorf("DOC5 = %v", doc5.Counts)
	}
	// Sans dossier de documentation : inconnu.
	checks, _ = docsChecks(t, map[string]string{"README.md": "x"}, nil)
	if c := byID(t, checks, CriterionDOC5); c.Status != CriterionUnknown || c.Reason != UnknownNoDocsDir {
		t.Errorf("DOC5 = %+v", c)
	}
	// Dossier des médias configuré.
	checks, _ = docsChecks(t, map[string]string{"documentation/Images/a.png": "", "documentation/INDEX.md": goodIndex},
		func(o *FileOptions) { o.Rules.MediaDir = "Images" })
	wantCheck(t, byID(t, checks, CriterionDOC5), CriterionConform)
}

func TestDocsArchives(t *testing.T) {
	files := map[string]string{
		"documentation/INDEX.md":         goodIndex,
		"livrables/v1.zip":               "",
		"src/data.TAR.GZ":                "",
		"documentation/01-x/a.7z":        "",
		"documentation/01-x/cahier.docx": "",
		"documentation/01-x/plan.PDF":    "",
		"rapport.pdf":                    "",
		"lib.jar":                        "",
	}
	checks, lists := docsChecks(t, files, nil)
	doc8 := byID(t, checks, CriterionDOC8)
	wantCheck(t, doc8, CriterionNonconform, FileArchive, FileOfficeDocument)
	if doc8.Counts[FileArchive] != 3 || doc8.Counts[FileOfficeDocument] != 2 {
		t.Errorf("DOC8 = %v", doc8.Counts)
	}
	if !reflect.DeepEqual(lists.Archives, []string{"documentation/01-x/a.7z", "livrables/v1.zip", "src/data.TAR.GZ"}) ||
		!reflect.DeepEqual(lists.OfficeDocuments, []string{"documentation/01-x/cahier.docx", "documentation/01-x/plan.PDF"}) {
		t.Errorf("listes = %+v", lists)
	}
	checks, lists = docsChecks(t, files, func(o *FileOptions) { o.Rules.ArchivesScope = ArchivesScopeDocumentation })
	if c := byID(t, checks, CriterionDOC8); c.Counts[FileArchive] != 1 || !slices.Equal(lists.Archives, []string{"documentation/01-x/a.7z"}) {
		t.Errorf("portée documentation : %+v %q", c, lists.Archives)
	}
	// Seuls des documents bureautiques : avertissement, conforme.
	checks, _ = docsChecks(t, map[string]string{"documentation/a.pdf": ""}, nil)
	wantCheck(t, byID(t, checks, CriterionDOC8), CriterionConform, FileOfficeDocument)
}
