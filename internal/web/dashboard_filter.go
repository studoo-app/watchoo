package web

// Tri et filtres des cartes du tableau de bord (1.7.0, documentation/fr/06-interface-web.md, page 1).
// Les choix viennent de la query string de « / » ou du fragment « dashboard-repos » (?sort=, ?state=,
// ?q=, ?warnings=, ?prs=) ; une valeur inconnue est ignorée. Fonctions pures : l'état des dépôts vient de
// internal/activity, le tri et le filtre ne font que réordonner et masquer des cartes. Pas de tri par
// score IA : le score ne classe jamais les dépôts (12-supervision-diagnostic.md).

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/i18n"
)

// Valeurs de ?sort= (stables : elles sont dans les adresses mises en favori).
const (
	sortAttention = "attention" // par défaut : sortDashboardRepos
	sortName      = "name"
	sortRecent    = "recent"
	sortCommits   = "commits"
	sortLines     = "lines"
)

// dashboardSorts est l'ordre des choix du tri dans la liste.
var dashboardSorts = []string{sortAttention, sortName, sortRecent, sortCommits, sortLines}

// Valeurs de ?state= (stables).
const (
	filterStateActive = "active"
	filterStateFlemme = "flemme"
	filterStateDead   = "dead"
	filterStateOther  = "other" // tous les états gris
)

// dashboardStates est l'ordre des choix du filtre d'état.
var dashboardStates = []string{filterStateActive, filterStateFlemme, filterStateDead, filterStateOther}

// Identifiants HTML du formulaire (en-tête HX-Trigger) et du fragment des cartes seules (en-tête
// HX-Target).
const (
	dashboardFiltersID = "dashboard-filters"
	dashboardCardsID   = "dashboard-cards"
)

// DashboardFilter est le tri et les filtres choisis. Sa valeur zéro est la vue par défaut (tri « à
// surveiller », aucun filtre).
type DashboardFilter struct {
	Sort     string // une valeur de dashboardSorts ; vide = sortAttention
	State    string // une valeur de dashboardStates ; vide = tous
	Query    string // texte cherché dans le nom affiché et owner/repo ; vide = tous
	Warnings bool   // seulement les dépôts avec avertissements ou en erreur
	PRs      bool   // seulement les dépôts avec au moins une pull request ouverte
}

// sortOptions renvoie les choix de la liste du tri, dans la langue de tr.
func sortOptions(tr *i18n.Translator) []filterOption {
	opts := make([]filterOption, 0, len(dashboardSorts))
	for _, v := range dashboardSorts {
		opts = append(opts, filterOption{Value: v, Label: tr.T("web.dash.sort." + v)})
	}
	return opts
}

// stateOptions renvoie les choix du filtre d'état, dans la langue de tr (libellés des pastilles ;
// « Autres » pour les états gris).
func stateOptions(tr *i18n.Translator) []filterOption {
	return []filterOption{
		{Value: filterStateActive, Label: activity.StateActive.LabelIn(tr)},
		{Value: filterStateFlemme, Label: activity.StateFlemme.LabelIn(tr)},
		{Value: filterStateDead, Label: activity.StateDead.LabelIn(tr)},
		{Value: filterStateOther, Label: tr.T("web.dash.stat_other")},
	}
}

// parseDashboardFilter lit les choix de la query string ; une valeur inconnue est ignorée.
func parseDashboardFilter(get func(string) string) DashboardFilter {
	f := DashboardFilter{Query: get("q"), Warnings: get("warnings") == "1", PRs: get("prs") == "1"}
	if s := get("sort"); slices.Contains(dashboardSorts, s) && s != sortAttention {
		f.Sort = s
	}
	if s := get("state"); slices.Contains(dashboardStates, s) {
		f.State = s
	}
	return f
}

// SortValue renvoie le tri actif (sortAttention par défaut).
func (f DashboardFilter) SortValue() string { return cmp.Or(f.Sort, sortAttention) }

// Filtered indique qu'au moins un filtre est actif (le tri n'en est pas un).
func (f DashboardFilter) Filtered() bool {
	return f.State != "" || f.Query != "" || f.Warnings || f.PRs
}

// Changed indique que la vue n'est pas celle par défaut (tri ou filtre).
func (f DashboardFilter) Changed() bool { return f.Sort != "" || f.Filtered() }

// pairs renvoie les paramètres non vides, pour withQuery.
func (f DashboardFilter) pairs() []any {
	flag := func(b bool) string {
		if b {
			return "1"
		}
		return ""
	}
	return []any{"sort", f.Sort, "state", f.State, "q", f.Query, "warnings", flag(f.Warnings), "prs", flag(f.PRs)}
}

