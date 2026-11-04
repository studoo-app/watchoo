package diagnostic

// Statistique robuste des signaux de rythme (spécification, section 4.2) : médiane, écart absolu médian
// (MAD) et score z modifié d'Iglewicz et Hoaglin (1993) :
//
//	z* = 0,6745 × (x − m) / MAD
//
// Seuil par défaut 3,5. Avec moins de MinHistory observations, ou un MAD nul, le score n'est pas défini
// (« historique insuffisant ») et le signal ne se déclenche pas. Référence : NIST/SEMATECH, e-Handbook of
// Statistical Methods, 1.3.5.17.

import (
	"slices"
	"time"
)

// Paramètres des signaux de rythme (spécification, section 4.2). Non calibrés : la phase 7.6 les ajustera
// à partir des revues humaines.
const (
	ZThreshold    = 3.5             // seuil du score z modifié
	MinHistory    = 20              // observations minimales de l'historique
	HistoryWindow = 90              // fenêtre glissante, en jours
	zFactor       = 0.6745          // facteur du score z modifié (quantile 0,75 de la loi normale)
	minR6Recent   = 3               // jours travaillés minimaux sur les 14 jours récents de R6
	r6RecentDays  = 14              // période récente de R6, en jours
	sameMinute    = 1 * time.Minute // R4 : écart minimal entre deux commits pour mesurer une vitesse
	historySpan   = HistoryWindow * 24 * time.Hour
)

// robust résume un historique : nombre d'observations, médiane et MAD.
type robust struct {
	N      int
	Median float64
	MAD    float64
}

// robustOf calcule la médiane et le MAD de xs (xs n'est pas modifié).
func robustOf(xs []float64) robust {
	if len(xs) == 0 {
		return robust{}
	}
	m := median(xs)
	dev := make([]float64, len(xs))
	for i, x := range xs {
		dev[i] = abs(x - m)
	}
	return robust{N: len(xs), Median: m, MAD: median(dev)}
}

// z renvoie le score z modifié de x ; ok faux si l'historique est insuffisant (moins de MinHistory
// observations ou MAD nul).
func (r robust) z(x float64) (float64, bool) {
	if r.N < MinHistory || r.MAD == 0 {
		return 0, false
	}
	return zFactor * (x - r.Median) / r.MAD, true
}

// median renvoie la médiane de xs (non vide), sans le modifier.
func median(xs []float64) float64 {
	s := slices.Clone(xs)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
