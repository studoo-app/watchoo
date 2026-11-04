package diagnostic

// Signaux de forme F1 à F5 (phase 7.5 ; spécification, section 4.3), calculés à la lecture à partir des
// mesures de forme de commit_signals, chaque apprenant étant comparé à son propre historique dans le dépôt
// (90 jours avant le commit ; F4 : ses 20 messages précédents). Indices d'appoint : un signal F ne lève
// jamais seul une piste (voir Level). F6 (classifieur de contenu) est reporté (section 9 du document 12).

import (
	"github.com/studoo-app/watchoo/internal/store"
)

// Identifiants des signaux de forme.
const (
	SignalCommentDensity = "F1" // densité de commentaires
	SignalDocstrings     = "F2" // docstrings systématiques
	SignalLexicon        = "F3" // diversité lexicale des identifiants
	SignalMessageStyle   = "F4" // style des messages
	SignalTests          = "F5" // tests livrés avec le code
)

// Seuils des signaux de forme (spécification, section 4.3 ; non calibrés avant la phase 7.6).
const (
	f1MinLines     = 20   // F1 : au moins 20 lignes source ajoutées
	f2MinFunctions = 3    // F2 : au moins 3 définitions ajoutées
	f2Ratio        = 0.90 // F2 : part documentée au-delà
	f2MedianMax    = 0.30 // F2 : médiane personnelle en deçà
	f4Previous     = 20   // F4 : messages précédents comparés
	f4MinBullets   = 2    // F4 : liste d'au moins 2 lignes
	f5MinCode      = 20   // F5 : au moins 20 lignes de code (hors tests)
	f5MinTests     = 5    // F5 : au moins 5 lignes de test
	f5ShareMax     = 0.10 // F5 : part historique des commits avec tests en deçà
)

// formSignals calcule F1 à F5 pour chaque commit d'un auteur (g trié par date) ; seuls les commits dont la
// forme est mesurée comptent, pour le commit comme pour l'historique.
func formSignals(g []store.CommitSignalView) map[string][]Hit {
	out := map[string][]Hit{}
	var known []store.CommitSignalView
	for _, c := range g {
		if c.Form.Known {
			known = append(known, c)
		}
	}
	start := 0
	for i, c := range known {
		for start < i && known[start].AuthoredAt.Before(c.AuthoredAt.Add(-historySpan)) {
			start++
		}
		// Historique : commits mesurés de [t − 90 j, t), ceux de la même seconde exclus.
		var density, documented, lexicon []float64
		codeCommits, withTests := 0, 0
		for _, p := range known[start:i] {
			if !p.AuthoredAt.Before(c.AuthoredAt) {
				continue
			}
			f := p.Form
			if f.SourceLines >= f1MinLines {
				density = append(density, ratio(f.CommentLines, f.SourceLines))
			}
			if f.Functions > 0 {
				documented = append(documented, ratio(f.DocumentedFunctions, f.Functions))
			}
			if f.HasDiversity {
				lexicon = append(lexicon, f.LexicalDiversity)
			}
			if f.SourceLines-f.TestLines >= f5MinCode {
				codeCommits++
				if f.TestLines >= f5MinTests {
					withTests++
				}
			}
		}
		f := c.Form
		var hits []Hit
		if f.SourceLines >= f1MinLines {
			x := ratio(f.CommentLines, f.SourceLines)
			r := robustOf(density)
			if z, ok := r.z(x); ok && z > ZThreshold {
				hits = append(hits, statHit(SignalCommentDensity, x, r.Median, z, true, true))
			}
		}
		if f.Functions >= f2MinFunctions && len(documented) >= MinHistory {
			x := ratio(f.DocumentedFunctions, f.Functions)
			if m := median(documented); x > f2Ratio && m < f2MedianMax {
				hits = append(hits, statHit(SignalDocstrings, x, m, 0, true, false))
			}
		}
		if f.HasDiversity {
			r := robustOf(lexicon)
			if z, ok := r.z(f.LexicalDiversity); ok && z < -ZThreshold {
				hits = append(hits, statHit(SignalLexicon, f.LexicalDiversity, r.Median, z, true, true))
			}
		}
		if h, ok := messageStyleHit(known[max(0, i-f4Previous):i], c); ok {
			hits = append(hits, h)
		}
		if f.SourceLines-f.TestLines >= f5MinCode && f.TestLines >= f5MinTests && codeCommits >= MinHistory {
			if share := ratio(withTests, codeCommits); share < f5ShareMax {
				hits = append(hits, statHit(SignalTests, float64(f.TestLines), share, 0, true, false))
			}
		}
		if len(hits) > 0 {
			out[c.SHA] = hits
		}
	}
	return out
}

// messageStyleHit calcule F4 : changement brusque du style du message de c par rapport aux f4Previous
// messages précédents (prev) : longueur au-delà du seuil du score z modifié, ET une nouveauté de forme
// absente de tous les messages précédents (liste d'au moins f4MinBullets lignes, ou sujet au format
// Conventional Commits).
func messageStyleHit(prev []store.CommitSignalView, c store.CommitSignalView) (Hit, bool) {
	if len(prev) < f4Previous {
		return Hit{}, false
	}
	lengths := make([]float64, len(prev))
	bullets, conv := false, false
	for i, p := range prev {
		lengths[i] = float64(p.Form.MessageLength)
		bullets = bullets || p.Form.MessageBullets >= f4MinBullets
		conv = conv || p.Form.MessageConventional
	}
	novelty := (c.Form.MessageBullets >= f4MinBullets && !bullets) || (c.Form.MessageConventional && !conv)
	if !novelty {
		return Hit{}, false
	}
	x := float64(c.Form.MessageLength)
	r := robustOf(lengths)
	z, ok := r.z(x)
	if !ok || z <= ZThreshold {
		return Hit{}, false
	}
	return statHit(SignalMessageStyle, x, r.Median, z, true, true), true
}

// ratio renvoie a/b (0 si b est nul).
func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
