package github

// Ce fichier contient le http.RoundTripper interne (seul détenteur du token) et le cache ETag (type
// Cache).

import (
	"bytes"
	"container/list"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

// Tailles par défaut du cache ETag (NewCache avec des valeurs nulles ou négatives).
const (
	// DefaultCacheEntries est le nombre maximum d'entrées du cache.
	DefaultCacheEntries = 1024
	// DefaultCacheBodyBytes est la taille maximale d'un corps mis en cache, en octets : une réponse
	// plus grande n'est pas mise en cache (elle est transmise normalement).
	DefaultCacheBodyBytes = 1 << 20
)

// En-têtes HTTP gérés par le transport.
const (
	headerAuthorization   = "Authorization"
	headerUserAgent       = "User-Agent"
	headerETag            = "ETag"
	headerIfNoneMatch     = "If-None-Match"
	headerIfModifiedSince = "If-Modified-Since"
	headerContentLength   = "Content-Length"
	headerDate            = "Date"
	// headerFromCache marque une réponse 200 reconstruite depuis le cache après un 304 (même
	// convention que les caches HTTP habituels, reconnue par go-github).
	headerFromCache = "X-From-Cache"
	// rateLimitHeaderPrefix est le préfixe canonique des en-têtes de quota (X-RateLimit-*).
	rateLimitHeaderPrefix = "X-Ratelimit-"
)

// rateLimitPath est le chemin de GET /rate_limit, gratuit et toujours permis (jamais refusé
// préventivement).
const rateLimitPath = "/rate_limit"

// Cache est le cache ETag des réponses de l'API GitHub, en mémoire seulement (jamais sur disque), à
// partager entre les Provider successifs via Options.Cache. Il est sûr pour un usage concurrent ; sa
// valeur zéro est utilisable (tailles par défaut), mais NewCache est la forme recommandée.
//
// Comportement (implémenté par le transport) :
//   - clé : l'URL complète de la requête (req.URL.String(), paramètres compris) ; seules les requêtes
//     GET vers l'origine de BaseURL sont concernées, sauf celles d'un commit seul
//     (GET /repos/{owner}/{name}/commits/{sha}, voir cacheablePath), jamais mises en cache ;
//   - une entrée garde l'ETag, les en-têtes et le corps d'une réponse 200 portant un en-tête ETag et un
//     corps d'au plus maxBodyBytes octets ; une entrée n'est jamais modifiée après son enregistrement ;
//   - le cache appartient au token qui l'a rempli (empreinte) : une lecture ou une écriture avec une
//     autre empreinte le vide d'abord (pas de réponse d'un compte servie à un autre) ;
//   - au-delà de maxEntries entrées, la moins récemment utilisée est retirée (LRU). La mémoire occupée
//     est donc bornée par maxEntries × maxBodyBytes (plus les en-têtes).
type Cache struct {
	mu           sync.Mutex
	maxEntries   int   // nombre maximum d'entrées
	maxBodyBytes int64 // taille maximale d'un corps mis en cache
	owner        fingerprint
	hasOwner     bool
	entries      map[string]*list.Element // clé (URL complète) → élément de lru, de valeur *cacheEntry
	lru          *list.List               // entrées, de la plus récemment utilisée (devant) à la moins récente
}

// cacheEntry est une réponse 200 mise en cache. Elle est immuable une fois enregistrée : plusieurs
// réponses servies depuis le cache peuvent lire le même corps en même temps.
type cacheEntry struct {
	key    string      // URL complète de la requête
	etag   string      // valeur de l'en-tête ETag, renvoyée telle quelle dans If-None-Match
	header http.Header // en-têtes de la réponse 200
	body   []byte      // corps de la réponse 200
}

// NewCache renvoie un cache ETag vide d'au plus maxEntries entrées, chacune d'un corps d'au plus
// maxBodyBytes octets. Une valeur nulle ou négative prend la valeur par défaut (DefaultCacheEntries,
// DefaultCacheBodyBytes).
func NewCache(maxEntries int, maxBodyBytes int64) *Cache {
	if maxEntries <= 0 {
		maxEntries = DefaultCacheEntries
	}
	if maxBodyBytes <= 0 {
		maxBodyBytes = DefaultCacheBodyBytes
	}
	return &Cache{maxEntries: maxEntries, maxBodyBytes: maxBodyBytes}
}

// Len renvoie le nombre d'entrées du cache.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		return 0
	}
	return c.lru.Len()
}

