package conventions

// CHANGELOG (CL1 à CL9, phase 8.3) : présence, en-tête, [Unreleased], titres de version et dates, ordre,
// catégories, entrées vides, cohérence avec les tags et section du jalon. Le fichier est lu en mémoire ;
// les blocs de code sont ignorés. Référence : Keep a Changelog 1.1.0.

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/semver"
)

// ChangelogFile est le nom attendu à la racine.
const ChangelogFile = "CHANGELOG.md"

var (
	changelogNameRe = regexp.MustCompile(`(?i)^change[-_]?log(\.md)?$`)
	isoDateRe       = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	frDateRe        = regexp.MustCompile(`^[0-9]{1,2}[/.-][0-9]{1,2}[/.-][0-9]{2,4}$`)
	yankedRe        = regexp.MustCompile(`(?i)\[\s*yanked\s*\]`)
)

// Catégories de Keep a Changelog, dans leur ordre, et leurs libellés français (normalisés).
var (
	changelogCategories   = []string{"added", "changed", "deprecated", "removed", "fixed", "security"}
	changelogCategoriesFR = map[string]string{
		"ajoute": "added", "ajoutes": "added", "ajout": "added", "ajouts": "added",
		"modifie": "changed", "modifies": "changed", "modification": "changed", "modifications": "changed",
		"obsolete": "deprecated", "obsoletes": "deprecated",
		"supprime": "removed", "supprimes": "removed", "suppression": "removed", "suppressions": "removed",
		"corrige": "fixed", "corriges": "fixed", "correction": "fixed", "corrections": "fixed",
		"securite": "security",
	}
	unreleasedFR = []string{"non publie", "non publiee", "non publies", "a venir"}
)

// findChangelog cherche le CHANGELOG : CHANGELOG.md à la racine, sinon un nom proche à la racine
// (« changelog.md », « CHANGELOG », « change-log.md »), sinon un nom proche dans un sous-dossier.
func findChangelog(idx *treeIndex) located {
	if idx.files[ChangelogFile] {
		return located{path: ChangelogFile}
	}
	var root, sub []string
	for _, p := range idx.paths {
		if !changelogNameRe.MatchString(path.Base(p)) || isVendored(p) {
			continue
		}
		if strings.Contains(p, "/") {
			sub = append(sub, p)
		} else {
			root = append(root, p)
		}
	}
	if len(root) > 0 {
		return located{path: pickShallowest(root), code: FileMisnamed}
	}
	if len(sub) > 0 {
		// Préférer un CHANGELOG.md exact à un nom approchant.
		var exact []string
		for _, p := range sub {
			if path.Base(p) == ChangelogFile {
				exact = append(exact, p)
			}
		}
		if len(exact) > 0 {
			return located{path: pickShallowest(exact), code: FileElsewhere}
		}
		return located{path: pickShallowest(sub), code: FileElsewhere}
	}
	return located{code: FileAbsent}
}

// clCategory est une catégorie (titre de niveau 3) d'une section.
type clCategory struct {
	canonical string // nom Keep a Changelog (« added »…) ; vide si inconnu
	french    bool
	bullets   int
}

// clSection est une section de niveau 2 du CHANGELOG.
type clSection struct {
	unreleased bool
	french     bool // [Unreleased] écrit en français
	bracketed  bool
	version    semver.Version
	hasVersion bool
	date       time.Time
	hasDate    bool
	yanked     bool
	codes      []string // codes CL4 de la section
	categories []clCategory
	outside    int // puces avant la première catégorie
	bullets    int // toutes les puces
	empty      int // puces vides ou « TODO »
}

// clDoc est un CHANGELOG analysé.
type clDoc struct {
	h1       string // texte du premier titre de niveau 1 ; vide si aucun
	hasH1    bool
	preamble string // texte avant la première section, normalisé (titres, paragraphes, liens)
	rawPre   string // même texte, brut (URL comprises)
	sections []clSection
}

