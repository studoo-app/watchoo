package diagnostic

// Signaux absolus A1 à A7 et contre-indicateurs H1 à H4 (phase 7.7, documentation/fr/12-supervision-diagnostic.md,
// section 9.6). Les signaux A ne demandent pas d'historique personnel : ils couvrent les projets rendus en peu de
// commits. Les contre-indicateurs (traces de travail humain) abaissent une piste d'un niveau, sans jamais la
// supprimer (voir Level). Seuils du référentiel d'enseignants (référence 34), non calibrés avant la phase 7.6.

import (
	"strconv"
	"time"

	"github.com/studoo-app/watchoo/internal/store"
)

// Identifiants des signaux absolus et des contre-indicateurs.
const (
	SignalMassiveCommit = "A1" // commit massif
	SignalOneGo         = "A2" // dépôt rendu « en une fois »
	SignalAfterSilence  = "A3" // poussée après un silence
	SignalRevertReplace = "A4" // revert puis remplacement
	SignalDecorative    = "A5" // commentaires décoratifs
	SignalNoAccents     = "A6" // commentaires français sans accents
	SignalFinalState    = "A7" // code « dans son état final »

	SignalWIP          = "H1" // marqueurs de travail en cours
	SignalShortVars    = "H2" // variables courtes
	SignalSkeleton     = "H3" // squelette initial
	SignalIterativeFix = "H4" // corrections itératives
)

// Seuils des signaux absolus et des contre-indicateurs.
const (
	a1MinLines   = 500                 // A1 : lignes source ajoutées
	a2MaxCommits = 3                   // A2 : commits mesurés de l'auteur dans le dépôt, au plus
	a2Share      = 0.80                // A2 : part du commit dominant
	a2MinLines   = 200                 // A2 : lignes source ajoutées par l'auteur, au moins
	a3MinLines   = 300                 // A3 : lignes source ajoutées
	a3Silence    = 14 * 24 * time.Hour // A3 : silence avant le commit
	a4Window     = time.Hour           // A4 : délai entre le revert et le remplacement
	a4MinLines   = 100                 // A4 : lignes source ajoutées du remplacement
	a5MinLines   = 3                   // A5 : lignes décoratives
	a6MinWords   = 10                  // A6 : mots français repérés
	a6Share      = 0.90                // A6 : part sans accents
	a7MinLines   = 800                 // A7 : lignes source ajoutées par l'auteur dans le dépôt
	h2MinShort   = 3                   // H2 : identifiants d'une lettre
	h3MinFiles   = 5                   // H3 : fichiers du premier commit
	h3MaxPerFile = 2                   // H3 : lignes ajoutées par fichier, en moyenne, au plus
	h4MinFixes   = 3                   // H4 : commits de correction
	h4Window     = 14 * 24 * time.Hour // H4 : période des corrections, avant le commit
)

// isRhythmLike indique si un signal compte comme un signal de rythme pour les niveaux (R1 à R6, A1 à A4).
func isRhythmLike(s string) bool {
	switch s {
	case SignalDailyLines, SignalCommitSize, SignalAddRatio, SignalSpeed, SignalFiles, SignalRegimeBreak,
		SignalMassiveCommit, SignalOneGo, SignalAfterSilence, SignalRevertReplace:
		return true
	}
	return false
}

// isFormLike indique si un signal compte comme un signal de forme pour les niveaux (F1 à F5, A5 à A7).
func isFormLike(s string) bool {
	switch s {
	case SignalCommentDensity, SignalDocstrings, SignalLexicon, SignalMessageStyle, SignalTests,
		SignalDecorative, SignalNoAccents, SignalFinalState:
		return true
	}
	return false
}

// isCounter indique si un signal est un contre-indicateur (H1 à H4).
func isCounter(s string) bool {
	switch s {
	case SignalWIP, SignalShortVars, SignalSkeleton, SignalIterativeFix:
		return true
	}
	return false
}

