package web

// Réglages (documentation/fr/06-interface-web.md, section 5). PROPRIÉTAIRE : agent
// contributors+settings (ce fichier, templates/settings.html, settings_test.go).
//
// Depuis la 1.8.0, les réglages sont répartis sur des pages, une par sujet, reliées par un menu :
// /settings/repos (dépôts), /settings/contributors (noms donnés aux contributeurs,
// settings_contributors.go), /settings/sync (intervalle, synchro, quota) et /settings/share (partage
// de la configuration). GET /settings reste valide : il redirige (302) vers la page du sujet du code
// msg, la query gardée (settingsSectionPath).
//
// Ce que les pages permettent, avec les mêmes vérifications que la CLI (internal/cli/repo.go) :
//   - ajouter un dépôt (POST /settings/repos) : repoimport.ParseRef, ValidateLabel,
//     ParseBranchPatterns, puis wsync.Engine.AddRepositories (accès vérifié auprès de GitHub, aucune
//     synchro lancée) ; un dépôt arrêté est repris de la même façon ;
//   - modifier le label et les branches (POST /settings/repos/{id}) ;
//   - retirer un dépôt : confirmation en GET (ne modifie rien), puis POST, en gardant l'historique
//     (suivi arrêté) ou en le supprimant (refusé pendant une synchro, qui pourrait l'écrire) ;
//   - changer l'intervalle de synchro POUR LA DURÉE DU PROCESSUS (POST /settings/interval) :
//     config.yaml n'est jamais réécrit ;
//   - « Synchroniser maintenant » (POST /sync, socle) et l'état du quota API (fragment chargé après
//     la page, GET /fragments/quota : requête gratuite /rate_limit, token redemandé à gh par le
//     moteur, jamais transmis au navigateur).
//
// Toute action est un POST (CSRF vérifié par le socle) ; la réponse est une redirection 303 vers la
// page du sujet, ?msg=<code>[&repo=<id>] (codes fixes : pas de texte venu de la requête dans l'URL), ou la
// page réaffichée avec le message d'erreur et la saisie (400, 404, 409, 422 ou 502).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/repoimport"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/textclean"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Adresses des réglages : settingsPath redirige vers la page du sujet (settingsSectionPath).
const (
	settingsPath      = "/settings"
	settingsReposPath = "/settings/repos"
	// settingsContributorsPath : noms donnés aux contributeurs (settings_contributors.go).
	settingsContributorsPath = "/settings/contributors"
	settingsSyncPath         = "/settings/sync"
	settingsSharePath        = "/settings/share"
)

// Pages des réglages (settingsView.Section), dans l'ordre du menu.
const (
	sectionRepos        = "repos"
	sectionContributors = "contributors"
	sectionSync         = "sync"
	sectionShare        = "share"
)

// settingsSection décrit une page des réglages : son code, son adresse et la clé de son titre.
type settingsSection struct {
	ID    string
	Path  string
	Title string // clé i18n
}

// settingsSections sont les pages des réglages, dans l'ordre du menu.
var settingsSections = []settingsSection{
	{sectionRepos, settingsReposPath, "web.settings.menu.repos"},
	{sectionContributors, settingsContributorsPath, "web.settings.menu.contributors"},
	{sectionSync, settingsSyncPath, "web.settings.menu.sync"},
	{sectionShare, settingsSharePath, "web.settings.menu.share"},
}

// sectionByID renvoie la page des réglages id (la page des dépôts si id est inconnu).
func sectionByID(id string) settingsSection {
	for _, sec := range settingsSections {
		if sec.ID == id {
			return sec
		}
	}
	return settingsSections[0]
}

// settingsSectionPath renvoie l'adresse de la page des réglages qui affiche le message du code msg :
// le bilan d'un import sur la page du partage, l'intervalle sur celle de la synchro, tout le reste sur
// celle des dépôts. Fonction pure.
func settingsSectionPath(msg string) string {
	switch msg {
	case msgImported:
		return settingsSharePath
	case msgInterval:
		return settingsSyncPath
	}
	return settingsReposPath
}

