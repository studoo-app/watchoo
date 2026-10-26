package github

// convertError et unexpected sont appelées par les appels d'API (repos.go, commits.go, ratelimit.go),
// qui ne classent jamais elles-mêmes une erreur de go-github. Seuls CompareCommits (404 sur un dépôt
// accessible → ErrUnknownBase) et CommitStats (422 « No commit found » → ErrNotFound) affinent ensuite
// la cause.

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	gogithub "github.com/google/go-github/v93/github"

	"github.com/studoo-app/watchoo/internal/provider"
)

// secondaryRateLimitMessage est le fragment du message d'erreur de GitHub (comparé sans tenir compte
// de la casse) qui signale une limite secondaire.
const secondaryRateLimitMessage = "secondary rate limit"

// errUnreadable est la cause d'une réponse 200 dont le corps n'a pas pu être décodé.
var errUnreadable = fmt.Errorf("réponse illisible (JSON invalide) : %w", provider.ErrUnexpected)

// convertError convertit l'erreur err renvoyée par go-github (avec la réponse resp, éventuellement nil)
// pour l'opération op, lancée avec le contexte ctx de l'appelant, en erreur du package provider. op est
// une description en français, qui peut citer owner/nom du dépôt et un SHA court, par exemple
// « lecture du dépôt alice/projet » ou « liste des commits de alice/projet ». Elle renvoie nil si err
// est nil, et err telle quelle si elle est déjà un *provider.APIError.
//
// Le résultat est un *provider.APIError {Op: op, Method, Path: apiPath de la dernière requête (après
// redirection), Status: code HTTP reçu (0 sans réponse), Err: cause}, où la cause est, dans l'ordre
// de priorité :
//   - le contexte de l'appelant terminé (ctx.Err() non nil) et err issue de cette fin (elle satisfait
//     errors.Is avec context.Canceled, context.DeadlineExceeded ou context.Cause(ctx)) : erreur qui
//     enveloppe ctx.Err() (message « requête annulée » ou « délai de l'appelant dépassé »). C'est
//     l'état de ctx qui décide, jamais la forme de err : un délai du http.Client (Options.Timeout)
//     peut remonter la valeur context.DeadlineExceeded brute (lecture du corps) sans que le contexte
//     de l'appelant soit terminé ;
//   - l'erreur du package provider renvoyée par le transport (refus en écriture, refus préventif de
//     quota *provider.RateLimitError) ou par la politique de redirection (ErrUnexpected), retrouvée à
//     travers *url.Error ;
//   - 401 → provider.ErrUnauthorized ;
//   - 403 ou 429 avec X-RateLimit-Remaining: 0 (*gogithub.RateLimitError) → *provider.RateLimitError
//     {Reset: X-RateLimit-Reset (à défaut, réception + Retry-After), RetryAfter: Retry-After s'il est
//     présent} ;
//   - 403 avec un en-tête X-GitHub-SSO commençant par « required » → *provider.SSORequiredError
//     {Organization: segment {org} de l'url « https://github.com/orgs/{org}/sso… » de l'en-tête, sinon
//     le propriétaire du dépôt s'il est connu par le chemin /repos/{owner}/… ; URL: l'url de l'en-tête,
//     si c'est une URL https absolue} ;
//   - 403 ou 429 avec Retry-After, ou *gogithub.AbuseRateLimitError, ou message d'erreur GitHub
//     contenant « secondary rate limit » (sans tenir compte de la casse), ou 429 sans autre
//     indication → *provider.RateLimitError {Secondary: true, RetryAfter, Reset : réception +
//     Retry-After, sinon réception + secondaryMinWait, comme le recommande GitHub} ; l'état du quota
//     partagé retient les requêtes suivantes jusqu'à Reset (quota.hold) ;
//   - autre 403, ou 451 → provider.ErrForbidden ;
//   - 404 → provider.ErrNotFound ;
//   - 409 → provider.ErrEmptyRepository (sur un GET, GitHub ne répond 409 que pour un dépôt vide) ;
//   - 5xx, erreur réseau, délai dépassé (http.Client.Timeout, avant ou pendant la lecture du corps) →
//     provider.ErrUnavailable, avec la nature de l'échec (« délai dépassé », « connexion
//     impossible »…) ; un délai dépassé du client ne satisfait PAS errors.Is(err,
//     context.DeadlineExceeded), réservé au contexte de l'appelant ;
//   - JSON illisible, 304 sans entrée de cache, 202, tout autre code → provider.ErrUnexpected.
//
// Le message de l'erreur ne reprend JAMAIS le corps de la réponse ni le message d'erreur de GitHub (ils
// servent seulement à classer), ni les paramètres de requête, ni le token, ni un en-tête de la requête.
// Pour une erreur réseau, seule la nature de l'échec est gardée (pas l'URL complète de *url.Error).
func (p *Provider) convertError(ctx context.Context, op string, resp *gogithub.Response, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*provider.APIError); ok {
		return err // déjà convertie
	}
	e := &provider.APIError{Op: op}
	hr := httpResponse(resp, err)
	if hr != nil {
		e.Status = hr.StatusCode
		if req := hr.Request; req != nil {
			e.Method = req.Method
			e.Path = p.tr.apiPath(req.URL)
		}
	}
	ue, _ := errors.AsType[*url.Error](err)
	if e.Path == "" && ue != nil {
		if u, perr := url.Parse(ue.URL); perr == nil && sameOrigin(u, p.tr.origin) {
			e.Method = strings.ToUpper(ue.Op)
			e.Path = p.tr.apiPath(u)
		}
	}
	e.Err = p.cause(ctx, hr, err, ue)
	return e
}