// parseChangelog structure le CHANGELOG.
func parseChangelog(doc mdDoc, rules FileRules, today string) clDoc {
	var out clDoc
	var pre, raw []string
	var cur *clSection
	var cat *clCategory
	for _, l := range doc.Lines {
		switch {
		case l.Kind == mdHeading && l.Level == 1 && cur == nil && !out.hasH1:
			out.h1, out.hasH1 = l.Text, true
			pre, raw = append(pre, l.Text), append(raw, l.Text)
		case l.Kind == mdHeading && l.Level == 2:
			out.sections = append(out.sections, parseSectionHeading(l.Text, rules, today))
			cur = &out.sections[len(out.sections)-1]
			cat = nil
		case cur == nil:
			if l.Kind != mdBlank {
				pre, raw = append(pre, l.Text), append(raw, l.Text)
			}
		case l.Kind == mdHeading && l.Level == 3:
			cur.categories = append(cur.categories, parseCategory(l.Text, rules))
			cat = &cur.categories[len(cur.categories)-1]
		case l.Kind == mdHeading && l.Level < 3:
			cat = nil
		case l.Kind == mdBullet:
			cur.bullets++
			if t := normalizeText(l.Text); t == "" || t == "todo" {
				cur.empty++
			}
			if cat == nil {
				cur.outside++
			} else {
				cat.bullets++
			}
		}
	}
	out.preamble = normalizeText(strings.Join(pre, " "))
	out.rawPre = strings.ToLower(strings.Join(raw, " "))
	return out
}

// parseCategory lit une catégorie : nom anglais, libellé français ou inconnu.
func parseCategory(text string, rules FileRules) clCategory {
	n := normalizeHeading(text)
	if slices.Contains(changelogCategories, n) {
		return clCategory{canonical: n}
	}
	if c, ok := changelogCategoriesFR[n]; ok && rules.CategoriesFR {
		return clCategory{canonical: c, french: true}
	}
	return clCategory{}
}

// parseSectionHeading lit un titre de niveau 2 : [Unreleased], ou « [X.Y.Z] - AAAA-MM-JJ [YANKED] ».
func parseSectionHeading(text string, rules FileRules, today string) clSection {
	var s clSection
	t := strings.TrimSpace(text)
	var label, rest string
	if strings.HasPrefix(t, "[") {
		s.bracketed = true
		end := strings.Index(t, "]")
		if end < 0 {
			label, rest = t[1:], ""
		} else {
			label, rest = t[1:end], t[end+1:]
			// « [1.0.0](https://…) » : le lien du titre est ignoré.
			if strings.HasPrefix(rest, "(") {
				if c := strings.Index(rest, ")"); c >= 0 {
					rest = rest[c+1:]
				}
			}
		}
	} else {
		label, rest, _ = strings.Cut(t, " ")
	}
	label = strings.TrimSpace(label)
	n := normalizeText(label)
	if !s.bracketed && n != "unreleased" {
		// Sans crochets, le titre entier est le libellé (« ## Non publié »).
		n = normalizeText(t)
	}
	switch {
	case n == "unreleased":
		s.unreleased = true
		return s
	case slices.Contains(unreleasedFR, n) && rules.CategoriesFR:
		s.unreleased, s.french = true, true
		return s
	}
	if !s.bracketed {
		s.codes = append(s.codes, FileNoBrackets)
	}
	ver := label
	if strings.HasPrefix(ver, "v") || strings.HasPrefix(ver, "V") {
		s.codes = append(s.codes, FileVPrefix)
		ver = ver[1:]
	}
	if v, ok := semver.ParseVersion(ver); ok && !strings.HasPrefix(ver, "v") {
		s.version, s.hasVersion = v, true
	} else {
		s.codes = append(s.codes, FileInvalidVersion)
	}
	s.yanked = yankedRe.MatchString(rest)
	rest = strings.TrimSpace(yankedRe.ReplaceAllString(rest, " "))
	rest = strings.TrimSpace(strings.TrimLeft(rest, "-–—: \t"))
	date, _, _ := strings.Cut(rest, " ")
	date = strings.Trim(date, "()")
	switch {
	case date == "":
		s.codes = append(s.codes, FileNoDate)
	case isoDateRe.MatchString(date):
		d, err := time.Parse(time.DateOnly, date)
		switch {
		case err != nil:
			s.codes = append(s.codes, FileInvalidDate)
		case date > today:
			s.codes = append(s.codes, FileFutureDate)
			s.date, s.hasDate = d, true
		default:
			s.date, s.hasDate = d, true
		}
	case frDateRe.MatchString(date):
		s.codes = append(s.codes, FileDateFR)
	default:
		s.codes = append(s.codes, FileInvalidDate)
	}
	return s
}