// quotaTimeout borne la lecture du quota (gh auth token puis GET /rate_limit).
const quotaTimeout = 10 * time.Second

// Champs des formulaires des réglages.
const (
	fieldRepo        = "repo"
	fieldLabel       = "label"
	fieldBranches    = "branches"
	fieldKeepHistory = "keep_history"
	fieldInterval    = "interval"
)

// Codes des messages affichés après une action réussie (paramètre msg de /settings).
const (
	msgAdded          = "added"
	msgUpdated        = "updated"
	msgAlready        = "already"
	msgSaved          = "saved"
	msgStopped        = "stopped"
	msgAlreadyStopped = "already_stopped"
	msgDeleted        = "deleted"
	msgInterval       = "interval"
)

// settingsView est le Content d'une page des réglages (et de la confirmation de retrait).
type settingsView struct {
	// Section est la page affichée (sectionRepos, sectionContributors, sectionSync, sectionShare) ;
	// Sections, le menu.
	Section  string
	Sections []settingsSection

	Repos      []settingsRepo
	Flash      string // message d'une action (résultat ou erreur) ; vide sinon
	FlashError bool   // Flash signale un échec

	// Formulaire d'ajout : saisie réaffichée et message après une erreur.
	Add      repoForm
	AddError string

	// Intervalle de synchro.
	Interval      time.Duration // intervalle en vigueur dans ce processus ; 0 = manuelle seule
	NextAuto      time.Time     // prochaine synchro automatique ; zéro si aucune
	FileInterval  time.Duration // sync.interval de config.yaml, relu sur le disque
	FileAuto      bool          // sync.auto de config.yaml, relu sur le disque
	FileChanged   bool          // config.yaml a changé depuis le lancement (import) : vaudra au prochain
	Overridden    bool          // l'intervalle en vigueur diffère de celui du lancement
	MinInterval   time.Duration // minimum accepté (hors 0)
	IntervalInput string        // saisie réaffichée après une erreur
	IntervalError string

	// Page des noms donnés aux contributeurs (settings_contributors.go).
	Contributors contributorLabels

	// Confirm est la confirmation de retrait d'un dépôt (GET /settings/repos/{id}/remove) ; nil sur la
	// page des réglages.
	Confirm *removeConfirm
}

// repoForm est la saisie d'un formulaire d'ajout ou de modification de dépôt.
type repoForm struct {
	Repo     string
	Label    string
	Branches string
}

// settingsRepo est un dépôt de la liste des réglages.
type settingsRepo struct {
	store.RepositorySummary
	State activity.Result
	// Edit est la saisie du formulaire de modification : valeurs actuelles, ou saisie refusée.
	Edit      repoForm
	EditError string // message d'erreur de la modification ; le formulaire est alors ouvert
}

// removeConfirm est la page de confirmation du retrait d'un dépôt.
type removeConfirm struct {
	Repo    model.Repository
	Commits int  // commits en base (supprimés sans keep_history)
	Running bool // une synchro est en cours : la suppression complète est refusée
}

// quotaView est le Content du fragment « settings-quota ».
type quotaView struct {
	Known     bool
	Limit     int
	Remaining int
	Used      int
	Reset     time.Time
	Low       bool   // garde-fou du moteur actif (wsync.QuotaLow)
	Percent   int    // part restante du quota, de 0 à 100
	Error     string // pourquoi le quota est inconnu (message du moteur, sans token)
	// Local : synchros par clones Git locaux (gh absent ou non connecté, V2) : pas de quota.
	Local bool
}

// branchesText décrit des motifs de branches pour l'affichage, dans la langue de tr : « toutes les
// branches » / "all branches", ou la liste des motifs.
func branchesText(tr *i18n.Translator, patterns []string) string {
	if len(patterns) == 0 {
		return tr.T("web.settings.all_branches")
	}
	return strings.Join(patterns, ", ")
}

