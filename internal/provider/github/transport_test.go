package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v93/github"

	"github.com/studoo-app/watchoo/internal/provider"
)

// Helpers des tests du transport (préfixe tr). Aucun appel réseau réel : uniquement httptest.

// trEpoch est l'instant de départ de l'horloge des tests.
var trEpoch = time.Unix(1_800_000_000, 0).UTC()

// Données personnelles factices glissées dans les corps : elles ne doivent apparaître ni dans un log
// ni dans une erreur.
const (
	trSecretMessage = "Message secret de commit"
	trSecretEmail   = "etudiant@example.com"
)

// trClock est une horloge réglable, sûre en concurrence.
type trClock struct{ ns atomic.Int64 }

func newTrClock(t time.Time) *trClock {
	c := &trClock{}
	c.set(t)
	return c
}

func (c *trClock) now() time.Time      { return time.Unix(0, c.ns.Load()).UTC() }
func (c *trClock) set(t time.Time)     { c.ns.Store(t.UnixNano()) }
func (c *trClock) add(d time.Duration) { c.ns.Add(int64(d)) }

// trRequest est une requête reçue par trServer.
type trRequest struct {
	Method, Path, RawQuery string
	IfNoneMatch            string
	Authorization          string
	UserAgent              string
	Status                 int
	Date                   string
	Remaining              string
}

// trServer est un faux GitHub minimal : ETag par URL, réponses 304, en-têtes de quota (le quota ne
// baisse que sur les réponses 200, comme sur GitHub), handlers spécifiques par chemin.
type trServer struct {
	t   testing.TB
	srv *httptest.Server

	mu         sync.Mutex
	remaining  int
	reset      int64
	versions   map[string]int              // URL relative (chemin?paramètres) → version de la ressource
	bodies     map[string]string           // chemin → corps fixe (sinon corps généré)
	links      map[string]string           // chemin → en-tête Link
	handlers   map[string]http.HandlerFunc // chemin → handler spécifique (sans ETag ni quota automatiques)
	reqs       []trRequest
	onNotModif func() // appelée juste avant l'envoi d'un 304
}

func newTrServer(t testing.TB) *trServer {
	s := &trServer{
		t:         t,
		remaining: 5000,
		reset:     trEpoch.Add(time.Hour).Unix(),
		versions:  map[string]int{},
		bodies:    map[string]string{},
		links:     map[string]string{},
		handlers:  map[string]http.HandlerFunc{},
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

// trStatusWriter retient le code HTTP envoyé.
type trStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *trStatusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *trStatusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *trStatusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *trServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	idx := len(s.reqs)
	s.reqs = append(s.reqs, trRequest{
		Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery,
		IfNoneMatch:   r.Header.Get("If-None-Match"),
		Authorization: r.Header.Get("Authorization"),
		UserAgent:     r.Header.Get("User-Agent"),
	})
	special := s.handlers[r.URL.Path]
	s.mu.Unlock()

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.t.Errorf("requête %s %s reçue : seules GET et HEAD sont permises", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sw := &trStatusWriter{ResponseWriter: w}
	defer func() {
		s.mu.Lock()
		s.reqs[idx].Status = sw.status
		s.reqs[idx].Date = w.Header().Get("Date")
		s.reqs[idx].Remaining = w.Header().Get("X-RateLimit-Remaining")
		s.mu.Unlock()
	}()
	if special != nil {
		special(sw, r)
		return
	}

	s.mu.Lock()
	key := r.URL.RequestURI()
	version := max(s.versions[key], 1)
	etag := fmt.Sprintf(`W/"%s-v%d"`, key, version)
	h := w.Header()
	h.Set("Date", trEpoch.Add(time.Duration(idx)*time.Minute).Format(http.TimeFormat))
	if r.Header.Get("If-None-Match") == etag {
		s.rateHeaders(h)
		h.Set("ETag", etag)
		onNotModif := s.onNotModif
		s.mu.Unlock()
		if onNotModif != nil {
			onNotModif()
		}
		sw.WriteHeader(http.StatusNotModified)
		return
	}
	s.remaining--
	s.rateHeaders(h)
	h.Set("ETag", etag)
	h.Set("Content-Type", "application/json; charset=utf-8")
	if link := s.links[r.URL.Path]; link != "" {
		h.Set("Link", link)
	}
	body, ok := s.bodies[r.URL.Path]
	if !ok {
		body = fmt.Sprintf(`{"url":%q,"version":%d,"message":%q,"email":%q}`, key, version, trSecretMessage, trSecretEmail)
	}
	s.mu.Unlock()
	sw.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(sw, body)
}

// rateHeaders pose les en-têtes de quota. Appelée avec s.mu verrouillé.
func (s *trServer) rateHeaders(h http.Header) {
	h.Set("X-RateLimit-Limit", "5000")
	h.Set("X-RateLimit-Remaining", strconv.Itoa(s.remaining))
	h.Set("X-RateLimit-Used", strconv.Itoa(5000-s.remaining))
	h.Set("X-RateLimit-Reset", strconv.FormatInt(s.reset, 10))
	h.Set("X-RateLimit-Resource", "core")
}

func (s *trServer) handle(path string, h http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[path] = h
}

func (s *trServer) setBody(path, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodies[path] = body
}

func (s *trServer) setLink(path, link string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[path] = link
}

// bump change la version (donc l'ETag et le corps) de la ressource d'URL relative key.
func (s *trServer) bump(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versions[key] = max(s.versions[key], 1) + 1
}

func (s *trServer) requests() []trRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]trRequest(nil), s.reqs...)
}

func (s *trServer) last() trRequest {
	reqs := s.requests()
	if len(reqs) == 0 {
		s.t.Fatal("aucune requête reçue")
	}
	return reqs[len(reqs)-1]
}

// trNew construit un Provider vers srv (BaseURL forcée), avec l'horloge clock si elle n'est pas nil.
func trNew(t testing.TB, baseURL string, token string, opts Options, clock *trClock) *Provider {
	t.Helper()
	opts.BaseURL = baseURL
	p, err := New(token, opts)
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	if clock != nil {
		p.tr.now = clock.now
	}
	return p
}

