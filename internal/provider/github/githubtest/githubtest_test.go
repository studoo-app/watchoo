package githubtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// t0 is the origin of the tests' dummy dates.
var t0 = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)

// recordTB replaces Errorf and Fatalf of a testing.TB to check the server's reports without
// failing the test. Fatalf ends the calling goroutine (runtime.Goexit), like the real one.
type recordTB struct {
	testing.TB
	mu     sync.Mutex
	errs   []string
	fatals []string
}

func (r *recordTB) Helper() {}

func (r *recordTB) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

func (r *recordTB) Fatalf(format string, args ...any) {
	r.mu.Lock()
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
	r.mu.Unlock()
	runtime.Goexit()
}

func (r *recordTB) counts() (errs, fatals int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.errs), len(r.fatals)
}

// expectFatal checks that fn calls Fatalf (in a separate goroutine, which Fatalf ends).
func expectFatal(t *testing.T, rec *recordTB, what string, fn func()) {
	t.Helper()
	_, before := rec.counts()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	<-done
	if _, after := rec.counts(); after != before+1 {
		t.Errorf("%s: %d call(s) to Fatalf, want 1", what, after-before)
	}
}

// result is a fully read response.
type result struct {
	status int
	header http.Header
	body   []byte
}

// noRedirect is a client that does not follow redirections.
var noRedirect = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	Timeout:       10 * time.Second,
}

// do sends a request to the server (path relative to URL(), parameters included) with the headers
// hdr, without following redirections.
func do(t *testing.T, s *Server, method, path string, hdr http.Header) result {
	t.Helper()
	req, err := http.NewRequest(method, s.URL()+strings.TrimPrefix(path, "/"), nil)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the response to %s %s: %v", method, path, err)
	}
	return result{status: resp.StatusCode, header: resp.Header, body: body}
}

// auth returns the headers of an authenticated request, plus the extra key/value pairs.
func auth(s *Server, extra ...string) http.Header {
	h := http.Header{"Authorization": {"Bearer " + s.Token()}}
	for i := 0; i+1 < len(extra); i += 2 {
		h.Add(extra[i], extra[i+1])
	}
	return h
}

// get sends an authenticated GET.
func get(t *testing.T, s *Server, path string, extra ...string) result {
	t.Helper()
	return do(t, s, http.MethodGet, path, auth(s, extra...))
}

// decode decodes the JSON body of r into v.
func decode(t *testing.T, r result, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("unreadable JSON body (%v): %s", err, r.body)
	}
}

// wantStatus checks the code of r.
func wantStatus(t *testing.T, what string, r result, want int) {
	t.Helper()
	if r.status != want {
		t.Fatalf("%s: HTTP %d, want %d (body: %s)", what, r.status, want, r.body)
	}
}

// message returns the "message" field of an error body.
func message(t *testing.T, r result) string {
	t.Helper()
	var e struct {
		Message string `json:"message"`
	}
	decode(t, r, &e)
	return e.Message
}

// fixedClock returns an adjustable clock.
type fixedClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fixedClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// commitList is the decoded form of a list of commits.
type commitList []struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Date  string `json:"date"`
		} `json:"author"`
		Committer struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Date  string `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
	HTMLURL string `json:"html_url"`
	Author  *struct {
		Login     string `json:"login"`
		ID        int64  `json:"id"`
		AvatarURL string `json:"avatar_url"`
	} `json:"author"`
	Committer *struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"committer"`
	Parents []struct {
		SHA string `json:"sha"`
	} `json:"parents"`
}

// shas returns the SHAs of a list of commits.
func (l commitList) shas() []string {
	out := make([]string, len(l))
	for i, c := range l {
		out[i] = c.SHA
	}
	return out
}

// graph is a history with a merged branch:
//
//	r1 ── m1 ── m2 ─────── M   (main)
//	  └── f1 ── f2 ──────┘     (feature: f2)
//
// dates: r1 t0, m1 t0+1h, f1 t0+2h, m2 t0+3h, f2 t0+4h, M t0+5h.
type graph struct {
	r1, m1, f1, m2, f2, merge string
}

// newGraphRepo adds the repository 101 alice/projet with the graph history.
func newGraphRepo(s *Server) graph {
	s.AddRepo(Repo{ID: 101, Owner: "alice", Name: "projet", Private: true})
	alice := Person{Name: "Alice Test", Email: "alice@example.test", Login: "alice"}
	bob := Person{Name: "Bob Test", Email: "bob@example.test"} // no linked account
	var g graph
	g.r1 = s.AddCommit(101, Commit{Message: "racine", Author: alice, CommittedAt: t0})
	g.m1 = s.AddCommit(101, Commit{Message: "m1", Author: alice, CommittedAt: t0.Add(time.Hour), Parents: []string{g.r1}})
	g.f1 = s.AddCommit(101, Commit{Message: "f1", Author: bob, CommittedAt: t0.Add(2 * time.Hour), Parents: []string{g.r1}})
	g.m2 = s.AddCommit(101, Commit{Message: "m2", Author: alice, CommittedAt: t0.Add(3 * time.Hour), Parents: []string{g.m1}})
	g.f2 = s.AddCommit(101, Commit{Message: "f2", Author: bob, CommittedAt: t0.Add(4 * time.Hour), Parents: []string{g.f1}})
	g.merge = s.AddCommit(101, Commit{Message: "fusion", Author: alice, Committer: bob, CommittedAt: t0.Add(5 * time.Hour),
		AuthoredAt: t0.Add(4*time.Hour + 30*time.Minute), Parents: []string{g.m2, g.f2}, Additions: 12, Deletions: 3})
	s.SetBranch(101, "main", g.merge)
	s.SetBranch(101, "feature", g.f2)
	return g
}

func TestFakeSHA(t *testing.T) {
	a, b := FakeSHA("a"), FakeSHA("b")
	if !shaPattern.MatchString(a) || len(a) != 40 {
		t.Errorf("FakeSHA(\"a\") = %q, want 40 lowercase hex", a)
	}
	if a != FakeSHA("a") {
		t.Error("FakeSHA is not deterministic")
	}
	if a == b {
		t.Error("FakeSHA(\"a\") == FakeSHA(\"b\")")
	}
}

func TestNewServerDefaults(t *testing.T) {
	clock := &fixedClock{now: t0.Add(500 * time.Millisecond)}
	s := NewServer(t, Options{Now: clock.Now})
	if !strings.HasPrefix(s.URL(), "http://127.0.0.1:") || !strings.HasSuffix(s.URL(), "/") {
		t.Errorf("URL() = %q, want http://127.0.0.1:port/", s.URL())
	}
	if s.Token() != Token {
		t.Errorf("Token() = %q, want %q", s.Token(), Token)
	}
	if q := s.Quota(); q.Limit != DefaultRateLimit || q.Remaining != DefaultRateLimit || !q.Reset.Equal(t0.Add(time.Hour)) || q.Status != 403 {
		t.Errorf("Quota() = %+v, want {5000 5000 %s 403}", q, t0.Add(time.Hour))
	}

	custom := NewServer(t, Options{Token: "gho_autre_faux"})
	if custom.Token() != "gho_autre_faux" {
		t.Errorf("Token() = %q, want the token of the options", custom.Token())
	}
	if r := do(t, custom, http.MethodGet, "rate_limit", http.Header{"Authorization": {"Bearer " + Token}}); r.status != 401 {
		t.Errorf("default token on a server with a custom token: HTTP %d, want 401", r.status)
	}
	if r := do(t, custom, http.MethodGet, "rate_limit", auth(custom)); r.status != 200 {
		t.Errorf("custom token: HTTP %d, want 200", r.status)
	}

	s.Close()
	s.Close() // no effect
}

