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

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
)

// Pages connues : nom du fichier templates/<nom>.html.
const (
	pageDashboard    = "dashboard"
	pageRepo         = "repo"
	pageContributors = "contributors"
	pageContributor  = "contributor"
	pageSettings     = "settings"
	pageErrors       = "errors"
)

var pageNames = []string{pageDashboard, pageRepo, pageContributors, pageContributor, pageSettings, pageSettingsImport, pageErrors}

// Entrées de la barre de navigation (PageData.Nav).
const (
	navDashboard    = "dashboard"
	navContributors = "contributors"
	navSettings     = "settings"
)

// PageData est la donnée de TOUS les templates (pages complètes et fragments). Construire avec s.page.
type PageData struct {
	Title     string   // titre propre à la page ; le layout ajoute le nom de l'espace et « Watchoo »
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
}

// UpdateView est le bandeau de nouvelle version du layout (0.6.0).
type UpdateView struct {
	Current    string // version installée
	Latest     string // dernière version publiée
	ReleaseURL string // page de la release (lien « Notes de version »)
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
	Heading string // titre court (« Page introuvable »)
	Message string // explication, sans détail interne
}

// page construit la PageData d'une requête.
func (s *server) page(r *http.Request, title, nav string, content any) PageData {
	return PageData{
		Title:     title,
		Nav:       nav,
		Content:   content,
		Workspace: s.cfg.Name,
		Version:   s.version,
		CSRFToken: csrfToken(r),
		Sync:      s.syncView(""),
		Update:    s.updateView(),
	}
}

// parseTemplates analyse layout.html et partials.html, puis chaque page dans un clone. Une erreur
// cite le fichier fautif ; chaque page doit définir « content ».
func (s *server) parseTemplates() (map[string]*template.Template, error) {
	base, err := template.New("").Funcs(s.funcs()).ParseFS(templateFS, "templates/layout.html", "templates/partials.html")
	if err != nil {
		return nil, fmt.Errorf("interface web : templates communs invalides : %w", err)
	}
	if base.Lookup("layout") == nil {
		return nil, fmt.Errorf("interface web : templates/layout.html ne définit pas « layout »")
	}
	pages := make(map[string]*template.Template, len(pageNames))
	for _, name := range pageNames {
		file := "templates/" + name + ".html"
		if _, err := fs.Stat(templateFS, file); err != nil {
			return nil, fmt.Errorf("interface web : template %s absent : %w", file, err)
		}
		t, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("interface web : template %s : %w", file, err)
		}
		if t, err = t.ParseFS(templateFS, file); err != nil {
			return nil, fmt.Errorf("interface web : template %s invalide : %w", file, err)
		}
		if t.Lookup("content") == nil {
			return nil, fmt.Errorf("interface web : template %s ne définit pas « content »", file)
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

// execute rend le bloc name de l'ensemble page dans un tampon, puis l'écrit.
func (s *server) execute(w http.ResponseWriter, r *http.Request, status int, page, name string, data PageData) {
	t, ok := s.pages[page]
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
		s.renderError(w, r, http.StatusInternalServerError, "Erreur interne",
			"La page n'a pas pu être affichée. Le détail est dans le journal de « watchoo serve ».")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Vary", "HX-Request")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes()) // client parti : rien à faire
}

// renderError écrit la page d'erreur (complète, ou fragment « error-alert » pour une requête HTMX).
// Pour 401 et 421, la page est publique : rien de l'espace n'y figure.
func (s *server) renderError(w http.ResponseWriter, r *http.Request, status int, heading, message string) {
	data := s.page(r, heading, "", ErrorView{Status: status, Heading: heading, Message: message})
	if status == http.StatusUnauthorized || status == http.StatusMisdirectedRequest {
		data = PageData{Title: heading, Public: true, Version: s.version,
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
	s.renderError(w, r, http.StatusNotFound, "Page introuvable",
		"Cette page n'existe pas, ou le dépôt demandé n'est plus suivi dans cet espace.")
}

// methodNotAllowed écrit la page 405.
func (s *server) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	s.renderError(w, r, http.StatusMethodNotAllowed, "Action impossible",
		"Cette adresse n'accepte pas ce type de requête.")
}

// badRequest écrit une page 400 avec message (en français, sans détail interne ; il peut citer la
// saisie de l'utilisateur, échappée par le template).
func (s *server) badRequest(w http.ResponseWriter, r *http.Request, message string) {
	s.renderError(w, r, http.StatusBadRequest, "Requête invalide", message)
}

// serverError journalise err (message sans donnée personnelle : erreurs du store et du moteur) et
// écrit la page 500, sans détail interne.
func (s *server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("erreur interne", "methode", r.Method, "erreur", err)
	s.renderError(w, r, http.StatusInternalServerError, "Erreur interne",
		"Une erreur est survenue. Le détail est dans le journal de « watchoo serve ».")
}

// writeFallbackError écrit une erreur 500 en texte brut (quand même la page d'erreur ne peut pas
// être rendue).
func (s *server) writeFallbackError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte("erreur interne de Watchoo\n"))
}
