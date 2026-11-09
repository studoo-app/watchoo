// Package githubtest provides a fake, programmable server of GitHub's REST API, based on
// net/http/httptest. It serves the tests of internal/provider/github (phase 2) and the end-to-end
// tests of the sync engine (phase 3). No real network call: the server listens on 127.0.0.1.
//
// Standard library only: this package imports neither go-github nor internal/provider/github
// (the tests of the github package import it: no cycle). Dummy data only.
//
// # Simulated behavior (faithful to the real API, checked with gh 2.103 in October 2026)
//
// Served routes (GET and HEAD; owner and name compared case-insensitively):
//   - GET /repos/{owner}/{name} and GET /repositories/{id}: repository metadata;
//   - GET …/branches?per_page&page: branches sorted by name, {"name","commit":{"sha","url"},"protected":false};
//   - GET …/commits?sha&since&until&per_page&page: commits reachable from sha (branch or full
//     SHA; empty = default branch), sorted by descending commit date (at equal dates, the last
//     added first), filtered by since/until (commit date, bounds included, RFC 3339; an unreadable
//     date → 422); unknown sha → 404 "Not Found";
//   - GET …/commits/{ref} (full SHA or branch name): one commit with
//     "stats":{"additions","deletions","total"} and "files":[]; unknown ref → 422 "No commit found
//     for SHA: …", like GitHub;
//   - GET …/compare/{base}...{head}?per_page&page (full SHAs or branch names): {"status",
//     "ahead_by","behind_by","total_commits","base_commit","merge_base_commit","commits","files":[]};
//     commits = reachable from head but not from base, from OLDEST to newest (ascending commit
//     date, then order of addition), paginated; unknown base or head, or no common ancestor
//     → 404 "Not Found" (like GitHub);
//   - GET …/pulls, …/releases, …/tags (V2): see pulls.go;
//   - GET /rate_limit: {"resources":{"core":{…},…},"rate":{…}}, free, always served (even with an
//     exhausted quota, even during a secondary limit).
//
// Any other route answers 404 "Not Found".
//
// For every request, in order:
//  1. method other than GET or HEAD: t.Errorf (the test fails) and a 405 response, with no other
//     effect (no entry in the log);
//  2. recording in the log (Requests);
//  3. Authorization header missing, repeated or different from "Bearer <token>" / "token <token>"
//     (case-insensitive scheme): 401 {"message":"Bad credentials",…}, without quota headers,
//     without consuming quota;
//  4. injected response (Inject) for this path: served as is (with the quota headers);
//  5. secondary limit in progress (TriggerSecondaryLimit): 403 or 429 with Retry-After, except
//     /rate_limit;
//  6. exhausted quota (Remaining == 0 and now < Reset): Quota.Status (403 by default) with
//     X-RateLimit-Remaining: 0 and {"message":"API rate limit exceeded for user ID 1.", …}, except
//     /rate_limit. When now >= Reset, a new window starts (Remaining = Limit,
//     Reset = now + 1 h): this check happens on every authorized request, before step 4;
//  7. routing: unknown repository or AccessHidden → 404 {"message":"Not Found","documentation_url":…,
//     "status":"404"}; AccessSSO → 403 with "X-GitHub-SSO: required; url=https://github.com/orgs/
//     {owner}/sso?authorization_request=fake" and {"message":"Resource protected by organization SAML
//     enforcement. …"}; old name of a renamed (accessible) repository → 301, Location =
//     URL()+"repositories/{id}" + rest of the path + original parameters, body {"message":"Moved
//     Permanently","url":…} (AccessHidden and AccessSSO also apply to old names, without
//     redirection: nothing is revealed about an inaccessible repository); repository without any
//     branch (empty): …/commits, …/commits/{ref} and …/compare → 409 {"message":"Git Repository is empty."},
//     …/branches → 200 [];
//  8. 200 response: JSON body, Content-Type "application/json; charset=utf-8", ETag
//     W/"<sha256 hex of the body>", Link in GitHub's format (rel="prev", "next", "last", "first"; absolute
//     URLs under URL()+"repositories/{id}/…", like GitHub) for paginated lists. If
//     If-None-Match matches the ETag (with or without W/, comma-separated list, or "*"):
//     304 without body, WITHOUT Link or Content-Type (like GitHub), with a strong ETag "<sha256 hex>", and
//     without consuming quota.
//
// Quota: every response at steps 4 to 8 (304s included) carries X-RateLimit-Limit, -Remaining, -Used,
// -Reset (Unix seconds) and -Resource: core; every authorized response, except 304s, /rate_limit and
// quota or secondary limit refusals, consumes one unit (Remaining--, Used++) before the headers are
// computed. The Date header follows the server's clock (Options.Now).
//
// Pagination: per_page 30 by default, at most 100 (a larger value is brought back to 100); page 1
// by default; a page past the last one returns [] (or "commits": [] for compare).
//
// Dates: as on GitHub, they are to the second; the dates received by AddRepo, AddCommit and
// AppendCommits are converted to UTC and truncated to the second.
//
// The server is safe for concurrent use. The setup methods (AddRepo, AddCommit…) are called from
// the test's goroutine and report a setup error there with t.Fatalf; the HTTP handlers only use
// t.Errorf.
package githubtest

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// Token is the fake token expected by default. The tests check that it never appears in the logs
// or in the errors.
const Token = "gho_fake_for_tests"

