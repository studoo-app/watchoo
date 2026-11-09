package web

// Templates : analyse au démarrage, rendu des pages et des fragments, pages d'erreur.
//
// Organisation de templates/ (embarqués) :
//   - layout.html   : « layout » (document complet : navbar, thème, logo, pied de page) ; il appelle
//     les blocs « content » (obligatoire), « head » et « scripts » (facultatifs) de la page ;
//   - partials.html : morceaux communs (« state-badge », « rel-time », « csrf-field »,
//     « sync-status », « sync-all-button », « sync-repo-button », « avatar », « flash ») ;
//   - un fichier par page (dashboard, repo, contributors, settings, errors) : il définit « content »
//     et ses fragments HTMX (blocs nommés « <page>-… », rendus seuls par renderFragment).
//
// Chaque page est analysée dans son propre ensemble (layout + partials + la page) : deux pages
// peuvent définir des blocs de même nom sans conflit.
//
// Langues : les templates sont analysés une fois par langue (anglais, français), chaque jeu avec ses
// fonctions liées au traducteur de sa langue (t, tn, relTime… : funcs.go). Une requête est rendue
// avec le jeu de PageData.Lang, la langue de l'espace lue au début de la requête (lang.go).

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// langs sont les langues des jeux de templates ; la première sert de repli.
var langs = []i18n.Lang{i18n.English, i18n.French}

// Pages connues : nom du fichier templates/<nom>.html.
const (
	pageDashboard    = "dashboard"
	pageRepo         = "repo"
	pageContributors = "contributors"
	pageContributor  = "contributor"
	pageSettings     = "settings"
	pageErrors       = "errors"
	// Diagnostic (phase 7.4).
	pageDiagnostic     = "diagnostic"
	pageDiagnosticLead = "diagnostic_lead"
	// Conventions (phase 8.2).
	pageConventions = "conventions"
)

var pageNames = []string{pageDashboard, pageRepo, pageContributors, pageContributor, pageSettings, pageSettingsImport,
	pageDiagnostic, pageDiagnosticLead, pageConventions, pageErrors}

// Entrées de la barre de navigation (PageData.Nav).
const (
	navDashboard    = "dashboard"
	navContributors = "contributors"
	navSettings     = "settings"
	navDiagnostic   = "diagnostic"
	navConventions  = "conventions"
)

// PageData est la donnée de TOUS les templates (pages complètes et fragments). Construire avec s.page.
type PageData struct {
	Title     string   // titre propre à la page (traduit) ; le layout ajoute le nom de l'espace et « Watchoo »
	Lang      string   // langue de la page (« en », « fr ») : jeu de templates et attribut <html lang>
	Path      string   // chemin de la requête, sans la query (champ return du sélecteur de langue)
	Nav       string   // entrée active de la navigation (navDashboard…) ; vide = aucune
	Content   any      // données propres à la page (type défini par le fichier Go de la page)
	Workspace string   // nom de l'espace (Config.Name)
	Version   string   // version de Watchoo
	CSRFToken string   // jeton CSRF de la requête (champ csrf_token, en-tête X-CSRF-Token)
	Sync      SyncView // état des synchros (fragment « sync-status » de la navbar)
	// Update est la nouvelle version de Watchoo connue (bandeau du layout) ; nil = aucune.
	Update *UpdateView
	// Public : page d'erreur montrée avant authentification ou sur un Host refusé : ni navigation, ni
	// nom d'espace, ni état de synchro.
	Public bool
	// Diagnostic : le diagnostic est activé (diagnostic.enabled) ; entrée « Diagnostic » de la navigation.
	Diagnostic bool
	// Conventions : les conventions sont activées (conventions.enabled) ; entrée « Conventions » de la
	// navigation (phase 8.2).
	Conventions bool
}

// UpdateView est le bandeau de nouvelle version du layout (0.6.0).
type UpdateView struct {
	Current    string // version installée
	Latest     string // dernière version publiée
	ReleaseURL string // page de la release (lien « Release notes »)
	Docker     bool   // image Docker : changer le tag au lieu de « watchoo update »
}

// updateView renvoie le bandeau à afficher, ou nil sans nouvelle version connue.
func (s *server) updateView() *UpdateView {
	if s.updates == nil {
		return nil
	}
	st, ok := s.updates.Available()
	if !ok || !st.UpdateAvailable {
		return nil
	}
	return &UpdateView{Current: st.Current, Latest: st.Latest, ReleaseURL: st.ReleaseURL, Docker: s.docker}
}

// ErrorView est le contenu de la page d'erreur (templates/errors.html).
type ErrorView struct {
	Status  int    // code HTTP
	Heading string // titre court, traduit (« Page not found »)
	Message string // explication traduite, sans détail interne
}

// page construit la PageData d'une requête, dans la langue de la requête (title est déjà traduit).
func (s *server) page(r *http.Request, title, nav string, content any) PageData {
	return PageData{
		Title:     title,
		Lang:      string(s.tr(r).Lang()),
		Path:      r.URL.EscapedPath(),
		Nav:       nav,
		Content:   content,
		Workspace: s.cfg.Name,
		Version:   s.version,
		CSRFToken: csrfToken(r),
		Sync:      s.syncView(s.tr(r), ""),
		Update:    s.updateView(),
		// Diagnostic : entrée de navigation réservée au formateur, seulement si le diagnostic est activé.
		Diagnostic: s.cfg.Diagnostic.Enabled,
		// Conventions : entrée de navigation seulement si les conventions sont activées.
		Conventions: s.cfg.Conventions.Enabled,
	}
}

