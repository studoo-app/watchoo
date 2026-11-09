package conventions

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Codes CC5 de formulation (avertissements, sans effet sur CC1 ; clés --json), en anglais.
const (
	WordingPastTense   = "past_tense"   // premier mot de la description : participe passé du lexique fermé
	WordingProgressive = "progressive"  // premier mot en « -ing » (hors exceptions)
	WordingFirstPerson = "first_person" // premier mot : pronom ou possessif de première personne
)

// AllWordingCodes liste les codes CC5 dans leur ordre d'affichage.
var AllWordingCodes = []string{WordingPastTense, WordingProgressive, WordingFirstPerson}

// Langues CC7 d'un sujet.
const (
	LanguageFR      = "fr"
	LanguageEN      = "en"
	LanguageUnknown = "unknown"
)

// pastTenseEN est le lexique fermé des participes anglais (CC5). Aucune règle sur la seule terminaison.
var pastTenseEN = []string{
	"added", "fixed", "updated", "removed", "changed", "created", "deleted", "renamed", "moved",
	"improved", "implemented", "refactored", "replaced", "upgraded", "modified", "corrected", "cleaned",
	"edited", "finished", "completed",
}

// pastTenseFRStems sont les participes français en « -é » du lexique fermé (CC5), déclinés en « -é »,
// « -ée », « -és », « -ées ». « mise » n'y figure pas : « mise à jour » est un nom.
var pastTenseFRStems = []string{
	"ajouté", "corrigé", "modifié", "supprimé", "créé", "changé", "renommé", "déplacé", "amélioré",
	"terminé", "remplacé", "nettoyé", "implémenté",
}

// pastTenseFRWords sont les autres participes français du lexique fermé.
var pastTenseFRWords = []string{"mis", "fait", "faits", "fini", "finie", "finis", "finies"}

// pastTense est le lexique CC5 complet, en minuscules.
var pastTense = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range pastTenseEN {
		m[w] = true
	}
	for _, w := range pastTenseFRWords {
		m[w] = true
	}
	for _, s := range pastTenseFRStems {
		for _, suffix := range []string{"", "e", "s", "es"} {
			m[s+suffix] = true
		}
	}
	return m
}()

// ProgressiveExceptions sont les mots en « -ing » qui ne sont pas une forme progressive (CC5) : noms
// courants, prépositions, emprunts du français.
var ProgressiveExceptions = []string{
	"string", "strings", "thing", "things", "bring", "ping", "ring", "sing", "king", "wing", "sting", "swing",
	"spring", "during", "nothing", "something", "everything", "anything", "logging", "padding", "spacing",
	"heading", "routing", "styling", "setting", "mapping", "parking", "meeting", "marketing",
}

// firstPerson sont les premiers mots de première personne (CC5) ; « j' » est reconnu à part.
var firstPerson = []string{"i", "je", "we", "nous", "my", "mon", "ma", "mes"}

// Mots-outils fermés de l'heuristique de langue (CC7) ; les mots courants dans les deux langues sont
// absents (testé) : « a », « me », « on » (pronom français), « an » (« un an »), « as », « but » (« le
// but »), « plus », « car », « son ».
var (
	functionWordsFR = map[string]bool{}
	functionWordsEN = map[string]bool{}
)

func init() {
	for _, w := range []string{
		"le", "la", "les", "l", "un", "une", "des", "du", "de", "d", "au", "aux", "et", "ou", "pour", "dans",
		"sur", "avec", "sans", "par", "en", "est", "sont", "qui", "que", "qu", "ce", "cet", "cette", "ces",
		"sa", "ses", "leur", "leurs", "ne", "pas", "lors", "quand", "si", "entre", "vers",
		"chez", "depuis", "mais", "donc", "nos", "notre", "votre", "vos", "il", "elle", "ils", "elles",
	} {
		functionWordsFR[w] = true
	}
	for _, w := range []string{
		"the", "and", "or", "for", "in", "with", "without", "by", "to", "of", "from", "is", "are",
		"was", "were", "be", "this", "that", "these", "those", "it", "its", "not", "when", "into", "onto", "at",
		"if", "so", "our", "your", "their", "there", "which", "who", "after", "before", "about",
		"instead", "than", "then", "now", "all", "only", "some", "more", "less", "can", "should", "will",
	} {
		functionWordsEN[w] = true
	}
}