// DefaultRateLimit is the default limit of the simulated quota (5,000 requests per hour, like GitHub).
const DefaultRateLimit = 5000

// quotaWindow is the duration of a quota window (one hour, like GitHub).
const quotaWindow = time.Hour

// shaPattern: full lowercase SHA, SHA-1 (40) or SHA-256 (64).
var shaPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// Options configures NewServer. Its zero value suits most tests.
type Options struct {
	// Token is the token expected in the Authorization header; empty = Token.
	Token string
	// Now is the server's clock (quota, reset, default Reset, Date header); nil = time.Now. It is
	// called under the server's lock.
	Now func() time.Time
}

// Server is a fake GitHub API server. It is stopped automatically at the end of the test
// (t.Cleanup).
type Server struct {
	tb        testing.TB
	srv       *httptest.Server
	token     string
	now       func() time.Time
	done      chan struct{} // closed by Close: interrupts the waits (Response.Delay)
	closeOnce sync.Once

	mu         sync.Mutex
	repos      map[int64]*repoState
	names      map[string]int64 // current name (nameKey) → repository identifier
	redirects  map[string]int64 // old name (nameKey) → repository identifier
	quota      quotaState
	secondary  *secondaryState // secondary limit in progress; nil = none
	injections map[string]*injection
	requests   []*Request
}

// repoState is the state of a simulated repository.
type repoState struct {
	Repo
	access   Access
	commits  map[string]*commitState
	seq      int               // number of commits added since creation (order of addition)
	branches map[string]string // name → head SHA
	pulls    map[int]PullRequest
	reviews  map[int][]Review // PR number → reviews (SetReviews), phase 8.6
	releases []Release
	tags     []Tag
}

// commitState is a simulated commit and its rank of addition in the repository. It is no longer
// modified after AddCommit.
type commitState struct {
	Commit
	seq int
}

// quotaState is the internal state of the simulated quota.
type quotaState struct {
	limit, remaining, used int
	reset                  time.Time
	status                 int
}

// secondaryState is a secondary limit in progress.
type secondaryState struct {
	retryAfter time.Duration
	status     int
	left       int // requests still refused
}

// injection is a response imposed by Inject.
type injection struct {
	resp Response
	left int // remaining requests; 0 = unlimited
}