// String décrit le cache sans rien en révéler : ni empreinte du token, ni contenu des réponses (qui
// portent des messages de commit, des noms et des e-mails). Un %v accidentel reste ainsi sans risque.
func (c *Cache) String() string {
	n := c.Len()
	if n <= 1 {
		return fmt.Sprintf("github.Cache(%d entrée)", n)
	}
	return fmt.Sprintf("github.Cache(%d entrées)", n)
}

// GoString est l'équivalent de String pour %#v.
func (c *Cache) GoString() string { return c.String() }

// String décrit l'entrée sans son contenu (voir Cache.String).
func (e *cacheEntry) String() string { return "github.cacheEntry" }

// GoString est l'équivalent de String pour %#v.
func (e *cacheEntry) GoString() string { return e.String() }

// bodyLimit renvoie la taille maximale d'un corps mis en cache.
func (c *Cache) bodyLimit() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maxBodyBytes <= 0 {
		return DefaultCacheBodyBytes
	}
	return c.maxBodyBytes
}

// bindLocked vide le cache s'il appartient à un autre token que fp, puis le lie à fp ; il initialise
// aussi les structures d'un cache à sa valeur zéro. Appelée avec c.mu verrouillé.
func (c *Cache) bindLocked(fp fingerprint) {
	if c.entries == nil {
		c.entries = make(map[string]*list.Element)
		c.lru = list.New()
	}
	if c.hasOwner && c.owner == fp {
		return
	}
	c.owner, c.hasOwner = fp, true
	clear(c.entries)
	c.lru.Init()
}

// get renvoie l'entrée de clé key pour le token fp (et la marque comme la plus récemment utilisée), ou
// nil. Un cache rempli par un autre token est d'abord vidé.
func (c *Cache) get(fp fingerprint, key string) *cacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindLocked(fp)
	el, ok := c.entries[key]
	if !ok {
		return nil
	}
	c.lru.MoveToFront(el)
	return el.Value.(*cacheEntry)
}

// put enregistre e pour le token fp (un cache rempli par un autre token est d'abord vidé), remplace
// une entrée de même clé et retire les entrées les moins récemment utilisées au-delà de maxEntries.
// Un corps plus grand que la limite n'est pas enregistré : false.
func (c *Cache) put(fp fingerprint, e *cacheEntry) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	maxBody := c.maxBodyBytes
	if maxBody <= 0 {
		maxBody = DefaultCacheBodyBytes
	}
	if int64(len(e.body)) > maxBody {
		return false
	}
	c.bindLocked(fp)
	if el, ok := c.entries[e.key]; ok {
		el.Value = e
		c.lru.MoveToFront(el)
		return true
	}
	c.entries[e.key] = c.lru.PushFront(e)
	maxEntries := c.maxEntries
	if maxEntries <= 0 {
		maxEntries = DefaultCacheEntries
	}
	for c.lru.Len() > maxEntries {
		oldest := c.lru.Back()
		c.lru.Remove(oldest)
		delete(c.entries, oldest.Value.(*cacheEntry).key)
	}
	return true
}

