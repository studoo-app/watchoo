package github

// État du quota et lecture des en-têtes de quota, utilisés par le transport (transport.go, errors.go)
// et par les appels d'API (ratelimit.go).

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

// En-têtes de quota et de limite secondaire envoyés par GitHub.
const (
	headerRateLimit     = "X-RateLimit-Limit"
	headerRateRemaining = "X-RateLimit-Remaining"
	headerRateUsed      = "X-RateLimit-Used"
	headerRateReset     = "X-RateLimit-Reset"
	headerRateResource  = "X-RateLimit-Resource"
	headerRetryAfter    = "Retry-After"
	headerSSO           = "X-GitHub-SSO"
)

// coreResource est la ressource de quota des appels REST ordinaires (5 000 requêtes par heure).
const coreResource = "core"

// fingerprint est l'empreinte SHA-256 d'un token. Elle lie le cache ETag et l'état du quota au compte
// qui les a remplis, sans garder le token. Elle reste en mémoire et n'est jamais journalisée ni
// affichée (elle n'a volontairement pas de méthode String).
type fingerprint [sha256.Size]byte

// fingerprintOf renvoie l'empreinte de token.
func fingerprintOf(token string) fingerprint {
	return sha256.Sum256([]byte(token))
}

// Quota est l'état du quota de l'API GitHub (ressource « core ») observé sur les réponses, partagé
// entre les Provider construits avec le même *Quota (Options.Quota) : le moteur de synchro construit un
// Provider par synchro, avec le token redemandé à gh, mais garde le même Quota d'une synchro à l'autre.
//
// L'état appartient au dernier token qui l'a mis à jour : un token différent (autre compte) repart d'un
// état vide. Quota est sûr pour un usage concurrent ; sa valeur zéro est prête à l'emploi.
type Quota struct {
	mu        sync.Mutex
	owner     fingerprint        // token auquel appartient l'état
	hasOwner  bool               // owner est renseigné
	rate      provider.RateLimit // dernier état retenu
	known     bool               // rate est renseigné
	holdUntil time.Time          // Retry-After reçu : aucune requête avant ce moment (zéro = aucune)
	holdSec   bool               // holdUntil vient d'une limite secondaire (sinon d'un quota épuisé)
}

// NewQuota renvoie un état de quota vide.
func NewQuota() *Quota {
	return &Quota{}
}

// String décrit l'état du quota sans l'empreinte du token, pour qu'un %v ou un log accidentel reste
// sans risque.
func (q *Quota) String() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.known {
		return "github.Quota(inconnu)"
	}
	return fmt.Sprintf("github.Quota(%d/%d restantes)", q.rate.Remaining, q.rate.Limit)
}

// GoString est l'équivalent de String pour %#v.
func (q *Quota) GoString() string { return q.String() }

// bind oublie l'état s'il appartient à un autre token que fp. Appelée avec q.mu verrouillé.
func (q *Quota) bind(fp fingerprint) {
	if q.hasOwner && q.owner == fp {
		return
	}
	q.owner, q.hasOwner = fp, true
	q.rate, q.known = provider.RateLimit{}, false
	q.holdUntil, q.holdSec = time.Time{}, false
}

// last renvoie le dernier état retenu pour le token fp ; false si aucun (ou si l'état appartient à un
// autre token, qui n'est alors pas effacé).
func (q *Quota) last(fp fingerprint) (provider.RateLimit, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.hasOwner || q.owner != fp || !q.known {
		return provider.RateLimit{}, false
	}
	return q.rate, true
}

// observe enregistre un état observé pour le token fp (en-têtes d'une réponse, 304 comprises, ou corps
// de GET /rate_limit). Des réponses concurrentes peuvent arriver dans le désordre ; la règle garde un
// état cohérent :
//   - fenêtre plus récente (Reset postérieur) : rl remplace l'état ;
//   - même fenêtre (même Reset) : rl ne remplace l'état que si Remaining ne remonte pas (on garde le
//     minimum observé) ;
//   - fenêtre plus ancienne : rl est ignoré (réponse en retard).
func (q *Quota) observe(fp fingerprint, rl provider.RateLimit) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.bind(fp)
	newer := !q.known || rl.Reset.After(q.rate.Reset)
	sameWindow := q.known && rl.Reset.Equal(q.rate.Reset) && rl.Remaining <= q.rate.Remaining
	if newer || sameWindow {
		q.rate, q.known = rl, true
	}
}