// parseIntervalInput lit l'intervalle saisi : une durée Go (« 30m », « 1h30m »), un nombre entier de
// minutes (« 45 ») ou 0 (synchro manuelle seule). Les bornes (0 ou au moins 15 min) sont vérifiées par
// le coordinateur. Erreurs dans la langue de tr. Fonction pure.
func parseIntervalInput(tr *i18n.Translator, s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New(tr.T("web.interval.empty"))
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 || n > 1_000_000 {
			return 0, errors.New(tr.T("web.interval.out_of_bounds", s))
		}
		return time.Duration(n) * time.Minute, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, errors.New(tr.T("web.interval.unreadable", truncate(40, s)))
	}
	return d, nil
}

// parseRepoForm vérifie la saisie d'un formulaire de dépôt comme « watchoo repo add » : label sans
// caractère de contrôle et de longueur bornée, motifs de branches valides (vide = toutes : nil).
func parseRepoForm(tr *i18n.Translator, label, branches string) (string, []string, error) {
	if err := repoimport.ValidateLabel(label); err != nil {
		return "", nil, err
	}
	patterns, err := repoimport.ParseBranchPatterns(branches)
	if err != nil {
		return "", nil, fmt.Errorf("%s%s%w", tr.T("web.settings.branches_prefix"), tr.T("web.colon"), err)
	}
	return label, patterns, nil
}

// settingsData construit le Content de la page des réglages section : la liste des dépôts pour
// sectionRepos, l'état des synchros pour sectionSync ; sectionShare ne lit rien.
func (s *server) settingsData(r *http.Request, section string) (settingsView, error) {
	v := settingsView{Section: section, Sections: settingsSections}
	switch section {
	case sectionRepos:
		return s.settingsRepos(r, v)
	case sectionContributors:
		return s.settingsContributors(r, v)
	case sectionSync:
		return s.settingsSync(v), nil
	}
	return v, nil
}

// settingsRepos complète v avec la liste des dépôts suivis.
func (s *server) settingsRepos(r *http.Request, v settingsView) (settingsView, error) {
	summaries, err := s.store.RepositorySummaries(r.Context())
	if err != nil {
		return settingsView{}, err
	}
	now := s.now()
	v.Repos = make([]settingsRepo, 0, len(summaries))
	for _, sum := range summaries {
		v.Repos = append(v.Repos, settingsRepo{
			RepositorySummary: sum,
			State:             activity.Evaluate(sum.Repository, sum.LastRun, s.cfg.Activity, now, s.loc),
			Edit:              repoForm{Label: sum.Label, Branches: strings.Join(sum.BranchPatterns, ", ")},
		})
	}
	return v, nil
}

// settingsSync complète v avec l'intervalle de synchro (en vigueur, et relu dans config.yaml).
func (s *server) settingsSync(v settingsView) settingsView {
	v.Interval, v.NextAuto = s.sync.Interval(), s.sync.State().NextAuto
	v.FileInterval, v.FileAuto = s.cfg.Sync.Interval, s.cfg.Sync.Auto
	v.MinInterval = workspace.MinSyncInterval
	fileEffective := s.cfg.Sync.Interval
	if !s.cfg.Sync.Auto {
		fileEffective = 0
	}
	v.Overridden = v.Interval != fileEffective
	// config.yaml peut avoir été réécrit depuis le lancement (import d'une configuration partagée) :
	// la ligne « Fichier config.yaml » montre la valeur relue sur le disque, et la page signale
	// qu'elle ne vaudra qu'au prochain lancement. Fichier illisible : valeur du lancement.
	if s.ws != nil {
		if ws, err := s.shareWorkspace(); err == nil {
			v.FileInterval, v.FileAuto = ws.Config.Sync.Interval, ws.Config.Sync.Auto
			v.FileChanged = v.FileInterval != s.cfg.Sync.Interval || v.FileAuto != s.cfg.Sync.Auto
		} else {
			s.logger.Warn("relecture de config.yaml impossible", "erreur", err)
		}
	}
	return v
}

