package conventions

// Outillage (TL1 à TL3, phase 8.3) : configuration commitlint, hook commit-msg versionné, gabarit de pull
// request. Un hook est « configuré » dans le dépôt, jamais « actif » : .git/hooks n'est pas versionné.

import (
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Outils de hook commit-msg reconnus (TL2), dans l'ordre des bits de CountTools.
const (
	HookHusky      = "husky"
	HookLefthook   = "lefthook"
	HookPreCommit  = "pre-commit"
	HookCommitizen = "commitizen"
	HookCocogitto  = "cocogitto"
)

// AllHookTools liste les outils de TL2 dans l'ordre des bits de CountTools.
var AllHookTools = []string{HookHusky, HookLefthook, HookPreCommit, HookCommitizen, HookCocogitto}

// Sections de la charte d'un corps de pull request (TL3), dans l'ordre de la charte.
const (
	PRSectionContext     = "context"
	PRSectionChanges     = "changes"
	PRSectionTesting     = "testing"
	PRSectionScreenshots = "screenshots"
	PRSectionCloses      = "closes"
)

// AllPRSections liste les sections de la charte d'un corps de pull request.
var AllPRSections = []string{PRSectionContext, PRSectionChanges, PRSectionTesting, PRSectionScreenshots,
	PRSectionCloses}

// prSectionSynonyms sont les titres reconnus des sections d'un gabarit, normalisés ; « closes » est reconnu
// dans le texte (« Closes #12 », « Fixes # »).
var prSectionSynonyms = map[string][]string{
	PRSectionContext: {"contexte", "context", "description", "pourquoi", "why", "motivation", "objectif"},
	PRSectionChanges: {"ce qui change", "changements", "changes", "modifications", "what changed",
		"what changes", "changes made", "ce qui a change"},
	PRSectionTesting: {"comment tester", "how to test", "tests", "testing", "test plan", "verification",
		"how has this been tested"},
	PRSectionScreenshots: {"captures", "captures d ecran", "capture d ecran", "screenshots", "screenshot",
		"apercu", "preview"},
	PRSectionCloses: {"issue liee", "issues liees", "ticket", "tickets", "related issue", "related issues",
		"closes"},
}

var (
	closesRe          = regexp.MustCompile(`(?i)\b(close[sd]?|fix(e[sd])?|resolve[sd]?|ferme)\s*:?\s*#`)
	commitlintFileRe  = regexp.MustCompile(`^(commitlint\.config\.(js|cjs|mjs|ts|cts|mts)|\.commitlintrc(\.(json|ya?ml|js|cjs|mjs|ts|cts))?)$`)
	lefthookFileRe    = regexp.MustCompile(`^\.?lefthook(-local)?\.(ya?ml|json|toml)$`)
	commitizenFileRe  = regexp.MustCompile(`^(\.?cz\.(toml|json|ya?ml)|\.czrc)$`)
	prTemplateFileRe  = regexp.MustCompile(`(?i)^pull_request_template\.md$`)
	prTemplateDirName = "pull_request_template"
)

// toolDepth est la profondeur maximale (nombre de dossiers) où une configuration est cherchée.
const toolDepth = 2

// shallow indique un chemin d'au plus toolDepth dossiers, hors dépendances versionnées.
func shallow(p string) bool { return strings.Count(p, "/") <= toolDepth && !isVendored(p) }

// toolingBlobs renvoie les fichiers d'outillage dont le contenu est lu.
func toolingBlobs(idx *treeIndex) []string {
	var out []string
	for _, p := range idx.paths {
		base := path.Base(p)
		root := !strings.Contains(p, "/")
		switch {
		case shallow(p) && (base == "package.json" || commitlintFileRe.MatchString(base)):
			out = append(out, p)
		case root && (lefthookFileRe.MatchString(base) || base == ".pre-commit-config.yaml" ||
			base == ".pre-commit-config.yml" || base == "pyproject.toml"):
			out = append(out, p)
		case isPRTemplate(p):
			out = append(out, p)
		}
	}
	return out
}

// isPRTemplate reconnaît un gabarit de pull request aux emplacements lus par GitHub (casse ignorée) :
// pull_request_template.md à la racine, dans .github/ ou docs/, ou un .md de pull_request_template/ dans
// ces mêmes dossiers.
func isPRTemplate(p string) bool {
	dir, base := path.Dir(p), path.Base(p)
	inBase := func(d string) bool {
		d = strings.ToLower(d)
		return d == "." || d == ".github" || d == "docs"
	}
	if prTemplateFileRe.MatchString(base) {
		return inBase(dir)
	}
	return strings.EqualFold(path.Base(dir), prTemplateDirName) && inBase(path.Dir(dir)) &&
		strings.EqualFold(path.Ext(base), ".md")
}

// packageJSON est ce que l'outillage lit d'un package.json.
type packageJSON struct {
	Commitlint      json.RawMessage   `json:"commitlint"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Scripts         map[string]string `json:"scripts"`
	Config          struct {
		Commitizen json.RawMessage `json:"commitizen"`
	} `json:"config"`
}

// readPackage lit un package.json ; faux s'il n'est pas lu ou pas du JSON.
func readPackage(tree FileTree, p string) (packageJSON, bool, bool) {
	data, reason := tree.readBlob(p)
	if reason != "" {
		return packageJSON{}, false, true
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return packageJSON{}, false, false
	}
	return pkg, true, false
}

// DetectTooling mesure TL1 à TL3.
func DetectTooling(tree FileTree, opts FileOptions) []FileCheck {
	idx := newTreeIndex(tree.Paths)
	if reason := treeUnknown(tree, idx); reason != "" {
		return unknownAll(ToolingCriteria, reason)
	}
	return []FileCheck{checkTL1(tree, idx), checkTL2(tree, idx), checkTL3(tree, idx)}
}

// checkTL1 : fichier de configuration commitlint (profondeur au plus 2) ou clé commitlint /
// dépendance @commitlint/cli d'un package.json ; CountExtends vaut 1 si une configuration étend
// @commitlint/config-conventional.
func checkTL1(tree FileTree, idx *treeIndex) FileCheck {
	c := newCheck(CriterionTL1)
	c.counts[CountConfigs], c.counts[CountExtends] = 0, 0
	unread := 0
	for _, p := range idx.paths {
		if !shallow(p) {
			continue
		}
		base := path.Base(p)
		switch {
		case commitlintFileRe.MatchString(base):
			c.counts[CountConfigs]++
			if data, reason := tree.readBlob(p); reason == "" && strings.Contains(string(data), "config-conventional") {
				c.counts[CountExtends] = 1
			}
		case base == "package.json":
			pkg, ok, notRead := readPackage(tree, p)
			if notRead {
				unread++
			}
			if !ok {
				continue
			}
			_, dep := pkg.Dependencies["@commitlint/cli"]
			_, dev := pkg.DevDependencies["@commitlint/cli"]
			if len(pkg.Commitlint) > 0 || dep || dev {
				c.counts[CountConfigs]++
			}
			if strings.Contains(string(pkg.Commitlint), "config-conventional") {
				c.counts[CountExtends] = 1
			}
		}
	}
	if c.counts[CountConfigs] == 0 {
		if unread > 0 {
			fc := unknownCheck(CriterionTL1, UnknownBlobUnreadable)
			fc.Counts[CountUnread] = unread
			return fc
		}
		c.add(FileNotFound)
	}
	return c.done()
}

// checkTL2 : hook commit-msg versionné : .husky/commit-msg (le script « prepare » qui installe husky est
// vérifié), lefthook ou pre-commit avec une entrée commit-msg, configuration commitizen ou cocogitto.
func checkTL2(tree FileTree, idx *treeIndex) FileCheck {
	c := newCheck(CriterionTL2)
	tools := 0
	unread := 0
	set := func(tool string) { tools |= 1 << slices.Index(AllHookTools, tool) }
	contains := func(p, s string) bool {
		data, reason := tree.readBlob(p)
		if reason != "" {
			unread++
			return false
		}
		return strings.Contains(string(data), s)
	}
	prepare, prepareKnown := false, false
	for _, p := range idx.paths {
		base := path.Base(p)
		root := !strings.Contains(p, "/")
		switch {
		case shallow(p) && (p == ".husky/commit-msg" || strings.HasSuffix(p, "/.husky/commit-msg")):
			set(HookHusky)
		case root && lefthookFileRe.MatchString(base):
			if contains(p, "commit-msg") {
				set(HookLefthook)
			}
		case root && (base == ".pre-commit-config.yaml" || base == ".pre-commit-config.yml"):
			if contains(p, "commit-msg") {
				set(HookPreCommit)
			}
		case root && commitizenFileRe.MatchString(base):
			set(HookCommitizen)
		case root && base == "pyproject.toml":
			if contains(p, "[tool.commitizen]") {
				set(HookCommitizen)
			}
		case root && base == "cog.toml":
			set(HookCocogitto)
		case shallow(p) && base == "package.json":
			pkg, ok, _ := readPackage(tree, p)
			if !ok {
				continue
			}
			prepareKnown = true
			if strings.Contains(pkg.Scripts["prepare"], "husky") {
				prepare = true
			}
			if root && len(pkg.Config.Commitizen) > 0 {
				set(HookCommitizen)
			}
		}
	}
	c.counts[CountTools] = tools
	switch {
	case tools == 0 && unread > 0:
		fc := unknownCheck(CriterionTL2, UnknownBlobUnreadable)
		fc.Counts[CountUnread] = unread
		return fc
	case tools == 0:
		c.add(FileNotFound)
	case tools&1 != 0 && prepareKnown && !prepare:
		c.add(FileHuskyNoPrepare)
	}
	return c.done()
}

// checkTL3 : gabarit de pull request aux emplacements lus par GitHub ; CountSections donne le plus grand
// nombre de sections de la charte trouvées dans un gabarit lu. Un gabarit d'organisation est invisible.
func checkTL3(tree FileTree, idx *treeIndex) FileCheck {
	c := newCheck(CriterionTL3)
	c.counts[CountExpected] = len(AllPRSections)
	c.counts[CountTemplates], c.counts[CountSections] = 0, 0
	for _, p := range idx.paths {
		if !isPRTemplate(p) {
			continue
		}
		c.counts[CountTemplates]++
		data, reason := tree.readBlob(p)
		if reason != "" {
			c.counts[CountUnread]++
			continue
		}
		c.counts[CountSections] = max(c.counts[CountSections], len(prSectionsIn(data)))
	}
	if c.counts[CountTemplates] == 0 {
		c.add(FileNotFound)
	}
	return c.done()
}

// prSectionsIn renvoie les sections de la charte présentes dans un gabarit.
func prSectionsIn(data []byte) []string {
	doc := parseMarkdown(data)
	found := map[string]bool{}
	for _, l := range doc.Lines {
		if l.Kind == mdHeading {
			h := normalizeHeading(l.Text)
			for key, syns := range prSectionSynonyms {
				if slices.ContainsFunc(syns, func(s string) bool { return containsPhrase(h, s) }) {
					found[key] = true
				}
			}
		}
		if closesRe.MatchString(l.Text) {
			found[PRSectionCloses] = true
		}
	}
	var out []string
	for _, key := range AllPRSections {
		if found[key] {
			out = append(out, key)
		}
	}
	return out
}