// trGet envoie GET path (relatif à BaseURL) par go-github et décode la réponse JSON.
func trGet(ctx context.Context, p *Provider, path string) (map[string]any, *gogithub.Response, error) {
	req, err := p.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var out map[string]any
	resp, err := p.client.Do(req, &out)
	return out, resp, err
}

// trMustGet est trGet qui fait échouer le test en cas d'erreur.
func trMustGet(t testing.TB, p *Provider, path string) (map[string]any, *gogithub.Response) {
	t.Helper()
	out, resp, err := trGet(context.Background(), p, path)
	if err != nil {
		t.Fatalf("GET %s : %v", path, err)
	}
	return out, resp
}

// trLogs capture les logs JSON d'un Provider.
type trLogs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *trLogs) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(b)
}

func (l *trLogs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// records renvoie les enregistrements de log décodés.
func (l *trLogs) records(t testing.TB) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(l.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log illisible %q : %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

func trLogger(l *trLogs, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(l, &slog.HandlerOptions{Level: level}))
}

// trRoundTripFunc adapte une fonction en http.RoundTripper.
type trRoundTripFunc func(*http.Request) (*http.Response, error)

func (f trRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// trCloseRecorder est un corps de requête qui retient sa fermeture.
type trCloseRecorder struct {
	io.Reader
	closed atomic.Bool
}

func (c *trCloseRecorder) Close() error {
	c.closed.Store(true)
	return nil
}

// TestTransportETag vérifie le cycle ETag : enregistrement d'un 200, requête conditionnelle, 304 servi
// avec les mêmes données, quota inchangé, nouvelle version de la ressource.
func TestTransportETag(t *testing.T) {
	s := newTrServer(t)
	clock := newTrClock(trEpoch)
	p := trNew(t, s.srv.URL, testToken, Options{}, clock)
	const path = "repos/alice/projet"

	first, resp := trMustGet(t, p, path)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-From-Cache") != "" {
		t.Fatalf("première réponse : %d, X-From-Cache %q", resp.StatusCode, resp.Header.Get("X-From-Cache"))
	}
	if got := s.last().IfNoneMatch; got != "" {
		t.Errorf("première requête : If-None-Match = %q, veut vide", got)
	}
	if p.cache.Len() != 1 {
		t.Fatalf("cache : %d entrées, veut 1", p.cache.Len())
	}
	before, ok := p.quota.last(p.fp)
	if !ok || before.Remaining != 4999 || before.Limit != 5000 || before.Used != 1 ||
		!before.ObservedAt.Equal(trEpoch) || !before.Reset.Equal(trEpoch.Add(time.Hour)) {
		t.Fatalf("quota après le 200 = %+v, %v", before, ok)
	}
	etag := resp.Header.Get("ETag")

	// Deuxième requête : conditionnelle, 304, mêmes données, quota inchangé.
	clock.add(time.Minute)
	second, resp := trMustGet(t, p, path)
	got := s.last()
	if got.IfNoneMatch != etag || got.Status != http.StatusNotModified {
		t.Fatalf("deuxième requête : If-None-Match %q (veut %q), code %d", got.IfNoneMatch, etag, got.Status)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-From-Cache") != "1" {
		t.Errorf("304 servi : code %d, X-From-Cache %q", resp.StatusCode, resp.Header.Get("X-From-Cache"))
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("304 servi : %v, veut %v", second, first)
	}
	after, _ := p.quota.last(p.fp)
	if after.Remaining != before.Remaining {
		t.Errorf("un 304 ne doit pas réduire le quota suivi : %d → %d", before.Remaining, after.Remaining)
	}
	if !after.ObservedAt.Equal(trEpoch.Add(time.Minute)) {
		t.Errorf("le 304 doit mettre à jour l'observation du quota : %v", after.ObservedAt)
	}

	// Nouvelle version de la ressource : 200, cache mis à jour, puis 304 sur le nouvel ETag.
	s.bump("/" + path)
	third, resp := trMustGet(t, p, path)
	if s.last().Status != http.StatusOK || third["version"] != 2.0 || resp.Header.Get("X-From-Cache") != "" {
		t.Fatalf("nouvelle version : code %d, %v", s.last().Status, third)
	}
	if p.cache.Len() != 1 {
		t.Errorf("cache : %d entrées, veut 1 (entrée remplacée)", p.cache.Len())
	}
	fourth, _ := trMustGet(t, p, path)
	if got := s.last(); got.Status != http.StatusNotModified || got.IfNoneMatch != resp.Header.Get("ETag") {
		t.Errorf("après mise à jour : code %d, If-None-Match %q", got.Status, got.IfNoneMatch)
	}
	if !reflect.DeepEqual(third, fourth) {
		t.Errorf("304 servi après mise à jour : %v, veut %v", fourth, third)
	}
}

// TestTransportCachedHeaders vérifie qu'une réponse servie depuis le cache garde les en-têtes de
// l'entrée (Link, donc la pagination ; Content-Type) mais prend le quota et la date de la 304.
func TestTransportCachedHeaders(t *testing.T) {
	s := newTrServer(t)
	p := trNew(t, s.srv.URL, testToken, Options{}, newTrClock(trEpoch))
	const path = "repos/alice/projet/commits"
	link := fmt.Sprintf(`<%s/%s?page=2&per_page=100>; rel="next", <%s/%s?page=3&per_page=100>; rel="last"`,
		s.srv.URL, path, s.srv.URL, path)
	s.setLink("/"+path, link)

	_, first := trMustGet(t, p, path+"?per_page=100")
	if first.NextPage != 2 || first.LastPage != 3 {
		t.Fatalf("pagination du 200 : next %d, last %d", first.NextPage, first.LastPage)
	}
	trMustGet(t, p, "repos/alice/autre") // fait baisser le quota côté serveur

	_, cached := trMustGet(t, p, path+"?per_page=100")
	notModified := s.last()
	if notModified.Status != http.StatusNotModified {
		t.Fatalf("code %d, veut 304", notModified.Status)
	}
	if cached.NextPage != 2 || cached.LastPage != 3 {
		t.Errorf("pagination servie depuis le cache : next %d, last %d", cached.NextPage, cached.LastPage)
	}
	if ct := cached.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	if got := cached.Header.Get("X-RateLimit-Remaining"); got != notModified.Remaining || got != "4998" {
		t.Errorf("X-RateLimit-Remaining = %q, veut celui de la 304 (%q)", got, notModified.Remaining)
	}
	if got := cached.Header.Get("Date"); got != notModified.Date {
		t.Errorf("Date = %q, veut celle de la 304 (%q)", got, notModified.Date)
	}
	if cached.Rate.Remaining != 4998 {
		t.Errorf("Rate vu par go-github = %d, veut 4998", cached.Rate.Remaining)
	}
	if cl := cached.ContentLength; cl <= 0 || cached.Header.Get("Content-Length") != strconv.FormatInt(cl, 10) {
		t.Errorf("Content-Length incohérent : %d / %q", cl, cached.Header.Get("Content-Length"))
	}
}

// TestTransportCacheKeyIsFullURL vérifie que la clé du cache est l'URL complète, paramètres compris.
func TestTransportCacheKeyIsFullURL(t *testing.T) {
	s := newTrServer(t)
	p := trNew(t, s.srv.URL, testToken, Options{}, nil)
	page1, _ := trMustGet(t, p, "repos/alice/projet/commits?page=1&per_page=100")
	page2, _ := trMustGet(t, p, "repos/alice/projet/commits?page=2&per_page=100")
	if reflect.DeepEqual(page1, page2) || p.cache.Len() != 2 {
		t.Fatalf("deux pages, deux entrées attendues : %d entrées", p.cache.Len())
	}
	again, _ := trMustGet(t, p, "repos/alice/projet/commits?page=2&per_page=100")
	got := s.last()
	if got.Status != http.StatusNotModified || !strings.Contains(got.IfNoneMatch, "page=2") {
		t.Errorf("page 2 : code %d, If-None-Match %q", got.Status, got.IfNoneMatch)
	}
	if !reflect.DeepEqual(again, page2) {
		t.Errorf("page 2 servie depuis le cache : %v, veut %v", again, page2)
	}
	trMustGet(t, p, "repos/alice/projet/commits?page=3&per_page=100")
	if got := s.last(); got.IfNoneMatch != "" {
		t.Errorf("page 3 jamais vue : If-None-Match %q, veut vide", got.IfNoneMatch)
	}
}

// TestTransportCacheTokenChange vérifie qu'un cache et un quota partagés ne servent jamais les
// réponses ou l'état d'un token à un autre.
func TestTransportCacheTokenChange(t *testing.T) {
	s := newTrServer(t)
	cache, quota := NewCache(0, 0), NewQuota()
	const other = testToken + "_autre"
	p1 := trNew(t, s.srv.URL, testToken, Options{Cache: cache, Quota: quota}, nil)
	p2 := trNew(t, s.srv.URL, other, Options{Cache: cache, Quota: quota}, nil)
	const path = "repos/alice/projet"

	trMustGet(t, p1, path)
	trMustGet(t, p1, path)
	if got := s.last(); got.Status != http.StatusNotModified || got.Authorization != "Bearer "+testToken {
		t.Fatalf("même token : code %d", got.Status)
	}

	// Autre token : pas de requête conditionnelle, le cache est vidé puis rempli pour lui.
	trMustGet(t, p2, path)
	if got := s.last(); got.IfNoneMatch != "" || got.Status != http.StatusOK || got.Authorization != "Bearer "+other {
		t.Errorf("autre token : If-None-Match %q, code %d", got.IfNoneMatch, got.Status)
	}
	if cache.Len() != 1 {
		t.Errorf("cache : %d entrées, veut 1", cache.Len())
	}
	if _, ok := p1.quota.last(p1.fp); ok {
		t.Error("le quota de l'ancien token ne doit plus être visible")
	}
	if _, ok := p2.quota.last(p2.fp); !ok {
		t.Error("le quota du nouveau token doit être connu")
	}

	// Retour au premier token : rien ne vient du cache du second.
	trMustGet(t, p1, path)
	if got := s.last(); got.IfNoneMatch != "" || got.Status != http.StatusOK {
		t.Errorf("retour au premier token : If-None-Match %q, code %d", got.IfNoneMatch, got.Status)
	}

	// Un nouveau Provider construit avec le même token (nouvelle synchro) profite du cache.
	p3 := trNew(t, s.srv.URL, testToken, Options{Cache: cache, Quota: quota}, nil)
	trMustGet(t, p3, path)
	if got := s.last(); got.Status != http.StatusNotModified {
		t.Errorf("nouveau Provider, même token : code %d, veut 304", got.Status)
	}
}

// TestTransportCacheBodyLimit vérifie qu'un corps plus grand que la limite n'est pas mis en cache et
// qu'il est transmis intact, avec ou sans Content-Length.
func TestTransportCacheBodyLimit(t *testing.T) {
	s := newTrServer(t)
	const limit = 64
	p := trNew(t, s.srv.URL, testToken, Options{Cache: NewCache(10, limit)}, nil)

	small := `{"n":"` + strings.Repeat("a", limit-8) + `"}` // exactement 64 octets
	big := `{"n":"` + strings.Repeat("b", 4*limit) + `"}`
	s.setBody("/small", small)
	s.setBody("/big", big)
	s.handle("/chunked", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"chunked"`)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, big[:10])
		w.(http.Flusher).Flush() // longueur inconnue : transfert par morceaux
		_, _ = io.WriteString(w, big[10:])
	})
	if len(small) != limit {
		t.Fatalf("corps de test de %d octets", len(small))
	}

	out, _ := trMustGet(t, p, "small")
	if out["n"] != strings.Repeat("a", limit-8) || p.cache.Len() != 1 {
		t.Fatalf("petit corps : %v, %d entrées", out, p.cache.Len())
	}
	for _, path := range []string{"big", "chunked"} {
		out, _ := trMustGet(t, p, path)
		if out["n"] != strings.Repeat("b", 4*limit) {
			t.Errorf("%s : corps tronqué ou altéré", path)
		}
		if p.cache.Len() != 1 {
			t.Errorf("%s : %d entrées, un grand corps ne doit pas être mis en cache", path, p.cache.Len())
		}
		trMustGet(t, p, path)
		if got := s.last(); got.IfNoneMatch != "" {
			t.Errorf("%s : If-None-Match %q sans entrée de cache", path, got.IfNoneMatch)
		}
	}
}

// TestTransportCacheLRU vérifie la borne du nombre d'entrées et le retrait de la moins récemment
// utilisée.
func TestTransportCacheLRU(t *testing.T) {
	s := newTrServer(t)
	p := trNew(t, s.srv.URL, testToken, Options{Cache: NewCache(2, 0)}, nil)
	conditional := func(path string) bool {
		t.Helper()
		trMustGet(t, p, path)
		return s.last().IfNoneMatch != ""
	}
	conditional("a")
	conditional("b")
	if !conditional("a") { // a redevient la plus récente
		t.Fatal("a doit être en cache")
	}
	conditional("c") // retire b
	if p.cache.Len() != 2 {
		t.Fatalf("cache : %d entrées, veut 2", p.cache.Len())
	}
	if !conditional("a") {
		t.Error("a doit être restée en cache")
	}
	if conditional("b") { // retire c
		t.Error("b aurait dû être retirée")
	}
	if conditional("c") {
		t.Error("c aurait dû être retirée")
	}
}

// TestCacheUnit teste Cache directement : liaison au token, valeur zéro, remplacement, borne.
func TestCacheUnit(t *testing.T) {
	fp, other := fingerprintOf(testToken), fingerprintOf(testToken+"_autre")
	entry := func(key string, n int) *cacheEntry {
		return &cacheEntry{key: key, etag: `"` + key + `"`, header: http.Header{}, body: bytes.Repeat([]byte("x"), n)}
	}

	var zero Cache // valeur zéro utilisable
	if zero.Len() != 0 || zero.get(fp, "a") != nil {
		t.Fatal("cache zéro non vide")
	}
	if !zero.put(fp, entry("a", 10)) || zero.Len() != 1 {
		t.Fatal("cache zéro : put")
	}
	if zero.put(fp, entry("big", DefaultCacheBodyBytes+1)) {
		t.Error("cache zéro : un corps au-delà de la limite par défaut ne doit pas être enregistré")
	}

	c := NewCache(3, 16)
	if c.put(fp, entry("trop-grand", 17)) || c.Len() != 0 {
		t.Error("corps de 17 octets avec une limite de 16 : refusé attendu")
	}
	for _, k := range []string{"a", "b", "c"} {
		c.put(fp, entry(k, 1))
	}
	c.put(fp, entry("b", 2)) // remplacement : pas de nouvelle entrée
	if c.Len() != 3 || len(c.get(fp, "b").body) != 2 {
		t.Fatalf("remplacement : %d entrées", c.Len())
	}
	c.put(fp, entry("d", 1)) // retire a, la moins récemment utilisée
	if c.get(fp, "a") != nil || c.get(fp, "c") == nil || c.Len() != 3 {
		t.Errorf("LRU : a doit être retirée, c gardée")
	}

	// Lecture par un autre token : vidé, rien n'est servi.
	if c.get(other, "c") != nil || c.Len() != 0 {
		t.Errorf("autre token : rien ne doit être servi (%d entrées)", c.Len())
	}
	if c.get(fp, "c") != nil {
		t.Error("le cache vidé ne doit pas revenir pour l'ancien token")
	}

	// NewCache avec des valeurs nulles : valeurs par défaut.
	d := NewCache(0, -1)
	if d.maxEntries != DefaultCacheEntries || d.maxBodyBytes != DefaultCacheBodyBytes {
		t.Errorf("NewCache(0, -1) = %d, %d", d.maxEntries, d.maxBodyBytes)
	}
}

// TestCacheNeverPrintsContents vérifie qu'afficher le cache ne révèle ni le contenu des réponses ni
// l'empreinte du token.
func TestCacheNeverPrintsContents(t *testing.T) {
	fp := fingerprintOf(testToken)
	c := NewCache(0, 0)
	e := &cacheEntry{key: "https://api.github.com/x", etag: `"e"`, header: http.Header{}, body: []byte(trSecretMessage + " " + trSecretEmail)}
	c.put(fp, e)
	for _, s := range []string{
		fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), fmt.Sprintf("%s", c),
		fmt.Sprintf("%v", e), fmt.Sprintf("%+v", e), fmt.Sprintf("%#v", e),
	} {
		for _, f := range []string{trSecretMessage, trSecretEmail, fmt.Sprintf("%x", fp[:4]), fmt.Sprint(fp[0], " ", fp[1])} {
			if strings.Contains(s, f) {
				t.Errorf("%q révèle %q", s, f)
			}
		}
	}
	if got := c.String(); got != "github.Cache(1 entrée)" {
		t.Errorf("String = %q", got)
	}
}

// TestTransport304AfterEviction vérifie qu'un 304 reste servi quand l'entrée a été retirée du cache
// pendant la requête (autre token, LRU) : l'entrée lue avant l'envoi est utilisée.
func TestTransport304AfterEviction(t *testing.T) {
	s := newTrServer(t)
	cache := NewCache(0, 0)
	p := trNew(t, s.srv.URL, testToken, Options{Cache: cache}, nil)
	first, _ := trMustGet(t, p, "repos/alice/projet")
	s.mu.Lock()
	s.onNotModif = func() { cache.get(fingerprintOf(testToken+"_autre"), "x") } // vide le cache
	s.mu.Unlock()
	second, resp := trMustGet(t, p, "repos/alice/projet")
	if s.last().Status != http.StatusNotModified || cache.Len() != 0 {
		t.Fatalf("code %d, %d entrées", s.last().Status, cache.Len())
	}
	if resp.StatusCode != http.StatusOK || !reflect.DeepEqual(first, second) {
		t.Errorf("304 après retrait de l'entrée : %d, %v", resp.StatusCode, second)
	}
}

// TestTransportRequestUntouched vérifie que la requête de l'appelant n'est jamais modifiée, que la
// réponse ne référence jamais la copie qui porte le token, et que les en-têtes conditionnels de
// l'appelant sont retirés quand rien n'est en cache.
func TestTransportRequestUntouched(t *testing.T) {
	s := newTrServer(t)
	p := trNew(t, s.srv.URL, testToken, Options{UserAgent: "watchoo/test"}, nil)
	for i := range 2 { // 200 puis 304
		req, _ := http.NewRequest(http.MethodGet, s.srv.URL+"/repos/alice/projet", nil)
		req.Header.Set("If-None-Match", `"fabrique"`)
		req.Header.Set("If-Modified-Since", "Mon, 26 Oct 2026 10:00:00 GMT")
		before := req.Header.Clone()
		resp, err := p.tr.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if !reflect.DeepEqual(req.Header, before) {
			t.Errorf("requête %d : en-têtes de l'appelant modifiés : %v", i, req.Header)
		}
		if resp.Request != req || resp.Request.Header.Get("Authorization") != "" {
			t.Errorf("requête %d : la réponse doit référencer la requête de l'appelant, sans token", i)
		}
		got := s.last()
		if got.Authorization != "Bearer "+testToken || got.UserAgent != "watchoo/test" {
			t.Errorf("requête %d : Authorization ou User-Agent absent", i)
		}
		if i == 0 && got.IfNoneMatch != "" {
			t.Errorf("If-None-Match de l'appelant transmis sans entrée de cache : %q", got.IfNoneMatch)
		}
		if i == 1 && (got.Status != http.StatusNotModified || got.IfNoneMatch == `"fabrique"`) {
			t.Errorf("deuxième requête : code %d, If-None-Match %q", got.Status, got.IfNoneMatch)
		}
	}
}

// TestTransportHead vérifie qu'une requête HEAD passe, sans cache, en mettant à jour le quota.
func TestTransportHead(t *testing.T) {
	s := newTrServer(t)
	p := trNew(t, s.srv.URL, testToken, Options{}, newTrClock(trEpoch))
	req, _ := http.NewRequest(http.MethodHead, s.srv.URL+"/repos/alice/projet", nil)
	resp, err := p.client.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || p.cache.Len() != 0 {
		t.Errorf("HEAD : code %d, %d entrées en cache", resp.StatusCode, p.cache.Len())
	}
	if _, ok := p.quota.last(p.fp); !ok {
		t.Error("HEAD : le quota doit être mis à jour")
	}
}

// TestTransportReadOnly vérifie, par go-github, que toute requête en écriture est refusée avant
// l'envoi (aucune requête reçue), corps fermé, et que l'erreur convertie le dit.
func TestTransportReadOnly(t *testing.T) {
	s := newTrServer(t)
	base := &countingTransport{next: http.DefaultTransport}
	p := trNew(t, s.srv.URL, testToken, Options{Transport: base}, nil)
	ctx := context.Background()

	resp, err := p.client.Repositories.Delete(ctx, "alice", "projet")
	e := p.convertError(context.Background(), "suppression du dépôt alice/projet", resp, err)
	var apiErr *provider.APIError
	if !errors.Is(e, provider.ErrWriteForbidden) || !errors.As(e, &apiErr) {
		t.Fatalf("Delete : %v", e)
	}
	if apiErr.Method != http.MethodDelete || apiErr.Path != "/repos/alice/projet" || apiErr.Status != 0 {
		t.Errorf("APIError = %+v", apiErr)
	}

	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete, http.MethodOptions, "PROPFIND"} {
		req, err := p.client.NewRequest(ctx, method, "repos/alice/projet/git/refs", map[string]string{"ref": "refs/heads/x"})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := p.client.Do(req, nil)
		if e := p.convertError(context.Background(), "écriture", resp, err); !errors.Is(e, provider.ErrWriteForbidden) {
			t.Errorf("%s : %v, veut ErrWriteForbidden", method, e)
		} else if strings.Contains(e.Error(), testToken) {
			t.Errorf("%s : l'erreur cite le token", method)
		}

		body := &trCloseRecorder{Reader: strings.NewReader("{}")}
		raw, _ := http.NewRequest(method, s.srv.URL+"/repos/alice/projet", body)
		if _, err := p.tr.RoundTrip(raw); !errors.Is(err, provider.ErrWriteForbidden) {
			t.Errorf("%s direct : %v", method, err)
		}
		if !body.closed.Load() {
			t.Errorf("%s : le corps de la requête refusée doit être fermé", method)
		}
	}
	if n := base.n.Load(); n != 0 || len(s.requests()) != 0 {
		t.Errorf("%d requête(s) envoyée(s), %d reçue(s) : aucune attendue", n, len(s.requests()))
	}
}

// TestTransportQuotaFromHeaders vérifie la mise à jour du quota par chaque réponse, erreurs
// comprises, et l'état partagé entre Provider construits avec le même Quota.
func TestTransportQuotaFromHeaders(t *testing.T) {
	s := newTrServer(t)
	s.handle("/repos/alice/absent", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.remaining--
		s.rateHeaders(w.Header())
		s.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	})
	s.handle("/search/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "1")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(trEpoch.Add(time.Minute).Unix(), 10))
		w.Header().Set("X-RateLimit-Resource", "search")
		_, _ = io.WriteString(w, `{}`)
	})
	quota := NewQuota()
	clock := newTrClock(trEpoch)
	p := trNew(t, s.srv.URL, testToken, Options{Quota: quota}, clock)

	trMustGet(t, p, "repos/alice/projet")
	if rl, _ := quota.last(p.fp); rl.Remaining != 4999 {
		t.Fatalf("Remaining = %d, veut 4999", rl.Remaining)
	}
	_, resp, err := trGet(context.Background(), p, "repos/alice/absent")
	if !errors.Is(p.convertError(context.Background(), "lecture", resp, err), provider.ErrNotFound) {
		t.Fatalf("404 attendu : %v", err)
	}
	if rl, _ := quota.last(p.fp); rl.Remaining != 4998 {
		t.Errorf("une réponse d'erreur doit aussi mettre à jour le quota : Remaining = %d", rl.Remaining)
	}
	trMustGet(t, p, "search/code")
	if rl, _ := quota.last(p.fp); rl.Remaining != 4998 {
		t.Errorf("une autre ressource que core ne doit pas toucher au quota : Remaining = %d", rl.Remaining)
	}

	// Un autre Provider (synchro suivante) avec le même Quota voit l'état.
	p2 := trNew(t, s.srv.URL, testToken, Options{Quota: quota}, clock)
	if rl, ok := p2.quota.last(p2.fp); !ok || rl.Remaining != 4998 {
		t.Errorf("quota partagé : %+v, %v", rl, ok)
	}
}

// TestTransportQuotaExhausted vérifie le refus préventif quand le quota est épuisé : aucune requête
// ne part jusqu'à la remise à zéro, sauf GET /rate_limit.
func TestTransportQuotaExhausted(t *testing.T) {
	s := newTrServer(t)
	reset := trEpoch.Add(30 * time.Minute)
	s.handle("/repos/alice/projet", func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-RateLimit-Limit", "5000")
		h.Set("X-RateLimit-Remaining", "0")
		h.Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"API rate limit exceeded for user ID 1."}`)
	})
	s.handle("/rate_limit", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"resources":{"core":{"limit":5000,"remaining":0,"used":5000,"reset":1}}}`)
	})
	base := &countingTransport{next: http.DefaultTransport}
	quota := NewQuota()
	clock := newTrClock(trEpoch)
	p := trNew(t, s.srv.URL, testToken, Options{Transport: base, Quota: quota}, clock)

	_, resp, err := trGet(context.Background(), p, "repos/alice/projet")
	e := p.convertError(context.Background(), "lecture du dépôt alice/projet", resp, err)
	var rle *provider.RateLimitError
	if !errors.As(e, &rle) || rle.Secondary || !rle.Reset.Equal(reset) {
		t.Fatalf("quota épuisé : %v", e)
	}
	sent := base.n.Load()

	// Refus préventif, y compris pour un autre Provider partageant le Quota.
	p2 := trNew(t, s.srv.URL, testToken, Options{Transport: base, Quota: quota}, clock)
	for _, pr := range []*Provider{p, p2} {
		_, resp, err := trGet(context.Background(), pr, "repos/alice/autre?per_page=100")
		e := pr.convertError(context.Background(), "lecture du dépôt alice/autre", resp, err)
		var apiErr *provider.APIError
		if !errors.As(e, &rle) || rle.Secondary || !rle.Reset.Equal(reset) || !errors.As(e, &apiErr) {
			t.Fatalf("refus préventif attendu : %v", e)
		}
		if apiErr.Status != 0 || apiErr.Method != http.MethodGet || apiErr.Path != "/repos/alice/autre" {
			t.Errorf("APIError du refus préventif = %+v", apiErr)
		}
		if strings.Contains(e.Error(), "per_page") || strings.Contains(e.Error(), "127.0.0.1") {
			t.Errorf("l'erreur cite l'URL : %v", e)
		}
	}
	if base.n.Load() != sent {
		t.Errorf("%d requête(s) envoyée(s) malgré le quota épuisé", base.n.Load()-sent)
	}

	// GET /rate_limit reste permis.
	if _, _, err := p.client.RateLimit.Get(context.Background()); err != nil {
		t.Errorf("GET /rate_limit : %v", err)
	}
	if base.n.Load() != sent+1 {
		t.Error("GET /rate_limit doit partir malgré le quota épuisé")
	}

	// Après la remise à zéro : les requêtes repartent.
	clock.set(reset)
	trMustGet(t, p, "repos/alice/autre")
	if base.n.Load() != sent+2 {
		t.Error("après la remise à zéro, la requête doit partir")
	}
}

// TestTransportSecondaryHold vérifie qu'un Retry-After sur 403 ou 429 retient les requêtes suivantes
// jusqu'à l'échéance, sauf GET /rate_limit.
func TestTransportSecondaryHold(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			s := newTrServer(t)
			s.handle("/repos/alice/projet", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "30")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"message":"You have exceeded a secondary rate limit."}`)
			})
			s.handle("/rate_limit", func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, `{"resources":{"core":{"limit":5000,"remaining":4000,"used":1000,"reset":1}}}`)
			})
			base := &countingTransport{next: http.DefaultTransport}
			clock := newTrClock(trEpoch)
			p := trNew(t, s.srv.URL, testToken, Options{Transport: base}, clock)

			_, resp, err := trGet(context.Background(), p, "repos/alice/projet")
			var rle *provider.RateLimitError
			if e := p.convertError(context.Background(), "lecture", resp, err); !errors.As(e, &rle) || !rle.Secondary ||
				rle.RetryAfter != 30*time.Second || !rle.Reset.Equal(trEpoch.Add(30*time.Second)) {
				t.Fatalf("limite secondaire : %v", e)
			}
			sent := base.n.Load()
			clock.add(10 * time.Second)
			_, resp, err = trGet(context.Background(), p, "repos/alice/autre")
			if e := p.convertError(context.Background(), "lecture", resp, err); !errors.As(e, &rle) || !rle.Secondary ||
				rle.RetryAfter != 20*time.Second {
				t.Fatalf("refus préventif attendu : %v", e)
			}
			if base.n.Load() != sent {
				t.Error("aucune requête ne doit partir pendant la limite secondaire")
			}
			if _, _, err := p.client.RateLimit.Get(context.Background()); err != nil || base.n.Load() != sent+1 {
				t.Errorf("GET /rate_limit doit rester permis : %v", err)
			}
			clock.add(20 * time.Second)
			trMustGet(t, p, "repos/alice/autre")
		})
	}
}