// renderSettings écrit la page des réglages v.Section avec v et le code status. Titre : « Réglages ·
// <page> ».
func (s *server) renderSettings(w http.ResponseWriter, r *http.Request, status int, v settingsView) {
	title := s.t(r, "web.nav.settings") + " · " + s.t(r, sectionByID(v.Section).Title)
	s.render(w, r, status, pageSettings, s.page(r, title, navSettings, v))
}

// settingsRedirect redirige vers la page des réglages du code de message code
// (settingsSectionPath) et, si id > 0, le dépôt concerné. L'adresse est construite ici à partir de
// valeurs fixes (pas de redirection ouverte).
func settingsRedirect(w http.ResponseWriter, r *http.Request, code string, id int64) {
	target := settingsSectionPath(code) + "?msg=" + code
	if id > 0 {
		target += "&repo=" + strconv.FormatInt(id, 10)
	}
	redirect(w, r, target)
}

// flashMessage renvoie le message du code msg dans la langue de tr, pour le dépôt repo (nom affiché,
// vide s'il est inconnu). Code inconnu : « ».
func flashMessage(tr *i18n.Translator, msg, repo string) string {
	name, which := tr.T("web.flash.the_repo"), tr.T("web.flash.this_repo")
	if repo != "" {
		name, which = tr.T("web.flash.the_repo_named", repo), tr.T("web.flash.repo_named", repo)
	}
	switch msg {
	case msgAdded:
		return tr.T("web.flash.added", name)
	case msgUpdated:
		return tr.T("web.flash.updated", name)
	case msgAlready:
		return tr.T("web.flash.already", name)
	case msgSaved:
		return tr.T("web.flash.saved", name)
	case msgStopped:
		return tr.T("web.flash.stopped", which)
	case msgAlreadyStopped:
		return tr.T("web.flash.already_stopped")
	case msgDeleted:
		return tr.T("web.flash.deleted")
	case msgInterval:
		return tr.T("web.flash.interval")
	}
	return ""
}

// handleSettings sert GET /settings : redirection (302) vers la page des réglages du code msg, la
// query gardée (favoris et anciens liens, d'avant la 1.8.0). La cible est une adresse fixe : pas de
// redirection ouverte.
func (s *server) handleSettings(w http.ResponseWriter, r *http.Request) {
	target := settingsSectionPath(queryValue(r, "msg"))
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// handleSettingsSection renvoie le handler de GET sur la page des réglages section (page complète,
// Nav navSettings). Les paramètres msg et repo affichent le résultat de la dernière action.
func (s *server) handleSettingsSection(section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.serveSettings(w, r, section) }
}

// serveSettings sert la page des réglages section.
func (s *server) serveSettings(w http.ResponseWriter, r *http.Request, section string) {
	v, err := s.settingsData(r, section)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if msg := queryValue(r, "msg"); msg != "" {
		name := ""
		if id, err := strconv.ParseInt(queryValue(r, "repo"), 10, 64); err == nil && id > 0 {
			for _, repo := range v.Repos {
				if repo.ID == id {
					name = repo.DisplayName()
				}
			}
		}
		v.Flash = flashMessage(s.tr(r), msg, textclean.Clean(name))
		if msg == msgLabelSaved || msg == msgLabelRemoved {
			v.Flash = v.Contributors.flash(s.tr(r), msg, queryValue(r, "contributor"))
		}
		if msg == msgImported {
			v.Flash, v.FlashError = importFlash(s.tr(r), r) // bilan d'un import (settings_share.go)
		}
	}
	s.renderSettings(w, r, http.StatusOK, v)
}

