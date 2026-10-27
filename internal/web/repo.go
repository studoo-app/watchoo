package web

// Détail d'un dépôt « /repos/{id} » (documentation/06-interface-web.md, page 2). PROPRIÉTAIRE : agent
// repo (ce fichier, templates/repo.html, static/app.js, repo_test.go).
//
// La page se compose de trois fragments HTMX, rafraîchis à la fin d'une synchro
// (hx-trigger="sync-finished from:body") :
//   - « repo-overview » (id repo-overview) : en-tête (nom, owner/name lié à GitHub, pastille d'état,
//     branches, étoiles, issues, dernière synchro, bouton « Synchroniser ce dépôt »). Il est servi
//     par GET /repos/{id} quand la requête HTMX vise #repo-overview (en-tête HX-Target) ;
//   - « repo-contributors » (id repo-contributors) : auteurs des commits du dépôt, servi de même
//     quand la requête vise #repo-contributors ;
//   - « repo-pulls » (id repo-pulls, V2) : pull requests ouvertes et dernières mergées, releases et
//     tags, servi de même quand la requête vise #repo-pulls ;
//   - « repo-commits » (id repo-commits) : filtres branche et auteur, historique paginé
//     (repoCommitsPerPage par page), liens précédent / suivant. Il est servi par GET
//     /repos/{id}/commits.
//
// Historique de navigation : un lien de pagination porte hx-push-url vers la page complète
// (/repos/{id}?page=…) ; un changement de filtre (requête déclenchée par le formulaire
// #repo-filters, en-tête HX-Trigger) reçoit l'en-tête HX-Push-Url vers la page complète ; le
// rafraîchissement après une synchro ne touche pas à l'historique. Toute URL poussée est donc une
// page complète, qui se recharge telle quelle.
//
// Graphique (activity.ChartDays jours) : la série est calculée côté Go (activity.DailyCounts, dans
// le fuseau d'affichage) et écrite en JSON dans <script type="application/json" id="activity-data"> ;
// static/app.js la lit et dessine #activity-chart avec Chart.js (chargé sur cette page seulement,
// bloc « scripts »). Le graphique n'est PAS dans un fragment HTMX : avec allowScriptTags:false
// (layout.html), HTMX retire toute balise <script> d'un contenu échangé, données JSON comprises. À la
// fin d'une synchro, app.js relit donc la page complète par fetch (même origine), en extrait
// #activity-data avec DOMParser (qui n'exécute rien) et met le graphique à jour.
//
// Le graphique d'activité par contributeur (buildContributorChart, charts.go : commits par semaine sur
// activity.ContributorWeeks semaines, ceux du dépôt seulement, chaque commit une fois quelle que soit
// sa branche) suit le même principe, avec #repo-contributors-chart-data, son résumé et sa table.
//
// Aucune règle métier ici : l'état vient de activity.Evaluate, les données de internal/store.

