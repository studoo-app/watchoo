package conventions

// Versions (phase 8.4) : formes des pré-versions (SV4), saut de version cohérent avec les commits (SV5),
// précédence et historique (SV7). Fonctions pures : l'ascendance des commits est lue par l'appelant dans
// le clone (AncestryFunc), les commits d'un intervalle aussi ; rien n'est stocké, tout est calculé à la
// lecture. Indicateurs de dépôt : rien n'est attribué à une personne.

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/aisignature"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/semver"
)

// SV4 : pré-versions.

// DefaultPrereleaseForms renvoie les formes de pré-version de la charte (conventions.tags.prerelease_forms).
func DefaultPrereleaseForms() []string {
	return []string{"rc", "beta", "alpha"}
}

// MaxPrereleaseFormLength plafonne la longueur d'une forme de pré-version.
const MaxPrereleaseFormLength = 32

// ValidPrereleaseForm indique une forme de pré-version admise dans les réglages : un mot de 1 à 32 lettres
// ASCII minuscules.
func ValidPrereleaseForm(s string) bool {
	if s == "" || len(s) > MaxPrereleaseFormLength {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return r < 'a' || r > 'z' })
}

// Codes SV4 (avertissements seulement, jamais des écarts ; clés --json), en anglais.
const (
	PrereleaseFreeForm   = "free_form"   // forme libre : ni « -<forme>.N » ni une forme configurée (« -rc1 », « -RC.1 », « -pre »)
	PrereleaseAfterFinal = "after_final" // pré-version posée après la version finale correspondante
	PrereleaseOrder      = "order"       // la précédence de deux pré-versions d'une même version contredit l'ascendance
)

// AllPrereleaseCodes liste les codes SV4 dans leur ordre d'affichage.
var AllPrereleaseCodes = []string{PrereleaseFreeForm, PrereleaseAfterFinal, PrereleaseOrder}

// PrereleaseFormConform indique une pré-version de forme conforme : exactement « <forme>.N », la forme dans
// forms (vide : DefaultPrereleaseForms), N entier sans zéro en tête.
func PrereleaseFormConform(v semver.Version, forms []string) bool {
	if len(forms) == 0 {
		forms = DefaultPrereleaseForms()
	}
	if len(v.Pre) != 2 || !slices.Contains(forms, v.Pre[0]) {
		return false
	}
	n := v.Pre[1]
	return isDigits(n) && (len(n) == 1 || n[0] != '0')
}

// VersionNote est un constat sur un tag (SV4, SV7) : Other est l'autre tag de la paire, vide si aucun.
type VersionNote struct {
	Code  string
	Tag   string
	Other string
}

// PrereleaseReport est SV4 d'un dépôt.
type PrereleaseReport struct {
	// Known indique un calcul fait (CheckPrereleases) ; faux : Reason vaut UnknownFiltered (rapport
	// restreint à des contributeurs).
	Known   bool
	Reason  string
	Checked int           // pré-versions de nom conforme (SV1)
	Conform int           // dont la forme est conforme
	Notes   []VersionNote // dans l'ordre d'AllPrereleaseCodes, puis par tag ; vide non nil
}

// versionTag est un tag conforme (SV1) et sa version.
type versionTag struct {
	tag model.Tag
	v   semver.Version
}

// conformVersions renvoie les tags de nom conforme (SV1) avec leur version, triés par précédence puis nom.
func conformVersions(tags []model.Tag) []versionTag {
	var out []versionTag
	for _, t := range tags {
		if r := CheckTag(t.Name); r.Conform() {
			out = append(out, versionTag{t, r.Version})
		}
	}
	slices.SortStableFunc(out, func(a, b versionTag) int {
		return cmp.Or(a.v.Compare(b.v), strings.Compare(a.tag.Name, b.tag.Name))
	})
	return out
}

// coreKey identifie MAJOR.MINOR.PATCH d'une version.
func coreKey(v semver.Version) [3]uint64 { return [3]uint64{v.Major, v.Minor, v.Patch} }

