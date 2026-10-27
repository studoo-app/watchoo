package provider

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Erreurs sentinelles du package, à tester avec errors.Is. Les implémentations les enveloppent avec du
// contexte (le plus souvent dans un *APIError), jamais avec le token, le corps d'une réponse, un message
// de commit, un nom de personne ou un e-mail.
var (
	// ErrNotFound : réponse 404. Sur GitHub, un dépôt privé auquel le compte n'a pas accès répond
	// exactement comme un dépôt inexistant : ne jamais en conclure que le dépôt a été supprimé.
	ErrNotFound = errors.New("dépôt introuvable ou compte sans accès")
	// ErrUnauthorized : réponse 401, le token est refusé (révoqué, expiré, ou vide).
	ErrUnauthorized = errors.New("token GitHub refusé : reconnectez-vous avec « gh auth login »")
	// ErrForbidden : réponse 403 (ou 451) hors quota, par exemple une organisation qui restreint
	// l'accès des applications OAuth tierces. *SSORequiredError la satisfait aussi.
	ErrForbidden = errors.New("accès refusé par GitHub")
	// ErrRateLimited : quota de l'API épuisé ou limite secondaire atteinte. Toujours portée par un
	// *RateLimitError (errors.As) qui donne le moment où réessayer.
	ErrRateLimited = errors.New("quota de l'API GitHub épuisé")
	// ErrEmptyRepository : réponse 409 d'un dépôt sans aucun commit (dépôt d'étudiant tout juste créé).
	// Branches et ListCommits la traduisent en liste vide ; les autres méthodes la renvoient.
	ErrEmptyRepository = errors.New("dépôt vide : aucun commit")
	// ErrUnknownBase : CompareCommits ne trouve plus le commit de base (historique réécrit, par exemple
	// après un force-push). Le moteur de synchro se replie alors (voir Provider.CompareCommits).
	ErrUnknownBase = errors.New("commit de base introuvable : l'historique a peut-être été réécrit (force-push)")
	// ErrWriteForbidden : une requête autre que GET ou HEAD a été bloquée avant tout envoi. Watchoo
	// n'accède à la forge qu'en lecture : cette erreur signale un défaut de programmation.
	ErrWriteForbidden = errors.New("requête en écriture refusée : Watchoo n'accède à GitHub qu'en lecture")
	// ErrInvalidRef : owner, nom de dépôt, branche, SHA ou identifiant invalide ; aucune requête n'est
	// partie.
	ErrInvalidRef = errors.New("référence invalide")
	// ErrUnavailable : GitHub injoignable (réseau, DNS, proxy), délai dépassé ou erreur serveur (5xx).
	// Erreur passagère : réessayer plus tard.
	ErrUnavailable = errors.New("GitHub est injoignable ou indisponible : réessayez plus tard")
	// ErrUnexpected : réponse illisible (JSON invalide) ou code HTTP imprévu.
	ErrUnexpected = errors.New("réponse inattendue de GitHub")
	// ErrUnsupported : la source ne connaît pas la notion demandée (par exemple les pull requests ou
	// les releases d'un dépôt suivi par clone Git local) ; aucune requête n'est partie. Le moteur de
	// synchro la traite comme « non disponible pour cette source », pas comme un échec.
	ErrUnsupported = errors.New("non disponible pour cette source")
	// ErrAccessDenied : git n'a pas pu lire le dépôt distant (clone Git local, V2) : clé SSH ou
	// identifiants Git absents ou refusés, hôte SSH inconnu, dépôt introuvable ou sans accès. git ne
	// distingue pas ces cas de façon fiable ; Watchoo ne demande ni ne stocke aucun identifiant.
	ErrAccessDenied = errors.New("accès refusé : vérifiez la clé SSH ou les identifiants Git de cette machine")
)

// APIError est l'erreur renvoyée pour une requête à la forge qui a échoué (ou qui a été bloquée avant
// l'envoi). Elle enveloppe la cause (Err), à tester avec errors.Is / errors.As : une erreur sentinelle du
// package, un *RateLimitError, un *SSORequiredError, ou ctx.Err().
//
// Elle ne contient jamais le token, ni le corps de la réponse, ni les paramètres de requête.
type APIError struct {
	Op     string // opération, en français (ex. « lecture du dépôt alice/projet »)
	Method string // méthode HTTP (GET) ; vide si sans objet
	Path   string // chemin de l'API, sans paramètres de requête (ex. « /repos/alice/projet ») ; vide si sans objet
	Status int    // code HTTP reçu ; 0 si aucune réponse (réseau, délai, requête bloquée)
	Err    error  // cause
}

// Error renvoie « <Op> (GET /repos/alice/projet, HTTP 404) : <cause> », en omettant les parties vides.
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
			b.WriteString(" : ")
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
		return fmt.Sprintf("%s jusqu'au %s", ErrRateLimited, e.Reset.UTC().Format(timeLayout))
	}
	const msg = "limite secondaire de l'API GitHub atteinte (requêtes trop rapprochées)"
	switch {
	case !e.Reset.IsZero():
		return fmt.Sprintf("%s : réessayez après le %s", msg, e.Reset.UTC().Format(timeLayout))
	case e.RetryAfter > 0:
		return fmt.Sprintf("%s : réessayez dans %s", msg, e.RetryAfter.Round(time.Second))
	default:
		return msg + " : réessayez plus tard"
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
	org := "de ce dépôt"
	if e.Organization != "" {
		org = "« " + e.Organization + " »"
	}
	msg := fmt.Sprintf("l'organisation %s impose l'authentification unique (SAML SSO) : autorisez GitHub CLI "+
		"pour cette organisation (« gh auth refresh », puis accordez l'accès à l'organisation", org)
	if page := ssoPage(e.URL); page != "" {
		msg += ", ou ouvrez " + page
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
