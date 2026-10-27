package web

// Partage de la configuration sur la page Réglages (documentation/06-interface-web.md, « Réglages »,
// « Partager la configuration »). Affichage seulement : le cœur est internal/configshare.
//
// PROPRIÉTAIRE : agent web (ce fichier, templates/settings_import.html, settings_share_test.go, et la
// section « Partager la configuration » de templates/settings.html).
//
//   - GET  /settings/export : configshare.Export puis Marshal ; pièce jointe JSON au nom sûr
//     (configshare.FileName), Cache-Control no-store ; aucun effet de bord ;
//   - POST /settings/import : multipart (champ « file », corps borné par limitBody et analysé par le
//     CSRF), configshare.ParseBytes puis configshare.Plan SANS vérification d'accès ; rend l'aperçu
//     (rien n'est écrit) avec le document dans le champ caché « document » ; 400 si invalide, 413 si
//     trop gros ;
//   - POST /settings/import/apply : champ « document », REVALIDÉ entièrement par ParseBytes puis
//     configshare.Apply (qui refait Plan) ; redirection 303 vers /settings?msg=imported&… avec un
//     bilan fait de nombres construits par le serveur (jamais de texte de la requête dans l'URL).
//
// Les réglages de l'espace sont relus sur le disque à chaque requête (workspace.Load sur une copie
// de s.ws) : l'export et l'aperçu partent ainsi de config.yaml tel qu'il est, y compris après un
// import fait par ce processus ou par la CLI. La configuration du processus (s.cfg, intervalle du
// coordinateur…) n'est jamais changée à chaud : les réglages importés valent au prochain lancement.
//
// Un seul import à la fois par processus (importMu) : deux « Appliquer » simultanés réécriraient
// config.yaml l'un après l'autre sans se voir.

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/studoo-app/watchoo/internal/configshare"
	"github.com/studoo-app/watchoo/internal/textclean"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Champs des formulaires d'import.
const (
	fieldImportFile     = "file"     // fichier envoyé (POST /settings/import)
	fieldImportDocument = "document" // contenu du fichier renvoyé par l'aperçu (POST /settings/import/apply)
)

// pageSettingsImport est le template de l'aperçu et des erreurs d'import (templates/settings_import.html).
const pageSettingsImport = "settings_import"

// msgImported est le code du bilan d'un import (paramètre msg de /settings).
const msgImported = "imported"

// Paramètres du bilan d'un import dans l'adresse de redirection (nombres seulement).
const (
	paramAdded        = "added"
	paramUpdated      = "updated"
	paramReenabled    = "reenabled"
	paramAlready      = "already"
	paramInaccessible = "inaccessible"
	paramErrors       = "errors"
	paramWritten      = "written"
)

// Bornes de l'affichage des textes venus du document (non fiables).
const (
	maxImportText     = 120 // nom de l'espace, version, nom du fichier, label
	maxImportBranches = 200 // liste des motifs d'un dépôt
	maxImportError    = 300 // une ligne d'un message de validation
)

// importMu sérialise les imports appliqués par ce processus.
var importMu sync.Mutex

// importView est le Content de templates/settings_import.html : aperçu d'un import, ou fichier refusé
// (Error). Tous les textes sont déjà nettoyés (textclean) et bornés ; html/template les échappe.
type importView struct {
	FileName string // nom du fichier envoyé (sans chemin) ; "" s'il est inconnu

	// Refus : Error est la première ligne du message, ErrorDetails les problèmes listés.
	Error        string
	ErrorDetails []string

	Source    importSource
	Settings  []importSetting // réglages modifiés
	Provided  int             // nombre de réglages fournis par le document
	Rewrite   bool            // config.yaml sera réécrit
	Changes   []importRepo    // dépôts à ajouter ou à mettre à jour, dans l'ordre du fichier
	Unchanged []importRepo    // dépôts inchangés
	Summary   configshare.Summary
	Warnings  []string
	// HasChanges : l'import changerait quelque chose (bouton « Appliquer » affiché).
	HasChanges bool
	// Document est le contenu exact du fichier, renvoyé tel quel par « Appliquer » (champ caché) : il
	// sera revalidé entièrement.
	Document string
}