func TestAuthorization(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	cases := []struct {
		name   string
		header http.Header
		want   int
	}{
		{"without header", nil, 401},
		{"wrong token", http.Header{"Authorization": {"Bearer gho_mauvais"}}, 401},
		{"unknown scheme", http.Header{"Authorization": {"Basic " + Token}}, 401},
		{"token alone", http.Header{"Authorization": {Token}}, 401},
		{"Bearer", http.Header{"Authorization": {"Bearer " + Token}}, 200},
		{"lowercase bearer", http.Header{"Authorization": {"bearer " + Token}}, 200},
		{"token", http.Header{"Authorization": {"token " + Token}}, 200},
	}
	for _, c := range cases {
		before := s.Quota().Remaining
		r := do(t, s, http.MethodGet, "repos/alice/projet", c.header)
		if r.status != c.want {
			t.Errorf("%s: HTTP %d, want %d", c.name, r.status, c.want)
			continue
		}
		consumed := before - s.Quota().Remaining
		if c.want == 401 {
			if msg := message(t, r); msg != "Bad credentials" {
				t.Errorf("%s: message %q, want \"Bad credentials\"", c.name, msg)
			}
			if h := r.header.Get("X-RateLimit-Remaining"); h != "" {
				t.Errorf("%s: X-RateLimit-Remaining = %q on a 401, want missing", c.name, h)
			}
			if consumed != 0 {
				t.Errorf("%s: %d quota unit(s) consumed by a 401", c.name, consumed)
			}
		} else if consumed != 1 {
			t.Errorf("%s: %d quota unit(s) consumed, want 1", c.name, consumed)
		}
	}

	reqs := s.Requests()
	if len(reqs) != len(cases) {
		t.Fatalf("log: %d requests, want %d", len(reqs), len(cases))
	}
	for i, c := range cases {
		if reqs[i].HasAuthorization != (c.header != nil) || reqs[i].ValidToken != (c.want == 200) || reqs[i].Status != c.want {
			t.Errorf("%s: log %+v", c.name, reqs[i])
		}
	}
}

func TestWriteMethodsFail(t *testing.T) {
	rec := &recordTB{TB: t}
	s := NewServer(rec, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	methods := []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete}
	for _, m := range methods {
		r := do(t, s, m, "repos/alice/projet", auth(s))
		if r.status != http.StatusMethodNotAllowed {
			t.Errorf("%s: HTTP %d, want 405", m, r.status)
		}
		if allow := r.header.Get("Allow"); allow != "GET, HEAD" {
			t.Errorf("%s: Allow = %q", m, allow)
		}
	}
	if errs, _ := rec.counts(); errs != len(methods) {
		t.Errorf("t.Errorf called %d times, want %d (one per write request)", errs, len(methods))
	}
	if n := len(s.Requests()); n != 0 {
		t.Errorf("log: %d request(s), want 0 (a write is not logged)", n)
	}
	if q := s.Quota(); q.Remaining != DefaultRateLimit {
		t.Errorf("quota consumed by a write: %+v", q)
	}
}

func TestRepository(t *testing.T) {
	s := NewServer(t, Options{})
	pushed := time.Date(2026, 9, 30, 17, 45, 12, 999, time.FixedZone("CEST", 2*3600))
	s.AddRepo(Repo{ID: 4242, Owner: "Ecole-Promo", Name: "projet.final", Private: true, Archived: true,
		DefaultBranch: "develop", Stars: 3, OpenIssues: 7, PushedAt: pushed})
	s.AddRepo(Repo{ID: 7, Owner: "bob", Name: "public"})

	for _, path := range []string{"repos/Ecole-Promo/projet.final", "repos/ecole-promo/PROJET.FINAL", "repositories/4242"} {
		r := get(t, s, path)
		wantStatus(t, path, r, 200)
		var repo struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			FullName string `json:"full_name"`
			Owner    struct {
				Login string `json:"login"`
				ID    int64  `json:"id"`
			} `json:"owner"`
			Private         bool    `json:"private"`
			Visibility      string  `json:"visibility"`
			Archived        bool    `json:"archived"`
			DefaultBranch   string  `json:"default_branch"`
			HTMLURL         string  `json:"html_url"`
			URL             string  `json:"url"`
			StargazersCount int     `json:"stargazers_count"`
			OpenIssuesCount int     `json:"open_issues_count"`
			PushedAt        *string `json:"pushed_at"`
		}
		decode(t, r, &repo)
		if repo.ID != 4242 || repo.Name != "projet.final" || repo.FullName != "Ecole-Promo/projet.final" ||
			repo.Owner.Login != "Ecole-Promo" || repo.Owner.ID == 0 || !repo.Private || repo.Visibility != "private" ||
			!repo.Archived || repo.DefaultBranch != "develop" || repo.StargazersCount != 3 || repo.OpenIssuesCount != 7 ||
			repo.HTMLURL != "https://github.com/Ecole-Promo/projet.final" || repo.URL != s.URL()+"repos/Ecole-Promo/projet.final" {
			t.Errorf("%s: %+v", path, repo)
		}
		if repo.PushedAt == nil || *repo.PushedAt != "2026-09-30T15:45:12Z" {
			t.Errorf("%s: pushed_at = %v, want 2026-09-30T15:45:12Z (UTC, to the second)", path, repo.PushedAt)
		}
		if ct := r.header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("%s: Content-Type = %q", path, ct)
		}
	}

	r := get(t, s, "repos/bob/public")
	var pub map[string]any
	decode(t, r, &pub)
	if pub["default_branch"] != "main" || pub["pushed_at"] != nil || pub["visibility"] != "public" || pub["private"] != false {
		t.Errorf("default repository: default_branch %v, pushed_at %v, visibility %v", pub["default_branch"], pub["pushed_at"], pub["visibility"])
	}

	before := s.Quota().Remaining
	for _, path := range []string{"repos/alice/inconnu", "repositories/999", "repositories/abc", "repos/alice", "users/alice", "repos/bob/public/inconnu"} {
		r := get(t, s, path)
		wantStatus(t, path, r, 404)
		var e map[string]string
		decode(t, r, &e)
		if e["message"] != "Not Found" || e["status"] != "404" || e["documentation_url"] == "" {
			t.Errorf("%s: body %v", path, e)
		}
	}
	if consumed := before - s.Quota().Remaining; consumed != 6 {
		t.Errorf("quota consumed by 6 404 responses: %d, want 6", consumed)
	}
}