import (
	"cmp"
	"net/http"
	"slices"
	"strings"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// repoCommitsPerPage est la taille d'une page de l'historique d'un dépôt.
const repoCommitsPerPage = 50

// Tailles des listes du fragment « repo-pulls » (V2) : dernières PR mergées, dernières releases,
// derniers tags. Les PR ouvertes sont toutes affichées.
const (
	repoMergedPulls = 10
	repoReleases    = 10
	repoTags        = 15
)

// Identifiants HTML des fragments et du formulaire de filtres (cibles HTMX, en-têtes HX-Target et
// HX-Trigger).
const (
	repoOverviewID     = "repo-overview"
	repoContributorsID = "repo-contributors"
	repoPullsID        = "repo-pulls"
	repoFiltersID      = "repo-filters"
)

// RepoView est le Content de la page de détail et de ses fragments.
type RepoView struct {
	Repo  store.RepositorySummary
	State activity.Result
	// TrackedBranches et OtherBranches : branches connues du dépôt, triées par nom.
	TrackedBranches []model.Branch
	OtherBranches   []model.Branch
	// Contributors : auteurs des commits du dépôt, du plus actif au moins actif.
	Contributors []store.ContributorStats
	// Chart : commits par jour sur ChartDays jours (JSON lu par static/app.js) ; ChartTotal et
	// ChartPeak en donnent le résumé textuel (accessibilité, absence de JavaScript).
	Chart      []activity.DayCount
	ChartDays  int
	ChartTotal int
	ChartPeak  activity.DayCount
	// ChartPeakDate : jour de ChartPeak au format « 26/10/2026 ».
	ChartPeakDate string
	// ContributorsChart : commits par semaine des contributeurs du dépôt sur activity.ContributorWeeks
	// semaines (même graphique que la page des contributeurs, restreint au dépôt).
	ContributorsChart contributorChart
	// Historique : page courante et filtres actifs.
	Commits RepoCommitsView
	// Thresholds : formulaire des seuils propres au dépôt (repo_thresholds.go).
	Thresholds thresholdsForm
	// Pulls : pull requests, releases et tags du dépôt (V2, fragment « repo-pulls »).
	Pulls RepoPullsView
}

// RepoPullsView est la partie « pull requests, releases et tags » de la page (fragment « repo-pulls »).
type RepoPullsView struct {
	Counts   store.PullRequestCounts
	Open     []store.PullRequestView // toutes les PR ouvertes, la plus récemment modifiée d'abord
	Merged   []store.PullRequestView // les repoMergedPulls dernières PR mergées
	Releases []store.ReleaseView     // les repoReleases dernières releases
	Tags     []store.TagView         // les repoTags derniers tags (commit le plus récent d'abord)
}

// RepoCommitsView est la partie « historique » de la page (fragment « repo-commits »).
type RepoCommitsView struct {
	RepoID int64
	Page   store.CommitPage
	Branch string // filtre branche actif ; vide = toutes
	Author string // filtre auteur actif (login ou nom) ; vide = tous
	// Choix du filtre branche (branches suivies, puis les autres) et du filtre auteur.
	TrackedOptions []filterOption
	OtherOptions   []filterOption
	Authors        []filterOption
	// AuthorIgnored : un filtre auteur qui ressemblait à une adresse e-mail a été ignoré (aucune
	// adresse ne doit apparaître dans une URL, l'historique du navigateur ou la page).
	AuthorIgnored bool
}

// Filtered indique qu'un filtre est actif.
func (v RepoCommitsView) Filtered() bool { return v.Branch != "" || v.Author != "" }

// filterOption est un choix d'un <select> de filtre.
type filterOption struct {
	Value string
	Label string
}

// handleRepo sert GET /repos/{id} : la page complète, ou le fragment « repo-overview » ou
// « repo-contributors » pour une requête HTMX qui le vise (en-tête HX-Target : rafraîchissement après
// une synchro).
func (s *server) handleRepo(w http.ResponseWriter, r *http.Request) {
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	view, err := s.repoView(r, id, true)
	if err != nil {
		s.repoLookupError(w, r, err)
		return
	}
	data := s.page(r, view.Repo.DisplayName(), "", view)
	if isHTMX(r) && !isHistoryRestore(r) {
		switch target := r.Header.Get("HX-Target"); target {
		case repoOverviewID, repoContributorsID, repoPullsID:
			s.renderFragment(w, r, http.StatusOK, pageRepo, target, data)
			return
		}
	}
	s.render(w, r, http.StatusOK, pageRepo, data)
}

// handleRepoCommits sert GET /repos/{id}/commits : le fragment « repo-commits » pour une requête
// HTMX (pagination, filtres, rafraîchissement), sinon la page complète avec les mêmes paramètres
// (adresse ouverte à la main ou rechargée).
func (s *server) handleRepoCommits(w http.ResponseWriter, r *http.Request) {
	if !isHTMX(r) || isHistoryRestore(r) {
		s.handleRepo(w, r)
		return
	}
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	view, err := s.repoView(r, id, false)
	if err != nil {
		s.repoLookupError(w, r, err)
		return
	}
	if r.Header.Get("HX-Trigger") == repoFiltersID {
		// Changement de filtre : l'adresse du navigateur suit, vers la page complète.
		c := view.Commits
		if u, err := withQuery(repoPath(id), "branch", c.Branch, "author", c.Author, "page", pageNumber(c.Page)); err == nil {
			w.Header().Set("HX-Push-Url", u)
		}
	}
	data := s.page(r, view.Repo.DisplayName(), "", view)
	s.renderFragment(w, r, http.StatusOK, pageRepo, "repo-commits", data)
}

// repoView lit les données de la page du dépôt id. Avec overview, elle lit aussi le graphique et les
// contributeurs (inutiles au seul fragment de l'historique). L'erreur store.ErrNotFound signale un
// dépôt inconnu.
func (s *server) repoView(r *http.Request, id int64, overview bool) (RepoView, error) {
	ctx := r.Context()
	sum, err := s.store.RepositorySummary(ctx, id)
	if err != nil {
		return RepoView{}, err
	}
	now := s.now()
	view := RepoView{
		Repo:       sum,
		State:      activity.Evaluate(sum.Repository, sum.LastRun, s.cfg.Activity, now, s.loc),
		Thresholds: s.newThresholdsForm(sum.FlemmeAfterDays, sum.DeadAfterDays),
	}
	view.Thresholds.Saved = queryValue(r, "msg") == msgThresholds

	branches, err := s.store.ListBranches(ctx, id)
	if err != nil {
		return RepoView{}, err
	}
	for _, b := range branches {
		if b.Tracked {
			view.TrackedBranches = append(view.TrackedBranches, b)
		} else {
			view.OtherBranches = append(view.OtherBranches, b)
		}
	}
	contributors, err := s.store.ContributorRanking(ctx, store.ContributorFilter{RepoID: id})
	if err != nil {
		return RepoView{}, err
	}
	if overview {
		view.Contributors = contributors
		view.ChartDays = activity.ChartDays
		times, err := s.store.CommitTimes(ctx, id, activity.SeriesStart(now, activity.ChartDays, s.loc))
		if err != nil {
			return RepoView{}, err
		}
		chart := newDailyChart(times, now, activity.ChartDays, s.loc)
		view.Chart, view.ChartTotal, view.ChartPeak, view.ChartPeakDate = chart.Series, chart.Total, chart.Peak, chart.PeakDate
		authors, err := s.store.AuthorCommitTimes(ctx, id, activity.WeeksStart(now, activity.ContributorWeeks, s.loc))
		if err != nil {
			return RepoView{}, err
		}
		view.ContributorsChart = buildContributorChart(contributors, authors, now, activity.ContributorWeeks,
			contributorChartTop, s.loc)
		if view.Pulls, err = s.repoPulls(r, id); err != nil {
			return RepoView{}, err
		}
	}

	c := RepoCommitsView{RepoID: id, Branch: queryValue(r, "branch"), Author: queryValue(r, "author")}
	if strings.Contains(c.Author, "@") {
		c.Author, c.AuthorIgnored = "", true
	}
	c.TrackedOptions, c.OtherOptions = branchOptions(view.TrackedBranches, view.OtherBranches, c.Branch)
	c.Authors = authorOptions(contributors, c.Author)
	c.Page, err = s.store.CommitPage(ctx, store.CommitFilter{RepoID: id, Branch: c.Branch, Author: c.Author},
		pageParam(r), repoCommitsPerPage)
	if err != nil {
		return RepoView{}, err
	}
	view.Commits = c
	return view, nil
}

// repoPulls lit les pull requests, releases et tags du dépôt id (fragment « repo-pulls »).
func (s *server) repoPulls(r *http.Request, id int64) (RepoPullsView, error) {
	ctx := r.Context()
	var v RepoPullsView
	var err error
	if v.Counts, err = s.store.PullRequestCounts(ctx, id); err != nil {
		return v, err
	}
	if v.Open, err = s.store.ListPullRequests(ctx, store.PullRequestFilter{RepoID: id,
		Status: model.PullRequestStatusOpen}); err != nil {
		return v, err
	}
	if v.Merged, err = s.store.ListPullRequests(ctx, store.PullRequestFilter{RepoID: id,
		Status: model.PullRequestStatusMerged, Limit: repoMergedPulls}); err != nil {
		return v, err
	}
	if v.Releases, err = s.store.ListReleases(ctx, store.ReleaseFilter{RepoID: id, Limit: repoReleases}); err != nil {
		return v, err
	}
	if v.Tags, err = s.store.ListTags(ctx, store.ReleaseFilter{RepoID: id, Limit: repoTags}); err != nil {
		return v, err
	}
	return v, nil
}

// branchOptions renvoie les choix du filtre branche : branches suivies, puis les autres. La branche
// sélectionnée est ajoutée aux autres si elle n'est pas (ou plus) connue, pour que le filtre affiché
// reste celui appliqué.
func branchOptions(tracked, other []model.Branch, selected string) (trackedOpts, otherOpts []filterOption) {
	found := selected == ""
	convert := func(branches []model.Branch) []filterOption {
		opts := make([]filterOption, 0, len(branches)+1)
		for _, b := range branches {
			opts = append(opts, filterOption{Value: b.Name, Label: b.Name})
			found = found || b.Name == selected
		}
		return opts
	}
	trackedOpts, otherOpts = convert(tracked), convert(other)
	if !found {
		otherOpts = append(otherOpts, filterOption{Value: selected, Label: selected + " (inconnue)"})
	}
	return trackedOpts, otherOpts
}

// authorOptions renvoie les choix du filtre auteur, triés par nom affiché : valeur = login de forge
// s'il est connu, sinon nom Git (jamais d'e-mail). Doublons de valeur retirés ; l'auteur sélectionné
// est ajouté s'il n'est pas dans la liste.
func authorOptions(contributors []store.ContributorStats, selected string) []filterOption {
	opts := make([]filterOption, 0, len(contributors)+1)
	seen := make(map[string]bool, len(contributors))
	for _, c := range contributors {
		value, label := c.ProviderLogin, c.DisplayName()
		if value == "" {
			value = c.Name
		} else if c.Name != "" && !strings.EqualFold(c.Name, value) {
			label = c.Name + " (" + value + ")"
		}
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		opts = append(opts, filterOption{Value: value, Label: label})
	}
	slices.SortStableFunc(opts, func(a, b filterOption) int {
		return cmp.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label))
	})
	if selected != "" && !seen[strings.ToLower(selected)] {
		opts = append(opts, filterOption{Value: selected, Label: selected + " (inconnu)"})
	}
	return opts
}

// pageNumber renvoie le numéro de page à mettre dans une URL : 0 (omis par withQuery) pour la
// première.
func pageNumber(p store.CommitPage) int {
	if p.Page <= 1 {
		return 0
	}
	return p.Page
}

// isHistoryRestore indique une requête HTMX de restauration d'historique (bouton « précédent »
// sans copie en cache) : elle attend la page complète.
func isHistoryRestore(r *http.Request) bool {
	return r.Header.Get("HX-History-Restore-Request") == "true"
}
