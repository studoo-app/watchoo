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

// Helpers of the transport tests (tr prefix). No real network call: httptest only.

// trEpoch is the start instant of the tests' clock.
var trEpoch = time.Unix(1_800_000_000, 0).UTC()

// Dummy personal data slipped into the bodies: they must appear neither in a log nor in an
// error.
const (
	trSecretMessage = "Message secret de commit"
	trSecretEmail   = "etudiant@example.com"
)

// trClock is an adjustable, concurrency-safe clock.
type trClock struct{ ns atomic.Int64 }

func newTrClock(t time.Time) *trClock {
	c := &trClock{}
	c.set(t)
	return c
}

func (c *trClock) now() time.Time      { return time.Unix(0, c.ns.Load()).UTC() }
func (c *trClock) set(t time.Time)     { c.ns.Store(t.UnixNano()) }
func (c *trClock) add(d time.Duration) { c.ns.Add(int64(d)) }

// trRequest is a request received by trServer.
type trRequest struct {
	Method, Path, RawQuery string
	IfNoneMatch            string
	Authorization          string
	UserAgent              string
	Status                 int
	Date                   string
	Remaining              string
}

// trServer is a minimal fake GitHub: ETag per URL, 304 responses, quota headers (the quota only goes
// down on 200 responses, as on GitHub), specific handlers per path.
type trServer struct {
	t   testing.TB
	srv *httptest.Server

	mu         sync.Mutex
	remaining  int
	reset      int64
	versions   map[string]int              // relative URL (path?parameters) → version of the resource
	bodies     map[string]string           // path → fixed body (otherwise generated body)
	links      map[string]string           // path → Link header
	handlers   map[string]http.HandlerFunc // path → specific handler (without automatic ETag or quota)
	reqs       []trRequest
	onNotModif func() // called right before sending a 304
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

// trStatusWriter remembers the HTTP code sent.
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
		s.t.Errorf("request %s %s received: only GET and HEAD are allowed", r.Method, r.URL.Path)
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

// rateHeaders sets the quota headers. Called with s.mu locked.
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

// bump changes the version (hence the ETag and the body) of the resource at relative URL key.
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
		s.t.Fatal("no request received")
	}
	return reqs[len(reqs)-1]
}