// transport est le http.RoundTripper interne, sous le http.Client de go-github. Il est le seul
// détenteur du token. Sûr pour un usage concurrent (tous ses champs sont fixés par New, puis lus
// seulement ; cache et quota ont leur propre verrou).
//
// RoundTrip, dans l'ordre :
//  1. méthode autre que GET ou HEAD : refus AVANT tout envoi, erreur enveloppant
//     provider.ErrWriteForbidden (le corps éventuel de la requête est fermé, comme le veut le contrat
//     de http.RoundTripper) ;
//  2. refus préventif : si quota.blocked(fp, now) renvoie une erreur, elle est renvoyée sans envoi
//     (sauf pour GET /rate_limit, gratuit et toujours permis) ;
//  3. la requête est clonée (req.Clone : celle de l'appelant n'est jamais modifiée) ; User-Agent =
//     userAgent ; Authorization = authHeader si l'URL a l'origine de BaseURL (sameOrigin), sinon
//     l'en-tête est retiré ;
//  4. GET vers l'origine de BaseURL dont le chemin peut être mis en cache (cacheablePath) et qui a une
//     entrée de cache pour l'URL : If-None-Match = ETag de l'entrée (sans entrée, un If-None-Match ou
//     If-Modified-Since de l'appelant est retiré : tout 304 reçu correspond ainsi à une entrée
//     connue) ;
//  5. envoi par base ;
//  6. toute réponse (304 et erreurs comprises) : rateFromHeader puis quota.observe ; une réponse 403
//     ou 429 avec Retry-After : quota.hold(fp, now + Retry-After), sauf si elle annonce un quota épuisé
//     (X-RateLimit-Remaining: 0) avec une remise à zéro future, qui retient déjà les requêtes (voir
//     observe) ;
//  7. 304 avec une entrée de cache : la réponse 304 est fermée et remplacée par une réponse 200
//     construite depuis l'entrée (même code, mêmes en-têtes, en particulier Link et Content-Type,
//     même corps), dont les en-têtes X-RateLimit-* et Date sont ceux de la 304, avec en plus
//     « X-From-Cache: 1 » ; log Debug « 304 servi depuis le cache » ; Request = la requête reçue.
//     L'entrée utilisée est celle lue à l'étape 4 : elle reste servie même si le cache a été vidé
//     entre-temps ;
//  8. 200 avec ETag sur un GET dont le chemin peut être mis en cache : corps lu (au plus maxBodyBytes + 1 octets) ; s'il tient dans la
//     limite, l'entrée est enregistrée et le corps rendu sous forme d'un lecteur en mémoire ; sinon le
//     corps est rendu intact (début lu + reste du flux) et rien n'est mis en cache. Une erreur de
//     lecture du corps est renvoyée comme une erreur de transport ;
//  9. log Debug : méthode, chemin de l'API (apiPath, sans paramètres), code HTTP, quota restant,
//     provenance du cache. JAMAIS authHeader, ni aucun en-tête de la requête, ni le corps.
//
// Toute réponse renvoyée a pour Request la requête reçue (celle de l'appelant), jamais le clone qui
// porte l'en-tête Authorization : le token ne sort pas du transport, même par http.Response.Request.
type transport struct {
	base       http.RoundTripper // transport de base (Options.Transport ou http.DefaultTransport)
	authHeader string            // « Bearer <token> » : jamais journalisé, jamais dans une erreur
	origin     *url.URL          // schéma et hôte de BaseURL : seule destination qui reçoit le token
	basePath   string            // chemin de BaseURL, avec « / » final (« / » pour api.github.com)
	userAgent  string            // en-tête User-Agent
	cache      *Cache            // cache ETag
	quota      *Quota            // état du quota
	fp         fingerprint       // empreinte du token
	logger     *slog.Logger      // logs Debug
	now        func() time.Time  // horloge (remplaçable dans les tests du package)
}