// PagePath renvoie l'adresse de « / » avec ces choix (barre d'adresse, favori).
func (f DashboardFilter) PagePath() string {
	u, err := withQuery("/", f.pairs()...)
	if err != nil {
		return "/"
	}
	return u
}

// FragmentPath renvoie l'adresse du fragment « dashboard-repos » avec ces choix (rafraîchissement de fin
// de synchro).
func (f DashboardFilter) FragmentPath() string {
	const base = "/fragments/dashboard/repos"
	u, err := withQuery(base, f.pairs()...)
	if err != nil {
		return base
	}
	return u
}

// StatePath renvoie l'adresse de « / » filtrée sur state, ou sans filtre d'état si state est déjà
// actif (clic sur un compteur : un second clic retire le filtre).
func (f DashboardFilter) StatePath(state string) string {
	if f.State == state {
		f.State = ""
	} else {
		f.State = state
	}
	return f.PagePath()
}

// ClearPath renvoie l'adresse de « / » sans filtre, en gardant le tri.
func (f DashboardFilter) ClearPath() string {
	return DashboardFilter{Sort: f.Sort}.PagePath()
}

// stateGroup renvoie la valeur de ?state= qui correspond à st.
func stateGroup(st activity.State) string {
	switch st {
	case activity.StateActive:
		return filterStateActive
	case activity.StateFlemme:
		return filterStateFlemme
	case activity.StateDead:
		return filterStateDead
	}
	return filterStateOther
}

// matches indique si la carte r passe les filtres de f.
func (f DashboardFilter) matches(r DashboardRepo) bool {
	if f.State != "" && stateGroup(r.State.State) != f.State {
		return false
	}
	if f.Warnings && len(r.LastRun.Warnings) == 0 && r.State.State != activity.StateError {
		return false
	}
	if f.PRs && r.OpenPullRequests == 0 {
		return false
	}
	if f.Query != "" {
		q := strings.ToLower(f.Query)
		if !strings.Contains(strings.ToLower(r.DisplayName()), q) && !strings.Contains(strings.ToLower(r.FullName()), q) {
			return false
		}
	}
	return true
}

// filterDashboardRepos renvoie les cartes de repos qui passent les filtres de f, dans le même ordre.
func filterDashboardRepos(repos []DashboardRepo, f DashboardFilter) []DashboardRepo {
	if !f.Filtered() {
		return repos
	}
	out := make([]DashboardRepo, 0, len(repos))
	for _, r := range repos {
		if f.matches(r) {
			out = append(out, r)
		}
	}
	return out
}

// sortDashboardReposBy trie les cartes selon sort (tri stable) ; égalités départagées par nom affiché
// (ordre naturel), puis par identifiant.
func sortDashboardReposBy(repos []DashboardRepo, sort string) {
	byName := func(a, b DashboardRepo) int {
		if c := naturalCompare(strings.ToLower(a.DisplayName()), strings.ToLower(b.DisplayName())); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	}
	var primary func(a, b DashboardRepo) int
	switch sort {
	case sortName:
		primary = func(DashboardRepo, DashboardRepo) int { return 0 }
	case sortRecent:
		primary = func(a, b DashboardRepo) int {
			// Sans commit (date zéro) à la fin.
			if c := cmp.Compare(boolRank(a.LastCommitAt.IsZero()), boolRank(b.LastCommitAt.IsZero())); c != 0 {
				return c
			}
			return b.LastCommitAt.Compare(a.LastCommitAt)
		}
	case sortCommits:
		primary = func(a, b DashboardRepo) int { return cmp.Compare(b.Commits, a.Commits) }
	case sortLines:
		primary = func(a, b DashboardRepo) int {
			if c := cmp.Compare(boolRank(!a.Lines.Known()), boolRank(!b.Lines.Known())); c != 0 {
				return c
			}
			return cmp.Compare(b.Lines.Additions, a.Lines.Additions)
		}
	default:
		sortDashboardRepos(repos)
		return
	}
	slices.SortStableFunc(repos, func(a, b DashboardRepo) int {
		if c := primary(a, b); c != 0 {
			return c
		}
		return byName(a, b)
	})
}

// boolRank renvoie 1 pour true, 0 pour false (ce qui vaut true va à la fin).
func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// naturalCompare compare a et b en lisant les suites de chiffres comme des nombres (« gr2 » < « gr10 »).
func naturalCompare(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		if unicode.IsDigit(ra[i]) && unicode.IsDigit(rb[j]) {
			si, sj := i, j
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if c := cmp.Compare(len(na), len(nb)); c != 0 {
				return c
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			continue
		}
		if c := cmp.Compare(ra[i], rb[j]); c != 0 {
			return c
		}
		i++
		j++
	}
	return cmp.Compare(len(ra)-i, len(rb)-j)
}
