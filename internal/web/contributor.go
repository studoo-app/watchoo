package web

// Page d'un contributeur « /contributors/{id} » (documentation/fr/06-interface-web.md, page 4, 0.5.0) :
// sa fiche et ses totaux, ses commits par jour et par semaine (une série par dépôt), ses dépôts, ses
// commits paginés et ses pull requests.
//
// Fragments HTMX, rafraîchis à la fin d'une synchro (hx-trigger="sync-finished from:body") :
//   - « contributor-overview », « contributor-repos », « contributor-pulls » : servis par GET
//     /contributors/{id} quand la requête HTMX les vise (en-tête HX-Target) ;
//   - « contributor-commits » : historique paginé (contributorCommitsPerPage par page), servi par GET
//     /contributors/{id}/commits (page complète hors HTMX). Un lien de pagination pousse l'adresse de
//     la page complète (/contributors/{id}?page=…), qui se recharge telle quelle.
//
// Les deux graphiques (dailyChart et buildRepoChart, charts.go) sont hors des fragments, comme sur la
// page d'un dépôt : static/app.js les dessine et relit la page à la fin d'une synchro.
//
// Aucune règle métier ici : les totaux viennent de internal/store, les états des dépôts de
// activity.Evaluate. Vie privée : seul le NOMBRE d'adresses e-mail est lu (ContributorProfile) ;
// aucune adresse n'est lue ni affichée, et rien de la page n'est journalisé (le journal d'accès
// n'écrit que le motif de route). Le lien de profil est l'adresse donnée par le provider et
// enregistrée en base : rien de propre à une forge n'est construit ici.

import (
	"net/http"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/store"
)

// contributorCommitsPerPage est la taille d'une page de l'historique d'un contributeur (comme celui
// d'un dépôt).
const contributorCommitsPerPage = repoCommitsPerPage

// contributorPulls est le nombre de pull requests affichées sur la page d'un contributeur (les plus
// récemment modifiées).
const contributorPulls = 20

// Identifiants HTML des fragments de la page (cibles HTMX, en-tête HX-Target).
const (
	contributorOverviewID = "contributor-overview"
	contributorReposID    = "contributor-repos"
	contributorPullsID    = "contributor-pulls"
)

// ContributorView est le Content de la page d'un contributeur et de ses fragments.
type ContributorView struct {
	Profile store.ContributorProfile
	// Repos : ses dépôts, commits décroissants puis dernier commit le plus récent.
	Repos []contributorRepoRow
	// Daily : ses commits par jour sur activity.ChartDays jours.
	Daily dailyChart
	// Weekly : ses commits par semaine et par dépôt sur activity.ContributorWeeks semaines.
	Weekly contributorChart
	// Commits : page courante de son historique.
	Commits ContributorCommitsView
	// HasAccount : compte de forge connu ; Pulls n'a de sens qu'alors (rapprochement par
	// pull_requests.author_provider_id).
	HasAccount bool
	Pulls      []store.PullRequestView
}

// contributorRepoRow est un dépôt de la page d'un contributeur : ses commits dans ce dépôt et l'état
// du dépôt.
type contributorRepoRow struct {
	store.ContributorRepoStats
	State    activity.Result
	HasState bool // faux si le dépôt n'a pas été retrouvé (supprimé entre deux lectures)
}

// ContributorCommitsView est la partie « historique » de la page (fragment « contributor-commits »).
type ContributorCommitsView struct {
	ContributorID int64
	Page          store.CommitPage
}

// ShowLogin indique s'il faut afficher le login à côté du nom (il existe et n'est pas déjà le nom).
func (v ContributorView) ShowLogin() bool {
	return v.Profile.ProviderLogin != "" && v.Profile.ProviderLogin != v.Profile.DisplayName()
}

// handleContributor sert GET /contributors/{id} : la page complète, ou un fragment pour une requête
// HTMX qui le vise (en-tête HX-Target : rafraîchissement après une synchro).
func (s *server) handleContributor(w http.ResponseWriter, r *http.Request) {
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	view, err := s.contributorView(r, id, true)
	if err != nil {
		s.repoLookupError(w, r, err)
		return
	}
	data := s.page(r, view.Profile.DisplayName(), navContributors, view)
	if isHTMX(r) && !isHistoryRestore(r) {
		switch target := r.Header.Get("HX-Target"); target {
		case contributorOverviewID, contributorReposID, contributorPullsID:
			s.renderFragment(w, r, http.StatusOK, pageContributor, target, data)
			return
		}
	}
	s.render(w, r, http.StatusOK, pageContributor, data)
}