// RoundTrip implémente http.RoundTripper (voir la description du type).
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	path := t.apiPath(req.URL)
	start := t.now()

	// 1. Lecture seule.
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		closeRequestBody(req)
		t.debug(ctx, "requête en écriture bloquée", req.Method, path)
		return nil, fmt.Errorf("requête %s %s bloquée : %w", req.Method, path, provider.ErrWriteForbidden)
	}

	// 2. Refus préventif : quota épuisé ou limite secondaire en cours.
	own := sameOrigin(req.URL, t.origin)
	if own && path != rateLimitPath {
		if rle := t.quota.blocked(t.fp, start); rle != nil {
			closeRequestBody(req)
			msg := "requête retenue : quota de l'API épuisé"
			if rle.Secondary {
				msg = "requête retenue : limite secondaire de l'API en cours"
			}
			t.debug(ctx, msg, req.Method, path, slog.Time("reprise", rle.Reset))
			return nil, rle
		}
	}

	// 3. Copie de la requête, en-têtes d'identification.
	out := req.Clone(ctx)
	if out.Header == nil {
		out.Header = make(http.Header)
	}
	out.Header.Set(headerUserAgent, t.userAgent)
	if own {
		out.Header.Set(headerAuthorization, t.authHeader)
	} else {
		out.Header.Del(headerAuthorization)
	}

	// 4. Requête conditionnelle si la réponse est en cache.
	var cached *cacheEntry
	key := req.URL.String()
	cacheable := own && req.Method == http.MethodGet && cacheablePath(path)
	if cacheable {
		cached = t.cache.get(t.fp, key)
	}
	if cached != nil {
		out.Header.Set(headerIfNoneMatch, cached.etag)
		out.Header.Del(headerIfModifiedSince)
	} else {
		out.Header.Del(headerIfNoneMatch)
		out.Header.Del(headerIfModifiedSince)
	}

	// 5. Envoi.
	resp, err := t.base.RoundTrip(out)
	if err != nil {
		t.debug(ctx, "échec de la requête à l'API GitHub", req.Method, path,
			slog.String("cause", failureNature(err)), slog.Duration("duree", t.now().Sub(start)))
		return nil, err
	}
	if resp.Body == nil {
		resp.Body = http.NoBody
	}
	resp.Request = req
	received := t.now()

	// 6. Quota.
	var rate provider.RateLimit
	var rateKnown bool
	if own {
		rate, rateKnown = t.observe(resp, received)
	}

	attrs := []slog.Attr{slog.Int("code", resp.StatusCode)}
	if rateKnown {
		attrs = append(attrs, slog.Int("restant", rate.Remaining))
	}
	attrs = append(attrs, slog.Duration("duree", received.Sub(start)))

	// 7. 304 : réponse 200 reconstruite depuis le cache.
	if resp.StatusCode == http.StatusNotModified && cached != nil {
		served := fromCache(req, resp, cached)
		t.debug(ctx, "304 servi depuis le cache", req.Method, path, attrs...)
		return served, nil
	}

	// 8. Mise en cache d'une réponse 200 avec ETag.
	if cacheable && resp.StatusCode == http.StatusOK {
		if etag := resp.Header.Get(headerETag); etag != "" {
			stored, err := t.store(key, etag, resp)
			if err != nil {
				t.debug(ctx, "échec de la lecture d'une réponse de l'API GitHub", req.Method, path,
					slog.String("cause", failureNature(err)))
				return nil, err
			}
			attrs = append(attrs, slog.Bool("mis_en_cache", stored))
		}
	}

	// 9. Log.
	t.debug(ctx, "requête à l'API GitHub", req.Method, path, attrs...)
	return resp, nil
}

// observe met à jour l'état du quota avec les en-têtes de resp, reçue à now, et enregistre l'attente
// annoncée par Retry-After sur une réponse 403 ou 429. Il renvoie l'état lu, s'il y en a un.
//
// Une réponse qui annonce un quota épuisé (X-RateLimit-Remaining: 0) est classée comme telle par
// convertError (statusCause), avec Reset = X-RateLimit-Reset : quand cette remise à zéro est connue et
// future, l'état du quota retient déjà les requêtes jusqu'à elle, et Retry-After n'est pas enregistré.
// Sinon, l'attente est enregistrée comme quota épuisé si Remaining vaut 0 (remise à zéro inconnue :
// convertError prend aussi réception + Retry-After), comme limite secondaire dans les autres cas. Les
// refus préventifs qui suivent décrivent ainsi la même limite, avec le même Reset, que la première
// erreur.
func (t *transport) observe(resp *http.Response, now time.Time) (provider.RateLimit, bool) {
	rl, ok := rateFromHeader(resp.Header, now)
	if ok {
		t.quota.observe(t.fp, rl)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if d, has := retryAfter(resp.Header, now); has && d > 0 {
			remaining, known := headerInt(resp.Header, headerRateRemaining)
			exhausted := known && remaining == 0
			if !(exhausted && ok && rl.Reset.After(now)) {
				t.quota.hold(t.fp, now.Add(d), !exhausted)
			}
		}
	}
	return rl, ok
}

// store lit le corps de resp (réponse 200 portant l'ETag etag) et l'enregistre dans le cache sous la
// clé key s'il tient dans la limite ; le corps de resp est remplacé par un lecteur équivalent. Il
// renvoie true si l'entrée a été enregistrée. En cas d'erreur de lecture, le corps est fermé et
// l'erreur renvoyée.
func (t *transport) store(key, etag string, resp *http.Response) (bool, error) {
	limit := t.cache.bodyLimit()
	if resp.ContentLength > limit {
		return false, nil // trop grand d'après Content-Length : transmis sans être lu
	}
	head, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		_ = resp.Body.Close()
		return false, err
	}
	if int64(len(head)) > limit {
		// Trop grand : le corps est rendu intact (début déjà lu, puis le reste du flux).
		resp.Body = &replayBody{Reader: io.MultiReader(bytes.NewReader(head), resp.Body), Closer: resp.Body}
		return false, nil
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(head))
	resp.ContentLength = int64(len(head))
	stored := t.cache.put(t.fp, &cacheEntry{key: key, etag: etag, header: resp.Header.Clone(), body: head})
	return stored, nil
}

