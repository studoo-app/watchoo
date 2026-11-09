package conventions

import (
	"errors"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Codes BR1 du nom d'une branche (clés --json, catalogue i18n), en anglais. Ils forment un espace distinct
// des codes CC2 : « unknown_type » d'une branche n'est pas celui d'un commit.
const (
	// Bloquants : la branche n'est pas conforme.
	BranchUnknownType = "unknown_type" // segment avant « / » absent de commit.types (casse ignorée)
	BranchUppercase   = "uppercase"    // au moins une majuscule
	BranchUnderscore  = "underscore"   // au moins un « _ »
	BranchSpace       = "space"        // au moins un blanc
	BranchAccent      = "accent"       // au moins une lettre hors ASCII
	BranchNoSlash     = "no_slash"     // aucun « / » : pas de type
	BranchPattern     = "pattern"      // ne suit pas le motif (personnalisé, ou intégré sans autre motif nommé)

	// Avertissement : « à revoir », sans effet sur la conformité.
	BranchSingleSegment = "single_segment" // un seul segment après « / » (« feat/login »)
)

// AllBranchCodes liste les codes BR1 dans leur ordre d'affichage : les bloquants, puis l'avertissement.
var AllBranchCodes = []string{
	BranchUnknownType, BranchUppercase, BranchUnderscore, BranchSpace, BranchAccent, BranchNoSlash,
	BranchPattern, BranchSingleSegment,
}

// BranchSeverityOf renvoie la gravité d'un code BR1 ; 0 pour un code inconnu.
func BranchSeverityOf(code string) Severity {
	switch code {
	case BranchUnknownType, BranchUppercase, BranchUnderscore, BranchSpace, BranchAccent, BranchNoSlash,
		BranchPattern:
		return SeverityBlocking
	case BranchSingleSegment:
		return SeverityWarning
	}
	return 0
}

// DefaultBranchExclude renvoie les exclusions de la charte (conventions.branch.exclude).
func DefaultBranchExclude() []string {
	return []string{"gh-pages", "dependabot/*", "renovate/*"}
}

// ErrInvalidBranchPattern signale un conventions.branch.pattern qui n'est pas une RE2 valide.
var ErrInvalidBranchPattern = errors.New("conventions: invalid branch pattern")

// ErrInvalidBranchExclude signale une entrée de conventions.branch.exclude vide ou mal formée.
var ErrInvalidBranchExclude = errors.New("conventions: invalid branch exclusion")

// ValidBranchPattern vérifie conventions.branch.pattern : vide (règle intégrée) ou RE2 valide.
func ValidBranchPattern(p string) error {
	if p == "" {
		return nil
	}
	if _, err := regexp.Compile(p); err != nil {
		return ErrInvalidBranchPattern
	}
	return nil
}

// ValidBranchExclude vérifie une entrée de conventions.branch.exclude : un motif path.Match non vide.
func ValidBranchExclude(glob string) error {
	if strings.TrimSpace(glob) == "" {
		return ErrInvalidBranchExclude
	}
	if _, err := path.Match(glob, ""); err != nil {
		return ErrInvalidBranchExclude
	}
	return nil
}

// BranchRules sont les règles BR1, tirées de conventions.commit.types et conventions.branch.*.
type BranchRules struct {
	Types   []string // types admis devant « / » (règle intégrée) ; vide : aucun nom n'est conforme
	Pattern string   // RE2 qui remplace la règle intégrée ; vide : règle intégrée
	Exclude []string // motifs path.Match des branches hors évaluation
}

// BranchResult est l'analyse d'un nom de branche.
type BranchResult struct {
	Name  string
	Codes []string // codes BR1, dans l'ordre d'AllBranchCodes ; vide non nil si aucun
}

// Conform indique un nom sans écart bloquant (single_segment seul reste conforme, « à revoir »).
func (r BranchResult) Conform() bool {
	return !slices.ContainsFunc(r.Codes, func(c string) bool { return BranchSeverityOf(c) == SeverityBlocking })
}

// Review indique un nom conforme à revoir (avertissement single_segment).
func (r BranchResult) Review() bool {
	return r.Conform() && slices.Contains(r.Codes, BranchSingleSegment)
}

// BranchChecker analyse des noms de branche selon des règles fixées ; il est sûr en lecture concurrente.
type BranchChecker struct {
	rules   BranchRules
	builtin *regexp.Regexp // règle intégrée ; nil si aucun type
	custom  *regexp.Regexp // conventions.branch.pattern ; nil si vide
}

// builtinSegments est la partie après « / » de la règle intégrée.
const builtinSegments = `[a-z0-9]+(-[a-z0-9]+)+`

// NewBranchChecker prépare l'analyse ; il renvoie ErrInvalidBranchPattern ou ErrInvalidBranchExclude si
// les règles n'ont pas été validées.
func NewBranchChecker(r BranchRules) (*BranchChecker, error) {
	c := &BranchChecker{rules: r}
	if r.Pattern != "" {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, ErrInvalidBranchPattern
		}
		c.custom = re
	}
	for _, x := range r.Exclude {
		if err := ValidBranchExclude(x); err != nil {
			return nil, err
		}
	}
	var alts []string
	for _, t := range r.Types {
		if t != "" {
			alts = append(alts, regexp.QuoteMeta(t))
		}
	}
	if len(alts) > 0 {
		c.builtin = regexp.MustCompile(`^(` + strings.Join(alts, "|") + `)/` + builtinSegments + `$`)
	}
	return c, nil
}