// NewServer starts a fake server (httptest.NewServer) and schedules its stop with t.Cleanup. The
// initial quota is {Limit: DefaultRateLimit, Remaining: DefaultRateLimit, Reset: now + 1 h,
// truncated to the second, Status: 403}.
func NewServer(t testing.TB, opts Options) *Server {
	t.Helper()
	s := &Server{
		tb:         t,
		token:      opts.Token,
		now:        opts.Now,
		done:       make(chan struct{}),
		repos:      make(map[int64]*repoState),
		names:      make(map[string]int64),
		redirects:  make(map[string]int64),
		injections: make(map[string]*injection),
	}
	if s.token == "" {
		s.token = Token
	}
	if s.now == nil {
		s.now = time.Now
	}
	s.quota = quotaState{
		limit:     DefaultRateLimit,
		remaining: DefaultRateLimit,
		reset:     s.clock().Add(quotaWindow).Truncate(time.Second),
		status:    http.StatusForbidden,
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serveHTTP))
	t.Cleanup(s.Close)
	return s
}

// URL returns the root of the simulated API, with a trailing "/" (e.g. "http://127.0.0.1:54321/"), to
// pass to github.Options.BaseURL.
func (s *Server) URL() string { return s.srv.URL + "/" }

// Token returns the token expected by the server.
func (s *Server) Token() string { return s.token }

// Close stops the server; no effect if it is already stopped (it also is by t.Cleanup). The
// delayed responses (Response.Delay) are abandoned.
func (s *Server) Close() {
	s.closeOnce.Do(func() { close(s.done) })
	s.srv.Close()
}

// clock returns the server's time, in UTC.
func (s *Server) clock() time.Time { return s.now().UTC() }

// fatalf reports a setup error (test goroutine only).
func (s *Server) fatalf(format string, args ...any) {
	s.tb.Helper()
	s.tb.Fatalf("githubtest: "+format, args...)
}

// nameKey returns the key of a repository: "owner/name" in lowercase (GitHub ignores case).
func nameKey(owner, name string) string {
	return strings.ToLower(owner + "/" + name)
}

// utcSecond converts t to UTC to the second; the zero value stays zero.
func utcSecond(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC().Truncate(time.Second)
}

// ----- Data -----

// Repo describes a simulated repository.
type Repo struct {
	ID            int64     // numeric identifier, > 0 and unique; it never changes
	Owner         string    // owner (login), unique with Name, case-insensitively
	Name          string    // name
	Private       bool      // private repository ("private": true, "visibility": "private")
	Archived      bool      // archived repository
	DefaultBranch string    // default branch; empty = "main"
	Stars         int       // "stargazers_count"
	OpenIssues    int       // "open_issues_count"
	PushedAt      time.Time // "pushed_at"; zero = null
	// Push gives the fake account the push right ("permissions.push"). Like GitHub, the merge methods
	// below are only served to an account with this right (phase 8.5).
	Push bool
	// AllowSquashMerge, AllowMergeCommit and AllowRebaseMerge are "allow_squash_merge",
	// "allow_merge_commit" and "allow_rebase_merge"; nil = absent. Served only if Push.
	AllowSquashMerge *bool
	AllowMergeCommit *bool
	AllowRebaseMerge *bool
}

// Access configures the server's response for a repository.
type Access int

// Values of Access.
const (
	// AccessGranted: normal access.
	AccessGranted Access = iota
	// AccessHidden: 404 "Not Found" on all the routes of the repository, exactly like a private
	// repository without access (or deleted).
	AccessHidden
	// AccessSSO: 403 with the X-GitHub-SSO header on all the routes of the repository (the
	// organization enforces SSO and the token is not authorized there).
	AccessSSO
)

// Person is the author or the committer of a simulated commit.
type Person struct {
	Name  string // Git name ("commit.author.name")
	Email string // Git e-mail ("commit.author.email")
	// Login is the linked GitHub account; empty = no linked account: the "author" (or "committer")
	// field of the JSON is null.
	Login     string
	ID        int64  // identifier of the linked account; 0 = derived from Login (stable)
	AvatarURL string // empty = "https://avatars.example.test/u/{ID}"
}