// cause classe err (voir convertError). ctx est le contexte de l'appelant, hr la réponse HTTP reçue
// (nil sans réponse), ue l'erreur du http.Client (nil si l'échec vient d'après l'envoi : lecture ou
// décodage du corps).
func (p *Provider) cause(ctx context.Context, hr *http.Response, err error, ue *url.Error) error {
	if c := contextCause(ctx, err); c != nil {
		return c
	}
	if ue != nil {
		if c := transportCause(ue.Err); c != nil {
			return c
		}
	}
	if hr != nil && hr.StatusCode != http.StatusOK {
		return p.statusCause(hr, err)
	}
	if ue != nil {
		return unavailable(ue.Err)
	}
	if isJSONError(err) {
		return errUnreadable
	}
	if isNetworkError(err) {
		return unavailable(err)
	}
	return provider.ErrUnexpected
}

// ctxError est la cause d'une requête interrompue parce que le contexte de l'appelant est terminé :
// un message en français, et errors.Is(err, context.Canceled) ou errors.Is(err,
// context.DeadlineExceeded).
type ctxError struct {
	msg string
	err error // context.Canceled ou context.DeadlineExceeded
}

func (e *ctxError) Error() string { return e.msg }
func (e *ctxError) Unwrap() error { return e.err }

// contextCause renvoie la cause « contexte de l'appelant terminé », ou nil.
//
// La décision repose sur l'état de ctx, pas sur la forme de err : go-github renvoie ctx.Err() tel quel
// quand le contexte de l'appelant est terminé pendant l'envoi, mais une erreur pendant la lecture du
// corps peut être la valeur context.DeadlineExceeded brute aussi bien pour l'échéance de l'appelant que
// pour le délai du http.Client (Options.Timeout) : le transport interne n'étant pas un *http.Transport,
// le http.Client pose à la fois une échéance sur le contexte de la requête et un minuteur, et la
// lecture peut échouer sur l'échéance avant que le minuteur n'ait marqué le délai. Seul un contexte de
// l'appelant effectivement terminé donne donc cette cause ; sinon, un délai est classé
// provider.ErrUnavailable (« délai dépassé »).
func contextCause(ctx context.Context, err error) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) &&
		!errors.Is(err, context.Cause(ctx)) {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &ctxError{msg: "délai de l'appelant dépassé", err: context.DeadlineExceeded}
	}
	return &ctxError{msg: "requête annulée", err: context.Canceled}
}

// transportCause renvoie l'erreur du package provider produite par le transport ou par la politique
// de redirection (inner est l'erreur enveloppée par *url.Error), ou nil.
func transportCause(inner error) error {
	if rle, ok := errors.AsType[*provider.RateLimitError](inner); ok {
		return rle // refus préventif : quota épuisé ou limite secondaire en cours
	}
	switch {
	case errors.Is(inner, provider.ErrWriteForbidden):
		return provider.ErrWriteForbidden // méthode et chemin sont déjà dans l'APIError
	case errors.Is(inner, provider.ErrUnexpected):
		return inner // redirection refusée : message de Watchoo, qui ne cite que l'hôte visé
	}
	return nil
}

// statusCause classe une réponse d'erreur hr (code autre que 200) reçue de GitHub.
func (p *Provider) statusCause(hr *http.Response, err error) error {
	now := p.tr.now()
	h := hr.Header
	status := hr.StatusCode
	limited := status == http.StatusForbidden || status == http.StatusTooManyRequests
	switch {
	case status == http.StatusUnauthorized:
		return provider.ErrUnauthorized
	case limited && primaryExhausted(h, err):
		return primaryLimit(h, err, now)
	case status == http.StatusForbidden && ssoRequired(h):
		return p.ssoError(hr)
	case limited && (isSecondary(h, err) || status == http.StatusTooManyRequests):
		rle := secondaryLimit(h, now)
		if rle.Reset.After(now) {
			p.quota.hold(p.fp, rle.Reset, true)
		}
		return rle
	case status == http.StatusForbidden || status == http.StatusUnavailableForLegalReasons:
		return provider.ErrForbidden
	case status == http.StatusNotFound:
		return provider.ErrNotFound
	case status == http.StatusConflict:
		return provider.ErrEmptyRepository
	case status >= http.StatusInternalServerError:
		return fmt.Errorf("erreur du serveur : %w", provider.ErrUnavailable)
	}
	return provider.ErrUnexpected
}