// trNew builds a Provider to srv (forced BaseURL), with the clock clock if it is not nil.
func trNew(t testing.TB, baseURL string, token string, opts Options, clock *trClock) *Provider {
	t.Helper()
	opts.BaseURL = baseURL
	p, err := New(token, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if clock != nil {
		p.tr.now = clock.now
	}
	return p
}

// trGet sends GET path (relative to BaseURL) through go-github and decodes the JSON response.
func trGet(ctx context.Context, p *Provider, path string) (map[string]any, *gogithub.Response, error) {
	req, err := p.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var out map[string]any
	resp, err := p.client.Do(req, &out)
	return out, resp, err
}

// trMustGet is trGet failing the test on error.
func trMustGet(t testing.TB, p *Provider, path string) (map[string]any, *gogithub.Response) {
	t.Helper()
	out, resp, err := trGet(context.Background(), p, path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return out, resp
}

// trLogs captures the JSON logs of a Provider.
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

// records returns the decoded log records.
func (l *trLogs) records(t testing.TB) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(l.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unreadable log %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

func trLogger(l *trLogs, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(l, &slog.HandlerOptions{Level: level}))
}

// trRoundTripFunc adapts a function into an http.RoundTripper.
type trRoundTripFunc func(*http.Request) (*http.Response, error)

func (f trRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// trCloseRecorder is a request body that remembers being closed.
type trCloseRecorder struct {
	io.Reader
	closed atomic.Bool
}

func (c *trCloseRecorder) Close() error {
	c.closed.Store(true)
	return nil
}

// TestTransportETag checks the ETag cycle: recording a 200, conditional request, 304 served with the
// same data, unchanged quota, new version of the resource.
func TestTransportETag(t *testing.T) {
	s := newTrServer(t)
	clock := newTrClock(trEpoch)
	p := trNew(t, s.srv.URL, testToken, Options{}, clock)
	const path = "repos/alice/projet"

	first, resp := trMustGet(t, p, path)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-From-Cache") != "" {
		t.Fatalf("first response: %d, X-From-Cache %q", resp.StatusCode, resp.Header.Get("X-From-Cache"))
	}
	if got := s.last().IfNoneMatch; got != "" {
		t.Errorf("first request: If-None-Match = %q, want empty", got)
	}
	if p.cache.Len() != 1 {
		t.Fatalf("cache: %d entries, want 1", p.cache.Len())
	}
	before, ok := p.quota.last(p.fp)
	if !ok || before.Remaining != 4999 || before.Limit != 5000 || before.Used != 1 ||
		!before.ObservedAt.Equal(trEpoch) || !before.Reset.Equal(trEpoch.Add(time.Hour)) {
		t.Fatalf("quota after the 200 = %+v, %v", before, ok)
	}
	etag := resp.Header.Get("ETag")

	// Second request: conditional, 304, same data, unchanged quota.
	clock.add(time.Minute)
	second, resp := trMustGet(t, p, path)
	got := s.last()
	if got.IfNoneMatch != etag || got.Status != http.StatusNotModified {
		t.Fatalf("second request: If-None-Match %q (want %q), code %d", got.IfNoneMatch, etag, got.Status)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-From-Cache") != "1" {
		t.Errorf("served 304: code %d, X-From-Cache %q", resp.StatusCode, resp.Header.Get("X-From-Cache"))
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("served 304: %v, want %v", second, first)
	}
	after, _ := p.quota.last(p.fp)
	if after.Remaining != before.Remaining {
		t.Errorf("a 304 must not reduce the tracked quota: %d → %d", before.Remaining, after.Remaining)
	}
	if !after.ObservedAt.Equal(trEpoch.Add(time.Minute)) {
		t.Errorf("the 304 must update the quota observation: %v", after.ObservedAt)
	}

	// New version of the resource: 200, cache updated, then 304 on the new ETag.
	s.bump("/" + path)
	third, resp := trMustGet(t, p, path)
	if s.last().Status != http.StatusOK || third["version"] != 2.0 || resp.Header.Get("X-From-Cache") != "" {
		t.Fatalf("new version: code %d, %v", s.last().Status, third)
	}
	if p.cache.Len() != 1 {
		t.Errorf("cache: %d entries, want 1 (entry replaced)", p.cache.Len())
	}
	fourth, _ := trMustGet(t, p, path)
	if got := s.last(); got.Status != http.StatusNotModified || got.IfNoneMatch != resp.Header.Get("ETag") {
		t.Errorf("after update: code %d, If-None-Match %q", got.Status, got.IfNoneMatch)
	}
	if !reflect.DeepEqual(third, fourth) {
		t.Errorf("304 served after update: %v, want %v", fourth, third)
	}
}

// TestTransportCachedHeaders checks that a response served from the cache keeps the headers of the
// entry (Link, hence the pagination; Content-Type) but takes the quota and the date of the 304.
func TestTransportCachedHeaders(t *testing.T) {
	s := newTrServer(t)
	p := trNew(t, s.srv.URL, testToken, Options{}, newTrClock(trEpoch))
	const path = "repos/alice/projet/commits"
	link := fmt.Sprintf(`<%s/%s?page=2&per_page=100>; rel="next", <%s/%s?page=3&per_page=100>; rel="last"`,
		s.srv.URL, path, s.srv.URL, path)
	s.setLink("/"+path, link)

	_, first := trMustGet(t, p, path+"?per_page=100")
	if first.NextPage != 2 || first.LastPage != 3 {
		t.Fatalf("pagination of the 200: next %d, last %d", first.NextPage, first.LastPage)
	}
	trMustGet(t, p, "repos/alice/autre") // lowers the quota on the server side

	_, cached := trMustGet(t, p, path+"?per_page=100")
	notModified := s.last()
	if notModified.Status != http.StatusNotModified {
		t.Fatalf("code %d, want 304", notModified.Status)
	}
	if cached.NextPage != 2 || cached.LastPage != 3 {
		t.Errorf("pagination served from the cache: next %d, last %d", cached.NextPage, cached.LastPage)
	}
	if ct := cached.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	if got := cached.Header.Get("X-RateLimit-Remaining"); got != notModified.Remaining || got != "4998" {
		t.Errorf("X-RateLimit-Remaining = %q, want the one of the 304 (%q)", got, notModified.Remaining)
	}
	if got := cached.Header.Get("Date"); got != notModified.Date {
		t.Errorf("Date = %q, want the one of the 304 (%q)", got, notModified.Date)
	}
	if cached.Rate.Remaining != 4998 {
		t.Errorf("Rate seen by go-github = %d, want 4998", cached.Rate.Remaining)
	}
	if cl := cached.ContentLength; cl <= 0 || cached.Header.Get("Content-Length") != strconv.FormatInt(cl, 10) {
		t.Errorf("inconsistent Content-Length: %d / %q", cl, cached.Header.Get("Content-Length"))
	}
}

// TestTransportCacheKeyIsFullURL checks that the cache key is the full URL, parameters included.
func TestTransportCacheKeyIsFullURL(t *testing.T) {
	s := newTrServer(t)
	p := trNew(t, s.srv.URL, testToken, Options{}, nil)
	page1, _ := trMustGet(t, p, "repos/alice/projet/commits?page=1&per_page=100")
	page2, _ := trMustGet(t, p, "repos/alice/projet/commits?page=2&per_page=100")
	if reflect.DeepEqual(page1, page2) || p.cache.Len() != 2 {
		t.Fatalf("two pages, want two entries: %d entries", p.cache.Len())
	}
	again, _ := trMustGet(t, p, "repos/alice/projet/commits?page=2&per_page=100")
	got := s.last()
	if got.Status != http.StatusNotModified || !strings.Contains(got.IfNoneMatch, "page=2") {
		t.Errorf("page 2: code %d, If-None-Match %q", got.Status, got.IfNoneMatch)
	}
	if !reflect.DeepEqual(again, page2) {
		t.Errorf("page 2 served from the cache: %v, want %v", again, page2)
	}
	trMustGet(t, p, "repos/alice/projet/commits?page=3&per_page=100")
	if got := s.last(); got.IfNoneMatch != "" {
		t.Errorf("page 3 never seen: If-None-Match %q, want empty", got.IfNoneMatch)
	}
}

// TestTransportCacheTokenChange checks that a shared cache and quota never serve the responses or
// the state of one token to another.
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
		t.Fatalf("same token: code %d", got.Status)
	}

	// Other token: no conditional request, the cache is emptied then filled for it.
	trMustGet(t, p2, path)
	if got := s.last(); got.IfNoneMatch != "" || got.Status != http.StatusOK || got.Authorization != "Bearer "+other {
		t.Errorf("other token: If-None-Match %q, code %d", got.IfNoneMatch, got.Status)
	}
	if cache.Len() != 1 {
		t.Errorf("cache: %d entries, want 1", cache.Len())
	}
	if _, ok := p1.quota.last(p1.fp); ok {
		t.Error("the old token's quota must no longer be visible")
	}
	if _, ok := p2.quota.last(p2.fp); !ok {
		t.Error("the new token's quota must be known")
	}

	// Back to the first token: nothing comes from the second one's cache.
	trMustGet(t, p1, path)
	if got := s.last(); got.IfNoneMatch != "" || got.Status != http.StatusOK {
		t.Errorf("back to the first token: If-None-Match %q, code %d", got.IfNoneMatch, got.Status)
	}

	// A new Provider built with the same token (new sync) benefits from the cache.
	p3 := trNew(t, s.srv.URL, testToken, Options{Cache: cache, Quota: quota}, nil)
	trMustGet(t, p3, path)
	if got := s.last(); got.Status != http.StatusNotModified {
		t.Errorf("new Provider, same token: code %d, want 304", got.Status)
	}
}