// Commit is a simulated commit.
type Commit struct {
	SHA         string    // full SHA (40 or 64 lowercase hex); empty = deterministic SHA derived from the content and the order of addition
	Message     string    // message
	Author      Person    // author
	Committer   Person    // committer; zero = Author
	AuthoredAt  time.Time // authoring date; zero = CommittedAt
	CommittedAt time.Time // commit date; zero = AuthoredAt (both zero: setup error)
	Parents     []string  // SHAs of the parents, already added to the same repository; empty = root commit
	Additions   int       // "stats.additions"
	Deletions   int       // "stats.deletions"
}

// AddRepo adds a repository (without branch: it is empty as long as no branch is set). ID or
// owner/name already taken, ID <= 0, owner or name empty or containing a reserved URL character ("/",
// "?", "#", "%", "\") or a control character: t.Fatalf. An old name of a renamed repository can be
// taken again: its redirection disappears, like on GitHub.
func (s *Server) AddRepo(r Repo) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.ID <= 0:
		s.fatalf("AddRepo: invalid identifier %d (want > 0)", r.ID)
	case !validRepoName(r.Owner) || !validRepoName(r.Name):
		s.fatalf("AddRepo: invalid owner %q or name %q", r.Owner, r.Name)
	}
	if _, ok := s.repos[r.ID]; ok {
		s.fatalf("AddRepo: identifier %d already taken", r.ID)
	}
	key := nameKey(r.Owner, r.Name)
	if _, ok := s.names[key]; ok {
		s.fatalf("AddRepo: repository %s/%s already present", r.Owner, r.Name)
	}
	if r.DefaultBranch == "" {
		r.DefaultBranch = "main"
	}
	r.PushedAt = utcSecond(r.PushedAt)
	delete(s.redirects, key)
	s.names[key] = r.ID
	s.repos[r.ID] = &repoState{
		Repo:     r,
		commits:  make(map[string]*commitState),
		branches: make(map[string]string),
	}
}

// validRepoName reports whether s can be used as an owner or repository name in a path: not empty,
// without reserved URL character or control character.
func validRepoName(s string) bool {
	return s != "" && !strings.ContainsAny(s, "/\\?#%") && strings.IndexFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) < 0
}

// RenameRepo renames or transfers the repository id to newOwner/newName. All the routes of the old
// name then answer 301 to /repositories/{id}/…; the successive old names stay redirected.
// Name already taken by another repository: t.Fatalf.
func (s *Server) RenameRepo(id int64, newOwner, newName string) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("RenameRepo", id)
	if !validRepoName(newOwner) || !validRepoName(newName) {
		s.fatalf("RenameRepo: invalid owner %q or name %q", newOwner, newName)
	}
	newKey := nameKey(newOwner, newName)
	if other, ok := s.names[newKey]; ok && other != id {
		s.fatalf("RenameRepo: repository %s/%s already present", newOwner, newName)
	}
	oldKey := nameKey(repo.Owner, repo.Name)
	delete(s.names, oldKey)
	if oldKey != newKey {
		s.redirects[oldKey] = id
	}
	delete(s.redirects, newKey)
	s.names[newKey] = id
	repo.Owner, repo.Name = newOwner, newName
}

// SetMergeMethods sets the push right and the merge methods of the repository id (phase 8.5; nil =
// absent from the response). Unknown repository: t.Fatalf.
func (s *Server) SetMergeMethods(id int64, push bool, squash, merge, rebase *bool) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("SetMergeMethods", id)
	repo.Push = push
	repo.AllowSquashMerge, repo.AllowMergeCommit, repo.AllowRebaseMerge = squash, merge, rebase
}

// SetRepoAccess configures the access to the repository id (AccessGranted by default).
func (s *Server) SetRepoAccess(id int64, a Access) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("SetRepoAccess", id)
	if a < AccessGranted || a > AccessSSO {
		s.fatalf("SetRepoAccess: unknown access %d", a)
	}
	repo.access = a
}

// repoLocked returns the repository id (t.Fatalf if it is unknown). Called with s.mu locked.
func (s *Server) repoLocked(op string, id int64) *repoState {
	s.tb.Helper()
	repo, ok := s.repos[id]
	if !ok {
		s.fatalf("%s: unknown repository %d", op, id)
	}
	return repo
}