// handleContributorCommits sert GET /contributors/{id}/commits : le fragment « contributor-commits »
// pour une requête HTMX (pagination, rafraîchissement), sinon la page complète.
func (s *server) handleContributorCommits(w http.ResponseWriter, r *http.Request) {
	if !isHTMX(r) || isHistoryRestore(r) {
		s.handleContributor(w, r)
		return
	}
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	view, err := s.contributorView(r, id, false)
	if err != nil {
		s.repoLookupError(w, r, err)
		return
	}
	s.renderFragment(w, r, http.StatusOK, pageContributor, "contributor-commits",
		s.page(r, view.Profile.DisplayName(), navContributors, view))
}

// contributorView lit les données de la page du contributeur id. Avec full, elle lit aussi ses dépôts,
// ses graphiques et ses pull requests (inutiles au seul fragment de l'historique). L'erreur
// store.ErrNotFound signale un contributeur inconnu.
func (s *server) contributorView(r *http.Request, id int64, full bool) (ContributorView, error) {
	ctx := r.Context()
	profile, err := s.store.ContributorProfile(ctx, id)
	if err != nil {
		return ContributorView{}, err
	}
	view := ContributorView{Profile: profile, HasAccount: profile.ProviderUserID > 0,
		Commits: ContributorCommitsView{ContributorID: id}}
	if view.Commits.Page, err = s.store.CommitPage(ctx, store.CommitFilter{AuthorID: id}, pageParam(r),
		contributorCommitsPerPage); err != nil {
		return ContributorView{}, err
	}
	if !full {
		return view, nil
	}

	now := s.now()
	repos, err := s.store.ContributorRepos(ctx, id)
	if err != nil {
		return ContributorView{}, err
	}
	summaries, err := s.store.RepositorySummaries(ctx)
	if err != nil {
		return ContributorView{}, err
	}
	view.Repos = s.contributorRepoRows(repos, summaries, now)

	// Une seule lecture pour les deux graphiques : depuis le plus ancien de leurs débuts.
	since := activity.SeriesStart(now, activity.ChartDays, s.loc)
	if w := activity.WeeksStart(now, activity.ContributorWeeks, s.loc); w.Before(since) {
		since = w
	}
	times, err := s.store.ContributorCommitTimes(ctx, id, since)
	if err != nil {
		return ContributorView{}, err
	}
	plain := make([]time.Time, 0, len(times))
	for _, t := range times {
		plain = append(plain, t.CommittedAt)
	}
	view.Daily = newDailyChart(plain, now, activity.ChartDays, s.loc)
	view.Weekly = buildRepoChart(repos, times, now, activity.ContributorWeeks, contributorChartTop, s.loc)

	if view.HasAccount {
		if view.Pulls, err = s.store.ListPullRequests(ctx, store.PullRequestFilter{
			AuthorProviderID: profile.ProviderUserID, Limit: contributorPulls}); err != nil {
			return ContributorView{}, err
		}
	}
	return view, nil
}

// contributorRepoRows associe à chaque dépôt du contributeur l'état du dépôt (activity.Evaluate, avec
// ses seuils) ; un dépôt absent des résumés (supprimé entre les deux lectures) garde un état zéro.
// L'ordre de repos est gardé.
func (s *server) contributorRepoRows(repos []store.ContributorRepoStats, summaries []store.RepositorySummary,
	now time.Time) []contributorRepoRow {
	byID := make(map[int64]store.RepositorySummary, len(summaries))
	for _, sum := range summaries {
		byID[sum.ID] = sum
	}
	rows := make([]contributorRepoRow, 0, len(repos))
	for _, rs := range repos {
		row := contributorRepoRow{ContributorRepoStats: rs}
		if sum, ok := byID[rs.RepoID]; ok {
			row.State = activity.Evaluate(sum.Repository, sum.LastRun, s.cfg.Activity, now, s.loc)
			row.HasState = true
		}
		rows = append(rows, row)
	}
	return rows
}
