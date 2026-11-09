package conventions

// README (RD1 à RD4, phase 8.3) : présence à la racine, sections attendues reconnues par leur titre
// (synonymes français et anglais), liens vers l'INDEX et le CHANGELOG, taille en mots hors code.

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// ReadmeFile est le nom attendu à la racine.
const ReadmeFile = "README.md"

// Clés des sections attendues du README (réglage conventions.readme.sections), dans l'ordre de la charte ;
// l'ordre fixe les bits de CountFoundMask.
const (
	SectionPresentation = "presentation" // présentation du projet
	SectionGroup        = "group"        // groupe, équipe
	SectionStack        = "stack"        // stack et prérequis
	SectionInstall      = "install"      // installation
	SectionRun          = "run"          // lancement, utilisation
	SectionEnv          = "env"          // variables d'environnement
	SectionTree         = "tree"         // arborescence
	SectionLicense      = "license"      // licence ou confidentialité
)

// AllReadmeSections renvoie les clés des sections du README, dans l'ordre de la charte.
func AllReadmeSections() []string {
	return []string{SectionPresentation, SectionGroup, SectionStack, SectionInstall, SectionRun, SectionEnv,
		SectionTree, SectionLicense}
}

// ValidReadmeSection indique une clé de section connue.
func ValidReadmeSection(key string) bool { return slices.Contains(AllReadmeSections(), key) }

// validReadmeSections garde les clés connues, dans l'ordre de la charte, sans doublon.
func validReadmeSections(keys []string) []string {
	var out []string
	for _, k := range AllReadmeSections() {
		if slices.Contains(keys, k) {
			out = append(out, k)
		}
	}
	return out
}

// readmeSynonyms sont les titres reconnus pour chaque section, normalisés (minuscules, sans accent ni
// ponctuation). Un titre reconnaît une section s'il contient l'un d'eux mot pour mot (« Installation et
// lancement » reconnaît install et run). Liste versionnée par FileRulesVersion.
var readmeSynonyms = map[string][]string{
	SectionPresentation: {"presentation", "description", "a propos", "about", "overview", "introduction",
		"apercu", "contexte", "context", "resume", "summary", "objectif", "objectifs", "le projet", "the project",
		"about the project", "a propos du projet"},
	SectionGroup: {"groupe", "equipe", "team", "membres", "members", "auteur", "auteurs", "author", "authors",
		"contributeurs", "contributeur", "contributors", "credits", "realise par", "made by"},
	SectionStack: {"stack", "technologies", "technologie", "technos", "techno", "prerequis", "pre requis",
		"requirements", "prerequisites", "dependances", "dependencies", "built with", "outils", "tools",
		"tech stack", "environnement technique"},
	SectionInstall: {"installation", "install", "installer", "setup", "mise en place", "getting started",
		"pour commencer", "deploiement", "deployment"},
	SectionRun: {"lancement", "lancer", "execution", "executer", "demarrage", "demarrer", "utilisation",
		"usage", "run", "running", "start", "quick start", "quickstart", "how to run", "how to use"},
	SectionEnv: {"variables d environnement", "variables", "environment variables", "environment", "env",
		"configuration", "config", "parametrage"},
	SectionTree: {"arborescence", "structure", "structure du projet", "project structure", "organisation",
		"organisation du projet", "tree", "folder structure", "architecture", "contenu du depot"},
	SectionLicense: {"licence", "license", "licensing", "confidentialite", "privacy", "droits",
		"droits d auteur", "copyright"},
}

var (
	readmeNameRe        = regexp.MustCompile(`(?i)^readme(\.(md|markdown|txt|rst))?$`)
	readmeTranslationRe = regexp.MustCompile(`(?i)^readme\.[a-z]{2}([-_][a-z]{2})?\.md$`)
	licenseNameRe       = regexp.MustCompile(`(?i)^(licen[cs]e|copying)(\.(md|txt))?$`)
)

// findReadme cherche le README : README.md à la racine, sinon un nom proche à la racine (« readme.md »,
// « README », « README.txt »), sinon README.md dans un sous-dossier. README.<langue>.md ne compte pas.
func findReadme(idx *treeIndex) located {
	if idx.files[ReadmeFile] {
		return located{path: ReadmeFile}
	}
	var root, sub []string
	for _, p := range idx.paths {
		base := path.Base(p)
		if !readmeNameRe.MatchString(base) || isVendored(p) {
			continue
		}
		if !strings.Contains(p, "/") {
			root = append(root, p)
		} else if strings.EqualFold(base, ReadmeFile) {
			sub = append(sub, p)
		}
	}
	if len(root) > 0 {
		return located{path: pickShallowest(root), code: FileMisnamed}
	}
	if len(sub) > 0 {
		return located{path: pickShallowest(sub), code: FileElsewhere}
	}
	return located{code: FileAbsent}
}

// CheckReadme mesure RD1 à RD4.
func CheckReadme(tree FileTree, opts FileOptions) []FileCheck {
	rules := opts.Rules.normalized()
	idx := newTreeIndex(tree.Paths)
	if reason := treeUnknown(tree, idx); reason != "" {
		return unknownAll(ReadmeCriteria, reason)
	}
	loc := findReadme(idx)
	rd1 := newCheck(CriterionRD1)
	if loc.code != "" {
		rd1.add(loc.code)
	}
	for _, p := range idx.paths {
		if !strings.Contains(p, "/") && readmeTranslationRe.MatchString(p) {
			rd1.counts[CountTranslations]++
		}
	}
	if loc.code == FileAbsent && rd1.counts[CountTranslations] > 0 {
		rd1.add(FileTranslationOnly)
	}
	out := []FileCheck{rd1.done()}
	if loc.path == "" {
		return append(out, unknownAll(ReadmeCriteria[1:], UnknownNoFile)...)
	}
	data, reason := tree.readBlob(loc.path)
	if reason != "" {
		return append(out, unknownAll(ReadmeCriteria[1:], reason)...)
	}
	doc := parseMarkdown(data)
	return append(out, checkRD2(doc, idx, rules), checkRD3(doc, loc.path, idx, rules, opts.Repo), checkRD4(doc, rules))
}