// importSource décrit l'origine du document (textes informatifs, non fiables).
type importSource struct {
	WorkspaceName  string
	WatchooVersion string
	ExportedAt     time.Time
}

// importSetting est un réglage modifié par l'import.
type importSetting struct {
	Key, Before, After string
	EnvVar             string // variable WATCHOO_* qui masque ce réglage ; "" sinon
}

// importRepo est un dépôt de l'aperçu.
type importRepo struct {
	Index    int
	Repo     string // owner/name
	Action   configshare.Action
	Reenable bool
	// Label et Branches : valeurs après l'import ; *Before : valeurs actuelles (dépôt déjà présent).
	Label, LabelBefore       string
	Branches, BranchesBefore string
	// Thresholds : seuils propres après l'import (« Flemme 3 j · Dead espace ») ; *Before : actuels.
	Thresholds, ThresholdsBefore string
	LabelChanged                 bool
	BranchesChanged              bool
	ThresholdsChanged            bool
}

// ActionText est le libellé de l'action.
func (r importRepo) ActionText() string {
	switch {
	case r.Action == configshare.ActionAdd:
		return "à ajouter"
	case r.Reenable:
		return "à reprendre"
	case r.Action == configshare.ActionUpdate:
		return "à mettre à jour"
	case r.Action == configshare.ActionUnchanged:
		return "inchangé"
	case r.Action == configshare.ActionInaccessible:
		return "inaccessible"
	}
	return "en erreur"
}

// ActionClass est la classe DaisyUI du badge de l'action.
func (r importRepo) ActionClass() string {
	switch {
	case r.Action == configshare.ActionAdd:
		return "badge-success"
	case r.Reenable:
		return "badge-info"
	case r.Action == configshare.ActionUpdate:
		return "badge-warning"
	case r.Action == configshare.ActionUnchanged:
		return "badge-ghost"
	}
	return "badge-error"
}

// shareWorkspace renvoie une copie de l'espace servi dont les réglages sont relus sur le disque
// (workspace.Load : fichiers et variables WATCHOO_*, droits vérifiés). s.ws n'est jamais modifié.
func (s *server) shareWorkspace() (*workspace.Workspace, error) {
	ws := &workspace.Workspace{Root: s.ws.Root, Dir: s.ws.Dir}
	if err := workspace.Load(ws); err != nil {
		return nil, err
	}
	return ws, nil
}

// shareUnavailable répond 503 quand l'espace n'est pas connu du serveur (Options.Workspace nil).
func (s *server) shareUnavailable(w http.ResponseWriter, r *http.Request) {
	s.renderError(w, r, http.StatusServiceUnavailable, "Partage indisponible",
		"Le partage de la configuration n'est pas disponible pour ce serveur. Utilisez « watchoo config export » "+
			"ou « watchoo config import » dans l'espace.")
}

// settingsUnreadable répond quand les réglages de l'espace ne peuvent pas être relus (config.yaml
// devenu invalide, droits…) : rien n'est fait.
func (s *server) settingsUnreadable(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("réglages de l'espace illisibles", "erreur", err)
	s.renderError(w, r, http.StatusInternalServerError, "Réglages illisibles",
		"Les réglages de l'espace ne peuvent pas être relus : "+truncate(maxImportError, firstLineOf(err.Error()))+
			". Corrigez config.yaml, puis recommencez. Rien n'a été modifié.")
}

// handleSettingsExport répond à GET /settings/export : le document de partage en pièce jointe. Aucun
// effet de bord, aucun appel à gh ni à GitHub.
func (s *server) handleSettingsExport(w http.ResponseWriter, r *http.Request) {
	if s.ws == nil {
		s.shareUnavailable(w, r)
		return
	}
	ws, err := s.shareWorkspace()
	if err != nil {
		s.settingsUnreadable(w, r, err)
		return
	}
	doc, err := configshare.Export(r.Context(), ws, s.store, configshare.ExportOptions{Now: s.now(), WatchooVersion: s.version})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data, err := doc.Marshal()
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	// FileName ne garde que des caractères ASCII sûrs : aucun guillemet ni saut de ligne possible.
	h.Set("Content-Disposition", `attachment; filename="`+configshare.FileName(ws.Config.Name)+`"`)
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) // client parti : rien à faire
}