// CheckChangelog mesure CL1 à CL8 (et CL9 quand opts.Version désigne le jalon de l'arbre).
func CheckChangelog(tree FileTree, opts FileOptions) []FileCheck {
	rules := opts.Rules.normalized()
	ids := ChangelogCriteria
	if opts.Version == "" {
		ids = ids[:len(ids)-1]
	}
	idx := newTreeIndex(tree.Paths)
	if reason := treeUnknown(tree, idx); reason != "" {
		return unknownAll(ids, reason)
	}
	loc := findChangelog(idx)
	cl1 := newCheck(CriterionCL1)
	if loc.code != "" {
		cl1.add(loc.code)
	}
	out := []FileCheck{cl1.done()}
	if loc.path == "" {
		return append(out, unknownAll(ids[1:], UnknownNoFile)...)
	}
	data, reason := tree.readBlob(loc.path)
	if reason != "" {
		return append(out, unknownAll(ids[1:], reason)...)
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	cl := parseChangelog(parseMarkdown(data), rules, now.Format(time.DateOnly))
	out = append(out, checkCL2(cl, rules), checkCL3(cl), checkCL4(cl), checkCL5(cl), checkCL6(cl), checkCL7(cl),
		checkCL8(cl, opts.Tags, opts.Version))
	if opts.Version != "" {
		out = append(out, checkCL9(cl, opts.Version))
	}
	return out
}

// checkCL2 : titre de niveau 1 et mentions de Keep a Changelog et de SemVer avant la première section.
func checkCL2(cl clDoc, rules FileRules) FileCheck {
	c := newCheck(CriterionCL2)
	switch h := normalizeHeading(cl.h1); {
	case !cl.hasH1:
		c.add(FileNoH1)
	case h == "changelog" || h == "change log":
	case h == "journal des modifications" && rules.CategoriesFR:
		c.add(FileFrenchLabel)
	default:
		c.add(FileH1Title)
	}
	if !containsPhrase(cl.preamble, "keep a changelog") && !strings.Contains(cl.rawPre, "keepachangelog.com") {
		c.add(FileNoKeepAChangelog)
	}
	semverMentioned := strings.Contains(cl.rawPre, "semver.org")
	for _, p := range []string{"semver", "semantic versioning", "versionnage semantique", "versionnement semantique",
		"gestion semantique de version", "gestion semantique des versions"} {
		semverMentioned = semverMentioned || containsPhrase(cl.preamble, p)
	}
	if !semverMentioned {
		c.add(FileNoSemVer)
	}
	return c.done()
}

// checkCL3 : la première section est [Unreleased], unique, entre crochets.
func checkCL3(cl clDoc) FileCheck {
	c := newCheck(CriterionCL3)
	n := 0
	for i, s := range cl.sections {
		if !s.unreleased {
			continue
		}
		n++
		if i > 0 && n == 1 {
			c.add(FileUnreleasedMisplaced)
		}
		if !s.bracketed {
			c.add(FileUnreleasedNoBrackets)
		}
		if s.french {
			c.add(FileFrenchLabel)
		}
	}
	switch {
	case n == 0:
		c.add(FileUnreleasedAbsent)
	case n > 1:
		c.add(FileUnreleasedDuplicate)
	}
	return c.done()
}

// checkCL4 : titres de version et dates ; les compteurs donnent le nombre de versions par code.
func checkCL4(cl clDoc) FileCheck {
	c := newCheck(CriterionCL4)
	for _, s := range cl.sections {
		if s.unreleased {
			continue
		}
		c.counts[CountVersions]++
		for _, code := range s.codes {
			c.addCount(code)
		}
	}
	return c.done()
}

// checkCL5 : précédence SemVer strictement décroissante, dates non croissantes (versions lisibles seulement).
func checkCL5(cl clDoc) FileCheck {
	c := newCheck(CriterionCL5)
	var prevV *semver.Version
	var prevD *time.Time
	for _, s := range cl.sections {
		if s.unreleased {
			continue
		}
		c.counts[CountVersions]++
		if s.hasVersion {
			if prevV != nil && prevV.Compare(s.version) <= 0 {
				c.addCount(FileVersionOrder)
			}
			v := s.version
			prevV = &v
		}
		if s.hasDate {
			if prevD != nil && s.date.After(*prevD) {
				c.addCount(FileDateOrder)
			}
			d := s.date
			prevD = &d
		}
	}
	return c.done()
}

// checkCL6 : catégories de Keep a Changelog, sans doublon, dans l'ordre (avertissement), aucune puce hors
// catégorie ; les compteurs donnent le nombre de sections par code.
func checkCL6(cl clDoc) FileCheck {
	c := newCheck(CriterionCL6)
	for _, s := range cl.sections {
		c.counts[CountSections]++
		seen := map[string]bool{}
		codes := map[string]bool{}
		last := -1
		for _, cat := range s.categories {
			if cat.canonical == "" {
				codes[FileUnknownCategory] = true
				continue
			}
			if cat.french {
				codes[FileFrenchLabel] = true
			}
			if seen[cat.canonical] {
				codes[FileDuplicateCategory] = true
			}
			seen[cat.canonical] = true
			pos := slices.Index(changelogCategories, cat.canonical)
			if pos < last {
				codes[FileCategoryOrder] = true
			}
			last = max(last, pos)
		}
		if s.outside > 0 {
			codes[FileBulletOutsideCategory] = true
		}
		for code := range codes {
			c.addCount(code)
		}
	}
	return c.done()
}

// checkCL7 : versions publiées sans entrée, catégories sans entrée, puces vides ou « TODO » ; [Unreleased]
// n'est pas évalué (vide entre deux versions, c'est attendu).
func checkCL7(cl clDoc) FileCheck {
	c := newCheck(CriterionCL7)
	for _, s := range cl.sections {
		if s.unreleased {
			continue
		}
		c.counts[CountVersions]++
		if s.bullets == 0 {
			c.addCount(FileEmptyVersion)
		}
		for _, cat := range s.categories {
			if cat.bullets == 0 {
				c.addCount(FileEmptyCategory)
			}
		}
		for range s.empty {
			c.addCount(FileEmptyBullet)
		}
	}
	return c.done()
}

// checkCL8 : chaque tag de version finale a sa section ; chaque section datée (non retirée) a son tag. Sur
// l'arbre d'un jalon, seuls les tags et les sections de précédence inférieure ou égale au jalon comptent.
func checkCL8(cl clDoc, tags []string, milestone string) FileCheck {
	c := newCheck(CriterionCL8)
	var limit *semver.Version
	if milestone != "" {
		if m := CheckTag(milestone); m.Designates {
			limit = &m.Version
		}
	}
	type tagged struct {
		v     semver.Version
		final bool
	}
	var tv []tagged
	for _, name := range tags {
		r := CheckTag(name)
		if !r.Designates || (limit != nil && r.Version.Compare(*limit) > 0) {
			continue
		}
		tv = append(tv, tagged{v: r.Version, final: len(r.Version.Pre) == 0})
	}
	c.counts[CountTags] = len(tv)
	hasSection := func(v semver.Version) bool {
		return slices.ContainsFunc(cl.sections, func(s clSection) bool {
			return s.hasVersion && s.version.Compare(v) == 0
		})
	}
	seen := map[string]bool{}
	for _, t := range tv {
		v := t.v
		v.Build = ""
		key := v.String()
		if t.final && !seen[key] && !hasSection(t.v) {
			c.addCount(FileTagWithoutSection)
		}
		seen[key] = true
	}
	for _, s := range cl.sections {
		if s.unreleased || !s.hasVersion || !s.hasDate || s.yanked || (limit != nil && s.version.Compare(*limit) > 0) {
			continue
		}
		if !slices.ContainsFunc(tv, func(t tagged) bool { return t.v.Compare(s.version) == 0 }) {
			c.addCount(FileSectionWithoutTag)
		}
	}
	return c.done()
}

// checkCL9 : dans le CHANGELOG du tag, la section de la version du jalon existe (titre lisible, crochets,
// « v » et date indifférents : leur forme relève de CL4).
func checkCL9(cl clDoc, milestone string) FileCheck {
	m := CheckTag(milestone)
	if !m.Designates {
		return unknownCheck(CriterionCL9, UnknownNoTag)
	}
	c := newCheck(CriterionCL9)
	if !slices.ContainsFunc(cl.sections, func(s clSection) bool {
		return s.hasVersion && s.version.Compare(m.Version) == 0
	}) {
		c.add(FileVersionMissing)
	}
	return c.done()
}
