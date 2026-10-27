package activity

// Série d'activité par jour (graphique de la page de détail d'un dépôt, interface web).

import "time"

// ChartDays est la durée par défaut du graphique d'activité de l'interface web, en jours.
const ChartDays = 90

// DayCount est le nombre de commits d'un jour calendaire du fuseau d'affichage.
type DayCount struct {
	Date  string `json:"date"`  // jour au format 2006-01-02, dans le fuseau d'affichage
	Count int    `json:"count"` // commits de ce jour
}

// SeriesStart renvoie le début (minuit, dans loc) du premier des days jours qui se terminent par le
// jour de now dans loc (nil = time.Local), en UTC : la borne « since » à passer à la lecture des dates
// de commit (store.CommitTimes) pour DailyCounts(…, now, days, loc). days < 1 est ramené à 1.
func SeriesStart(now time.Time, days int, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	days = max(days, 1)
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d-(days-1), 0, 0, 0, 0, loc).UTC()
}

// DailyCounts compte les commits de times par jour calendaire dans loc (nil = time.Local), sur les
// days jours qui se terminent par le jour de now (days < 1 ramené à 1) : exactement days entrées, du
// plus ancien au plus récent, jours sans commit compris (Count 0). Les dates hors de cette fenêtre
// (avant le premier jour, ou après le jour de now) sont ignorées. Fonction pure : le résultat est
// sérialisé en JSON par l'interface web pour Chart.js.
func DailyCounts(times []time.Time, now time.Time, days int, loc *time.Location) []DayCount {
	if loc == nil {
		loc = time.Local
	}
	days = max(days, 1)
	y, m, d := now.In(loc).Date()
	out := make([]DayCount, days)
	index := make(map[string]int, days)
	for i := range days {
		// time.Date normalise les jours hors du mois ; midi évite tout effet des changements d'heure.
		day := time.Date(y, m, d-(days-1-i), 12, 0, 0, 0, loc).Format(time.DateOnly)
		out[i] = DayCount{Date: day}
		index[day] = i
	}
	for _, t := range times {
		if i, ok := index[t.In(loc).Format(time.DateOnly)]; ok {
			out[i].Count++
		}
	}
	return out
}
