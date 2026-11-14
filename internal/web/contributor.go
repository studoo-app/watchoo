package web

// Page d'un contributeur « /contributors/{id} » (documentation/fr/06-interface-web.md, page 4, 0.5.0) :
// sa fiche et ses totaux, ses commits par jour et par semaine (une série par dépôt), ses langages
// (1.2.0), ses dépôts, ses commits paginés et ses pull requests.
//
// Fragments HTMX, rafraîchis à la fin d'une synchro (hx-trigger="sync-finished from:body") :
//   - « contributor-overview », « contributor-languages » (1.2.0), « contributor-repos »,
//     « contributor-pulls » : servis par GET /contributors/{id} quand la requête HTMX les vise (en-tête
//     HX-Target) ;
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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/diagnostic"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// contributorCommitsPerPage est la taille d'une page de l'historique d'un contributeur (comme celui
// d'un dépôt).
const contributorCommitsPerPage = repoCommitsPerPage

// contributorPulls est le nombre de pull requests affichées sur la page d'un contributeur (les plus
// récemment modifiées).
const contributorPulls = 20

// contributorLanguagesShown est le nombre de langages de la carte « Langages » d'un contributeur ; les
// suivants sont regroupés dans « Autres langages ».
const contributorLanguagesShown = 8

// Identifiants HTML des fragments de la page (cibles HTMX, en-tête HX-Target).
const (
	contributorOverviewID  = "contributor-overview"
	contributorLanguagesID = "contributor-languages"
	contributorReposID     = "contributor-repos"
	contributorPullsID     = "contributor-pulls"
)

// ContributorView est le Content de la page d'un contributeur et de ses fragments.
type ContributorView struct {
	Profile store.ContributorProfile
	// Languages : ses langages (fragment « contributor-languages », 1.2.0).
	Languages contributorLanguages
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
	// Accounts : comptes rattachés à ce contributeur principal (1.9.0) ; vide si aucun.
	Accounts []store.AttachedAccount
	// Diagnostic : le diagnostic est activé ; Diag porte alors son score IA, le même que dans la
	// liste des contributeurs : pistes de ses commits (auteur) et de ses pull requests (login).
	Diagnostic bool
	Diag       diagnostic.Summary
	// Conventions : ligne « Commits conformes n/m (x %) », tous dépôts confondus (1.7.0,
	// repo_conventions.go) ; nil si les conventions sont désactivées ou hors de la page complète.
	Conventions *contributorConventions
}

// contributorLanguages est la carte « Langages » d'un contributeur : les contributorLanguagesShown
// langages qui ont le plus de lignes ajoutées, puis le reste regroupé.
type contributorLanguages struct {
	Totals model.LanguageTotals
	Shown  []languageShare
	Others languageShare // Lines = 0 : aucun langage regroupé
}

// languageShare est un langage de la carte, avec sa part des lignes ajoutées en pour mille.
type languageShare struct {
	Language string
	Lines    int
	Permille int
}

// newContributorLanguages découpe les langages de t pour la carte.
func newContributorLanguages(t model.LanguageTotals) contributorLanguages {
	out := contributorLanguages{Totals: t}
	for i, l := range t.Lines {
		if i < contributorLanguagesShown {
			out.Shown = append(out.Shown, languageShare{Language: l.Language, Lines: l.Lines, Permille: t.Share(l)})
			continue
		}
		out.Others.Lines += l.Lines
	}
	if out.Others.Lines > 0 {
		out.Others.Permille = model.Permille(int64(out.Others.Lines), int64(t.Total()))
	}
	return out
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
	// Leads : pistes de réflexion des commits de la page (diagnostic activé), par clé commitLeadKey.
	Leads map[string]commitLead
}

// commitLead signale un commit qui porte une piste de réflexion : son niveau et la fiche de revue.
type commitLead struct {
	Level string // diagnostic.LevelDeclared, LevelProbable ou LevelToReview
	Path  string // fiche de revue (/diagnostic/lead)
}

// commitLeadKey est la clé d'un commit dans Leads : dépôt et SHA (deux dépôts issus d'un même modèle
// peuvent partager des SHA).
func commitLeadKey(repoID int64, sha string) string {
	return strconv.FormatInt(repoID, 10) + "/" + sha
}