func TestRepositoryAccess(t *testing.T) {
	s := NewServer(t, Options{})
	g := newGraphRepo(s)
	routes := []string{
		"repos/alice/projet", "repositories/101", "repos/alice/projet/branches", "repos/alice/projet/commits",
		"repositories/101/commits", "repos/alice/projet/commits/" + g.merge, "repos/alice/projet/compare/" + g.m1 + "..." + g.merge,
	}

	s.SetRepoAccess(101, AccessHidden)
	for _, path := range routes {
		r := get(t, s, path)
		if r.status != 404 || message(t, r) != "Not Found" {
			t.Errorf("AccessHidden, %s: HTTP %d %s, want 404 Not Found", path, r.status, r.body)
		}
	}

	s.SetRepoAccess(101, AccessSSO)
	for _, path := range routes {
		r := get(t, s, path)
		if r.status != 403 || !strings.HasPrefix(message(t, r), "Resource protected by organization SAML enforcement.") {
			t.Errorf("AccessSSO, %s: HTTP %d %s", path, r.status, r.body)
		}
		if sso := r.header.Get("X-GitHub-SSO"); sso != "required; url=https://github.com/orgs/alice/sso?authorization_request=fake" {
			t.Errorf("AccessSSO, %s: X-GitHub-SSO = %q", path, sso)
		}
		if r.header.Get("X-RateLimit-Remaining") == "" {
			t.Errorf("AccessSSO, %s: quota headers missing", path)
		}
	}

	s.SetRepoAccess(101, AccessGranted)
	for _, path := range routes {
		if r := get(t, s, path); r.status != 200 {
			t.Errorf("AccessGranted, %s: HTTP %d", path, r.status)
		}
	}
}

func TestRenameRepo(t *testing.T) {
	s := NewServer(t, Options{})
	g := newGraphRepo(s)
	s.RenameRepo(101, "ecole", "projet-alice")

	r := get(t, s, "repos/alice/projet/commits?sha=main&per_page=2")
	wantStatus(t, "old name", r, 301)
	wantLoc := s.URL() + "repositories/101/commits?sha=main&per_page=2"
	if loc := r.header.Get("Location"); loc != wantLoc {
		t.Errorf("Location = %q, want %q", loc, wantLoc)
	}
	var moved map[string]string
	decode(t, r, &moved)
	if moved["message"] != "Moved Permanently" || moved["url"] != wantLoc {
		t.Errorf("body of the 301: %v", moved)
	}
	if r.header.Get("X-RateLimit-Remaining") == "" {
		t.Error("301 without quota headers")
	}

	if r := get(t, s, "repos/Alice/Projet"); r.status != 301 || r.header.Get("Location") != s.URL()+"repositories/101" {
		t.Errorf("old name (different case): HTTP %d, Location %q", r.status, r.header.Get("Location"))
	}

	// A client that follows redirections gets the repository under its current name.
	req, _ := http.NewRequest(http.MethodGet, s.URL()+"repos/alice/projet", nil)
	req.Header.Set("Authorization", "Bearer "+s.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var repo struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || repo.ID != 101 || repo.FullName != "ecole/projet-alice" {
		t.Errorf("after redirection: HTTP %d, %+v", resp.StatusCode, repo)
	}

	// The commits followed through /repositories/{id} return the URLs of the current name.
	r = get(t, s, "repositories/101/commits?sha="+g.m1)
	var list commitList
	decode(t, r, &list)
	if len(list) != 2 || list[0].HTMLURL != "https://github.com/ecole/projet-alice/commit/"+g.m1 {
		t.Errorf("commits after renaming: %+v", list)
	}

	// Successive renamings: all the old names stay redirected.
	s.RenameRepo(101, "ecole", "projet-final")
	for _, old := range []string{"repos/alice/projet", "repos/ecole/projet-alice"} {
		if r := get(t, s, old); r.status != 301 {
			t.Errorf("%s after two renamings: HTTP %d, want 301", old, r.status)
		}
	}
	if r := get(t, s, "repos/ecole/projet-final"); r.status != 200 {
		t.Errorf("current name: HTTP %d", r.status)
	}

	// An inaccessible old name does not redirect.
	s.SetRepoAccess(101, AccessHidden)
	if r := get(t, s, "repos/alice/projet"); r.status != 404 {
		t.Errorf("old name of an inaccessible repository: HTTP %d, want 404", r.status)
	}
	s.SetRepoAccess(101, AccessGranted)

	// A new repository takes an old name again: the redirection disappears.
	s.AddRepo(Repo{ID: 202, Owner: "alice", Name: "projet"})
	r = get(t, s, "repos/alice/projet")
	var other struct {
		ID int64 `json:"id"`
	}
	decode(t, r, &other)
	if r.status != 200 || other.ID != 202 {
		t.Errorf("old name taken again: HTTP %d, id %d, want 200 and 202", r.status, other.ID)
	}
}

func TestBranches(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	if r := get(t, s, "repos/alice/projet/branches"); r.status != 200 || string(r.body) != "[]" {
		t.Errorf("empty repository: HTTP %d %s, want 200 []", r.status, r.body)
	}

	sha := s.AddCommit(1, Commit{Message: "init", CommittedAt: t0})
	for _, name := range []string{"main", "feature/b", "dev", "feature/a", "release"} {
		s.SetBranch(1, name, sha)
	}
	r := get(t, s, "repos/alice/projet/branches?per_page=2")
	wantStatus(t, "branches", r, 200)
	var branches []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
			URL string `json:"url"`
		} `json:"commit"`
		Protected *bool `json:"protected"`
	}
	decode(t, r, &branches)
	if len(branches) != 2 || branches[0].Name != "dev" || branches[1].Name != "feature/a" ||
		branches[0].Commit.SHA != sha || branches[0].Protected == nil || *branches[0].Protected {
		t.Errorf("page 1: %+v", branches)
	}
	wantLink := fmt.Sprintf(`<%[1]srepositories/1/branches?page=2&per_page=2>; rel="next", <%[1]srepositories/1/branches?page=3&per_page=2>; rel="last"`, s.URL())
	if link := r.header.Get("Link"); link != wantLink {
		t.Errorf("Link = %q\nwant   %q", link, wantLink)
	}

	var names []string
	for p := 1; p <= 4; p++ {
		r := get(t, s, "repos/alice/projet/branches?per_page=2&page="+strconv.Itoa(p))
		decode(t, r, &branches)
		for _, b := range branches {
			names = append(names, b.Name)
		}
	}
	if want := []string{"dev", "feature/a", "feature/b", "main", "release"}; !reflect.DeepEqual(names, want) {
		t.Errorf("all the pages: %v, want %v", names, want)
	}

	s.DeleteBranch(1, "dev")
	r = get(t, s, "repos/alice/projet/branches")
	decode(t, r, &branches)
	if len(branches) != 4 || r.header.Get("Link") != "" {
		t.Errorf("after deletion, a single page: %d branches, Link %q", len(branches), r.header.Get("Link"))
	}
}

func TestEmptyRepository(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "vide"})
	sha := FakeSHA("x")
	for _, path := range []string{"repos/alice/vide/commits", "repos/alice/vide/commits?sha=main", "repositories/1/commits",
		"repos/alice/vide/commits/" + sha, "repos/alice/vide/compare/" + sha + "..." + sha} {
		r := get(t, s, path)
		if r.status != 409 || message(t, r) != "Git Repository is empty." {
			t.Errorf("%s: HTTP %d %s, want 409 Git Repository is empty.", path, r.status, r.body)
		}
	}
	if r := get(t, s, "repos/alice/vide"); r.status != 200 {
		t.Errorf("metadata of an empty repository: HTTP %d", r.status)
	}
}

