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
// HEAD est accepté partout où GET l'est (getHead) : même handler, même en-têtes, corps vide (net/http
// n'envoie pas le corps d'une réponse à HEAD).
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
//
// Toute route GET accepte aussi HEAD.

import (
	"net/http"
	"slices"
	"strings"
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

	get := getHead(r)

	// Pages et fragments.
	get("/", s.handleDashboard)
	get("/fragments/dashboard/repos", s.handleDashboardRepos)
	get("/fragments/dashboard/feed", s.handleDashboardFeed)
	get("/repos/"+idPattern, s.handleRepo)
	get("/repos/"+idPattern+"/commits", s.handleRepoCommits)
	get("/contributors", s.handleContributors)
	get("/fragments/contributors", s.handleContributorsTable)
	get("/settings", s.handleSettings)
	get("/settings/repos/"+idPattern+"/remove", s.handleRepoRemoveConfirm)
	get("/fragments/quota", s.handleQuota)
	get("/fragments/sync", s.handleSyncStatus)

	// Actions (POST seulement, protégées par le CSRF).
	r.Post("/sync", s.handleSyncAll)
	r.Post("/repos/"+idPattern+"/sync", s.handleSyncRepo)
	r.Post("/settings/repos", s.handleRepoAdd)
	r.Post("/settings/repos/"+idPattern, s.handleRepoUpdate)
	r.Post("/settings/repos/"+idPattern+"/remove", s.handleRepoRemove)
	r.Post("/settings/interval", s.handleIntervalUpdate)

	// Fichiers statiques.
	get("/static/*", s.handleStatic)
	for _, name := range rootAssets {
		get("/"+name, func(w http.ResponseWriter, r *http.Request) { s.serveAsset(w, r, name) })
	}
	return r
}

// rootAssets sont les fichiers statiques servis à la racine (en plus de /static/*), que les
// navigateurs demandent d'eux-mêmes.
var rootAssets = []string{"favicon.ico", "apple-touch-icon.png", "manifest.webmanifest"}

// getHead renvoie une fonction qui enregistre un handler pour GET et pour HEAD.
func getHead(r chi.Router) func(pattern string, h http.HandlerFunc) {
	return func(pattern string, h http.HandlerFunc) {
		r.Get(pattern, h)
		r.Head(pattern, h)
	}
}

// isAssetPath indique si path désigne un fichier statique (/static/…, ou un de rootAssets) : ni page
// ni fragment HTML, donc pas de cookie CSRF à poser.
func isAssetPath(path string) bool {
	if strings.HasPrefix(path, "/static/") {
		return true
	}
	return slices.Contains(rootAssets, strings.TrimPrefix(path, "/"))
}
