package web

// Middlewares communs à toutes les routes (ordre : voir routes.go) : récupération de panique,
// journal d'accès, en-têtes de sécurité, contrôle de l'en-tête Host, authentification basique, borne
// de la taille des corps de requête. Le CSRF est dans csrf.go.

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/studoo-app/watchoo/internal/configshare"
)

// MaxBodyBytes borne la taille du corps d'une requête (formulaires des réglages).
const MaxBodyBytes = 64 << 10

// MaxImportBodyBytes borne le corps des requêtes qui portent un fichier de configuration partagé
// (importPaths) : le fichier (configshare.MaxFileSize) plus le reste du formulaire multipart.
const MaxImportBodyBytes = configshare.MaxFileSize + MaxBodyBytes

// importPaths sont les routes dont le corps porte un fichier de configuration (MaxImportBodyBytes).
var importPaths = []string{"/settings/import", "/settings/import/apply"}

// contentSecurityPolicy n'autorise que les ressources servies par Watchoo, plus les avatars GitHub :
// aucun script ni style en ligne, aucun eval (HTMX est réglé en conséquence dans layout.html).
const contentSecurityPolicy = "default-src 'self'; " +
	"img-src 'self' https://avatars.githubusercontent.com data:; " +
	"script-src 'self'; style-src 'self'; connect-src 'self'; font-src 'self'; " +
	"form-action 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'"

// securityHeaders pose les en-têtes de sécurité sur toutes les réponses. Les pages et fragments ne
// sont jamais mis en cache (données de dépôts privés) ; les fichiers statiques remplacent
// Cache-Control (assets.go).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), interest-cohort=()")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// hostCheck refuse (421) une requête dont l'en-tête Host n'est pas dans s.hosts (protection contre
// le DNS rebinding : une page d'un autre site qui ferait pointer son nom vers 127.0.0.1 n'obtient
// rien). Sans liste (hors boucle locale avec authentification basique), pas de contrôle.
func (s *server) hostCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hosts.allows(r.Host) {
			s.logger.Warn("requête refusée : en-tête Host inattendu")
			s.renderError(w, r, http.StatusMisdirectedRequest, "Adresse inattendue",
				"Ouvrez l'interface par http://localhost ou http://127.0.0.1, avec le port de « watchoo serve ».")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// basicAuth exige l'authentification basique quand elle est configurée : identifiant et mot de passe
// comparés en temps constant (sur leurs empreintes SHA-256, de longueur fixe), les deux toujours
// évalués. Échec → 401 avec WWW-Authenticate. Le mot de passe n'est jamais journalisé.
func (s *server) basicAuth(next http.Handler) http.Handler {
	if !s.auth.Enabled() {
		return next
	}
	wantUser := sha256.Sum256([]byte(s.auth.Username))
	wantPass := sha256.Sum256([]byte(s.auth.Password))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		gotUser := sha256.Sum256([]byte(user))
		gotPass := sha256.Sum256([]byte(pass))
		userOK := subtle.ConstantTimeCompare(gotUser[:], wantUser[:])
		passOK := subtle.ConstantTimeCompare(gotPass[:], wantPass[:])
		if !ok || userOK&passOK != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Watchoo", charset="UTF-8"`)
			s.renderError(w, r, http.StatusUnauthorized, "Authentification requise",
				"Identifiez-vous pour accéder à l'interface de Watchoo.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// limitBody borne le corps des requêtes à MaxBodyBytes, ou MaxImportBodyBytes pour importPaths
// (formulaire trop gros → erreur de lecture *http.MaxBytesError, traitée par le handler : 413 pour
// l'import, requête invalide ailleurs).
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			limit := int64(MaxBodyBytes)
			if slices.Contains(importPaths, r.URL.Path) {
				limit = MaxImportBodyBytes
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

// statusRecorder retient le code de réponse pour le journal d'accès.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap permet à http.ResponseController d'atteindre le ResponseWriter d'origine.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// accessLog journalise chaque requête au niveau Info : méthode, motif de route chi (« /repos/{id} »,
// jamais l'URL réelle ni la query string), code et durée. Ni adresse IP, ni en-tête, ni donnée
// personnelle.
func (s *server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := s.now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		route := routePattern(r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		s.logger.Info("requête http", "methode", r.Method, "route", route, "code", status,
			"duree", s.now().Sub(start).Round(time.Millisecond))
	})
}

// recoverer transforme une panique d'un handler en page d'erreur 500 (sans détail interne) et la
// journalise (route seulement). http.ErrAbortHandler est relancée, comme le fait net/http.
func (s *server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rv := recover()
			if rv == nil {
				return
			}
			if err, ok := rv.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rv)
			}
			route := routePattern(r)
			s.logger.Error("panique dans un gestionnaire http", "route", route)
			s.renderError(w, r, http.StatusInternalServerError, "Erreur interne",
				"Une erreur inattendue est survenue. Le détail est dans le journal de « watchoo serve ».")
		}()
		next.ServeHTTP(w, r)
	})
}

// routePattern renvoie le motif de la route chi de r, sans les contraintes des paramètres
// (« /repos/{id} ») ; « (inconnue) » hors route.
func routePattern(r *http.Request) string {
	rc := chi.RouteContext(r.Context())
	if rc == nil || rc.RoutePattern() == "" {
		return "(inconnue)"
	}
	return strings.ReplaceAll(rc.RoutePattern(), ":[0-9]{1,18}", "")
}