// CheckPrereleases calcule SV4 sur les tags du dépôt, pré-versions de nom conforme (SV1) seulement :
//   - forme libre (free_form) : pré-version qui n'est pas « <forme>.N » avec une forme de forms ;
//   - posée après la version finale (after_final) : la version finale conforme de même MAJOR.MINOR.PATCH
//     est un ancêtre strict de la pré-version dans le clone ; si l'ascendance ne tranche pas (même commit,
//     commits sans lien, clone absent ou illisible), la pré-version a été observée pour la première fois
//     après la version finale (firstSeen, par nom ; sans observation des deux, rien n'est signalé) ;
//   - ordre (order) : de deux pré-versions successives (précédence) d'une même version, la plus grande
//     est posée sur un ancêtre strict de la plus petite (clone seulement).
//
// Tous sont des avertissements. anc nil : sans clone.
func CheckPrereleases(tags []model.Tag, forms []string, firstSeen map[string]time.Time, anc AncestryFunc) PrereleaseReport {
	rep := PrereleaseReport{Known: true, Notes: []VersionNote{}}
	finals := map[[3]uint64]versionTag{}
	pres := map[[3]uint64][]versionTag{}
	var keys [][3]uint64
	for _, vt := range conformVersions(tags) {
		k := coreKey(vt.v)
		if len(vt.v.Pre) == 0 {
			if vt.v.Build == "" {
				finals[k] = vt
			}
			continue
		}
		if _, seen := pres[k]; !seen {
			keys = append(keys, k)
		}
		pres[k] = append(pres[k], vt)
	}
	var free, after, order []VersionNote
	for _, k := range keys {
		group := pres[k] // par précédence croissante
		for i, p := range group {
			rep.Checked++
			if PrereleaseFormConform(p.v, forms) {
				rep.Conform++
			} else {
				free = append(free, VersionNote{Code: PrereleaseFreeForm, Tag: p.tag.Name})
			}
			if f, ok := finals[k]; ok && prereleaseAfter(p.tag, f.tag, firstSeen, anc) {
				after = append(after, VersionNote{Code: PrereleaseAfterFinal, Tag: p.tag.Name, Other: f.tag.Name})
			}
			if i > 0 {
				lo := group[i-1]
				if lo.tag.SHA != p.tag.SHA && ancestry(anc, p.tag.SHA, lo.tag.SHA) == AncestryYes {
					order = append(order, VersionNote{Code: PrereleaseOrder, Tag: p.tag.Name, Other: lo.tag.Name})
				}
			}
		}
	}
	for _, g := range [][]VersionNote{free, after, order} {
		slices.SortStableFunc(g, func(a, b VersionNote) int {
			return cmp.Or(strings.Compare(a.Tag, b.Tag), strings.Compare(a.Other, b.Other))
		})
		rep.Notes = append(rep.Notes, g...)
	}
	return rep
}

// prereleaseAfter indique une pré-version pre posée après la version finale final : par l'ascendance du
// clone si elle tranche, sinon par la première observation.
func prereleaseAfter(pre, final model.Tag, firstSeen map[string]time.Time, anc AncestryFunc) bool {
	if pre.SHA != final.SHA {
		switch ancestry(anc, final.SHA, pre.SHA) {
		case AncestryYes:
			return true
		case AncestryNo:
			if ancestry(anc, pre.SHA, final.SHA) == AncestryYes {
				return false
			}
		}
	}
	fp, okp := firstSeen[pre.Name]
	ff, okf := firstSeen[final.Name]
	return okp && okf && !fp.IsZero() && !ff.IsZero() && fp.After(ff)
}

// SV7 : précédence et historique.

// Codes SV7 (clés --json), en anglais.
const (
	// PrecedenceAncestry (écart, avertissement de cohérence) : de deux versions finales successives, la
	// plus grande est posée sur un ancêtre strict de la plus petite.
	PrecedenceAncestry = "precedence_ancestry"
	// PrecedenceSkip (information) : saut d'incrément entre deux versions finales successives (v1.4.0 →
	// v3.0.0 sans v2.0.0, v1.0.0 → v1.0.2).
	PrecedenceSkip = "increment_skip"
)

