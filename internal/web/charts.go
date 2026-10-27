package web

// Graphiques d'activité de l'interface (Chart.js, dessinés par static/app.js) : données calculées ici
// par des fonctions pures à partir des séries de internal/activity, transmises en JSON dans un
// <script type="application/json"> (jamais dans du code JavaScript : CSP stricte).
//
//   - détail d'un dépôt et tableau de bord : commits par jour sur activity.ChartDays jours
//     (dailyChart) ;
//   - contributeurs : commits par semaine sur activity.ContributorWeeks semaines, une série par
//     contributeur parmi les contributorChartTop plus actifs de la période, les autres regroupés dans
//     « Autres contributeurs » (contributorChart).
//
// Les jours et les semaines (du lundi au dimanche) suivent le fuseau d'affichage de l'espace.

import (
	"cmp"
	"slices"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/store"
)

// contributorChartTop est le nombre de contributeurs qui ont leur propre série dans le graphique des
// contributeurs : 8, le nombre de couleurs de la palette catégorielle de static/app.js (au-delà, les
// couleurs ne se distinguent plus) ; les suivants sont regroupés dans « Autres contributeurs ».
const contributorChartTop = 8

// contributorNameMax borne la longueur (en caractères) d'un nom dans la légende du graphique.
const contributorNameMax = 40

// otherContributorsLabel est le nom de la série qui regroupe les contributeurs hors du classement du
// graphique.
const otherContributorsLabel = "Autres contributeurs"

// dailyChart est un graphique des commits par jour et son résumé textuel (accessibilité, absence de
// JavaScript).
type dailyChart struct {
	Series   []activity.DayCount // JSON lu par app.js : [{date, count}, …]
	Days     int                 // durée, en jours
	Total    int                 // commits de la période
	Peak     activity.DayCount   // jour le plus chargé (Count 0 si aucun commit)
	PeakDate string              // jour de Peak au format « 26/10/2026 » ; vide si aucun commit
}

// newDailyChart compte times par jour sur days jours qui se terminent par le jour de now, dans loc.
// Fonction pure.
func newDailyChart(times []time.Time, now time.Time, days int, loc *time.Location) dailyChart {
	c := dailyChart{Series: activity.DailyCounts(times, now, days, loc), Days: max(days, 1)}
	c.Total, c.Peak = activity.Summarize(c.Series)
	if c.Peak.Count > 0 {
		if t, err := time.Parse(time.DateOnly, c.Peak.Date); err == nil {
			c.PeakDate = t.Format("02/01/2006")
		}
	}
	return c
}

// contributorChartData est le JSON du graphique des contributeurs, lu par app.js.
type contributorChartData struct {
	Weeks  []string            `json:"weeks"`  // lundis des semaines (2006-01-02), du plus ancien au plus récent
	Series []contributorSeries `json:"series"` // dans l'ordre d'empilement et de la légende
}

// contributorSeries est la série d'un contributeur (ou des « Autres contributeurs »).
type contributorSeries struct {
	Name   string `json:"name"`            // nom affiché, nettoyé et tronqué ; jamais un e-mail
	Counts []int  `json:"counts"`          // commits par semaine, alignés sur Weeks
	Total  int    `json:"total"`           // commits de la période
	Other  bool   `json:"other,omitempty"` // série des autres contributeurs (couleur neutre)
}

// contributorChart est le graphique des contributeurs et son résumé textuel.
type contributorChart struct {
	Data   contributorChartData
	Weeks  int    // durée, en semaines
	Total  int    // commits d'auteur connu de la période
	Active int    // contributeurs avec au moins un commit sur la période
	Top    int    // nombre maximal de contributeurs qui ont leur propre série
	Start  string // premier jour de la période, au format « 03/08/2026 »
	// WeekLabels : lundis des semaines au format « 26/10 », pour la table des données (vue sans
	// graphique : couleurs, lecteurs d'écran).
	WeekLabels []string
}

// buildContributorChart compte les commits de times (auteur et date) par contributeur et par semaine
// sur les weeks semaines qui se terminent par celle de now, dans loc. Les top contributeurs les plus
// actifs de la période (au moins un commit ; égalités départagées par l'ordre de rows, le classement
// global) ont chacun leur série, de la plus fournie à la moins fournie ; les autres, et les auteurs
// absents de rows, sont regroupés dans une dernière série « Autres contributeurs » (si elle n'est pas
// vide). Fonction pure.
func buildContributorChart(rows []contributorRow, times []store.AuthorCommitTime, now time.Time, weeks, top int,
	loc *time.Location) contributorChart {
	keys := activity.WeekKeys(now, weeks, loc)
	index := make(map[string]int, len(keys))
	for i, k := range keys {
		index[k] = i
	}
	top = max(top, 0)
	c := contributorChart{Weeks: len(keys), Top: top, Data: contributorChartData{Weeks: keys, Series: []contributorSeries{}}}
	for i, k := range keys {
		t, err := time.Parse(time.DateOnly, k)
		if err != nil {
			continue
		}
		if i == 0 {
			c.Start = t.Format("02/01/2006")
		}
		c.WeekLabels = append(c.WeekLabels, t.Format("02/01"))
	}

	counts := make(map[int64][]int)
	for _, at := range times {
		i, ok := index[activity.WeekOf(at.CommittedAt, loc)]
		if !ok {
			continue
		}
		if counts[at.ContributorID] == nil {
			counts[at.ContributorID] = make([]int, len(keys))
		}
		counts[at.ContributorID][i]++
		c.Total++
	}
	c.Active = len(counts)

	type candidate struct {
		rank   int // position dans rows
		series contributorSeries
	}
	var candidates []candidate
	for i, row := range rows {
		per, ok := counts[row.ID]
		if !ok {
			continue
		}
		delete(counts, row.ID)
		s := contributorSeries{Name: truncate(contributorNameMax, row.DisplayName()), Counts: per}
		for _, n := range per {
			s.Total += n
		}
		candidates = append(candidates, candidate{rank: i, series: s})
	}
	slices.SortStableFunc(candidates, func(a, b candidate) int {
		if d := cmp.Compare(b.series.Total, a.series.Total); d != 0 {
			return d
		}
		return cmp.Compare(a.rank, b.rank)
	})

	other := contributorSeries{Name: otherContributorsLabel, Counts: make([]int, len(keys)), Other: true}
	add := func(per []int) {
		for i, n := range per {
			other.Counts[i] += n
			other.Total += n
		}
	}
	for i, cand := range candidates {
		if i < top {
			c.Data.Series = append(c.Data.Series, cand.series)
		} else {
			add(cand.series.Counts)
		}
	}
	// Auteurs absents du classement (ajoutés entre les deux lectures, par une synchro en cours).
	for _, per := range counts {
		add(per)
	}
	if other.Total > 0 {
		c.Data.Series = append(c.Data.Series, other)
	}
	return c
}

// TableColumns renvoie le nombre de colonnes de la table des données : contributeur, une par semaine,
// total.
func (c contributorChart) TableColumns() int { return len(c.WeekLabels) + 2 }