// cacheablePath indique si la réponse d'un GET sur le chemin d'API path (forme apiPath) peut être mise
// en cache. Le seul chemin exclu est celui d'un commit seul : GET /repos/{owner}/{name}/commits/{sha}
// (CommitStats), ou /repositories/{id}/commits/{sha} après la redirection d'un dépôt renommé. Chaque
// commit n'est demandé qu'une fois, pour ses statistiques : l'entrée ne serait jamais relue, elle
// chasserait des entrées utiles (dépôts, branches, pages de commits), et sa réponse porte des extraits
// du code du dépôt privé (patch des fichiers), à ne pas retenir en mémoire. La liste des commits
// (…/commits) et la comparaison restent en cache.
func cacheablePath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case len(parts) == 5 && parts[0] == "repos" && parts[3] == "commits" && parts[4] != "":
		return false
	case len(parts) == 4 && parts[0] == "repositories" && parts[2] == "commits" && parts[3] != "":
		return false
	}
	return true
}

// fromCache construit la réponse 200 servie à la place de la réponse 304 notModified, à partir de
// l'entrée e : en-têtes et corps de l'entrée, en-têtes de quota et Date de la 304. notModified est
// fermée.
func fromCache(req *http.Request, notModified *http.Response, e *cacheEntry) *http.Response {
	_, _ = io.Copy(io.Discard, io.LimitReader(notModified.Body, 4<<10))
	_ = notModified.Body.Close()

	h := e.header.Clone()
	for k := range h {
		if refreshedHeader(k) {
			delete(h, k)
		}
	}
	for k, v := range notModified.Header {
		if refreshedHeader(k) {
			h[k] = slices.Clone(v)
		}
	}
	h.Set(headerFromCache, "1")
	h.Set(headerContentLength, strconv.Itoa(len(e.body)))

	return &http.Response{
		Status:        fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)),
		StatusCode:    http.StatusOK,
		Proto:         notModified.Proto,
		ProtoMajor:    notModified.ProtoMajor,
		ProtoMinor:    notModified.ProtoMinor,
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(e.body)),
		ContentLength: int64(len(e.body)),
		Request:       req,
		TLS:           notModified.TLS,
	}
}

// refreshedHeader indique si l'en-tête k d'une réponse servie depuis le cache doit venir de la
// réponse 304 (état du quota, date) plutôt que de l'entrée.
func refreshedHeader(k string) bool {
	k = http.CanonicalHeaderKey(k)
	return k == headerDate || strings.HasPrefix(k, rateLimitHeaderPrefix)
}

// replayBody est le corps d'une réponse dont le début a déjà été lu : Reader enchaîne le début et le
// reste du flux, Closer ferme le flux d'origine.
type replayBody struct {
	io.Reader
	io.Closer
}

// closeRequestBody ferme le corps d'une requête qui ne sera pas envoyée (contrat de
// http.RoundTripper).
func closeRequestBody(req *http.Request) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
}

// debug écrit un log de niveau Debug : méthode, chemin de l'API, puis attrs. Les valeurs passées ici
// ne contiennent jamais le token, un en-tête de requête, un paramètre de requête ni un corps.
func (t *transport) debug(ctx context.Context, msg, method, path string, attrs ...slog.Attr) {
	if t.logger == nil || !t.logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	all := make([]slog.Attr, 0, len(attrs)+2)
	all = append(all, slog.String("methode", method), slog.String("chemin", path))
	all = append(all, attrs...)
	t.logger.LogAttrs(ctx, slog.LevelDebug, msg, all...)
}

// String décrit le transport sans le token.
func (t *transport) String() string { return "github.transport" }

// GoString est l'équivalent de String pour %#v.
func (t *transport) GoString() string { return t.String() }

// apiPath renvoie le chemin de u relatif à la racine de l'API, avec un « / » initial et sans
// paramètres de requête : « /repos/alice/projet » pour https://api.github.com/repos/alice/projet?page=2.
// C'est la seule forme de l'URL qui apparaît dans les logs et les erreurs.
func (t *transport) apiPath(u *url.URL) string {
	if u == nil {
		return ""
	}
	p := u.Path
	if rest, ok := strings.CutPrefix(p, t.basePath); ok {
		p = "/" + rest
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}
