package conventions

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// treeOf construit un arbre dont tous les fichiers sont lus.
func treeOf(files map[string]string) FileTree {
	tree := FileTree{Blobs: map[string]Blob{}}
	for p, content := range files {
		tree.Paths = append(tree.Paths, p)
		tree.Blobs[p] = Blob{Data: []byte(content)}
	}
	slices.Sort(tree.Paths)
	return tree
}

// byID renvoie le critère id d'une grille.
func byID(t *testing.T, checks []FileCheck, id string) FileCheck {
	t.Helper()
	i := slices.IndexFunc(checks, func(c FileCheck) bool { return c.ID == id })
	if i < 0 {
		t.Fatalf("critère %s absent de %+v", id, checks)
	}
	return checks[i]
}

// wantCheck vérifie l'état et les codes d'un critère.
func wantCheck(t *testing.T, c FileCheck, status string, codes ...string) {
	t.Helper()
	if codes == nil {
		codes = []string{}
	}
	if c.Status != status || !reflect.DeepEqual(c.Codes, codes) {
		t.Errorf("%s : état %q codes %q (raison %q), attendu %q %q", c.ID, c.Status, c.Codes, c.Reason, status, codes)
	}
}

// fileOpts renvoie des options de la charte, datées du 9 novembre 2026.
func fileOpts() FileOptions {
	return FileOptions{Rules: DefaultFileRules(), Repo: "promo/projet",
		Now: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
}

func TestFileSeverity(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range AllFileCodes {
		if seen[c] {
			t.Errorf("code %q en double", c)
		}
		seen[c] = true
		if FileSeverityOf(c) == 0 {
			t.Errorf("code %q sans gravité", c)
		}
	}
	for _, c := range []string{FileFrenchLabel, FileVPrefix, FileCategoryOrder, FileOfficeDocument,
		FileHuskyNoPrepare, FileTranslationOnly} {
		if FileSeverityOf(c) != SeverityWarning {
			t.Errorf("%q devrait être un avertissement", c)
		}
	}
	if FileSeverityOf(FileArchive) != SeverityBlocking || FileSeverityOf("inconnu") != 0 {
		t.Error("gravités inattendues")
	}
	// Les avertissements suivent les bloquants dans l'ordre d'affichage.
	first := slices.IndexFunc(AllFileCodes, func(c string) bool { return FileSeverityOf(c) == SeverityWarning })
	if slices.ContainsFunc(AllFileCodes[first:], func(c string) bool { return FileSeverityOf(c) == SeverityBlocking }) {
		t.Error("un code bloquant suit un avertissement")
	}
}

func TestFileRulesDefaultsAndValidation(t *testing.T) {
	r := DefaultFileRules()
	if !r.CategoriesFR || r.ReadmeMinWords != 150 || r.DocsDir != "documentation" || r.MediaDir != "Media" ||
		r.ArchivesScope != ArchivesScopeRepository || !reflect.DeepEqual(r.ReadmeSections, AllReadmeSections()) {
		t.Errorf("règles par défaut = %+v", r)
	}
	r.ReadmeSections[0] = "x"
	if AllReadmeSections()[0] != SectionPresentation {
		t.Error("AllReadmeSections doit renvoyer une copie")
	}
	for _, s := range []string{"documentation", "docs/projet", "Doc Projet", "été"} {
		if !ValidDocsDir(s) {
			t.Errorf("ValidDocsDir(%q) devrait être vrai", s)
		}
	}
	for _, s := range []string{"", "/docs", "docs/", "../docs", "docs/../x", "a//b", "./docs", "a\x00b", "a\nb",
		" docs", `a\b`, strings.Repeat("a", 256)} {
		if ValidDocsDir(s) {
			t.Errorf("ValidDocsDir(%q) devrait être faux", s)
		}
	}
	if !ValidMediaDir("Media") || ValidMediaDir("a/b") || ValidMediaDir("") || ValidMediaDir("..") {
		t.Error("ValidMediaDir inattendu")
	}
	if !ValidArchivesScope("repository") || !ValidArchivesScope("documentation") || ValidArchivesScope("") {
		t.Error("ValidArchivesScope inattendu")
	}
	if !ValidReadmeSection("install") || ValidReadmeSection("Install") {
		t.Error("ValidReadmeSection inattendu")
	}
}

func TestFileRulesFingerprint(t *testing.T) {
	base := DefaultFileRules()
	fp := base.Fingerprint("alice/projet", []string{"v1.0.0", "v2.0.0"})
	if len(fp) != 64 {
		t.Fatalf("empreinte = %q", fp)
	}
	same := base
	same.ReadmeSections = []string{SectionLicense, SectionPresentation, SectionGroup, SectionStack, SectionInstall,
		SectionRun, SectionEnv, SectionTree, SectionInstall, "inconnue"}
	if got := same.Fingerprint("alice/projet", []string{"v2.0.0", "release", "v1.0.0", "v1.0.0"}); got != fp {
		t.Error("l'ordre, les doublons, les clés inconnues et les tags sans version ne changent pas l'empreinte")
	}
	empty := FileRules{CategoriesFR: true, ReadmeSections: AllReadmeSections()}
	if empty.Fingerprint("alice/projet", []string{"v1.0.0", "v2.0.0"}) != fp {
		t.Error("des valeurs vides valent celles de la charte")
	}
	changes := []func(*FileRules){
		func(r *FileRules) { r.CategoriesFR = false },
		func(r *FileRules) { r.ReadmeSections = []string{SectionInstall} },
		func(r *FileRules) { r.ReadmeMinWords = 151 },
		func(r *FileRules) { r.DocsDir = "docs" },
		func(r *FileRules) { r.MediaDir = "media" },
		func(r *FileRules) { r.ArchivesScope = ArchivesScopeDocumentation },
	}
	for i, change := range changes {
		r := DefaultFileRules()
		change(&r)
		if r.Fingerprint("alice/projet", []string{"v1.0.0", "v2.0.0"}) == fp {
			t.Errorf("changement %d sans effet sur l'empreinte", i)
		}
	}
	if base.Fingerprint("alice/projet", []string{"v1.0.0"}) == fp {
		t.Error("les tags entrent dans l'empreinte (CL8)")
	}
	if base.Fingerprint("alice/renomme", []string{"v1.0.0", "v2.0.0"}) == fp {
		t.Error("le dépôt entre dans l'empreinte (RD3, DOC3, DOC4)")
	}
}

func TestResolveLink(t *testing.T) {
	cases := []struct {
		source, target string
		want           string
		kind           linkKind
	}{
		{"README.md", "documentation/INDEX.md", "documentation/INDEX.md", linkRelative},
		{"README.md", "./CHANGELOG.md#unreleased", "CHANGELOG.md", linkRelative},
		{"README.md", "/documentation/INDEX.md", "documentation/INDEX.md", linkRelative},
		{"documentation/INDEX.md", "01-intro/a%20b.md?plain=1", "documentation/01-intro/a b.md", linkRelative},
		{"documentation/INDEX.md", "../README.md", "README.md", linkRelative},
		{"documentation/INDEX.md", "../../hors.md", "../hors.md", linkRelative},
		{"documentation/INDEX.md", "./", "documentation", linkRelative},
		{"README.md", "/", "", linkRelative},
		{"README.md", "#installation", "", linkSkip},
		{"README.md", "", "", linkSkip},
		{"README.md", "?q", "", linkSkip},
		{"README.md", "mailto:a@b.fr", "", linkSkip},
		{"README.md", "https://exemple.fr/CHANGELOG.md", "", linkSkip},
		{"README.md", "//cdn.exemple.fr/x.js", "", linkSkip},
		{"README.md", "https://github.com/promo/projet/blob/main/documentation/INDEX.md#x", "documentation/INDEX.md", linkRepoURL},
		{"README.md", "HTTPS://GitHub.com/Promo/Projet.git/tree/main/documentation", "documentation", linkRepoURL},
		{"README.md", "https://github.com/promo/projet", "", linkRepoURL},
		{"README.md", "https://github.com/promo/projet/tree/main", "", linkRepoURL},
		{"README.md", "https://github.com/promo/projet/issues/3", "", linkSkip},
		{"README.md", "https://github.com/autre/projet/blob/main/CHANGELOG.md", "", linkSkip},
	}
	for _, tc := range cases {
		got, kind := resolveLink(tc.source, tc.target, "promo/projet")
		if got != tc.want || kind != tc.kind {
			t.Errorf("resolveLink(%q, %q) = %q, %d ; attendu %q, %d", tc.source, tc.target, got, kind, tc.want, tc.kind)
		}
	}
	if _, kind := resolveLink("README.md", "https://github.com/promo/projet/blob/main/X.md", ""); kind != linkSkip {
		t.Error("sans dépôt connu, aucune URL n'est admise")
	}
	if got := percentDecode("a%2Fb%zz%4"); got != "a/b%zz%4" {
		t.Errorf("percentDecode = %q", got)
	}
}

func TestFileBlobPaths(t *testing.T) {
	var docs []string
	paths := []string{"CHANGELOG.md", "README.md", "README.fr.md", "src/main.go", "package.json",
		"front/package.json", "a/b/c/package.json", "node_modules/x/package.json", "commitlint.config.js",
		".github/PULL_REQUEST_TEMPLATE.md", "lefthook.yml", "pyproject.toml", ".pre-commit-config.yaml",
		"documentation/INDEX.md", "documentation/Media/notes.md", "documentation/01-intro/a.md",
		"documentation/01-intro/schema.png"}
	for i := range MaxDocuments + 5 {
		docs = append(docs, "documentation/02-x/"+string(rune('a'+i%26))+strings.Repeat("z", i/26)+".md")
	}
	got := FileBlobPaths(append(paths, docs...), DefaultFileRules())
	for _, want := range []string{"CHANGELOG.md", "README.md", "package.json", "front/package.json",
		"commitlint.config.js", ".github/PULL_REQUEST_TEMPLATE.md", "lefthook.yml", "pyproject.toml",
		".pre-commit-config.yaml", "documentation/INDEX.md", "documentation/01-intro/a.md"} {
		if !slices.Contains(got, want) {
			t.Errorf("%q absent de %q", want, got)
		}
	}
	for _, not := range []string{"README.fr.md", "src/main.go", "a/b/c/package.json", "node_modules/x/package.json",
		"documentation/Media/notes.md", "documentation/01-intro/schema.png"} {
		if slices.Contains(got, not) {
			t.Errorf("%q ne devrait pas être lu", not)
		}
	}
	n := 0
	for _, p := range got {
		if strings.HasPrefix(p, "documentation/") && p != "documentation/INDEX.md" {
			n++
		}
	}
	if n != MaxDocuments {
		t.Errorf("documents lus = %d, attendu %d", n, MaxDocuments)
	}
	if !slices.IsSorted(got) {
		t.Error("liste non triée")
	}
}

// TestFileResultsHoldNoPath vérifie qu'un résultat sérialisé ne contient ni chemin ni contenu.
func TestFileResultsHoldNoPath(t *testing.T) {
	tree := treeOf(map[string]string{
		"README.md":                         "# Secret-Projet\n\nVoir [doc](documentation/manquant-secret.md).\n",
		"CHANGELOG.md":                      "# Notes secretes\n\n## [1.0.0] - 2026-01-01\n\n- Alice Martin a tout fait\n",
		"documentation/INDEX.md":            "| Document | Thème | Version | Date | Responsable |\n|---|---|---|---|---|\n| [a](01-x/doc-secrete.md) | t | 1 | d | Alice Martin |\n",
		"documentation/Archive Secrète.zip": "",
		"documentation/01-x/doc-secrete.md": "texte",
		"documentation/Rapport Secret.docx": "",
	})
	res, lists := CheckFiles(tree, fileOpts())
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"ecret", "Alice", "Martin", "documentation", "README", ".md", ".zip"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("le résultat contient %q : %s", leak, raw)
		}
	}
	if res.RulesVersion != FileRulesVersion || len(res.Changelog) != 8 || len(res.Readme) != 4 ||
		len(res.Docs) != 6 || len(res.Tooling) != 3 {
		t.Errorf("grilles = %+v", res)
	}
	if len(lists.Archives) != 1 || len(lists.OfficeDocuments) != 1 || len(lists.DeadLinks) != 1 {
		t.Errorf("listes = %+v", lists)
	}
	var back FileResults
	if err := json.Unmarshal(raw, &back); err != nil || !reflect.DeepEqual(back, res) {
		t.Errorf("aller-retour JSON : %v\n%+v\n%+v", err, back, res)
	}
	if c, ok := res.Find(CriterionDOC8); !ok || c.ID != CriterionDOC8 {
		t.Error("Find(DOC8)")
	}
	if _, ok := res.Find(CriterionCL9); ok {
		t.Error("CL9 n'est mesuré que sur l'arbre d'un jalon")
	}
	if k, u := TallyChecks(res.Docs); k+u > len(res.Docs) {
		t.Errorf("TallyChecks = %d, %d", k, u)
	}
}

func TestFileTreeUnusable(t *testing.T) {
	for _, tc := range []struct {
		tree   FileTree
		reason string
	}{
		{FileTree{}, UnknownEmptyRepository},
		{FileTree{Paths: []string{"", "/abs", "../x", "a/../b"}}, UnknownEmptyRepository},
		{FileTree{Paths: []string{"README.md"}, Truncated: true}, UnknownTreeTruncated},
	} {
		opts := fileOpts()
		opts.Version = "v1.0.0"
		res, _ := CheckFiles(tc.tree, opts)
		for _, fam := range [][]FileCheck{res.Changelog, res.Readme, res.Docs, res.Tooling} {
			for _, c := range fam {
				if c.Status != CriterionUnknown || c.Reason != tc.reason {
					t.Errorf("%s : %q %q, attendu inconnu %q", c.ID, c.Status, c.Reason, tc.reason)
				}
			}
		}
		if len(res.Changelog) != 9 {
			t.Errorf("CL9 attendu pour un jalon : %d critères", len(res.Changelog))
		}
	}
}