// Excluded indique une branche hors évaluation : la branche par défaut, ou un nom désigné par un motif de
// conventions.branch.exclude. Un motif désigne le nom lui-même ou l'un de ses préfixes de chemin :
// « dependabot/* » exclut aussi « dependabot/npm_and_yarn/lodash-4.17.21 », que path.Match seul ne
// reconnaîtrait pas (« * » ne traverse pas « / »).
func (c *BranchChecker) Excluded(name, defaultBranch string) bool {
	if name == "" || name == defaultBranch {
		return true
	}
	for _, x := range c.rules.Exclude {
		for p := name; p != ""; {
			if ok, _ := path.Match(x, p); ok {
				return true
			}
			i := strings.LastIndexByte(p, '/')
			if i < 0 {
				break
			}
			p = p[:i]
		}
	}
	return false
}

// Check analyse un nom de branche (sans tenir compte des exclusions).
//
// Motif personnalisé : conforme s'il correspond, sinon le seul code pattern. Règle intégrée
// ^(<types>)/[a-z0-9]+(-[a-z0-9]+)+$ : chaque défaut reconnaissable reçoit son code (type inconnu,
// majuscule, « _ », blanc, accent, « / » absent) ; un seul segment après « / » est un avertissement ; un
// nom qui ne correspond pas sans autre motif nommé (« feat/a.b », « feat/-x ») reçoit pattern.
func (c *BranchChecker) Check(name string) BranchResult {
	res := BranchResult{Name: name, Codes: []string{}}
	if c.custom != nil {
		if !c.custom.MatchString(name) {
			res.Codes = append(res.Codes, BranchPattern)
		}
		return res
	}
	codes := map[string]bool{}
	for _, r := range name {
		switch {
		case unicode.IsSpace(r):
			codes[BranchSpace] = true
		case r == '_':
			codes[BranchUnderscore] = true
		case r > unicode.MaxASCII && unicode.IsLetter(r):
			codes[BranchAccent] = true
			if unicode.IsUpper(r) {
				codes[BranchUppercase] = true
			}
		case r >= 'A' && r <= 'Z':
			codes[BranchUppercase] = true
		}
	}
	typ, rest, ok := strings.Cut(name, "/")
	if !ok {
		codes[BranchNoSlash] = true
	} else {
		if !slices.ContainsFunc(c.rules.Types, func(t string) bool { return strings.EqualFold(t, typ) }) {
			codes[BranchUnknownType] = true
		}
		if rest != "" && !strings.ContainsAny(rest, "-/") {
			codes[BranchSingleSegment] = true
		}
	}
	if len(codes) == 0 && (c.builtin == nil || !c.builtin.MatchString(name)) {
		codes[BranchPattern] = true
	}
	for _, code := range AllBranchCodes {
		if codes[code] {
			res.Codes = append(res.Codes, code)
		}
	}
	return res
}

// CheckAll analyse des noms de branche venus de plusieurs sources (clone, head_branch des PR) : chaque
// nom est compté une fois, les exclusions et la branche par défaut sont retirées, le résultat est trié par
// nom.
func (c *BranchChecker) CheckAll(names []string, defaultBranch string) []BranchResult {
	seen := map[string]bool{}
	out := []BranchResult{}
	for _, n := range names {
		if seen[n] || c.Excluded(n, defaultBranch) {
			continue
		}
		seen[n] = true
		out = append(out, c.Check(n))
	}
	slices.SortFunc(out, func(a, b BranchResult) int { return strings.Compare(a.Name, b.Name) })
	return out
}
