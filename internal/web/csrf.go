package web

// Protection CSRF de toute requête qui modifie quelque chose (POST) : deux contrôles indépendants.
//
//  1. Origine : http.CrossOriginProtection (Sec-Fetch-Site, sinon Origin comparé à Host) refuse une
//     requête venue d'un autre site.
//  2. Jeton : un jeton aléatoire par navigateur, dans un cookie HttpOnly SameSite=Strict, doit être
//     renvoyé dans le champ de formulaire csrf_token ou l'en-tête X-CSRF-Token (HTMX : hx-headers sur
//     <body>) ; comparaison en temps constant (double soumission du cookie).
//
// Le nom du cookie comprend le port écouté : les cookies ne sont pas isolés par port, deux « watchoo
// serve » sur la même machine ne s'écrasent ainsi pas leur jeton.

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net"
	"net/http"
	"strings"
)

// Noms du champ de formulaire et de l'en-tête qui portent le jeton CSRF.
const (
	CSRFField  = "csrf_token"
	CSRFHeader = "X-CSRF-Token"
)

// csrfTokenBytes est la taille du jeton (avant encodage base64url sans remplissage : 43 caractères).
const csrfTokenBytes = 32

type csrfKey struct{}

// csrfProtector applique les deux contrôles.
type csrfProtector struct {
	cookie string
	origin *http.CrossOriginProtection
}

// newCSRFProtector construit la protection pour l'adresse écoutée listenAddr.
func newCSRFProtector(listenAddr string) *csrfProtector {
	name := "watchoo_csrf"
	if _, port, err := net.SplitHostPort(listenAddr); err == nil && port != "" && port != "0" {
		name += "_" + port
	}
	return &csrfProtector{cookie: name, origin: http.NewCrossOriginProtection()}
}

// middleware place le jeton du navigateur dans le contexte (en créant le cookie s'il manque ou est
// invalide, sauf pour un fichier statique : le cookie n'est posé qu'avec une page ou un fragment HTML,
// isAssetPath) et, pour toute méthode autre que GET, HEAD et OPTIONS, vérifie l'origine puis le jeton :
// échec → 403 (page d'erreur, rien n'est modifié).
func (c *csrfProtector) middleware(s *server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, fromCookie := c.cookieToken(r)
			switch {
			case fromCookie:
			case isAssetPath(r.URL.Path):
				token = "" // aucun cookie posé : un POST sur ce chemin est refusé plus bas
			default:
				token = newCSRFToken()
				http.SetCookie(w, &http.Cookie{
					Name:     c.cookie,
					Value:    token,
					Path:     "/",
					HttpOnly: true,
					Secure:   r.TLS != nil,
					SameSite: http.SameSiteStrictMode,
				})
			}
			r = r.WithContext(context.WithValue(r.Context(), csrfKey{}, token))
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			if err := c.origin.Check(r); err != nil {
				s.logger.Warn("requête refusée : origine étrangère", "methode", r.Method)
				s.renderError(w, r, http.StatusForbidden, "Requête refusée",
					"Cette action doit être lancée depuis l'interface de Watchoo.")
				return
			}
			sent := r.Header.Get(CSRFHeader)
			if sent == "" {
				sent = r.PostFormValue(CSRFField) // corps déjà borné par limitBody
			}
			if !fromCookie || subtle.ConstantTimeCompare([]byte(sent), []byte(token)) != 1 {
				s.logger.Warn("requête refusée : jeton CSRF absent ou invalide", "methode", r.Method)
				s.renderError(w, r, http.StatusForbidden, "Requête refusée",
					"Le jeton de sécurité de la page a expiré : rechargez la page, puis recommencez.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// cookieToken renvoie le jeton du cookie s'il est présent et bien formé.
func (c *csrfProtector) cookieToken(r *http.Request) (string, bool) {
	ck, err := r.Cookie(c.cookie)
	if err != nil || !validCSRFToken(ck.Value) {
		return "", false
	}
	return ck.Value, true
}

// newCSRFToken renvoie un jeton aléatoire (crypto/rand).
func newCSRFToken() string {
	b := make([]byte, csrfTokenBytes)
	_, _ = rand.Read(b) // crypto/rand.Read ne renvoie jamais d'erreur
	return base64.RawURLEncoding.EncodeToString(b)
}

// validCSRFToken indique si s a la forme d'un jeton (43 caractères base64url).
func validCSRFToken(s string) bool {
	if len(s) != base64.RawURLEncoding.EncodedLen(csrfTokenBytes) {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	})
}

// csrfToken renvoie le jeton CSRF de la requête (placé par le middleware) ; vide hors middleware.
func csrfToken(r *http.Request) string {
	token, _ := r.Context().Value(csrfKey{}).(string)
	return token
}
