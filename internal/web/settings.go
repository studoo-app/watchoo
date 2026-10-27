package web

// Réglages « /settings » (documentation/06-interface-web.md, page 4). PROPRIÉTAIRE : agent
// contributors+settings (ce fichier, templates/settings.html, settings_test.go).
//
// Ce que la page permet, avec les mêmes vérifications que la CLI (internal/cli/repo.go) :
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
// Toute action est un POST (CSRF vérifié par le socle) ; la réponse est une redirection 303 vers
// /settings?msg=<code>[&repo=<id>] (codes fixes : pas de texte venu de la requête dans l'URL), ou la
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
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/repoimport"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/textclean"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// settingsPath est l'adresse de la page des réglages (cible des redirections).
const settingsPath = "/settings"

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

// settingsView est le Content de la page des réglages (et de la confirmation de retrait).
type settingsView struct {
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

// BranchesText décrit les branches suivies : « toutes les branches » ou la liste des motifs.
func (r settingsRepo) BranchesText() string {
	return branchesText(r.BranchPatterns)
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
	Error     string // pourquoi le quota est inconnu (en français, sans token)
	// Local : synchros par clones Git locaux (gh absent ou non connecté, V2) : pas de quota.
	Local bool
}

// branchesText décrit des motifs de branches pour l'affichage.
func branchesText(patterns []string) string {
	if len(patterns) == 0 {
		return "toutes les branches"
	}
	return strings.Join(patterns, ", ")
}

// parseIntervalInput lit l'intervalle saisi : une durée Go (« 30m », « 1h30m »), un nombre entier de
// minutes (« 45 ») ou 0 (synchro manuelle seule). Les bornes (0 ou au moins 15 min) sont vérifiées par
// le coordinateur. Fonction pure.
func parseIntervalInput(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("intervalle vide : indiquez par exemple 30m, 1h ou 0 pour la synchro manuelle seule")
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 || n > 1_000_000 {
			return 0, fmt.Errorf("intervalle %q invalide : nombre de minutes hors des bornes", s)
		}
		return time.Duration(n) * time.Minute, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("intervalle %q illisible : indiquez une durée comme 30m, 1h ou 1h30m, ou 0 pour la synchro manuelle seule",
			truncate(40, s))
	}
	return d, nil
}

// parseRepoForm vérifie la saisie d'un formulaire de dépôt comme « watchoo repo add » : label sans
// caractère de contrôle et de longueur bornée, motifs de branches valides (vide = toutes : nil).
func parseRepoForm(label, branches string) (string, []string, error) {
	if err := repoimport.ValidateLabel(label); err != nil {
		return "", nil, err
	}
	patterns, err := repoimport.ParseBranchPatterns(branches)
	if err != nil {
		return "", nil, fmt.Errorf("branches : %w", err)
	}
	return label, patterns, nil
}

// settingsData lit la base et l'état des synchros, et construit le Content de la page.
func (s *server) settingsData(r *http.Request) (settingsView, error) {
	summaries, err := s.store.RepositorySummaries(r.Context())
	if err != nil {
		return settingsView{}, err
	}
	now := s.now()
	st := s.sync.State()
	v := settingsView{
		Repos:        make([]settingsRepo, 0, len(summaries)),
		Interval:     s.sync.Interval(),
		NextAuto:     st.NextAuto,
		FileInterval: s.cfg.Sync.Interval,
		FileAuto:     s.cfg.Sync.Auto,
		MinInterval:  workspace.MinSyncInterval,
	}
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
	for _, sum := range summaries {
		v.Repos = append(v.Repos, settingsRepo{
			RepositorySummary: sum,
			State:             activity.Evaluate(sum.Repository, sum.LastRun, s.cfg.Activity, now, s.loc),
			Edit:              repoForm{Label: sum.Label, Branches: strings.Join(sum.BranchPatterns, ", ")},
		})
	}
	return v, nil
}

// renderSettings écrit la page des réglages avec v et le code status.
func (s *server) renderSettings(w http.ResponseWriter, r *http.Request, status int, v settingsView) {
	s.render(w, r, status, pageSettings, s.page(r, "Réglages", navSettings, v))
}

// settingsRedirect redirige vers /settings avec le code de message code et, si id > 0, le dépôt
// concerné. L'adresse est construite ici à partir de valeurs fixes (pas de redirection ouverte).
func settingsRedirect(w http.ResponseWriter, r *http.Request, code string, id int64) {
	target := settingsPath + "?msg=" + code
	if id > 0 {
		target += "&repo=" + strconv.FormatInt(id, 10)
	}
	redirect(w, r, target)
}