// Lead renvoie la piste du commit c, nil s'il n'en porte pas (ou si le diagnostic est désactivé).
func (v ContributorCommitsView) Lead(c store.CommitView) *commitLead {
	if l, ok := v.Leads[commitLeadKey(c.RepoID, c.SHA)]; ok {
		return &l
	}
	return nil
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
	if s.redirectToPrincipal(w, r, id) {
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
		case contributorOverviewID, contributorLanguagesID, contributorReposID, contributorPullsID:
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
	if s.redirectToPrincipal(w, r, id) {
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
		Languages: newContributorLanguages(profile.Languages), Commits: ContributorCommitsView{ContributorID: id}}
	accounts, err := s.store.AttachedAccounts(ctx)
	if err != nil {
		return ContributorView{}, err
	}
	view.Accounts = accounts[id]
	for _, a := range view.Accounts {
		view.HasAccount = view.HasAccount || a.ProviderUserID > 0
	}
	if view.Commits.Page, err = s.store.CommitPage(ctx, store.CommitFilter{AuthorID: id}, pageParam(r),
		contributorCommitsPerPage); err != nil {
		return ContributorView{}, err
	}
	if err := s.contributorDiagnostic(r, &view); err != nil {
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
	if view.Conventions, err = s.contributorConventionsOf(r, id); err != nil {
		return ContributorView{}, err
	}

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
	view.Daily = newDailyChart(s.tr(r), plain, now, activity.ChartDays, s.loc)
	view.Weekly = buildRepoChart(s.tr(r), repos, times, now, activity.ContributorWeeks, contributorChartTop, s.loc)

	if view.HasAccount {
		if view.Pulls, err = s.personPulls(r, profile.Contributor, view.Accounts); err != nil {
			return ContributorView{}, err
		}
	}
	return view, nil
}

// contributorDiagnostic renseigne, si le diagnostic est activé, le score IA du contributeur (même
// calcul que la liste, contributorScores, pour la seule personne de la page) et les pistes des commits
// de la page courante de son historique (une seule lecture du diagnostic).
func (s *server) contributorDiagnostic(r *http.Request, v *ContributorView) error {
	if v.Diagnostic = s.cfg.Diagnostic.Enabled; !v.Diagnostic {
		return nil
	}
	rep, err := diagnostic.Build(r.Context(), s.store, store.DiagnosticFilter{}, s.rhythmOptions())
	if err != nil {
		return err
	}
	logins := map[string]int64{}
	if login := v.Profile.ProviderLogin; login != "" {
		logins[strings.ToLower(login)] = v.Profile.ID
	}
	for _, a := range v.Accounts { // comptes rattachés (1.9.0) : leurs pull requests comptent pour la personne
		if a.ProviderLogin != "" {
			logins[strings.ToLower(a.ProviderLogin)] = v.Profile.ID
		}
	}
	v.Diag = diagnostic.SummarizeByContributor(rep.Leads, logins)[v.Profile.ID]

	shown := make(map[string]bool, len(v.Commits.Page.Commits))
	for _, c := range v.Commits.Page.Commits {
		shown[commitLeadKey(c.RepoID, c.SHA)] = true
	}
	v.Commits.Leads = map[string]commitLead{}
	for _, l := range rep.Leads {
		if key := commitLeadKey(l.RepoID, l.SHA); l.Kind == diagnostic.KindCommit && shown[key] {
			v.Commits.Leads[key] = commitLead{Level: l.Level, Path: leadPath(l.RepoID, l.Key())}
		}
	}
	return nil
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

// redirectToPrincipal redirige (302) la page d'un compte rattaché (1.9.0) vers celle de son principal,
// la query gardée, et renvoie true ; false si id est un principal ou s'il est inconnu (la page répond
// alors elle-même, 404 compris).
func (s *server) redirectToPrincipal(w http.ResponseWriter, r *http.Request, id int64) bool {
	primary, err := s.store.PrimaryOf(r.Context(), id)
	if err != nil || primary == id {
		return false
	}
	target := strings.Replace(r.URL.Path, contributorPath(id), contributorPath(primary), 1)
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusFound)
	return true
}

// personPulls renvoie les pull requests d'une personne (1.9.0) : celles du compte principal et de ses
// comptes rattachés connus de la forge, de la plus récemment modifiée à la plus ancienne, au plus
// contributorPulls.
func (s *server) personPulls(r *http.Request, principal model.Contributor, accounts []store.AttachedAccount) ([]store.PullRequestView, error) {
	ids := []int64{}
	if principal.ProviderUserID > 0 {
		ids = append(ids, principal.ProviderUserID)
	}
	for _, a := range accounts {
		if a.ProviderUserID > 0 {
			ids = append(ids, a.ProviderUserID)
		}
	}
	var out []store.PullRequestView
	for _, pid := range ids {
		pulls, err := s.store.ListPullRequests(r.Context(), store.PullRequestFilter{AuthorProviderID: pid, Limit: contributorPulls})
		if err != nil {
			return nil, err
		}
		out = append(out, pulls...)
	}
	if len(ids) > 1 {
		slices.SortStableFunc(out, func(a, b store.PullRequestView) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
		if len(out) > contributorPulls {
			out = out[:contributorPulls]
		}
	}
	return out, nil
}