func TestListCommits(t *testing.T) {
	s := NewServer(t, Options{})
	g := newGraphRepo(s)

	r := get(t, s, "repos/alice/projet/commits")
	wantStatus(t, "commits", r, 200)
	var list commitList
	decode(t, r, &list)
	if want := []string{g.merge, g.f2, g.m2, g.f1, g.m1, g.r1}; !reflect.DeepEqual(list.shas(), want) {
		t.Errorf("default branch: %v\nwant %v", list.shas(), want)
	}

	m := list[0]
	if m.Commit.Message != "fusion" || m.Commit.Author.Name != "Alice Test" || m.Commit.Author.Email != "alice@example.test" ||
		m.Commit.Author.Date != "2026-10-01T12:30:00Z" || m.Commit.Committer.Name != "Bob Test" ||
		m.Commit.Committer.Date != "2026-10-01T13:00:00Z" || m.HTMLURL != "https://github.com/alice/projet/commit/"+g.merge {
		t.Errorf("merge commit: %+v", m)
	}
	if m.Author == nil || m.Author.Login != "alice" || m.Author.ID == 0 || m.Author.AvatarURL != avatarURL(m.Author.ID) {
		t.Errorf("author of the merge commit: %+v", m.Author)
	}
	if m.Committer != nil {
		t.Errorf("committer without linked account: %+v, want null", m.Committer)
	}
	if len(m.Parents) != 2 || m.Parents[0].SHA != g.m2 || m.Parents[1].SHA != g.f2 {
		t.Errorf("parents of the merge commit: %+v", m.Parents)
	}
	if root := list[5]; root.Parents == nil || len(root.Parents) != 0 {
		t.Errorf("parents of the root commit: %#v, want []", root.Parents)
	}
	if !strings.Contains(string(r.body), `"parents":[]`) {
		t.Error("the root commit must have \"parents\":[] in the JSON")
	}

	cases := []struct {
		query string
		want  []string
	}{
		{"sha=main", []string{g.merge, g.f2, g.m2, g.f1, g.m1, g.r1}},
		{"sha=feature", []string{g.f2, g.f1, g.r1}},
		{"sha=" + g.m2, []string{g.m2, g.m1, g.r1}},
		{"since=2026-10-01T10:00:00Z", []string{g.merge, g.f2, g.m2, g.f1}},
		{"since=2026-10-01T10:00:00%2B02:00", []string{g.merge, g.f2, g.m2, g.f1, g.m1, g.r1}},
		{"until=2026-10-01T10:00:00Z", []string{g.f1, g.m1, g.r1}},
		{"sha=feature&since=2026-10-01T09:00:00Z&until=2026-10-01T11:00:00Z", []string{g.f1}},
	}
	for _, c := range cases {
		r := get(t, s, "repos/alice/projet/commits?"+c.query)
		wantStatus(t, c.query, r, 200)
		decode(t, r, &list)
		if !reflect.DeepEqual(list.shas(), c.want) {
			t.Errorf("%s: %v\nwant %v", c.query, list.shas(), c.want)
		}
	}

	for _, q := range []string{"sha=inconnue", "sha=" + FakeSHA("absent")} {
		if r := get(t, s, "repos/alice/projet/commits?"+q); r.status != 404 || message(t, r) != "Not Found" {
			t.Errorf("%s: HTTP %d %s, want 404", q, r.status, r.body)
		}
	}
	if r := get(t, s, "repos/alice/projet/commits?since=hier"); r.status != 422 {
		t.Errorf("unreadable since: HTTP %d, want 422", r.status)
	}

	// At equal dates, the last added first.
	a := s.AddCommit(101, Commit{Message: "a", CommittedAt: t0.Add(6 * time.Hour), Parents: []string{g.merge}})
	b := s.AddCommit(101, Commit{Message: "b", CommittedAt: t0.Add(6 * time.Hour), Parents: []string{g.merge}})
	mergeAB := s.AddCommit(101, Commit{Message: "ab", CommittedAt: t0.Add(7 * time.Hour), Parents: []string{a, b}})
	s.SetBranch(101, "main", mergeAB)
	r = get(t, s, "repos/alice/projet/commits?per_page=3")
	decode(t, r, &list)
	if want := []string{mergeAB, b, a}; !reflect.DeepEqual(list.shas(), want) {
		t.Errorf("equal dates: %v, want %v", list.shas(), want)
	}
}

func TestListCommitsPagination(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 3, Owner: "alice", Name: "gros"})
	shas := s.AppendCommits(3, "main", 250, t0, time.Minute)

	var got []string
	for p := 1; p <= 4; p++ {
		r := get(t, s, fmt.Sprintf("repos/alice/gros/commits?sha=main&per_page=100&page=%d", p))
		wantStatus(t, "page", r, 200)
		var list commitList
		decode(t, r, &list)
		if wantLen := []int{100, 100, 50, 0}[p-1]; len(list) != wantLen {
			t.Errorf("page %d: %d commits, want %d", p, len(list), wantLen)
		}
		got = append(got, list.shas()...)

		link := func(n int, rel string) string {
			return fmt.Sprintf(`<%srepositories/3/commits?page=%d&per_page=100&sha=main>; rel="%s"`, s.URL(), n, rel)
		}
		wantLink := map[int]string{
			1: link(2, "next") + ", " + link(3, "last"),
			2: link(1, "prev") + ", " + link(3, "next") + ", " + link(3, "last") + ", " + link(1, "first"),
			3: link(2, "prev") + ", " + link(1, "first"),
			4: link(3, "prev") + ", " + link(1, "first"),
		}[p]
		if l := r.header.Get("Link"); l != wantLink {
			t.Errorf("page %d: Link = %q\nwant %q", p, l, wantLink)
		}
	}
	if len(got) != 250 || got[0] != shas[249] || got[249] != shas[0] {
		t.Errorf("all the pages: %d commits, first %s (want %s), last %s (want %s)", len(got),
			got[0], shas[249], got[len(got)-1], shas[0])
	}

	for q, want := range map[string]int{"": 30, "per_page=500": 100, "per_page=0": 30, "per_page=abc&page=-1": 30, "per_page=7": 7} {
		r := get(t, s, "repos/alice/gros/commits?"+q)
		var list commitList
		decode(t, r, &list)
		if len(list) != want {
			t.Errorf("%q: %d commits, want %d", q, len(list), want)
		}
	}
}

func TestCommit(t *testing.T) {
	s := NewServer(t, Options{})
	g := newGraphRepo(s)
	for _, ref := range []string{g.merge, "main"} {
		r := get(t, s, "repos/alice/projet/commits/"+ref)
		wantStatus(t, ref, r, 200)
		var c struct {
			SHA   string `json:"sha"`
			Stats struct {
				Total     int `json:"total"`
				Additions int `json:"additions"`
				Deletions int `json:"deletions"`
			} `json:"stats"`
			Files   []any `json:"files"`
			Parents []any `json:"parents"`
		}
		decode(t, r, &c)
		if c.SHA != g.merge || c.Stats.Additions != 12 || c.Stats.Deletions != 3 || c.Stats.Total != 15 ||
			c.Files == nil || len(c.Files) != 0 || len(c.Parents) != 2 {
			t.Errorf("%s: %+v", ref, c)
		}
	}
	r := get(t, s, "repos/alice/projet/commits/"+g.r1)
	if !strings.Contains(string(r.body), `"stats":{"total":0,"additions":0,"deletions":0}`) {
		t.Errorf("zero stats missing: %s", r.body)
	}

	// The list contains neither stats nor files.
	r = get(t, s, "repos/alice/projet/commits")
	if strings.Contains(string(r.body), `"stats"`) || strings.Contains(string(r.body), `"files"`) {
		t.Errorf("the list of commits must contain neither stats nor files: %s", r.body)
	}

	unknown := FakeSHA("absent")
	r = get(t, s, "repos/alice/projet/commits/"+unknown)
	if r.status != 422 || message(t, r) != "No commit found for SHA: "+unknown {
		t.Errorf("unknown commit: HTTP %d %s, want 422", r.status, r.body)
	}
}