// AllPrecedenceCodes liste les codes SV7 dans leur ordre d'affichage.
var AllPrecedenceCodes = []string{PrecedenceAncestry, PrecedenceSkip}

// PrecedenceReport est SV7 d'un dépôt.
type PrecedenceReport struct {
	// Known indique que l'ascendance a été lue (clone) ; faux : seuls les sauts d'incrément sont calculés,
	// Reason vaut UnknownNoClone.
	Known   bool
	Reason  string
	Pairs   int           // paires de versions finales successives
	Unknown int           // paires dont l'ascendance n'a pas pu être lue (commit absent du clone)
	Notes   []VersionNote // dans l'ordre d'AllPrecedenceCodes, puis par précédence ; vide non nil
}

// Conform indique une précédence lue et cohérente avec l'historique (aucun precedence_ancestry, aucune
// paire illisible).
func (r PrecedenceReport) Conform() bool {
	return r.Known && r.Unknown == 0 && !slices.ContainsFunc(r.Notes, func(n VersionNote) bool {
		return n.Code == PrecedenceAncestry
	})
}

// CheckPrecedence calcule SV7 sur les tags finals conformes (FinalTags), paire par paire de versions
// successives par précédence : la plus grande posée sur un ancêtre strict de la plus petite (clone ; deux
// tags sur le même commit sont cohérents) ; saut d'incrément en information (sans clone aussi). Seules
// les paires successives sont comparées : une incohérence entre versions non successives, toutes deux sans
// lien avec la version qui les sépare, n'est pas vue. anc nil : sans clone.
func CheckPrecedence(tags []model.Tag, anc AncestryFunc) PrecedenceReport {
	rep := PrecedenceReport{Known: anc != nil, Notes: []VersionNote{}}
	if anc == nil {
		rep.Reason = UnknownNoClone
	}
	finals := FinalTags(tags)
	var bad, skips []VersionNote
	for i := 1; i < len(finals); i++ {
		lo, hi := finals[i-1], finals[i]
		rep.Pairs++
		if incrementSkipped(CheckTag(lo.Name).Version, CheckTag(hi.Name).Version) {
			skips = append(skips, VersionNote{Code: PrecedenceSkip, Tag: hi.Name, Other: lo.Name})
		}
		if anc == nil || lo.SHA == hi.SHA {
			continue
		}
		switch ancestry(anc, hi.SHA, lo.SHA) {
		case AncestryYes:
			bad = append(bad, VersionNote{Code: PrecedenceAncestry, Tag: hi.Name, Other: lo.Name})
		case AncestryUnknown:
			rep.Unknown++
		}
	}
	rep.Notes = append(append(rep.Notes, bad...), skips...)
	return rep
}

// incrementSkipped indique que hi ne suit pas lo d'un seul incrément : PATCH + 1, MINOR + 1 avec PATCH à
// 0, ou MAJOR + 1 avec MINOR et PATCH à 0.
func incrementSkipped(lo, hi semver.Version) bool {
	switch {
	case hi.Major == lo.Major && hi.Minor == lo.Minor:
		return hi.Patch != lo.Patch+1
	case hi.Major == lo.Major:
		return hi.Minor != lo.Minor+1 || hi.Patch != 0
	default:
		return hi.Major != lo.Major+1 || hi.Minor != 0 || hi.Patch != 0
	}
}

// SV5 : saut de version cohérent avec les commits déclarés.

// Sauts de version (clés --json), en anglais.
const (
	BumpPatch = "patch"
	BumpMinor = "minor"
	BumpMajor = "major"
)

// États SV5 d'une paire de tags (clés --json), en anglais.
const (
	BumpConsistent   = "consistent"   // saut au moins égal au saut attendu (le sur-versionnement n'est pas signalé)
	BumpUnder        = "under"        // saut inférieur au saut attendu (sous-versionnement)
	BumpUndetermined = "undetermined" // commits trop peu conformes, ou absents de la base
	BumpUnknown      = "unknown"      // base non lue (clone absent ou illisible) ou sans base
)

