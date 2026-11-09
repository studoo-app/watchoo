package conventions

// PR5 (phase 8.5) : corps structuré d'une pull request et mot-clé de fermeture. Le corps n'est lu qu'en
// mémoire, au moment de la synchro (comme le signal D2 du diagnostic) ; seul le masque de bits renvoyé par
// CheckPRBody est gardé (pull_requests.body_sections). Fonctions pures : ni base, ni forge, ni journal.

import (
	"regexp"
	"slices"
	"strings"
)

// PRRulesVersion est la version des règles de lecture du corps des PR (synonymes de sections, mot-clé de
// fermeture, masque). La monter fait relire une fois la liste complète des PR de chaque dépôt
// (repositories.conventions_prs_version) pour recalculer les masques.
const PRRulesVersion = 1

// Les clés des sections attendues dans le corps d'une PR (réglage conventions.pr.body_sections) sont
// celles du gabarit (TL3, tooling.go) : PRSectionContext, PRSectionChanges, PRSectionTesting et
// PRSectionScreenshots ; le mot-clé de fermeture (PRSectionCloses) a son propre réglage
// (conventions.pr.require_closes).

// Bits du masque body_sections. Un masque connu vaut au moins 0 (corps vide ou sans section reconnue).
const (
	PRBodyContext     int64 = 1 << iota // section contexte non vide
	PRBodyChanges                       // section « ce qui change » non vide
	PRBodyTesting                       // section « comment tester » non vide
	PRBodyScreenshots                   // section captures non vide
	PRBodyCloses                        // mot-clé de fermeture suivi d'un numéro d'issue
)

// prSectionBits associe chaque clé de section à son bit.
var prSectionBits = map[string]int64{
	PRSectionContext:     PRBodyContext,
	PRSectionChanges:     PRBodyChanges,
	PRSectionTesting:     PRBodyTesting,
	PRSectionScreenshots: PRBodyScreenshots,
}

// AllPRBodySections renvoie les clés des sections du corps de PR (PR5), dans l'ordre de la charte.
func AllPRBodySections() []string {
	return []string{PRSectionContext, PRSectionChanges, PRSectionTesting, PRSectionScreenshots}
}

// DefaultPRBodySections renvoie les sections attendues par défaut : toutes.
func DefaultPRBodySections() []string { return AllPRBodySections() }

// ValidPRBodySection indique une clé de section du corps de PR connue (« closes » n'en est pas une).
func ValidPRBodySection(key string) bool { return slices.Contains(AllPRBodySections(), key) }

// PRBodySectionBit renvoie le bit d'une clé de section ; 0 pour une clé inconnue.
func PRBodySectionBit(key string) int64 { return prSectionBits[key] }

// prBodySynonyms sont les libellés reconnus pour chaque section du corps (PR5), normalisés (normalizeHeading :
// minuscules, sans accent ni ponctuation). Un titre Markdown reconnaît une section s'il contient l'un
// d'eux mot pour mot ; un libellé en gras doit lui être égal. Liste versionnée par PRRulesVersion,
// distincte de celle du gabarit (TL3, versionnée par FileRulesVersion).
var prBodySynonyms = map[string][]string{
	PRSectionContext: {"contexte", "context", "pourquoi", "why", "motivation", "probleme", "problem",
		"description", "objectif", "objectifs", "objective", "goal", "goals", "background", "resume", "summary",
		"enjeu", "enjeux"},
	PRSectionChanges: {"ce qui change", "ce qui a change", "changements", "changes", "changes made",
		"proposed changes", "what changed", "what changes", "what s changed", "what does this pr do",
		"modifications", "modifications apportees", "solution", "implementation", "approche", "approach",
		"contenu", "travail realise"},
	PRSectionTesting: {"comment tester", "how to test", "how to test it", "how has this been tested",
		"testing", "tests", "test", "tester", "test plan", "plan de test", "plan de tests", "verification",
		"validation", "steps to test", "etapes de test", "recette"},
	PRSectionScreenshots: {"captures", "capture", "captures d ecran", "capture d ecran", "screenshots",
		"screenshot", "screen", "screens", "images", "demo", "demonstration", "visuel", "visuels", "preview",
		"gif", "video", "rendu"},
}