// short shortens a SHA for the messages.
func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// compareResult is the decoded form of a comparison.
type compareResult struct {
	Status       string               `json:"status"`
	AheadBy      int                  `json:"ahead_by"`
	BehindBy     int                  `json:"behind_by"`
	TotalCommits int                  `json:"total_commits"`
	BaseCommit   struct{ SHA string } `json:"base_commit"`
	MergeBase    struct{ SHA string } `json:"merge_base_commit"`
	Commits      commitList           `json:"commits"`
	Files        []any                `json:"files"`
}

func TestCompare(t *testing.T) {
	s := NewServer(t, Options{})
	g := newGraphRepo(s)
	cases := []struct {
		base, head    string
		status        string
		ahead, behind int
		mergeBase     string
		commits       []string
	}{
		{g.m2, g.merge, "ahead", 3, 0, g.m2, []string{g.f1, g.f2, g.merge}},
		{g.m1, "main", "ahead", 4, 0, g.m1, []string{g.f1, g.m2, g.f2, g.merge}},
		{g.merge, g.m2, "behind", 0, 3, g.m2, []string{}},
		{g.f2, g.m2, "diverged", 2, 2, g.r1, []string{g.m1, g.m2}},
		{"feature", g.m2, "diverged", 2, 2, g.r1, []string{g.m1, g.m2}},
		{g.merge, g.merge, "identical", 0, 0, g.merge, []string{}},
	}
	for _, c := range cases {
		path := "repos/alice/projet/compare/" + c.base + "..." + c.head
		r := get(t, s, path)
		wantStatus(t, path, r, 200)
		var res compareResult
		decode(t, r, &res)
		if res.Status != c.status || res.AheadBy != c.ahead || res.BehindBy != c.behind || res.TotalCommits != c.ahead ||
			res.MergeBase.SHA != c.mergeBase || res.Files == nil || res.Commits == nil {
			t.Errorf("%s...%s: status %s, ahead %d, behind %d, total %d, merge base %s; want %s, %d, %d, %s",
				short(c.base), short(c.head), res.Status, res.AheadBy, res.BehindBy, res.TotalCommits, short(res.MergeBase.SHA),
				c.status, c.ahead, c.behind, short(c.mergeBase))
		}
		if !reflect.DeepEqual(res.Commits.shas(), c.commits) {
			t.Errorf("%s...%s: commits %v\nwant %v", short(c.base), short(c.head), res.Commits.shas(), c.commits)
		}
	}

	// Pagination: from oldest to newest, page 1 = the oldest.
	s.AddRepo(Repo{ID: 5, Owner: "alice", Name: "long"})
	shas := s.AppendCommits(5, "main", 5, t0, time.Hour)
	var got []string
	for p := 1; p <= 3; p++ {
		r := get(t, s, fmt.Sprintf("repos/alice/long/compare/%s...%s?per_page=2&page=%d", shas[0], shas[4], p))
		var res compareResult
		decode(t, r, &res)
		if res.TotalCommits != 4 {
			t.Errorf("page %d: total_commits %d, want 4", p, res.TotalCommits)
		}
		got = append(got, res.Commits.shas()...)
		if p == 1 && !strings.Contains(r.header.Get("Link"), `rel="next"`) {
			t.Errorf("page 1 without next link: %q", r.header.Get("Link"))
		}
		if p == 1 && !strings.Contains(r.header.Get("Link"), s.URL()+"repositories/5/compare/"+shas[0]+"..."+shas[4]+"?page=2&per_page=2") {
			t.Errorf("link of page 2: %q", r.header.Get("Link"))
		}
	}
	if !reflect.DeepEqual(got, shas[1:]) {
		t.Errorf("compare pages: %v\nwant %v", got, shas[1:])
	}

	// Unknown base or head, removed base, histories without common ancestor: 404.
	orphan := s.AddCommit(101, Commit{Message: "orphelin", CommittedAt: t0.Add(9 * time.Hour)})
	s.SetBranch(101, "orpheline", orphan)
	s.SetBranch(101, "feature", g.f1) // force-push: f2 is no longer the head of anything
	s.RemoveCommit(101, g.f2)
	for _, spec := range []string{FakeSHA("absent") + "..." + g.merge, g.merge + "..." + FakeSHA("absent"),
		g.f2 + "..." + g.merge, g.m1 + "..." + orphan, g.m1 + ".." + g.merge} {
		r := get(t, s, "repos/alice/projet/compare/"+spec)
		if r.status != 404 || message(t, r) != "Not Found" {
			t.Errorf("compare %s: HTTP %d %s, want 404", spec, r.status, r.body)
		}
	}
}

func TestForcePush(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	old := s.AppendCommits(1, "main", 3, t0, time.Hour)
	s.SetBranch(1, "main", old[0]) // going back
	rewritten := s.AppendCommits(1, "main", 2, t0.Add(10*time.Hour), time.Hour)

	r := get(t, s, "repos/alice/projet/commits")
	var list commitList
	decode(t, r, &list)
	if want := []string{rewritten[1], rewritten[0], old[0]}; !reflect.DeepEqual(list.shas(), want) {
		t.Errorf("after force-push: %v, want %v", list.shas(), want)
	}
	if list[0].Commit.Message != "commit 2" || list[1].Parents[0].SHA != old[0] {
		t.Errorf("rewritten history: %+v", list[:2])
	}

	// Before cleanup, the old head stays comparable; after, 404.
	spec := old[2] + "..." + rewritten[1]
	if r := get(t, s, "repos/alice/projet/compare/"+spec); r.status != 200 {
		t.Errorf("compare before cleanup: HTTP %d", r.status)
	}
	s.RemoveCommit(1, old[2])
	if r := get(t, s, "repos/alice/projet/compare/"+spec); r.status != 404 {
		t.Errorf("compare after cleanup: HTTP %d, want 404", r.status)
	}
	// The parent of the removed commit stays present.
	if r := get(t, s, "repos/alice/projet/commits/"+old[1]); r.status != 200 {
		t.Errorf("commit not removed: HTTP %d", r.status)
	}
}