// TestTransportCacheBodyLimit checks that a body larger than the limit is not cached and that it is
// passed on intact, with or without Content-Length.
func TestTransportCacheBodyLimit(t *testing.T) {
	s := newTrServer(t)
	const limit = 64
	p := trNew(t, s.srv.URL, testToken, Options{Cache: NewCache(10, limit)}, nil)

	small := `{"n":"` + strings.Repeat("a", limit-8) + `"}` // exactly 64 bytes
	big := `{"n":"` + strings.Repeat("b", 4*limit) + `"}`
	s.setBody("/small", small)
	s.setBody("/big", big)
	s.handle("/chunked", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"chunked"`)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, big[:10])
		w.(http.Flusher).Flush() // unknown length: chunked transfer
		_, _ = io.WriteString(w, big[10:])
	})
	if len(small) != limit {
		t.Fatalf("test body of %d bytes", len(small))
	}

	out, _ := trMustGet(t, p, "small")
	if out["n"] != strings.Repeat("a", limit-8) || p.cache.Len() != 1 {
		t.Fatalf("small body: %v, %d entries", out, p.cache.Len())
	}
	for _, path := range []string{"big", "chunked"} {
		out, _ := trMustGet(t, p, path)
		if out["n"] != strings.Repeat("b", 4*limit) {
			t.Errorf("%s: truncated or altered body", path)
		}
		if p.cache.Len() != 1 {
			t.Errorf("%s: %d entries, a large body must not be cached", path, p.cache.Len())
		}
		trMustGet(t, p, path)
		if got := s.last(); got.IfNoneMatch != "" {
			t.Errorf("%s: If-None-Match %q without cache entry", path, got.IfNoneMatch)
		}
	}
}

// TestTransportCacheLRU checks the bound on the number of entries and the eviction of the least
// recently used one.
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
	if !conditional("a") { // a becomes the most recent again
		t.Fatal("a must be in the cache")
	}
	conditional("c") // evicts b
	if p.cache.Len() != 2 {
		t.Fatalf("cache: %d entries, want 2", p.cache.Len())
	}
	if !conditional("a") {
		t.Error("a must have stayed in the cache")
	}
	if conditional("b") { // evicts c
		t.Error("b should have been evicted")
	}
	if conditional("c") {
		t.Error("c should have been evicted")
	}
}

// TestCacheUnit tests Cache directly: binding to the token, zero value, replacement, bound.
func TestCacheUnit(t *testing.T) {
	fp, other := fingerprintOf(testToken), fingerprintOf(testToken+"_autre")
	entry := func(key string, n int) *cacheEntry {
		return &cacheEntry{key: key, etag: `"` + key + `"`, header: http.Header{}, body: bytes.Repeat([]byte("x"), n)}
	}

	var zero Cache // usable zero value
	if zero.Len() != 0 || zero.get(fp, "a") != nil {
		t.Fatal("zero cache not empty")
	}
	if !zero.put(fp, entry("a", 10)) || zero.Len() != 1 {
		t.Fatal("zero cache: put")
	}
	if zero.put(fp, entry("big", DefaultCacheBodyBytes+1)) {
		t.Error("zero cache: a body beyond the default limit must not be recorded")
	}

	c := NewCache(3, 16)
	if c.put(fp, entry("too-large", 17)) || c.Len() != 0 {
		t.Error("17-byte body with a 16-byte limit: want refused")
	}
	for _, k := range []string{"a", "b", "c"} {
		c.put(fp, entry(k, 1))
	}
	c.put(fp, entry("b", 2)) // replacement: no new entry
	if c.Len() != 3 || len(c.get(fp, "b").body) != 2 {
		t.Fatalf("replacement: %d entries", c.Len())
	}
	c.put(fp, entry("d", 1)) // evicts a, the least recently used
	if c.get(fp, "a") != nil || c.get(fp, "c") == nil || c.Len() != 3 {
		t.Errorf("LRU: a must be evicted, c kept")
	}

	// Read by another token: emptied, nothing is served.
	if c.get(other, "c") != nil || c.Len() != 0 {
		t.Errorf("other token: nothing must be served (%d entries)", c.Len())
	}
	if c.get(fp, "c") != nil {
		t.Error("the emptied cache must not come back for the old token")
	}

	// NewCache with null values: default values.
	d := NewCache(0, -1)
	if d.maxEntries != DefaultCacheEntries || d.maxBodyBytes != DefaultCacheBodyBytes {
		t.Errorf("NewCache(0, -1) = %d, %d", d.maxEntries, d.maxBodyBytes)
	}
}

// TestCacheNeverPrintsContents checks that printing the cache reveals neither the content of the
// responses nor the token's fingerprint.
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
				t.Errorf("%q reveals %q", s, f)
			}
		}
	}
	if got := c.String(); got != "github.Cache(1 entrée)" {
		t.Errorf("String = %q", got)
	}
}

// TestTransport304AfterEviction checks that a 304 is still served when the entry was evicted from the
// cache during the request (other token, LRU): the entry read before sending is used.
func TestTransport304AfterEviction(t *testing.T) {
	s := newTrServer(t)
	cache := NewCache(0, 0)
	p := trNew(t, s.srv.URL, testToken, Options{Cache: cache}, nil)
	first, _ := trMustGet(t, p, "repos/alice/projet")
	s.mu.Lock()
	s.onNotModif = func() { cache.get(fingerprintOf(testToken+"_autre"), "x") } // empties the cache
	s.mu.Unlock()
	second, resp := trMustGet(t, p, "repos/alice/projet")
	if s.last().Status != http.StatusNotModified || cache.Len() != 0 {
		t.Fatalf("code %d, %d entries", s.last().Status, cache.Len())
	}
	if resp.StatusCode != http.StatusOK || !reflect.DeepEqual(first, second) {
		t.Errorf("304 after eviction of the entry: %d, %v", resp.StatusCode, second)
	}
}

// TestTransportRequestUntouched checks that the caller's request is never modified, that the
// response never references the copy carrying the token, and that the caller's conditional headers
// are removed when nothing is cached.
func TestTransportRequestUntouched(t *testing.T) {
	s := newTrServer(t)
	p := trNew(t, s.srv.URL, testToken, Options{UserAgent: "watchoo/test"}, nil)
	for i := range 2 { // 200 then 304
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
			t.Errorf("request %d: caller's headers modified: %v", i, req.Header)
		}
		if resp.Request != req || resp.Request.Header.Get("Authorization") != "" {
			t.Errorf("request %d: the response must reference the caller's request, without token", i)
		}
		got := s.last()
		if got.Authorization != "Bearer "+testToken || got.UserAgent != "watchoo/test" {
			t.Errorf("request %d: Authorization or User-Agent missing", i)
		}
		if i == 0 && got.IfNoneMatch != "" {
			t.Errorf("caller's If-None-Match passed on without cache entry: %q", got.IfNoneMatch)
		}
		if i == 1 && (got.Status != http.StatusNotModified || got.IfNoneMatch == `"fabrique"`) {
			t.Errorf("second request: code %d, If-None-Match %q", got.Status, got.IfNoneMatch)
		}
	}
}

// TestTransportHead checks that a HEAD request goes through, without cache, updating the quota.
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
		t.Errorf("HEAD: code %d, %d cache entries", resp.StatusCode, p.cache.Len())
	}
	if _, ok := p.quota.last(p.fp); !ok {
		t.Error("HEAD: the quota must be updated")
	}
}

// TestTransportReadOnly checks, through go-github, that any write request is refused before sending
// (no request received), body closed, and that the converted error says so.
func TestTransportReadOnly(t *testing.T) {
	s := newTrServer(t)
	base := &countingTransport{next: http.DefaultTransport}
	p := trNew(t, s.srv.URL, testToken, Options{Transport: base}, nil)
	ctx := context.Background()

	resp, err := p.client.Repositories.Delete(ctx, "alice", "projet")
	e := p.convertError(context.Background(), "deleting repository alice/projet", resp, err)
	var apiErr *provider.APIError
	if !errors.Is(e, provider.ErrWriteForbidden) || !errors.As(e, &apiErr) {
		t.Fatalf("Delete: %v", e)
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
		if e := p.convertError(context.Background(), "writing", resp, err); !errors.Is(e, provider.ErrWriteForbidden) {
			t.Errorf("%s: %v, want ErrWriteForbidden", method, e)
		} else if strings.Contains(e.Error(), testToken) {
			t.Errorf("%s: the error quotes the token", method)
		}

		body := &trCloseRecorder{Reader: strings.NewReader("{}")}
		raw, _ := http.NewRequest(method, s.srv.URL+"/repos/alice/projet", body)
		if _, err := p.tr.RoundTrip(raw); !errors.Is(err, provider.ErrWriteForbidden) {
			t.Errorf("%s direct: %v", method, err)
		}
		if !body.closed.Load() {
			t.Errorf("%s: the body of the refused request must be closed", method)
		}
	}
	if n := base.n.Load(); n != 0 || len(s.requests()) != 0 {
		t.Errorf("%d request(s) sent, %d received: want none", n, len(s.requests()))
	}
}

// TestTransportQuotaFromHeaders checks the quota update by every response, errors included, and the
// state shared between Providers built with the same Quota.
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
		t.Fatalf("Remaining = %d, want 4999", rl.Remaining)
	}
	_, resp, err := trGet(context.Background(), p, "repos/alice/absent")
	if !errors.Is(p.convertError(context.Background(), "lecture", resp, err), provider.ErrNotFound) {
		t.Fatalf("want 404: %v", err)
	}
	if rl, _ := quota.last(p.fp); rl.Remaining != 4998 {
		t.Errorf("an error response must also update the quota: Remaining = %d", rl.Remaining)
	}
	trMustGet(t, p, "search/code")
	if rl, _ := quota.last(p.fp); rl.Remaining != 4998 {
		t.Errorf("a resource other than core must not touch the quota: Remaining = %d", rl.Remaining)
	}

	// Another Provider (next sync) with the same Quota sees the state.
	p2 := trNew(t, s.srv.URL, testToken, Options{Quota: quota}, clock)
	if rl, ok := p2.quota.last(p2.fp); !ok || rl.Remaining != 4998 {
		t.Errorf("shared quota: %+v, %v", rl, ok)
	}
}

// TestTransportQuotaExhausted checks the preventive refusal when the quota is exhausted: no request
// leaves until the reset, except GET /rate_limit.
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
	e := p.convertError(context.Background(), "reading repository alice/projet", resp, err)
	var rle *provider.RateLimitError
	if !errors.As(e, &rle) || rle.Secondary || !rle.Reset.Equal(reset) {
		t.Fatalf("exhausted quota: %v", e)
	}
	sent := base.n.Load()

	// Preventive refusal, including for another Provider sharing the Quota.
	p2 := trNew(t, s.srv.URL, testToken, Options{Transport: base, Quota: quota}, clock)
	for _, pr := range []*Provider{p, p2} {
		_, resp, err := trGet(context.Background(), pr, "repos/alice/autre?per_page=100")
		e := pr.convertError(context.Background(), "reading repository alice/autre", resp, err)
		var apiErr *provider.APIError
		if !errors.As(e, &rle) || rle.Secondary || !rle.Reset.Equal(reset) || !errors.As(e, &apiErr) {
			t.Fatalf("want a preventive refusal: %v", e)
		}
		if apiErr.Status != 0 || apiErr.Method != http.MethodGet || apiErr.Path != "/repos/alice/autre" {
			t.Errorf("APIError of the preventive refusal = %+v", apiErr)
		}
		if strings.Contains(e.Error(), "per_page") || strings.Contains(e.Error(), "127.0.0.1") {
			t.Errorf("the error quotes the URL: %v", e)
		}
	}
	if base.n.Load() != sent {
		t.Errorf("%d request(s) sent despite the exhausted quota", base.n.Load()-sent)
	}

	// GET /rate_limit stays allowed.
	if _, _, err := p.client.RateLimit.Get(context.Background()); err != nil {
		t.Errorf("GET /rate_limit: %v", err)
	}
	if base.n.Load() != sent+1 {
		t.Error("GET /rate_limit must leave despite the exhausted quota")
	}

	// After the reset: the requests start again.
	clock.set(reset)
	trMustGet(t, p, "repos/alice/autre")
	if base.n.Load() != sent+2 {
		t.Error("after the reset, the request must leave")
	}
}

// TestTransportSecondaryHold checks that a Retry-After on 403 or 429 holds the following requests
// back until the deadline, except GET /rate_limit.
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
				t.Fatalf("secondary limit: %v", e)
			}
			sent := base.n.Load()
			clock.add(10 * time.Second)
			_, resp, err = trGet(context.Background(), p, "repos/alice/autre")
			if e := p.convertError(context.Background(), "lecture", resp, err); !errors.As(e, &rle) || !rle.Secondary ||
				rle.RetryAfter != 20*time.Second {
				t.Fatalf("want a preventive refusal: %v", e)
			}
			if base.n.Load() != sent {
				t.Error("no request must leave during the secondary limit")
			}
			if _, _, err := p.client.RateLimit.Get(context.Background()); err != nil || base.n.Load() != sent+1 {
				t.Errorf("GET /rate_limit must stay allowed: %v", err)
			}
			clock.add(20 * time.Second)
			trMustGet(t, p, "repos/alice/autre")
		})
	}
}

// TestTransportPrimaryWithRetryAfter: a 403 or 429 announcing an exhausted quota (X-RateLimit-Remaining:
// 0) with a Retry-After as well (GitHub may send both). The first error and the following preventive
// refusals describe the same limit: exhausted quota (Secondary = false) until X-RateLimit-Reset.
// Non-regression: the Retry-After was recorded as a secondary limit, and the following refusals said
// "secondary limit" with a Reset too early (reception + Retry-After). Without a reset date, the wait
// comes from Retry-After, still as an exhausted quota.
func TestTransportPrimaryWithRetryAfter(t *testing.T) {
	reset := trEpoch.Add(30 * time.Minute)
	for _, tt := range []struct {
		name    string
		status  int
		headers map[string]string
		reset   time.Time // expected Reset, on the first error as on the preventive refusals
		resume  time.Duration
	}{
		{"403 with reset", http.StatusForbidden,
			map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Limit": "5000",
				"X-RateLimit-Reset": strconv.FormatInt(reset.Unix(), 10), "Retry-After": "60"},
			reset, 30 * time.Minute},
		{"429 with reset", http.StatusTooManyRequests,
			map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Limit": "5000",
				"X-RateLimit-Reset": strconv.FormatInt(reset.Unix(), 10), "Retry-After": "60"},
			reset, 30 * time.Minute},
		{"403 without reset", http.StatusForbidden,
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
					t.Fatalf("%s: %v, want a RateLimitError", what, e)
				}
				if rle.Secondary || !rle.Reset.Equal(tt.reset) {
					t.Errorf("%s: Secondary %v, Reset %s; want exhausted quota until %s", what, rle.Secondary, rle.Reset, tt.reset)
				}
				if strings.Contains(e.Error(), "secondary") {
					t.Errorf("%s: %q must not talk about a secondary limit", what, e)
				}
			}

			_, resp, err := trGet(context.Background(), p, "repos/alice/projet")
			check("first error", p.convertError(context.Background(), "reading", resp, err))
			sent := base.n.Load()
			for _, after := range []time.Duration{10 * time.Second, 70 * time.Second} {
				if after >= tt.resume {
					break
				}
				clock.set(trEpoch.Add(after))
				_, resp, err = trGet(context.Background(), p, "repos/alice/autre")
				check(fmt.Sprintf("preventive refusal at +%s", after), p.convertError(context.Background(), "reading", resp, err))
			}
			if base.n.Load() != sent {
				t.Errorf("%d request(s) sent despite the exhausted quota", base.n.Load()-sent)
			}
			clock.set(trEpoch.Add(tt.resume))
			trMustGet(t, p, "repos/alice/autre")
		})
	}
}

// TestTransportNeverLeaks goes through the main paths of the transport (200, 304, errors, refusals,
// network failure) and checks that neither the token, nor the Authorization header, nor the personal
// data of the bodies, nor the request parameters appear in the logs or the errors.
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
			errs = append(errs, p.convertError(context.Background(), "operation", resp, err))
		}
		if resp != nil && resp.Request != nil && resp.Request.Header.Get("Authorization") != "" {
			t.Errorf("the response exposes the Authorization header (%s)", resp.Request.URL.Path)
		}
	}
	for _, path := range []string{
		"repos/alice/projet?per_page=100&sha=main", "repos/alice/projet?per_page=100&sha=main", // 200 then 304
		"repos/alice/absent", "repos/alice/refus", "repos/alice/vide", "repos/alice/panne", "repos/ecole/sso",
		"repos/alice/lent", "repos/alice/projet", // the last one is held back (secondary limit)
	} {
		_, resp, err := trGet(ctx, p, path)
		collect(resp, err)
	}
	resp, err := p.client.Repositories.Delete(ctx, "alice", "projet")
	collect(resp, err)

	// Network failure: closed server.
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	pc := trNew(t, closed.URL, testToken, Options{Logger: logger}, clock)
	_, resp, err = trGet(ctx, pc, "repos/alice/projet?per_page=100")
	if err == nil {
		t.Fatal("closed server: want an error")
	}
	errs = append(errs, pc.convertError(context.Background(), "operation", resp, err))

	if len(errs) != 9 {
		t.Fatalf("%d errors, want 9", len(errs))
	}
	forbidden := []string{testToken, "Bearer", "uthorization", trSecretMessage, trSecretEmail, "per_page", "sha=main", "SECRET42"}
	for _, e := range errs {
		for _, f := range forbidden {
			if strings.Contains(e.Error(), f) {
				t.Errorf("the error %q contains %q", e, f)
			}
		}
		if strings.Contains(e.Error(), strings.TrimPrefix(closed.URL, "http://")) {
			t.Errorf("the network error quotes the address: %q", e)
		}
	}
	out := logs.String()
	for _, f := range forbidden {
		if strings.Contains(out, f) {
			t.Errorf("the logs contain %q:\n%s", f, out)
		}
	}
	recs := logs.records(t)
	if len(recs) < 10 {
		t.Errorf("only %d logs", len(recs))
	}
	for _, rec := range recs {
		if rec["level"] != "DEBUG" {
			t.Errorf("log at level %v: only Debug is allowed (%v)", rec["level"], rec["msg"])
		}
	}

	// At Info level, the transport writes nothing.
	quiet := &trLogs{}
	pq := trNew(t, s.srv.URL, testToken, Options{Logger: trLogger(quiet, slog.LevelInfo)}, nil)
	trMustGet(t, pq, "repos/alice/projet")
	_, _, _ = trGet(ctx, pq, "repos/alice/absent")
	if quiet.String() != "" {
		t.Errorf("logs at Info level: %s", quiet.String())
	}
}

// TestTransportLogs checks the content of the Debug logs: method, path without parameters, code,
// remaining quota, cache origin.
func TestTransportLogs(t *testing.T) {
	s := newTrServer(t)
	logs := &trLogs{}
	p := trNew(t, s.srv.URL, testToken, Options{Logger: trLogger(logs, slog.LevelDebug)}, newTrClock(trEpoch))
	trMustGet(t, p, "repos/alice/projet/commits?per_page=100")
	trMustGet(t, p, "repos/alice/projet/commits?per_page=100")
	recs := logs.records(t)
	if len(recs) != 2 {
		t.Fatalf("%d logs, want 2:\n%s", len(recs), logs.String())
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

// TestTransportReadErrorWhileCaching checks that a cut while reading a body to be cached gives an
// ErrUnavailable error, without caching anything.
func TestTransportReadErrorWhileCaching(t *testing.T) {
	s := newTrServer(t)
	s.handle("/repos/alice/projet", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"coupe"`)
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, `{"id":`) // the server closes the connection: incomplete body
	})
	p := trNew(t, s.srv.URL, testToken, Options{}, nil)
	_, resp, err := trGet(context.Background(), p, "repos/alice/projet")
	e := p.convertError(context.Background(), "reading repository alice/projet", resp, err)
	if !errors.Is(e, provider.ErrUnavailable) || !strings.Contains(e.Error(), "connection interrupted") {
		t.Errorf("incomplete body: %v", e)
	}
	if p.cache.Len() != 0 {
		t.Errorf("nothing must be cached: %d entries", p.cache.Len())
	}
}