// issueRef cite une issue : « Closes #12 », « fixes: #3 » (mot-clé sans casse, deux-points facultatif).
var issueRef = regexp.MustCompile(`(?i)\b(?:closes|fixes|resolves|refs):? #[0-9]+\b`)

// Traits sont les caractéristiques informatives d'un message (CC5 à CC7), calculées en mémoire. Elles ne
// gardent rien du texte.
type Traits struct {
	Wording  []string // codes CC5, dans l'ordre d'AllWordingCodes ; vide non nil si aucun
	Footer   bool     // CC6 : bloc de pieds valide (dernier paragraphe, après une ligne vide)
	Issue    bool     // CC6 : cite une issue (Closes|Fixes|Resolves|Refs #n), sujet compris
	Language string   // CC7 : fr, en ou unknown, d'après la description du sujet
}

// Inspect calcule les caractéristiques CC5 à CC7 d'un message de commit. La description est la partie
// du sujet (sans suffixe « (#n) ») après un en-tête « type(scope)!: », sinon le sujet entier.
func Inspect(message string) Traits {
	subject, rest, _ := splitMessage(message)
	desc := description(StripSquashSuffix(subject))
	return Traits{
		Wording:  Wording(desc),
		Footer:   hasFooterBlock(rest),
		Issue:    issueRef.MatchString(message),
		Language: DetectLanguage(desc),
	}
}

// description renvoie la description d'un sujet : après l'en-tête s'il y en a un, sinon le sujet entier.
func description(s string) string {
	if m := looseHeader.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[6])
	}
	return s
}

// Wording renvoie les codes CC5 du premier mot d'une description.
func Wording(desc string) []string {
	out := []string{}
	raw := firstWord(desc)
	w := strings.ToLower(raw)
	if w == "" {
		return out
	}
	if pastTense[w] {
		out = append(out, WordingPastTense)
	}
	if len(w) > len("ing") && strings.HasSuffix(w, "ing") && !slices.Contains(ProgressiveExceptions, w) {
		out = append(out, WordingProgressive)
	}
	if slices.Contains(firstPerson, w) || strings.HasPrefix(w, "j'") || strings.HasPrefix(w, "j’") {
		out = append(out, WordingFirstPerson)
	}
	return out
}

// firstWord renvoie le premier mot de s, sans la ponctuation qui le suit (« added, » → « added »).
func firstWord(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return strings.TrimRightFunc(f[0], func(r rune) bool { return unicode.IsPunct(r) && r != '\'' && r != '’' })
}

// hasFooterBlock indique un bloc de pieds : le dernier paragraphe, après une ligne vide, dont la première
// ligne est un pied valide (même règle que la vérification CC2 de footer_format).
func hasFooterBlock(rest []string) bool {
	for i := len(rest) - 1; i >= 0; i-- {
		if strings.TrimSpace(rest[i]) == "" {
			return i+1 < len(rest) && footerLine.MatchString(rest[i+1])
		}
	}
	return false
}

// DetectLanguage devine la langue d'un texte court sans dépendance : chaque mot-outil français ou mot
// avec une lettre accentuée compte pour le français, chaque mot-outil anglais pour l'anglais ; la langue
// la plus comptée l'emporte, unknown à égalité (zéro compris).
func DetectLanguage(s string) string {
	fr, en := 0, 0
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	for _, w := range words {
		switch {
		case functionWordsFR[w] || hasAccent(w):
			fr++
		case functionWordsEN[w]:
			en++
		}
	}
	switch {
	case fr > en:
		return LanguageFR
	case en > fr:
		return LanguageEN
	}
	return LanguageUnknown
}