var (
	// prClosesRe reconnaît un mot-clé de fermeture de GitHub suivi d'un numéro d'issue : « Closes #12 »,
	// « fixes: #12 », « Resolves owner/repo#12 » (sans casse, en début de mot).
	prClosesRe = regexp.MustCompile(`(?i)(?:^|[^\pL\pN_])(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?):?[ \t]+(?:[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9_.-]+)?#[0-9]+\b`)
	// prCommentRe retire un commentaire HTML ouvert et fermé sur la même ligne.
	prCommentRe = regexp.MustCompile(`<!--.*?-->`)
)

// prBodyLine est une ligne du corps hors commentaire HTML.
type prBodyLine struct {
	text    string // ligne sans commentaire HTML ; code compris
	code    bool   // ligne d'un bloc de code clôturé (clôtures exclues)
	fence   bool   // ligne de clôture d'un bloc de code
	heading int    // niveau d'un titre ATX ; 0 sinon
	title   string // texte du titre
	bold    bool   // ligne commençant par un libellé en gras (« **Contexte** », « __Tests :__ »)
	label   string // libellé en gras, normalisé
	rest    string // texte qui suit le libellé en gras sur la même ligne
}

// blank indique une ligne sans contenu (blancs, séparateur thématique, clôture de bloc de code).
func (l prBodyLine) blank() bool {
	if l.fence {
		return true
	}
	t := strings.TrimSpace(l.text)
	return t == "" || (!l.code && mdBreakRe.MatchString(t))
}

// splitPRBody découpe le corps en lignes, sans les commentaires HTML (sur une ou plusieurs lignes) ; les
// lignes de code clôturé sont gardées, marquées.
func splitPRBody(body string) []prBodyLine {
	body = strings.TrimPrefix(strings.ToValidUTF8(body, "\uFFFD"), "\uFEFF")
	raw := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	var out []prBodyLine
	var fence string
	inComment := false
	for _, line := range raw {
		line = strings.TrimRight(line, " \t\r")
		if fence != "" {
			if isFenceClose(line, fence) {
				fence = ""
				out = append(out, prBodyLine{text: line, fence: true})
				continue
			}
			out = append(out, prBodyLine{text: line, code: true})
			continue
		}
		if inComment {
			i := strings.Index(line, "-->")
			if i < 0 {
				continue
			}
			inComment = false
			line = line[i+3:]
		}
		line = prCommentRe.ReplaceAllString(line, "")
		if i := strings.Index(line, "<!--"); i >= 0 {
			inComment = true
			line = line[:i]
		}
		if m := mdFenceRe.FindStringSubmatch(line); m != nil && (m[1][0] != '`' || !strings.Contains(m[2], "`")) {
			fence = m[1]
			out = append(out, prBodyLine{text: line, fence: true})
			continue
		}
		l := prBodyLine{text: line}
		if lvl, txt, ok := atxHeading(line); ok {
			l.heading, l.title = lvl, txt
		} else if label, rest, ok := boldLabel(line); ok {
			l.bold, l.label, l.rest = true, normalizeHeading(label), rest
		}
		out = append(out, l)
	}
	return out
}

// boldLabel lit un libellé en gras en tête de ligne : « **Contexte** », « **Contexte :** suite »,
// « __Tests__: suite ». rest est le texte qui suit, sans deux-points ni blancs autour.
func boldLabel(line string) (label, rest string, ok bool) {
	t := strings.TrimSpace(line)
	var mark string
	switch {
	case strings.HasPrefix(t, "**"):
		mark = "**"
	case strings.HasPrefix(t, "__"):
		mark = "__"
	default:
		return "", "", false
	}
	end := strings.Index(t[2:], mark)
	if end <= 0 {
		return "", "", false
	}
	label = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(t[2:2+end]), ":"))
	rest = strings.TrimSpace(t[2+end+2:])
	rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
	return label, rest, label != ""
}

// prSectionsOf renvoie les sections reconnues par un titre (le titre contient un synonyme) ou par un
// libellé en gras (égal à un synonyme).
func prSectionsOf(text string, exact bool) []string {
	var keys []string
	for _, key := range AllPRBodySections() {
		if slices.ContainsFunc(prBodySynonyms[key], func(s string) bool {
			if exact {
				return text == s
			}
			return containsPhrase(text, s)
		}) {
			keys = append(keys, key)
		}
	}
	return keys
}