// TestTransportPrimaryWithRetryAfter : un 403 ou 429 qui annonce un quota épuisé (X-RateLimit-Remaining:
// 0) avec aussi un Retry-After (GitHub peut envoyer les deux). La première erreur et les refus préventifs
// qui suivent décrivent la même limite : quota épuisé (Secondary = false) jusqu'à X-RateLimit-Reset.
// Non-régression : le Retry-After était enregistré comme limite secondaire, et les refus suivants
// disaient « limite secondaire » avec un Reset trop tôt (réception + Retry-After). Sans date de remise
// à zéro, l'attente vient de Retry-After, toujours comme quota épuisé.
func TestTransportPrimaryWithRetryAfter(t *testing.T) {
	reset := trEpoch.Add(30 * time.Minute)
	for _, tt := range []struct {
		name    string
		status  int
		headers map[string]string
		reset   time.Time // Reset attendu, sur la première erreur comme sur les refus préventifs
		resume  time.Duration
	}{
		{"403 avec remise à zéro", http.StatusForbidden,
			map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Limit": "5000",
				"X-RateLimit-Reset": strconv.FormatInt(reset.Unix(), 10), "Retry-After": "60"},
			reset, 30 * time.Minute},
		{"429 avec remise à zéro", http.StatusTooManyRequests,
			map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Limit": "5000",
				"X-RateLimit-Reset": strconv.FormatInt(reset.Unix(), 10), "Retry-After": "60"},
			reset, 30 * time.Minute},
		{"403 sans remise à zéro", http.StatusForbidden,
			map[string]string{"X-RateLimit-Remaining": "0", "Retry-After": "60"},
			trEpoch.Add(time.Minute), time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newTrServer(t)
			s.handle("/repos/alice/projet", func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"message":"API rate limit exceeded for user ID 1."}`)
			})
			base := &countingTransport{next: http.DefaultTransport}
			clock := newTrClock(trEpoch)
			p := trNew(t, s.srv.URL, testToken, Options{Transport: base}, clock)
			check := func(what string, e error) {
				t.Helper()
				var rle *provider.RateLimitError
				if !errors.As(e, &rle) {
					t.Fatalf("%s : %v, veut une RateLimitError", what, e)
				}
				if rle.Secondary || !rle.Reset.Equal(tt.reset) {
					t.Errorf("%s : Secondary %v, Reset %s ; veut quota épuisé jusqu'au %s", what, rle.Secondary, rle.Reset, tt.reset)
				}
				if strings.Contains(e.Error(), "secondary") {
					t.Errorf("%s : %q ne doit pas parler de limite secondaire", what, e)
				}
			}

			_, resp, err := trGet(context.Background(), p, "repos/alice/projet")
			check("première erreur", p.convertError(context.Background(), "lecture", resp, err))
			sent := base.n.Load()
			for _, after := range []time.Duration{10 * time.Second, 70 * time.Second} {
				if after >= tt.resume {
					break
				}
				clock.set(trEpoch.Add(after))
				_, resp, err = trGet(context.Background(), p, "repos/alice/autre")
				check(fmt.Sprintf("refus préventif à +%s", after), p.convertError(context.Background(), "lecture", resp, err))
			}
			if base.n.Load() != sent {
				t.Errorf("%d requête(s) envoyée(s) malgré le quota épuisé", base.n.Load()-sent)
			}
			clock.set(trEpoch.Add(tt.resume))
			trMustGet(t, p, "repos/alice/autre")
		})
	}
}

// TestTransportNeverLeaks parcourt les principaux chemins du transport (200, 304, erreurs, refus,
// échec réseau) et vérifie que ni le token, ni l'en-tête Authorization, ni les données personnelles
// des corps, ni les paramètres de requête n'apparaissent dans les logs ou les erreurs.
func TestTransportNeverLeaks(t *testing.T) {
	s := newTrServer(t)
	errorBody := fmt.Sprintf(`{"message":"%s %s %s"}`, trSecretMessage, trSecretEmail, testToken)
	for path, status := range map[string]int{
		"/repos/alice/absent": http.StatusNotFound, "/repos/alice/refus": http.StatusUnauthorized,
		"/repos/alice/vide": http.StatusConflict, "/repos/alice/panne": http.StatusBadGateway,
	} {
		s.handle(path, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, errorBody)
		})
	}
	s.handle("/repos/ecole/sso", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-GitHub-SSO", "required; url=https://github.com/orgs/ecole/sso?authorization_request=SECRET42")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, errorBody)
	})
	s.handle("/repos/alice/lent", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, errorBody)
	})
	logs := &trLogs{}
	logger := trLogger(logs, slog.LevelDebug)
	clock := newTrClock(trEpoch)
	p := trNew(t, s.srv.URL, testToken, Options{Logger: logger}, clock)
	ctx := context.Background()

	var errs []error
	collect := func(resp *gogithub.Response, err error) {
		if err != nil {
			errs = append(errs, p.convertError(context.Background(), "opération", resp, err))
		}
		if resp != nil && resp.Request != nil && resp.Request.Header.Get("Authorization") != "" {
			t.Errorf("la réponse expose l'en-tête Authorization (%s)", resp.Request.URL.Path)
		}
	}
	for _, path := range []string{
		"repos/alice/projet?per_page=100&sha=main", "repos/alice/projet?per_page=100&sha=main", // 200 puis 304
		"repos/alice/absent", "repos/alice/refus", "repos/alice/vide", "repos/alice/panne", "repos/ecole/sso",
		"repos/alice/lent", "repos/alice/projet", // la dernière est retenue (limite secondaire)
	} {
		_, resp, err := trGet(ctx, p, path)
		collect(resp, err)
	}
	resp, err := p.client.Repositories.Delete(ctx, "alice", "projet")
	collect(resp, err)

	// Échec réseau : serveur fermé.
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	pc := trNew(t, closed.URL, testToken, Options{Logger: logger}, clock)
	_, resp, err = trGet(ctx, pc, "repos/alice/projet?per_page=100")
	if err == nil {
		t.Fatal("serveur fermé : erreur attendue")
	}
	errs = append(errs, pc.convertError(context.Background(), "opération", resp, err))

	if len(errs) != 9 {
		t.Fatalf("%d erreurs, veut 9", len(errs))
	}
	forbidden := []string{testToken, "Bearer", "uthorization", trSecretMessage, trSecretEmail, "per_page", "sha=main", "SECRET42"}
	for _, e := range errs {
		for _, f := range forbidden {
			if strings.Contains(e.Error(), f) {
				t.Errorf("l'erreur %q contient %q", e, f)
			}
		}
		if strings.Contains(e.Error(), strings.TrimPrefix(closed.URL, "http://")) {
			t.Errorf("l'erreur réseau cite l'adresse : %q", e)
		}
	}
	out := logs.String()
	for _, f := range forbidden {
		if strings.Contains(out, f) {
			t.Errorf("les logs contiennent %q :\n%s", f, out)
		}
	}
	recs := logs.records(t)
	if len(recs) < 10 {
		t.Errorf("%d logs seulement", len(recs))
	}
	for _, rec := range recs {
		if rec["level"] != "DEBUG" {
			t.Errorf("log de niveau %v : seul Debug est permis (%v)", rec["level"], rec["msg"])
		}
	}

	// Au niveau Info, le transport n'écrit rien.
	quiet := &trLogs{}
	pq := trNew(t, s.srv.URL, testToken, Options{Logger: trLogger(quiet, slog.LevelInfo)}, nil)
	trMustGet(t, pq, "repos/alice/projet")
	_, _, _ = trGet(ctx, pq, "repos/alice/absent")
	if quiet.String() != "" {
		t.Errorf("logs au niveau Info : %s", quiet.String())
	}
}

// TestTransportLogs vérifie le contenu des logs Debug : méthode, chemin sans paramètres, code, quota
// restant, provenance du cache.
func TestTransportLogs(t *testing.T) {
	s := newTrServer(t)
	logs := &trLogs{}
	p := trNew(t, s.srv.URL, testToken, Options{Logger: trLogger(logs, slog.LevelDebug)}, newTrClock(trEpoch))
	trMustGet(t, p, "repos/alice/projet/commits?per_page=100")
	trMustGet(t, p, "repos/alice/projet/commits?per_page=100")
	recs := logs.records(t)
	if len(recs) != 2 {
		t.Fatalf("%d logs, veut 2 :\n%s", len(recs), logs.String())
	}
	want := []struct {
		msg    string
		code   float64
		cached any
	}{
		{"requête à l'API GitHub", 200, true},
		{"304 servi depuis le cache", 304, nil},
	}
	for i, w := range want {
		rec := recs[i]
		if rec["msg"] != w.msg || rec["methode"] != "GET" || rec["chemin"] != "/repos/alice/projet/commits" ||
			rec["code"] != w.code || rec["restant"] != 4999.0 || rec["mis_en_cache"] != w.cached {
			t.Errorf("log %d = %v", i, rec)
		}
	}
}

// TestTransportReadErrorWhileCaching vérifie qu'une coupure pendant la lecture d'un corps à mettre en
// cache donne une erreur ErrUnavailable, sans rien mettre en cache.
func TestTransportReadErrorWhileCaching(t *testing.T) {
	s := newTrServer(t)
	s.handle("/repos/alice/projet", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"coupe"`)
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, `{"id":`) // le serveur ferme la connexion : corps incomplet
	})
	p := trNew(t, s.srv.URL, testToken, Options{}, nil)
	_, resp, err := trGet(context.Background(), p, "repos/alice/projet")
	e := p.convertError(context.Background(), "lecture du dépôt alice/projet", resp, err)
	if !errors.Is(e, provider.ErrUnavailable) || !strings.Contains(e.Error(), "connection interrupted") {
		t.Errorf("corps incomplet : %v", e)
	}
	if p.cache.Len() != 0 {
		t.Errorf("rien ne doit être mis en cache : %d entrées", p.cache.Len())
	}
}

