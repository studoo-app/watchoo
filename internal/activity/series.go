package activity

// Séries d'activité : commits par jour (graphiques d'un dépôt et du tableau de bord, interface web),
// par semaine, et par semaine pour chaque contributeur (graphiques des contributeurs d'un espace ou
// d'un dépôt, TUI, « watchoo contributors --weeks »).

import (
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

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

// Summarize renvoie le total des commits de series et son jour le plus chargé (le premier en cas
// d'égalité ; zéro si aucun commit). Fonction pure : résumé textuel d'un graphique (accessibilité,
// absence de JavaScript).
func Summarize(series []DayCount) (total int, peak DayCount) {
	for _, d := range series {
		total += d.Count
		if d.Count > peak.Count {
			peak = d
		}
	}
	return total, peak
}

// ContributorWeeks est la durée des séries d'activité par contributeur, en semaines : graphiques de
// l'interface web (contributeurs, détail d'un dépôt) et détail d'un dépôt dans le TUI.
const ContributorWeeks = 12

// WeekCount est le nombre de commits d'une semaine calendaire (du lundi au dimanche) du fuseau
// d'affichage.
type WeekCount struct {
	Week  string `json:"week"`  // lundi de la semaine au format 2006-01-02, dans le fuseau d'affichage
	Count int    `json:"count"` // commits de cette semaine
}

// mondayOf renvoie l'année, le mois et le jour du lundi de la semaine de t (déjà dans son fuseau).
func mondayOf(t time.Time) (int, time.Month, int) {
	y, m, d := t.Date()
	offset := (int(t.Weekday()) + 6) % 7 // lundi = 0 … dimanche = 6
	return y, m, d - offset
}

// WeeksStart renvoie le début (lundi à minuit, dans loc) de la première des weeks semaines qui se
// terminent par la semaine de now dans loc (nil = time.Local), en UTC : la borne « since » de
// WeeklyCounts(…, now, weeks, loc). weeks < 1 est ramené à 1.
func WeeksStart(now time.Time, weeks int, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	weeks = max(weeks, 1)
	y, m, d := mondayOf(now.In(loc))
	return time.Date(y, m, d-7*(weeks-1), 0, 0, 0, 0, loc).UTC()
}

// WeekKeys renvoie les lundis (format 2006-01-02, dans loc, nil = time.Local) des weeks semaines qui
// se terminent par la semaine de now (weeks < 1 ramené à 1), du plus ancien au plus récent.
func WeekKeys(now time.Time, weeks int, loc *time.Location) []string {
	if loc == nil {
		loc = time.Local
	}
	weeks = max(weeks, 1)
	y, m, d := mondayOf(now.In(loc))
	out := make([]string, weeks)
	for i := range weeks {
		// Midi évite tout effet des changements d'heure ; time.Date normalise les jours hors du mois.
		out[i] = time.Date(y, m, d-7*(weeks-1-i), 12, 0, 0, 0, loc).Format(time.DateOnly)
	}
	return out
}

// WeekOf renvoie le lundi (format 2006-01-02) de la semaine de t dans loc (nil = time.Local).
func WeekOf(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	y, m, d := mondayOf(t.In(loc))
	return time.Date(y, m, d, 12, 0, 0, 0, loc).Format(time.DateOnly)
}

// WeeklyCounts compte les commits de times par semaine calendaire (lundi → dimanche) dans loc
// (nil = time.Local), sur les weeks semaines qui se terminent par la semaine de now (weeks < 1 ramené
// à 1) : exactement weeks entrées, de la plus ancienne à la plus récente, semaines sans commit
// comprises. Les dates hors de cette fenêtre sont ignorées. Fonction pure.
func WeeklyCounts(times []time.Time, now time.Time, weeks int, loc *time.Location) []WeekCount {
	keys := WeekKeys(now, weeks, loc)
	out := make([]WeekCount, len(keys))
	index := make(map[string]int, len(keys))
	for i, k := range keys {
		out[i] = WeekCount{Week: k}
		index[k] = i
	}
	for _, t := range times {
		if i, ok := index[WeekOf(t, loc)]; ok {
			out[i].Count++
		}
	}
	return out
}

// ContributorWeekCounts est l'activité par semaine de chaque contributeur sur une fenêtre de semaines
// (CountContributorWeeks), ou de chaque dépôt d'un contributeur (CountRepoWeeks : les clés sont alors
// des identifiants de dépôt).
type ContributorWeekCounts struct {
	Weeks []string // lundis des semaines (2006-01-02), de la plus ancienne à la plus récente (WeekKeys)
	// Counts : commits par semaine de chaque contributeur qui a au moins un commit dans la fenêtre,
	// alignés sur Weeks.
	Counts map[int64][]int
	Total  int // commits de la fenêtre, tous contributeurs confondus
}

// Of renvoie les commits par semaine du contributeur id, alignés sur Weeks (que des zéros s'il n'a
// aucun commit dans la fenêtre). La tranche renvoyée est une copie.
func (c ContributorWeekCounts) Of(id int64) []int {
	out := make([]int, len(c.Weeks))
	copy(out, c.Counts[id])
	return out
}

// TotalOf renvoie les commits du contributeur id dans la fenêtre.
func (c ContributorWeekCounts) TotalOf(id int64) int {
	n := 0
	for _, v := range c.Counts[id] {
		n += v
	}
	return n
}

// CountContributorWeeks compte les commits de times par contributeur et par semaine calendaire (lundi
// → dimanche) dans loc (nil = time.Local), sur les weeks semaines qui se terminent par la semaine de
// now (weeks < 1 ramené à 1). Les dates hors de cette fenêtre sont ignorées. Lire times avec la borne
// WeeksStart(now, weeks, loc). Fonction pure.
func CountContributorWeeks(times []model.AuthorCommitTime, now time.Time, weeks int,
	loc *time.Location) ContributorWeekCounts {
	return countKeyedWeeks(len(times), func(i int) (int64, time.Time) {
		return times[i].ContributorID, times[i].CommittedAt
	}, now, weeks, loc)
}

// CountRepoWeeks compte les commits de times par dépôt et par semaine calendaire, avec les mêmes règles
// que CountContributorWeeks : le résultat a la même forme, ses clés sont des identifiants de dépôt
// (page d'un contributeur, 0.5.0 : ses commits par dépôt). Lire times avec la borne
// WeeksStart(now, weeks, loc). Fonction pure.
func CountRepoWeeks(times []model.RepoCommitTime, now time.Time, weeks int, loc *time.Location) ContributorWeekCounts {
	return countKeyedWeeks(len(times), func(i int) (int64, time.Time) {
		return times[i].RepoID, times[i].CommittedAt
	}, now, weeks, loc)
}

// countKeyedWeeks compte n dates (at(i) : clé et date) par clé et par semaine calendaire.
func countKeyedWeeks(n int, at func(i int) (int64, time.Time), now time.Time, weeks int,
	loc *time.Location) ContributorWeekCounts {
	keys := WeekKeys(now, weeks, loc)
	index := make(map[string]int, len(keys))
	for i, k := range keys {
		index[k] = i
	}
	c := ContributorWeekCounts{Weeks: keys, Counts: make(map[int64][]int)}
	for j := range n {
		id, t := at(j)
		i, ok := index[WeekOf(t, loc)]
		if !ok {
			continue
		}
		if c.Counts[id] == nil {
			c.Counts[id] = make([]int, len(keys))
		}
		c.Counts[id][i]++
		c.Total++
	}
	return c
}