// hasAccent indique une lettre hors ASCII (é, à, ç…).
func hasAccent(w string) bool {
	for _, r := range w {
		if r > unicode.MaxASCII && unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// MixedThreshold est la part (des sujets reconnus) que chaque langue doit dépasser pour un « mélange ».
const MixedThreshold = 0.20

// TraitsTally compte les caractéristiques CC5 à CC7 d'un ensemble de commits évalués (dépôt ou
// contributeur).
type TraitsTally struct {
	Evaluated int            // commits comptés
	Wording   map[string]int // CC5 : commits par code (toutes les clés d'AllWordingCodes)
	Footer    int            // CC6 : commits avec un bloc de pieds valide
	Issue     int            // CC6 : commits qui citent une issue
	Languages map[string]int // CC7 : commits par langue (fr, en, unknown)
}

// NewTraitsTally renvoie un décompte vide, toutes clés présentes.
func NewTraitsTally() TraitsTally {
	t := TraitsTally{Wording: map[string]int{}, Languages: map[string]int{}}
	for _, c := range AllWordingCodes {
		t.Wording[c] = 0
	}
	for _, l := range []string{LanguageFR, LanguageEN, LanguageUnknown} {
		t.Languages[l] = 0
	}
	return t
}

// Add compte les caractéristiques d'un commit évalué.
func (t *TraitsTally) Add(tr Traits) {
	if t.Wording == nil || t.Languages == nil {
		*t = t.Merge(NewTraitsTally())
	}
	t.Evaluated++
	for _, c := range tr.Wording {
		t.Wording[c]++
	}
	if tr.Footer {
		t.Footer++
	}
	if tr.Issue {
		t.Issue++
	}
	lang := tr.Language
	if lang != LanguageFR && lang != LanguageEN {
		lang = LanguageUnknown
	}
	t.Languages[lang]++
}

// Merge renvoie la somme de t et b (un dépôt à partir de ses contributeurs, par exemple).
func (t TraitsTally) Merge(b TraitsTally) TraitsTally {
	a := NewTraitsTally()
	for _, x := range []TraitsTally{t, b} {
		a.Evaluated += x.Evaluated
		a.Footer += x.Footer
		a.Issue += x.Issue
		for k, v := range x.Wording {
			a.Wording[k] += v
		}
		for k, v := range x.Languages {
			a.Languages[k] += v
		}
	}
	return a
}

// LanguageSummary est le résumé CC7 d'un dépôt.
type LanguageSummary struct {
	FR, EN, Unknown int
	Expected        string // commit.language : fr, en ou any
	Other           int    // commits reconnus dans l'autre langue que Expected ; 0 si Expected vaut any
	Mixed           bool   // les deux langues dépassent chacune MixedThreshold des commits reconnus
}

// LanguageSummary résume CC7 pour la langue attendue (conventions.commit.language).
func (t TraitsTally) LanguageSummary(expected string) LanguageSummary {
	s := LanguageSummary{
		FR: t.Languages[LanguageFR], EN: t.Languages[LanguageEN], Unknown: t.Languages[LanguageUnknown],
		Expected: expected,
	}
	switch expected {
	case LanguageFR:
		s.Other = s.EN
	case LanguageEN:
		s.Other = s.FR
	}
	if known := s.FR + s.EN; known > 0 {
		s.Mixed = float64(s.FR) > MixedThreshold*float64(known) && float64(s.EN) > MixedThreshold*float64(known)
	}
	return s
}

// TallyOf recompte un Tally (CC1 et ses écarts) sur les résultats retenus par keep, parmi ceux qui ne sont
// pas antérieurs à la date de début : CC1 par intervalle entre tags, tendance de CC3 avant et depuis le
// dernier tag final.
func TallyOf(results []CommitResult, keep func(CommitResult) bool) Tally {
	t := newTally()
	for _, r := range results {
		if r.BeforeSince || !keep(r) {
			continue
		}
		t.add(r)
	}
	t.sortIssues()
	return t
}