// CheckPRBody lit le corps d'une pull request (en mémoire) et renvoie son masque PR5 : un bit par section
// reconnue et non vide, plus PRBodyCloses si un mot-clé de fermeture (close, closes, closed, fix, fixes,
// fixed, resolve, resolves, resolved) est suivi d'un numéro d'issue.
//
// Une section commence à un titre Markdown (ATX) dont le texte normalisé contient un synonyme, ou à une
// ligne qui commence par un libellé en gras égal à un synonyme. Elle est non vide si une ligne de contenu
// la suit : texte, puce, image, tableau ou code, hors commentaire HTML, avant le titre suivant de niveau
// inférieur ou égal (un sous-titre et son contenu en font partie), ou, pour un libellé en gras, avant le
// titre ou le libellé en gras suivant ; le texte écrit après le libellé, sur la même ligne, compte. Le
// mot-clé de fermeture n'est cherché ni dans le code ni dans les commentaires HTML. Faux négatif connu :
// une issue liée par la barre latérale « Development » de GitHub n'apparaît pas dans le corps.
func CheckPRBody(body string) int64 {
	lines := splitPRBody(body)
	var mask int64
	for i, l := range lines {
		if !l.code && !l.fence && prClosesRe.MatchString(stripCodeSpans(l.text)) {
			mask |= PRBodyCloses
		}
		var keys []string
		switch {
		case l.heading > 0:
			keys = prSectionsOf(normalizeHeading(l.title), false)
		case l.bold:
			keys = prSectionsOf(l.label, true)
		}
		if len(keys) == 0 || !sectionFilled(lines, i) {
			continue
		}
		for _, k := range keys {
			mask |= prSectionBits[k]
		}
	}
	return mask
}

// sectionFilled indique que la section ouverte à la ligne i a du contenu.
func sectionFilled(lines []prBodyLine, i int) bool {
	start := lines[i]
	if start.bold && strings.TrimSpace(start.rest) != "" {
		return true
	}
	for _, l := range lines[i+1:] {
		switch {
		case l.code || l.fence:
		case l.heading > 0 && (start.bold || l.heading <= start.heading):
			return false
		case start.bold && l.bold:
			return false
		}
		if !l.blank() && l.heading == 0 {
			return true
		}
	}
	return false
}

// PRBodyRules sont les attentes PR5, tirées des réglages conventions.pr.*.
type PRBodyRules struct {
	Sections      []string // sections attendues (clés d'AllPRBodySections) ; vide : aucune
	RequireCloses bool     // mot-clé de fermeture exigé
}

// DefaultPRBodyRules renvoie les attentes de la charte : toutes les sections et « Closes #n ».
func DefaultPRBodyRules() PRBodyRules {
	return PRBodyRules{Sections: DefaultPRBodySections(), RequireCloses: true}
}

// PRBodyResult est l'évaluation PR5 d'un masque. Elle ne garde rien du corps.
type PRBodyResult struct {
	Known   bool     // masque connu (body_sections non NULL)
	Found   []string // sections trouvées, ordre d'AllPRBodySections ; vide non nil si aucune
	Missing []string // sections attendues absentes ou vides, ordre d'AllPRBodySections ; vide non nil si aucune
	Closes  bool     // mot-clé de fermeture présent
	Conform bool     // masque connu, aucune section attendue manquante et, si exigé, mot-clé présent
}

// EvalPRBody évalue un masque body_sections (nil : inconnu) selon les attentes r. Une clé inconnue de
// r.Sections est ignorée.
func EvalPRBody(mask *int64, r PRBodyRules) PRBodyResult {
	out := PRBodyResult{Found: []string{}, Missing: []string{}}
	if mask == nil {
		return out
	}
	out.Known = true
	out.Closes = *mask&PRBodyCloses != 0
	for _, k := range AllPRBodySections() {
		has := *mask&prSectionBits[k] != 0
		if has {
			out.Found = append(out.Found, k)
		} else if slices.Contains(r.Sections, k) {
			out.Missing = append(out.Missing, k)
		}
	}
	out.Conform = len(out.Missing) == 0 && (!r.RequireCloses || out.Closes)
	return out
}