// handleRepoAdd sert POST /settings/repos (champs repo, label, branches). Un dépôt déjà suivi est mis
// à jour (label et branches s'ils sont donnés) et repris s'il était arrêté, comme « repo add ».
func (s *server) handleRepoAdd(w http.ResponseWriter, r *http.Request) {
	form := repoForm{Repo: formValue(r, fieldRepo), Label: formValue(r, fieldLabel), Branches: formValue(r, fieldBranches)}
	fail := func(status int, msg string) {
		v, err := s.settingsData(r, sectionRepos)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		v.Add, v.AddError = form, msg
		s.renderSettings(w, r, status, v)
	}
	ref, err := repoimport.ParseRef(form.Repo)
	if err != nil {
		// Une saisie avec « @ » peut contenir des identifiants (https://user:token@…) : elle n'est
		// pas réaffichée (ParseRef ne la cite pas non plus).
		if strings.Contains(form.Repo, "@") {
			form.Repo = ""
		}
		fail(http.StatusBadRequest, err.Error())
		return
	}
	label, patterns, err := parseRepoForm(s.tr(r), form.Label, form.Branches)
	if err != nil {
		fail(http.StatusBadRequest, err.Error())
		return
	}
	// Champ branches vide : non fourni (nil), soit toutes les branches pour un nouveau dépôt ; les
	// motifs d'un dépôt déjà suivi ne changent pas (même règle que « repo add » sans --branch).
	results, err := s.repos.AddRepositories(r.Context(),
		[]wsync.AddRequest{{Ref: ref, Label: label, BranchPatterns: patterns}}, wsync.AddOptions{})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if len(results) != 1 {
		s.serverError(w, r, fmt.Errorf("ajout du dépôt %s : résultat manquant", ref)) // journal seulement
		return
	}
	res := results[0]
	switch res.Outcome {
	case wsync.OutcomeAdded:
		settingsRedirect(w, r, msgAdded, res.Repo.ID)
	case wsync.OutcomeUpdated:
		settingsRedirect(w, r, msgUpdated, res.Repo.ID)
	case wsync.OutcomeAlreadyTracked:
		settingsRedirect(w, r, msgAlready, res.Repo.ID)
	case wsync.OutcomeInaccessible:
		fail(http.StatusUnprocessableEntity, s.t(r, "web.settings.inaccessible", ref, textclean.Clean(res.Error)))
	default:
		s.logger.Error("ajout d'un dépôt impossible", "depot", ref.String(), "erreur", res.Error)
		fail(http.StatusBadGateway, s.t(r, "web.settings.add_failed", ref))
	}
}

// handleRepoUpdate sert POST /settings/repos/{id} (champs label et branches : vide = pas de label,
// toutes les branches).
func (s *server) handleRepoUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	form := repoForm{Label: formValue(r, fieldLabel), Branches: formValue(r, fieldBranches)}
	label, patterns, err := parseRepoForm(s.tr(r), form.Label, form.Branches)
	if err != nil {
		v, lerr := s.settingsData(r, sectionRepos)
		if lerr != nil {
			s.serverError(w, r, lerr)
			return
		}
		found := false
		for i := range v.Repos {
			if v.Repos[i].ID == id {
				v.Repos[i].Edit, v.Repos[i].EditError, found = form, err.Error(), true
			}
		}
		if !found {
			s.notFound(w, r)
			return
		}
		v.Flash, v.FlashError = s.t(r, "web.settings.edit_refused", err), true
		s.renderSettings(w, r, http.StatusBadRequest, v)
		return
	}
	if _, err := s.store.UpdateRepositorySettings(r.Context(), id, label, patterns); err != nil {
		s.repoLookupError(w, r, err)
		return
	}
	settingsRedirect(w, r, msgSaved, id)
}

// handleRepoRemoveConfirm sert GET /settings/repos/{id}/remove : page de confirmation, qui ne
// modifie rien.
func (s *server) handleRepoRemoveConfirm(w http.ResponseWriter, r *http.Request) {
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	sum, err := s.store.RepositorySummary(r.Context(), id)
	if err != nil {
		s.repoLookupError(w, r, err)
		return
	}
	v := settingsView{Section: sectionRepos, Sections: settingsSections, Confirm: &removeConfirm{Repo: sum.Repository, Commits: sum.Commits, Running: s.sync.State().Running}}
	s.render(w, r, http.StatusOK, pageSettings,
		s.page(r, s.t(r, "web.remove.title", textclean.Clean(sum.DisplayName())), navSettings, v))
}

