package web

// Petits outils des handlers : paramètres de route et de requête, HTMX, redirections.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/studoo-app/watchoo/internal/store"
)

// maxPage borne le numéro de page accepté dans la query string (au-delà : ramené à maxPage, puis à
// la dernière page par store.CommitPage).
const maxPage = 1_000_000

// maxQueryValue borne la longueur d'un filtre lu dans la query string (branche, auteur).
const maxQueryValue = 200

// repoIDParam lit l'identifiant {id} de la route (déjà contraint à 1 à 18 chiffres par routes.go) ;
// false s'il est absent, nul ou hors de int64.
func repoIDParam(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// pageParam lit le paramètre « page » de la query string : entier >= 1, 1 par défaut ou si invalide,
// borné à maxPage.
func pageParam(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 {
		return 1
	}
	return min(n, maxPage)
}

// queryValue lit le paramètre name de la query string, sans les espaces autour ; vide s'il est trop
// long (plus de maxQueryValue octets) ou contient un caractère de contrôle.
func queryValue(r *http.Request, name string) string {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if len(v) > maxQueryValue || strings.ContainsFunc(v, func(c rune) bool { return c < 0x20 || c == 0x7f }) {
		return ""
	}
	return v
}

// formValue lit le champ name d'un formulaire POST (corps borné par limitBody), sans les espaces
// autour.
func formValue(r *http.Request, name string) string {
	return strings.TrimSpace(r.PostFormValue(name))
}

// isHTMX indique une requête HTMX (en-tête HX-Request) : on répond alors par un fragment.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// redirect répond 303 See Other vers path, un chemin LOCAL fixé par le handler (jamais une valeur de
// la requête : pas de redirection ouverte). Pour une requête HTMX, l'en-tête HX-Redirect fait la même
// chose côté navigateur.
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", path)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}

// repoLookupError répond à l'échec de la lecture d'un dépôt : 404 si store.ErrNotFound, sinon 500.
func (s *server) repoLookupError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	s.serverError(w, r, err)
}
