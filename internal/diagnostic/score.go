package diagnostic

// Score d'un dépôt (1.6.0, documentation/fr/12-supervision-diagnostic.md section 6) : somme des poids des
// signaux de ses pistes, jamais affichée seule (nombre de pistes par niveau, décompte et points de chaque
// signal). Il ne classe pas les dépôts entre eux et ne sert à aucune évaluation.

import (
	"cmp"
	"slices"
	"strings"
)

// Poids des signaux dans le score (spécification, section 6).
const (
	WeightDeclared = 10 // D1, D2, D3, D5
	WeightRhythm   = 3  // R1 à R6, A1 à A4
	WeightForm     = 1  // F1 à F5, A5 à A7
)

// Weight renvoie le poids d'un signal : 0 pour un contre-indicateur (H1 à H4), le contexte D4 ou un
// signal inconnu.
func Weight(signal string) int {
	switch {
	case signal == SignalCoAuthor || signal == SignalSignature || signal == SignalBot || signal == SignalAgentBranch:
		return WeightDeclared
	case isRhythmLike(signal):
		return WeightRhythm
	case isFormLike(signal):
		return WeightForm
	}
	return 0
}

// SignalCount est le décompte d'un signal dans les pistes d'un dépôt.
type SignalCount struct {
	Signal string // identifiant (D1… H4)
	Leads  int    // pistes qui portent ce signal
	Weight int    // poids d'une occurrence
	Points int    // Leads × Weight
}

// Summary résume les pistes d'un dépôt : score, nombre de pistes par niveau et décompte des signaux.
type Summary struct {
	Score    int
	Leads    int
	Declared int
	Probable int
	ToReview int
	Signals  []SignalCount // des plus de points aux moins, puis par identifiant
}

// SummarizeByContributor résume les pistes de chaque contributeur auteur (score IA d'un contributeur,
// 1.6.0) : commit et jour par AuthorID, pull request par login de forge (logins : login en minuscules →
// contributeur). Une piste sans auteur connu est ignorée.
func SummarizeByContributor(leads []Lead, logins map[string]int64) map[int64]Summary {
	by := map[int64][]Lead{}
	for _, l := range leads {
		id := l.AuthorID
		if id == 0 && l.AuthorLogin != "" {
			id = logins[strings.ToLower(l.AuthorLogin)]
		}
		if id != 0 {
			by[id] = append(by[id], l)
		}
	}
	out := make(map[int64]Summary, len(by))
	for id, ls := range by {
		out[id] = Summarize(ls)
	}
	return out
}

// Summarize résume leads. Chaque signal déclenché compte une fois par piste, même s'il y porte plusieurs
// valeurs (deux outils déclarés en D1, par exemple).
func Summarize(leads []Lead) Summary {
	s := Summary{Leads: len(leads)}
	counts := map[string]int{}
	for _, l := range leads {
		switch l.Level {
		case LevelDeclared:
			s.Declared++
		case LevelProbable:
			s.Probable++
		case LevelToReview:
			s.ToReview++
		}
		seen := map[string]bool{}
		for _, h := range l.Hits {
			if !seen[h.Signal] {
				seen[h.Signal] = true
				counts[h.Signal]++
			}
		}
	}
	for sig, n := range counts {
		w := Weight(sig)
		s.Signals = append(s.Signals, SignalCount{Signal: sig, Leads: n, Weight: w, Points: n * w})
		s.Score += n * w
	}
	slices.SortFunc(s.Signals, func(a, b SignalCount) int {
		return cmp.Or(cmp.Compare(b.Points, a.Points), cmp.Compare(a.Signal, b.Signal))
	})
	return s
}