// flashMessage renvoie le message du code msg, pour le dépôt repo (nom affiché, vide s'il est
// inconnu). Code inconnu : « ».
func flashMessage(msg, repo string) string {
	name, which := "Le dépôt", "ce dépôt"
	if repo != "" {
		name, which = "Le dépôt « "+repo+" »", "le dépôt « "+repo+" »"
	}
	switch msg {
	case msgAdded:
		return name + " est suivi. Il sera synchronisé à la prochaine synchro ; « Synchroniser maintenant » la lance tout de suite."
	case msgUpdated:
		return name + " était déjà suivi : ses réglages sont mis à jour (et son suivi repris s'il était arrêté)."
	case msgAlready:
		return name + " est déjà suivi : rien à changer."
	case msgSaved:
		return name + " est modifié. Les nouvelles branches seront prises en compte à la prochaine synchro."
	case msgStopped:
		return "Suivi arrêté pour " + which + " ; son historique est gardé. « Reprendre le suivi » le relance."
	case msgAlreadyStopped:
		return "Le suivi de ce dépôt était déjà arrêté : rien n'a changé."
	case msgDeleted:
		return "Le dépôt et son historique sont supprimés de la base. Rien n'a été modifié sur GitHub."
	case msgInterval:
		return "Intervalle de synchro changé pour ce processus. config.yaml n'est pas modifié."
	}
	return ""
}

// handleSettings sert GET /settings (page complète, Nav navSettings, titre « Réglages »). Les
// paramètres msg et repo affichent le résultat de la dernière action.
func (s *server) handleSettings(w http.ResponseWriter, r *http.Request) {
	v, err := s.settingsData(r)
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
		v.Flash = flashMessage(msg, textclean.Clean(name))
		if msg == msgImported {
			v.Flash, v.FlashError = importFlash(r) // bilan d'un import (settings_share.go)
		}
	}
	s.renderSettings(w, r, http.StatusOK, v)
}

// handleRepoAdd sert POST /settings/repos (champs repo, label, branches). Un dépôt déjà suivi est mis
// à jour (label et branches s'ils sont donnés) et repris s'il était arrêté, comme « repo add ».
func (s *server) handleRepoAdd(w http.ResponseWriter, r *http.Request) {
	form := repoForm{Repo: formValue(r, fieldRepo), Label: formValue(r, fieldLabel), Branches: formValue(r, fieldBranches)}
	fail := func(status int, msg string) {
		v, err := s.settingsData(r)
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
	label, patterns, err := parseRepoForm(form.Label, form.Branches)
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
		s.serverError(w, r, fmt.Errorf("ajout du dépôt %s : résultat manquant", ref))
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
		fail(http.StatusUnprocessableEntity, fmt.Sprintf(
			"Dépôt %s inaccessible : %s. Vérifiez son nom et que le compte gh (sans gh : la clé SSH ou les "+
				"identifiants Git de cette machine) y a accès.", ref, textclean.Clean(res.Error)))
	default:
		s.logger.Error("ajout d'un dépôt impossible", "depot", ref.String(), "erreur", res.Error)
		fail(http.StatusBadGateway, fmt.Sprintf(
			"Ajout du dépôt %s impossible pour le moment (GitHub injoignable, quota épuisé ou erreur de la base). "+
				"Le détail est dans le journal de « watchoo serve ».", ref))
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
	label, patterns, err := parseRepoForm(form.Label, form.Branches)
	if err != nil {
		v, lerr := s.settingsData(r)
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
		v.Flash, v.FlashError = "Modification refusée : "+err.Error(), true
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
	v := settingsView{Confirm: &removeConfirm{Repo: sum.Repository, Commits: sum.Commits, Running: s.sync.State().Running}}
	s.render(w, r, http.StatusOK, pageSettings,
		s.page(r, "Retirer "+textclean.Clean(sum.DisplayName()), navSettings, v))
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
		s.renderError(w, r, http.StatusConflict, "Suppression impossible",
			"Une synchro est en cours : réessayez quand elle sera terminée. Rien n'a été supprimé.")
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
	d, err := parseIntervalInput(input)
	if err == nil {
		err = s.sync.SetInterval(d)
	}
	if err != nil {
		if errors.Is(err, wsync.ErrCoordinatorClosed) {
			s.renderError(w, r, http.StatusConflict, "Changement impossible", msgSyncClosed)
			return
		}
		v, lerr := s.settingsData(r)
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
			s.renderFragment(w, r, http.StatusOK, pageSettings, "settings-quota", s.page(r, "Réglages", navSettings, q))
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
	s.renderFragment(w, r, http.StatusOK, pageSettings, "settings-quota", s.page(r, "Réglages", navSettings, q))
}