// hold enregistre, pour le token fp, une attente imposée par GitHub : aucune requête avant until. La
// plus lointaine des échéances connues est gardée, avec sa nature : secondary indique une limite
// secondaire, sinon un quota épuisé dont la remise à zéro n'est connue que par Retry-After.
func (q *Quota) hold(fp fingerprint, until time.Time, secondary bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.bind(fp)
	if until.After(q.holdUntil) {
		q.holdUntil, q.holdSec = until, secondary
	}
}

// blocked renvoie l'erreur à renvoyer SANS envoyer la requête quand l'état connu pour le token fp dit
// qu'elle échouerait à coup sûr, ou nil. Deux échéances peuvent la retenir :
//   - l'attente de hold (now avant son échéance) : Reset = échéance, RetryAfter = temps restant,
//     Secondary selon sa nature ;
//   - le quota connu comme épuisé (Remaining <= 0) et sa remise à zéro pas encore atteinte : Reset =
//     remise à zéro, Secondary = false.
//
// Si les deux sont en cours, c'est la plus LOINTAINE qui est renvoyée (à échéance égale, le quota
// épuisé) : aucune requête ne peut réussir avant, et le moteur de synchro, qui se fie à Reset et à
// Secondary, ne se réveille pas trop tôt pour se faire refuser de nouveau.
//
// Le transport ne l'applique pas à GET /rate_limit, qui reste permis quand le quota est épuisé.
func (q *Quota) blocked(fp fingerprint, now time.Time) *provider.RateLimitError {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.hasOwner || q.owner != fp {
		return nil
	}
	held := now.Before(q.holdUntil)
	exhausted := q.known && q.rate.Remaining <= 0 && now.Before(q.rate.Reset)
	switch {
	case held && (!exhausted || q.holdUntil.After(q.rate.Reset)):
		return &provider.RateLimitError{
			Secondary:  q.holdSec,
			Reset:      q.holdUntil.UTC(),
			RetryAfter: q.holdUntil.Sub(now),
		}
	case exhausted:
		return &provider.RateLimitError{Reset: q.rate.Reset.UTC()}
	}
	return nil
}

// rateFromHeader lit l'état du quota dans les en-têtes d'une réponse. Il faut au moins
// X-RateLimit-Remaining et X-RateLimit-Reset valides (entiers positifs ou nuls, Reset en secondes
// depuis l'époque Unix) ; X-RateLimit-Limit est facultatif (0 s'il manque) et X-RateLimit-Used vaut
// Limit - Remaining s'il manque. Un en-tête X-RateLimit-Resource autre que « core » (search, graphql…)
// est ignoré : false. ObservedAt vaut now, en UTC.
func rateFromHeader(h http.Header, now time.Time) (provider.RateLimit, bool) {
	if res := h.Get(headerRateResource); res != "" && !strings.EqualFold(res, coreResource) {
		return provider.RateLimit{}, false
	}
	remaining, ok := headerInt(h, headerRateRemaining)
	if !ok {
		return provider.RateLimit{}, false
	}
	reset, ok := headerInt(h, headerRateReset)
	if !ok || reset == 0 {
		return provider.RateLimit{}, false
	}
	limit, _ := headerInt(h, headerRateLimit)
	used, ok := headerInt(h, headerRateUsed)
	if !ok {
		used = max(limit-remaining, 0)
	}
	return provider.RateLimit{
		Limit:      int(limit),
		Remaining:  int(remaining),
		Used:       int(used),
		Reset:      time.Unix(reset, 0).UTC(),
		ObservedAt: now.UTC(),
	}, true
}

// retryAfter lit l'en-tête Retry-After : un nombre de secondes (forme utilisée par GitHub) ou une date
// HTTP. false s'il est absent ou illisible ; une date passée donne 0.
func retryAfter(h http.Header, now time.Time) (time.Duration, bool) {
	v := strings.TrimSpace(h.Get(headerRetryAfter))
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		if secs < 0 {
			return 0, false
		}
		const maxSecs = int64(365 * 24 * time.Hour / time.Second)
		return time.Duration(min(secs, maxSecs)) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0), true
	}
	return 0, false
}

// headerInt lit un en-tête entier positif ou nul.
func headerInt(h http.Header, key string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(h.Get(key)), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
