// Package github est l'implémentation GitHub de provider.Provider. C'est le SEUL package de Watchoo
// qui importe go-github (google/go-github) : rien de spécifique à GitHub n'en sort, les autres packages
// ne voient que les types et les erreurs de internal/provider.
//
// # Organisation
//
//   - client.go : type Provider, Options, New (construction légère, une fois par synchro) ;
//   - transport.go : http.RoundTripper interne (token, User-Agent, lecture seule, cache ETag, quota,
//     logs) et type Cache ;
//   - quota.go : état du quota partagé (type Quota), lecture des en-têtes de quota ;
//   - errors.go : conversion des réponses et erreurs de go-github vers les erreurs de provider ;
//   - repos.go, commits.go, pulls.go (pull requests, releases, tags), ratelimit.go : appels d'API et
//     conversion des types go-github.
//
// # Token
//
// Le token vient de « gh auth token » (internal/auth) ; le moteur de synchro le redemande à chaque
// synchro et construit un nouveau Provider avec New. Il n'est posé dans l'en-tête Authorization
// (« Bearer <token> ») que par le transport interne, et seulement vers l'origine de BaseURL (schéma,
// hôte et port) : une redirection vers un autre hôte est refusée, et une requête vers un autre hôte ne
// le recevrait pas. Le token n'est jamais journalisé, jamais inclus dans une erreur, jamais stocké
// ailleurs qu'en mémoire dans le transport. Pas de golang.org/x/oauth2.
//
// # Lecture seule
//
// Le transport refuse toute méthode autre que GET et HEAD AVANT tout envoi réseau
// (provider.ErrWriteForbidden).
//
// # Cache ETag et quota
//
// Le cache ETag (Cache) et l'état du quota (Quota) peuvent être partagés entre instances via Options :
// un Provider vit le temps d'une synchro, le cache et le quota d'une synchro à l'autre. Les deux sont
// liés à une empreinte du token (SHA-256, en mémoire seulement) : un autre token repart de zéro. Une
// réponse 304 ne consomme pas de quota.
//
// # Concurrence
//
// Un Provider, un Cache et un Quota sont sûrs pour un usage concurrent (4 dépôts synchronisés en
// parallèle).
//
// # Logs
//
// Niveau Debug seulement : méthode, chemin de l'API (sans paramètres de requête), code HTTP, quota
// restant, « 304 servi depuis le cache ». JAMAIS le token, l'en-tête Authorization, un message de
// commit, un nom de personne ou un e-mail.
package github

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	gogithub "github.com/google/go-github/v93/github"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
)

// errWrapped compose un texte traduit et une erreur enveloppée, avec la ponctuation de la langue
// (« texte : err » en français, « texte: err » en anglais).
func errWrapped(text string, err error) error {
	return fmt.Errorf("%s%s%w", text, i18n.T("provider.err.sep"), err)
}

// Valeurs par défaut et constantes du client.
const (
	// DefaultBaseURL est la racine de l'API REST de GitHub.
	DefaultBaseURL = "https://api.github.com/"
	// DefaultUserAgent est l'en-tête User-Agent utilisé si Options.UserAgent est vide ; Watchoo passe
	// « watchoo/<version> ».
	DefaultUserAgent = "watchoo"
	// DefaultTimeout est le délai maximum d'une requête, redirections et lecture du corps comprises.
	DefaultTimeout = 30 * time.Second
	// PerPage est la taille des pages demandées aux listes de l'API (maximum de GitHub). Elle est
	// fixe, pour que les clés du cache ETag restent stables d'une synchro à l'autre.
	PerPage = 100
	// maxRedirects est le nombre maximum de redirections suivies pour une requête.
	maxRedirects = 10
	// WebBaseURL est la racine des pages web de GitHub (html_url des comptes et des dépôts).
	WebBaseURL = "https://github.com"
)

