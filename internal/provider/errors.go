package provider

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// i18nError est une erreur sentinelle dont le message vient du catalogue i18n : le texte est traduit
// au moment de l'affichage (Error), dans la langue du processus. La valeur reste comparable avec ==
// (une structure d'un champ) : errors.Is retrouve l'erreur sentinelle à travers les emballages.
type i18nError struct{ key string }

func (e i18nError) Error() string { return i18n.T(e.key) }

// Erreurs sentinelles du package, à tester avec errors.Is. Les implémentations les enveloppent avec du
// contexte (le plus souvent dans un *APIError), jamais avec le token, le corps d'une réponse, un message
// de commit, un nom de personne ou un e-mail.
var (
	// ErrNotFound : réponse 404. Sur GitHub, un dépôt privé auquel le compte n'a pas accès répond
	// exactement comme un dépôt inexistant : ne jamais en conclure que le dépôt a été supprimé.
	ErrNotFound = i18nError{"provider.err.not_found"}
	// ErrUnauthorized : réponse 401, le token est refusé (révoqué, expiré, ou vide).
	ErrUnauthorized = i18nError{"provider.err.unauthorized"}
	// ErrForbidden : réponse 403 (ou 451) hors quota, par exemple une organisation qui restreint
	// l'accès des applications OAuth tierces. *SSORequiredError la satisfait aussi.
	ErrForbidden = i18nError{"provider.err.forbidden"}
	// ErrRateLimited : quota de l'API épuisé ou limite secondaire atteinte. Toujours portée par un
	// *RateLimitError (errors.As) qui donne le moment où réessayer.
	ErrRateLimited = i18nError{"provider.err.rate_limited"}
	// ErrEmptyRepository : réponse 409 d'un dépôt sans aucun commit (dépôt d'étudiant tout juste créé).
	// Branches et ListCommits la traduisent en liste vide ; les autres méthodes la renvoient.
	ErrEmptyRepository = i18nError{"provider.err.empty_repository"}
	// ErrUnknownBase : CompareCommits ne trouve plus le commit de base (historique réécrit, par exemple
	// après un force-push). Le moteur de synchro se replie alors (voir Provider.CompareCommits).
	ErrUnknownBase = i18nError{"provider.err.unknown_base"}
	// ErrWriteForbidden : une requête autre que GET ou HEAD a été bloquée avant tout envoi. Watchoo
	// n'accède à la forge qu'en lecture : cette erreur signale un défaut de programmation.
	ErrWriteForbidden = i18nError{"provider.err.write_forbidden"}
	// ErrInvalidRef : owner, nom de dépôt, branche, SHA ou identifiant invalide ; aucune requête n'est
	// partie.
	ErrInvalidRef = i18nError{"provider.err.invalid_ref"}
	// ErrUnavailable : GitHub injoignable (réseau, DNS, proxy), délai dépassé ou erreur serveur (5xx).
	// Erreur passagère : réessayer plus tard.
	ErrUnavailable = i18nError{"provider.err.unavailable"}
	// ErrUnexpected : réponse illisible (JSON invalide) ou code HTTP imprévu.
	ErrUnexpected = i18nError{"provider.err.unexpected"}
	// ErrUnsupported : la source ne connaît pas la notion demandée (par exemple les pull requests ou
	// les releases d'un dépôt suivi par clone Git local) ; aucune requête n'est partie. Le moteur de
	// synchro la traite comme « non disponible pour cette source », pas comme un échec.
	ErrUnsupported = i18nError{"provider.err.unsupported"}
	// ErrAccessDenied : git n'a pas pu lire le dépôt distant (clone Git local, V2) : clé SSH ou
	// identifiants Git absents ou refusés, hôte SSH inconnu, dépôt introuvable ou sans accès. git ne
	// distingue pas ces cas de façon fiable ; Watchoo ne demande ni ne stocke aucun identifiant.
	ErrAccessDenied = i18nError{"provider.err.access_denied"}
)

// APIError est l'erreur renvoyée pour une requête à la forge qui a échoué (ou qui a été bloquée avant
// l'envoi). Elle enveloppe la cause (Err), à tester avec errors.Is / errors.As : une erreur sentinelle du
// package, un *RateLimitError, un *SSORequiredError, ou ctx.Err().
//
// Elle ne contient jamais le token, ni le corps de la réponse, ni les paramètres de requête.
type APIError struct {
	Op     string // opération, dans la langue de l'interface (ex. « lecture du dépôt alice/projet »)
	Method string // méthode HTTP (GET) ; vide si sans objet
	Path   string // chemin de l'API, sans paramètres de requête (ex. « /repos/alice/projet ») ; vide si sans objet
	Status int    // code HTTP reçu ; 0 si aucune réponse (réseau, délai, requête bloquée)
	Err    error  // cause
}