// primaryExhausted indique un quota horaire épuisé : X-RateLimit-Remaining vaut 0 (go-github renvoie
// alors un *gogithub.RateLimitError).
func primaryExhausted(h http.Header, err error) bool {
	if _, ok := errors.AsType[*gogithub.RateLimitError](err); ok {
		return true
	}
	remaining, ok := headerInt(h, headerRateRemaining)
	return ok && remaining == 0
}

// primaryLimit construit l'erreur d'un quota horaire épuisé : Reset = X-RateLimit-Reset (à défaut,
// celui lu par go-github, puis réception + Retry-After), RetryAfter = Retry-After s'il est présent.
func primaryLimit(h http.Header, err error, now time.Time) *provider.RateLimitError {
	rle := &provider.RateLimitError{}
	if d, ok := retryAfter(h, now); ok {
		rle.RetryAfter = d
	}
	switch reset, ok := headerInt(h, headerRateReset); {
	case ok && reset > 0:
		rle.Reset = time.Unix(reset, 0).UTC()
	default:
		if gerr, ok := errors.AsType[*gogithub.RateLimitError](err); ok && !gerr.Rate.Reset.IsZero() {
			rle.Reset = gerr.Rate.Reset.UTC()
		} else if rle.RetryAfter > 0 {
			rle.Reset = now.Add(rle.RetryAfter).UTC()
		}
	}
	return rle
}

// isSecondary indique une limite secondaire : en-tête Retry-After, *gogithub.AbuseRateLimitError
// (documentation_url de GitHub), ou message d'erreur contenant « secondary rate limit ».
func isSecondary(h http.Header, err error) bool {
	if strings.TrimSpace(h.Get(headerRetryAfter)) != "" {
		return true
	}
	if _, ok := errors.AsType[*gogithub.AbuseRateLimitError](err); ok {
		return true
	}
	return strings.Contains(strings.ToLower(githubMessage(err)), secondaryRateLimitMessage)
}

// secondaryMinWait est l'attente après une limite secondaire sans en-tête Retry-After : GitHub
// recommande alors d'attendre « au moins une minute » avant de réessayer. X-RateLimit-Reset n'est pas
// utilisé : il désigne la fin de la fenêtre du quota horaire (jusqu'à une heure), sans rapport avec
// une limite secondaire, qui survient quand le quota n'est pas épuisé.
const secondaryMinWait = time.Minute

// secondaryLimit construit l'erreur d'une limite secondaire : RetryAfter = Retry-After, Reset =
// réception + Retry-After, sinon réception + secondaryMinWait.
func secondaryLimit(h http.Header, now time.Time) *provider.RateLimitError {
	rle := &provider.RateLimitError{Secondary: true}
	if d, ok := retryAfter(h, now); ok {
		rle.RetryAfter = d
		rle.Reset = now.Add(d).UTC()
		return rle
	}
	rle.Reset = now.Add(secondaryMinWait).UTC()
	return rle
}

// ssoRequired indique un en-tête « X-GitHub-SSO: required; url=… » (l'organisation impose le SSO et
// le token n'y est pas autorisé). La forme « partial-results; organizations=… » ne compte pas.
func ssoRequired(h http.Header) bool {
	directive, _, _ := strings.Cut(h.Get(headerSSO), ";")
	return strings.EqualFold(strings.TrimSpace(directive), "required")
}

// ssoError construit l'erreur SSO d'une réponse 403 portant « X-GitHub-SSO: required; url=… ».
func (p *Provider) ssoError(hr *http.Response) *provider.SSORequiredError {
	link := ssoURL(hr.Header.Get(headerSSO))
	org := ssoOrganization(link)
	if org == "" && hr.Request != nil {
		org = repoOwner(p.tr.apiPath(hr.Request.URL))
	}
	return &provider.SSORequiredError{Organization: org, URL: link}
}

// ssoURL renvoie le paramètre url de l'en-tête X-GitHub-SSO s'il s'agit d'une URL https absolue (sans
// identifiants ni caractère de contrôle), sinon "".
func ssoURL(header string) string {
	_, params, _ := strings.Cut(header, ";")
	for param := range strings.SplitSeq(params, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "url") {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if strings.ContainsFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			return ""
		}
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return ""
		}
		return value
	}
	return ""
}