func TestETag(t *testing.T) {
	s := NewServer(t, Options{})
	newGraphRepo(s)
	path := "repos/alice/projet/commits?per_page=2"
	r := get(t, s, path)
	etag := r.header.Get("ETag")
	if !regexp.MustCompile(`^W/"[0-9a-f]{64}"$`).MatchString(etag) {
		t.Fatalf("ETag = %q, want W/\"<sha256 hex>\"", etag)
	}
	if r.header.Get("Link") == "" {
		t.Fatal("paginated list without Link")
	}
	strong := strings.TrimPrefix(etag, "W/")

	for _, inm := range []string{etag, strong, `"autre", ` + etag, "*", `W/"autre",` + strong} {
		before := s.Quota()
		r := get(t, s, path, "If-None-Match", inm)
		if r.status != 304 {
			t.Errorf("If-None-Match %q: HTTP %d, want 304", inm, r.status)
			continue
		}
		if len(r.body) != 0 || r.header.Get("Link") != "" || r.header.Get("Content-Type") != "" {
			t.Errorf("304: body %q, Link %q, Content-Type %q; want all empty", r.body, r.header.Get("Link"), r.header.Get("Content-Type"))
		}
		if r.header.Get("ETag") != strong {
			t.Errorf("304: ETag %q, want the strong ETag %q", r.header.Get("ETag"), strong)
		}
		if after := s.Quota(); after.Remaining != before.Remaining {
			t.Errorf("a 304 consumed quota: %d → %d", before.Remaining, after.Remaining)
		}
		if got := r.header.Get("X-RateLimit-Remaining"); got != strconv.Itoa(before.Remaining) || r.header.Get("X-RateLimit-Resource") != "core" {
			t.Errorf("304: X-RateLimit-Remaining %q, want %d", got, before.Remaining)
		}
	}

	if r := get(t, s, path, "If-None-Match", `W/"autre"`); r.status != 200 || r.header.Get("ETag") != etag {
		t.Errorf("If-None-Match without match: HTTP %d, ETag %q", r.status, r.header.Get("ETag"))
	}
	// Other page, other ETag.
	if r := get(t, s, path+"&page=2", "If-None-Match", etag); r.status != 200 || r.header.Get("ETag") == etag {
		t.Errorf("page 2 with the ETag of page 1: HTTP %d, ETag %q", r.status, r.header.Get("ETag"))
	}
	// New commit: the ETag changes.
	sha := s.AddCommit(101, Commit{Message: "nouveau", CommittedAt: t0.Add(24 * time.Hour), Parents: []string{s.headLocked(101, "main")}})
	s.SetBranch(101, "main", sha)
	if r := get(t, s, path, "If-None-Match", etag); r.status != 200 || r.header.Get("ETag") == etag {
		t.Errorf("after a new commit: HTTP %d, ETag %q", r.status, r.header.Get("ETag"))
	}
	// ETag also on /rate_limit and on the repository.
	for _, p := range []string{"rate_limit", "repos/alice/projet"} {
		r := get(t, s, p)
		if r2 := get(t, s, p, "If-None-Match", r.header.Get("ETag")); r2.status != 304 {
			t.Errorf("%s: HTTP %d with its ETag, want 304", p, r2.status)
		}
	}
	// No ETag on an error.
	if r := get(t, s, "repos/alice/inconnu"); r.header.Get("ETag") != "" {
		t.Errorf("ETag on a 404: %q", r.header.Get("ETag"))
	}
}

// headLocked returns the head of a branch (test helper).
func (s *Server) headLocked(id int64, branch string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.repos[id].branches[branch]
}

func TestQuota(t *testing.T) {
	clock := &fixedClock{now: t0}
	s := NewServer(t, Options{Now: clock.Now})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})

	r := get(t, s, "repos/alice/projet")
	h := r.header
	if h.Get("X-RateLimit-Limit") != "5000" || h.Get("X-RateLimit-Remaining") != "4999" || h.Get("X-RateLimit-Used") != "1" ||
		h.Get("X-RateLimit-Reset") != strconv.FormatInt(t0.Add(time.Hour).Unix(), 10) || h.Get("X-RateLimit-Resource") != "core" {
		t.Errorf("quota headers: %v", h)
	}
	if d := h.Get("Date"); d != t0.Format(http.TimeFormat) {
		t.Errorf("Date = %q, want the server's time %q", d, t0.Format(http.TimeFormat))
	}

	// /rate_limit is free.
	r = get(t, s, "rate_limit")
	var rl struct {
		Resources struct {
			Core struct {
				Limit, Used, Remaining int
				Reset                  int64
			} `json:"core"`
		} `json:"resources"`
		Rate struct {
			Remaining int `json:"remaining"`
		} `json:"rate"`
	}
	decode(t, r, &rl)
	core := rl.Resources.Core
	if core.Limit != 5000 || core.Remaining != 4999 || core.Used != 1 || core.Reset != t0.Add(time.Hour).Unix() || rl.Rate.Remaining != 4999 {
		t.Errorf("/rate_limit: %+v", rl)
	}
	if q := s.Quota(); q.Remaining != 4999 {
		t.Errorf("/rate_limit consumed quota: %+v", q)
	}

	// Exhausted quota.
	reset := t0.Add(30 * time.Minute)
	s.SetQuota(Quota{Limit: 100, Remaining: 1, Reset: reset})
	if r := get(t, s, "repos/alice/projet"); r.status != 200 || r.header.Get("X-RateLimit-Remaining") != "0" || r.header.Get("X-RateLimit-Used") != "100" {
		t.Errorf("last request of the quota: HTTP %d, Remaining %q, Used %q", r.status, r.header.Get("X-RateLimit-Remaining"), r.header.Get("X-RateLimit-Used"))
	}
	for _, path := range []string{"repos/alice/projet", "repos/alice/inconnu"} {
		r := get(t, s, path)
		if r.status != 403 || !strings.HasPrefix(message(t, r), "API rate limit exceeded") ||
			r.header.Get("X-RateLimit-Remaining") != "0" || r.header.Get("X-RateLimit-Reset") != strconv.FormatInt(reset.Unix(), 10) {
			t.Errorf("exhausted quota, %s: HTTP %d %s %v", path, r.status, r.body, r.header)
		}
	}
	if r := get(t, s, "rate_limit"); r.status != 200 {
		t.Errorf("/rate_limit with exhausted quota: HTTP %d, want 200", r.status)
	}
	if q := s.Quota(); q.Remaining != 0 || q.Limit != 100 || !q.Reset.Equal(reset) {
		t.Errorf("Quota() = %+v", q)
	}

	s.SetQuota(Quota{Limit: 100, Remaining: 0, Reset: reset, Status: 429})
	if r := get(t, s, "repos/alice/projet"); r.status != 429 {
		t.Errorf("exhausted quota with Status 429: HTTP %d", r.status)
	}

	// New window once Reset has passed.
	clock.Advance(30 * time.Minute)
	r = get(t, s, "repos/alice/projet")
	if r.status != 200 || r.header.Get("X-RateLimit-Remaining") != "99" || r.header.Get("X-RateLimit-Used") != "1" ||
		r.header.Get("X-RateLimit-Reset") != strconv.FormatInt(t0.Add(90*time.Minute).Unix(), 10) {
		t.Errorf("new window: HTTP %d, %v", r.status, r.header)
	}
}

func TestSecondaryLimit(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	s.TriggerSecondaryLimit(SecondaryLimit{RetryAfter: 1500 * time.Millisecond, Requests: 2})
	before := s.Quota().Remaining
	for i := range 2 {
		r := get(t, s, "repos/alice/projet")
		var body map[string]string
		decode(t, r, &body)
		if r.status != 403 || r.header.Get("Retry-After") != "2" ||
			!strings.Contains(body["message"], "secondary rate limit") ||
			!strings.HasSuffix(body["documentation_url"], "#about-secondary-rate-limits") {
			t.Errorf("refusal %d: HTTP %d, Retry-After %q, body %v", i+1, r.status, r.header.Get("Retry-After"), body)
		}
		if r.header.Get("X-RateLimit-Remaining") != strconv.Itoa(before) {
			t.Errorf("refusal %d: X-RateLimit-Remaining %q, want %d", i+1, r.header.Get("X-RateLimit-Remaining"), before)
		}
		if i == 0 {
			if r := get(t, s, "rate_limit"); r.status != 200 {
				t.Errorf("/rate_limit during a secondary limit: HTTP %d", r.status)
			}
		}
	}
	if q := s.Quota(); q.Remaining != before {
		t.Errorf("secondary limit: %d units consumed", before-q.Remaining)
	}
	if r := get(t, s, "repos/alice/projet"); r.status != 200 {
		t.Errorf("after the secondary limit: HTTP %d", r.status)
	}

	s.TriggerSecondaryLimit(SecondaryLimit{Status: 429, RetryAfter: 100 * time.Millisecond})
	if r := get(t, s, "repos/alice/projet"); r.status != 429 || r.header.Get("Retry-After") != "1" {
		t.Errorf("429: HTTP %d, Retry-After %q (want 1 at least)", r.status, r.header.Get("Retry-After"))
	}
	s.TriggerSecondaryLimit(SecondaryLimit{})
	if r := get(t, s, "repos/alice/projet"); r.status != 403 || r.header.Get("Retry-After") != "" ||
		!strings.Contains(message(t, r), "secondary rate limit") {
		t.Errorf("without Retry-After: HTTP %d, Retry-After %q", r.status, r.header.Get("Retry-After"))
	}
}