// handleRepoRemove sert POST /settings/repos/{id}/remove. Avec keep_history=1, le suivi s'arrête et
// l'historique (base et clone Git local) reste ; sinon le dépôt, son historique et son clone Git local
// sont supprimés (refusé pendant une synchro).
func (s *server) handleRepoRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	ctx := r.Context()
	repo, err := s.store.GetRepository(ctx, id)
	if err != nil {
		s.repoLookupError(w, r, err)
		return
	}
	if formValue(r, fieldKeepHistory) == "1" {
		if !repo.Enabled {
			settingsRedirect(w, r, msgAlreadyStopped, id)
			return
		}
		if err := s.store.SetRepositoryEnabled(ctx, id, false); err != nil {
			s.repoLookupError(w, r, err)
			return
		}
		settingsRedirect(w, r, msgStopped, id)
		return
	}
	if s.sync.State().Running {
		s.renderError(w, r, http.StatusConflict, s.t(r, "web.remove.impossible"), s.t(r, "web.remove.impossible_text"))
		return
	}
	reposDir := ""
	if s.ws != nil {
		reposDir = s.ws.ReposDir() // clone Git local supprimé avec le dépôt (V2)
	}
	if err := wsync.DeleteRepository(ctx, s.store, reposDir, repo); err != nil {
		s.repoLookupError(w, r, err)
		return
	}
	settingsRedirect(w, r, msgDeleted, 0)
}

// handleIntervalUpdate sert POST /settings/interval (champ interval) : change l'intervalle de la
// synchro automatique pour la durée du processus (0 ou au moins 15 min ; config.yaml n'est pas
// réécrit).
func (s *server) handleIntervalUpdate(w http.ResponseWriter, r *http.Request) {
	input := formValue(r, fieldInterval)
	d, err := parseIntervalInput(s.tr(r), input)
	if err == nil {
		err = s.sync.SetInterval(d)
	}
	if err != nil {
		if errors.Is(err, wsync.ErrCoordinatorClosed) {
			s.renderError(w, r, http.StatusConflict, s.t(r, "web.settings.change_impossible"), s.t(r, msgSyncClosed))
			return
		}
		v, lerr := s.settingsData(r, sectionSync)
		if lerr != nil {
			s.serverError(w, r, lerr)
			return
		}
		v.IntervalInput, v.IntervalError = input, err.Error()
		s.renderSettings(w, r, http.StatusBadRequest, v)
		return
	}
	settingsRedirect(w, r, msgInterval, 0)
}

// handleQuota sert GET /fragments/quota (fragment « settings-quota ») : lecture gratuite du quota
// (moteur : token redemandé à gh, jamais renvoyé). Une erreur s'affiche « quota inconnu ».
func (s *server) handleQuota(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), quotaTimeout)
	defer cancel()
	var q quotaView
	rl, err := s.repos.RateLimit(ctx)
	if err != nil {
		if errors.Is(err, provider.ErrUnsupported) {
			// Sans gh : synchros par clones Git locaux, qui n'ont pas de quota (V2).
			q.Local = true
			s.renderFragment(w, r, http.StatusOK, pageSettings, "settings-quota", s.page(r, s.t(r, "web.nav.settings"), navSettings, q))
			return
		}
		if r.Context().Err() != nil || errors.Is(err, context.Canceled) {
			// Requête abandonnée (page quittée) ou arrêt de « watchoo serve » : rien d'anormal.
			s.logger.Debug("lecture du quota GitHub interrompue", "erreur", err)
		} else {
			s.logger.Warn("quota GitHub illisible", "erreur", err)
		}
		q.Error = textclean.Clean(err.Error())
	} else {
		q = quotaView{Known: true, Limit: rl.Limit, Remaining: rl.Remaining, Used: rl.Used, Reset: rl.Reset,
			Low: wsync.QuotaLow(rl, s.now()), Percent: percent(rl.Remaining, rl.Limit)}
	}
	s.renderFragment(w, r, http.StatusOK, pageSettings, "settings-quota", s.page(r, s.t(r, "web.nav.settings"), navSettings, q))
}
