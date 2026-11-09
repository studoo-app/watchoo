// Package conventions mesure le respect de la charte de conventions d'un projet (phase 8,
// documentation/fr/13-conventions.md) : des indicateurs de conformité déterministes, sans soupçon, séparés
// du diagnostic. Fonctions pures, sur le modèle d'internal/aisignature : rien n'y lit la base ni la forge,
// rien n'y journalise. Les messages lus restent en mémoire : un résultat ne porte que des codes stables, des
// compteurs et des SHA, jamais le texte d'un message.
//
// Phase 8.1 : parseur strict des messages de commit (CC2) et agrégation CC1 à CC4.
package conventions

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// RulesVersion est la version des règles du parseur et de l'agrégation. Elle change quand un même message
// peut recevoir d'autres codes ; les résultats calculés avant ne sont alors plus comparables.
const RulesVersion = 1

// Codes d'écart CC2 : identifiants stables (clés --json, catalogue i18n), en anglais.
const (
	// Bloquants : le commit n'est pas conforme.
	CodeEmpty          = "empty"           // message, sujet (hors suffixe « (#n) ») ou description vide
	CodeForbidden      = "forbidden"       // sujet de la liste interdite, ou préfixe wip, fixup!, squash!, amend!
	CodeGitHubDefault  = "github_default"  // message proposé par GitHub (« Update README.md », « Add files via upload »)
	CodeNoType         = "no_type"         // aucun en-tête « type: » reconnaissable
	CodeUnknownType    = "unknown_type"    // type absent de commit.types
	CodeTypeCase       = "type_case"       // type de commit.types écrit avec une autre casse
	CodeScope          = "scope"           // scope mal formé ou absent de commit.scopes
	CodeSeparator      = "separator"       // séparateur autre que « : » suivi d'une espace
	CodeTrailingPeriod = "trailing_period" // sujet terminé par un point
	CodeBodySeparator  = "body_separator"  // corps collé au sujet, sans ligne vide
	CodeLengthHard     = "length_hard"     // sujet plus long que commit.max_subject_hard

	// Avertissements : signalés, sans effet sur la conformité.
	CodeDescCapitalized = "desc_capitalized" // description commençant par une majuscule (sauf sigle)
	CodeLengthSoft      = "length_soft"      // sujet plus long que commit.max_subject_soft
	CodeFooterFormat    = "footer_format"    // pied de message mal formé (clé avec espaces, BREAKING CHANGE mal écrit)
	CodeGitRevert       = "git_revert"       // « Revert "…" » produit par Git : compté, sans être un écart
)

// AllCodes liste les codes CC2 dans leur ordre d'affichage : les bloquants, puis les avertissements.
var AllCodes = []string{
	CodeEmpty, CodeForbidden, CodeGitHubDefault, CodeNoType, CodeUnknownType, CodeTypeCase, CodeScope,
	CodeSeparator, CodeTrailingPeriod, CodeBodySeparator, CodeLengthHard,
	CodeDescCapitalized, CodeLengthSoft, CodeFooterFormat, CodeGitRevert,
}

// Severity est la gravité d'un code d'écart.
type Severity int

const (
	// SeverityBlocking rend le commit non conforme.
	SeverityBlocking Severity = iota + 1
	// SeverityWarning est signalé sans effet sur la conformité.
	SeverityWarning
)

// String renvoie le nom stable de la gravité (clés --json) : « blocking » ou « warning ».
func (s Severity) String() string {
	switch s {
	case SeverityBlocking:
		return "blocking"
	case SeverityWarning:
		return "warning"
	}
	return ""
}

// SeverityOf renvoie la gravité d'un code ; 0 pour un code inconnu.
func SeverityOf(code string) Severity {
	switch code {
	case CodeEmpty, CodeForbidden, CodeGitHubDefault, CodeNoType, CodeUnknownType, CodeTypeCase, CodeScope,
		CodeSeparator, CodeTrailingPeriod, CodeBodySeparator, CodeLengthHard:
		return SeverityBlocking
	case CodeDescCapitalized, CodeLengthSoft, CodeFooterFormat, CodeGitRevert:
		return SeverityWarning
	}
	return 0
}

