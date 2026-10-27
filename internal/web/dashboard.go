package web

// Tableau de bord « / » (documentation/06-interface-web.md, page 1). PROPRIÉTAIRE : agent dashboard
// (ce fichier, templates/dashboard.html, dashboard_test.go).
//
// La page montre :
//   - les compteurs par état (DaisyUI stat) : Actif, Flemme, Dead et états gris ;
//   - une carte par dépôt : nom affiché, pastille d'état (activity.Evaluate, comme la CLI), dernier
//     commit relatif et son auteur ; triées par sortDashboardRepos (Dead et Flemme d'abord) ;
//   - le fil des 20 derniers commits, tous dépôts confondus (jamais d'e-mail) ;
//   - le graphique d'activité de l'espace : commits par jour sur activity.ChartDays jours, tous dépôts
//     confondus (charts.go, dessiné par static/app.js).
//
// Le graphique est hors des fragments (HTMX retire les <script> des fragments) : à la fin d'une
// synchro, app.js relit la page complète et en extrait les données. Les deux autres blocs sont des
// fragments HTMX (« dashboard-repos » et « dashboard-feed ») rafraîchis à la
// fin d'une synchro (hx-trigger="sync-finished from:body"). Aucune règle métier ici : les états
// viennent de internal/activity, les données de internal/store.

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// dashboardFeedSize est le nombre de commits du fil du tableau de bord.
const dashboardFeedSize = 20

// DashboardView est le Content de la page et de ses fragments.
type DashboardView struct {
	Repos  []DashboardRepo    // cartes, dans l'ordre d'affichage (sortDashboardRepos)
	Counts DashboardCounts    // compteurs par état
	Feed   []store.CommitView // derniers commits, du plus récent au plus ancien
	// Thresholds sont les seuils de l'espace (activity.*), rappelés sous les compteurs ; un dépôt
	// peut avoir les siens (repository_settings), appliqués par activity.Evaluate.
	Thresholds activity.Thresholds
	// Activity : commits par jour de l'espace (page complète seulement, pas les fragments).
	Activity dailyChart
}

// DashboardRepo est une carte du tableau de bord : le résumé du dépôt et son état calculé.
type DashboardRepo struct {
	store.RepositorySummary
	State activity.Result
}

// Accent renvoie la classe de la bordure gauche de la carte, de la couleur de la pastille. Classes
// écrites en toutes lettres : Tailwind lit ce fichier (@source de tailwind/input.css).
func (d DashboardRepo) Accent() string {
	switch d.State.State {
	case activity.StateActive:
		return "border-success"
	case activity.StateFlemme:
		return "border-warning"
	case activity.StateDead:
		return "border-error"
	default:
		return "border-base-300"
	}
}

// DashboardCounts compte les dépôts par état ; Gray regroupe tous les états gris.
type DashboardCounts struct {
	Total, Active, Flemme, Dead, Gray int
}

// countStates compte les dépôts de repos par état.
func countStates(repos []DashboardRepo) DashboardCounts {
	c := DashboardCounts{Total: len(repos)}
	for _, r := range repos {
		switch st := r.State.State; {
		case st == activity.StateActive:
			c.Active++
		case st == activity.StateFlemme:
			c.Flemme++
		case st == activity.StateDead:
			c.Dead++
		default:
			c.Gray++
		}
	}
	return c
}

// stateRank donne le rang d'affichage d'un état : Dead, puis Flemme, puis Actif, puis les états
// gris (en erreur d'abord, pour qu'un dépôt qui échoue se voie).
func stateRank(st activity.State) int {
	switch st {
	case activity.StateDead:
		return 0
	case activity.StateFlemme:
		return 1
	case activity.StateActive:
		return 2
	case activity.StateError:
		return 3
	default:
		return 4
	}
}

// sortDashboardRepos trie les cartes (fonction pure, tri stable) : par rang d'état (stateRank) ;
// dans Dead, Flemme et Actif, le dernier commit le plus ancien d'abord (le plus en retard en tête) ;
// puis par nom affiché (sans tenir compte de la casse) et par identifiant.
func sortDashboardRepos(repos []DashboardRepo) {
	slices.SortStableFunc(repos, func(a, b DashboardRepo) int {
		if c := cmp.Compare(stateRank(a.State.State), stateRank(b.State.State)); c != 0 {
			return c
		}
		if !a.State.State.Gray() {
			if c := a.LastCommitAt.Compare(b.LastCommitAt); c != 0 {
				return c
			}
		}
		if c := strings.Compare(strings.ToLower(a.DisplayName()), strings.ToLower(b.DisplayName())); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

// dashboardThresholds renvoie les seuils d'activité de l'espace (ceux d'un dépôt sans réglage propre).
func (s *server) dashboardThresholds() activity.Thresholds {
	return activity.ThresholdsFor(model.Repository{}, s.cfg.Activity)
}

// dashboardRepos lit les résumés des dépôts, calcule leur état et les trie.
func (s *server) dashboardRepos(ctx context.Context) ([]DashboardRepo, error) {
	summaries, err := s.store.RepositorySummaries(ctx)
	if err != nil {
		return nil, fmt.Errorf("tableau de bord : %w", err)
	}
	now := s.now()
	repos := make([]DashboardRepo, 0, len(summaries))
	for _, sum := range summaries {
		repos = append(repos, DashboardRepo{
			RepositorySummary: sum,
			State:             activity.Evaluate(sum.Repository, sum.LastRun, s.cfg.Activity, now, s.loc),
		})
	}
	sortDashboardRepos(repos)
	return repos, nil
}

// dashboardFeed lit les derniers commits, tous dépôts confondus.
func (s *server) dashboardFeed(ctx context.Context) ([]store.CommitView, error) {
	feed, err := s.store.ListRecentCommits(ctx, store.CommitFilter{Limit: dashboardFeedSize})
	if err != nil {
		return nil, fmt.Errorf("fil du tableau de bord : %w", err)
	}
	return feed, nil
}

// handleDashboard sert GET / (page complète, Nav navDashboard, titre « Tableau de bord »).
func (s *server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	repos, err := s.dashboardRepos(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	feed, err := s.dashboardFeed(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	now := s.now()
	times, err := s.store.CommitTimes(r.Context(), 0, activity.SeriesStart(now, activity.ChartDays, s.loc))
	if err != nil {
		s.serverError(w, r, fmt.Errorf("activité du tableau de bord : %w", err))
		return
	}
	view := DashboardView{Repos: repos, Counts: countStates(repos), Feed: feed, Thresholds: s.dashboardThresholds(),
		Activity: newDailyChart(times, now, activity.ChartDays, s.loc)}
	s.render(w, r, http.StatusOK, pageDashboard, s.page(r, "Tableau de bord", navDashboard, view))
}

// handleDashboardRepos sert GET /fragments/dashboard/repos (fragment « dashboard-repos » : compteurs
// et cartes).
func (s *server) handleDashboardRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := s.dashboardRepos(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	view := DashboardView{Repos: repos, Counts: countStates(repos), Thresholds: s.dashboardThresholds()}
	s.renderFragment(w, r, http.StatusOK, pageDashboard, "dashboard-repos",
		s.page(r, "Tableau de bord", navDashboard, view))
}

// handleDashboardFeed sert GET /fragments/dashboard/feed (fragment « dashboard-feed »).
func (s *server) handleDashboardFeed(w http.ResponseWriter, r *http.Request) {
	feed, err := s.dashboardFeed(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.renderFragment(w, r, http.StatusOK, pageDashboard, "dashboard-feed",
		s.page(r, "Tableau de bord", navDashboard, DashboardView{Feed: feed}))
}