func TestInject(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	s.Inject("/repos/alice/projet", Response{
		Status: 502,
		Header: http.Header{"X-Test": {"oui"}, "X-Ratelimit-Limit": {"60"}},
		Body:   `{"message":"Server Error"}`,
		Times:  2,
	})
	before := s.Quota().Remaining
	for i := range 2 {
		r := get(t, s, "repos/alice/projet")
		if r.status != 502 || string(r.body) != `{"message":"Server Error"}` || r.header.Get("X-Test") != "oui" ||
			r.header.Get("X-RateLimit-Limit") != "60" || r.header.Get("X-RateLimit-Remaining") == "" ||
			r.header.Get("Content-Type") != "application/json; charset=utf-8" {
			t.Errorf("injection %d: HTTP %d %s %v", i+1, r.status, r.body, r.header)
		}
	}
	if consumed := before - s.Quota().Remaining; consumed != 2 {
		t.Errorf("injections: %d units consumed, want 2", consumed)
	}
	if r := get(t, s, "repos/alice/projet"); r.status != 200 {
		t.Errorf("after Times injections: HTTP %d, want 200", r.status)
	}
	if r := get(t, s, "repos/alice/projet?x=1"); r.status != 200 {
		t.Errorf("same path, other parameters: HTTP %d", r.status)
	}

	// Times 0 = all, until ClearInjections; a new injection replaces the previous one.
	s.Inject("/repos/alice/projet/branches", Response{Body: `[]`, Header: http.Header{"Content-Type": {"text/plain"}}})
	s.Inject("/repos/alice/projet/branches", Response{Status: 304})
	before = s.Quota().Remaining
	for range 3 {
		if r := get(t, s, "repos/alice/projet/branches"); r.status != 304 {
			t.Errorf("replaced injection: HTTP %d, want 304", r.status)
		}
	}
	if s.Quota().Remaining != before {
		t.Error("an injected 304 consumed quota")
	}
	// The injection takes precedence over the exhausted quota and the secondary limit.
	s.SetQuota(Quota{Remaining: 0})
	if r := get(t, s, "repos/alice/projet/branches"); r.status != 304 {
		t.Errorf("injection with exhausted quota: HTTP %d", r.status)
	}
	s.ClearInjections()
	if r := get(t, s, "repos/alice/projet/branches"); r.status != 403 {
		t.Errorf("after ClearInjections, exhausted quota: HTTP %d, want 403", r.status)
	}
	// But not over a refused token.
	s.Inject("/repos/alice/projet", Response{Body: `{}`})
	if r := do(t, s, http.MethodGet, "repos/alice/projet", nil); r.status != 401 {
		t.Errorf("injection without token: HTTP %d, want 401", r.status)
	}
}

func TestInjectDelay(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	s.Inject("/repos/alice/projet", Response{Body: `{}`, Delay: time.Minute})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.URL()+"repos/alice/projet", nil)
	req.Header.Set("Authorization", "Bearer "+s.Token())
	start := time.Now()
	_, err := http.DefaultClient.Do(req)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("delayed request: error %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("late abort: %s", elapsed)
	}

	// Close interrupts a wait in progress.
	s2 := NewServer(t, Options{})
	s2.Inject("/rate_limit", Response{Body: `{}`, Delay: time.Minute})
	errc := make(chan error, 1)
	go func() {
		req, _ := http.NewRequest(http.MethodGet, s2.URL()+"rate_limit", nil)
		req.Header.Set("Authorization", "Bearer "+s2.Token())
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		errc <- err
	}()
	for s2.Count("/rate_limit") == 0 {
		time.Sleep(time.Millisecond)
	}
	start = time.Now()
	s2.Close()
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Close blocked %s by a delayed response", elapsed)
	}
	if err := <-errc; err == nil {
		t.Error("a delayed response interrupted by Close was served")
	}
}

func TestJournal(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	get(t, s, "repos/alice/projet", "User-Agent", "watchoo/test", "If-None-Match", `W/"abc"`)
	get(t, s, "repos/alice/projet/branches?per_page=100&page=2")
	do(t, s, http.MethodHead, "repos/alice/projet", auth(s))

	reqs := s.Requests()
	if len(reqs) != 3 {
		t.Fatalf("log: %d requests, want 3", len(reqs))
	}
	want0 := Request{Method: "GET", Path: "/repos/alice/projet", Query: url.Values{}, IfNoneMatch: `W/"abc"`,
		UserAgent: "watchoo/test", HasAuthorization: true, ValidToken: true, Status: 200}
	if !reflect.DeepEqual(reqs[0], want0) {
		t.Errorf("request 1: %+v\nwant %+v", reqs[0], want0)
	}
	if reqs[1].RawQuery != "per_page=100&page=2" || reqs[1].Query.Get("page") != "2" || reqs[1].Path != "/repos/alice/projet/branches" {
		t.Errorf("request 2: %+v", reqs[1])
	}
	if reqs[2].Method != "HEAD" || reqs[2].Status != 200 {
		t.Errorf("request 3: %+v", reqs[2])
	}

	reqs[1].Query.Set("page", "99")
	if s.Requests()[1].Query.Get("page") != "2" {
		t.Error("Requests does not return a copy")
	}
	if n := s.Count("/repos/alice/projet"); n != 2 {
		t.Errorf("Count = %d, want 2", n)
	}
	s.ResetRequests()
	if len(s.Requests()) != 0 || s.Count("/repos/alice/projet") != 0 {
		t.Error("ResetRequests did not empty the log")
	}
}

func TestHead(t *testing.T) {
	s := NewServer(t, Options{})
	newGraphRepo(s)
	g := get(t, s, "repos/alice/projet/commits?per_page=2")
	h := do(t, s, http.MethodHead, "repos/alice/projet/commits?per_page=2", auth(s))
	if h.status != 200 || len(h.body) != 0 || h.header.Get("ETag") != g.header.Get("ETag") || h.header.Get("Link") != g.header.Get("Link") {
		t.Errorf("HEAD: HTTP %d, body %q, ETag %q (GET %q)", h.status, h.body, h.header.Get("ETag"), g.header.Get("ETag"))
	}
}

