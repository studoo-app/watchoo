package diagnostic

// Signaux de rythme R1 à R6 (spécification, section 4.2), calculés à partir des signaux par commit de
// commit_signals. L'historique est celui de l'auteur dans le dépôt (un projet d'apprenant par dépôt), sur
// les 90 jours qui précèdent l'événement ; un jour est un jour du fuseau d'affichage.

import (
	"cmp"
	"slices"
	"strconv"
	"time"

	"github.com/studoo-app/watchoo/internal/store"
)

// Identifiants des signaux de rythme.
const (
	SignalDailyLines  = "R1" // lignes nettes par jour
	SignalCommitSize  = "R2" // lignes ajoutées par le commit
	SignalAddRatio    = "R3" // ratio d'ajout
	SignalSpeed       = "R4" // vitesse apparente
	SignalFiles       = "R5" // fichiers par commit
	SignalRegimeBreak = "R6" // rupture de régime
)

// Seuils des signaux de rythme (spécification, section 4.2).
const (
	r2MinLines   = 150  // R2 : au moins 150 lignes ajoutées
	r3MinLines   = 150  // R3 : commit de plus de 150 lignes ajoutées
	r3Ratio      = 0.95 // R3 : ratio d'ajout au-delà
	r3MedianMax  = 0.80 // R3 : médiane personnelle en deçà
	r4MinLines   = 100  // R4 : commit de plus de 100 lignes ajoutées
	r4Speed      = 10.0 // R4 : lignes par minute au-delà
	r5MinFiles   = 8    // R5 : au moins 8 fichiers
	r6RatioLimit = 3.0  // R6 : rapport des médianes au-delà
)

// RhythmOptions règle le calcul des signaux de rythme.
type RhythmOptions struct {
	Location    *time.Location // fuseau des jours (R1, R4, R6) ; nil = UTC
	SpeedSignal bool           // calcule R4 (réglage diagnostic.speed_signal)
	// RetentionCutoff : aucune piste datée d'avant (durée de conservation, diagnostic.retention_days) ;
	// zéro = pas de borne. Vaut aussi pour les pistes de PR, dont la branche (D5) reste en base.
	RetentionCutoff time.Time
}

// AuthorHistory résume l'historique d'un auteur dans un dépôt.
type AuthorHistory struct {
	RepoID int64
	store.RepoRef
	AuthorID    int64
	AuthorName  string
	AuthorLogin string
	Commits     int  // commits mesurés
	WorkedDays  int  // jours avec au moins un commit mesuré
	Sufficient  bool // au moins MinHistory commits mesurés dans les 90 jours avant le dernier
	LastCommit  time.Time
	lastWindowN int
}

// commitRhythm est le résultat des signaux de rythme d'un commit.
type commitRhythm struct {
	view store.CommitSignalView
	hits []Hit
}

// dayRhythm est le résultat des signaux de rythme d'un jour d'un auteur dans un dépôt.
type dayRhythm struct {
	group *AuthorHistory
	day   time.Time // minuit du jour, dans le fuseau
	hits  []Hit
}

// rhythm calcule les signaux de rythme de commits (triés ou non) : par commit (R2 à R5) et par jour
// (R1, R6), et l'historique de chaque auteur. Les commits sans auteur connu sont ignorés.
func rhythm(views []store.CommitSignalView, opts RhythmOptions) ([]commitRhythm, []dayRhythm, []*AuthorHistory) {
	loc := opts.Location
	if loc == nil {
		loc = time.UTC
	}
	type key struct{ repo, author int64 }
	groups := map[key][]store.CommitSignalView{}
	var keys []key
	for _, v := range views {
		if v.AuthorID == 0 {
			continue
		}
		k := key{v.RepoID, v.AuthorID}
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], v)
	}
	slices.SortFunc(keys, func(a, b key) int { return cmp.Or(cmp.Compare(a.repo, b.repo), cmp.Compare(a.author, b.author)) })

	var commits []commitRhythm
	var days []dayRhythm
	var histories []*AuthorHistory
	for _, k := range keys {
		g := groups[k]
		slices.SortStableFunc(g, func(a, b store.CommitSignalView) int {
			return cmp.Or(a.AuthoredAt.Compare(b.AuthoredAt), cmp.Compare(a.SHA, b.SHA))
		})
		h := &AuthorHistory{RepoID: k.repo, RepoRef: g[0].RepoRef, AuthorID: k.author,
			AuthorName: g[0].AuthorName, AuthorLogin: g[0].AuthorLogin, Commits: len(g)}
		commits = append(commits, commitSignals(g, loc, opts.SpeedSignal, h)...)
		ds, worked := daySignals(g, loc, h)
		days = append(days, ds...)
		h.WorkedDays = worked
		h.Sufficient = h.lastWindowN >= MinHistory
		histories = append(histories, h)
	}
	return commits, days, histories
}