// TestTransportConcurrent fait travailler 50 goroutines en parallèle, chacune avec son Provider
// (comme une synchro par goroutine) sur un Cache et un Quota partagés et petits (évictions
// fréquentes) : à lancer avec -race. Chaque réponse doit correspondre à son URL.
func TestTransportConcurrent(t *testing.T) {
	s := newTrServer(t)
	cache, quota := NewCache(8, 0), NewQuota()
	shared := trNew(t, s.srv.URL, testToken, Options{Cache: cache, Quota: quota}, nil)
	const goroutines, rounds, paths = 50, 10, 12
	providers := make([]*Provider, goroutines)
	for g := range providers {
		providers[g] = shared
		if g%2 == 1 { // une goroutine sur deux a son propre Provider (même token, cache et quota partagés)
			providers[g] = trNew(t, s.srv.URL, testToken, Options{Cache: cache, Quota: quota}, nil)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*rounds)
	for g, p := range providers {
		wg.Go(func() {
			for r := range rounds {
				key := fmt.Sprintf("/repos/alice/projet-%d/commits?per_page=100", (g+r)%paths)
				out, _, err := trGet(context.Background(), p, strings.TrimPrefix(key, "/"))
				switch {
				case err != nil:
					errs <- fmt.Errorf("%s : %w", key, err)
				case out["url"] != key:
					errs <- fmt.Errorf("%s : réponse d'une autre URL (%v)", key, out["url"])
				}
				_, _ = p.quota.last(p.fp)
				_ = cache.Len()
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if cache.Len() > 8 {
		t.Errorf("cache : %d entrées, au plus 8", cache.Len())
	}
	if _, ok := quota.last(shared.fp); !ok {
		t.Error("le quota doit être connu")
	}
	notModified := 0
	for _, r := range s.requests() {
		if r.Status == http.StatusNotModified {
			notModified++
		}
	}
	if notModified == 0 {
		t.Error("aucun 304 : le cache partagé n'a pas servi")
	}
}

// TestCacheablePath vérifie que seules les réponses d'un commit seul (statistiques, jamais relues, qui
// portent des extraits du code du dépôt privé) échappent au cache ETag.
func TestCacheablePath(t *testing.T) {
	for path, want := range map[string]bool{
		"/repos/alice/projet":                                       true,
		"/repos/alice/projet/branches":                              true,
		"/repos/alice/projet/commits":                               true,
		"/repos/alice/projet/compare/" + apiBaseSHA + "..." + apiM5: true,
		"/repositories/42":                                          true,
		"/repositories/42/commits":                                  true,
		"/rate_limit":                                               true,
		"/repos/alice/projet/commits/" + apiC3:                      false,
		"/repositories/42/commits/" + apiC3:                         false,
	} {
		if got := cacheablePath(path); got != want {
			t.Errorf("cacheablePath(%q) = %v, veut %v", path, got, want)
		}
	}
}
