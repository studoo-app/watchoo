package conventions

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/semver"
)

// Codes SV1 du nom d'un tag (clés --json, catalogue i18n), en anglais.
const (
	// Bloquants : le tag n'est pas conforme.
	TagNoV              = "no_v"              // pas de préfixe « v » (« 1.0.0 »)
	TagUpperV           = "upper_v"           // préfixe « V » majuscule
	TagMissingComponent = "missing_component" // moins de trois composants (« v1.0 », « v1 »)
	TagLeadingZero      = "leading_zero"      // composant numérique avec un zéro en tête (« v01.0.0 », « v1.0.0-rc.01 »)
	TagEmptyPrerelease  = "empty_prerelease"  // pré-version vide ou identifiant vide (« v1.0.0- », « v1.0.0-rc..1 »)
	TagInvalid          = "invalid"           // tout autre nom (« release-1 », « v1.0.0.1 », « v1.a.0 »)

	// Avertissement : signalé, sans effet sur la conformité.
	TagBuildMetadata = "build_metadata" // métadonnées de build « +… » (« v1.0.0+build.5 »)
)

// AllTagCodes liste les codes SV1 dans leur ordre d'affichage : les bloquants, puis l'avertissement.
var AllTagCodes = []string{
	TagNoV, TagUpperV, TagMissingComponent, TagLeadingZero, TagEmptyPrerelease, TagInvalid, TagBuildMetadata,
}

// TagSeverityOf renvoie la gravité d'un code SV1 ; 0 pour un code inconnu.
func TagSeverityOf(code string) Severity {
	switch code {
	case TagNoV, TagUpperV, TagMissingComponent, TagLeadingZero, TagEmptyPrerelease, TagInvalid:
		return SeverityBlocking
	case TagBuildMetadata:
		return SeverityWarning
	}
	return 0
}

// TagResult est l'analyse du nom d'un tag.
type TagResult struct {
	Name  string
	Codes []string // codes SV1, dans l'ordre d'AllTagCodes ; vide non nil si aucun
	// Version est la version que le nom désigne : exacte pour un nom conforme, lue avec tolérance (« v »
	// absent ou majuscule, composants manquants complétés par 0, zéros en tête ignorés) sinon. Elle sert à
	// reconnaître un jalon posé sous un nom non conforme (SV2) et la pré-version d'une release (SV6).
	Version semver.Version
	// Designates indique que Version a pu être lue (faux pour invalid et empty_prerelease).
	Designates bool
}

// Conform indique un nom sans écart bloquant (build_metadata seul reste conforme).
func (r TagResult) Conform() bool {
	return !slices.ContainsFunc(r.Codes, func(c string) bool { return TagSeverityOf(c) == SeverityBlocking })
}

// Final indique un tag conforme de version finale : ni pré-version ni métadonnées de build. Seuls ces tags
// bornent les intervalles de CC1 et valent jalon.
func (r TagResult) Final() bool { return r.Conform() && r.Version.IsFinal() }

// CheckTag analyse le nom d'un tag (SV1) : « v » minuscule puis une version SemVer 2.0.0.
func CheckTag(name string) TagResult {
	res := TagResult{Name: name, Codes: []string{}}
	codes, v, ok := classifyTag(name)
	if ok {
		res.Version, res.Designates = v, true
	}
	for _, c := range AllTagCodes {
		if codes[c] {
			res.Codes = append(res.Codes, c)
		}
	}
	// Garde-fou : un nom conforme est une version que le comparateur commun accepte telle quelle.
	if res.Conform() {
		exact, valid := semver.ParseVersion(name)
		if !valid {
			res.Codes, res.Designates = []string{TagInvalid}, false
			res.Version = semver.Version{}
		} else {
			res.Version = exact
		}
	}
	return res
}