// parseTemplates analyse les templates pour chaque langue de langs.
func (s *server) parseTemplates() (map[i18n.Lang]map[string]*template.Template, error) {
	out := make(map[i18n.Lang]map[string]*template.Template, len(langs))
	for _, l := range langs {
		pages, err := s.parsePages(i18n.For(l))
		if err != nil {
			return nil, err
		}
		out[l] = pages
	}
	return out, nil
}

// parsePages analyse layout.html et partials.html, puis chaque page dans un clone, avec les
// fonctions liées au traducteur tr. Une erreur cite le fichier fautif ; chaque page doit définir
// « content ».
func (s *server) parsePages(tr *i18n.Translator) (map[string]*template.Template, error) {
	base, err := template.New("").Funcs(s.funcs(tr)).ParseFS(templateFS, "templates/layout.html", "templates/partials.html")
	if err != nil {
		return nil, fmt.Errorf("%s%s%w", i18n.T("web.start.templates_invalid"), i18n.T("web.colon"), err)
	}
	if base.Lookup("layout") == nil {
		return nil, errors.New(i18n.T("web.start.no_block", "templates/layout.html", "layout"))
	}
	pages := make(map[string]*template.Template, len(pageNames))
	for _, name := range pageNames {
		file := "templates/" + name + ".html"
		if _, err := fs.Stat(templateFS, file); err != nil {
			return nil, fmt.Errorf("%s%s%w", i18n.T("web.start.template_missing", file), i18n.T("web.colon"), err)
		}
		t, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("%s%s%w", i18n.T("web.start.template_invalid", file), i18n.T("web.colon"), err)
		}
		if t, err = t.ParseFS(templateFS, file); err != nil {
			return nil, fmt.Errorf("%s%s%w", i18n.T("web.start.template_invalid", file), i18n.T("web.colon"), err)
		}
		if t.Lookup("content") == nil {
			return nil, errors.New(i18n.T("web.start.no_block", file, "content"))
		}
		pages[name] = t
	}
	return pages, nil
}

// render écrit la page complète page (layout) avec data et le code status. Le rendu est fait en
// mémoire d'abord : une erreur de template donne une page d'erreur 500 propre, jamais une page coupée.
func (s *server) render(w http.ResponseWriter, r *http.Request, status int, page string, data PageData) {
	s.execute(w, r, status, page, "layout", data)
}

// renderFragment écrit le seul bloc fragment du template de page (réponse à une requête HTMX), avec
// data et le code status. Le fragment reçoit la même PageData qu'une page complète.
func (s *server) renderFragment(w http.ResponseWriter, r *http.Request, status int, page, fragment string, data PageData) {
	s.execute(w, r, status, page, fragment, data)
}

// execute rend le bloc name de l'ensemble page, dans la langue data.Lang (repli : anglais), dans un
// tampon, puis l'écrit.
func (s *server) execute(w http.ResponseWriter, r *http.Request, status int, page, name string, data PageData) {
	set, ok := s.pages[i18n.Lang(data.Lang)]
	if !ok {
		set = s.pages[langs[0]]
		data.Lang = string(langs[0])
	}
	t, ok := set[page]
	if !ok || t.Lookup(name) == nil {
		s.logger.Error("template inconnu", "page", page, "bloc", name)
		s.writeFallbackError(w)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		s.logger.Error("rendu du template impossible", "page", page, "bloc", name, "erreur", err)
		if page == pageErrors {
			s.writeFallbackError(w)
			return
		}
		s.renderError(w, r, http.StatusInternalServerError, s.t(r, "web.error.internal"),
			s.t(r, "web.error.render"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Vary", "HX-Request")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes()) // client parti : rien à faire
}

// renderError écrit la page d'erreur (complète, ou fragment « error-alert » pour une requête HTMX),
// heading et message étant déjà traduits. Pour 401 et 421, la page est publique : rien de l'espace
// n'y figure.
func (s *server) renderError(w http.ResponseWriter, r *http.Request, status int, heading, message string) {
	data := s.page(r, heading, "", ErrorView{Status: status, Heading: heading, Message: message})
	if status == http.StatusUnauthorized || status == http.StatusMisdirectedRequest {
		data = PageData{Title: heading, Lang: data.Lang, Public: true, Version: s.version,
			Content: ErrorView{Status: status, Heading: heading, Message: message}}
	}
	if isHTMX(r) && !data.Public {
		s.execute(w, r, status, pageErrors, "error-alert", data)
		return
	}
	s.execute(w, r, status, pageErrors, "layout", data)
}

// notFound écrit la page 404.
func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	s.renderError(w, r, http.StatusNotFound, s.t(r, "web.error.not_found"), s.t(r, "web.error.not_found_text"))
}

// methodNotAllowed écrit la page 405.
func (s *server) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	s.renderError(w, r, http.StatusMethodNotAllowed, s.t(r, "web.error.method"), s.t(r, "web.error.method_text"))
}

// badRequest écrit une page 400 avec message (traduit, sans détail interne ; il peut citer la saisie
// de l'utilisateur, échappée par le template).
func (s *server) badRequest(w http.ResponseWriter, r *http.Request, message string) {
	s.renderError(w, r, http.StatusBadRequest, s.t(r, "web.error.bad_request"), message)
}

// serverError journalise err (message sans donnée personnelle : erreurs du store et du moteur) et
// écrit la page 500, sans détail interne.
func (s *server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("erreur interne", "methode", r.Method, "erreur", err)
	s.renderError(w, r, http.StatusInternalServerError, s.t(r, "web.error.internal"), s.t(r, "web.error.internal_text"))
}

// writeFallbackError écrit une erreur 500 en texte brut (quand même la page d'erreur ne peut pas
// être rendue), en anglais et en français : la langue de la requête peut être inconnue ici.
func (s *server) writeFallbackError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte("Watchoo internal error / erreur interne de Watchoo\n"))
}
