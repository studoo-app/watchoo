package activity

// Formats courts communs à la CLI et au TUI (documentation/fr/05-cli.md).

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// Short renvoie l'affichage court de l'état, comme la colonne ÉTAT de « watchoo repo list » :
//   - Active : « - » ; Flemme : « ~ 9 j » ; Dead : « ⚠ 42 j » (« ! 42 j » si unicode est faux) ;
//   - états gris : « arrêté », « erreur », « jamais », « archivé », « vide » (sans accent si unicode
//     est faux : « arrete », « archive »).
func (r Result) Short(unicode bool) string {
	switch r.State {
	case StateActive:
		return "-"
	case StateFlemme:
		return fmt.Sprintf("~ %d j", r.Days)
	case StateDead:
		if unicode {
			return fmt.Sprintf("⚠ %d j", r.Days)
		}
		return fmt.Sprintf("! %d j", r.Days)
	case StateStopped:
		return pickText(unicode, "arrêté", "arrete")
	case StateError:
		return "erreur"
	case StateNeverSynced:
		return "jamais"
	case StateArchived:
		return pickText(unicode, "archivé", "archive")
	case StateEmpty:
		return "vide"
	default:
		return string(r.State)
	}
}

// pickText renvoie withAccents si unicode est vrai, sinon ascii.
func pickText(unicode bool, withAccents, ascii string) string {
	if unicode {
		return withAccents
	}
	return ascii
}

// Relative renvoie la date t relative à now, en français, pour l'affichage : t zéro → « jamais » ;
// moins d'une minute (ou t dans le futur) → « à l'instant » ; moins d'une heure → « il y a 5 min » ;
// moins de 24 h → « il y a 2 h » ; sinon « il y a 9 j » en jours calendaires dans loc (CalendarDays,
// pour concorder avec Short). unicode faux : accents retirés (« a l'instant »).
func Relative(t, now time.Time, loc *time.Location, unicode bool) string {
	if t.IsZero() {
		return "jamais"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return pickText(unicode, "à l'instant", "a l'instant")
	case d < time.Hour:
		return fmt.Sprintf("il y a %d min", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("il y a %d h", int(d/time.Hour))
	default:
		// Au moins 24 h d'écart : au moins un jour calendaire, quel que soit le fuseau.
		return fmt.Sprintf("il y a %d j", max(CalendarDays(t, now, loc), 1))
	}
}

// ParseSince analyse une durée relative d'option --since : un entier strictement positif suivi d'une
// unité, « d » (jours de 24 h), « h » (heures) ou « m » (minutes), ex. « 7d », « 30d », « 12h ». Toute
// autre forme donne une erreur en français qui cite la valeur et les formes acceptées.
func ParseSince(s string) (time.Duration, error) {
	invalid := func() (time.Duration, error) {
		return 0, fmt.Errorf("--since %q invalide : attendu un nombre entier positif suivi de d (jours), "+
			"h (heures) ou m (minutes), par exemple 7d, 12h ou 30m", s)
	}
	if len(s) < 2 {
		return invalid()
	}
	var unit time.Duration
	switch s[len(s)-1] {
	case 'd':
		unit = 24 * time.Hour
	case 'h':
		unit = time.Hour
	case 'm':
		unit = time.Minute
	default:
		return invalid()
	}
	digits := s[:len(s)-1]
	for _, c := range digits {
		if c < '0' || c > '9' {
			return invalid() // refuse aussi les signes, espaces et décimales
		}
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || n <= 0 || n > int64(math.MaxInt64/unit) {
		return invalid()
	}
	return time.Duration(n) * unit, nil
}

// sparkLevels sont les huit hauteurs de Sparkline, de la plus basse à la plus haute.
var sparkLevels = []rune("▁▂▃▄▅▆▇█")

// sparkASCII sont les hauteurs de Sparkline sans Unicode : aucun commit, peu, moyen, beaucoup.
var sparkASCII = []rune(".-=#")

// Sparkline renvoie une courbe d'une ligne, un caractère par valeur de counts, de hauteur
// proportionnelle au maximum de counts : « ▁ » à « █ » ; sans Unicode (unicode faux), « . », « - »,
// « = », « # ». Une valeur nulle donne toujours le caractère le plus bas, une valeur non nulle jamais.
// Fonction pure (TUI).
func Sparkline(counts []int, unicode bool) string {
	levels := sparkLevels
	if !unicode {
		levels = sparkASCII
	}
	peak := 0
	for _, n := range counts {
		peak = max(peak, n)
	}
	out := make([]rune, len(counts))
	for i, n := range counts {
		level := 0
		if n > 0 && peak > 0 {
			// 1 … len(levels)-1 : une semaine active ne se confond jamais avec une semaine vide.
			level = 1 + (n*(len(levels)-1)-1)/peak
			level = min(max(level, 1), len(levels)-1)
		}
		out[i] = levels[level]
	}
	return string(out)
}