// IsBlocking indique si le code rend le commit non conforme.
func IsBlocking(code string) bool { return SeverityOf(code) == SeverityBlocking }

// Valeurs par défaut de la charte (documentation/fr/13-conventions.md, sections 3 et 8).
const (
	DefaultMaxSubjectSoft = 50
	DefaultMaxSubjectHard = 72
)

// DefaultTypes renvoie les types de la charte, dans son ordre.
func DefaultTypes() []string {
	return []string{"feat", "fix", "docs", "style", "refactor", "perf", "test", "build", "ci", "chore", "revert"}
}

// DefaultForbidden renvoie les sujets interdits de la charte, normalisés (minuscules, espaces simples) :
// un sujet entier égal à l'un d'eux est un écart, pas un sujet qui le contient.
func DefaultForbidden() []string {
	return []string{
		"update", "updates", "maj", "mise à jour", "wip", "test", "tests", "modif", "modifs", "modification",
		"modifications", "fix", "fixes", "correction", "corrections", "commit", "save", "tmp", "temp", "misc",
		"dépôt 1", "depot 1", ".", "..", "...", "initial commit", "first commit", "premier commit",
	}
}

// Rules sont les règles du parseur, tirées des réglages conventions.commit.*.
type Rules struct {
	Types          []string // types admis, sensibles à la casse ; vide : aucun en-tête n'est conforme
	Scopes         []string // scopes admis, sensibles à la casse ; vide : tout scope bien formé
	MaxSubjectSoft int      // au-delà : avertissement length_soft ; 0 ou moins : pas de limite
	MaxSubjectHard int      // au-delà : écart length_hard ; 0 ou moins : pas de limite
	Forbidden      []string // sujets interdits (comparés après normalisation)
}

// DefaultRules renvoie les règles de la charte.
func DefaultRules() Rules {
	return Rules{
		Types:          DefaultTypes(),
		MaxSubjectSoft: DefaultMaxSubjectSoft,
		MaxSubjectHard: DefaultMaxSubjectHard,
		Forbidden:      DefaultForbidden(),
	}
}

// Result est l'analyse d'un message de commit. Il ne garde rien du texte du message.
type Result struct {
	Type     string   // type lu dans l'en-tête, tel qu'écrit ; vide si aucun
	Scope    string   // scope lu dans l'en-tête, sans parenthèses ; vide si aucun
	Breaking bool     // rupture déclarée par « ! » ou par un pied BREAKING CHANGE
	Codes    []string // codes CC2, dans l'ordre d'AllCodes, sans doublon ; vide non nil si aucun
}

// Conform indique un message sans écart bloquant.
func (r Result) Conform() bool {
	return !slices.ContainsFunc(r.Codes, IsBlocking)
}