// ssoOrganization renvoie l'organisation {org} d'un lien « https://github.com/orgs/{org}/sso… », ou ""
// (lien vide, d'une autre forme, ou nom d'organisation invalide).
func ssoOrganization(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "orgs" || provider.ValidateOwner(parts[1]) != nil {
		return ""
	}
	return parts[1]
}

// repoOwner renvoie le propriétaire d'un chemin d'API « /repos/{owner}/… », ou "".
func repoOwner(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "repos" || provider.ValidateOwner(parts[1]) != nil {
		return ""
	}
	return parts[1]
}

// httpResponse renvoie la réponse HTTP liée à l'erreur : celle de resp, sinon celle portée par
// l'erreur de go-github ; nil s'il n'y en a pas (erreur réseau, requête bloquée).
func httpResponse(resp *gogithub.Response, err error) *http.Response {
	if resp != nil && resp.Response != nil {
		return resp.Response
	}
	if e, ok := errors.AsType[*gogithub.ErrorResponse](err); ok && e.Response != nil {
		return e.Response
	}
	if e, ok := errors.AsType[*gogithub.RateLimitError](err); ok && e.Response != nil {
		return e.Response
	}
	if e, ok := errors.AsType[*gogithub.AbuseRateLimitError](err); ok && e.Response != nil {
		return e.Response
	}
	if e, ok := errors.AsType[*gogithub.TwoFactorAuthError](err); ok && e.Response != nil {
		return e.Response
	}
	if e, ok := errors.AsType[*gogithub.RedirectionError](err); ok && e.Response != nil {
		return e.Response
	}
	return nil
}

// githubMessage renvoie le message d'erreur de GitHub porté par err (champ message du corps), ou "".
// Il sert seulement à classer l'erreur : il n'apparaît jamais dans un message de Watchoo.
func githubMessage(err error) string {
	if e, ok := errors.AsType[*gogithub.ErrorResponse](err); ok {
		return e.Message
	}
	if e, ok := errors.AsType[*gogithub.AbuseRateLimitError](err); ok {
		return e.Message
	}
	if e, ok := errors.AsType[*gogithub.RateLimitError](err); ok {
		return e.Message
	}
	return ""
}

// isJSONError indique une erreur de décodage du JSON d'une réponse.
func isJSONError(err error) bool {
	if _, ok := errors.AsType[*json.SyntaxError](err); ok {
		return true
	}
	if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return true
	}
	_, ok := errors.AsType[*json.InvalidUnmarshalError](err)
	return ok
}

// isNetworkError indique un échec de transport (réseau, délai, connexion interrompue) survenu
// pendant la lecture du corps d'une réponse.
func isNetworkError(err error) bool {
	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE)
}

// unavailable renvoie la cause d'un échec de transport : sa nature, enveloppant
// provider.ErrUnavailable (sans le message d'origine, qui peut citer l'URL ou l'adresse).
func unavailable(err error) error {
	return fmt.Errorf("%s : %w", failureNature(err), provider.ErrUnavailable)
}

// failureNature décrit en français la nature d'un échec de transport, sans URL, adresse ni détail
// venu du serveur (utilisée dans les erreurs et les logs).
func failureNature(err error) string {
	netErr, isNet := errors.AsType[net.Error](err)
	opErr, isOp := errors.AsType[*net.OpError](err)
	switch {
	case isDNSError(err):
		return "nom d'hôte introuvable"
	case (isNet && netErr.Timeout()) || errors.Is(err, context.DeadlineExceeded):
		return "délai dépassé"
	case errors.Is(err, context.Canceled):
		return "requête annulée"
	case isTLSError(err):
		return "connexion sécurisée (TLS) impossible"
	case isOp && opErr.Op == "dial":
		return "connexion impossible"
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF),
		errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE):
		return "connexion interrompue"
	}
	return "erreur réseau"
}

// isDNSError indique un échec de résolution de nom.
func isDNSError(err error) bool {
	_, ok := errors.AsType[*net.DNSError](err)
	return ok
}

// isTLSError indique un échec de la négociation TLS (certificat refusé, enregistrement invalide).
func isTLSError(err error) bool {
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return true
	}
	_, ok := errors.AsType[tls.RecordHeaderError](err)
	return ok
}

// unexpected renvoie l'erreur d'une réponse acceptée par go-github mais inexploitable (corps « null »,
// champ obligatoire absent : identifiant de dépôt nul, SHA vide…) pour l'opération op :
// *provider.APIError {Op, Method, Path, Status de resp s'il est connu, Err: provider.ErrUnexpected}.
func (p *Provider) unexpected(op string, resp *gogithub.Response) error {
	e := &provider.APIError{Op: op, Err: provider.ErrUnexpected}
	if resp != nil && resp.Response != nil {
		e.Status = resp.StatusCode
		if req := resp.Request; req != nil {
			e.Method = req.Method
			e.Path = p.tr.apiPath(req.URL)
		}
	}
	return e
}