// AddCommit adds a commit to the repository repoID and returns its SHA. Malformed or already present
// SHA, unknown parent, both dates zero or unknown repository: t.Fatalf. Adding a commit moves no
// branch (see SetBranch).
func (s *Server) AddCommit(repoID int64, c Commit) string {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addCommitLocked(s.repoLocked("AddCommit", repoID), c)
}

// addCommitLocked adds c to repo. Called with s.mu locked.
func (s *Server) addCommitLocked(repo *repoState, c Commit) string {
	s.tb.Helper()
	if c.CommittedAt.IsZero() && c.AuthoredAt.IsZero() {
		s.fatalf("AddCommit: commit without date (AuthoredAt and CommittedAt zero)")
	}
	if c.CommittedAt.IsZero() {
		c.CommittedAt = c.AuthoredAt
	}
	if c.AuthoredAt.IsZero() {
		c.AuthoredAt = c.CommittedAt
	}
	c.AuthoredAt, c.CommittedAt = utcSecond(c.AuthoredAt), utcSecond(c.CommittedAt)
	if c.Committer == (Person{}) {
		c.Committer = c.Author
	}
	for _, p := range c.Parents {
		if _, ok := repo.commits[p]; !ok {
			s.fatalf("AddCommit: unknown parent %s in repository %d", p, repo.ID)
		}
	}
	c.Parents = append([]string{}, c.Parents...)
	repo.seq++
	if c.SHA == "" {
		c.SHA = FakeSHA(fmt.Sprintf("githubtest/%d/%d/%s/%s/%s", repo.ID, repo.seq, c.Message,
			c.CommittedAt.Format(time.RFC3339), strings.Join(c.Parents, ",")))
	} else if !shaPattern.MatchString(c.SHA) {
		s.fatalf("AddCommit: malformed SHA %q (want 40 or 64 lowercase hex)", c.SHA)
	}
	if _, ok := repo.commits[c.SHA]; ok {
		s.fatalf("AddCommit: commit %s already present in repository %d", c.SHA, repo.ID)
	}
	repo.commits[c.SHA] = &commitState{Commit: c, seq: repo.seq}
	return c.SHA
}

// SetBranch creates the branch name of the repository repoID, or moves it (including backwards:
// force-push), to the commit headSHA, which must exist (otherwise t.Fatalf). Empty name: t.Fatalf.
func (s *Server) SetBranch(repoID int64, name, headSHA string) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("SetBranch", repoID)
	if name == "" {
		s.fatalf("SetBranch: empty branch name")
	}
	if _, ok := repo.commits[headSHA]; !ok {
		s.fatalf("SetBranch: unknown commit %s in repository %d", headSHA, repoID)
	}
	repo.branches[name] = headSHA
}

// DeleteBranch deletes the branch name of the repository repoID (the commits stay). Unknown branch:
// t.Fatalf.
func (s *Server) DeleteBranch(repoID int64, name string) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("DeleteBranch", repoID)
	if _, ok := repo.branches[name]; !ok {
		s.fatalf("DeleteBranch: unknown branch %q in repository %d", name, repoID)
	}
	delete(repo.branches, name)
}

// RemoveCommit removes the commit sha from the repository repoID, like GitHub's cleanup after a
// force-push: a compare whose base it is then answers 404. The commit must no longer be the head of
// a branch (otherwise t.Fatalf); the commits referencing it as a parent keep it in "parents" (the
// history walk stops there).
func (s *Server) RemoveCommit(repoID int64, sha string) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("RemoveCommit", repoID)
	if _, ok := repo.commits[sha]; !ok {
		s.fatalf("RemoveCommit: unknown commit %s in repository %d", sha, repoID)
	}
	for name, head := range repo.branches {
		if head == sha {
			s.fatalf("RemoveCommit: commit %s is the head of branch %q", sha, name)
		}
	}
	delete(repo.commits, sha)
}