var (
	// squashSuffix est le suffixe « (#n) » que GitHub ajoute au sujet d'une fusion en squash.
	squashSuffix = regexp.MustCompile(` ?\(#[0-9]+\)$`)
	// githubDefault reconnaît les sujets proposés par l'éditeur web de GitHub ; un renommage y est écrit
	// « Rename a.md to b.md ».
	githubDefault = regexp.MustCompile(`^(?:(?:Update|Create|Delete|Rename) \S+|Rename \S+ to \S+|Add files via upload)$`)
	// gitRevert reconnaît le sujet produit par « git revert » (« Reapply » pour un revert de revert).
	gitRevert = regexp.MustCompile(`^(?:Revert|Reapply) ".*"$`)
	// defaultMerge reconnaît les messages de fusion par défaut de Git et de GitHub.
	defaultMerge = regexp.MustCompile(`^(?:Merge pull request #[0-9]+ |Merge branch '|Merge remote-tracking branch ')`)
	// wipPrefix reconnaît un sujet marqué « travail en cours » (« WIP: », « [wip] », « wip »).
	wipPrefix = regexp.MustCompile(`(?i)^\[?wip(?:$|[^\p{L}\p{N}])`)
	// looseHeader reconnaît un en-tête « mot(scope)!: » mal écrit, pour nommer l'écart.
	looseHeader = regexp.MustCompile(`^([A-Za-z]+)(\([^()]*\))?(!)?(\s*):(\s*)(.*)$`)
	// scopeFormat est la forme d'un scope admis par le parseur strict.
	scopeFormat = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*$`)
	// footerLine est un pied de message Conventional Commits : « Clé: valeur », « Clé #valeur », la clé
	// sans espace (tirets), sauf BREAKING CHANGE.
	footerLine = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z0-9-]*|BREAKING CHANGE)(?:: | #)\S`)
	// breakingFooter est le pied de rupture exact.
	breakingFooter = regexp.MustCompile(`^BREAKING[ -]CHANGE: \S`)
	// breakingLike est une tentative de pied de rupture, quelle que soit son écriture.
	breakingLike = regexp.MustCompile(`(?i)^breaking[ _-]?changes?\s*:`)
	// badFooter est un pied mal formé : clé de deux ou trois mots, espace avant les deux-points, ou rien
	// entre les deux-points et la valeur.
	badFooter = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z0-9-]*(?: [A-Za-z][A-Za-z0-9-]*){1,2}\s*:\s*\S|[A-Za-z][A-Za-z0-9-]*\s+:\s*\S|[A-Za-z][A-Za-z0-9-]*:[^\s/])`)
)

// typeFormat est la forme d'un type admis dans commit.types : des lettres minuscules.
var typeFormat = regexp.MustCompile(`^[a-z]+$`)

// ValidType indique si t peut figurer dans conventions.commit.types : des lettres minuscules seulement,
// la forme que l'analyse d'un en-tête mal écrit sait reconnaître.
func ValidType(t string) bool { return typeFormat.MatchString(t) }

// ValidScope indique si s peut figurer dans conventions.commit.scopes : la forme de scope admise par le
// parseur strict ([a-z0-9][a-z0-9._/-]*).
func ValidScope(s string) bool { return scopeFormat.MatchString(s) }

// Parser analyse les messages de commit selon des règles fixées ; il est sûr en lecture concurrente.
type Parser struct {
	rules     Rules
	strict    *regexp.Regexp // nil si aucun type n'est admis
	forbidden map[string]bool
}

// NewParser prépare un parseur pour les règles r.
func NewParser(r Rules) *Parser {
	p := &Parser{rules: r, forbidden: map[string]bool{}}
	var alts []string
	for _, t := range r.Types {
		if t != "" {
			alts = append(alts, regexp.QuoteMeta(t))
		}
	}
	if len(alts) > 0 {
		p.strict = regexp.MustCompile(`^(` + strings.Join(alts, "|") + `)(\([a-z0-9][a-z0-9._/-]*\))?(!)?: (\S.*)$`)
	}
	for _, f := range r.Forbidden {
		if n := normalize(f); n != "" {
			p.forbidden[n] = true
		}
	}
	return p
}

// ParseCommit analyse un message avec les règles r ; pour de nombreux messages, NewParser évite de
// recompiler les règles à chaque appel.
func ParseCommit(message string, r Rules) Result {
	return NewParser(r).Parse(message)
}

// Subject renvoie la première ligne non vide du message, sans espaces autour ; le suffixe « (#n) » est
// gardé. Vide si le message ne contient que des blancs.
func Subject(message string) string {
	s, _, _ := splitMessage(message)
	return s
}

// StripSquashSuffix retire du sujet le suffixe « (#n) » d'une fusion en squash GitHub.
func StripSquashSuffix(subject string) string {
	return strings.TrimSpace(squashSuffix.ReplaceAllString(subject, ""))
}

// IsSquashSubject indique si le sujet du message se termine par « (#n) ». Un commit de squash est
// identifié par ce suffixe ET un committer GitHub (aisignature.IsGitHubCommitter) : le suffixe seul peut
// avoir été écrit à la main.
func IsSquashSubject(message string) bool {
	return squashSuffix.MatchString(Subject(message))
}

// IsDefaultMergeMessage indique un message de fusion par défaut de Git ou de GitHub (« Merge pull request
// #n from … », « Merge branch '…' », « Merge remote-tracking branch '…' ») : repli de CC4 quand le nombre
// de parents du commit est inconnu.
func IsDefaultMergeMessage(message string) bool {
	return defaultMerge.MatchString(Subject(message))
}

// IsGitRevert indique un sujet produit par « git revert » (« Revert "…" », « Reapply "…" »), suffixe
// « (#n) » compris.
func IsGitRevert(message string) bool {
	return gitRevert.MatchString(StripSquashSuffix(Subject(message)))
}

// splitMessage renvoie le sujet (première ligne non vide, sans espaces autour) et les lignes qui le
// suivent (sans retour chariot final) ; ok est faux si le message est vide.
func splitMessage(message string) (subject string, rest []string, ok bool) {
	lines := strings.Split(message, "\n")
	for i, l := range lines {
		if t := strings.TrimSpace(l); t != "" {
			rest = lines[i+1:]
			for j := range rest {
				rest[j] = strings.TrimRight(rest[j], "\r")
			}
			return t, rest, true
		}
	}
	return "", nil, false
}

// normalize ramène un sujet à sa forme de comparaison avec la liste interdite : minuscules, espaces
// simples, sans point final (sauf pour un sujet fait de points).
func normalize(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	if t := strings.TrimRight(s, "."); t != "" {
		s = strings.TrimSpace(t)
	}
	return s
}

// Parse analyse un message de commit.
//
// Ordre : un message vide, interdit ou proposé par GitHub reçoit ce seul code (le reste n'aurait pas de
// sens) ; un « Revert "…" » de Git reçoit git_revert et la vérification du corps, son sujet étant écrit par
// Git ; sinon l'en-tête, la ponctuation, la longueur, puis le corps et les pieds sont vérifiés.
func (p *Parser) Parse(message string) Result {
	res := Result{Codes: []string{}}
	codes := map[string]bool{}
	subject, rest, ok := splitMessage(message)
	s := StripSquashSuffix(subject)
	switch {
	case !ok || s == "":
		codes[CodeEmpty] = true
	case p.isForbidden(s):
		codes[CodeForbidden] = true
	case githubDefault.MatchString(s):
		codes[CodeGitHubDefault] = true
	case gitRevert.MatchString(s):
		codes[CodeGitRevert] = true
		res.Breaking = checkBody(rest, codes)
	default:
		desc := p.checkHeader(s, &res, codes)
		if strings.HasSuffix(s, ".") {
			codes[CodeTrailingPeriod] = true
		}
		if desc != "" && capitalized(desc) {
			codes[CodeDescCapitalized] = true
		}
		n := utf8.RuneCountInString(s)
		switch {
		case p.rules.MaxSubjectHard > 0 && n > p.rules.MaxSubjectHard:
			codes[CodeLengthHard] = true
		case p.rules.MaxSubjectSoft > 0 && n > p.rules.MaxSubjectSoft:
			codes[CodeLengthSoft] = true
		}
		if checkBody(rest, codes) {
			res.Breaking = true
		}
	}
	for _, c := range AllCodes {
		if codes[c] {
			res.Codes = append(res.Codes, c)
		}
	}
	return res
}

// isForbidden indique un sujet interdit : sujet entier de la liste, ou préfixe de travail en cours ou de
// commit de correction à replier (fixup!, squash!, amend!).
func (p *Parser) isForbidden(s string) bool {
	if p.forbidden[normalize(s)] || wipPrefix.MatchString(s) {
		return true
	}
	for _, prefix := range []string{"fixup!", "squash!", "amend!"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// checkHeader vérifie l'en-tête « type(scope)!: description » du sujet s (sans suffixe de squash),
// remplit le type, le scope et la rupture de res, ajoute les écarts à codes et renvoie la description
// (vide si elle n'a pas pu être lue).
func (p *Parser) checkHeader(s string, res *Result, codes map[string]bool) string {
	if p.strict != nil {
		if m := p.strict.FindStringSubmatch(s); m != nil {
			res.Type, res.Scope, res.Breaking = m[1], strings.Trim(m[2], "()"), m[3] != ""
			if m[2] != "" && !p.scopeAllowed(res.Scope) {
				codes[CodeScope] = true
			}
			return m[4]
		}
	}
	if m := looseHeader.FindStringSubmatch(s); m != nil {
		res.Type, res.Breaking = m[1], m[3] != ""
		p.checkType(m[1], codes)
		if m[2] != "" {
			res.Scope = strings.TrimSuffix(strings.TrimPrefix(m[2], "("), ")")
			if !scopeFormat.MatchString(res.Scope) || !p.scopeAllowed(res.Scope) {
				codes[CodeScope] = true
			}
		}
		if m[6] == "" {
			codes[CodeEmpty] = true
			return ""
		}
		if m[4] != "" || m[5] != " " {
			codes[CodeSeparator] = true
		}
		return m[6]
	}
	// Sans deux-points : un premier mot qui est un type admis, suivi d'un scope, d'un « ! », d'une espace
	// ou d'un tiret, est un en-tête au mauvais séparateur (« feat add login », « feat - add login »). Un
	// type à la mauvaise casse n'est reconnu que suivi d'un scope, d'un « ! » ou d'un tiret : « Fix typo »
	// est une phrase sans type, pas un type mal écrit.
	word := leadingWord(s)
	next := strings.TrimPrefix(s, word)
	switch {
	case word != "" && slices.Contains(p.rules.Types, word) &&
		(next == "" || strings.ContainsAny(next[:1], "(! -")):
		res.Type = word
		codes[CodeSeparator] = true
	case word != "" && p.typeFold(word) &&
		(strings.HasPrefix(next, "(") || strings.HasPrefix(next, "!") || strings.HasPrefix(next, " - ")):
		res.Type = word
		codes[CodeTypeCase] = true
		codes[CodeSeparator] = true
	default:
		codes[CodeNoType] = true
	}
	return ""
}

// checkType ajoute unknown_type ou type_case si t n'est pas un type admis tel qu'écrit.
func (p *Parser) checkType(t string, codes map[string]bool) {
	switch {
	case slices.Contains(p.rules.Types, t):
	case p.typeFold(t):
		codes[CodeTypeCase] = true
	default:
		codes[CodeUnknownType] = true
	}
}

// typeFold indique si t est un type admis, casse ignorée.
func (p *Parser) typeFold(t string) bool {
	return slices.ContainsFunc(p.rules.Types, func(x string) bool { return strings.EqualFold(x, t) })
}

// scopeAllowed indique si le scope est admis par commit.scopes (liste vide : tous).
func (p *Parser) scopeAllowed(scope string) bool {
	return len(p.rules.Scopes) == 0 || slices.Contains(p.rules.Scopes, scope)
}

// leadingWord renvoie les lettres ASCII en tête de s.
func leadingWord(s string) string {
	i := 0
	for i < len(s) && (s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z') {
		i++
	}
	return s[:i]
}

// capitalized indique une description qui commence par une majuscule, hors sigle : un premier mot d'au
// moins deux lettres ou chiffres, sans minuscule (« API », « README.md », « CI », « HTTP2 »), n'en est pas
// une.
func capitalized(desc string) bool {
	r, _ := utf8.DecodeRuneInString(desc)
	if !unicode.IsUpper(r) {
		return false
	}
	letters, lower := 0, false
	for _, c := range desc {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			break
		}
		letters++
		lower = lower || unicode.IsLower(c)
	}
	return letters < 2 || lower
}

// checkBody vérifie les lignes qui suivent le sujet : ligne vide avant le corps (body_separator) et pieds
// de message (footer_format). Elle renvoie vrai si un pied BREAKING CHANGE est présent.
func checkBody(rest []string, codes map[string]bool) bool {
	if len(rest) > 0 && strings.TrimSpace(rest[0]) != "" {
		codes[CodeBodySeparator] = true
	}
	breaking := false
	for _, l := range rest {
		switch {
		case breakingFooter.MatchString(l):
			breaking = true
		case breakingLike.MatchString(l):
			codes[CodeFooterFormat] = true
		}
	}
	// Bloc de pieds : le dernier paragraphe, après une ligne vide, quand sa première ligne est un pied.
	// Une ligne qui n'est pas un pied y continue la valeur du précédent (permis par la spécification) ;
	// seule une clé mal formée est un écart.
	start := -1
	for i := len(rest) - 1; i >= 0; i-- {
		if strings.TrimSpace(rest[i]) == "" {
			start = i + 1
			break
		}
	}
	if start <= 0 || start >= len(rest) || !footerLine.MatchString(rest[start]) {
		return breaking
	}
	for _, l := range rest[start+1:] {
		if !footerLine.MatchString(l) && badFooter.MatchString(l) {
			codes[CodeFooterFormat] = true
		}
	}
	return breaking
}