// Error renvoie « <Op> (GET /repos/alice/projet, HTTP 404) : <cause> », en omettant les parties vides.
// La ponctuation (l'espace avant « : ») suit la langue du processus.
func (e *APIError) Error() string {
	var b strings.Builder
	b.WriteString(e.Op)
	var where []string
	if req := strings.TrimSpace(e.Method + " " + e.Path); req != "" {
		where = append(where, req)
	}
	if e.Status != 0 {
		where = append(where, fmt.Sprintf("HTTP %d", e.Status))
	}
	if len(where) > 0 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("(" + strings.Join(where, ", ") + ")")
	}
	if e.Err != nil {
		if b.Len() > 0 {
			b.WriteString(i18n.T("provider.err.sep"))
		}
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

// Unwrap renvoie la cause, pour errors.Is et errors.As.
func (e *APIError) Unwrap() error { return e.Err }

// RateLimitError signale un quota épuisé (Secondary = false : réponse 403 ou 429 avec
// X-RateLimit-Remaining: 0, ou quota connu comme épuisé avant l'envoi) ou une limite secondaire
// (Secondary = true : 403 ou 429 avec Retry-After ou un message « secondary rate limit »). Elle
// satisfait errors.Is(err, ErrRateLimited).
//
// Le Provider n'attend jamais : c'est au moteur de synchro de patienter jusqu'à Reset.
type RateLimitError struct {
	// Reset est le moment (UTC) à partir duquel réessayer : la remise à zéro du quota
	// (X-RateLimit-Reset) ; pour une limite secondaire, réception de la réponse + RetryAfter si
	// Retry-After est présent, sinon réception + 1 minute (attente minimale recommandée par GitHub).
	// Zéro si inconnu.
	Reset time.Time
	// RetryAfter est le délai demandé par l'en-tête Retry-After ; 0 s'il est absent.
	RetryAfter time.Duration
	// Secondary indique une limite secondaire (requêtes trop rapprochées) plutôt que le quota horaire.
	Secondary bool
}

// timeLayout est le format des dates dans les messages d'erreur (toujours en UTC, pour qu'un message
// soit reproductible ; la conversion en heure locale appartient aux couches d'affichage).
const timeLayout = "2006-01-02 15:04:05 UTC"

// Error décrit la limite atteinte et quand réessayer.
func (e *RateLimitError) Error() string {
	if !e.Secondary {
		if e.Reset.IsZero() {
			return ErrRateLimited.Error()
		}
		return i18n.T("provider.ratelimit.until", ErrRateLimited.Error(), e.Reset.UTC().Format(timeLayout))
	}
	msg := i18n.T("provider.ratelimit.secondary")
	switch {
	case !e.Reset.IsZero():
		return i18n.T("provider.ratelimit.secondary_at", msg, e.Reset.UTC().Format(timeLayout))
	case e.RetryAfter > 0:
		return i18n.T("provider.ratelimit.secondary_in", msg, e.RetryAfter.Round(time.Second))
	default:
		return i18n.T("provider.ratelimit.secondary_later", msg)
	}
}

// Is permet errors.Is(err, ErrRateLimited).
func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// SSORequiredError signale une réponse 403 accompagnée de l'en-tête « X-GitHub-SSO: required; url=… » :
// l'organisation impose l'authentification unique (SAML SSO) et le token de gh n'y est pas autorisé. Il
// faut autoriser l'application GitHub CLI pour cette organisation. Elle satisfait
// errors.Is(err, ErrForbidden).
type SSORequiredError struct {
	// Organization est l'organisation qui impose le SSO (tirée du lien, sinon le propriétaire du
	// dépôt) ; vide si inconnue.
	Organization string
	// URL est le lien d'autorisation donné par GitHub dans l'en-tête X-GitHub-SSO ; vide s'il est
	// absent. Les couches d'affichage peuvent le proposer ; Error ne le reprend pas (il porte un
	// identifiant de demande d'autorisation).
	URL string
}

// Error explique quoi faire, sans reprendre les paramètres du lien d'autorisation.
func (e *SSORequiredError) Error() string {
	org := i18n.T("provider.sso.org_default")
	if e.Organization != "" {
		org = i18n.T("provider.sso.org_named", e.Organization)
	}
	msg := i18n.T("provider.sso.msg", org)
	if page := ssoPage(e.URL); page != "" {
		msg += i18n.T("provider.sso.or_open", page)
	}
	return msg + ")"
}

// Is permet errors.Is(err, ErrForbidden).
func (e *SSORequiredError) Is(target error) bool { return target == ErrForbidden }

// ssoPage renvoie le lien d'autorisation SSO sans paramètres de requête ni fragment, ou "" s'il n'est
// pas une URL https absolue.
func ssoPage(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ""
	}
	u.RawQuery, u.Fragment, u.RawFragment = "", "", ""
	return u.String()
}
