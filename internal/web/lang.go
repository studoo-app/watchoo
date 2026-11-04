package web

// Langue des pages (documentation/fr/06-interface-web.md, « Langue de l'interface ») : le choix du
// navigateur (sélecteur EN / FR de l'en-tête, cookie watchoo_lang, 1.6.0) s'il en a fait un, sinon le
// réglage lang de l'espace (« en » par défaut, ou « fr »), relu à chaque requête pour qu'un import de
// configuration qui le change s'applique sans redémarrer « watchoo serve ».
//
// La langue est lue une fois par requête (middleware withLang) et portée par son contexte : la page,
// ses fragments et ses messages d'erreur sont tous dans la même langue. config.yaml n'est relu
// (workspace.Load sur une copie de s.ws : fichiers et variables WATCHOO_*, dont WATCHOO_LANG) que si
// sa date de modification ou sa taille a changé depuis la dernière lecture.

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// langKey est la clé du traducteur de la requête dans son contexte.
type langKey struct{}

// fileStamp identifie une version de config.yaml : date de modification et taille (zéro si absent).
type fileStamp struct {
	mod  time.Time
	size int64
}

// langSource donne la langue en vigueur de l'espace. Sûr pour un usage concurrent.
type langSource struct {
	ws *workspace.Workspace // espace servi ; nil = langue fixe

	mu    sync.Mutex
	lang  i18n.Lang
	stamp fileStamp
	read  bool // stamp correspond à une lecture (réussie ou non) de config.yaml
}

// newLangSource renvoie la source de la langue : lang (Options.Config.Lang) tant que config.yaml n'a
// pas été relu.
func newLangSource(ws *workspace.Workspace, lang string) *langSource {
	l := i18n.Lang(workspace.NormalizeLang(lang))
	if !i18n.Valid(l) {
		l = i18n.English
	}
	return &langSource{ws: ws, lang: l}
}

// current renvoie la langue en vigueur. Avec un espace, config.yaml est relu quand il a changé ; un
// fichier illisible ou invalide garde la langue précédente (l'erreur est signalée ailleurs, par la
// page Réglages ou l'import).
func (l *langSource) current() i18n.Lang {
	if l.ws == nil {
		return l.lang
	}
	var stamp fileStamp
	if info, err := os.Stat(l.ws.ConfigPath()); err == nil {
		stamp = fileStamp{mod: info.ModTime(), size: info.Size()}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.read && stamp == l.stamp {
		return l.lang
	}
	l.stamp, l.read = stamp, true
	ws := &workspace.Workspace{Root: l.ws.Root, Dir: l.ws.Dir}
	if err := workspace.Load(ws); err == nil {
		if lang := i18n.Lang(ws.Config.Lang); i18n.Valid(lang) {
			l.lang = lang
		}
	}
	return l.lang
}

// invalidate force la relecture de config.yaml à la prochaine requête (après un import appliqué par
// ce processus, même si la date de modification n'a pas bougé).
func (l *langSource) invalidate() {
	l.mu.Lock()
	l.read = false
	l.mu.Unlock()
}

// langCookieMaxAge est la durée de vie du cookie de langue : un an.
const langCookieMaxAge = 365 * 24 * 60 * 60

// langCookieName renvoie le nom du cookie de langue pour l'adresse écoutée listenAddr : suffixé par le
// port comme le cookie CSRF (les cookies ne distinguent pas les ports : un serve par espace).
func langCookieName(listenAddr string) string {
	name := "watchoo_lang"
	if _, port, err := net.SplitHostPort(listenAddr); err == nil && port != "" && port != "0" {
		name += "_" + port
	}
	return name
}

// requestLang renvoie la langue de la requête : celle du cookie de langue s'il est valide, sinon la
// langue en vigueur de l'espace.
func (s *server) requestLang(r *http.Request) i18n.Lang {
	if ck, err := r.Cookie(s.langCookie); err == nil {
		if lang := i18n.Lang(ck.Value); i18n.Valid(lang) {
			return lang
		}
	}
	return s.lang.current()
}

// withLang place le traducteur de la langue de la requête dans son contexte.
func (s *server) withLang(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tr := i18n.For(s.requestLang(r))
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), langKey{}, tr)))
	})
}