// AppendCommits adds n linear commits on top of the head of branch (or a new history if the branch
// does not exist), dated start, start+step, start+2*step… (authoring date = commit date), with
// messages "commit 1" … "commit n", by the dummy author {Name: "Étudiant Test", Email:
// "etudiant@example.test", Login: "etudiant-test"} (same committer), then puts branch on the last
// one. Returns the SHAs in creation order (from oldest to newest; empty non-nil slice if n is 0,
// without touching the branch). Negative n, empty branch or zero start: t.Fatalf.
func (s *Server) AppendCommits(repoID int64, branch string, n int, start time.Time, step time.Duration) []string {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("AppendCommits", repoID)
	switch {
	case branch == "":
		s.fatalf("AppendCommits: empty branch name")
	case n < 0:
		s.fatalf("AppendCommits: negative number of commits %d", n)
	case n > 0 && start.IsZero():
		s.fatalf("AppendCommits: zero start date")
	}
	author := Person{Name: "Étudiant Test", Email: "etudiant@example.test", Login: "etudiant-test"}
	parent := repo.branches[branch]
	shas := make([]string, 0, n)
	for i := range n {
		at := start.Add(time.Duration(i) * step)
		c := Commit{Message: fmt.Sprintf("commit %d", i+1), Author: author, AuthoredAt: at, CommittedAt: at}
		if parent != "" {
			c.Parents = []string{parent}
		}
		parent = s.addCommitLocked(repo, c)
		shas = append(shas, parent)
	}
	if n > 0 {
		repo.branches[branch] = parent
	}
	return shas
}

// FakeSHA returns a dummy, deterministic SHA-1 (40 lowercase hex) derived from seed.
func FakeSHA(seed string) string {
	sum := sha1.Sum([]byte(seed))
	return hex.EncodeToString(sum[:])
}

// ----- Quota -----

// Quota is the state of the simulated quota (core resource).
type Quota struct {
	Limit     int       // requests per window; 0 = DefaultRateLimit
	Remaining int       // remaining requests
	Reset     time.Time // end of the window (truncated to the second); zero = now + 1 h
	Status    int       // code returned when the quota is exhausted: 403 (default, 0) or 429
}

// SetQuota replaces the quota state (Used = Limit - Remaining). Remaining at 0 with a future Reset:
// the following requests (except /rate_limit) get the quota refusal. Negative Limit or Remaining,
// Status other than 0, 403 or 429: t.Fatalf.
func (s *Server) SetQuota(q Quota) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case q.Limit < 0 || q.Remaining < 0:
		s.fatalf("SetQuota: Limit (%d) and Remaining (%d) must be positive", q.Limit, q.Remaining)
	case q.Status != 0 && q.Status != http.StatusForbidden && q.Status != http.StatusTooManyRequests:
		s.fatalf("SetQuota: invalid code %d (want 403 or 429)", q.Status)
	}
	if q.Limit == 0 {
		q.Limit = DefaultRateLimit
	}
	if q.Reset.IsZero() {
		q.Reset = s.clock().Add(quotaWindow)
	}
	if q.Status == 0 {
		q.Status = http.StatusForbidden
	}
	s.quota = quotaState{
		limit:     q.Limit,
		remaining: q.Remaining,
		used:      max(q.Limit-q.Remaining, 0),
		reset:     q.Reset.UTC().Truncate(time.Second),
		status:    q.Status,
	}
}

// Quota returns the current quota state (after the consumptions; an expired window is only
// renewed at the next request).
func (s *Server) Quota() Quota {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Quota{Limit: s.quota.limit, Remaining: s.quota.remaining, Reset: s.quota.reset, Status: s.quota.status}
}

// SecondaryLimit describes a secondary (anti-abuse) limit to simulate.
type SecondaryLimit struct {
	// RetryAfter is the value of the Retry-After header, rounded to the second (1 s at least); 0 =
	// no Retry-After header (the "secondary rate limit" message stays present).
	RetryAfter time.Duration
	// Status is the returned code: 403 (default, 0) or 429.
	Status int
	// Requests is the number of refused requests before going back to normal; 0 = 1.
	Requests int
}