// readmeSectionsFound renvoie les sections reconnues par les titres du document.
func readmeSectionsFound(doc mdDoc) map[string]bool {
	found := map[string]bool{}
	for _, l := range doc.Lines {
		if l.Kind != mdHeading {
			continue
		}
		h := normalizeHeading(l.Text)
		for key, syns := range readmeSynonyms {
			if slices.ContainsFunc(syns, func(s string) bool { return containsPhrase(h, s) }) {
				found[key] = true
			}
		}
	}
	return found
}

// checkRD2 : sections attendues trouvées par leur titre ; un fichier LICENSE à la racine vaut section
// licence (noté par CountLicenseFile). Seuls des masques et des nombres sont gardés.
func checkRD2(doc mdDoc, idx *treeIndex, rules FileRules) FileCheck {
	c := newCheck(CriterionRD2)
	found := readmeSectionsFound(doc)
	for _, p := range idx.paths {
		if !strings.Contains(p, "/") && licenseNameRe.MatchString(p) {
			c.counts[CountLicenseFile] = 1
			found[SectionLicense] = true
		}
	}
	expected := validReadmeSections(rules.ReadmeSections)
	c.counts[CountExpected] = len(expected)
	c.counts[CountFound] = 0
	c.counts[CountFoundMask] = 0
	c.counts[CountExpectedMask] = 0
	for i, key := range AllReadmeSections() {
		bit := 1 << i
		if found[key] {
			c.counts[CountFoundMask] |= bit
		}
		if !slices.Contains(expected, key) {
			continue
		}
		c.counts[CountExpectedMask] |= bit
		if found[key] {
			c.counts[CountFound]++
		} else {
			c.addCount(FileMissingSection)
		}
	}
	return c.done()
}

// ReadmeSectionFound indique, d'après RD2, si la section key a été trouvée ; faux si RD2 n'a pas été mesuré.
func ReadmeSectionFound(rd2 FileCheck, key string) bool {
	i := slices.Index(AllReadmeSections(), key)
	if rd2.ID != CriterionRD2 || rd2.Status == CriterionUnknown || i < 0 {
		return false
	}
	return rd2.Counts[CountFoundMask]&(1<<i) != 0
}

// États d'un lien attendu du README (RD3), du meilleur au pire.
const (
	linkValid = iota
	linkFolder
	linkDead
	linkAbsent
)

// checkRD3 : liens du README vers l'INDEX de la documentation et vers le CHANGELOG. Un lien est relatif,
// absolu depuis la racine (« /documentation/INDEX.md ») ou une URL github.com du même dépôt.
func checkRD3(doc mdDoc, source string, idx *treeIndex, rules FileRules, repo string) FileCheck {
	c := newCheck(CriterionRD3)
	index, changelog := linkAbsent, linkAbsent
	for _, l := range doc.Links {
		if l.Image {
			continue
		}
		p, kind := resolveLink(source, l.Target, repo)
		if kind == linkSkip {
			continue
		}
		switch {
		case isIndexLike(p, rules):
			index = min(index, fileLinkState(idx, p))
		case p != "" && strings.EqualFold(p, rules.DocsDir):
			if idx.dirs[p] {
				index = min(index, linkFolder)
			} else {
				index = min(index, linkDead)
			}
		case changelogNameRe.MatchString(path.Base(p)):
			changelog = min(changelog, fileLinkState(idx, p))
		}
	}
	switch index {
	case linkAbsent:
		c.add(FileIndexLinkAbsent)
	case linkDead:
		c.add(FileIndexLinkDead)
	case linkFolder:
		c.add(FileIndexLinkFolder)
	}
	switch changelog {
	case linkAbsent:
		c.add(FileChangelogLinkAbsent)
	case linkDead:
		c.add(FileChangelogLinkDead)
	}
	return c.done()
}

// isIndexLike indique un chemin qui désigne un INDEX de documentation : « index.md » (casse ignorée) à la
// racine, dans le dossier de documentation ou dans un dossier doc, docs ou documentation.
func isIndexLike(p string, rules FileRules) bool {
	if !strings.EqualFold(path.Base(p), "index.md") {
		return false
	}
	dir := strings.ToLower(path.Dir(p))
	return dir == "." || dir == strings.ToLower(rules.DocsDir) || slices.Contains(docsDirNames, path.Base(dir))
}

// fileLinkState : valide si le fichier visé existe tel quel, mort sinon (une autre casse ne suffit pas :
// GitHub distingue la casse).
func fileLinkState(idx *treeIndex, p string) int {
	if idx.files[p] {
		return linkValid
	}
	return linkDead
}

// checkRD4 : nombre de mots hors code, comparé à readme.min_words.
func checkRD4(doc mdDoc, rules FileRules) FileCheck {
	c := newCheck(CriterionRD4)
	c.counts[CountWords] = doc.Words
	c.counts[CountMinWords] = rules.ReadmeMinWords
	if doc.Words < rules.ReadmeMinWords {
		c.add(FileTooShort)
	}
	return c.done()
}