// Options règle New. Sa valeur zéro convient à l'API publique de GitHub.
type Options struct {
	// BaseURL est la racine de l'API ; vide = DefaultBaseURL. Le « / » final est ajouté s'il manque.
	// Elle doit être en https, sauf vers une adresse de bouclage (127.0.0.1, ::1, localhost), pour les
	// tests (httptest) ; ni identifiants, ni paramètres, ni fragment.
	BaseURL string
	// UserAgent est l'en-tête User-Agent de chaque requête, par exemple « watchoo/v0.2.0 » ; vide =
	// DefaultUserAgent. Il ne doit contenir aucun caractère de contrôle.
	UserAgent string
	// Transport est le transport HTTP de base, sous le transport interne ; nil = http.DefaultTransport.
	// Les tests peuvent y brancher un transport qui compte ou bloque les requêtes.
	Transport http.RoundTripper
	// Timeout est le délai maximum par requête (http.Client.Timeout : redirections et lecture du corps
	// comprises) ; 0 = DefaultTimeout ; négatif refusé.
	Timeout time.Duration
	// Cache est le cache ETag, à partager entre les Provider successifs ; nil = cache propre à ce
	// Provider (NewCache(0, 0)).
	Cache *Cache
	// Quota est l'état du quota, à partager entre les Provider successifs ; nil = état propre à ce
	// Provider.
	Quota *Quota
	// Logger reçoit les logs de diagnostic (niveau Debug seulement) ; nil = slog.Default().
	Logger *slog.Logger
}

// Provider est l'implémentation GitHub de provider.Provider. Il est construit par New et sûr pour un
// usage concurrent. Sa valeur zéro n'est pas utilisable.
type Provider struct {
	// client est le client go-github, branché sur un http.Client dont le transport est tr ; son propre
	// contrôle du quota est désactivé (WithDisableRateLimitCheck) : c'est le transport qui s'en
	// charge, avec l'état partagé quota.
	client *gogithub.Client
	tr     *transport   // transport interne (seul détenteur du token)
	cache  *Cache       // cache ETag (partagé si Options.Cache)
	quota  *Quota       // état du quota (partagé si Options.Quota)
	fp     fingerprint  // empreinte du token, clé de cache et de quota
	logger *slog.Logger // logs de diagnostic, niveau Debug seulement
}

// Vérifie à la compilation que *Provider implémente provider.Provider.
var _ provider.Provider = (*Provider)(nil)

