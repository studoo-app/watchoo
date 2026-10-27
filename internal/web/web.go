// Package web est l'interface web locale de Watchoo (« watchoo serve », documentation/06-interface-web.md).
//
// Couche d'affichage seulement, comme internal/cli et internal/tui : les données viennent de
// internal/store, les états d'activité et formats de internal/activity, les synchros du coordinateur
// de internal/sync. Aucune règle métier ici. Les handlers sont des http.Handler standard ; go-chi/chi
// ne sert qu'au routage.
//
// # Contrat pour les pages (phase 4)
//
// Chaque zone de pages a son fichier Go, son template et ses tests, écrits par un seul agent :
//
//	dashboard.go     templates/dashboard.html     dashboard_test.go     tableau de bord « / » et son fil
//	repo.go          templates/repo.html          repo_test.go          détail « /repos/{id} », static/app.js
//	contributors.go  templates/contributors.html  contributors_test.go  classement « /contributors »
//	settings.go      templates/settings.html      settings_test.go      réglages « /settings »
//	settings_share.go templates/settings_import.html settings_share_test.go partage de la configuration
//	                 (export, aperçu et application d'un import ; section de templates/settings.html)
//
// Tout le reste (web.go, routes.go, middleware.go, csrf.go, security.go, render.go, funcs.go,
// helpers.go, syncview.go, assets.go, serve.go, templates/layout.html, templates/partials.html,
// templates/errors.html, static/theme.js, static/manifest.webmanifest, tailwind/) est le socle commun :
// une page ne le modifie pas sans accord (voir le résumé de l'architecte).
//
// Règles communes :
//   - toute URL qui contient des données (filtres, page) est construite par withQuery, y compris dans
//     hx-get et hx-push-url, que html/template traite comme du texte et n'encode pas ;
//   - un handler lit ses paramètres avec repoIDParam, pageParam, isHTMX (helpers.go), lit la base par
//     s.store (méthodes de internal/store, jamais de SQL ici) et répond par s.render (page complète),
//     s.renderFragment (fragment HTMX), s.notFound, s.badRequest ou s.serverError (render.go) ;
//   - toutes les données d'un template sont une PageData (s.page) : Content porte les données propres à
//     la page ; les templates n'appellent que les fonctions de funcs.go et les méthodes des types du
//     cœur (DisplayName, FullName…) ;
//   - aucune action en GET : toute modification est un POST, protégé par le middleware CSRF (jeton
//     dans le champ csrf_token, template « csrf-field », ou l'en-tête X-CSRF-Token que HTMX ajoute
//     grâce à hx-headers sur <body>) ;
//   - données pour du JavaScript : uniquement en JSON dans <script type="application/json" id="…">
//     {{.Content.X}}</script> (html/template l'encode et l'échappe), lu par static/app.js ; jamais de
//     script en ligne, jamais d'attribut on*, jamais de template.HTML construit à partir de données ;
//   - textes venus de la base (messages, noms, labels) : html/template les échappe ; passer en plus par
//     clean ou firstLine (textclean) pour les caractères de contrôle ; aucun e-mail affiché ;
//   - fragments rafraîchis après une synchro : hx-get vers leur URL et
//     hx-trigger="sync-finished from:body" (événement envoyé par le fragment de synchro, syncview.go).
package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// SyncController pilote les synchros du processus ; *wsync.Coordinator le satisfait.
type SyncController interface {
	SyncAll() error              // tous les dépôts ; wsync.ErrSyncRunning si une synchro tourne
	SyncRepo(repoID int64) error // un dépôt ; mêmes erreurs
	State() wsync.SyncState
	Interval() time.Duration
	// SetInterval change l'intervalle pour la durée du processus (0 ou >= 15 min).
	SetInterval(d time.Duration) error
}

// RepoManager ajoute des dépôts et lit le quota ; *wsync.Engine le satisfait.
type RepoManager interface {
	AddRepositories(ctx context.Context, reqs []wsync.AddRequest, opts wsync.AddOptions) ([]wsync.AddResult, error)
	RateLimit(ctx context.Context) (provider.RateLimit, error)
}

var (
	_ SyncController = (*wsync.Coordinator)(nil)
	_ RepoManager    = (*wsync.Engine)(nil)
)

// Auth est l'authentification basique de l'interface ; sa valeur zéro n'en demande aucune. Password
// vient UNIQUEMENT de la variable d'environnement workspace.WebPasswordEnv ; il n'est jamais journalisé.
type Auth struct {
	Username string
	Password string
}

// Enabled indique si l'authentification basique est demandée.
func (a Auth) Enabled() bool { return a.Username != "" }