// classifyTag nomme les défauts du nom et lit, si possible, la version qu'il désigne.
func classifyTag(name string) (codes map[string]bool, v semver.Version, ok bool) {
	codes = map[string]bool{}
	invalid := func() (map[string]bool, semver.Version, bool) {
		return map[string]bool{TagInvalid: true}, semver.Version{}, false
	}
	rest := name
	switch {
	case strings.HasPrefix(name, "v"):
		rest = name[1:]
	case strings.HasPrefix(name, "V"):
		codes[TagUpperV] = true
		rest = name[1:]
	default:
		codes[TagNoV] = true
	}
	if rest == "" || rest[0] < '0' || rest[0] > '9' {
		return invalid()
	}
	if core, build, has := strings.Cut(rest, "+"); has {
		if !identifiersCharset(build) || slices.Contains(strings.Split(build, "."), "") {
			return invalid()
		}
		codes[TagBuildMetadata] = true
		v.Build = build
		rest = core
	}
	core, pre, hasPre := strings.Cut(rest, "-")
	designates := true
	if hasPre {
		if !identifiersCharset(pre) {
			return invalid()
		}
		ids := strings.Split(pre, ".")
		if pre == "" || slices.Contains(ids, "") {
			codes[TagEmptyPrerelease] = true
			designates = false
		} else {
			for _, id := range ids {
				if isDigits(id) && len(id) > 1 && id[0] == '0' {
					codes[TagLeadingZero] = true
				}
			}
			v.Pre = ids
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) > 3 {
		return invalid()
	}
	nums := [3]*uint64{&v.Major, &v.Minor, &v.Patch}
	for i, p := range parts {
		if !isDigits(p) {
			return invalid()
		}
		if len(p) > 1 && p[0] == '0' {
			codes[TagLeadingZero] = true
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return invalid()
		}
		*nums[i] = n
	}
	if len(parts) < 3 {
		codes[TagMissingComponent] = true
	}
	if !designates {
		return codes, semver.Version{}, false
	}
	return codes, v, true
}

// isDigits indique une chaîne non vide de chiffres ASCII.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// identifiersCharset indique des identifiants SemVer en [0-9A-Za-z-] séparés par des points (les
// identifiants vides sont vérifiés à part).
func identifiersCharset(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}

// Jalons (conventions.tags.milestones) : « vX.Y.Z » ou « vX.Y.Z@AAAA-MM-JJ » pour une échéance.

// MilestoneDueLayout est la forme de l'échéance d'un jalon.
const MilestoneDueLayout = "2006-01-02"

// Erreurs de validation d'un jalon.
var (
	// ErrMilestoneVersion : la version n'est pas un nom de tag conforme de version finale.
	ErrMilestoneVersion = errors.New("conventions: milestone is not a final vX.Y.Z version")
	// ErrMilestoneDue : l'échéance n'est pas une date AAAA-MM-JJ valide.
	ErrMilestoneDue = errors.New("conventions: invalid milestone due date")
)

// DefaultMilestones renvoie les jalons de la charte, sans échéance.
func DefaultMilestones() []string {
	return []string{"v1.0.0", "v2.0.0", "v3.0.0"}
}

// Milestone est un jalon configuré.
type Milestone struct {
	Name    string         // nom de tag attendu (« v1.0.0 »)
	Version semver.Version // version du jalon
	Due     string         // échéance AAAA-MM-JJ ; vide si aucune
}

// ParseMilestone lit une entrée de conventions.tags.milestones. La version doit être un nom de tag
// conforme (SV1) de version finale, sans métadonnées de build ; l'échéance, une date AAAA-MM-JJ.
func ParseMilestone(s string) (Milestone, error) {
	name, due, hasDue := strings.Cut(strings.TrimSpace(s), "@")
	t := CheckTag(name)
	if len(t.Codes) > 0 || !t.Final() {
		return Milestone{}, ErrMilestoneVersion
	}
	m := Milestone{Name: name, Version: t.Version}
	if hasDue {
		d, err := time.Parse(MilestoneDueLayout, due)
		if err != nil || d.Format(MilestoneDueLayout) != due {
			return Milestone{}, ErrMilestoneDue
		}
		m.Due = due
	}
	return m, nil
}

// ParseMilestones lit toutes les entrées ; la première erreur arrête la lecture.
func ParseMilestones(entries []string) ([]Milestone, error) {
	out := make([]Milestone, 0, len(entries))
	for _, e := range entries {
		m, err := ParseMilestone(e)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// HasDue indique un jalon avec échéance.
func (m Milestone) HasDue() bool { return m.Due != "" }

// DueEnd renvoie la fin de l'échéance dans le fuseau loc (UTC si nil) : minuit au début du jour qui suit,
// borne exclue. ok est faux sans échéance.
func (m Milestone) DueEnd(loc *time.Location) (end time.Time, ok bool) {
	if m.Due == "" {
		return time.Time{}, false
	}
	if loc == nil {
		loc = time.UTC
	}
	d, err := time.ParseInLocation(MilestoneDueLayout, m.Due, loc)
	if err != nil {
		return time.Time{}, false
	}
	return d.AddDate(0, 0, 1), true
}

// FindMilestone renvoie le jalon de nom name (comparaison exacte).
func FindMilestone(ms []Milestone, name string) (Milestone, bool) {
	i := slices.IndexFunc(ms, func(m Milestone) bool { return m.Name == name })
	if i < 0 {
		return Milestone{}, false
	}
	return ms[i], true
}

// États SV2 d'un jalon (présence seule ; l'échéance comparée à la pose arrive avec la phase 8.4).
const (
	MilestonePresent           = "present"            // tag de nom exact
	MilestoneNonconformingName = "nonconforming_name" // tag qui désigne la version sous un nom non conforme
	MilestoneAbsent            = "absent"             // aucun tag ne la désigne (une pré-version ne compte pas)
)

// MilestoneResult est l'état SV2 d'un jalon dans un dépôt.
type MilestoneResult struct {
	Milestone
	Status string   // MilestonePresent, MilestoneNonconformingName ou MilestoneAbsent
	Tag    string   // tag du jalon : nom exact, ou premier nom non conforme (ordre alphabétique) ; vide si absent
	Others []string // noms non conformes qui désignent aussi le jalon, triés ; vide non nil si aucun
}

// CheckMilestones calcule SV2 pour chaque jalon, dans l'ordre donné, d'après les noms des tags du dépôt.
// Un tag désigne un jalon si sa version (lue avec tolérance) a les mêmes MAJOR, MINOR et PATCH, sans
// pré-version ; les métadonnées de build sont ignorées (« v1.0.0+b » désigne v1.0.0 sous un nom non
// conforme au jalon).
func CheckMilestones(ms []Milestone, tags []string) []MilestoneResult {
	checked := make([]TagResult, 0, len(tags))
	for _, t := range tags {
		checked = append(checked, CheckTag(t))
	}
	out := make([]MilestoneResult, 0, len(ms))
	for _, m := range ms {
		r := MilestoneResult{Milestone: m, Status: MilestoneAbsent, Others: []string{}}
		exact := false
		for _, t := range checked {
			switch {
			case t.Name == m.Name:
				exact = true
			case t.Designates && len(t.Version.Pre) == 0 && sameCore(t.Version, m.Version):
				r.Others = append(r.Others, t.Name)
			}
		}
		slices.Sort(r.Others)
		r.Others = slices.Compact(r.Others)
		switch {
		case exact:
			r.Status, r.Tag = MilestonePresent, m.Name
		case len(r.Others) > 0:
			r.Status, r.Tag = MilestoneNonconformingName, r.Others[0]
			r.Others = r.Others[1:]
		}
		out = append(out, r)
	}
	return out
}

// sameCore indique les mêmes MAJOR, MINOR et PATCH.
func sameCore(a, b semver.Version) bool {
	return a.Major == b.Major && a.Minor == b.Minor && a.Patch == b.Patch
}

// Codes SV6 (information seule, hors conformité ; clés --json), en anglais.
const (
	ReleaseMilestoneMissing = "milestone_without_release" // jalon présent sans release (hors brouillons)
	ReleaseTagMissing       = "release_without_tag"       // release dont le tag est absent du dépôt
	ReleasePrerelease       = "prerelease_mismatch"       // drapeau prerelease contraire à la pré-version du tag
)

// AllReleaseCodes liste les codes SV6 dans leur ordre d'affichage.
var AllReleaseCodes = []string{ReleaseMilestoneMissing, ReleaseTagMissing, ReleasePrerelease}

// ReleaseNote est une information SV6 sur un tag.
type ReleaseNote struct {
	Code string
	Tag  string // tag concerné (jalon, ou tag de la release)
}

// CheckReleases calcule SV6 : jalons présents (exact ou sous un nom non conforme) sans release, releases
// dont le tag est absent, drapeau prerelease contraire à la pré-version SemVer du tag (tag lu avec
// tolérance ; un tag illisible n'est pas jugé). Les brouillons sont ignorés. Ordre : celui d'AllReleaseCodes,
// puis par tag.
func CheckReleases(ms []MilestoneResult, tags []string, releases []model.Release) []ReleaseNote {
	out := []ReleaseNote{}
	released := map[string]bool{}
	var published []model.Release
	for _, r := range releases {
		if r.Draft {
			continue
		}
		released[r.TagName] = true
		published = append(published, r)
	}
	var missing []string
	for _, m := range ms {
		if m.Status != MilestoneAbsent && !released[m.Tag] {
			missing = append(missing, m.Tag)
		}
	}
	known := map[string]bool{}
	for _, t := range tags {
		known[t] = true
	}
	var noTag, mismatch []string
	for _, r := range published {
		if !known[r.TagName] {
			noTag = append(noTag, r.TagName)
		}
		if t := CheckTag(r.TagName); t.Designates && r.Prerelease != (len(t.Version.Pre) > 0) {
			mismatch = append(mismatch, r.TagName)
		}
	}
	for _, g := range []struct {
		code string
		tags []string
	}{{ReleaseMilestoneMissing, missing}, {ReleaseTagMissing, noTag}, {ReleasePrerelease, mismatch}} {
		slices.Sort(g.tags)
		for _, t := range slices.Compact(g.tags) {
			out = append(out, ReleaseNote{Code: g.code, Tag: t})
		}
	}
	return out
}

// Intervalles entre tags finals (CC1 par intervalle, revue de jalon, tendance de CC3).

// FinalTags renvoie les tags conformes de version finale (SV1 sans écart, ni pré-version ni build),
// triés par précédence croissante.
func FinalTags(tags []model.Tag) []model.Tag {
	type ver struct {
		tag model.Tag
		v   semver.Version
	}
	var vs []ver
	for _, t := range tags {
		if r := CheckTag(t.Name); r.Final() && len(r.Codes) == 0 {
			vs = append(vs, ver{t, r.Version})
		}
	}
	slices.SortStableFunc(vs, func(a, b ver) int { return a.v.Compare(b.v) })
	out := make([]model.Tag, 0, len(vs))
	for _, x := range vs {
		out = append(out, x.tag)
	}
	return out
}

// TagInterval est l'intervalle From..To entre deux tags finals successifs : les commits atteignables
// depuis To et pas depuis From (git rev-list To ^From). From vide : depuis le début de l'historique.
type TagInterval struct {
	From model.Tag // tag final précédent ; Name vide pour le premier intervalle
	To   model.Tag
}

// TagIntervals renvoie les intervalles entre tags finals successifs, dans l'ordre de précédence.
func TagIntervals(tags []model.Tag) []TagInterval {
	finals := FinalTags(tags)
	out := make([]TagInterval, 0, len(finals))
	var prev model.Tag
	for _, t := range finals {
		out = append(out, TagInterval{From: prev, To: t})
		prev = t
	}
	return out
}

// IntervalTo renvoie l'intervalle qui se termine au tag final name ; ok est faux si name n'est pas un tag
// final du dépôt.
func IntervalTo(tags []model.Tag, name string) (TagInterval, bool) {
	for _, iv := range TagIntervals(tags) {
		if iv.To.Name == name {
			return iv, true
		}
	}
	return TagInterval{}, false
}

// PreviousFinal renvoie le plus grand tag final de précédence strictement inférieure à v : la borne basse
// de l'intervalle d'un jalon dont le tag est absent. ok est faux s'il n'y en a pas.
func PreviousFinal(tags []model.Tag, v semver.Version) (model.Tag, bool) {
	finals := FinalTags(tags)
	for i := len(finals) - 1; i >= 0; i-- {
		if CheckTag(finals[i].Name).Version.Compare(v) < 0 {
			return finals[i], true
		}
	}
	return model.Tag{}, false
}

// LastFinal renvoie le tag final de plus grande précédence (borne de la tendance de CC3) ; ok est faux
// sans tag final.
func LastFinal(tags []model.Tag) (model.Tag, bool) {
	finals := FinalTags(tags)
	if len(finals) == 0 {
		return model.Tag{}, false
	}
	return finals[len(finals)-1], true
}
