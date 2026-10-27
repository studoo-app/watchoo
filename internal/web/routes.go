package web

// Routeur chi : toutes les routes de l'interface et l'ordre des middlewares.
//
// Ordre des middlewares (du premier au dernier appelé) :
//
//	recoverer → accessLog → securityHeaders → hostCheck → basicAuth → limitBody → csrf → timeout
//
// Le contrôle Host et l'authentification passent avant tout le reste (fichiers statiques compris) ;
// le CSRF ne laisse passer un POST qu'avec la bonne origine et le bon jeton. Aucune route ne modifie
// quoi que ce soit en GET.
//
// Routes (handler, fichier, propriétaire) :
//
//	GET  /                              handleDashboard        dashboard.go     dashboard
//	GET  /fragments/dashboard/repos     handleDashboardRepos   dashboard.go     dashboard
//	GET  /fragments/dashboard/feed      handleDashboardFeed    dashboard.go     dashboard
//	GET  /repos/{id}                    handleRepo             repo.go          repo
//	GET  /repos/{id}/commits            handleRepoCommits      repo.go          repo
//	GET  /contributors                  handleContributors     contributors.go  contributors+settings
//	GET  /fragments/contributors        handleContributorsTable contributors.go contributors+settings
//	GET  /settings                      handleSettings         settings.go      contributors+settings
//	POST /settings/repos                handleRepoAdd          settings.go      contributors+settings
//	POST /settings/repos/{id}           handleRepoUpdate       settings.go      contributors+settings
//	GET  /settings/repos/{id}/remove    handleRepoRemoveConfirm settings.go     contributors+settings
//	POST /settings/repos/{id}/remove    handleRepoRemove       settings.go      contributors+settings
//	POST /settings/interval             handleIntervalUpdate   settings.go      contributors+settings
//	GET  /fragments/quota               handleQuota            settings.go      contributors+settings
//	POST /sync                          handleSyncAll          syncview.go      socle
//	POST /repos/{id}/sync               handleSyncRepo         syncview.go      socle
//	GET  /fragments/sync                handleSyncStatus       syncview.go      socle
//	GET  /static/*                      handleStatic           assets.go        socle
//	GET  /favicon.ico, /manifest.webmanifest, /apple-touch-icon.png             socle

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// requestTimeout borne la durée d'un handler (lecture de la base, vérification d'accès d'un dépôt).
const requestTimeout = 30 * time.Second

// idPattern contraint l'identifiant d'un dépôt dans les routes : 1 à 18 chiffres (sinon 404).
const idPattern = "{id:[0-9]{1,18}}"

// routes construit le routeur.
func (s *server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(s.recoverer, s.accessLog, securityHeaders, s.hostCheck, s.basicAuth, limitBody,
		s.csrf.middleware(s), middleware.Timeout(requestTimeout))
	r.NotFound(s.notFound)
	r.MethodNotAllowed(s.methodNotAllowed)

	// Pages et fragments.
	r.Get("/", s.handleDashboard)
	r.Get("/fragments/dashboard/repos", s.handleDashboardRepos)
	r.Get("/fragments/dashboard/feed", s.handleDashboardFeed)
	r.Get("/repos/"+idPattern, s.handleRepo)
	r.Get("/repos/"+idPattern+"/commits", s.handleRepoCommits)
	r.Get("/contributors", s.handleContributors)
	r.Get("/fragments/contributors", s.handleContributorsTable)
	r.Get("/settings", s.handleSettings)
	r.Get("/settings/repos/"+idPattern+"/remove", s.handleRepoRemoveConfirm)
	r.Get("/fragments/quota", s.handleQuota)
	r.Get("/fragments/sync", s.handleSyncStatus)

	// Actions (POST seulement, protégées par le CSRF).
	r.Post("/sync", s.handleSyncAll)
	r.Post("/repos/"+idPattern+"/sync", s.handleSyncRepo)
	r.Post("/settings/repos", s.handleRepoAdd)
	r.Post("/settings/repos/"+idPattern, s.handleRepoUpdate)
	r.Post("/settings/repos/"+idPattern+"/remove", s.handleRepoRemove)
	r.Post("/settings/interval", s.handleIntervalUpdate)

	// Fichiers statiques.
	r.Get("/static/*", s.handleStatic)
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { s.serveAsset(w, r, "favicon.ico") })
	r.Get("/apple-touch-icon.png", func(w http.ResponseWriter, r *http.Request) {
		s.serveAsset(w, r, "apple-touch-icon.png")
	})
	r.Get("/manifest.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		s.serveAsset(w, r, "manifest.webmanifest")
	})
	return r
}