// handleLang (POST /lang) enregistre la langue choisie dans le navigateur (champ lang : « en » ou
// « fr ») et renvoie vers la page d'où vient le choix (champ return, chemin local sans query : les
// filtres d'une page peuvent contenir un e-mail ; sinon « / »).
// Rien n'est écrit dans config.yaml : le choix ne vaut que pour ce navigateur. Langue inconnue → 400.
func (s *server) handleLang(w http.ResponseWriter, r *http.Request) {
	lang := i18n.Lang(formValue(r, "lang"))
	if !i18n.Valid(lang) {
		s.renderError(w, r, http.StatusBadRequest, s.t(r, "web.error.bad_request"), s.t(r, "web.lang.invalid"))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.langCookie,
		Value:    string(lang),
		Path:     "/",
		MaxAge:   langCookieMaxAge,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	redirect(w, r, localReturnPath(formValue(r, "return")))
}

// localReturnPath renvoie le chemin de raw s'il désigne un chemin local de l'interface (« /repos/3 »,
// query et fragment retirés), sinon « / » : jamais d'adresse d'un autre site (pas de redirection
// ouverte). Fonction pure.
func localReturnPath(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.ContainsAny(raw, "\\\r\n\t") {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || !strings.HasPrefix(u.Path, "/") {
		return "/"
	}
	return u.EscapedPath()
}

// tr renvoie le traducteur de la requête (langue de l'espace) ; hors middleware, celui de la langue
// en vigueur.
func (s *server) tr(r *http.Request) *i18n.Translator {
	if tr, ok := r.Context().Value(langKey{}).(*i18n.Translator); ok {
		return tr
	}
	return i18n.For(s.lang.current())
}

// t traduit key dans la langue de la requête.
func (s *server) t(r *http.Request, key string, args ...any) string {
	return s.tr(r).T(key, args...)
}

// pluralText renvoie le texte de key.one ou key.other (selon n et la règle de la langue), formaté
// avec n : « 1 commit », « 3 commits » ; « 0 commits » en anglais, « 0 commit » en français.
func pluralText(tr *i18n.Translator, key string, n int) string {
	one := n == 1 || n == -1
	if tr.Lang() == i18n.French {
		one = n >= -1 && n <= 1
	}
	if one {
		return tr.T(key+".one", n)
	}
	return tr.T(key+".other", n)
}

// jsTexts sont les textes de static/app.js (graphiques), écrits en JSON dans le layout
// (<script type="application/json" id="watchoo-texts">) : jamais de script en ligne (CSP).
type jsTexts struct {
	Locale  string `json:"locale"`  // locale de Intl.DateTimeFormat (« en-US », « fr-FR »)
	Commit  string `json:"commit"`  // « {n} commit »
	Commits string `json:"commits"` // « {n} commits »
	Week    string `json:"week"`    // « Week of {start} to {end} »
	Total   string `json:"total"`   // « Total: {n} »
	Sep     string `json:"sep"`     // séparateur entre une série et son nombre (« : » / « : »)
	Line    string `json:"line"`    // « {n} line added » (débit du diagnostic)
	Lines   string `json:"lines"`   // « {n} lines added »
}

// newJSTexts renvoie les textes de app.js dans la langue de tr.
func newJSTexts(tr *i18n.Translator) jsTexts {
	return jsTexts{
		Locale:  tr.T("web.js.locale"),
		Commit:  tr.T("web.js.commit"),
		Commits: tr.T("web.js.commits"),
		Week:    tr.T("web.js.week"),
		Total:   tr.T("web.js.total"),
		Sep:     tr.T("web.colon"),
		Line:    tr.T("web.js.line"),
		Lines:   tr.T("web.js.lines"),
	}
}