// New construit un Provider GitHub authentifié par token. La construction est légère (aucune requête,
// aucune goroutine) : le moteur de synchro en construit un à chaque synchro, avec le token redemandé à
// gh, en lui passant le même Cache et le même Quota.
//
// Erreurs : token vide ou mal formé (espace, caractère de contrôle) → erreur enveloppant
// provider.ErrUnauthorized, sans citer le token ; BaseURL, UserAgent ou Timeout invalide → erreur
// décrivant le réglage en cause.
func New(token string, opts Options) (*Provider, error) {
	if !validToken(token) {
		return nil, errWrapped(i18n.T("provider.github.client.token"), provider.ErrUnauthorized)
	}
	base, err := parseBaseURL(opts.BaseURL)
	if err != nil {
		return nil, err
	}
	userAgent := opts.UserAgent
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	if strings.IndexFunc(userAgent, unicode.IsControl) >= 0 || !utf8.ValidString(userAgent) {
		return nil, errors.New(i18n.T("provider.github.client.useragent"))
	}
	timeout := opts.Timeout
	switch {
	case timeout < 0:
		return nil, errors.New(i18n.T("provider.github.client.timeout_negative", timeout))
	case timeout == 0:
		timeout = DefaultTimeout
	}
	baseTransport := opts.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	cache := opts.Cache
	if cache == nil {
		cache = NewCache(0, 0)
	}
	quota := opts.Quota
	if quota == nil {
		quota = NewQuota()
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	fp := fingerprintOf(token)
	tr := &transport{
		base:       baseTransport,
		authHeader: "Bearer " + token,
		origin:     &url.URL{Scheme: base.Scheme, Host: base.Host},
		basePath:   base.Path,
		userAgent:  userAgent,
		cache:      cache,
		quota:      quota,
		fp:         fp,
		logger:     logger,
		now:        time.Now,
	}
	httpClient := &http.Client{
		Transport:     tr,
		Timeout:       timeout,
		CheckRedirect: checkRedirect(tr.origin),
	}
	baseStr := base.String()
	client, err := gogithub.NewClient(
		gogithub.WithHTTPClient(httpClient),
		gogithub.WithURLs(&baseStr, &baseStr), // aucun envoi de fichier : l'URL d'upload est celle de l'API
		gogithub.WithUserAgent(userAgent),
		gogithub.WithDisableRateLimitCheck(),
	)
	if err != nil {
		return nil, errWrapped(i18n.T("provider.github.client.create"), err)
	}
	return &Provider{
		client: client,
		tr:     tr,
		cache:  cache,
		quota:  quota,
		fp:     fp,
		logger: logger,
	}, nil
}

// Name implémente provider.Provider : « github ».
func (p *Provider) Name() string { return model.ProviderGitHub }

// String décrit le Provider sans rien en révéler (ni token ni empreinte), pour qu'un %v ou un log
// accidentel reste sans risque.
func (p *Provider) String() string { return "github.Provider" }

// GoString est l'équivalent de String pour %#v.
func (p *Provider) GoString() string { return p.String() }

// validToken indique si token peut être posé dans un en-tête : non vide, UTF-8 valide, sans espace ni
// caractère de contrôle.
func validToken(token string) bool {
	return token != "" && utf8.ValidString(token) &&
		strings.IndexFunc(token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}

// parseBaseURL valide Options.BaseURL (vide = DefaultBaseURL) et la renvoie avec un « / » final.
func parseBaseURL(raw string) (*url.URL, error) {
	if raw == "" {
		raw = DefaultBaseURL
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errWrapped(i18n.T("provider.github.client.baseurl_invalid"), err)
	}
	switch {
	case u.Host == "" || u.Opaque != "":
		return nil, errors.New(i18n.T("provider.github.client.baseurl_not_absolute", raw))
	case u.User != nil:
		return nil, errors.New(i18n.T("provider.github.client.baseurl_credentials"))
	case u.RawQuery != "" || u.Fragment != "" || u.ForceQuery:
		return nil, errors.New(i18n.T("provider.github.client.baseurl_query", raw))
	case u.Scheme == "https":
	case u.Scheme == "http" && isLoopback(u.Hostname()):
	default:
		return nil, errors.New(i18n.T("provider.github.client.baseurl_https", raw))
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
		u.RawPath = ""
	}
	return u, nil
}

// isLoopback indique si host (sans port) désigne la machine locale.
func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sameOrigin indique si u a la même origine (schéma, hôte, port) que origin. Les ports par défaut
// (443 pour https, 80 pour http) sont explicités avant comparaison. Seule l'origine de BaseURL reçoit
// le token.
func sameOrigin(u, origin *url.URL) bool {
	if u == nil || origin == nil {
		return false
	}
	return strings.EqualFold(u.Scheme, origin.Scheme) &&
		strings.EqualFold(u.Hostname(), origin.Hostname()) &&
		portOf(u) == portOf(origin)
}

// portOf renvoie le port de u, ou le port par défaut de son schéma.
func portOf(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}

// checkRedirect est la politique de redirection du http.Client : au plus maxRedirects redirections,
// et seulement vers l'origine de BaseURL (GitHub redirige un dépôt renommé ou transféré vers
// /repositories/{id} sur le même hôte). L'erreur renvoyée enveloppe provider.ErrUnexpected et ne cite
// que l'hôte visé.
func checkRedirect(origin *url.URL) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errWrapped(i18n.T("provider.github.client.too_many_redirects", maxRedirects), provider.ErrUnexpected)
		}
		if !sameOrigin(req.URL, origin) {
			return errWrapped(i18n.T("provider.github.client.redirect_other_host", req.URL.Host), provider.ErrUnexpected)
		}
		return nil
	}
}

// ProfileURL implémente provider.Provider : « https://github.com/<login> », la valeur que l'API donne
// en html_url pour le compte d'un utilisateur ou d'une organisation, sans requête. Vide si login n'a
// pas la forme d'un login (provider.ValidLogin) : vide, caractère interdit, compte d'application
// « nom[bot] » (dont la page est ailleurs, sous /apps/).
func (p *Provider) ProfileURL(login string) string {
	if !provider.ValidLogin(login) {
		return ""
	}
	return WebBaseURL + "/" + login
}