// commitSignals calcule R2 à R5 pour chaque commit d'un auteur (g trié par date).
func commitSignals(g []store.CommitSignalView, loc *time.Location, speed bool, h *AuthorHistory) []commitRhythm {
	var out []commitRhythm
	start := 0
	for i, c := range g {
		for start < i && g[start].AuthoredAt.Before(c.AuthoredAt.Add(-historySpan)) {
			start++
		}
		// Historique : commits de [t − 90 j, t), le commit lui-même et ceux de la même seconde exclus.
		var adds, files, ratios []float64
		for _, p := range g[start:i] {
			if !p.AuthoredAt.Before(c.AuthoredAt) {
				continue
			}
			adds = append(adds, float64(p.AdditionsKept))
			files = append(files, float64(p.FilesKept))
			if tot := p.AdditionsKept + p.DeletionsKept; tot > 0 {
				ratios = append(ratios, float64(p.AdditionsKept)/float64(tot))
			}
		}
		if i == len(g)-1 {
			h.lastWindowN, h.LastCommit = len(adds), c.AuthoredAt
		}
		var hits []Hit
		add := float64(c.AdditionsKept)
		if r := robustOf(adds); c.AdditionsKept > r2MinLines {
			if z, ok := r.z(add); ok && z > ZThreshold {
				hits = append(hits, statHit(SignalCommitSize, add, r.Median, z, true, true))
			}
		}
		if tot := c.AdditionsKept + c.DeletionsKept; c.AdditionsKept > r3MinLines && tot > 0 && len(ratios) >= MinHistory {
			ratio := add / float64(tot)
			if m := median(ratios); ratio > r3Ratio && m < r3MedianMax {
				hits = append(hits, statHit(SignalAddRatio, ratio, m, 0, true, false))
			}
		}
		if speed && i > 0 && c.AdditionsKept > r4MinLines {
			prev := g[i-1]
			gap := c.AuthoredAt.Sub(prev.AuthoredAt)
			if sameDay(prev.AuthoredAt, c.AuthoredAt, loc) && gap >= sameMinute {
				if v := add / gap.Minutes(); v > r4Speed {
					hits = append(hits, statHit(SignalSpeed, v, 0, 0, false, false))
				}
			}
		}
		if r := robustOf(files); c.FilesKept >= r5MinFiles {
			if z, ok := r.z(float64(c.FilesKept)); ok && z > ZThreshold {
				hits = append(hits, statHit(SignalFiles, float64(c.FilesKept), r.Median, z, true, true))
			}
		}
		if len(hits) > 0 {
			out = append(out, commitRhythm{view: c, hits: hits})
		}
	}
	return out
}

// daySignals calcule R1 et R6 pour chaque jour travaillé d'un auteur ; renvoie aussi le nombre de jours
// travaillés.
func daySignals(g []store.CommitSignalView, loc *time.Location, h *AuthorHistory) ([]dayRhythm, int) {
	net := map[time.Time]float64{}
	var worked []time.Time
	for _, c := range g {
		d := dayOf(c.AuthoredAt, loc)
		if _, ok := net[d]; !ok {
			worked = append(worked, d)
		}
		net[d] += float64(c.AdditionsKept - c.DeletionsKept)
	}
	slices.SortFunc(worked, func(a, b time.Time) int { return a.Compare(b) })

	var out []dayRhythm
	prevR6 := false
	for i, d := range worked {
		var hits []Hit
		// R1 : jours travaillés de [d − 90 j, d).
		var hist []float64
		for _, p := range worked[:i] {
			if !p.Before(d.AddDate(0, 0, -HistoryWindow)) {
				hist = append(hist, net[p])
			}
		}
		r := robustOf(hist)
		if z, ok := r.z(net[d]); ok && z > ZThreshold {
			hits = append(hits, statHit(SignalDailyLines, net[d], r.Median, z, true, true))
		}
		// R6 : [d − 13 j, d] contre les 90 jours qui précèdent.
		recentStart := d.AddDate(0, 0, -(r6RecentDays - 1))
		refStart := recentStart.AddDate(0, 0, -HistoryWindow)
		var recent, ref []float64
		for _, p := range worked[:i+1] {
			switch {
			case !p.Before(recentStart):
				recent = append(recent, net[p])
			case !p.Before(refStart):
				ref = append(ref, net[p])
			}
		}
		triggered := false
		if len(recent) >= minR6Recent && len(ref) >= MinHistory {
			if mRef := median(ref); mRef > 0 {
				mRecent := median(recent)
				if ratio := mRecent / mRef; ratio > r6RatioLimit {
					triggered = true
					if !prevR6 {
						hits = append(hits, Hit{Signal: SignalRegimeBreak, Value: formatNumber(ratio),
							Raw: ratio, Median: mRef, HasMedian: true})
					}
				}
			}
		}
		prevR6 = triggered
		if len(hits) > 0 {
			out = append(out, dayRhythm{group: h, day: d, hits: hits})
		}
	}
	return out, len(worked)
}

// statHit construit un signal de rythme avec ses valeurs brutes.
func statHit(signal string, raw, med, z float64, hasMedian, hasZ bool) Hit {
	return Hit{Signal: signal, Value: formatNumber(raw), Raw: raw, Median: med, Z: z,
		HasMedian: hasMedian, HasZ: hasZ}
}

// formatNumber écrit un nombre sans décimale s'il est entier, sinon avec deux décimales.
func formatNumber(x float64) string {
	if x == float64(int64(x)) {
		return strconv.FormatInt(int64(x), 10)
	}
	return strconv.FormatFloat(x, 'f', 2, 64)
}

// dayOf renvoie minuit du jour de t dans loc.
func dayOf(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// sameDay indique si a et b tombent le même jour dans loc.
func sameDay(a, b time.Time, loc *time.Location) bool {
	return dayOf(a, loc).Equal(dayOf(b, loc))
}