// TestTransportConcurrent runs 50 goroutines in parallel, each with its Provider (like one sync per
// goroutine) on a shared and small Cache and Quota (frequent evictions): run with -race. Every
// response must match its URL.
func TestTransportConcurrent(t *testing.T) {
	s := newTrServer(t)
	cache, quota := NewCache(8, 0), NewQuota()
	shared := trNew(t, s.srv.URL, testToken, Options{Cache: cache, Quota: quota}, nil)
	const goroutines, rounds, paths = 50, 10, 12
	providers := make([]*Provider, goroutines)
	for g := range providers {
		providers[g] = shared
		if g%2 == 1 { // one goroutine out of two has its own Provider (same token, shared cache and quota)
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
					errs <- fmt.Errorf("%s: %w", key, err)
				case out["url"] != key:
					errs <- fmt.Errorf("%s: response of another URL (%v)", key, out["url"])
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
		t.Errorf("cache: %d entries, at most 8", cache.Len())
	}
	if _, ok := quota.last(shared.fp); !ok {
		t.Error("the quota must be known")
	}
	notModified := 0
	for _, r := range s.requests() {
		if r.Status == http.StatusNotModified {
			notModified++
		}
	}
	if notModified == 0 {
		t.Error("no 304: the shared cache did not serve")
	}
}

// TestCacheablePath checks that only the responses of a single commit (statistics, never read again,
// which carry extracts of the private repository's code) bypass the ETag cache.
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
			t.Errorf("cacheablePath(%q) = %v, want %v", path, got, want)
		}
	}
}