// TriggerSecondaryLimit makes the next requests (except /rate_limit) refused by a secondary
// limit: code l.Status, Retry-After if l.RetryAfter > 0, normal quota headers
// (X-RateLimit-Remaining not zero), body {"message":"You have exceeded a secondary rate limit. Please
// wait a few minutes before you try again.","documentation_url":"https://docs.github.com/rest/overview/
// rate-limits-for-the-rest-api#about-secondary-rate-limits"}. These refusals consume no quota. A
// new call replaces the limit in progress.
func (s *Server) TriggerSecondaryLimit(l SecondaryLimit) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case l.RetryAfter < 0 || l.Requests < 0:
		s.fatalf("TriggerSecondaryLimit: RetryAfter (%s) and Requests (%d) must be positive", l.RetryAfter, l.Requests)
	case l.Status != 0 && l.Status != http.StatusForbidden && l.Status != http.StatusTooManyRequests:
		s.fatalf("TriggerSecondaryLimit: invalid code %d (want 403 or 429)", l.Status)
	}
	if l.Status == 0 {
		l.Status = http.StatusForbidden
	}
	if l.Requests == 0 {
		l.Requests = 1
	}
	s.secondary = &secondaryState{retryAfter: l.RetryAfter, status: l.Status, left: l.Requests}
}

// ----- Injected responses -----

// Response is a response imposed by Inject.
type Response struct {
	Status int         // HTTP code; 0 = 200
	Header http.Header // added headers (they take precedence over the server's headers: quota, Content-Type)
	Body   string      // body, served as is (Content-Type "application/json; charset=utf-8" unless a header says otherwise)
	Times  int         // number of requests concerned; 0 = all, until ClearInjections
	// Delay is waited before answering (to test Options.Timeout); the wait is interrupted if the
	// client abandons the request (no response is then written) or if the server stops.
	Delay time.Duration
}

// Inject imposes the response r on the authorized requests whose path (r.URL.Path) is exactly
// path, for example "/repos/alice/projet"; a new injection for the same path replaces the
// previous one. These responses consume quota like the others (except code 304 and /rate_limit).
// Path without a leading "/", negative Times or Delay: t.Fatalf.
func (s *Server) Inject(path string, r Response) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case !strings.HasPrefix(path, "/"):
		s.fatalf("Inject: path %q without a leading \"/\"", path)
	case r.Times < 0 || r.Delay < 0:
		s.fatalf("Inject: Times (%d) and Delay (%s) must be positive", r.Times, r.Delay)
	}
	if r.Status == 0 {
		r.Status = http.StatusOK
	}
	r.Header = r.Header.Clone()
	s.injections[path] = &injection{resp: r, left: r.Times}
}

// ClearInjections removes all the injected responses.
func (s *Server) ClearInjections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.injections)
}

// ----- Log -----

// Request is a request received by the server. It never keeps the value of the Authorization
// header.
type Request struct {
	Method           string     // HTTP method
	Path             string     // path (r.URL.Path), e.g. "/repos/alice/projet/commits"
	RawQuery         string     // raw parameters, e.g. "per_page=100&sha=main"
	Query            url.Values // decoded parameters
	IfNoneMatch      string     // If-None-Match header received; empty if missing
	UserAgent        string     // User-Agent header received
	HasAuthorization bool       // the Authorization header was present
	ValidToken       bool       // it carried the expected token
	Status           int        // HTTP code returned (or planned, if the client gave up during Response.Delay)
}

// Requests returns a copy of the log of received requests, in order of arrival.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.requests))
	for i, r := range s.requests {
		out[i] = *r
		out[i].Query = cloneValues(r.Query)
	}
	return out
}

// ResetRequests empties the log.
func (s *Server) ResetRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = nil
}

// Count returns the number of received requests whose path is exactly path.
func (s *Server) Count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.requests {
		if r.Path == path {
			n++
		}
	}
	return n
}

// cloneValues copies v (nil stays nil).
func cloneValues(v url.Values) url.Values {
	if v == nil {
		return nil
	}
	out := make(url.Values, len(v))
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