func TestAppendCommits(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	if got := s.AppendCommits(1, "main", 0, time.Time{}, 0); got == nil || len(got) != 0 {
		t.Errorf("n = 0: %#v, want an empty non-nil slice", got)
	}
	first := s.AppendCommits(1, "main", 2, t0, time.Hour)
	second := s.AppendCommits(1, "main", 2, t0.Add(5*time.Hour), time.Minute)
	if first[0] == second[0] || first[1] == second[1] {
		t.Error("two successive calls produce the same SHAs")
	}
	r := get(t, s, "repos/alice/projet/commits")
	var list commitList
	decode(t, r, &list)
	if want := []string{second[1], second[0], first[1], first[0]}; !reflect.DeepEqual(list.shas(), want) {
		t.Fatalf("history: %v, want %v", list.shas(), want)
	}
	c := list[1]
	if c.Commit.Message != "commit 1" || c.Commit.Author.Name != "Étudiant Test" || c.Commit.Author.Email != "etudiant@example.test" ||
		c.Commit.Committer.Name != "Étudiant Test" || c.Author == nil || c.Author.Login != "etudiant-test" ||
		c.Committer == nil || c.Committer.Login != "etudiant-test" || c.Commit.Author.Date != "2026-10-01T13:00:00Z" ||
		c.Commit.Committer.Date != c.Commit.Author.Date || len(c.Parents) != 1 || c.Parents[0].SHA != first[1] {
		t.Errorf("added commit: %+v", c)
	}
	if len(list[3].Parents) != 0 {
		t.Errorf("first commit: parents %v, want none", list[3].Parents)
	}
	if s.headLocked(1, "main") != second[1] {
		t.Error("the branch is not put on the last commit")
	}
	// Another branch: new history.
	other := s.AppendCommits(1, "autre", 1, t0, 0)
	r = get(t, s, "repos/alice/projet/commits?sha=autre")
	decode(t, r, &list)
	if !reflect.DeepEqual(list.shas(), other) {
		t.Errorf("new branch: %v, want %v", list.shas(), other)
	}
}

func TestAddCommitDefaults(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	explicit := FakeSHA("explicite")
	got := s.AddCommit(1, Commit{SHA: explicit, Message: "x", AuthoredAt: time.Date(2026, 1, 2, 3, 4, 5, 600, time.FixedZone("X", 3600)),
		Author: Person{Name: "A", Login: "compte", ID: 77, AvatarURL: "https://avatars.example.test/perso"}})
	if got != explicit {
		t.Errorf("AddCommit returned %s, want the given SHA %s", got, explicit)
	}
	s.SetBranch(1, "main", got)
	r := get(t, s, "repos/alice/projet/commits")
	var list commitList
	decode(t, r, &list)
	c := list[0]
	if c.Commit.Author.Date != "2026-01-02T02:04:05Z" || c.Commit.Committer.Date != c.Commit.Author.Date {
		t.Errorf("default dates: %+v", c.Commit)
	}
	if c.Author == nil || c.Author.ID != 77 || c.Author.AvatarURL != "https://avatars.example.test/perso" ||
		c.Committer == nil || c.Committer.ID != 77 {
		t.Errorf("default committer = author: %+v %+v", c.Author, c.Committer)
	}
	if accountID("Alice") != accountID("alice") || accountID("alice") == 0 || accountID("alice") == accountID("bob") {
		t.Error("accountID must be stable, not zero and case-insensitive")
	}
}

func TestPreparationErrors(t *testing.T) {
	rec := &recordTB{TB: t}
	s := NewServer(rec, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	sha := s.AddCommit(1, Commit{Message: "init", CommittedAt: t0})
	s.SetBranch(1, "main", sha)

	cases := map[string]func(){
		"AddRepo zero ID":               func() { s.AddRepo(Repo{Owner: "a", Name: "b"}) },
		"AddRepo taken ID":              func() { s.AddRepo(Repo{ID: 1, Owner: "a", Name: "b"}) },
		"AddRepo taken name":            func() { s.AddRepo(Repo{ID: 2, Owner: "ALICE", Name: "Projet"}) },
		"AddRepo empty name":            func() { s.AddRepo(Repo{ID: 2, Owner: "alice"}) },
		"AddRepo name with /":           func() { s.AddRepo(Repo{ID: 2, Owner: "alice", Name: "a/b"}) },
		"RenameRepo unknown":            func() { s.RenameRepo(9, "a", "b") },
		"SetRepoAccess unknown access":  func() { s.SetRepoAccess(1, Access(9)) },
		"AddCommit unknown repository":  func() { s.AddCommit(9, Commit{CommittedAt: t0}) },
		"AddCommit without date":        func() { s.AddCommit(1, Commit{Message: "x"}) },
		"AddCommit unknown parent":      func() { s.AddCommit(1, Commit{CommittedAt: t0, Parents: []string{FakeSHA("p")}}) },
		"AddCommit malformed SHA":       func() { s.AddCommit(1, Commit{SHA: "ABC", CommittedAt: t0}) },
		"AddCommit SHA already present": func() { s.AddCommit(1, Commit{SHA: sha, CommittedAt: t0}) },
		"SetBranch unknown commit":      func() { s.SetBranch(1, "dev", FakeSHA("inconnu")) },
		"SetBranch empty name":          func() { s.SetBranch(1, "", sha) },
		"DeleteBranch unknown":          func() { s.DeleteBranch(1, "inconnue") },
		"RemoveCommit head":             func() { s.RemoveCommit(1, sha) },
		"RemoveCommit unknown":          func() { s.RemoveCommit(1, FakeSHA("inconnu")) },
		"AppendCommits negative n":      func() { s.AppendCommits(1, "main", -1, t0, 0) },
		"AppendCommits without date":    func() { s.AppendCommits(1, "main", 1, time.Time{}, 0) },
		"SetQuota invalid code":         func() { s.SetQuota(Quota{Status: 500}) },
		"SetQuota negative":             func() { s.SetQuota(Quota{Remaining: -1}) },
		"Secondary invalid code":        func() { s.TriggerSecondaryLimit(SecondaryLimit{Status: 500}) },
		"Inject relative path":          func() { s.Inject("repos/a/b", Response{}) },
		"Inject negative Times":         func() { s.Inject("/a", Response{Times: -1}) },
	}
	for what, fn := range cases {
		expectFatal(t, rec, what, fn)
	}
	// The lock was released despite the Fatalf calls: the server still answers.
	if r := get(t, s, "repos/alice/projet"); r.status != 200 {
		t.Errorf("after setup errors: HTTP %d", r.status)
	}
	if errs, _ := rec.counts(); errs != 0 {
		t.Errorf("%d unexpected call(s) to Errorf", errs)
	}
}

func TestConcurrentRequests(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	s.AppendCommits(1, "main", 20, t0, time.Minute)
	client := &http.Client{Timeout: 10 * time.Second}

	const workers, perWorker = 8, 25
	var wg sync.WaitGroup
	errc := make(chan error, workers*perWorker)
	for w := range workers {
		wg.Go(func() {
			for i := range perWorker {
				path := []string{"repos/alice/projet", "repos/alice/projet/commits?per_page=5", "repos/alice/projet/branches", "rate_limit"}[(w+i)%4]
				req, _ := http.NewRequest(http.MethodGet, s.URL()+path, nil)
				req.Header.Set("Authorization", "Bearer "+s.Token())
				resp, err := client.Do(req)
				if err != nil {
					errc <- err
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 {
					errc <- fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
				}
			}
		})
	}
	// Concurrent setup from the test's goroutine.
	for i := range 10 {
		s.AppendCommits(1, "dev", 1, t0.Add(time.Duration(i)*time.Hour), 0)
		_ = s.Requests()
		_ = s.Quota()
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Error(err)
	}
	if n := len(s.Requests()); n != workers*perWorker {
		t.Errorf("log: %d requests, want %d", n, workers*perWorker)
	}
	paying := workers * perWorker * 3 / 4 // /rate_limit is free
	if consumed := DefaultRateLimit - s.Quota().Remaining; consumed != paying {
		t.Errorf("consumed quota: %d, want %d", consumed, paying)
	}
}