// AllBumpStatuses liste les états SV5 dans leur ordre d'affichage.
var AllBumpStatuses = []string{BumpConsistent, BumpUnder, BumpUndetermined, BumpUnknown}

// Raisons d'un SV5 indéterminé ou inconnu (clés --json), en anglais ; s'ajoutent à UnknownNoClone,
// UnknownCloneUnreadable et UnknownPartial.
const (
	UnknownNoBase     = "no_base"     // aucun tag final conforme inférieur n'est ancêtre du tag
	UnknownLowConform = "low_conform" // moins de BumpMinConformPercent % des commits évalués sont conformes
)

// BumpMinConformPercent est la part minimale de commits conformes pour juger un saut (seuil à valider sur
// des dépôts réels).
const BumpMinConformPercent = 80

// BumpOf renvoie le saut de from à to : major si MAJOR change, minor si MINOR change, patch sinon.
func BumpOf(from, to semver.Version) string {
	switch {
	case from.Major != to.Major:
		return BumpMajor
	case from.Minor != to.Minor:
		return BumpMinor
	}
	return BumpPatch
}

// ExpectedBump renvoie le saut attendu depuis la version base : une rupture demande MAJOR (MINOR avant
// 1.0.0, base en 0.y.z), une fonctionnalité au moins MINOR, sinon PATCH.
func ExpectedBump(breaking, feature bool, base semver.Version) string {
	switch {
	case breaking && base.Major > 0:
		return BumpMajor
	case breaking, feature:
		return BumpMinor
	}
	return BumpPatch
}

// bumpRank ordonne les sauts.
func bumpRank(b string) int {
	return slices.Index([]string{BumpPatch, BumpMinor, BumpMajor}, b)
}

// BumpLine est SV5 d'une paire de tags finals : de la base From au tag To.
type BumpLine struct {
	From     string // base : plus grand tag final conforme inférieur à To qui en est ancêtre ; vide si inconnue
	To       string
	Status   string // BumpConsistent, BumpUnder, BumpUndetermined ou BumpUnknown
	Reason   string // raison d'un état indéterminé ou inconnu ; vide sinon
	Actual   string // saut de From à To ; vide sans base
	Expected string // saut attendu d'après les commits ; vide sans base
	// Evaluated compte les commits de From..To hors fusions et bots ; Conform ceux qui sont conformes ;
	// Breaking et Features, parmi eux, les ruptures et les « feat » ; Missing les commits absents de la base.
	Evaluated, Conform, Breaking, Features, Missing int
}

// BumpReport est SV5 d'un dépôt.
type BumpReport struct {
	Known  bool       // faux sans clone : Reason vaut UnknownNoClone et Lines est vide
	Reason string     //
	Lines  []BumpLine // par précédence croissante de To, premier tag final exclu ; vide non nil
	Counts map[string]int
}

// BumpPair est une paire de SV5 : le tag To et sa base From.
type BumpPair struct {
	From   model.Tag // Name vide si la base est inconnue
	To     model.Tag
	Reason string // vide si From est trouvée ; UnknownCloneUnreadable ou UnknownNoBase sinon
}

// BumpBases trouve la base de chaque tag final conforme (sauf le plus petit) : le plus grand tag final
// conforme inférieur qui en est ancêtre (un tag sur le même commit l'est). La recherche descend par
// précédence et s'arrête à la première ascendance illisible (UnknownCloneUnreadable) ; aucun ancêtre :
// UnknownNoBase. anc nil : aucune paire (sans clone).
func BumpBases(tags []model.Tag, anc AncestryFunc) []BumpPair {
	out := []BumpPair{}
	if anc == nil {
		return out
	}
	finals := FinalTags(tags)
	for i := 1; i < len(finals); i++ {
		p := BumpPair{To: finals[i], Reason: UnknownNoBase}
	search:
		for j := i - 1; j >= 0; j-- {
			switch ancestry(anc, finals[j].SHA, finals[i].SHA) {
			case AncestryYes:
				p.From, p.Reason = finals[j], ""
				break search
			case AncestryUnknown:
				p.Reason = UnknownCloneUnreadable
				break search
			}
		}
		out = append(out, p)
	}
	return out
}