// Options règle New.
type Options struct {
	Store *store.Store   // base de l'espace (obligatoire)
	Sync  SyncController // coordinateur des synchros (obligatoire)
	Repos RepoManager    // ajout de dépôts et quota : le moteur (obligatoire)
	// Config est la configuration de l'espace (nom, seuils d'activité, sync.*, web.*).
	Config workspace.Config
	// Workspace est l'espace servi (racine, .watchoo/) : export et import de la configuration
	// (internal/configshare), qui réécrit .watchoo/config.yaml. nil = partage indisponible (les routes
	// /settings/export et /settings/import* répondent 503). Config reste la configuration du processus :
	// un import ne la change pas (prise en compte au prochain lancement).
	Workspace *workspace.Workspace
	// Location est le fuseau d'affichage (Config.Location) ; nil = time.Local.
	Location *time.Location
	Now      func() time.Time // horloge ; nil = time.Now
	Logger   *slog.Logger     // journal d'accès et erreurs ; nil = slog.Default()
	Version  string           // version affichée dans le pied de page
	// ListenAddr est l'adresse réellement écoutée (net.Listener.Addr().String()) : contrôle de l'en-tête
	// Host en écoute locale (DNS rebinding) et nom du cookie CSRF. Vide = « 127.0.0.1:8080 ».
	ListenAddr string
	Auth       Auth // authentification basique ; zéro = aucune
	// TrustedProxy : un reverse proxy authentifiant est devant (web.trusted_proxy). Le contrôle de
	// l'en-tête Host reste actif (DNS rebinding) : AllowedHosts liste les noms publics du proxy.
	TrustedProxy bool
	// AllowedHosts (web.allowed_hosts) : noms acceptés dans l'en-tête Host, quel que soit le port, en
	// plus de localhost, 127.0.0.1, [::1] et de l'hôte écouté.
	AllowedHosts []string
}

// server porte l'état partagé des handlers. Il est en lecture seule après New (sûr pour un usage
// concurrent) ; tout ce qui change vit dans le store et le coordinateur.
type server struct {
	store    *store.Store
	sync     SyncController
	repos    RepoManager
	cfg      workspace.Config
	ws       *workspace.Workspace // espace servi (partage de la configuration) ; nil = indisponible
	loc      *time.Location
	now      func() time.Time
	logger   *slog.Logger
	version  string
	pages    map[string]*template.Template // un ensemble de templates par page (render.go)
	assets   *assetStore                   // fichiers statiques embarqués (assets.go)
	csrf     *csrfProtector
	auth     Auth
	hosts    *hostPolicy // valeurs acceptées de l'en-tête Host ; nil = pas de contrôle
	listenAt string
}

// New vérifie les options, analyse les templates embarqués (erreur claire si l'un est invalide) et
// renvoie le handler de l'interface : routes, fichiers statiques et middlewares de sécurité (voir
// routes.go). Aucune requête réseau, aucune synchro n'est lancée.
func New(opts Options) (http.Handler, error) {
	s, err := newServer(opts)
	if err != nil {
		return nil, err
	}
	return s.routes(), nil
}

// newServer construit le server de New (les tests du package s'en servent directement).
func newServer(opts Options) (*server, error) {
	switch {
	case opts.Store == nil:
		return nil, errors.New("interface web : base non renseignée")
	case opts.Sync == nil:
		return nil, errors.New("interface web : coordinateur de synchro non renseigné")
	case opts.Repos == nil:
		return nil, errors.New("interface web : moteur de synchro non renseigné")
	}
	s := &server{
		store:    opts.Store,
		sync:     opts.Sync,
		repos:    opts.Repos,
		cfg:      opts.Config,
		ws:       opts.Workspace,
		loc:      opts.Location,
		now:      opts.Now,
		logger:   opts.Logger,
		version:  opts.Version,
		auth:     opts.Auth,
		listenAt: opts.ListenAddr,
	}
	if s.loc == nil {
		s.loc = time.Local
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.listenAt == "" {
		s.listenAt = workspace.DefaultWebAddr
	}
	if s.auth.Username != "" && s.auth.Password == "" {
		return nil, fmt.Errorf("interface web : mot de passe vide pour l'identifiant de web.auth.username (variable %s)",
			workspace.WebPasswordEnv)
	}
	hosts, err := allowedHosts(s.listenAt, opts.AllowedHosts, opts.TrustedProxy, s.auth.Enabled())
	if err != nil {
		return nil, err
	}
	s.hosts = hosts
	s.csrf = newCSRFProtector(s.listenAt)
	if s.assets, err = loadAssets(); err != nil {
		return nil, err
	}
	if s.pages, err = s.parseTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}
