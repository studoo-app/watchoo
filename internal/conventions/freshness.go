package conventions

// CL10 (phase 8.4) : fraîcheur du CHANGELOG, nombre de commits « feat » et « fix » arrivés sur la branche
// par défaut depuis le dernier commit qui a touché le CHANGELOG à la racine. Fonctions pures : l'appelant
// lit dans le clone ce dernier commit et l'intervalle qui le suit, et dans la base les messages (analysés
// en mémoire, jamais gardés). Information seulement : grisée quand CL9 du dernier jalon est conforme.

import (
	"github.com/studoo-app/watchoo/internal/model"
)

// CriterionCL10 : fraîcheur du CHANGELOG (information).
const CriterionCL10 = "CL10"

// Compteurs de CL10 (clés --json).
const (
	CountFeat    = "feat"    // commits « feat » conformes depuis le dernier commit qui a touché le CHANGELOG
	CountFix     = "fix"     // commits « fix » conformes depuis ce commit
	CountMissing = "missing" // commits de l'intervalle absents de la base (non comptés)
)

// ChangelogFreshness est CL10 d'un dépôt.
type ChangelogFreshness struct {
	// Known indique une mesure ; faux : Reason vaut UnknownNoClone (sans clone), UnknownNoFile (aucun
	// commit n'a touché le CHANGELOG à la racine de la tête) ou UnknownCloneUnreadable.
	Known  bool
	Reason string
	Since  string // SHA du dernier commit de la branche par défaut qui a touché le CHANGELOG
	Feat   int
	Fix    int
	// Missing compte les commits de l'intervalle absents de la base : le décompte est alors un minimum.
	Missing int
	// Grayed indique que CL9 du dernier jalon est conforme : le décompte n'est qu'une information.
	Grayed bool
}

// UnknownFreshness renvoie un CL10 inconnu pour la raison reason.
func UnknownFreshness(reason string) ChangelogFreshness {
	return ChangelogFreshness{Reason: reason}
}

// CheckFreshness calcule CL10 : since est le dernier commit qui a touché le CHANGELOG ; shas les commits
// de la branche par défaut accessibles depuis sa tête et pas depuis since (lus dans le clone) ; byKey les
// commits connus en base. Les fusions et les bots sont écartés (comme SV5) ; seuls les commits conformes
// au parseur strict de type « feat » ou « fix » comptent. grayed : CL9 du dernier jalon est conforme.
func CheckFreshness(since string, shas []string, byKey map[string]model.ConventionCommit, rules Rules, grayed bool) ChangelogFreshness {
	out := ChangelogFreshness{Known: true, Since: since, Grayed: grayed}
	parser := NewParser(rules)
	seen := map[string]bool{}
	for _, s := range shas {
		if seen[s] {
			continue
		}
		seen[s] = true
		row, ok := byKey[s]
		if !ok {
			out.Missing++
			continue
		}
		c := toCommit(row)
		if bumpExcluded(c) {
			continue
		}
		res := parser.Parse(c.Message)
		if !res.Conform() {
			continue
		}
		switch res.Type {
		case "feat":
			out.Feat++
		case "fix":
			out.Fix++
		}
	}
	return out
}

// Counts renvoie les compteurs de CL10 (clés CountFeat, CountFix, CountMissing) ; vide non nil si inconnu.
func (f ChangelogFreshness) Counts() map[string]int {
	if !f.Known {
		return map[string]int{}
	}
	return map[string]int{CountFeat: f.Feat, CountFix: f.Fix, CountMissing: f.Missing}
}