// bumpExcluded indique un commit hors de SV5 et de CL10 : fusion (nombre de parents, ou message de fusion
// par défaut s'il est inconnu) ou auteur bot, comme CC4.
func bumpExcluded(c Commit) bool {
	return isMerge(c) || c.Bot || aisignature.IsBot(c.AuthorName) || aisignature.IsBot(c.AuthorLogin)
}

// EvalBump calcule SV5 de la paire p (base trouvée) sur les commits de From..To (lus par l'appelant dans le
// clone puis en base ; missing compte ceux qui sont absents de la base). Les fusions et les bots sont
// écartés ; seuls les commits conformes au parseur strict déclarent un type ou une rupture. Indéterminé si
// des commits manquent en base, ou si moins de BumpMinConformPercent % des commits évalués sont conformes ;
// sinon sous-versionné si le saut réel est inférieur au saut attendu.
func EvalBump(p BumpPair, commits []Commit, missing int, rules Rules) BumpLine {
	line := BumpLine{From: p.From.Name, To: p.To.Name, Missing: missing}
	if p.Reason != "" || p.From.Name == "" {
		line.Status, line.Reason = BumpUnknown, cmp.Or(p.Reason, UnknownNoBase)
		return line
	}
	parser := NewParser(rules)
	seen := map[string]bool{}
	for _, c := range commits {
		if seen[c.SHA] || bumpExcluded(c) {
			continue
		}
		seen[c.SHA] = true
		line.Evaluated++
		res := parser.Parse(c.Message)
		if !res.Conform() {
			continue
		}
		line.Conform++
		if res.Breaking {
			line.Breaking++
		}
		if res.Type == "feat" {
			line.Features++
		}
	}
	from, to := CheckTag(p.From.Name).Version, CheckTag(p.To.Name).Version
	line.Actual = BumpOf(from, to)
	line.Expected = ExpectedBump(line.Breaking > 0, line.Features > 0, from)
	switch {
	case missing > 0:
		line.Status, line.Reason = BumpUndetermined, UnknownPartial
	case line.Conform*100 < line.Evaluated*BumpMinConformPercent:
		line.Status, line.Reason = BumpUndetermined, UnknownLowConform
	case bumpRank(line.Actual) < bumpRank(line.Expected):
		line.Status = BumpUnder
	default:
		line.Status = BumpConsistent
	}
	return line
}

// BumpRange lit les commits de From..To d'une paire : commits connus en base et nombre de commits absents
// de la base ; ok est faux si l'intervalle n'a pas pu être lu dans le clone.
type BumpRange func(p BumpPair) (commits []Commit, missing int, ok bool)

// CheckBumps calcule SV5 d'un dépôt : bases par BumpBases, commits de chaque intervalle par read. anc nil :
// inconnu (UnknownNoClone). Un intervalle illisible rend la paire inconnue (UnknownCloneUnreadable).
func CheckBumps(tags []model.Tag, anc AncestryFunc, read BumpRange, rules Rules) BumpReport {
	rep := BumpReport{Known: anc != nil, Lines: []BumpLine{}, Counts: map[string]int{}}
	for _, s := range AllBumpStatuses {
		rep.Counts[s] = 0
	}
	if anc == nil {
		rep.Reason = UnknownNoClone
		return rep
	}
	for _, p := range BumpBases(tags, anc) {
		var line BumpLine
		switch {
		case p.Reason != "":
			line = EvalBump(p, nil, 0, rules)
		default:
			commits, missing, ok := read(p)
			if !ok {
				p.Reason = UnknownCloneUnreadable
				line = EvalBump(p, nil, 0, rules)
			} else {
				line = EvalBump(p, commits, missing, rules)
			}
		}
		rep.Lines = append(rep.Lines, line)
		rep.Counts[line.Status]++
	}
	return rep
}