// handleSettingsImport répond à POST /settings/import : validation et aperçu, rien n'est appliqué.
func (s *server) handleSettingsImport(w http.ResponseWriter, r *http.Request) {
	if s.ws == nil {
		s.shareUnavailable(w, r)
		return
	}
	var v importView
	if r.MultipartForm == nil || len(r.MultipartForm.File[fieldImportFile]) == 0 {
		v.Error = "Aucun fichier reçu : choisissez le fichier JSON exporté par « watchoo config export » ou par le lien « Exporter »."
		s.renderImport(w, r, http.StatusBadRequest, v)
		return
	}
	fh := r.MultipartForm.File[fieldImportFile][0]
	v.FileName = truncate(maxImportText, filepath.Base(strings.ReplaceAll(fh.Filename, `\`, "/")))
	if v.FileName == "." || v.FileName == "/" {
		v.FileName = ""
	}
	data, err := readUpload(fh)
	if err != nil {
		s.logger.Warn("fichier de configuration illisible", "erreur", err)
		v.Error = "Le fichier envoyé n'a pas pu être lu. Rien n'a été modifié."
		s.renderImport(w, r, http.StatusBadRequest, v)
		return
	}
	s.previewImport(w, r, v, data)
}

// readUpload lit un fichier envoyé, au plus configshare.MaxFileSize + 1 octets (ParseBytes refuse
// ensuite un fichier trop gros).
func readUpload(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, configshare.MaxFileSize+1))
}

// previewImport valide data, calcule l'aperçu (sans vérification d'accès) et le rend.
func (s *server) previewImport(w http.ResponseWriter, r *http.Request, v importView, data []byte) {
	f, err := configshare.ParseBytes(data)
	if err != nil {
		s.importRefused(w, r, v, err)
		return
	}
	ws, err := s.shareWorkspace()
	if err != nil {
		s.settingsUnreadable(w, r, err)
		return
	}
	p, err := configshare.Plan(r.Context(), ws, s.store, f, configshare.PlanOptions{})
	if err != nil {
		s.importRefused(w, r, v, err)
		return
	}
	fillPreview(&v, p)
	v.Document = string(data)
	s.renderImport(w, r, http.StatusOK, v)
}

// importRefused rend la page d'import avec le refus err : 413 (trop gros), 400 (document invalide),
// 500 sinon (base illisible : détail au journal seulement).
func (s *server) importRefused(w http.ResponseWriter, r *http.Request, v importView, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, configshare.ErrTooLarge):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, configshare.ErrInvalid), errors.Is(err, configshare.ErrUnsupportedFormat),
		errors.Is(err, configshare.ErrNewerVersion):
	default:
		s.serverError(w, r, err)
		return
	}
	setImportError(&v, err.Error())
	s.renderImport(w, r, status, v)
}

// setImportError découpe un message de validation (première ligne, puis un problème par ligne), nettoie
// et borne chaque ligne.
func setImportError(v *importView, msg string) {
	lines := strings.Split(textclean.CleanLines(msg), "\n")
	v.Error = upperFirst(truncate(maxImportError, lines[0]))
	if !strings.HasSuffix(v.Error, ".") && !strings.HasSuffix(v.Error, ":") && !strings.HasSuffix(v.Error, "…") {
		v.Error += "."
	}
	v.ErrorDetails = nil
	for _, l := range lines[1:] {
		if l = truncate(maxImportError, l); l != "" {
			v.ErrorDetails = append(v.ErrorDetails, l)
		}
	}
	if !strings.Contains(msg, "rien n'a été importé") {
		v.ErrorDetails = append(v.ErrorDetails, "Rien n'a été importé.")
	}
	v.Document, v.HasChanges = "", false
}

// fillPreview recopie l'aperçu p dans v, textes nettoyés et bornés.
func fillPreview(v *importView, p *configshare.Preview) {
	v.Source = importSource{
		WorkspaceName:  truncate(maxImportText, p.Source.WorkspaceName),
		WatchooVersion: truncate(maxImportText, p.Source.WatchooVersion),
		ExportedAt:     p.Source.ExportedAt,
	}
	v.Provided = len(p.Settings.Provided)
	v.Rewrite = p.Settings.Rewrite
	for _, c := range p.Settings.Changes {
		v.Settings = append(v.Settings, importSetting{
			Key: c.Key, Before: truncate(maxImportBranches, c.Before), After: truncate(maxImportBranches, c.After),
			EnvVar: c.EnvVar,
		})
	}
	for _, c := range p.Repositories {
		row := importRepo{
			Index: c.Index, Repo: truncate(maxImportText, c.Repo), Action: c.Action, Reenable: c.Reenable,
			Label: truncate(maxImportText, c.After.Label), Branches: truncate(maxImportBranches, branchesText(c.After.Branches)),
			Thresholds: thresholdsText(c.After.FlemmeAfterDays, c.After.DeadAfterDays),
		}
		if c.Before != nil {
			row.LabelBefore = truncate(maxImportText, c.Before.Label)
			row.BranchesBefore = truncate(maxImportBranches, branchesText(c.Before.Branches))
			row.LabelChanged = c.Before.Label != c.After.Label
			row.BranchesChanged = row.BranchesBefore != row.Branches
			row.ThresholdsBefore = thresholdsText(c.Before.FlemmeAfterDays, c.Before.DeadAfterDays)
			row.ThresholdsChanged = row.ThresholdsBefore != row.Thresholds
		}
		if c.Action == configshare.ActionUnchanged {
			v.Unchanged = append(v.Unchanged, row)
		} else {
			v.Changes = append(v.Changes, row)
		}
	}
	v.Summary = p.Summary
	for _, warn := range p.Warnings {
		v.Warnings = append(v.Warnings, truncate(maxImportError, warn.Message))
	}
	v.HasChanges = p.Settings.Rewrite || p.Summary.Added+p.Summary.Updated > 0
}

// renderImport écrit la page d'aperçu / de refus d'un import.
func (s *server) renderImport(w http.ResponseWriter, r *http.Request, status int, v importView) {
	s.render(w, r, status, pageSettingsImport, s.page(r, "Importer une configuration", navSettings, v))
}

// handleSettingsImportApply répond à POST /settings/import/apply : le document renvoyé par l'aperçu
// est revalidé entièrement (jamais de confiance dans l'aperçu), puis appliqué par configshare.Apply :
// réglages (config.yaml réécrit si un réglage change), puis dépôts (accès vérifié, rien n'est retiré).
func (s *server) handleSettingsImportApply(w http.ResponseWriter, r *http.Request) {
	if s.ws == nil {
		s.shareUnavailable(w, r)
		return
	}
	data := []byte(r.PostFormValue(fieldImportDocument))
	var v importView
	if len(strings.TrimSpace(string(data))) == 0 {
		v.Error = "Aucun document à appliquer : envoyez d'abord le fichier pour voir l'aperçu."
		s.renderImport(w, r, http.StatusBadRequest, v)
		return
	}
	f, err := configshare.ParseBytes(data)
	if err != nil {
		s.importRefused(w, r, v, err)
		return
	}
	if !importMu.TryLock() {
		s.renderError(w, r, http.StatusConflict, "Import déjà en cours",
			"Un autre import de configuration est en cours : attendez qu'il se termine, puis recommencez. Rien n'a été modifié.")
		return
	}
	defer importMu.Unlock()
	ws, err := s.shareWorkspace()
	if err != nil {
		s.settingsUnreadable(w, r, err)
		return
	}
	res, err := configshare.Apply(r.Context(), ws, s.store, s.repos, f, configshare.ApplyOptions{})
	if res == nil {
		if errors.Is(err, configshare.ErrInvalid) {
			s.importRefused(w, r, v, err)
			return
		}
		s.logger.Error("import de la configuration impossible", "erreur", err)
		setImportError(&v, "Import impossible : "+firstLineOf(err.Error()))
		s.renderImport(w, r, http.StatusInternalServerError, v)
		return
	}
	if err != nil {
		// Erreur globale en cours de route (gh, contexte) : les dépôts non traités sont comptés en
		// erreur dans le bilan ; les réglages ont pu être écrits.
		s.logger.Error("import des dépôts interrompu", "erreur", err)
	}
	for _, c := range res.Preview.Repositories {
		if c.Action == configshare.ActionInaccessible || c.Action == configshare.ActionError {
			s.logger.Warn("dépôt non importé", "depot", c.Repo, "resultat", string(c.Action), "erreur", c.Error)
		}
	}
	redirect(w, r, importRedirectTarget(res))
}

// importRedirectTarget construit l'adresse du bilan : /settings?msg=imported suivi des nombres du
// résultat (valeurs nulles omises).
func importRedirectTarget(res *configshare.Result) string {
	q := url.Values{}
	q.Set("msg", msgImported)
	put := func(key string, n int) {
		if n > 0 {
			q.Set(key, strconv.Itoa(n))
		}
	}
	sum := res.Summary
	put(paramAdded, sum.Added)
	put(paramUpdated, sum.Updated)
	put(paramReenabled, sum.Reenabled)
	put(paramAlready, sum.AlreadyTracked)
	put(paramInaccessible, sum.Inaccessible)
	put(paramErrors, sum.Errors)
	if res.Settings.Written {
		q.Set(paramWritten, "1")
	}
	return settingsPath + "?" + q.Encode()
}

// importFlash construit le message du bilan d'un import à partir des nombres de l'adresse (bornés ;
// rien d'autre de la requête n'est repris). Le second résultat signale des dépôts non importés.
func importFlash(r *http.Request) (string, bool) {
	count := func(key string) int {
		n, err := strconv.Atoi(queryValue(r, key))
		if err != nil || n < 0 {
			return 0
		}
		return min(n, configshare.MaxRepositories)
	}
	added, updated, reenabled := count(paramAdded), count(paramUpdated), count(paramReenabled)
	already, inaccessible, failed := count(paramAlready), count(paramInaccessible), count(paramErrors)
	written := queryValue(r, paramWritten) == "1"

	var parts []string
	if added > 0 {
		parts = append(parts, pluralize(added, "dépôt ajouté", "dépôts ajoutés"))
	}
	if updated > 0 {
		p := pluralize(updated, "dépôt mis à jour", "dépôts mis à jour")
		if reenabled > 0 {
			p += fmt.Sprintf(" (dont %d repris)", min(reenabled, updated))
		}
		parts = append(parts, p)
	}
	if already > 0 {
		parts = append(parts, pluralize(already, "dépôt déjà suivi", "dépôts déjà suivis"))
	}
	var b strings.Builder
	b.WriteString("Configuration importée")
	if len(parts) == 0 && !written && inaccessible+failed == 0 {
		b.WriteString(" : rien n'a changé.")
	} else if len(parts) > 0 {
		b.WriteString(" : " + strings.Join(parts, ", ") + ".")
	} else {
		b.WriteString(".")
	}
	if written {
		b.WriteString(" config.yaml est réécrit (ancien fichier gardé dans " + workspace.BackupFileName +
			") : les réglages importés seront pris en compte au prochain lancement de « watchoo serve ».")
	}
	if inaccessible+failed > 0 {
		var ko []string
		if inaccessible > 0 {
			ko = append(ko, pluralize(inaccessible, "dépôt inaccessible", "dépôts inaccessibles"))
		}
		if failed > 0 {
			ko = append(ko, pluralize(failed, "dépôt en erreur", "dépôts en erreur"))
		}
		b.WriteString(" Non importés : " + strings.Join(ko, ", ") + ". Vérifiez leur nom et l'accès du compte gh, " +
			"puis importez de nouveau le fichier (sans risque) ; le détail est dans le journal de « watchoo serve » " +
			"et « watchoo config import --dry-run » vérifie chaque dépôt.")
	}
	return b.String(), inaccessible+failed > 0
}

// upperFirst met la première lettre de s en majuscule (début d'une phrase affichée).
func upperFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// firstLineOf renvoie la première ligne de s.
func firstLineOf(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