// absoluteSignals calcule A1 à A7 et H1 à H4 pour les commits d'un auteur dans un dépôt (g trié par date).
// A2 et A7 portent sur l'ensemble de l'auteur dans le dépôt : ils sont rattachés à son commit le plus gros
// (lignes source ajoutées), pour qu'une même piste réunisse tous les indices du dépôt rendu d'un coup.
// Conventions actives (opts.ConventionsEnabled, phase 8.1), un commit de squash ne lève pas A1 et A2 ne
// compte que les commits non squash (nombre, total, commit dominant) ; A7 reste calculé sur tous.
func absoluteSignals(g []store.CommitSignalView, opts RhythmOptions) map[string][]Hit {
	out := map[string][]Hit{}
	add := func(sha string, h Hit) { out[sha] = append(out[sha], h) }

	allKnown := true  // A7 : forme mesurée sur tous les commits
	allKnown2 := true // A2 : forme mesurée sur les commits non squash
	total, wip, short := 0, 0, 0
	biggest := -1
	// A2 : commits, total et commit dominant hors squash quand les conventions sont actives.
	n2, total2, biggest2 := 0, 0, -1
	for i, c := range g {
		f := c.Form
		if !opts.squashIgnored(c) {
			n2++
		}
		if !f.Known {
			allKnown = false
			if !opts.squashIgnored(c) {
				allKnown2 = false
			}
			continue
		}
		total += f.SourceLines
		wip += f.WIPMarkers
		short += f.ShortIdentifiers
		if biggest < 0 || f.SourceLines > g[biggest].Form.SourceLines {
			biggest = i
		}
		if !opts.squashIgnored(c) {
			total2 += f.SourceLines
			if biggest2 < 0 || f.SourceLines > g[biggest2].Form.SourceLines {
				biggest2 = i
			}
		}
	}

	for i, c := range g {
		f := c.Form
		if f.Known {
			if f.SourceLines >= a1MinLines && !opts.squashIgnored(c) {
				add(c.SHA, countHit(SignalMassiveCommit, f.SourceLines))
			}
			if i > 0 && f.SourceLines >= a3MinLines {
				if gap := c.AuthoredAt.Sub(g[i-1].AuthoredAt); gap >= a3Silence {
					add(c.SHA, countHit(SignalAfterSilence, int(gap.Hours()/24)))
				}
			}
			if i > 0 && f.SourceLines >= a4MinLines {
				if p := g[i-1]; p.Form.Known && p.Form.MessageRevert && c.AuthoredAt.Sub(p.AuthoredAt) <= a4Window {
					add(c.SHA, countHit(SignalRevertReplace, int(c.AuthoredAt.Sub(p.AuthoredAt).Minutes())))
				}
			}
			if f.DecorativeLines >= a5MinLines {
				add(c.SHA, countHit(SignalDecorative, f.DecorativeLines))
			}
			if f.FrWords >= a6MinWords {
				if share := ratio(f.FrWordsUnaccented, f.FrWords); share >= a6Share {
					add(c.SHA, statHit(SignalNoAccents, share, 0, 0, false, false))
				}
			}
			// Contre-indicateurs du commit.
			if f.WIPMarkers > 0 {
				add(c.SHA, countHit(SignalWIP, f.WIPMarkers))
			}
			if f.ShortIdentifiers >= h2MinShort {
				add(c.SHA, countHit(SignalShortVars, f.ShortIdentifiers))
			}
		}
		if fixes := recentFixes(g, i); fixes >= h4MinFixes {
			add(c.SHA, countHit(SignalIterativeFix, fixes))
		}
	}
	if len(g) > 0 && isSkeleton(g[0]) {
		// Le squelette initial tempère toutes les pistes de l'auteur dans le dépôt.
		for _, c := range g {
			add(c.SHA, countHit(SignalSkeleton, g[0].FilesKept))
		}
	}

	if biggest2 >= 0 {
		b := g[biggest2]
		if allKnown2 && n2 <= a2MaxCommits && total2 >= a2MinLines &&
			ratio(b.Form.SourceLines, total2) >= a2Share {
			add(b.SHA, statHit(SignalOneGo, ratio(b.Form.SourceLines, total2), 0, 0, false, false))
		}
	}
	if biggest >= 0 {
		b := g[biggest]
		if allKnown && total >= a7MinLines && wip == 0 && short == 0 {
			add(b.SHA, countHit(SignalFinalState, total))
		}
	}
	return out
}

// recentFixes compte les commits de correction de l'auteur dans les h4Window qui précèdent g[i] (exclu).
func recentFixes(g []store.CommitSignalView, i int) int {
	n := 0
	for j := i - 1; j >= 0 && g[i].AuthoredAt.Sub(g[j].AuthoredAt) <= h4Window; j-- {
		if g[j].Form.Known && g[j].Form.MessageFix {
			n++
		}
	}
	return n
}

// isSkeleton indique si le premier commit de l'auteur est un squelette : au moins h3MinFiles fichiers
// retenus, et au plus h3MaxPerFile lignes ajoutées par fichier en moyenne.
func isSkeleton(c store.CommitSignalView) bool {
	return c.FilesKept >= h3MinFiles && c.AdditionsKept <= h3MaxPerFile*c.FilesKept
}

// countHit construit un signal dont la valeur est un nombre entier.
func countHit(signal string, n int) Hit {
	return Hit{Signal: signal, Value: strconv.Itoa(n), Raw: float64(n)}
}
